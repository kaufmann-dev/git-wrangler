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
	status := 0
	committed := 0
	skipped := 0
	failed := 0
	for _, r := range repos {
		if _, err := a.git.Capture(a.ctx, r.dir, nil, "add", "-A"); err != nil {
			a.error(r.display, "Could not stage changes")
			status = 1
			failed++
			continue
		}
		if _, err := a.git.Capture(a.ctx, r.dir, nil, "diff", "--cached", "--quiet"); err == nil {
			a.skip(r.display, "No changes to commit. Skipping...")
			skipped++
			continue
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "commit", "-m", message); err == nil {
			a.ok(r.display, "Commit created")
			committed++
		} else {
			a.error(r.display, "Could not commit changes:")
			fmt.Fprintf(a.stderr, "%s\n\n", out)
			status = 1
			failed++
		}
	}
	fmt.Fprintf(a.stdout, "Summary: %d committed, %d skipped, %d failed\n", committed, skipped, failed)
	return status
}
