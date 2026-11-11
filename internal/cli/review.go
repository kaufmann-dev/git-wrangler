package cli

import (
	"fmt"
	gitclient "github.com/kaufmann-dev/git-wrangler/internal/git"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

type reviewOptions struct {
	target targetOptions
	json   jsonOptions
	fetch  fetchOptions
}

func reviewOptionsFromCommand(cmd *cobra.Command) reviewOptions {
	return reviewOptions{
		target: targetOptionsFromCommand(cmd),
		json:   jsonOptionsFromCommand(cmd),
		fetch:  fetchOptionsFromCommand(cmd),
	}
}

type reviewResult struct {
	Name       string                 `json:"name"`
	Path       string                 `json:"path"`
	Comparison gitclient.Comparison   `json:"comparison"`
	Staged     []gitclient.FileChange `json:"staged"`
	Unstaged   []gitclient.FileChange `json:"unstaged"`
	Untracked  []gitclient.FileChange `json:"untracked"`
	Unpushed   []gitclient.FileChange `json:"unpushed"`
	Error      *jsonError             `json:"error,omitempty"`
}

func runReview(a *app, cmd *cobra.Command, args []string) int {
	opts := reviewOptionsFromCommand(cmd)
	fatal := func(err error) int {
		if opts.json.enabled {
			return writeJSONStatus(a, map[string]any{"ok": false, "error": jsonError{Message: err.Error()}})
		}
		a.error(err.Error())
		return 1
	}
	if _, err := a.runner.LookPath("git"); err != nil {
		return fatal(fmt.Errorf("'git' is required for review: %w", err))
	}
	repos, err := opts.target.repositories()
	if err != nil {
		return fatal(err)
	}
	failures := map[string]originRefreshResult{}
	if !opts.fetch.noFetch {
		failures = refreshFailuresByDir(refreshOrigin(a, repos))
	}
	if a.ctx.Err() != nil {
		if !opts.json.enabled {
			interrupted(a)
			return 1
		}
		return fatal(a.ctx.Err())
	}
	collect := func(r repo) reviewResult {
		row := reviewResult{Name: r.display, Path: r.dir, Comparison: gitclient.Comparison{State: "unavailable"}}
		record := func(err error) {
			if err != nil {
				if row.Error == nil {
					row.Error = &jsonError{Message: err.Error()}
				} else {
					row.Error.Message += "; " + err.Error()
				}
			}
		}
		var e error
		row.Staged, e = a.git.DiffChanges(a.ctx, r.dir, "--cached")
		record(e)
		row.Unstaged, e = a.git.DiffChanges(a.ctx, r.dir)
		record(e)
		row.Untracked, e = a.git.UntrackedChanges(a.ctx, r.dir)
		record(e)
		if failure, ok := failures[r.dir]; ok {
			record(fmt.Errorf("%s", fetchFailureMessage(failure)))
			return row
		}
		row.Comparison, e = a.git.CompareOrigin(a.ctx, r.dir)
		record(e)
		if e != nil || row.Comparison.State == "detached" {
			return row
		}
		row.Unpushed = []gitclient.FileChange{}
		if row.Comparison.Ahead != nil && *row.Comparison.Ahead > 0 {
			row.Unpushed, e = a.git.DiffChanges(a.ctx, r.dir, row.Comparison.Base, "HEAD")
			record(e)
		}
		return row
	}
	var results []reviewResult
	if opts.json.enabled {
		results = parallelRepos(a.ctx, repos, collect)
	} else {
		results = parallelReposProgress(a.ctx, repos, newProgress(a, "Reviewing repositories", len(repos)), collect)
	}
	if a.ctx.Err() != nil {
		if !opts.json.enabled {
			interrupted(a)
			return 1
		}
		return fatal(a.ctx.Err())
	}
	summary := map[string]int{"repositories": len(results), "changed": 0, "clean": 0, "unavailable": 0, "failed": 0}
	for _, r := range results {
		changed := len(r.Staged)+len(r.Unstaged)+len(r.Untracked) > 0 || (r.Comparison.Ahead != nil && *r.Comparison.Ahead > 0)
		switch {
		case r.Error != nil:
			summary["failed"]++
		case changed:
			summary["changed"]++
		case r.Unpushed == nil:
			summary["unavailable"]++
		default:
			summary["clean"]++
		}
		if opts.json.enabled || (!changed && r.Unpushed != nil && r.Error == nil) {
			continue
		}
		renderRepoHeader(a, r.Name)
		renderReviewSection(a, "Staged", r.Staged, false)
		renderReviewSection(a, "Unstaged", r.Unstaged, false)
		renderReviewSection(a, "Untracked", r.Untracked, false)
		if r.Unpushed == nil {
			fmt.Fprintf(a.stdout, "  %sUnpushed: unavailable (%s)%s\n", a.ui.Muted, r.Comparison.State, a.ui.Reset)
		} else if r.Comparison.Ahead != nil && *r.Comparison.Ahead > 0 {
			label := fmt.Sprintf("Unpushed — %d %s → %s", *r.Comparison.Ahead, plural(*r.Comparison.Ahead, "commit", "commits"), reviewPath(r.Comparison.Target))
			if r.Comparison.State == "unpublished" {
				label += " (new branch on origin; full snapshot)"
			} else if r.Comparison.Method != "merge base" {
				label += " (" + r.Comparison.Method + ")"
			}
			renderReviewSection(a, label, r.Unpushed, true)
		}
		fmt.Fprintln(a.stdout)
		if r.Error != nil {
			renderErrorBlock(a, r.Name+": could not complete review", r.Error.Message)
		}
	}
	code := 0
	if summary["failed"] > 0 {
		code = 1
	}
	if opts.json.enabled {
		if results == nil {
			results = []reviewResult{}
		}
		_ = writeJSON(a, map[string]any{"ok": code == 0, "summary": summary, "repositories": results})
		return code
	}
	renderSummary(a, summaryCount{label: "with changes", value: summary["changed"], color: a.ui.Yellow}, summaryCount{label: "clean", value: summary["clean"], color: a.ui.Green}, summaryCount{label: "comparison unavailable", value: summary["unavailable"], color: a.ui.Muted}, summaryCount{label: "failed", value: summary["failed"], color: a.ui.Red})
	return code
}
func reviewPath(path string) string {
	if strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return strconv.Quote(path)
	}
	return path
}
func reviewStatusStyle(a *app, status string) (string, string) {
	switch status {
	case "A":
		return "Added", a.ui.Green
	case "C":
		return "Copied", a.ui.Green
	case "M":
		return "Edited", a.ui.Yellow
	case "D":
		return "Removed", a.ui.Red
	case "R":
		return "Renamed", a.ui.Cyan
	case "T":
		return "Type changed", a.ui.Cyan
	case "U":
		return "Conflicted", a.ui.Red
	}
	return status, ""
}

func renderReviewSection(a *app, label string, files []gitclient.FileChange, force bool) {
	if len(files) == 0 && !force {
		return
	}
	added, deleted := 0, 0
	for _, f := range files {
		if f.Additions != nil {
			added += *f.Additions
		}
		if f.Deletions != nil {
			deleted += *f.Deletions
		}
	}
	fmt.Fprintf(a.stdout, "  %s%s%s — %d %s, %s+%d%s %s-%d%s\n", a.ui.Bold, label, a.ui.Reset, len(files), plural(len(files), "file", "files"), a.ui.Green, added, a.ui.Reset, a.ui.Red, deleted, a.ui.Reset)
	if len(files) == 0 {
		fmt.Fprintf(a.stdout, "    %sNo net file changes%s\n", a.ui.Muted, a.ui.Reset)
		return
	}
	type row struct {
		label, color, path string
		add, del           int
		counted, binary    bool
	}
	rows := make([]row, 0, len(files))
	labelWidth, pathWidth, maxChange := 0, 0, 0
	for _, f := range files {
		r := row{path: reviewPath(f.Path), binary: f.Binary}
		r.label, r.color = reviewStatusStyle(a, f.Status)
		if f.PreviousPath != "" {
			r.path = reviewPath(f.PreviousPath) + " → " + r.path
		}
		if !f.Binary && f.Additions != nil && f.Deletions != nil {
			r.add, r.del, r.counted = *f.Additions, *f.Deletions, true
			maxChange = max(maxChange, r.add+r.del)
		}
		labelWidth = max(labelWidth, visibleWidth(r.label))
		pathWidth = max(pathWidth, visibleWidth(r.path))
		rows = append(rows, r)
	}
	countWidth := max(len(strconv.Itoa(maxChange)), len("Bin"))
	graphWidth := 40
	if width := terminalWidth(a.stdout); width > 0 {
		prefix := 4 + labelWidth + 2 + pathWidth + 3 + countWidth + 1
		graphWidth = min(max(width-prefix, 10), 50)
	}
	for _, r := range rows {
		fmt.Fprintf(a.stdout, "    %s%s%s%s  %s%s | ", r.color, r.label, a.ui.Reset, strings.Repeat(" ", labelWidth-visibleWidth(r.label)), r.path, strings.Repeat(" ", pathWidth-visibleWidth(r.path)))
		switch {
		case r.binary:
			fmt.Fprintf(a.stdout, "%s%*s%s\n", a.ui.Muted, countWidth, "Bin", a.ui.Reset)
		case !r.counted:
			fmt.Fprintf(a.stdout, "%sunavailable%s\n", a.ui.Muted, a.ui.Reset)
		default:
			fmt.Fprintf(a.stdout, "%*d%s\n", countWidth, r.add+r.del, diffstatBar(a, r.add, r.del, graphWidth, maxChange))
		}
	}
}

func diffstatBar(a *app, add, del, width, maxChange int) string {
	add, del = diffstatScale(add, del, width, maxChange)
	if add+del == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte(' ')
	if add > 0 {
		b.WriteString(a.ui.Green + strings.Repeat("+", add) + a.ui.Reset)
	}
	if del > 0 {
		b.WriteString(a.ui.Red + strings.Repeat("-", del) + a.ui.Reset)
	}
	return b.String()
}

// diffstatScale scales a file's additions and deletions into a graph of at
// most width columns, matching git's --stat scaling.
func diffstatScale(add, del, width, maxChange int) (int, int) {
	if maxChange <= width {
		return add, del
	}
	scale := func(it, w, m int) int {
		if it == 0 {
			return 0
		}
		return 1 + it*(w-1)/m
	}
	total := scale(add+del, width, maxChange)
	if total < 2 && add > 0 && del > 0 {
		total = 2
	}
	if add < del {
		add = scale(add, total, add+del)
		return add, total - add
	}
	del = scale(del, total, add+del)
	return total - del, del
}
