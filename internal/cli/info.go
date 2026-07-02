package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type infoOptions struct {
	target targetOptions
	json   jsonOptions
	fetch  fetchOptions
}

func infoOptionsFromCommand(cmd *cobra.Command) infoOptions {
	return infoOptions{
		target: targetOptionsFromCommand(cmd),
		json:   jsonOptionsFromCommand(cmd),
		fetch:  fetchOptionsFromCommand(cmd),
	}
}

func runInfo(a *app, cmd *cobra.Command, args []string) int {
	opts := infoOptionsFromCommand(cmd)
	if opts.json.enabled {
		return runInfoJSON(a, opts)
	}
	if !requireGit(a, "info") {
		return 1
	}
	repos, err := opts.target.repositories()
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	fetchFailures := map[string]originRefreshResult{}
	if !opts.fetch.noFetch {
		fetchFailures = refreshFailuresByDir(refreshOrigin(a, repos))
		if interrupted(a) {
			return 1
		}
	}
	statusCode := 0
	failed := 0
	results := parallelReposProgress(a.ctx, repos, newProgress(a, "Inspecting repositories", len(repos)), func(r repo) infoResult {
		if failure, ok := fetchFailures[r.dir]; ok {
			return infoErrorResult(a, r, "Could not fetch origin", fmt.Errorf("%s", fetchFailureMessage(failure)))
		}
		return collectInfo(a, r)
	})
	if interrupted(a) {
		return 1
	}
	for _, result := range results {
		fmt.Fprint(a.stdout, result.stdout)
		if result.stderr != "" {
			fmt.Fprint(a.stderr, result.stderr)
		}
		if result.failed {
			statusCode = 1
			failed++
		}
	}
	if len(repos) > 1 && failed > 0 {
		renderSummary(a,
			summaryCount{label: "inspected", value: len(repos) - failed, color: a.ui.Green},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
	}
	return statusCode
}

func runInfoJSON(a *app, opts infoOptions) int {
	if _, err := a.runner.LookPath("git"); err != nil {
		return writeJSONStatus(a, map[string]any{
			"ok":      false,
			"summary": map[string]any{"repositories": 0, "failed": 1},
			"error":   jsonError{Message: "'git' is required for info. Install it and make sure it is on PATH."},
		}, 1)
	}
	repos, err := opts.target.repositories()
	if err != nil {
		return writeJSONStatus(a, map[string]any{
			"ok":      false,
			"summary": map[string]any{"repositories": 0, "failed": 1},
			"error":   jsonError{Message: err.Error()},
		}, 1)
	}
	fetchFailures := map[string]originRefreshResult{}
	if !opts.fetch.noFetch {
		fetchFailures = refreshFailuresByDir(refreshOrigin(a, repos))
		if a.ctx.Err() != nil {
			return writeJSONStatus(a, map[string]any{
				"ok":      false,
				"summary": map[string]any{"repositories": len(repos), "failed": len(repos)},
				"error":   jsonError{Message: "operation cancelled"},
			}, 1)
		}
	}
	type infoJSONRepo struct {
		Name          string     `json:"name"`
		Path          string     `json:"path"`
		Status        string     `json:"status,omitempty"`
		Branch        string     `json:"branch,omitempty"`
		Ahead         string     `json:"ahead,omitempty"`
		Behind        string     `json:"behind,omitempty"`
		HasLicense    bool       `json:"hasLicense"`
		CommitCount   string     `json:"commitCount,omitempty"`
		LastCommit    string     `json:"lastCommit,omitempty"`
		InitialCommit string     `json:"initialCommit,omitempty"`
		Remotes       []string   `json:"remotes,omitempty"`
		Error         *jsonError `json:"error,omitempty"`
	}
	results := parallelRepos(a.ctx, repos, func(r repo) infoJSONRepo {
		row := infoJSONRepo{Name: r.display, Path: r.dir, HasLicense: fileExists(filepath.Join(r.dir, "LICENSE"))}
		if failure, ok := fetchFailures[r.dir]; ok {
			row.Error = &jsonError{Message: fetchFailureMessage(failure)}
			return row
		}
		status, err := a.git.StatusPorcelain(a.ctx, r.dir)
		if err != nil {
			row.Error = &jsonError{Message: "could not inspect status: " + err.Error()}
			return row
		}
		if strings.TrimSpace(status) == "" {
			row.Status = "clean"
		} else {
			row.Status = "dirty"
		}
		branch, err := a.git.CurrentBranch(a.ctx, r.dir)
		if err != nil {
			row.Error = &jsonError{Message: "could not inspect branch: " + err.Error()}
			return row
		}
		row.Branch = strings.TrimSpace(branch)
		if ab, _ := a.git.Stdout(a.ctx, r.dir, nil, "rev-list", "--left-right", "--count", "HEAD...@{u}"); len(strings.Fields(ab)) >= 2 {
			fields := strings.Fields(ab)
			row.Ahead = fields[0]
			row.Behind = fields[1]
		}
		hasCommits := a.git.HasHead(a.ctx, r.dir)
		if count, err := a.git.Stdout(a.ctx, r.dir, nil, "rev-list", "--all", "--count"); err == nil {
			row.CommitCount = strings.TrimSpace(count)
		} else {
			row.Error = &jsonError{Message: "could not count commits: " + err.Error()}
			return row
		}
		if hasCommits {
			if initial, err := a.git.Stdout(a.ctx, r.dir, nil, "log", "--reverse", "--format=%ci - %s"); err == nil {
				row.InitialCommit = firstLine(strings.TrimSpace(initial))
			}
			if last, err := a.git.Stdout(a.ctx, r.dir, nil, "log", "-1", "--format=%ci - %s"); err == nil {
				row.LastCommit = strings.TrimSpace(last)
			}
		}
		if remotes, err := infoRemotes(a, r.dir); err == nil {
			row.Remotes = remotes
		} else {
			row.Error = &jsonError{Message: "could not inspect remotes: " + err.Error()}
		}
		return row
	})
	if a.ctx.Err() != nil {
		return writeJSONStatus(a, map[string]any{
			"ok":      false,
			"summary": map[string]any{"repositories": len(repos), "failed": len(repos)},
			"error":   jsonError{Message: "operation cancelled"},
		}, 1)
	}
	failed := 0
	for _, result := range results {
		if result.Error != nil {
			failed++
		}
	}
	code := 0
	if failed > 0 {
		code = 1
	}
	_ = writeJSON(a, map[string]any{
		"ok":           code == 0,
		"summary":      map[string]int{"repositories": len(results), "failed": failed},
		"repositories": results,
	})
	return code
}

type infoResult struct {
	stdout string
	stderr string
	failed bool
}

func infoErrorResult(a *app, r repo, label string, err error) infoResult {
	var stderr bytes.Buffer
	fmt.Fprintf(&stderr, "%sError: %s for %s:\n%s%s\n\n", a.ui.Red, label, r.display, err.Error(), a.ui.Reset)
	return infoResult{stderr: stderr.String(), failed: true}
}

func collectInfo(a *app, r repo) infoResult {
	var stdout bytes.Buffer
	errorf := func(label string, err error) infoResult {
		result := infoErrorResult(a, r, label, err)
		result.stdout = stdout.String()
		return result
	}

	fmt.Fprintf(&stdout, "Repository:         %s%s%s\n", a.ui.RepoColor, r.display, a.ui.Reset)
	status, err := a.git.StatusPorcelain(a.ctx, r.dir)
	if err != nil {
		return errorf("Could not inspect status", err)
	}
	if strings.TrimSpace(status) == "" {
		fmt.Fprintf(&stdout, "Status:             %sClean%s\n", a.ui.Green, a.ui.Reset)
	} else {
		fmt.Fprintf(&stdout, "Status:             %sDirty (uncommitted changes or untracked files)%s\n", a.ui.Yellow, a.ui.Reset)
	}
	writeLicenseInfo(&stdout, a, r.dir)
	branch, err := a.git.CurrentBranch(a.ctx, r.dir)
	if err != nil {
		return errorf("Could not inspect branch", err)
	}
	fmt.Fprintf(&stdout, "Current branch:     %s\n", strings.TrimSpace(branch))
	hasCommits := a.git.HasHead(a.ctx, r.dir)
	ab, _ := a.git.Stdout(a.ctx, r.dir, nil, "rev-list", "--left-right", "--count", "HEAD...@{u}")
	if fields := strings.Fields(ab); len(fields) >= 2 {
		fmt.Fprintf(&stdout, "Ahead/behind:       %s ahead, %s behind remote\n", fields[0], fields[1])
	} else {
		fmt.Fprintln(&stdout, "Ahead/behind:       No upstream set")
	}
	if err := writeBranches(&stdout, a, r.dir); err != nil {
		return errorf("Could not inspect branches", err)
	}
	if err := writeRemotes(&stdout, a, r.dir); err != nil {
		return errorf("Could not inspect remotes", err)
	}
	initial, err := a.git.Stdout(a.ctx, r.dir, nil, "log", "--reverse", "--format=%ci - %s")
	if err != nil && hasCommits {
		return errorf("Could not inspect initial commit", err)
	}
	if hasCommits && strings.TrimSpace(initial) != "" {
		fmt.Fprintf(&stdout, "Initial commit:     %s\n", firstLine(initial))
	} else {
		fmt.Fprintln(&stdout, "Initial commit:     None (repository is empty)")
	}
	commitCount, err := a.git.Stdout(a.ctx, r.dir, nil, "rev-list", "--all", "--count")
	if err != nil {
		return errorf("Could not count commits", err)
	}
	fmt.Fprintf(&stdout, "Total commits:      %s\n", strings.TrimSpace(commitCount))
	lastMonth, err := a.git.Stdout(a.ctx, r.dir, nil, "log", "--since=1 month ago", "--format=%ci")
	if err != nil && hasCommits {
		return errorf("Could not inspect recent commits", err)
	}
	fmt.Fprintf(&stdout, "Commits last month: %d\n", len(splitLines(lastMonth)))
	last, err := a.git.Stdout(a.ctx, r.dir, nil, "log", "-1", "--format=%ci - %s")
	if err != nil && hasCommits {
		return errorf("Could not inspect last commit", err)
	}
	fmt.Fprintf(&stdout, "Last commit:        %s\n", strings.TrimSpace(last))
	if hasCommits {
		if err := writeTopAuthors(&stdout, a, r.dir); err != nil {
			return errorf("Could not inspect authors", err)
		}
	} else {
		fmt.Fprintln(&stdout, "Top authors:        None (repository is empty)")
	}
	if err := writeLargestFiles(&stdout, a, r.dir); err != nil {
		return errorf("Could not inspect largest files", err)
	}
	fmt.Fprintln(&stdout)
	return infoResult{stdout: stdout.String()}
}

func printLicenseInfo(a *app, dir string) {
	writeLicenseInfo(a.stdout, a, dir)
}

func writeLicenseInfo(w io.Writer, a *app, dir string) {
	fmt.Fprint(w, "License:            ")
	data, err := os.ReadFile(filepath.Join(dir, "LICENSE"))
	if err != nil {
		fmt.Fprintf(w, "%sNone%s\n", a.ui.Yellow, a.ui.Reset)
		return
	}
	line := firstLine(string(data))
	if line == "" {
		fmt.Fprintf(w, "%sYes%s\n", a.ui.Green, a.ui.Reset)
	} else {
		fmt.Fprintf(w, "%s'%s'%s\n", a.ui.Green, truncate(line, 70), a.ui.Reset)
	}
}

func printBranches(a *app, dir string) error {
	return writeBranches(a.stdout, a, dir)
}

func writeBranches(w io.Writer, a *app, dir string) error {
	out, err := a.git.Stdout(a.ctx, dir, nil, "branch", "-a", "--no-color")
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
	fmt.Fprintf(w, "Branches (%d):       ", len(branches))
	for i, branch := range branches {
		if i == 0 {
			fmt.Fprintln(w, branch)
		} else {
			fmt.Fprintf(w, "%-20s%s\n", "", branch)
		}
	}
	if len(branches) == 0 {
		fmt.Fprintln(w)
	}
	return nil
}

func printRemotes(a *app, dir string) error {
	return writeRemotes(a.stdout, a, dir)
}

func writeRemotes(w io.Writer, a *app, dir string) error {
	remotes, err := infoRemotes(a, dir)
	if err != nil {
		return err
	}
	if len(remotes) == 0 {
		fmt.Fprintln(w, "Remotes:            None")
		return nil
	}
	for i, remote := range remotes {
		if i == 0 {
			fmt.Fprintf(w, "Remotes:            %s\n", remote)
		} else {
			fmt.Fprintf(w, "%-20s%s\n", "", remote)
		}
	}
	return nil
}

func infoRemotes(a *app, dir string) ([]string, error) {
	out, err := a.git.Stdout(a.ctx, dir, nil, "remote", "-v")
	if err != nil {
		return nil, err
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
	return remotes, nil
}

func printTopAuthors(a *app, dir string) error {
	return writeTopAuthors(a.stdout, a, dir)
}

func writeTopAuthors(w io.Writer, a *app, dir string) error {
	out, err := a.git.Stdout(a.ctx, dir, nil, "log", "--format=%an <%ae>")
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
			fmt.Fprintf(w, "Top authors:        %d - %s\n", row.count, row.name)
		} else {
			fmt.Fprintf(w, "%-20s%d - %s\n", "", row.count, row.name)
		}
	}
	return nil
}

type largestFileRow struct {
	size int64
	path string
}

func printLargestFiles(a *app, dir string) error {
	return writeLargestFiles(a.stdout, a, dir)
}

func writeLargestFiles(w io.Writer, a *app, dir string) error {
	seen := map[string]bool{}
	rows := []largestFileRow{}
	err := a.git.CatFileBatchCheckAllObjects(a.ctx, dir, func(output io.Reader) error {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			line := scanner.Text()
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
			rows = appendTopLargest(rows, largestFileRow{size: size, path: path}, 3)
		}
		return scanner.Err()
	})
	if err != nil {
		return err
	}
	for i, row := range rows {
		if i == 0 {
			fmt.Fprintf(w, "Largest files:      %s - %s\n", humanSize(row.size), row.path)
		} else {
			fmt.Fprintf(w, "%-20s%s - %s\n", "", humanSize(row.size), row.path)
		}
	}
	return nil
}

func appendTopLargest(rows []largestFileRow, next largestFileRow, limit int) []largestFileRow {
	rows = append(rows, next)
	sort.Slice(rows, func(i, j int) bool { return rows[i].size > rows[j].size })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
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
