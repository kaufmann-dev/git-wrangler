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

type cloneOptions struct {
	visibility string
	user       string
	limit      int
	into       string
}

func cloneOptionsFromCommand(a *app, cmd *cobra.Command) (cloneOptions, bool) {
	user, ok := requiredStringFlag(a, cmd, "user", "GitHub user or organization: ")
	if !ok {
		return cloneOptions{}, false
	}
	return cloneOptions{
		visibility: stringFlagValue(cmd, "visibility"),
		user:       user,
		limit:      intFlagValue(cmd, "limit"),
		into:       stringFlagValue(cmd, "into"),
	}, true
}

func runClone(a *app, cmd *cobra.Command, args []string) int {
	opts, ok := cloneOptionsFromCommand(a, cmd)
	if !ok {
		return 1
	}
	if !requireCommand(a, "gh", "clone") || !requireGit(a, "clone") {
		return 1
	}
	if opts.limit < 1 {
		a.plainErrorf("--limit must be 1 or greater.")
		return 1
	}
	if !validateStringEnum(opts.visibility, "all", "public", "private") {
		a.plainErrorf("Invalid visibility option. Use 'all', 'public', or 'private'.")
		return 1
	}
	ghEnv := githubcli.UnauthenticatedEnv()
	if opts.visibility == "private" || opts.visibility == "all" {
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
			a.errorf("Git Wrangler GitHub auth is required for %s repository cloning. Run 'git-wrangler init' or 'git-wrangler config set github.auth'.", opts.visibility)
			return 1
		}
		ghEnv = resolvedEnv
		renderStatusLine(a, a.stdout, statusInfo, "GitHub auth", string(authSource))
	}

	listArgs := githubcli.RepoListArgs(opts.user, opts.visibility, "1")
	out, err := stdoutGitHubWithRetry(a, "", ghEnv, listArgs...)
	if err != nil {
		renderErrorBlock(a, "could not list repositories", err.Error())
		return 1
	}
	if lineCount(out) == 0 {
		if opts.visibility == "public" || opts.visibility == "private" {
			renderStatusLine(a, a.stdout, statusSkip, fmt.Sprintf("no %s repositories found for '%s'", opts.visibility, opts.user), "")
		} else {
			renderStatusLine(a, a.stdout, statusSkip, fmt.Sprintf("no repositories found for '%s'", opts.user), "")
		}
		renderSummary(a,
			summaryCount{label: "cloned", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: 0, color: a.ui.Yellow},
			summaryCount{label: "failed", value: 0, color: a.ui.Red},
		)
		return 0
	}

	if opts.into == "" {
		opts.into = opts.user
	}
	if info, err := os.Stat(opts.into); err == nil && !info.IsDir() {
		a.errorf("Unable to create or access the specified directory '%s'.", opts.into)
		return 1
	}
	if err := os.MkdirAll(opts.into, 0o755); err != nil {
		a.errorf("Unable to create or access the specified directory '%s'.", opts.into)
		return 1
	}

	listArgs = githubcli.RepoListArgs(opts.user, opts.visibility, strconv.Itoa(opts.limit))
	reposOut, err := stdoutGitHubWithRetry(a, "", ghEnv, listArgs...)
	if err != nil {
		renderErrorBlock(a, "could not list repositories", err.Error())
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
		result := cloneRepository(a, ghEnv, fields[0], opts.into)
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
	ownedTarget := !fileExists(target)
	var out string
	var err error
	for attempt := 1; attempt <= remoteRetryAttempts; attempt++ {
		out, err = a.gh.CaptureEnv(a.ctx, "", ghEnv, "repo", "clone", fullName, target)
		if err == nil {
			abs, _ := filepath.Abs(target)
			return cloneResult{name: repoName, detail: abs}
		}
		if a.ctx.Err() != nil || !isTransientRemoteFailure(out, err) || attempt == remoteRetryAttempts {
			return cloneResult{name: repoName, output: out, err: err}
		}
		if ownedTarget {
			if cleanupErr := os.RemoveAll(target); cleanupErr != nil {
				return cloneResult{name: repoName, output: outputOrError(out, err), err: cleanupErr}
			}
		}
		if !waitRemoteRetry(a.ctx, attempt) {
			return cloneResult{name: repoName, output: out, err: a.ctx.Err()}
		}
	}
	return cloneResult{name: repoName, output: out, err: err}
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
