package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func runReset(a *app, cmd *cobra.Command, args []string) int {
	confirmed, _ := cmd.Flags().GetBool("confirm")
	if !requireGit(a, "reset") {
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
	for _, r := range repos {
		branchOut, _ := runStdout(r.dir, nil, "git", "rev-parse", "--abbrev-ref", "HEAD")
		branch := strings.TrimSpace(branchOut)
		if branch == "HEAD" {
			fmt.Fprintf(a.stdout, "%s%s is in detached HEAD state. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		if out, err := runCapture(r.dir, nil, "git", "fetch", "origin", branch); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Fetch failed for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			continue
		}
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "--verify", "--quiet", "origin/"+branch); err != nil {
			fmt.Fprintf(a.stdout, "%sBranch '%s' has no remote counterpart in %s. Skipping...%s\n", a.ui.Yellow, branch, r.display, a.ui.Reset)
			continue
		}
		ahead := strings.TrimSpace(mustStdout(r.dir, "git", "rev-list", "--count", "origin/"+branch+".."+branch))
		behind := strings.TrimSpace(mustStdout(r.dir, "git", "rev-list", "--count", branch+"..origin/"+branch))
		if ahead == "" {
			ahead = "0"
		}
		if behind == "" {
			behind = "0"
		}
		fmt.Fprintf(a.stdout, "%s--- %s (%s) ---%s\n", a.ui.Cyan, r.display, branch, a.ui.Reset)
		if ahead == "0" && behind == "0" {
			fmt.Fprintf(a.stdout, "%sBranch '%s' is already up to date with origin/%s in %s. Nothing to reset.%s\n", a.ui.Yellow, branch, branch, r.display, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stderr, "Divergence: %sahead %s%s, %sbehind %s%s\n", a.ui.Cyan, ahead, a.ui.Reset, a.ui.Red, behind, a.ui.Reset)
		dirty, _ := runStdout(r.dir, nil, "git", "status", "--porcelain")
		if strings.TrimSpace(dirty) != "" {
			fmt.Fprintf(a.stderr, "%sWarning: Working tree has uncommitted changes that will be discarded.%s\n", a.ui.Red, a.ui.Reset)
		}
		fmt.Fprintf(a.stderr, "%sThis will hard reset %s to origin/%s, discarding %s local commit(s).%s\n", a.ui.Red, branch, branch, ahead, a.ui.Reset)
		if !confirmed && !confirm(a, "Proceed with reset for "+r.display+"?") {
			fmt.Fprintf(a.stdout, "%sSkipping %s.%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		if out, err := runCapture(r.dir, nil, "git", "reset", "--hard", "origin/"+branch); err == nil {
			fmt.Fprintf(a.stdout, "%sSuccessfully reset %s to origin/%s%s\n", a.ui.Green, r.display, branch, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Reset failed for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
		}
	}
	return 0
}
