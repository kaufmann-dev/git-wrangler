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
	scans := parallelReposProgress(a.ctx, repos, newProgress(a, "Scanning ignored tracked files", len(repos)), func(r repo) untrackScan {
		if !fileExists(filepath.Join(r.dir, ".gitignore")) {
			return untrackScan{repo: r}
		}
		out, err := a.git.Stdout(a.ctx, r.dir, nil, "ls-files", "--ignored", "--cached", "--exclude-standard")
		return untrackScan{repo: r, out: out, err: err, hasGitignore: true}
	})
	if interrupted(a) {
		return 1
	}
	status := 0
	applies := []untrackScan{}
	unchanged := 0
	failed := 0
	for _, scan := range scans {
		r := scan.repo
		if !scan.hasGitignore {
			unchanged++
			continue
		}
		if scan.err != nil {
			renderErrorBlock(a, r.display+": could not list ignored tracked files", scan.err.Error())
			status = 1
			failed++
			continue
		}
		if strings.TrimSpace(scan.out) == "" {
			unchanged++
			continue
		}
		renderRepoHeader(a, r.display)
		fmt.Fprintf(a.stdout, "  %sTracked ignored files:%s %d\n", a.ui.Yellow, a.ui.Reset, lineCount(scan.out))
		fmt.Fprintln(a.stdout)
		applies = append(applies, scan)
	}
	renderSummary(a,
		summaryCount{label: "with tracked ignored files", value: len(applies), color: a.ui.Yellow},
		summaryCount{label: "unchanged", value: unchanged, color: a.ui.Green},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	if len(applies) == 0 {
		return status
	}
	renderWarning(a, fmt.Sprintf("This operation will stop tracking ignored files and create commits in %d repositories.", len(applies)))
	confirmation := confirmOrSkip(a, yes, fmt.Sprintf("Stop tracking ignored files and commit for %d repositories?", len(applies)))
	if confirmation == confirmationUnavailable || confirmation == confirmationCancelled {
		return 1
	}
	if confirmation == confirmationDeclined {
		renderSummary(a,
			summaryCount{label: "updated", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: len(applies), color: a.ui.Yellow},
			summaryCount{label: "failed", value: 0, color: a.ui.Red},
		)
		return status
	}
	updated := 0
	applyFailed := 0
	type applyError struct {
		subject string
		output  string
	}
	applyErrors := []applyError{}
	progress := newProgress(a, "Untracking ignored files", len(applies))
	for _, apply := range applies {
		r := apply.repo
		progress.start(r.display)
		zout, err := a.git.Stdout(a.ctx, r.dir, nil, "ls-files", "--ignored", "--cached", "--exclude-standard", "-z")
		if err != nil {
			applyErrors = append(applyErrors, applyError{subject: r.display + ": could not list ignored tracked files", output: err.Error()})
			status = 1
			applyFailed++
			progress.advance(r.display)
			continue
		}
		files := strings.Split(strings.TrimRight(zout, "\x00"), "\x00")
		failed := false
		for _, chunk := range chunkStrings(files, 100) {
			rmArgs := append([]string{"rm", "--cached", "-q", "--"}, chunk...)
			if out, err := a.git.Capture(a.ctx, r.dir, nil, rmArgs...); err != nil {
				applyErrors = append(applyErrors, applyError{subject: r.display + ": could not untrack files", output: out})
				status = 1
				applyFailed++
				failed = true
				break
			}
		}
		if failed {
			progress.advance(r.display)
			continue
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "commit", "-q", "-m", "Stop tracking files defined in .gitignore"); err == nil {
			updated++
		} else {
			applyErrors = append(applyErrors, applyError{subject: r.display + ": could not commit changes", output: out})
			status = 1
			applyFailed++
		}
		progress.advance(r.display)
	}
	finishProgressBeforeOutput(progress)
	for _, err := range applyErrors {
		renderErrorBlock(a, err.subject, err.output)
	}
	renderSummary(a,
		summaryCount{label: "updated", value: updated, color: a.ui.Green},
		summaryCount{label: "skipped", value: 0, color: a.ui.Yellow},
		summaryCount{label: "failed", value: applyFailed, color: a.ui.Red},
	)
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
