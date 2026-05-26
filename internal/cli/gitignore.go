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
	repos, err := resolveRepositoryTargets("")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	candidates := []string{"bin/", "obj/", ".idea/", "vendor/", "node_modules/", "dist/", "build/", "wp-includes/", ".DS_Store", "Thumbs.db", "*.log"}
	status := 0
	for _, r := range repos {
		added := []string{}
		covered := []string{}
		notPresent := []string{}
		for _, entry := range candidates {
			match := findExistingMatch(r.dir, entry)
			if match == "" {
				notPresent = append(notPresent, entry)
				continue
			}
			if _, err := a.git.Capture(a.ctx, r.dir, nil, "check-ignore", "-q", match); err == nil {
				covered = append(covered, entry)
				continue
			}
			if fileContainsLine(filepath.Join(r.dir, ".gitignore"), entry) {
				covered = append(covered, entry)
			} else {
				added = append(added, entry)
			}
		}
		printedRepo := false
		if len(added) > 0 {
			fmt.Fprintf(a.stdout, "%s%s:%s\n", a.ui.RepoColor, r.display, a.ui.Reset)
			fmt.Fprintf(a.stdout, "  %sWill add:%s %s\n", a.ui.Yellow, a.ui.Reset, strings.Join(added, ", "))
			printedRepo = true
			fmt.Fprintf(a.stderr, "%sWARNING: This operation will modify .gitignore and create a commit in %s.%s\n", a.ui.Red, r.display, a.ui.Reset)
			if !yes && !confirm(a, "Apply and commit .gitignore updates for "+r.display+"?") {
				fmt.Fprintf(a.stdout, "%sSkipping %s.%s\n", a.ui.Yellow, r.display, a.ui.Reset)
				continue
			}
			if err := appendGitignoreEntries(filepath.Join(r.dir, ".gitignore"), added); err != nil {
				fmt.Fprintf(a.stderr, "  %sError: Could not update .gitignore:\n%s%s\n", a.ui.Red, err.Error(), a.ui.Reset)
				status = 1
				continue
			}
		}
		if !printedRepo {
			fmt.Fprintf(a.stdout, "%s%s:%s\n", a.ui.RepoColor, r.display, a.ui.Reset)
		}
		if len(added) > 0 {
			fmt.Fprintf(a.stdout, "  %sAdded:%s %s\n", a.ui.Green, a.ui.Reset, strings.Join(added, ", "))
			if out, err := a.git.Capture(a.ctx, r.dir, nil, "add", ".gitignore"); err != nil {
				fmt.Fprintf(a.stderr, "  %sError: Could not stage .gitignore:\n%s%s\n", a.ui.Red, out, a.ui.Reset)
				status = 1
				continue
			}
			if out, err := a.git.Capture(a.ctx, r.dir, nil, "commit", "-m", "Update .gitignore with missing entries"); err == nil {
				fmt.Fprintf(a.stdout, "  %sCommitted .gitignore updates%s\n", a.ui.Green, a.ui.Reset)
			} else {
				fmt.Fprintf(a.stderr, "  %sError: Could not commit .gitignore:\n%s%s\n", a.ui.Red, out, a.ui.Reset)
				status = 1
			}
		}
		if len(covered) > 0 {
			fmt.Fprintf(a.stdout, "  %sSkipped (already covered):%s %s\n", a.ui.Yellow, a.ui.Reset, strings.Join(covered, ", "))
		}
		if len(notPresent) > 0 {
			fmt.Fprintf(a.stdout, "  %sSkipped (not present on disk):%s %s\n", a.ui.Yellow, a.ui.Reset, strings.Join(notPresent, ", "))
		}
		if len(added) == 0 && len(covered) == 0 && len(notPresent) == 0 {
			fmt.Fprintf(a.stdout, "  %sNo changes needed.%s\n", a.ui.Yellow, a.ui.Reset)
		}
	}
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
