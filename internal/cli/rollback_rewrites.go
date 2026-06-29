package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type rollbackRewritesOptions struct {
	yes bool
}

type rollbackRewriteScan struct {
	repo               repo
	err                error
	errLabel           string
	hasHead            bool
	noBaseline         bool
	manifest           rewriteBaselineManifest
	branches           []rollbackRewriteBranchPlan
	knownCommits       int
	unbaselinedCommits int
	affectedBranches   int
	replayBranches     int
}

type rollbackRewriteBranchPlan struct {
	Name               string
	CurrentHead        string
	Affected           bool
	KnownCommits       int
	UnbaselinedCommits int
	NeedsReplay        bool
}

type rollbackRewriteCandidate struct {
	repo     repo
	manifest rewriteBaselineManifest
	branches []rollbackRewriteBranchPlan
}

type rollbackRewriteApply struct {
	candidate rollbackRewriteCandidate
}

type rollbackRewriteResult struct {
	apply  rollbackRewriteApply
	err    error
	output string
}

func runRollbackRewrites(a *app, cmd *cobra.Command, args []string) int {
	opts := rollbackRewritesOptions{yes: yesFlag(cmd)}
	if !requireGit(a, "rollback-rewrites") {
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

	scans := parallelReposProgress(a.ctx, repos, newProgress(a, "Preparing rewrite rollback", len(repos)), func(r repo) rollbackRewriteScan {
		return scanRollbackRewriteRepo(a, r)
	})
	if interrupted(a) {
		return 1
	}

	status := 0
	skipped := 0
	failed := 0
	candidates := []rollbackRewriteCandidate{}
	totalKnown := 0
	totalUnbaselined := 0
	totalAffectedBranches := 0
	totalReplayBranches := 0
	for _, scan := range scans {
		if scan.err != nil {
			renderErrorBlock(a, scan.repo.display+": "+scan.errLabel, scan.err.Error())
			status = 1
			failed++
			continue
		}
		if !scan.hasHead || scan.noBaseline || scan.affectedBranches == 0 {
			skipped++
			continue
		}
		candidates = append(candidates, rollbackRewriteCandidate{
			repo:     scan.repo,
			manifest: scan.manifest,
			branches: scan.branches,
		})
		totalKnown += scan.knownCommits
		totalUnbaselined += scan.unbaselinedCommits
		totalAffectedBranches += scan.affectedBranches
		totalReplayBranches += scan.replayBranches
	}
	if len(candidates) == 0 {
		renderSummary(a,
			summaryCount{label: "with rollback", value: 0, color: a.ui.Yellow},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		return status
	}
	renderRollbackRewritePlan(a, candidates, totalKnown, totalUnbaselined, totalAffectedBranches, totalReplayBranches)
	renderSummary(a,
		summaryCount{label: "with rollback", value: len(candidates), color: a.ui.Yellow},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	renderWarning(a, fmt.Sprintf("This rollback rewrites local branch history in %d repositories. Replayed commits may get new hashes or lose signatures.", len(candidates)))
	renderWarning(a, "Rollback is local-only. Propagate restored history with git-wrangler push --force.")
	confirmation := confirmOrSkip(a, opts.yes, fmt.Sprintf("Proceed with rollback for %d repositories?", len(candidates)))
	if confirmation == confirmationUnavailable || confirmation == confirmationCancelled {
		return 1
	}
	if confirmation == confirmationDeclined {
		renderSummary(a,
			summaryCount{label: "rolled back", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: skipped + len(candidates), color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		return status
	}

	applies := make([]rollbackRewriteApply, 0, len(candidates))
	for _, candidate := range candidates {
		applies = append(applies, rollbackRewriteApply{candidate: candidate})
	}
	results := parallelItemsWithWorkersProgress(a.ctx, applies, gitMutationWorkerCount(len(applies)), newProgress(a, "Rolling back rewrites", len(applies)), func(apply rollbackRewriteApply) (string, string) {
		return apply.candidate.repo.display, apply.candidate.repo.display
	}, func(apply rollbackRewriteApply) rollbackRewriteResult {
		err := applyRollbackRewriteCandidate(a, apply.candidate)
		return rollbackRewriteResult{apply: apply, err: err}
	})
	if interrupted(a) {
		return 1
	}
	rolledBack := 0
	applyFailed := 0
	for _, result := range results {
		r := result.apply.candidate.repo
		if result.err == nil {
			rolledBack++
			continue
		}
		renderErrorBlock(a, r.display+": rollback failed", result.err.Error())
		status = 1
		applyFailed++
	}
	renderSummary(a,
		summaryCount{label: "rolled back", value: rolledBack, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed + applyFailed, color: a.ui.Red},
	)
	return status
}

func scanRollbackRewriteRepo(a *app, r repo) rollbackRewriteScan {
	if !a.git.HasHead(a.ctx, r.dir) {
		return rollbackRewriteScan{repo: r}
	}
	manifest, found, err := loadRewriteBaseline(r.gitDir)
	if err != nil {
		return rollbackRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not read rewrite baseline"}
	}
	if !found || len(manifest.Entries) == 0 {
		return rollbackRewriteScan{repo: r, hasHead: true, noBaseline: true}
	}
	branches, err := localBranchRefs(a, r.dir)
	if err != nil {
		return rollbackRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not list local branches"}
	}
	currentSet := map[string]bool{}
	for _, entry := range manifest.Entries {
		currentSet[entry.CurrentSHA] = true
		currentSet[entry.FirstSHA] = true
		for _, sha := range entry.KnownSHAs {
			currentSet[sha] = true
		}
	}
	plans := []rollbackRewriteBranchPlan{}
	knownTotal := 0
	unbaselinedTotal := 0
	affected := 0
	replay := 0
	for _, branch := range branches {
		branchPlan, err := planRollbackRewriteBranch(a, r.dir, branch, currentSet)
		if err != nil {
			return rollbackRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not inspect rollback branches"}
		}
		plans = append(plans, branchPlan)
		if !branchPlan.Affected {
			continue
		}
		knownTotal += branchPlan.KnownCommits
		unbaselinedTotal += branchPlan.UnbaselinedCommits
		affected++
		if branchPlan.NeedsReplay {
			replay++
		}
	}
	if affected > 0 && !rewriteDatesWorkingTreeClean(a, r.dir) {
		return rollbackRewriteScan{repo: r, hasHead: true, err: fmt.Errorf("working tree must be clean before rollback"), errLabel: "could not prepare rollback"}
	}
	return rollbackRewriteScan{
		repo:               r,
		hasHead:            true,
		manifest:           manifest,
		branches:           plans,
		knownCommits:       knownTotal,
		unbaselinedCommits: unbaselinedTotal,
		affectedBranches:   affected,
		replayBranches:     replay,
	}
}

func planRollbackRewriteBranch(a *app, dir string, branch dateBranchRef, currentSet map[string]bool) (rollbackRewriteBranchPlan, error) {
	out, err := a.git.Stdout(a.ctx, dir, nil, "rev-list", "--topo-order", "--reverse", branch.SHA)
	if err != nil {
		return rollbackRewriteBranchPlan{}, err
	}
	plan := rollbackRewriteBranchPlan{Name: branch.Name, CurrentHead: branch.SHA}
	for _, sha := range splitLines(out) {
		if currentSet[sha] {
			plan.Affected = true
			plan.KnownCommits++
			continue
		}
		if plan.Affected {
			plan.UnbaselinedCommits++
			plan.NeedsReplay = true
		}
	}
	return plan, nil
}

func renderRollbackRewritePlan(a *app, candidates []rollbackRewriteCandidate, knownTotal, unbaselinedTotal, affectedBranches, replayBranches int) {
	fmt.Fprintln(a.stdout, "Rewrite Rollback Plan")
	renderKeyValuesTo(a.stdout, []keyValueRow{
		{key: "Repositories", value: fmt.Sprintf("%d", len(candidates))},
		{key: "Baselined commits", value: fmt.Sprintf("%d", knownTotal)},
		{key: "Unbaselined commits on affected branches", value: fmt.Sprintf("%d", unbaselinedTotal)},
		{key: "Affected branches", value: fmt.Sprintf("%d", affectedBranches)},
		{key: "Branches replaying commits", value: fmt.Sprintf("%d", replayBranches)},
	})
	fmt.Fprintln(a.stdout)
	for _, candidate := range candidates {
		renderRepoHeader(a, candidate.repo.display)
		fmt.Fprintf(a.stdout, "  Baseline entries: %d\n", len(candidate.manifest.Entries))
		fmt.Fprintln(a.stdout, "  Branches:")
		shown := 0
		for _, branch := range candidate.branches {
			if !branch.Affected {
				continue
			}
			action := "restore"
			if branch.NeedsReplay {
				action = "restore + replay"
			}
			fmt.Fprintf(a.stdout, "    %s %s (%d baselined, %d unbaselined)\n", branch.Name, action, branch.KnownCommits, branch.UnbaselinedCommits)
			shown++
			if shown >= 5 {
				break
			}
		}
		if affectedBranchCount(candidate.branches) > shown {
			fmt.Fprintln(a.stdout, "    ...")
		}
		fmt.Fprintln(a.stdout)
	}
}

func applyRollbackRewriteCandidate(a *app, candidate rollbackRewriteCandidate) error {
	if !rewriteDatesWorkingTreeClean(a, candidate.repo.dir) {
		return fmt.Errorf("working tree must be clean before rollback")
	}
	if err := importRewriteBaselineBundles(a, candidate.repo, candidate.manifest); err != nil {
		return err
	}
	byCurrent := rewriteBaselineEntriesByKnownSHA(candidate.manifest)
	byFirst := rewriteBaselineEntriesByFirst(candidate.manifest)
	rewritten := map[string]string{}
	movedRefs := map[string]bool{}
	for _, branch := range candidate.branches {
		if !branch.Affected {
			continue
		}
		newHead, err := rebuildRollbackRewriteBranch(a, candidate.repo.dir, branch, byCurrent, byFirst, rewritten)
		if err != nil {
			return fmt.Errorf("could not rebuild %s: %w", branch.Name, err)
		}
		if newHead == "" {
			return fmt.Errorf("could not rebuild %s: empty head", branch.Name)
		}
		if newHead != branch.CurrentHead {
			if _, err := a.git.Capture(a.ctx, candidate.repo.dir, nil, "update-ref", branch.Name, newHead, branch.CurrentHead); err != nil {
				return fmt.Errorf("could not move %s to %s: %w", branch.Name, prefix(newHead, 8), err)
			}
			movedRefs[branch.Name] = true
		}
	}
	if err := resetRewriteDatesCheckedOutBranch(a, candidate.repo.dir, movedRefs); err != nil {
		return fmt.Errorf("could not reset worktree after rollback: %w", err)
	}
	if err := clearManagedRewriteMetadata(a, candidate.repo.dir, candidate.repo.gitDir); err != nil {
		return err
	}
	return nil
}

func rebuildRollbackRewriteBranch(a *app, dir string, branch rollbackRewriteBranchPlan, byCurrent, byFirst map[string]rewriteBaselineEntry, rewritten map[string]string) (string, error) {
	out, err := a.git.Stdout(a.ctx, dir, nil, "rev-list", "--topo-order", "--reverse", branch.CurrentHead)
	if err != nil {
		return "", err
	}
	shas := splitLines(out)
	newHead := branch.CurrentHead
	for _, sha := range shas {
		if mapped, ok := rewritten[sha]; ok {
			newHead = mapped
			continue
		}
		if entry, ok := byCurrent[sha]; ok {
			newSHA, err := restoreBaselinedCommit(a, dir, entry, rewritten)
			if err != nil {
				return "", err
			}
			rewritten[sha] = newSHA
			rewritten[entry.FirstSHA] = newSHA
			for _, known := range entry.KnownSHAs {
				rewritten[known] = newSHA
			}
			newHead = newSHA
			continue
		}
		if entry, ok := byFirst[sha]; ok {
			newSHA, err := restoreBaselinedCommit(a, dir, entry, rewritten)
			if err != nil {
				return "", err
			}
			rewritten[sha] = newSHA
			if entry.CurrentSHA != "" {
				rewritten[entry.CurrentSHA] = newSHA
			}
			for _, known := range entry.KnownSHAs {
				rewritten[known] = newSHA
			}
			newHead = newSHA
			continue
		}
		commit, err := readRewriteBaselineCommit(a, dir, sha)
		if err != nil {
			return "", err
		}
		parents, changed := remapParents(commit.Parents, rewritten)
		if !changed {
			rewritten[sha] = sha
			newHead = sha
			continue
		}
		newSHA, err := createCommitFromBaselineData(a, dir, commit, parents)
		if err != nil {
			return "", err
		}
		rewritten[sha] = newSHA
		newHead = newSHA
	}
	return newHead, nil
}

func restoreBaselinedCommit(a *app, dir string, entry rewriteBaselineEntry, rewritten map[string]string) (string, error) {
	parents, changed := remapParents(entry.ParentSHAs, rewritten)
	if !changed {
		if _, err := a.git.Capture(a.ctx, dir, nil, "cat-file", "-e", entry.FirstSHA+"^{commit}"); err == nil {
			return entry.FirstSHA, nil
		}
	}
	return createCommitFromBaselineData(a, dir, baselineEntryCommitData(entry), parents)
}

func remapParents(parents []string, rewritten map[string]string) ([]string, bool) {
	remapped := make([]string, len(parents))
	changed := false
	for i, parent := range parents {
		next := parent
		if mapped, ok := rewritten[parent]; ok && mapped != "" {
			next = mapped
		}
		if next != parent {
			changed = true
		}
		remapped[i] = next
	}
	return remapped, changed
}

func rewriteBaselineEntriesByKnownSHA(manifest rewriteBaselineManifest) map[string]rewriteBaselineEntry {
	entries := map[string]rewriteBaselineEntry{}
	for _, entry := range manifest.Entries {
		if entry.CurrentSHA != "" {
			entries[entry.CurrentSHA] = entry
		}
		if entry.FirstSHA != "" {
			entries[entry.FirstSHA] = entry
		}
		for _, sha := range entry.KnownSHAs {
			if sha != "" {
				entries[sha] = entry
			}
		}
	}
	return entries
}

func rewriteBaselineEntriesByFirst(manifest rewriteBaselineManifest) map[string]rewriteBaselineEntry {
	entries := map[string]rewriteBaselineEntry{}
	for _, entry := range manifest.Entries {
		if entry.FirstSHA != "" {
			entries[entry.FirstSHA] = entry
		}
	}
	return entries
}

func affectedBranchCount(branches []rollbackRewriteBranchPlan) int {
	count := 0
	for _, branch := range branches {
		if branch.Affected {
			count++
		}
	}
	return count
}

func sortedRollbackBranchNames(branches []rollbackRewriteBranchPlan) []string {
	names := []string{}
	for _, branch := range branches {
		if branch.Affected {
			names = append(names, branch.Name)
		}
	}
	sort.Strings(names)
	return names
}
