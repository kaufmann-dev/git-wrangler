package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func runCommit(a *app, cmd *cobra.Command, args []string) int {
	message, _ := cmd.Flags().GetString("message")
	if message == "" {
		fmt.Fprintf(a.stderr, "%sError: A commit message is required. Use --message <commit_message>.%s\n", a.ui.Red, a.ui.Reset)
		return 1
	}
	if !requireGit(a, "commit") {
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
		if _, err := runCapture(r.dir, nil, "git", "add", "-A"); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not stage changes for %s%s\n", a.ui.Red, r.display, a.ui.Reset)
			continue
		}
		if _, err := runCapture(r.dir, nil, "git", "diff", "--cached", "--quiet"); err == nil {
			fmt.Fprintf(a.stdout, "%sNo changes to commit for %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		if out, err := runCapture(r.dir, nil, "git", "commit", "-m", message); err == nil {
			fmt.Fprintf(a.stdout, "%sCommit created for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not commit changes for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
		}
	}
	return 0
}
