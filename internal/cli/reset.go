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
	repos, err := commandRepositoryTargets(cmd)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	type resetApply struct {
		repo   repo
		branch string
		ahead  string
		behind string
		dirty  bool
	}
	type resetSkip struct {
		repo   string
		reason string
	}
	type resetFailure struct {
		subject string
		output  string
	}
	status := 0
	applies := []resetApply{}
	skips := []resetSkip{}
	failures := []resetFailure{}
	progress := newProgress(a, "Preparing resets", len(repos))
	for _, r := range repos {
		branchOut, err := a.git.CurrentBranch(a.ctx, r.dir)
		if err != nil {
			failures = append(failures, resetFailure{subject: r.display + ": could not determine current branch", output: err.Error()})
			status = 1
			progress.advance(r.display)
			continue
		}
		branch := strings.TrimSpace(branchOut)
		if branch == "HEAD" {
			skips = append(skips, resetSkip{repo: r.display, reason: "detached HEAD"})
			progress.advance(r.display)
			continue
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "fetch", "origin", branch); err != nil {
			failures = append(failures, resetFailure{subject: r.display + ": fetch failed", output: outputOrError(out, err)})
			status = 1
			progress.advance(r.display)
			continue
		}
		if !a.git.VerifyRef(a.ctx, r.dir, "origin/"+branch) {
			skips = append(skips, resetSkip{repo: r.display, reason: "branch '" + branch + "' has no remote counterpart"})
			progress.advance(r.display)
			continue
		}
		ahead, err := a.git.Stdout(a.ctx, r.dir, nil, "rev-list", "--count", "origin/"+branch+".."+branch)
		if err != nil {
			failures = append(failures, resetFailure{subject: r.display + ": could not calculate ahead count", output: err.Error()})
			status = 1
			progress.advance(r.display)
			continue
		}
		behind, err := a.git.Stdout(a.ctx, r.dir, nil, "rev-list", "--count", branch+"..origin/"+branch)
		if err != nil {
			failures = append(failures, resetFailure{subject: r.display + ": could not calculate behind count", output: err.Error()})
			status = 1
			progress.advance(r.display)
			continue
		}
		ahead = strings.TrimSpace(ahead)
		behind = strings.TrimSpace(behind)
		if ahead == "0" && behind == "0" {
			skips = append(skips, resetSkip{repo: r.display, reason: "already up to date"})
			progress.advance(r.display)
			continue
		}
		dirty, err := a.git.StatusPorcelain(a.ctx, r.dir)
		if err != nil {
			failures = append(failures, resetFailure{subject: r.display + ": could not inspect working tree", output: err.Error()})
			status = 1
			progress.advance(r.display)
			continue
		}
		applies = append(applies, resetApply{repo: r, branch: branch, ahead: ahead, behind: behind, dirty: strings.TrimSpace(dirty) != ""})
		progress.advance(r.display)
	}
	finishProgressBeforeOutput(progress)
	for _, failure := range failures {
		renderErrorBlock(a, failure.subject, failure.output)
	}
	for _, skip := range skips {
		renderStatusLine(a, a.stdout, statusSkip, skip.repo, skip.reason)
	}
	if len(applies) == 0 {
		renderSummary(a,
			summaryCount{label: "reset", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: len(skips), color: a.ui.Yellow},
			summaryCount{label: "failed", value: len(failures), color: a.ui.Red},
		)
		return status
	}
	tableRows := make([][]string, 0, len(applies))
	for _, apply := range applies {
		dirty := "no"
		if apply.dirty {
			dirty = a.ui.Red + "yes" + a.ui.Reset
		}
		tableRows = append(tableRows, []string{apply.repo.display, apply.branch, apply.ahead, apply.behind, dirty})
	}
	renderTable(a, []tableColumn{{header: "Repository"}, {header: "Branch"}, {header: "Ahead"}, {header: "Behind"}, {header: "Dirty"}}, tableRows)
	fmt.Fprintln(a.stdout)
	renderWarning(a, fmt.Sprintf("This will hard reset %d repositories and discard local commits or working tree changes.", len(applies)))
	if !confirmOrSkip(a, yes, fmt.Sprintf("Proceed with reset for %d repositories?", len(applies))) {
		renderSummary(a,
			summaryCount{label: "reset", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: len(applies) + len(skips), color: a.ui.Yellow},
			summaryCount{label: "failed", value: len(failures), color: a.ui.Red},
		)
		return status
	}
	reset := 0
	applyFailures := []resetFailure{}
	applyProgress := newProgress(a, "Resetting repositories", len(applies))
	for _, apply := range applies {
		r := apply.repo
		branch := apply.branch
		applyProgress.start(r.display)
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "reset", "--hard", "origin/"+branch); err == nil {
			reset++
		} else {
			applyFailures = append(applyFailures, resetFailure{subject: r.display + ": reset failed", output: outputOrError(out, err)})
			status = 1
		}
		applyProgress.advance(r.display)
	}
	finishProgressBeforeOutput(applyProgress)
	for _, failure := range applyFailures {
		renderErrorBlock(a, failure.subject, failure.output)
	}
	renderSummary(a,
		summaryCount{label: "reset", value: reset, color: a.ui.Green},
		summaryCount{label: "skipped", value: len(skips), color: a.ui.Yellow},
		summaryCount{label: "failed", value: len(failures) + len(applyFailures), color: a.ui.Red},
	)
	return status
}
