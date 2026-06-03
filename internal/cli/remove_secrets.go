package cli

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func runRemoveSecrets(a *app, cmd *cobra.Command, args []string) int {
	yes := yesFlag(cmd)
	if !requireGit(a, "remove-secrets") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "remove-secrets")
	if !ok {
		return 1
	}
	repos, err := commandRepositoryTargets(cmd)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	patterns := []string{
		".env", ".env.*", ".npmrc", ".pypirc", ".netrc", ".git-credentials",
		"*.pem", "*.key", "*.p12", "*.pfx", "*.asc", "*.gpg", "*.crt", "*.cer", "*.cert",
		"id_rsa", "id_rsa.pub", "id_ed25519", "id_ed25519.pub", "*_rsa", "*_ed25519",
		"secrets.json", "credentials.json", "*secret*.json", "*credential*.json", "*.secret",
		"config/credentials.yml.enc", ".docker/config.json", ".kube/config", "kubeconfig",
		".aws/credentials", ".aws/config", ".config/gcloud/*", "application_default_credentials.json",
		"azureProfile.json", "accessTokens.json",
	}
	type secretScan struct {
		repo            repo
		err             error
		invalidRepo     bool
		matchedPatterns []string
		matchedFiles    []string
	}
	scans := parallelReposProgress(a.ctx, repos, newProgress(a, "Scanning history for secrets", len(repos)), func(r repo) secretScan {
		if _, err := a.git.Capture(a.ctx, r.dir, nil, "rev-parse", "--is-inside-work-tree"); err != nil {
			return secretScan{repo: r, err: err, invalidRepo: true}
		}
		args := append([]string{"log", "--all", "--format=", "--name-only", "--"}, patterns...)
		files, err := a.git.Stdout(a.ctx, r.dir, nil, args...)
		if err != nil {
			return secretScan{repo: r, err: err}
		}
		matchedFiles := sortedUnique(splitLines(files))
		return secretScan{
			repo:            r,
			matchedFiles:    matchedFiles,
			matchedPatterns: matchedSecretPatterns(patterns, matchedFiles),
		}
	})
	if interrupted(a) {
		return 1
	}
	status := 0
	type secretApply struct {
		repo         repo
		filterArgs   []string
		matchedFiles []string
	}
	type secretApplyResult struct {
		apply      secretApply
		output     string
		err        error
		restoreErr error
	}
	applies := []secretApply{}
	clean := 0
	scanFailed := 0
	for _, scan := range scans {
		r := scan.repo
		if scan.invalidRepo {
			renderErrorBlock(a, r.display+": not a valid or accessible git repository", scan.err.Error())
			status = 1
			scanFailed++
			continue
		}
		if scan.err != nil {
			renderErrorBlock(a, r.display+": could not scan history", scan.err.Error())
			status = 1
			scanFailed++
			continue
		}
		matchedPatterns := scan.matchedPatterns
		matchedFiles := scan.matchedFiles
		if len(matchedPatterns) == 0 {
			clean++
			continue
		}
		renderRepoHeader(a, r.display)
		fmt.Fprintf(a.stdout, "  %sMatched files:%s %d across %d pattern(s)\n", a.ui.Yellow, a.ui.Reset, len(matchedFiles), len(matchedPatterns))
		for _, file := range matchedFiles {
			fmt.Fprintf(a.stdout, "  %s\n", file)
		}
		fmt.Fprintln(a.stdout)
		filterArgs := []string{}
		for _, pattern := range matchedPatterns {
			filterArgs = append(filterArgs, "--path-glob", pattern)
		}
		filterArgs = append(filterArgs, "--invert-paths", "--partial", "--force")
		applies = append(applies, secretApply{repo: r, filterArgs: filterArgs, matchedFiles: matchedFiles})
	}
	renderSummary(a,
		summaryCount{label: "with matches", value: len(applies), color: a.ui.Yellow},
		summaryCount{label: "clean", value: clean, color: a.ui.Green},
		summaryCount{label: "failed", value: scanFailed, color: a.ui.Red},
	)
	if len(applies) > 0 {
		renderWarning(a, fmt.Sprintf("This operation rewrites Git history in %d repositories. A force push will be required to update any remote.", len(applies)))
		if !confirmOrSkip(a, yes, fmt.Sprintf("Purge these files from history in %d repositories?", len(applies))) {
			renderSummary(a,
				summaryCount{label: "rewritten", value: 0, color: a.ui.Green},
				summaryCount{label: "clean", value: clean, color: a.ui.Green},
				summaryCount{label: "skipped", value: len(applies), color: a.ui.Yellow},
				summaryCount{label: "failed", value: scanFailed, color: a.ui.Red},
			)
			return status
		}
	}
	results := parallelItemsWithWorkersProgress(a.ctx, applies, gitMutationWorkerCount(len(applies)), newProgress(a, "Removing secrets", len(applies)), func(apply secretApply) (string, string) {
		return apply.repo.display, apply.repo.display
	}, func(apply secretApply) secretApplyResult {
		out, err, restoreErr := runFilterRepoRestoringOrigin(a, apply.repo.dir, apply.repo.gitDir, filterCmd, apply.filterArgs, nil)
		return secretApplyResult{apply: apply, output: out, err: err, restoreErr: restoreErr}
	})
	if interrupted(a) {
		return 1
	}
	rewritten := 0
	applyFailed := 0
	for _, result := range results {
		r := result.apply.repo
		if result.err == nil {
			if result.restoreErr != nil {
				renderErrorBlock(a, r.display+": secret removal completed, but origin could not be restored", result.restoreErr.Error())
				status = 1
				applyFailed++
				continue
			}
			rewritten++
			continue
		}
		renderErrorBlock(a, r.display+": rewrite failed", result.output)
		if result.restoreErr != nil {
			renderErrorBlock(a, r.display+": rewrite failed, and origin could not be restored", result.restoreErr.Error())
		}
		status = 1
		applyFailed++
	}
	renderSummary(a,
		summaryCount{label: "rewritten", value: rewritten, color: a.ui.Green},
		summaryCount{label: "clean", value: clean, color: a.ui.Green},
		summaryCount{label: "skipped", value: 0, color: a.ui.Yellow},
		summaryCount{label: "failed", value: scanFailed + applyFailed, color: a.ui.Red},
	)
	return status
}

func matchedSecretPatterns(patterns, files []string) []string {
	matched := []string{}
	for _, pattern := range patterns {
		for _, file := range files {
			if secretPatternMatches(pattern, file) {
				matched = append(matched, pattern)
				break
			}
		}
	}
	return matched
}

func secretPatternMatches(pattern, file string) bool {
	if pattern == file {
		return true
	}
	if ok, _ := path.Match(pattern, file); ok {
		return true
	}
	if !strings.Contains(pattern, "/") {
		if ok, _ := path.Match(pattern, path.Base(file)); ok {
			return true
		}
	}
	return false
}

func runFilterRepo(a *app, dir, gitDir string, filterCmd []string, args []string, env []string) (string, error) {
	if len(filterCmd) == 0 {
		return "", errors.New("missing filter command")
	}
	if err := removeFilterRepoAlreadyRan(gitDir); err != nil {
		return "", err
	}
	return a.runCapture(dir, env, filterCmd[0], append(filterCmd[1:], args...)...)
}

func removeFilterRepoAlreadyRan(gitDir string) error {
	if gitDir == "" {
		return nil
	}
	metadataDir := gitDir
	if info, err := os.Stat(gitDir); err != nil {
		return fmt.Errorf("could not inspect git-filter-repo state: %w", err)
	} else if !info.IsDir() {
		data, err := os.ReadFile(gitDir)
		if err != nil {
			return fmt.Errorf("could not inspect git-filter-repo state: %w", err)
		}
		target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
		if !ok {
			return nil
		}
		metadataDir = strings.TrimSpace(target)
		if !filepath.IsAbs(metadataDir) {
			metadataDir = filepath.Join(filepath.Dir(gitDir), metadataDir)
		}
	}
	marker := filepath.Join(metadataDir, "filter-repo", "already_ran")
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not clear git-filter-repo continuation marker: %w", err)
	}
	return nil
}

func runFilterRepoRestoringOrigin(a *app, dir, gitDir string, filterCmd []string, args []string, env []string) (string, error, error) {
	remoteURL := originURL(a, dir)
	out, err := runFilterRepo(a, dir, gitDir, filterCmd, args, env)
	restoreErr := restoreOrigin(a, dir, remoteURL)
	return out, err, restoreErr
}
