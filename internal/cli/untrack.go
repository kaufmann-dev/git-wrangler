package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func runUntrack(a *app, cmd *cobra.Command, args []string) int {
	confirmed, _ := cmd.Flags().GetBool("confirm")
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
	status := 0
	for _, r := range repos {
		if !fileExists(filepath.Join(r.dir, ".gitignore")) {
			fmt.Fprintf(a.stdout, "%sNo .gitignore file found for %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		out, err := runStdout(r.dir, nil, "git", "ls-files", "--ignored", "--cached", "--exclude-standard")
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not list ignored tracked files for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			status = 1
			continue
		}
		if strings.TrimSpace(out) == "" {
			fmt.Fprintf(a.stdout, "%sNo currently tracked files match .gitignore in %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stdout, "%s%s:%s %d tracked ignored file(s) will be removed from the index.\n", a.ui.RepoColor, r.display, a.ui.Reset, lineCount(out))
		fmt.Fprintf(a.stderr, "%sWARNING: This operation will stop tracking ignored files and create a commit in %s.%s\n", a.ui.Red, r.display, a.ui.Reset)
		if !confirmed && !confirm(a, "Stop tracking ignored files and commit for "+r.display+"?") {
			fmt.Fprintf(a.stdout, "%sSkipping %s.%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		zout, err := runStdout(r.dir, nil, "git", "ls-files", "--ignored", "--cached", "--exclude-standard", "-z")
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not list ignored tracked files for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			status = 1
			continue
		}
		files := strings.Split(strings.TrimRight(zout, "\x00"), "\x00")
		failed := false
		for _, chunk := range chunkStrings(files, 100) {
			rmArgs := append([]string{"rm", "--cached", "-q", "--"}, chunk...)
			if out, err := runCapture(r.dir, nil, "git", rmArgs...); err != nil {
				fmt.Fprintf(a.stderr, "%sError: Could not untrack files for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
				status = 1
				failed = true
				break
			}
		}
		if failed {
			continue
		}
		if out, err := runCapture(r.dir, nil, "git", "commit", "-q", "-m", "Stop tracking files defined in .gitignore"); err == nil {
			fmt.Fprintf(a.stdout, "%sStopped tracking and committed ignored files for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not commit changes for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			status = 1
		}
	}
	return status
}

func chunkStrings(items []string, size int) [][]string {
	var chunks [][]string
	if size <= 0 {
		size = len(items)
	}
	for start := 0; start < len(items); start += size {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		chunk := items[start:end]
		if len(chunk) == 1 && chunk[0] == "" {
			continue
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}
