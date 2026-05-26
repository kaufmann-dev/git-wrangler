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
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}

	fmt.Fprintf(a.stdout, "%-30s | %-5s | %s\n", "REPOSITORY", "STATE", "TRACKING")
	fmt.Fprintln(a.stdout, "-------------------------------+-------+------------------------")

	totalDirty := 0
	totalBehind := 0
	totalNoRemote := 0

	for _, r := range repos {
		row, err := statusRow(a, r)
		if err != nil {
			continue
		}
		fmt.Fprintf(a.stdout, "%-30s | %s | %s\n", row.name, row.state, row.tracking)
		totalDirty += row.dirty
		totalBehind += row.behind
		totalNoRemote += row.noRemote
	}

	fmt.Fprintln(a.stdout, "-------------------------------+-------+------------------------")
	fmt.Fprintf(a.stderr, "Summary: %s%d dirty%s, %s%d behind%s, %s%d no remote%s\n",
		a.ui.Yellow, totalDirty, a.ui.Reset,
		a.ui.Red, totalBehind, a.ui.Reset,
		a.ui.Muted, totalNoRemote, a.ui.Reset)
	return 0
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
	out, _ := runStdout(r.dir, nil, "git", "status", "--porcelain=v2", "--branch")
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
