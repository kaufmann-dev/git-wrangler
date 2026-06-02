package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func runReset(a *app, cmd *cobra.Command, args []string) int {
	yes := yesFlag(cmd)
	if !requireGit(a, "reset") {
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
	status := 0
	progress := newProgress(a, "Resetting repositories", len(repos))
	for _, r := range repos {
		branchOut, err := a.git.CurrentBranch(a.ctx, r.dir)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not determine current branch for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			status = 1
			progress.advance(r.display)
			continue
		}
		branch := strings.TrimSpace(branchOut)
		if branch == "HEAD" {
			fmt.Fprintf(a.stdout, "%s%s is in detached HEAD state. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			progress.advance(r.display)
			continue
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "fetch", "origin", branch); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Fetch failed for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			status = 1
			progress.advance(r.display)
			continue
		}
		if !a.git.VerifyRef(a.ctx, r.dir, "origin/"+branch) {
			fmt.Fprintf(a.stdout, "%sBranch '%s' has no remote counterpart in %s. Skipping...%s\n", a.ui.Yellow, branch, r.display, a.ui.Reset)
			progress.advance(r.display)
			continue
		}
		ahead, err := a.git.Stdout(a.ctx, r.dir, nil, "rev-list", "--count", "origin/"+branch+".."+branch)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not calculate ahead count for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			status = 1
			progress.advance(r.display)
			continue
		}
		behind, err := a.git.Stdout(a.ctx, r.dir, nil, "rev-list", "--count", branch+"..origin/"+branch)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not calculate behind count for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			status = 1
			progress.advance(r.display)
			continue
		}
		ahead = strings.TrimSpace(ahead)
		behind = strings.TrimSpace(behind)
		fmt.Fprintf(a.stdout, "%s--- %s (%s) ---%s\n", a.ui.Cyan, r.display, branch, a.ui.Reset)
		if ahead == "0" && behind == "0" {
			fmt.Fprintf(a.stdout, "%sBranch '%s' is already up to date with origin/%s in %s. Nothing to reset.%s\n", a.ui.Yellow, branch, branch, r.display, a.ui.Reset)
			progress.advance(r.display)
			continue
		}
		fmt.Fprintf(a.stderr, "Divergence: %sahead %s%s, %sbehind %s%s\n", a.ui.Cyan, ahead, a.ui.Reset, a.ui.Red, behind, a.ui.Reset)
		dirty, err := a.git.StatusPorcelain(a.ctx, r.dir)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect working tree for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			status = 1
			progress.advance(r.display)
			continue
		}
		if strings.TrimSpace(dirty) != "" {
			fmt.Fprintf(a.stderr, "%sWarning: Working tree has uncommitted changes that will be discarded.%s\n", a.ui.Red, a.ui.Reset)
		}
		fmt.Fprintf(a.stderr, "%sThis will hard reset %s to origin/%s, discarding %s local commit(s).%s\n", a.ui.Red, branch, branch, ahead, a.ui.Reset)
		if !yes && !confirm(a, "Proceed with reset for "+r.display+"?") {
			fmt.Fprintf(a.stdout, "%sSkipping %s.%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			progress.advance(r.display)
			continue
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "reset", "--hard", "origin/"+branch); err == nil {
			fmt.Fprintf(a.stdout, "%sSuccessfully reset %s to origin/%s%s\n", a.ui.Green, r.display, branch, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Reset failed for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			status = 1
		}
		progress.advance(r.display)
	}
	progress.done()
	return status
}
