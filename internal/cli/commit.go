package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func runCommit(a *app, cmd *cobra.Command, args []string) int {
	message, ok := requiredStringFlag(a, cmd, "message", "Commit message: ")
	if !ok {
		return 1
	}
	if !requireGit(a, "commit") {
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
	type commitResult struct {
		repo      repo
		out       string
		err       error
		staged    bool
		skipped   bool
		committed bool
	}
	status := 0
	committed := 0
	skipped := 0
	failed := 0
	results := parallelGitMutationsProgress(repos, newProgress(a, "Committing repositories", len(repos)), func(r repo) commitResult {
		if _, err := a.git.Capture(a.ctx, r.dir, nil, "add", "-A"); err != nil {
			return commitResult{repo: r, err: err}
		}
		if _, err := a.git.Capture(a.ctx, r.dir, nil, "diff", "--cached", "--quiet"); err == nil {
			return commitResult{repo: r, staged: true, skipped: true}
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "commit", "-m", message); err == nil {
			return commitResult{repo: r, staged: true, out: out, committed: true}
		} else {
			return commitResult{repo: r, staged: true, out: out, err: err}
		}
	})
	for _, result := range results {
		switch {
		case result.committed:
			a.ok(result.repo.display, "Commit created")
			committed++
		case result.skipped:
			a.skip(result.repo.display, "No changes to commit. Skipping...")
			skipped++
		case !result.staged:
			a.error(result.repo.display, "Could not stage changes")
			status = 1
			failed++
		default:
			a.error(result.repo.display, "Could not commit changes:")
			fmt.Fprintf(a.stderr, "%s\n\n", result.out)
			status = 1
			failed++
		}
	}
	fmt.Fprintf(a.stdout, "Summary: %d committed, %d skipped, %d failed\n", committed, skipped, failed)
	return status
}
