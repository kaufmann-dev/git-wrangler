package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
	"github.com/kaufmann-dev/git-wrangler/internal/githubcli"
	"github.com/spf13/cobra"
)

func runClone(a *app, cmd *cobra.Command, args []string) int {
	visibility, _ := cmd.Flags().GetString("visibility")
	user, _ := cmd.Flags().GetString("user")
	limitInt, _ := cmd.Flags().GetInt("limit")
	into, _ := cmd.Flags().GetString("into")

	var ok bool
	user, ok = requiredStringFlag(a, cmd, "user", "GitHub user or organization: ")
	if !ok {
		return 1
	}
	if !requireCommand(a, "gh", "clone") || !requireGit(a, "clone") {
		return 1
	}
	if limitInt < 1 {
		a.error("--limit must be 1 or greater.")
		return 1
	}
	if visibility != "all" && visibility != "public" && visibility != "private" {
		a.error("Invalid visibility option. Use 'all', 'public', or 'private'.")
		return 1
	}
	ghEnv := githubcli.UnauthenticatedEnv()
	if visibility == "private" || visibility == "all" {
		resolvedEnv, authSource, ok, err := githubAuthEnv(a)
		if err != nil {
			if errors.Is(err, errGitHubCredentialStorageUnavailable) {
				a.error("Secure credential storage is unavailable. Set GIT_WRANGLER_GITHUB_TOKEN, or use --visibility public to clone only public repositories.")
			} else {
				a.error(err.Error())
			}
			return 1
		}
		if !ok {
			a.errorf("Git Wrangler GitHub auth is required for %s repository cloning. Run 'git-wrangler init' or 'git-wrangler config set github.auth'.", visibility)
			return 1
		}
		ghEnv = resolvedEnv
		renderStatusLine(a, a.stdout, statusInfo, "GitHub auth", string(authSource))
	}

	listArgs := githubcli.RepoListArgs(user, visibility, "1")
	out, err := a.gh.StdoutEnv(a.ctx, "", ghEnv, listArgs...)
	if err != nil {
		fmt.Fprintf(a.stderr, "%sError: Failed to list repositories:\n%s%s\n\n", a.ui.Red, err.Error(), a.ui.Reset)
		return 1
	}
	if lineCount(out) == 0 {
		if visibility == "public" || visibility == "private" {
			a.errorf("No %s repositories found for '%s'.", visibility, user)
		} else {
			a.errorf("No repositories found for '%s'.", user)
		}
		return 1
	}

	if into == "" {
		into = user
	}
	if info, err := os.Stat(into); err == nil && !info.IsDir() {
		a.errorf("Unable to create or access the specified directory '%s'.", into)
		return 1
	}
	if err := os.MkdirAll(into, 0o755); err != nil {
		a.errorf("Unable to create or access the specified directory '%s'.", into)
		return 1
	}

	listArgs = githubcli.RepoListArgs(user, visibility, strconv.Itoa(limitInt))
	reposOut, err := a.gh.StdoutEnv(a.ctx, "", ghEnv, listArgs...)
	if err != nil {
		fmt.Fprintf(a.stderr, "%sError: Failed to list repositories:\n%s%s\n\n", a.ui.Red, err.Error(), a.ui.Reset)
		return 1
	}
	status := 0
	cloned := 0
	skipped := 0
	failed := 0
	successLines := []string{}
	results := []cloneResult{}
	progress := newProgress(a, "Cloning repositories", lineCount(reposOut))
	for _, line := range splitLines(reposOut) {
		fields := strings.Split(line, "\t")
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		progress.start(fields[0])
		result := cloneRepository(a, ghEnv, fields[0], into)
		progress.advance(fields[0])
		results = append(results, result)
	}
	finishProgressBeforeOutput(progress)
	for _, result := range results {
		switch {
		case result.err != nil:
			status = 1
			failed++
			renderErrorBlock(a, result.name+": clone failed", outputOrError(result.output, result.err))
		case result.skipped:
			skipped++
			renderStatusLine(a, a.stdout, statusSkip, result.name, result.detail)
		default:
			cloned++
			successLines = append(successLines, fmt.Sprintf("Cloned %s into %s", result.name, result.detail))
		}
	}
	if cloned <= 2 {
		for _, line := range successLines {
			renderStatusLine(a, a.stdout, statusOK, line, "")
		}
	}
	renderSummary(a,
		summaryCount{label: "cloned", value: cloned, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	return status
}

type cloneResult struct {
	name    string
	detail  string
	output  string
	skipped bool
	err     error
}

var errGitHubCredentialStorageUnavailable = errors.New("GitHub credential storage unavailable")

func cloneRepository(a *app, ghEnv []string, fullName, into string) cloneResult {
	repoName := fullName
	if idx := strings.LastIndex(fullName, "/"); idx >= 0 {
		repoName = fullName[idx+1:]
	}
	target := filepath.Join(into, repoName)
	if isDir(target) {
		abs, _ := filepath.Abs(into)
		return cloneResult{name: repoName, detail: "already exists in " + abs, skipped: true}
	}
	if out, err := a.gh.CaptureEnv(a.ctx, "", ghEnv, "repo", "clone", fullName, target); err == nil {
		abs, _ := filepath.Abs(target)
		return cloneResult{name: repoName, detail: abs}
	} else {
		return cloneResult{name: repoName, output: out, err: err}
	}
}

func githubAuthEnv(a *app) ([]string, credentials.Source, bool, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, credentials.SourceMissing, false, err
	}
	resolved := credentials.ResolveGitHubToken(a.creds, cfg.GitHub.Host)
	if resolved.Err != nil {
		return nil, resolved.Source, false, errGitHubCredentialStorageUnavailable
	}
	if resolved.Value == "" {
		return nil, resolved.Source, false, nil
	}
	return githubcli.Env(resolved.Value, cfg.GitHub.Host), resolved.Source, true, nil
}
