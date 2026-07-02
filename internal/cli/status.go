package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type statusOptions struct {
	target targetOptions
	json   jsonOptions
	fetch  fetchOptions
}

func statusOptionsFromCommand(cmd *cobra.Command) statusOptions {
	return statusOptions{
		target: targetOptionsFromCommand(cmd),
		json:   jsonOptionsFromCommand(cmd),
		fetch:  fetchOptionsFromCommand(cmd),
	}
}

func runStatus(a *app, cmd *cobra.Command, args []string) int {
	opts := statusOptionsFromCommand(cmd)
	if opts.json.enabled {
		return runStatusJSON(a, opts)
	}
	if !requireGit(a, "status") {
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

	totalClean := 0
	totalDirty := 0
	totalBehind := 0
	totalNoRemote := 0
	totalFailed := 0
	status := 0
	tableRows := [][]string{}

	type statusResult struct {
		repo repo
		row  statusTableRow
		err  error
	}
	results := parallelReposProgress(a.ctx, repos, newProgress(a, "Checking status", len(repos)), func(r repo) statusResult {
		if failure, ok := fetchFailures[r.dir]; ok {
			return statusResult{repo: r, err: fmt.Errorf("%s", fetchFailureMessage(failure))}
		}
		row, err := statusRow(a, r)
		return statusResult{repo: r, row: row, err: err}
	})
	if interrupted(a) {
		return 1
	}
	for _, result := range results {
		if result.err != nil {
			renderErrorBlock(a, fmt.Sprintf("%s: could not inspect status", result.repo.display), result.err.Error())
			tableRows = append(tableRows, []string{result.repo.display, a.ui.Red + "ERROR" + a.ui.Reset, "failed"})
			status = 1
			totalFailed++
			continue
		}
		row := result.row
		tableRows = append(tableRows, []string{row.name, row.state, row.tracking})
		if row.dirty == 0 {
			totalClean++
		}
		totalDirty += row.dirty
		totalBehind += row.behind
		totalNoRemote += row.noRemote
	}

	renderTable(a, []tableColumn{{header: "Repository", max: 30}, {header: "State"}, {header: "Tracking"}}, tableRows)
	fmt.Fprintln(a.stdout)
	renderSummary(a,
		summaryCount{label: "clean", value: totalClean, color: a.ui.Green},
		summaryCount{label: "dirty", value: totalDirty, color: a.ui.Yellow},
		summaryCount{label: "behind", value: totalBehind, color: a.ui.Red},
		summaryCount{label: "no remote", value: totalNoRemote, color: a.ui.Muted},
		summaryCount{label: "failed", value: totalFailed, color: a.ui.Red},
	)
	return status
}

func runStatusJSON(a *app, opts statusOptions) int {
	if _, err := a.runner.LookPath("git"); err != nil {
		return writeJSONStatus(a, map[string]any{
			"ok":      false,
			"summary": map[string]any{"repositories": 0, "failed": 1},
			"error":   jsonError{Message: "'git' is required for status. Install it and make sure it is on PATH."},
		})
	}
	repos, err := opts.target.repositories()
	if err != nil {
		return writeJSONStatus(a, map[string]any{
			"ok":      false,
			"summary": map[string]any{"repositories": 0, "failed": 1},
			"error":   jsonError{Message: err.Error()},
		})
	}
	fetchFailures := map[string]originRefreshResult{}
	if !opts.fetch.noFetch {
		fetchFailures = refreshFailuresByDir(refreshOrigin(a, repos))
		if a.ctx.Err() != nil {
			return writeJSONStatus(a, map[string]any{
				"ok":      false,
				"summary": map[string]any{"repositories": len(repos), "failed": len(repos)},
				"error":   jsonError{Message: "operation cancelled"},
			})
		}
	}
	type statusJSONRepo struct {
		Name     string     `json:"name"`
		Path     string     `json:"path"`
		State    string     `json:"state"`
		Tracking string     `json:"tracking"`
		Ahead    int        `json:"ahead"`
		Behind   int        `json:"behind"`
		Error    *jsonError `json:"error,omitempty"`
	}
	rows := []statusJSONRepo{}
	summary := map[string]int{"repositories": len(repos), "clean": 0, "dirty": 0, "behind": 0, "noRemote": 0, "failed": 0}
	for _, r := range repos {
		row := statusJSONRepo{Name: r.display, Path: r.dir, State: "clean", Tracking: "up to date"}
		if failure, ok := fetchFailures[r.dir]; ok {
			row.Error = &jsonError{Message: fetchFailureMessage(failure)}
			summary["failed"]++
			rows = append(rows, row)
			continue
		}
		detail, err := statusDetails(a, r)
		row.Ahead = detail.ahead
		row.Behind = detail.behind
		if err != nil {
			row.Error = &jsonError{Message: err.Error()}
			summary["failed"]++
			rows = append(rows, row)
			continue
		}
		if detail.dirty {
			row.State = "dirty"
			summary["dirty"]++
		} else {
			summary["clean"]++
		}
		switch {
		case !detail.hasUpstream:
			row.Tracking = "no remote"
			summary["noRemote"]++
		case detail.ahead > 0 && detail.behind > 0:
			row.Tracking = "ahead and behind"
			summary["behind"]++
		case detail.ahead > 0:
			row.Tracking = "ahead"
		case detail.behind > 0:
			row.Tracking = "behind"
			summary["behind"]++
		}
		rows = append(rows, row)
	}
	code := 0
	if summary["failed"] > 0 {
		code = 1
	}
	_ = writeJSON(a, map[string]any{
		"ok":           code == 0,
		"summary":      summary,
		"repositories": rows,
	})
	return code
}

type statusTableRow struct {
	name     string
	state    string
	tracking string
	dirty    int
	behind   int
	noRemote int
}

type statusDetail struct {
	dirty       bool
	hasUpstream bool
	ahead       int
	behind      int
}

func statusDetails(a *app, r repo) (statusDetail, error) {
	out, err := a.git.StatusPorcelainBranch(a.ctx, r.dir)
	if err != nil {
		return statusDetail{}, err
	}
	detail := statusDetail{}
	for _, line := range splitLines(out) {
		switch {
		case strings.HasPrefix(line, "# branch.upstream "):
			detail.hasUpstream = true
		case strings.HasPrefix(line, "# branch.ab "):
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				detail.ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[2], "+"))
				detail.behind, _ = strconv.Atoi(strings.TrimPrefix(fields[3], "-"))
			}
		case strings.HasPrefix(line, "#"), line == "":
		default:
			detail.dirty = true
		}
	}
	return detail, nil
}

func statusRow(a *app, r repo) (statusTableRow, error) {
	detail, err := statusDetails(a, r)
	if err != nil {
		return statusTableRow{}, err
	}

	name := r.display
	if len(name) > 30 {
		name = truncate(name, 30)
	}
	state := a.ui.Green + "clean" + a.ui.Reset
	dirty := 0
	if detail.dirty {
		state = a.ui.Yellow + "dirty" + a.ui.Reset
		dirty = 1
	}

	tracking := "up to date"
	behind := 0
	noRemote := 0
	if !detail.hasUpstream {
		tracking = a.ui.Muted + "no remote" + a.ui.Reset
		noRemote = 1
	} else if detail.ahead > 0 && detail.behind > 0 {
		tracking = fmt.Sprintf("%sahead %d%s, %sbehind %d%s", a.ui.Cyan, detail.ahead, a.ui.Reset, a.ui.Red, detail.behind, a.ui.Reset)
		behind = 1
	} else if detail.ahead > 0 {
		tracking = fmt.Sprintf("%sahead %d%s", a.ui.Cyan, detail.ahead, a.ui.Reset)
	} else if detail.behind > 0 {
		tracking = fmt.Sprintf("%sbehind %d%s", a.ui.Red, detail.behind, a.ui.Reset)
		behind = 1
	}

	return statusTableRow{name: name, state: state, tracking: tracking, dirty: dirty, behind: behind, noRemote: noRemote}, nil
}
