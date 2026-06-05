package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type originRefreshResult struct {
	repo repo
	out  string
	err  error
}

func noFetchFlagValue(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Flags().Lookup("no-fetch") == nil {
		return false
	}
	value, _ := cmd.Flags().GetBool("no-fetch")
	return value
}

func refreshOrigin(a *app, repos []repo) []originRefreshResult {
	return parallelGitMutationsProgress(a.ctx, repos, newProgress(a, "Fetching repositories", len(repos)), func(r repo) originRefreshResult {
		out, err := a.git.Capture(a.ctx, r.dir, nil, "fetch", "--prune", "origin")
		return originRefreshResult{repo: r, out: out, err: err}
	})
}

func refreshFailuresByDir(results []originRefreshResult) map[string]originRefreshResult {
	failures := map[string]originRefreshResult{}
	for _, result := range results {
		if result.err != nil {
			failures[result.repo.dir] = result
		}
	}
	return failures
}

func fetchFailureMessage(result originRefreshResult) string {
	return "git fetch failed: " + outputOrError(result.out, result.err)
}

func refreshOriginForRewrite(a *app, cmd *cobra.Command, repos []repo) bool {
	if noFetchFlagValue(cmd) {
		renderWarning(a, "Using local remote-tracking refs without fetching first; remote-only commits may be missed.")
		return true
	}
	results := refreshOrigin(a, repos)
	if interrupted(a) {
		return false
	}
	failed := 0
	for _, result := range results {
		if result.err == nil {
			continue
		}
		renderErrorBlock(a, fmt.Sprintf("%s: git fetch failed", result.repo.display), outputOrError(result.out, result.err))
		failed++
	}
	if failed == 0 {
		return true
	}
	renderSummary(a, summaryCount{label: "fetch failed", value: failed, color: a.ui.Red})
	return false
}
