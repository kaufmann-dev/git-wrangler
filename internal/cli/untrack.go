package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func runUntrack(a *app, cmd *cobra.Command, args []string) int {
	if len(args) > 0 {
		a.errorf("Unknown option: %s", args[0])
		return 1
	}
	if !requireGit(a, "untrack") {
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
		if !fileExists(filepath.Join(r.dir, ".gitignore")) {
			fmt.Fprintf(a.stdout, "%sNo .gitignore file found for %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		out, _ := runStdout(r.dir, nil, "git", "ls-files", "--ignored", "--cached", "--exclude-standard")
		if strings.TrimSpace(out) == "" {
			fmt.Fprintf(a.stdout, "%sNo currently tracked files match .gitignore in %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		zout, _ := runStdout(r.dir, nil, "git", "ls-files", "--ignored", "--cached", "--exclude-standard", "-z")
		files := strings.Split(strings.TrimRight(zout, "\x00"), "\x00")
		rmArgs := append([]string{"rm", "--cached", "-q", "--"}, files...)
		if out, err := runCapture(r.dir, nil, "git", rmArgs...); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not untrack files for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			continue
		}
		if out, err := runCapture(r.dir, nil, "git", "commit", "-q", "-m", "Stop tracking files defined in .gitignore"); err == nil {
			fmt.Fprintf(a.stdout, "%sStopped tracking and committed ignored files for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not commit changes for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
		}
	}
	return 0
}
