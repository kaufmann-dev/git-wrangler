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
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		pullArgs := []string{"pull"}
		if rebase {
			pullArgs = append(pullArgs, "--rebase")
		}
		if force {
			pullArgs = append(pullArgs, "--force")
		}
		out, err := runCapture(r.dir, nil, "git", pullArgs...)
		if err == nil {
			if strings.Contains(out, "Already up to date") {
				a.skip(r.display, "Already up to date. Skipping...")
			} else {
				a.ok(r.display, "Git pull completed")
			}
		} else {
			a.error(r.display, "Git pull failed:")
			fmt.Fprintf(a.stderr, "%s\n\n", out)
		}
	}
	return 0
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
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		pushArgs := []string{"push", "origin", "HEAD"}
		if force {
			pushArgs = []string{"push", "--force-with-lease", "origin", "HEAD"}
		} else if forceUnsafe {
			if !confirm(a, "Raw force push "+r.display+" with --force?") {
				a.skip(r.display, "Skipping unsafe force push.")
				continue
			}
			pushArgs = []string{"push", "--force", "origin", "HEAD"}
		}
		out, err := runCapture(r.dir, nil, "git", pushArgs...)
		if err == nil {
			if strings.Contains(out, "Everything up-to-date") {
				a.skip(r.display, "No changes to push. Skipping...")
			} else {
				a.ok(r.display, "Git push completed")
			}
		} else {
			a.error(r.display, "Git push failed:")
			fmt.Fprintf(a.stderr, "%s\n\n", out)
		}
	}
	return 0
}
