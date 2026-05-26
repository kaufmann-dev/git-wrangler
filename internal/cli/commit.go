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
	for _, r := range repos {
		if _, err := a.git.Capture(a.ctx, r.dir, nil, "add", "-A"); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not stage changes for %s%s\n", a.ui.Red, r.display, a.ui.Reset)
			status = 1
			continue
		}
		if _, err := a.git.Capture(a.ctx, r.dir, nil, "diff", "--cached", "--quiet"); err == nil {
			fmt.Fprintf(a.stdout, "%sNo changes to commit for %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "commit", "-m", message); err == nil {
			fmt.Fprintf(a.stdout, "%sCommit created for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not commit changes for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			status = 1
		}
	}
	return status
}
