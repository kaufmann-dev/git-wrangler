package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func runFixGitignore(a *app, cmd *cobra.Command, args []string) int {
	yes := yesFlag(cmd)
	if !requireGit(a, "fix-gitignore") {
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
	candidates := []string{"bin/", "obj/", ".idea/", "vendor/", "node_modules/", "dist/", "build/", "wp-includes/", ".DS_Store", "Thumbs.db", "*.log"}
	type gitignoreScan struct {
		repo       repo
		added      []string
		covered    []string
		notPresent []string
	}
	scans := parallelReposProgress(repos, newProgress(a, "Scanning .gitignore candidates", len(repos)), func(r repo) gitignoreScan {
		scan := gitignoreScan{repo: r}
		for _, entry := range candidates {
			match := findExistingMatch(r.dir, entry)
			if match == "" {
				scan.notPresent = append(scan.notPresent, entry)
				continue
			}
			if _, err := a.git.Capture(a.ctx, r.dir, nil, "check-ignore", "-q", match); err == nil {
				scan.covered = append(scan.covered, entry)
				continue
			}
			if fileContainsLine(filepath.Join(r.dir, ".gitignore"), entry) {
				scan.covered = append(scan.covered, entry)
			} else {
				scan.added = append(scan.added, entry)
			}
		}
		return scan
	})
	status := 0
	applies := []gitignoreScan{}
	unchanged := 0
	for _, scan := range scans {
		r := scan.repo
		added := scan.added
		if len(added) > 0 {
			renderRepoHeader(a, r.display)
			fmt.Fprintf(a.stdout, "  %sWill add:%s %s\n", a.ui.Yellow, a.ui.Reset, strings.Join(added, ", "))
			fmt.Fprintln(a.stdout)
			applies = append(applies, scan)
		} else {
			unchanged++
		}
	}
	renderSummary(a,
		summaryCount{label: "with changes", value: len(applies), color: a.ui.Yellow},
		summaryCount{label: "unchanged", value: unchanged, color: a.ui.Green},
		summaryCount{label: "failed", value: 0, color: a.ui.Red},
	)
	if len(applies) == 0 {
		return status
	}
	renderWarning(a, fmt.Sprintf("This operation will modify .gitignore and create commits in %d repositories.", len(applies)))
	if !confirmOrSkip(a, yes, fmt.Sprintf("Apply and commit .gitignore updates for %d repositories?", len(applies))) {
		renderSummary(a,
			summaryCount{label: "updated", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: len(applies), color: a.ui.Yellow},
			summaryCount{label: "failed", value: 0, color: a.ui.Red},
		)
		return status
	}
	updated := 0
	failed := 0
	progress := newProgress(a, "Applying .gitignore updates", len(applies))
	type applyError struct {
		subject string
		output  string
	}
	applyErrors := []applyError{}
	for _, apply := range applies {
		r := apply.repo
		progress.start(r.display)
		if err := appendGitignoreEntries(filepath.Join(r.dir, ".gitignore"), apply.added); err != nil {
			progress.advance(r.display)
			applyErrors = append(applyErrors, applyError{subject: r.display + ": could not update .gitignore", output: err.Error()})
			status = 1
			failed++
			continue
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "add", ".gitignore"); err != nil {
			progress.advance(r.display)
			applyErrors = append(applyErrors, applyError{subject: r.display + ": could not stage .gitignore", output: out})
			status = 1
			failed++
			continue
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "commit", "-m", "Update .gitignore with missing entries"); err == nil {
			updated++
		} else {
			applyErrors = append(applyErrors, applyError{subject: r.display + ": could not commit .gitignore", output: out})
			status = 1
			failed++
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
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	return status
}

func findExistingMatch(root, entry string) string {
	var result string
	wantDir := strings.HasSuffix(entry, "/")
	pattern := strings.TrimSuffix(entry, "/")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || result != "" {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if wantDir {
			if d.IsDir() && d.Name() == pattern {
				result = "./" + filepath.ToSlash(rel)
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			if ok, _ := filepath.Match(pattern, d.Name()); ok {
				result = "./" + filepath.ToSlash(rel)
			}
		}
		return nil
	})
	return result
}

func fileContainsLine(path, line string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if scanner.Text() == line {
			return true
		}
	}
	return false
}

func appendGitignoreEntries(path string, entries []string) error {
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 && data[len(data)-1] != '\n' {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if _, err := f.WriteString("\n"); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, entry := range entries {
		if _, err := fmt.Fprintln(f, entry); err != nil {
			return err
		}
	}
	return nil
}
