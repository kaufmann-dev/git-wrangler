package cli

type originRefreshResult struct {
	repo repo
	out  string
	err  error
}

func refreshOrigin(a *app, repos []repo) []originRefreshResult {
	return parallelGitMutationsProgress(a.ctx, repos, newProgress(a, "Fetching repositories", len(repos)), func(r repo) originRefreshResult {
		out, err := captureRemoteGitWithRetry(a, r.dir, nil, "fetch", "--prune", "origin")
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
	return remoteGitFailureMessage("fetch", result.out, result.err)
}

func refreshOriginForRewriteOptions(a *app, opts fetchOptions, repos []repo) bool {
	if opts.noFetch {
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
		renderRemoteGitFailure(a, result.repo, "fetch", result.out, result.err)
		failed++
	}
	if failed == 0 {
		return true
	}
	renderSummary(a, summaryCount{label: "fetch failed", value: failed, color: a.ui.Red})
	renderWarning(a, "Use --no-fetch only when local remote-tracking refs are acceptable.")
	return false
}
