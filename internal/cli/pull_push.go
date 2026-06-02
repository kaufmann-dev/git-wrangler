package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func runPull(a *app, cmd *cobra.Command, args []string) int {
	rebase, _ := cmd.Flags().GetBool("rebase")
	force, _ := cmd.Flags().GetBool("force")
	if !requireGit(a, "pull") {
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
	updated := 0
	skipped := 0
	failed := 0
	for _, r := range repos {
		pullArgs := []string{"pull"}
		if rebase {
			pullArgs = append(pullArgs, "--rebase")
		}
		if force {
			pullArgs = append(pullArgs, "--force")
		}
		out, err := a.git.Capture(a.ctx, r.dir, nil, pullArgs...)
		if err == nil {
			if strings.Contains(out, "Already up to date") {
				a.skip(r.display, "Already up to date. Skipping...")
				skipped++
			} else {
				a.ok(r.display, "Git pull completed")
				updated++
			}
		} else {
			a.error(r.display, "Git pull failed:")
			fmt.Fprintf(a.stderr, "%s\n\n", out)
			status = 1
			failed++
		}
	}
	fmt.Fprintf(a.stdout, "Summary: %d updated, %d skipped, %d failed\n", updated, skipped, failed)
	return status
}

func runPush(a *app, cmd *cobra.Command, args []string) int {
	force, _ := cmd.Flags().GetBool("force")
	forceUnsafe, _ := cmd.Flags().GetBool("force-unsafe")
	if force && forceUnsafe {
		a.error("Use either --force or --force-unsafe, not both.")
		return 1
	}
	if !requireGit(a, "push") {
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
	pushed := 0
	skipped := 0
	failed := 0
	for _, r := range repos {
		pushArgs := []string{"push", "origin", "HEAD"}
		if force {
			pushArgs = []string{"push", "--force-with-lease", "origin", "HEAD"}
		} else if forceUnsafe {
			if !yesFlag(cmd) && !confirm(a, "Raw force push "+r.display+" with --force?") {
				a.skip(r.display, "Skipping unsafe force push.")
				skipped++
				continue
			}
			pushArgs = []string{"push", "--force", "origin", "HEAD"}
		}
		out, err := a.git.Capture(a.ctx, r.dir, nil, pushArgs...)
		if err == nil {
			if strings.Contains(out, "Everything up-to-date") {
				a.skip(r.display, "No changes to push. Skipping...")
				skipped++
			} else {
				a.ok(r.display, "Git push completed")
				pushed++
			}
		} else {
			a.error(r.display, "Git push failed:")
			fmt.Fprintf(a.stderr, "%s\n\n", out)
			status = 1
			failed++
		}
	}
	fmt.Fprintf(a.stdout, "Summary: %d pushed, %d skipped, %d failed\n", pushed, skipped, failed)
	return status
}
