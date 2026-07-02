package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type rewriteHoursOptions struct {
	target       targetOptions
	fetch        fetchOptions
	confirmation confirmationOptions
	window       string
	timeSchedule commitTimeSchedule
	bounds       currentRewriteDateBounds
}

type hourRewriteScan struct {
	repo             repo
	err              error
	errLabel         string
	hasHead          bool
	noBranches       bool
	noCommits        bool
	branches         []dateBranchRef
	commits          []rewriteDateCommit
	selected         []int
	tzOffset         string
	hasTags          bool
	hasSignedObjects bool
}

type hourCandidate struct {
	repo             repo
	branches         []dateBranchRef
	commits          []rewriteDateCommit
	selected         []int
	tzOffset         string
	hasTags          bool
	hasSignedObjects bool
}

type hourRewritePlan struct {
	candidates        []dateCandidate
	totalSelected     int
	tzOffset          string
	schedule          commitTimeSchedule
	skippedCandidates int
}

type hourApply struct {
	candidate dateCandidate
}

type hourApplyResult struct {
	apply      hourApply
	output     string
	err        error
	restoreErr error
}

func runRewriteHours(a *app, cmd *cobra.Command, args []string) int {
	opts, ok := rewriteHoursOptionsFromCommand(a, cmd)
	if !ok {
		return 1
	}
	if !requireGit(a, "rewrite-hours") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-hours")
	if !ok {
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
	if !refreshOriginForRewriteOptions(a, opts.fetch, repos) {
		return 1
	}
	return runRewriteHoursRewrite(a, repos, filterCmd, opts)
}

func rewriteHoursOptionsFromCommand(a *app, cmd *cobra.Command) (rewriteHoursOptions, bool) {
	boundOpts, err := rewriteBoundOptionsFromCommand(cmd)
	if err != nil {
		a.error(err.Error())
		return rewriteHoursOptions{}, false
	}
	opts := rewriteHoursOptions{
		target:       targetOptionsFromCommand(cmd),
		fetch:        fetchOptionsFromCommand(cmd),
		confirmation: confirmationOptionsFromCommand(cmd),
		bounds:       boundOpts.bounds,
		window:       stringFlagValue(cmd, "window"),
	}
	if strings.TrimSpace(opts.window) == "" {
		a.error("--window is required.")
		return rewriteHoursOptions{}, false
	}
	parsed, err := parseCommitTimeSchedule(opts.window)
	if err != nil {
		a.errorf("--window %s.", err.Error())
		return rewriteHoursOptions{}, false
	}
	if parsed.empty() {
		a.error("--window must assign at least one day.")
		return rewriteHoursOptions{}, false
	}
	opts.timeSchedule = parsed
	return opts, true
}

func runRewriteHoursRewrite(a *app, repos []repo, filterCmd []string, opts rewriteHoursOptions) int {
	scans := parallelReposProgress(a.ctx, repos, newProgress(a, "Preparing hour rewrites", len(repos)), func(r repo) hourRewriteScan {
		return scanRewriteHoursRepo(a, r, opts.bounds)
	})
	if interrupted(a) {
		return 1
	}

	status := 0
	skipped := 0
	failed := 0
	candidates := []dateCandidate{}
	for _, scan := range scans {
		if scan.err != nil {
			renderErrorBlock(a, scan.repo.display+": "+scan.errLabel, scan.err.Error())
			status = 1
			failed++
			continue
		}
		if !scan.hasHead || scan.noBranches || scan.noCommits || len(scan.selected) == 0 {
			skipped++
			continue
		}
		candidates = append(candidates, dateCandidate{
			repo:             scan.repo,
			branches:         scan.branches,
			commits:          scan.commits,
			selected:         scan.selected,
			tzOffset:         scan.tzOffset,
			hasTags:          scan.hasTags,
			hasSignedObjects: scan.hasSignedObjects,
		})
	}
	if len(candidates) == 0 {
		renderSummary(a,
			summaryCount{label: "with rewrites", value: 0, color: a.ui.Yellow},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		return status
	}
	plan, err := planRewriteHourCandidates(candidates, opts.timeSchedule)
	skipped += plan.skippedCandidates
	if err != nil {
		renderErrorBlock(a, "rewrite-hours: could not plan hours", err.Error())
		renderSummary(a,
			summaryCount{label: "with rewrites", value: len(plan.candidates), color: a.ui.Yellow},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed + 1, color: a.ui.Red},
		)
		return 1
	}
	if len(plan.candidates) == 0 {
		renderSummary(a,
			summaryCount{label: "with rewrites", value: 0, color: a.ui.Yellow},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		return status
	}
	renderRewriteHourPlan(a, plan)
	renderSummary(a,
		summaryCount{label: "with rewrites", value: len(plan.candidates), color: a.ui.Yellow},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	renderWarning(a, fmt.Sprintf("This operation rewrites Git history in %d repositories. A force push will be required to update any remote. Tags may still point at old history, and commit or tag signatures may become invalid.", len(plan.candidates)))
	confirmation := confirmOrSkip(a, opts.confirmation.yes, fmt.Sprintf("Proceed with rewrite for %d repositories?", len(plan.candidates)))
	if confirmation == confirmationUnavailable || confirmation == confirmationCancelled {
		return 1
	}
	if confirmation == confirmationDeclined {
		renderSummary(a,
			summaryCount{label: "rewritten", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: skipped + len(plan.candidates), color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		return status
	}

	applies := make([]hourApply, 0, len(plan.candidates))
	for _, candidate := range plan.candidates {
		applies = append(applies, hourApply{candidate: candidate})
	}
	results := parallelItemsWithWorkersProgress(a.ctx, applies, gitMutationWorkerCount(len(applies)), newProgress(a, "Rewriting commit hours", len(applies)), func(apply hourApply) (string, string) {
		return apply.candidate.repo.display, apply.candidate.repo.display
	}, func(apply hourApply) hourApplyResult {
		out, err, restoreErr := applyRewriteHourCandidate(a, filterCmd, apply.candidate)
		return hourApplyResult{apply: apply, output: out, err: err, restoreErr: restoreErr}
	})
	if interrupted(a) {
		return 1
	}
	rewritten := 0
	applyFailed := 0
	for _, result := range results {
		r := result.apply.candidate.repo
		if result.err == nil {
			if result.restoreErr != nil {
				renderErrorBlock(a, r.display+": hour rewrite completed, but origin could not be restored", result.restoreErr.Error())
				status = 1
				applyFailed++
				continue
			}
			rewritten++
			continue
		}
		renderErrorBlock(a, r.display+": rewrite failed", outputOrError(result.output, result.err))
		if result.restoreErr != nil {
			renderErrorBlock(a, r.display+": hour rewrite failed, and origin could not be restored", result.restoreErr.Error())
		}
		status = 1
		applyFailed++
	}
	renderSummary(a,
		summaryCount{label: "rewritten", value: rewritten, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed + applyFailed, color: a.ui.Red},
	)
	return status
}

func scanRewriteHoursRepo(a *app, r repo, bounds currentRewriteDateBounds) hourRewriteScan {
	if !a.git.HasHead(a.ctx, r.dir) {
		return hourRewriteScan{repo: r}
	}
	if _, _, err := loadRewriteBaseline(r.gitDir); err != nil {
		return hourRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not read rewrite baseline"}
	}
	branches, err := localBranchRefs(a, r.dir)
	if err != nil {
		return hourRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not list local branches"}
	}
	if len(branches) == 0 {
		return hourRewriteScan{repo: r, hasHead: true, noBranches: true}
	}
	commits, err := collectRewriteDateCommits(a, r.dir, branchRefNames(branches))
	if err != nil {
		return hourRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not read commit metadata"}
	}
	if len(commits) == 0 {
		return hourRewriteScan{repo: r, hasHead: true, noCommits: true}
	}
	selected := make([]int, 0, len(commits))
	hasSigned := false
	for i := range commits {
		if commitHasSignature(commits[i]) {
			hasSigned = true
		}
		if !bounds.selectsCurrentAuthorDate(commits[i]) {
			continue
		}
		commits[i].selected = true
		commits[i].originalSHA = commits[i].hash
		commits[i].originalAuthorEpoch = commits[i].authorEpoch
		commits[i].originalAuthorTZ = commits[i].authorTZ
		commits[i].originalAuthorDate = commits[i].authorDate
		commits[i].originalCommitterEpoch = commits[i].committerEpoch
		commits[i].originalCommitterTZ = commits[i].committerTZ
		commits[i].originalCommitterDate = commits[i].committerDate
		selected = append(selected, i)
	}
	return hourRewriteScan{
		repo:             r,
		hasHead:          true,
		branches:         branches,
		commits:          commits,
		selected:         selected,
		tzOffset:         dominantCurrentTimezoneOffsetFromCommits(commits),
		hasTags:          rewriteDatesRepoHasTags(a, r.dir),
		hasSignedObjects: hasSigned,
	}
}

func planRewriteHourCandidates(candidates []dateCandidate, schedule commitTimeSchedule) (hourRewritePlan, error) {
	selected := selectedDateCommits(candidates)
	sortSelectedHourCommits(candidates, selected)
	tzOffset := dominantCurrentPlanningTimezoneOffset(candidates, selected)
	planned := cloneDateCandidates(candidates)
	for i := range planned {
		planned[i].tzOffset = tzOffset
	}
	selected = filterHourScheduleSelection(planned, selected, tzOffset, schedule)
	skippedCandidates := countHourCandidatesWithoutSelection(planned)
	if len(selected) == 0 {
		return hourRewritePlan{tzOffset: tzOffset, schedule: schedule, skippedCandidates: skippedCandidates}, nil
	}
	assignHourWindowEpochs(planned, selected, tzOffset, schedule)
	if err := verifySelectedTopologyForHours(planned); err != nil {
		return hourRewritePlan{candidates: hourCandidatesWithSelection(planned), totalSelected: len(selected), tzOffset: tzOffset, schedule: schedule, skippedCandidates: skippedCandidates}, err
	}
	if err := verifySelectedDateSchedule(planned, selected, schedule); err != nil {
		return hourRewritePlan{candidates: hourCandidatesWithSelection(planned), totalSelected: len(selected), tzOffset: tzOffset, schedule: schedule, skippedCandidates: skippedCandidates}, err
	}
	return hourRewritePlan{
		candidates:        hourCandidatesWithSelection(planned),
		totalSelected:     len(selected),
		tzOffset:          tzOffset,
		schedule:          schedule,
		skippedCandidates: skippedCandidates,
	}, nil
}

func sortSelectedHourCommits(candidates []dateCandidate, selected []selectedDateCommit) {
	order := map[string]int{}
	for _, candidate := range candidates {
		for i, commit := range candidate.commits {
			order[candidate.repo.display+"\x00"+commit.hash] = i
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		leftCandidate := candidates[selected[i].candidate]
		rightCandidate := candidates[selected[j].candidate]
		left := leftCandidate.commits[selected[i].commit]
		right := rightCandidate.commits[selected[j].commit]
		if left.authorEpoch == right.authorEpoch {
			if leftCandidate.repo.display == rightCandidate.repo.display {
				leftOrder := order[leftCandidate.repo.display+"\x00"+left.hash]
				rightOrder := order[rightCandidate.repo.display+"\x00"+right.hash]
				if leftOrder == rightOrder {
					return left.hash < right.hash
				}
				return leftOrder < rightOrder
			}
			return leftCandidate.repo.display < rightCandidate.repo.display
		}
		return left.authorEpoch < right.authorEpoch
	})
}

func filterHourScheduleSelection(candidates []dateCandidate, selected []selectedDateCommit, tzOffset string, schedule commitTimeSchedule) []selectedDateCommit {
	scheduled := make([]map[int]bool, len(candidates))
	for _, ref := range selected {
		commit := candidates[ref.candidate].commits[ref.commit]
		day := floorDayInOffset(commit.authorEpoch, tzOffset)
		if _, ok := schedule.windowForDay(day, tzOffset); !ok {
			continue
		}
		if scheduled[ref.candidate] == nil {
			scheduled[ref.candidate] = map[int]bool{}
		}
		scheduled[ref.candidate][ref.commit] = true
	}
	filteredRefs := []selectedDateCommit{}
	for candidateIndex := range candidates {
		filteredIndexes := candidates[candidateIndex].selected[:0]
		for _, commitIndex := range candidates[candidateIndex].selected {
			if scheduled[candidateIndex][commitIndex] {
				filteredIndexes = append(filteredIndexes, commitIndex)
				filteredRefs = append(filteredRefs, selectedDateCommit{candidate: candidateIndex, commit: commitIndex})
			}
		}
		candidates[candidateIndex].selected = filteredIndexes
	}
	sortSelectedHourCommits(candidates, filteredRefs)
	return filteredRefs
}

func countHourCandidatesWithoutSelection(candidates []dateCandidate) int {
	skipped := 0
	for _, candidate := range candidates {
		if len(candidate.selected) == 0 {
			skipped++
		}
	}
	return skipped
}

func hourCandidatesWithSelection(candidates []dateCandidate) []dateCandidate {
	filtered := make([]dateCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate.selected) > 0 {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func assignHourWindowEpochs(candidates []dateCandidate, selected []selectedDateCommit, tzOffset string, schedule commitTimeSchedule) {
	byDay := map[int64][]selectedDateCommit{}
	days := []int64{}
	seen := map[int64]bool{}
	for _, ref := range selected {
		commit := candidates[ref.candidate].commits[ref.commit]
		day := floorDayInOffset(commit.authorEpoch, tzOffset)
		byDay[day] = append(byDay[day], ref)
		if !seen[day] {
			seen[day] = true
			days = append(days, day)
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
	for _, day := range days {
		refs := byDay[day]
		sortSelectedHourCommits(candidates, refs)
		window, ok := schedule.windowForDay(day, tzOffset)
		if !ok {
			continue
		}
		start, end := commitTimeWindowBounds(day, window)
		timestamps := evenlyDistributedEpochs(len(refs), start, end)
		for i, ref := range refs {
			commit := &candidates[ref.candidate].commits[ref.commit]
			commit.rawPlannedEpoch = timestamps[i]
			commit.plannedEpoch = timestamps[i]
		}
	}
}

func verifySelectedTopologyForHours(candidates []dateCandidate) error {
	for _, candidate := range candidates {
		if err := verifySelectedTopology(candidate); err != nil {
			return err
		}
	}
	return nil
}

func dominantCurrentPlanningTimezoneOffset(candidates []dateCandidate, selected []selectedDateCommit) string {
	counts := map[string]int{}
	for _, ref := range selected {
		offset := candidates[ref.candidate].commits[ref.commit].authorTZ
		if timezoneOffsetRe.MatchString(offset) {
			counts[offset]++
		}
	}
	offset, _ := dominantTimezoneOffsetFromCounts(counts)
	return offset
}

func dominantCurrentTimezoneOffsetFromCommits(commits []rewriteDateCommit) string {
	counts := map[string]int{}
	for _, commit := range commits {
		if timezoneOffsetRe.MatchString(commit.authorTZ) {
			counts[commit.authorTZ]++
		}
	}
	offset, _ := dominantTimezoneOffsetFromCounts(counts)
	return offset
}

func renderRewriteHourPlan(a *app, plan hourRewritePlan) {
	fmt.Fprintln(a.stdout, "Hour Rewrite Plan")
	renderKeyValuesTo(a.stdout, []keyValueRow{
		{key: "Repositories", value: fmt.Sprintf("%d", len(plan.candidates))},
		{key: "Selected commits", value: fmt.Sprintf("%d", plan.totalSelected)},
		{key: "Time window", value: plan.schedule.Text},
		{key: "Planning timezone", value: plan.tzOffset},
	})
	fmt.Fprintln(a.stdout)
	for _, candidate := range plan.candidates {
		renderRepoHeader(a, candidate.repo.display)
		fmt.Fprintf(a.stdout, "  Selected commits: %d\n", len(candidate.selected))
		first, last := plannedRange(candidate)
		fmt.Fprintf(a.stdout, "  Planned range: %s -> %s\n", formatEpoch(first, candidate.tzOffset), formatEpoch(last, candidate.tzOffset))
		fmt.Fprintf(a.stdout, "  Timezone: %s\n", candidate.tzOffset)
		warnings := rewriteDateCandidateWarnings(candidate)
		if len(warnings) > 0 {
			fmt.Fprintf(a.stdout, "  Warnings: %s\n", strings.Join(warnings, "; "))
		}
		fmt.Fprintln(a.stdout, "  Sample:")
		for i, commitIndex := range candidate.selected {
			if i >= 3 {
				if len(candidate.selected) > 3 {
					fmt.Fprintln(a.stdout, "    ...")
				}
				break
			}
			commit := candidate.commits[commitIndex]
			fmt.Fprintf(a.stdout, "  %s  %s -> %s\n", prefix(commit.hash, 8), formatEpoch(commit.authorEpoch, commit.authorTZ), formatEpoch(commit.plannedEpoch, candidate.tzOffset))
		}
		fmt.Fprintln(a.stdout)
	}
}

func applyRewriteHourCandidate(a *app, filterCmd []string, candidate dateCandidate) (string, error, error) {
	mapping := map[string]dateCallbackDates{}
	for _, commitIndex := range candidate.selected {
		commit := candidate.commits[commitIndex]
		date := fmt.Sprintf("%d %s", commit.plannedEpoch, candidate.tzOffset)
		mapping[commit.hash] = dateCallbackDates{Author: date, Committer: date}
	}
	return applyDateCallbackCandidate(a, filterCmd, candidate, mapping)
}
