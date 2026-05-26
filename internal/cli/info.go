package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
	"github.com/spf13/cobra"
)

func runInfo(a *app, cmd *cobra.Command, args []string) int {
	repoName, _ := cmd.Flags().GetString("repo")
	if !requireGit(a, "info") {
		return 1
	}
	root := "."
	if repoName != "" {
		root = repoName
	}
	repos, err := findGitRepositories(root)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	statusCode := 0
	for _, r := range repos {
		fmt.Fprintf(a.stdout, "Repository:         %s%s%s\n", a.ui.RepoColor, r.display, a.ui.Reset)
		status, err := runStdout(r.dir, nil, "git", "status", "--porcelain")
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect status for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			statusCode = 1
			continue
		}
		if strings.TrimSpace(status) == "" {
			fmt.Fprintf(a.stdout, "Status:             %sClean%s\n", a.ui.Green, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stdout, "Status:             %sDirty (uncommitted changes or untracked files)%s\n", a.ui.Yellow, a.ui.Reset)
		}
		printLicenseInfo(a, r.dir)
		branch, err := runStdout(r.dir, nil, "git", "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect branch for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			statusCode = 1
			continue
		}
		fmt.Fprintf(a.stdout, "Current branch:     %s\n", strings.TrimSpace(branch))
		hasCommits := false
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "HEAD"); err == nil {
			hasCommits = true
		}
		ab, _ := runStdout(r.dir, nil, "git", "rev-list", "--left-right", "--count", "HEAD...@{u}")
		if fields := strings.Fields(ab); len(fields) >= 2 {
			fmt.Fprintf(a.stdout, "Ahead/behind:       %s ahead, %s behind remote\n", fields[0], fields[1])
		} else {
			fmt.Fprintln(a.stdout, "Ahead/behind:       No upstream set")
		}
		if err := printBranches(a, r.dir); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect branches for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			statusCode = 1
			continue
		}
		if err := printRemotes(a, r.dir); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect remotes for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			statusCode = 1
			continue
		}
		initial, err := runStdout(r.dir, nil, "git", "log", "--reverse", "--format=%ci - %s")
		if err != nil && hasCommits {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect initial commit for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			statusCode = 1
			continue
		}
		if hasCommits && strings.TrimSpace(initial) != "" {
			fmt.Fprintf(a.stdout, "Initial commit:     %s\n", firstLine(initial))
		} else {
			fmt.Fprintln(a.stdout, "Initial commit:     None (repository is empty)")
		}
		commitCount, err := runStdout(r.dir, nil, "git", "rev-list", "--all", "--count")
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not count commits for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			statusCode = 1
			continue
		}
		fmt.Fprintf(a.stdout, "Total commits:      %s\n", strings.TrimSpace(commitCount))
		lastMonth, err := runStdout(r.dir, nil, "git", "log", "--since=1 month ago", "--format=%ci")
		if err != nil && hasCommits {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect recent commits for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			statusCode = 1
			continue
		}
		fmt.Fprintf(a.stdout, "Commits last month: %d\n", len(splitLines(lastMonth)))
		last, err := runStdout(r.dir, nil, "git", "log", "-1", "--format=%ci - %s")
		if err != nil && hasCommits {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect last commit for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			statusCode = 1
			continue
		}
		fmt.Fprintf(a.stdout, "Last commit:        %s\n", strings.TrimSpace(last))
		if hasCommits {
			if err := printTopAuthors(a, r.dir); err != nil {
				fmt.Fprintf(a.stderr, "%sError: Could not inspect authors for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
				statusCode = 1
				continue
			}
		} else {
			fmt.Fprintln(a.stdout, "Top authors:        None (repository is empty)")
		}
		if err := printLargestFiles(a, r.dir); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect largest files for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			statusCode = 1
			continue
		}
		fmt.Fprintln(a.stdout)
	}
	return statusCode
}

func printLicenseInfo(a *app, dir string) {
	fmt.Fprint(a.stdout, "License:            ")
	data, err := os.ReadFile(filepath.Join(dir, "LICENSE"))
	if err != nil {
		fmt.Fprintf(a.stdout, "%sNone%s\n", a.ui.Yellow, a.ui.Reset)
		return
	}
	line := firstLine(string(data))
	if line == "" {
		fmt.Fprintf(a.stdout, "%sYes%s\n", a.ui.Green, a.ui.Reset)
	} else {
		fmt.Fprintf(a.stdout, "%s'%s'%s\n", a.ui.Green, truncate(line, 70), a.ui.Reset)
	}
}

func printBranches(a *app, dir string) error {
	out, err := runStdout(dir, nil, "git", "branch", "-a", "--no-color")
	if err != nil {
		return err
	}
	branches := []string{}
	for _, line := range splitLines(out) {
		if strings.Contains(line, "remotes") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "* "))
		if line != "" {
			branches = append(branches, line)
		}
	}
	fmt.Fprintf(a.stdout, "Branches (%d):       ", len(branches))
	for i, branch := range branches {
		if i == 0 {
			fmt.Fprintln(a.stdout, branch)
		} else {
			fmt.Fprintf(a.stdout, "%-20s%s\n", "", branch)
		}
	}
	if len(branches) == 0 {
		fmt.Fprintln(a.stdout)
	}
	return nil
}

func printRemotes(a *app, dir string) error {
	out, err := runStdout(dir, nil, "git", "remote", "-v")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	remotes := []string{}
	for _, line := range splitLines(out) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && !seen[fields[1]] {
			seen[fields[1]] = true
			remotes = append(remotes, fields[1])
		}
	}
	sort.Strings(remotes)
	if len(remotes) == 0 {
		fmt.Fprintln(a.stdout, "Remotes:            None")
		return nil
	}
	for i, remote := range remotes {
		if i == 0 {
			fmt.Fprintf(a.stdout, "Remotes:            %s\n", remote)
		} else {
			fmt.Fprintf(a.stdout, "%-20s%s\n", "", remote)
		}
	}
	return nil
}

func printTopAuthors(a *app, dir string) error {
	out, err := runStdout(dir, nil, "git", "log", "--format=%an <%ae>")
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, line := range splitLines(out) {
		counts[line]++
	}
	type row struct {
		name  string
		count int
	}
	rows := []row{}
	for name, count := range counts {
		rows = append(rows, row{name: name, count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count == rows[j].count {
			return rows[i].name < rows[j].name
		}
		return rows[i].count > rows[j].count
	})
	for i, row := range rows {
		if i >= 3 {
			break
		}
		if i == 0 {
			fmt.Fprintf(a.stdout, "Top authors:        %d - %s\n", row.count, row.name)
		} else {
			fmt.Fprintf(a.stdout, "%-20s%d - %s\n", "", row.count, row.name)
		}
	}
	return nil
}

func printLargestFiles(a *app, dir string) error {
	objects, err := runStdout(dir, nil, "git", "rev-list", "--objects", "--all")
	if err != nil {
		return err
	}
	if strings.TrimSpace(objects) == "" {
		return nil
	}
	type row struct {
		size int64
		path string
	}
	rows := []row{}
	seen := map[string]bool{}
	batch, err := git.CatFileBatchCheck(context.Background(), dir, objects)
	if err != nil {
		return err
	}
	for _, line := range splitLines(batch) {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		size, _ := strconv.ParseInt(fields[0], 10, 64)
		path := strings.Join(fields[2:], " ")
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		rows = append(rows, row{size: size, path: path})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].size > rows[j].size })
	for i, row := range rows {
		if i >= 3 {
			break
		}
		if i == 0 {
			fmt.Fprintf(a.stdout, "Largest files:      %s - %s\n", humanSize(row.size), row.path)
		} else {
			fmt.Fprintf(a.stdout, "%-20s%s - %s\n", "", humanSize(row.size), row.path)
		}
	}
	return nil
}

func humanSize(size int64) string {
	switch {
	case size >= 1073741824:
		return fmt.Sprintf("%.2f GB", float64(size)/1073741824)
	case size >= 1048576:
		return fmt.Sprintf("%.2f MB", float64(size)/1048576)
	case size >= 1024:
		return fmt.Sprintf("%.2f KB", float64(size)/1024)
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}
