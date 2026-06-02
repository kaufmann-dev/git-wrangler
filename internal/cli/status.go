package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func runStatus(a *app, cmd *cobra.Command, args []string) int {
	if len(args) > 0 {
		a.errorf("Unknown option: %s", args[0])
		return 1
	}
	if !requireGit(a, "status") {
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

	fmt.Fprintf(a.stdout, "%-30s | %-5s | %s\n", "REPOSITORY", "STATE", "TRACKING")
	fmt.Fprintln(a.stdout, "-------------------------------+-------+------------------------")

	totalClean := 0
	totalDirty := 0
	totalBehind := 0
	totalNoRemote := 0
	totalFailed := 0
	status := 0

	type statusResult struct {
		repo repo
		row  statusTableRow
		err  error
	}
	results := parallelReposProgress(repos, newProgress(a, "Checking status", len(repos)), func(r repo) statusResult {
		row, err := statusRow(a, r)
		return statusResult{repo: r, row: row, err: err}
	})
	for _, result := range results {
		if result.err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect status for %s:\n%s%s\n\n", a.ui.Red, result.repo.display, result.err.Error(), a.ui.Reset)
			status = 1
			totalFailed++
			continue
		}
		row := result.row
		fmt.Fprintf(a.stdout, "%-30s | %s | %s\n", row.name, row.state, row.tracking)
		if row.dirty == 0 {
			totalClean++
		}
		totalDirty += row.dirty
		totalBehind += row.behind
		totalNoRemote += row.noRemote
	}

	fmt.Fprintln(a.stdout, "-------------------------------+-------+------------------------")
	fmt.Fprintf(a.stdout, "Summary: %s%d clean%s, %s%d dirty%s, %s%d behind%s, %s%d no remote%s, %s%d failed%s\n",
		a.ui.Green, totalClean, a.ui.Reset,
		a.ui.Yellow, totalDirty, a.ui.Reset,
		a.ui.Red, totalBehind, a.ui.Reset,
		a.ui.Muted, totalNoRemote, a.ui.Reset,
		a.ui.Red, totalFailed, a.ui.Reset)
	return status
}

type statusTableRow struct {
	name     string
	state    string
	tracking string
	dirty    int
	behind   int
	noRemote int
}

func statusRow(a *app, r repo) (statusTableRow, error) {
	out, err := a.git.StatusPorcelainBranch(a.ctx, r.dir)
	if err != nil {
		return statusTableRow{}, err
	}
	isDirty := false
	hasUpstream := false
	aheadCount := 0
	behindCount := 0
	for _, line := range splitLines(out) {
		switch {
		case strings.HasPrefix(line, "# branch.upstream "):
			hasUpstream = true
		case strings.HasPrefix(line, "# branch.ab "):
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				aheadCount, _ = strconv.Atoi(strings.TrimPrefix(fields[2], "+"))
				behindCount, _ = strconv.Atoi(strings.TrimPrefix(fields[3], "-"))
			}
		case strings.HasPrefix(line, "#"), line == "":
		default:
			isDirty = true
		}
	}

	name := r.display
	if len(name) > 30 {
		name = truncate(name, 30)
	}
	state := a.ui.Green + "clean" + a.ui.Reset
	dirty := 0
	if isDirty {
		state = a.ui.Yellow + "dirty" + a.ui.Reset
		dirty = 1
	}

	tracking := "up to date"
	behind := 0
	noRemote := 0
	if !hasUpstream {
		tracking = a.ui.Muted + "no remote" + a.ui.Reset
		noRemote = 1
	} else if aheadCount > 0 && behindCount > 0 {
		tracking = fmt.Sprintf("%sahead %d%s, %sbehind %d%s", a.ui.Cyan, aheadCount, a.ui.Reset, a.ui.Red, behindCount, a.ui.Reset)
		behind = 1
	} else if aheadCount > 0 {
		tracking = fmt.Sprintf("%sahead %d%s", a.ui.Cyan, aheadCount, a.ui.Reset)
	} else if behindCount > 0 {
		tracking = fmt.Sprintf("%sbehind %d%s", a.ui.Red, behindCount, a.ui.Reset)
		behind = 1
	}

	return statusTableRow{name: name, state: state, tracking: tracking, dirty: dirty, behind: behind, noRemote: noRemote}, nil
}
