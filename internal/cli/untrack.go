package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func runUntrack(a *app, cmd *cobra.Command, args []string) int {
	yes := yesFlag(cmd)
	if !requireGit(a, "untrack") {
		return 1
	}
	repos, err := commandRepositoryTargets(cmd)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	type untrackScan struct {
		repo         repo
		out          string
		err          error
		hasGitignore bool
	}
	scans := parallelReposProgress(repos, newProgress(a, "Scanning ignored tracked files", len(repos)), func(r repo) untrackScan {
		if !fileExists(filepath.Join(r.dir, ".gitignore")) {
			return untrackScan{repo: r}
		}
		out, err := a.git.Stdout(a.ctx, r.dir, nil, "ls-files", "--ignored", "--cached", "--exclude-standard")
		return untrackScan{repo: r, out: out, err: err, hasGitignore: true}
	})
	status := 0
	applies := []untrackScan{}
	for _, scan := range scans {
		r := scan.repo
		if !scan.hasGitignore {
			fmt.Fprintf(a.stdout, "%sNo .gitignore file found for %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		if scan.err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not list ignored tracked files for %s:\n%s%s\n\n", a.ui.Red, r.display, scan.err.Error(), a.ui.Reset)
			status = 1
			continue
		}
		if strings.TrimSpace(scan.out) == "" {
			fmt.Fprintf(a.stdout, "%sNo currently tracked files match .gitignore in %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stdout, "%s%s:%s %d tracked ignored file(s) will be removed from the index.\n", a.ui.RepoColor, r.display, a.ui.Reset, lineCount(scan.out))
		applies = append(applies, scan)
	}
	if len(applies) == 0 {
		return status
	}
	fmt.Fprintf(a.stderr, "%sWARNING: This operation will stop tracking ignored files and create commits in %d repositories.%s\n", a.ui.Red, len(applies), a.ui.Reset)
	if !confirmOrSkip(a, yes, fmt.Sprintf("Stop tracking ignored files and commit for %d repositories?", len(applies))) {
		for _, apply := range applies {
			fmt.Fprintf(a.stdout, "%sSkipping %s.%s\n", a.ui.Yellow, apply.repo.display, a.ui.Reset)
		}
		return status
	}
	for _, apply := range applies {
		r := apply.repo
		zout, err := a.git.Stdout(a.ctx, r.dir, nil, "ls-files", "--ignored", "--cached", "--exclude-standard", "-z")
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not list ignored tracked files for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			status = 1
			continue
		}
		files := strings.Split(strings.TrimRight(zout, "\x00"), "\x00")
		failed := false
		for _, chunk := range chunkStrings(files, 100) {
			rmArgs := append([]string{"rm", "--cached", "-q", "--"}, chunk...)
			if out, err := a.git.Capture(a.ctx, r.dir, nil, rmArgs...); err != nil {
				fmt.Fprintf(a.stderr, "%sError: Could not untrack files for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
				status = 1
				failed = true
				break
			}
		}
		if failed {
			continue
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "commit", "-q", "-m", "Stop tracking files defined in .gitignore"); err == nil {
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
