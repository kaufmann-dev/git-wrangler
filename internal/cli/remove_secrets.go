package cli

import (
	"errors"
	"fmt"
	"path"
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
	repos, err := resolveRepositoryTargets("")
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
	scans := parallelRepos(repos, func(r repo) secretScan {
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
	status := 0
	for _, scan := range scans {
		r := scan.repo
		if scan.invalidRepo {
			fmt.Fprintf(a.stderr, "%sError: %s is not a valid or accessible git repository. Skipping...%s\n", a.ui.Red, r.display, a.ui.Reset)
			status = 1
			continue
		}
		if scan.err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not scan history for %s:\n%s%s\n\n", a.ui.Red, r.display, scan.err.Error(), a.ui.Reset)
			status = 1
			continue
		}
		matchedPatterns := scan.matchedPatterns
		matchedFiles := scan.matchedFiles
		if len(matchedPatterns) == 0 {
			fmt.Fprintf(a.stdout, "%sNo target patterns found in history. Skipping %s cleanly...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stdout, "%sFound %d sensitive file(s) matching %d pattern(s) in %s:%s\n", a.ui.Yellow, len(matchedFiles), len(matchedPatterns), r.display, a.ui.Reset)
		for _, file := range matchedFiles {
			fmt.Fprintf(a.stdout, "  %s\n", file)
		}
		fmt.Fprintln(a.stdout)
		if !yes && !confirm(a, "Purge these files from history for "+r.display+"?") {
			a.error(r.display, "Refusing to rewrite history without confirmation.")
			status = 1
			continue
		}
		fmt.Fprintf(a.stderr, "%sWARNING: This operation rewrites Git history. A force push will be required to update any remote.%s\n", a.ui.Red, a.ui.Reset)
		filterArgs := []string{}
		for _, pattern := range matchedPatterns {
			filterArgs = append(filterArgs, "--path-glob", pattern)
		}
		filterArgs = append(filterArgs, "--invert-paths", "--partial", "--force")
		out, err, restoreErr := runFilterRepoRestoringOrigin(a, r.dir, filterCmd, filterArgs, nil)
		if err == nil {
			fmt.Fprintf(a.stdout, "%sSuccessfully purged %d sensitive file(s) from %s%s\n", a.ui.Green, len(matchedFiles), r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Rewrite failed for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			if restoreErr != nil {
				fmt.Fprintf(a.stderr, "%sWarning: Rewrite failed for %s, and origin could not be restored:\n%s%s\n\n", a.ui.Red, r.display, restoreErr.Error(), a.ui.Reset)
			}
			status = 1
			continue
		}
		if restoreErr != nil {
			fmt.Fprintf(a.stderr, "%sWarning: Secret removal completed for %s, but origin could not be restored:\n%s%s\n\n", a.ui.Red, r.display, restoreErr.Error(), a.ui.Reset)
			status = 1
		}
	}
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

func runFilterRepo(a *app, dir string, filterCmd []string, args []string, env []string) (string, error) {
	if len(filterCmd) == 0 {
		return "", errors.New("missing filter command")
	}
	return a.runCapture(dir, env, filterCmd[0], append(filterCmd[1:], args...)...)
}

func runFilterRepoRestoringOrigin(a *app, dir string, filterCmd []string, args []string, env []string) (string, error, error) {
	remoteURL := originURL(a, dir)
	out, err := runFilterRepo(a, dir, filterCmd, args, env)
	restoreErr := restoreOrigin(a, dir, remoteURL)
	return out, err, restoreErr
}
