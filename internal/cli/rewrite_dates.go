package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
	"github.com/kaufmann-dev/git-wrangler/internal/run"
	"github.com/spf13/cobra"
)

const (
	rewriteDatesStateRef     = "refs/git-wrangler/state/rewrite-dates"
	rewriteDatesBackupPrefix = "refs/git-wrangler/backup/rewrite-dates"
	rewriteDatesStateVersion = 1
)

var timezoneOffsetRe = regexp.MustCompile(`^[+-][0-9]{4}$`)

type rewriteDatesOptions struct {
	startDate         string
	endDate           string
	rewriteBeforeDate string
	rewriteAfterDate  string
	untilDate         string
	seed              string
	intensity         string
	days              int
	rollback          bool
	yes               bool

	hasRewriteBefore bool
	rewriteBefore    int64
	hasRewriteAfter  bool
	rewriteAfter     int64
}

type rewriteDateIntensity struct {
	name        string
	activeRatio float64
	pauseChance float64
	maxBurst    int
}

type dateBranchRef struct {
	Name string
	SHA  string
}

type rewriteDateCommit struct {
	hash                   string
	parents                []string
	authorEpoch            int64
	authorTZ               string
	authorDate             string
	committerEpoch         int64
	committerTZ            string
	committerDate          string
	signatureStatus        string
	knownInState           bool
	originalSHA            string
	originalAuthorEpoch    int64
	originalAuthorTZ       string
	originalAuthorDate     string
	originalCommitterEpoch int64
	originalCommitterTZ    string
	originalCommitterDate  string
	selected               bool
	plannedEpoch           int64
}

type rewriteDatesState struct {
	Version  int                       `json:"version"`
	Seed     string                    `json:"seed,omitempty"`
	Branches []rewriteDatesStateBranch `json:"branches,omitempty"`
	Commits  []rewriteDatesStateCommit `json:"commits"`
}

type rewriteDatesStateBranch struct {
	Name          string `json:"name"`
	OriginalHead  string `json:"original_head"`
	RewrittenHead string `json:"current_rewritten_head"`
	BackupRef     string `json:"original_backup_ref"`
	RunID         string `json:"last_run_id"`
}

type rewriteDatesStateCommit struct {
	OriginalSHA            string `json:"original_sha"`
	CurrentSHA             string `json:"current_sha"`
	OriginalAuthorDate     string `json:"original_author_date"`
	OriginalAuthorEpoch    int64  `json:"original_author_epoch"`
	OriginalAuthorTZ       string `json:"original_author_tz"`
	OriginalCommitterDate  string `json:"original_committer_date"`
	OriginalCommitterEpoch int64  `json:"original_committer_epoch"`
	OriginalCommitterTZ    string `json:"original_committer_tz"`
}

type dateRewriteScan struct {
	repo             repo
	err              error
	errLabel         string
	hasHead          bool
	noBranches       bool
	tooFew           bool
	stateFound       bool
	state            rewriteDatesState
	branches         []dateBranchRef
	commits          []rewriteDateCommit
	selected         []int
	tzOffset         string
	hasTags          bool
	hasSignedObjects bool
	rollbackPlan     dateRollbackPlan
}

type dateCandidate struct {
	repo             repo
	stateFound       bool
	state            rewriteDatesState
	branches         []dateBranchRef
	commits          []rewriteDateCommit
	selected         []int
	tzOffset         string
	hasTags          bool
	hasSignedObjects bool
	rollbackPlan     dateRollbackPlan
}

type dateRollbackAction string

const (
	dateRollbackSkip   dateRollbackAction = "skip"
	dateRollbackExact  dateRollbackAction = "exact"
	dateRollbackReplay dateRollbackAction = "replay"
)

type dateRollbackPlan struct {
	branches      []dateRollbackBranchPlan
	exact         int
	replay        int
	skipped       int
	replayCommits int
}

type dateRollbackBranchPlan struct {
	Name          string
	CurrentHead   string
	OriginalHead  string
	RewrittenHead string
	BackupRef     string
	RunID         string
	Action        dateRollbackAction
	ReplayCommits int
}

type selectedDateCommit struct {
	candidate int
	commit    int
}

type rewriteDatePlan struct {
	candidates    []dateCandidate
	seed          string
	seedSource    string
	targetStart   int64
	targetEnd     int64
	totalSelected int
	intensity     rewriteDateIntensity
	startFixed    bool
	endFixed      bool
}

type dateCallbackDates struct {
	Author    string
	Committer string
}

type dateApply struct {
	candidate dateCandidate
}

type dateApplyResult struct {
	apply      dateApply
	output     string
	err        error
	restoreErr error
}

type rewriteDatesBackupRef struct {
	Branch dateBranchRef
	Ref    string
}

func runRewriteDates(a *app, cmd *cobra.Command, args []string) int {
	opts, ok := rewriteDatesOptionsFromFlags(a, cmd)
	if !ok {
		return 1
	}
	if !requireGit(a, "rewrite-dates") {
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
	if !refreshOriginForRewrite(a, cmd, repos) {
		return 1
	}
	if opts.rollback {
		return runRewriteDatesRollback(a, repos, opts)
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-dates")
	if !ok {
		return 1
	}
	return runRewriteDatesRewrite(a, repos, filterCmd, opts)
}

func rewriteDatesOptionsFromFlags(a *app, cmd *cobra.Command) (rewriteDatesOptions, bool) {
	opts := rewriteDatesOptions{yes: yesFlag(cmd)}
	opts.startDate, _ = cmd.Flags().GetString("start-date")
	opts.endDate, _ = cmd.Flags().GetString("end-date")
	opts.rewriteBeforeDate, _ = cmd.Flags().GetString("rewrite-before")
	opts.rewriteAfterDate, _ = cmd.Flags().GetString("rewrite-after")
	opts.untilDate, _ = cmd.Flags().GetString("until")
	opts.seed, _ = cmd.Flags().GetString("seed")
	opts.intensity, _ = cmd.Flags().GetString("intensity")
	opts.days, _ = cmd.Flags().GetInt("days")
	opts.rollback, _ = cmd.Flags().GetBool("rollback")
	daysSet := cmd.Flags().Changed("days")

	for _, value := range []struct {
		name string
		date string
	}{
		{name: "start-date", date: opts.startDate},
		{name: "end-date", date: opts.endDate},
		{name: "rewrite-before", date: opts.rewriteBeforeDate},
		{name: "rewrite-after", date: opts.rewriteAfterDate},
		{name: "until", date: opts.untilDate},
	} {
		if value.date != "" && !validDate(value.date) {
			a.errorf("--%s must be in YYYY-MM-DD format.", value.name)
			return rewriteDatesOptions{}, false
		}
	}
	if daysSet && opts.days <= 0 {
		a.error("--days must be a positive integer.")
		return rewriteDatesOptions{}, false
	}
	if opts.days > 0 && (opts.startDate != "" || opts.endDate != "") {
		a.error("--days cannot be combined with --start-date or --end-date.")
		return rewriteDatesOptions{}, false
	}
	if opts.untilDate != "" && opts.days == 0 {
		a.error("--until requires --days.")
		return rewriteDatesOptions{}, false
	}
	if _, ok := rewriteDateIntensityProfile(opts.intensity); !ok {
		a.error("--intensity must be low, medium, or high.")
		return rewriteDatesOptions{}, false
	}
	if opts.rewriteBeforeDate != "" {
		opts.hasRewriteBefore = true
		opts.rewriteBefore = parseDateStart(opts.rewriteBeforeDate)
	}
	if opts.rewriteAfterDate != "" {
		opts.hasRewriteAfter = true
		opts.rewriteAfter = parseDateStart(opts.rewriteAfterDate)
	}
	if opts.hasRewriteBefore && opts.hasRewriteAfter && opts.rewriteAfter >= opts.rewriteBefore {
		a.error("--rewrite-after must be before --rewrite-before.")
		return rewriteDatesOptions{}, false
	}
	if opts.rollback {
		if opts.startDate != "" || opts.endDate != "" || opts.rewriteBeforeDate != "" || opts.rewriteAfterDate != "" || opts.days > 0 || opts.untilDate != "" || opts.seed != "" || opts.intensity != "medium" {
			a.error("--rollback cannot be combined with date planning flags.")
			return rewriteDatesOptions{}, false
		}
	}
	return opts, true
}

func runRewriteDatesRewrite(a *app, repos []repo, filterCmd []string, opts rewriteDatesOptions) int {
	scans := parallelReposProgress(a.ctx, repos, newProgress(a, "Preparing date rewrites", len(repos)), func(r repo) dateRewriteScan {
		return scanRewriteDatesRepo(a, r, opts)
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
		if !scan.hasHead || scan.noBranches || scan.tooFew || len(scan.selected) == 0 {
			skipped++
			continue
		}
		candidates = append(candidates, dateCandidate{
			repo:             scan.repo,
			stateFound:       scan.stateFound,
			state:            scan.state,
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

	plan, err := planRewriteDateCandidates(candidates, opts)
	if err != nil {
		renderErrorBlock(a, "rewrite-dates: could not plan dates", err.Error())
		renderSummary(a,
			summaryCount{label: "with rewrites", value: len(candidates), color: a.ui.Yellow},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed + 1, color: a.ui.Red},
		)
		return 1
	}
	renderRewriteDatePlan(a, plan, opts)
	renderSummary(a,
		summaryCount{label: "with rewrites", value: len(plan.candidates), color: a.ui.Yellow},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	renderWarning(a, fmt.Sprintf("This operation rewrites Git history in %d repositories. A force push will be required to update any remote. Tags may still point at old history, and commit or tag signatures may become invalid.", len(plan.candidates)))
	if !confirmOrSkip(a, opts.yes, fmt.Sprintf("Proceed with rewrite for %d repositories?", len(plan.candidates))) {
		renderSummary(a,
			summaryCount{label: "rewritten", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: skipped + len(plan.candidates), color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		return status
	}

	applies := make([]dateApply, 0, len(plan.candidates))
	for _, candidate := range plan.candidates {
		applies = append(applies, dateApply{candidate: candidate})
	}
	results := parallelItemsWithWorkersProgress(a.ctx, applies, gitMutationWorkerCount(len(applies)), newProgress(a, "Rewriting commit dates", len(applies)), func(apply dateApply) (string, string) {
		return apply.candidate.repo.display, apply.candidate.repo.display
	}, func(apply dateApply) dateApplyResult {
		out, err, restoreErr := applyRewriteDateCandidate(a, filterCmd, apply.candidate)
		return dateApplyResult{apply: apply, output: out, err: err, restoreErr: restoreErr}
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
				renderErrorBlock(a, r.display+": date rewrite completed, but origin could not be restored", result.restoreErr.Error())
				status = 1
				applyFailed++
				continue
			}
			rewritten++
			continue
		}
		renderErrorBlock(a, r.display+": rewrite failed", outputOrError(result.output, result.err))
		if result.restoreErr != nil {
			renderErrorBlock(a, r.display+": date rewrite failed, and origin could not be restored", result.restoreErr.Error())
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

func runRewriteDatesRollback(a *app, repos []repo, opts rewriteDatesOptions) int {
	scans := parallelReposProgress(a.ctx, repos, newProgress(a, "Preparing date rollback", len(repos)), func(r repo) dateRewriteScan {
		return scanRewriteDatesRepo(a, r, opts)
	})
	if interrupted(a) {
		return 1
	}

	status := 0
	skipped := 0
	failed := 0
	candidates := []dateCandidate{}
	knownTotal := 0
	unknownTotal := 0
	exactBranches := 0
	replayBranches := 0
	replayCommits := 0
	skippedBranches := 0
	for _, scan := range scans {
		if scan.err != nil {
			renderErrorBlock(a, scan.repo.display+": "+scan.errLabel, scan.err.Error())
			status = 1
			failed++
			continue
		}
		if !scan.hasHead || scan.noBranches || !scan.stateFound {
			skipped++
			continue
		}
		known := 0
		unknown := 0
		for _, commit := range scan.commits {
			if commit.knownInState {
				known++
			} else {
				unknown++
			}
		}
		if known == 0 {
			skipped++
			continue
		}
		if scan.rollbackPlan.exact == 0 && scan.rollbackPlan.replay == 0 {
			skipped++
			continue
		}
		knownTotal += known
		unknownTotal += unknown
		exactBranches += scan.rollbackPlan.exact
		replayBranches += scan.rollbackPlan.replay
		replayCommits += scan.rollbackPlan.replayCommits
		skippedBranches += scan.rollbackPlan.skipped
		candidates = append(candidates, dateCandidate{
			repo:             scan.repo,
			stateFound:       scan.stateFound,
			state:            scan.state,
			branches:         scan.branches,
			commits:          scan.commits,
			selected:         rollbackSelectedIndexes(scan.commits),
			tzOffset:         scan.tzOffset,
			hasTags:          scan.hasTags,
			hasSignedObjects: scan.hasSignedObjects,
			rollbackPlan:     scan.rollbackPlan,
		})
	}
	if len(candidates) == 0 {
		renderSummary(a,
			summaryCount{label: "with rollback", value: 0, color: a.ui.Yellow},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		return status
	}
	renderRewriteDateRollbackPlan(a, candidates, knownTotal, unknownTotal, exactBranches, replayBranches, replayCommits, skippedBranches)
	renderSummary(a,
		summaryCount{label: "with rollback", value: len(candidates), color: a.ui.Yellow},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	renderWarning(a, fmt.Sprintf("This rollback rewrites Git history in %d repositories. Exact rollback restores original commit objects and preserves signatures for known history; replayed new commits may get new hashes or lose signatures. A force push will be required to update any remote.", len(candidates)))
	if !confirmOrSkip(a, opts.yes, fmt.Sprintf("Proceed with rollback for %d repositories?", len(candidates))) {
		renderSummary(a,
			summaryCount{label: "rolled back", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: skipped + len(candidates), color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		return status
	}

	applies := make([]dateApply, 0, len(candidates))
	for _, candidate := range candidates {
		applies = append(applies, dateApply{candidate: candidate})
	}
	results := parallelItemsWithWorkersProgress(a.ctx, applies, gitMutationWorkerCount(len(applies)), newProgress(a, "Rolling back commit dates", len(applies)), func(apply dateApply) (string, string) {
		return apply.candidate.repo.display, apply.candidate.repo.display
	}, func(apply dateApply) dateApplyResult {
		out, err, restoreErr := applyRollbackDateCandidate(a, apply.candidate)
		return dateApplyResult{apply: apply, output: out, err: err, restoreErr: restoreErr}
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
				renderErrorBlock(a, r.display+": date rollback completed, but origin could not be restored", result.restoreErr.Error())
				status = 1
				applyFailed++
				continue
			}
			rewritten++
			continue
		}
		renderErrorBlock(a, r.display+": rollback failed", outputOrError(result.output, result.err))
		if result.restoreErr != nil {
			renderErrorBlock(a, r.display+": date rollback failed, and origin could not be restored", result.restoreErr.Error())
		}
		status = 1
		applyFailed++
	}
	renderSummary(a,
		summaryCount{label: "rolled back", value: rewritten, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed + applyFailed, color: a.ui.Red},
	)
	return status
}

func scanRewriteDatesRepo(a *app, r repo, opts rewriteDatesOptions) dateRewriteScan {
	if !a.git.HasHead(a.ctx, r.dir) {
		return dateRewriteScan{repo: r}
	}
	branches, err := localBranchRefs(a, r.dir)
	if err != nil {
		return dateRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not list local branches"}
	}
	if len(branches) == 0 {
		return dateRewriteScan{repo: r, hasHead: true, noBranches: true}
	}
	state, stateFound, err := readRewriteDatesState(a, r.dir)
	if err != nil {
		return dateRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not read rewrite state"}
	}
	if !opts.rollback && stateFound {
		if err := validateRewriteDatesBranchBaselines(a, r.dir, state, branches); err != nil {
			return dateRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not validate rewrite state"}
		}
	}
	commits, err := collectRewriteDateCommits(a, r.dir, branchRefNames(branches))
	if err != nil {
		return dateRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not read commit metadata"}
	}
	if !opts.rollback && len(commits) < 2 {
		return dateRewriteScan{repo: r, hasHead: true, tooFew: true}
	}
	applyRewriteDatesStateToCommits(state, commits)
	state = mergeRewriteDatesState(state, commits)
	selected := []int{}
	hasSigned := false
	for i := range commits {
		if commitHasSignature(commits[i]) {
			hasSigned = true
		}
		if opts.rollback {
			continue
		}
		if rewriteDateCommitSelected(commits[i], opts) {
			commits[i].selected = true
			selected = append(selected, i)
		}
	}
	rollbackPlan := dateRollbackPlan{}
	if opts.rollback && stateFound {
		rollbackPlan, err = planRewriteDatesRollback(a, r, state, branches, commits)
		if err != nil {
			return dateRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not plan date rollback"}
		}
	}
	return dateRewriteScan{
		repo:             r,
		hasHead:          true,
		stateFound:       stateFound,
		state:            state,
		branches:         branches,
		commits:          commits,
		selected:         selected,
		tzOffset:         dominantTimezoneOffsetFromCommits(commits),
		hasTags:          rewriteDatesRepoHasTags(a, r.dir),
		hasSignedObjects: hasSigned,
		rollbackPlan:     rollbackPlan,
	}
}

func localBranchRefs(a *app, dir string) ([]dateBranchRef, error) {
	out, err := a.git.Stdout(a.ctx, dir, nil, "for-each-ref", "--format=%(refname)%00%(objectname)", "refs/heads")
	if err != nil {
		return nil, err
	}
	branches := []dateBranchRef{}
	for _, line := range splitLines(out) {
		fields := strings.Split(line, "\x00")
		if len(fields) != 2 {
			return nil, fmt.Errorf("malformed local branch line %q", line)
		}
		name := strings.TrimSpace(fields[0])
		sha := strings.TrimSpace(fields[1])
		if name == "" || sha == "" || strings.HasPrefix(name, "refs/git-wrangler/") {
			continue
		}
		branches = append(branches, dateBranchRef{Name: name, SHA: sha})
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].Name < branches[j].Name })
	return branches, nil
}

func branchRefNames(branches []dateBranchRef) []string {
	refs := make([]string, 0, len(branches))
	for _, branch := range branches {
		refs = append(refs, branch.Name)
	}
	return refs
}

func planRewriteDatesRollback(a *app, r repo, state rewriteDatesState, branches []dateBranchRef, commits []rewriteDateCommit) (dateRollbackPlan, error) {
	if len(state.Branches) == 0 {
		return dateRollbackPlan{}, fmt.Errorf("rewrite state is missing branch rollback metadata")
	}
	byName := rewriteDatesBranchStateByName(state.Branches)
	plan := dateRollbackPlan{}
	for _, branch := range branches {
		meta, ok := byName[branch.Name]
		if !ok {
			containsKnown, err := branchContainsKnownRewrittenCommit(a, r.dir, branch.SHA, state)
			if err != nil {
				return dateRollbackPlan{}, err
			}
			if containsKnown {
				return dateRollbackPlan{}, fmt.Errorf("%s contains rewritten commits but has no matching branch rollback metadata", branch.Name)
			}
			plan.branches = append(plan.branches, dateRollbackBranchPlan{Name: branch.Name, CurrentHead: branch.SHA, Action: dateRollbackSkip})
			plan.skipped++
			continue
		}
		branchPlan, err := classifyRewriteDatesRollbackBranch(a, r.dir, branch, meta)
		if err != nil {
			return dateRollbackPlan{}, err
		}
		plan.branches = append(plan.branches, branchPlan)
		switch branchPlan.Action {
		case dateRollbackExact:
			plan.exact++
		case dateRollbackReplay:
			plan.replay++
			plan.replayCommits += branchPlan.ReplayCommits
		default:
			plan.skipped++
		}
	}
	if (plan.exact > 0 || plan.replay > 0) && !rewriteDatesWorkingTreeClean(a, r.dir) {
		return dateRollbackPlan{}, fmt.Errorf("working tree must be clean before exact rollback")
	}
	return plan, nil
}

func rewriteDatesBranchStateByName(branches []rewriteDatesStateBranch) map[string]rewriteDatesStateBranch {
	byName := map[string]rewriteDatesStateBranch{}
	for _, branch := range branches {
		if branch.Name == "" {
			continue
		}
		byName[branch.Name] = branch
	}
	return byName
}

func classifyRewriteDatesRollbackBranch(a *app, dir string, branch dateBranchRef, meta rewriteDatesStateBranch) (dateRollbackBranchPlan, error) {
	plan := dateRollbackBranchPlan{
		Name:          branch.Name,
		CurrentHead:   branch.SHA,
		OriginalHead:  meta.OriginalHead,
		RewrittenHead: meta.RewrittenHead,
		BackupRef:     meta.BackupRef,
		RunID:         meta.RunID,
		Action:        dateRollbackSkip,
	}
	if meta.OriginalHead == "" || meta.RewrittenHead == "" || meta.BackupRef == "" {
		return dateRollbackBranchPlan{}, fmt.Errorf("%s has incomplete branch rollback metadata", branch.Name)
	}
	backupHead, err := resolveRewriteDatesCommit(a, dir, meta.BackupRef)
	if err != nil {
		return dateRollbackBranchPlan{}, fmt.Errorf("%s backup ref %s is unavailable: %w", branch.Name, meta.BackupRef, err)
	}
	if backupHead != meta.OriginalHead {
		return dateRollbackBranchPlan{}, fmt.Errorf("%s backup ref %s points to %s, expected %s", branch.Name, meta.BackupRef, prefix(backupHead, 8), prefix(meta.OriginalHead, 8))
	}
	if branch.SHA == meta.OriginalHead {
		return plan, nil
	}
	alreadyRestored, err := gitIsAncestor(a, dir, meta.OriginalHead, branch.SHA)
	if err != nil {
		return dateRollbackBranchPlan{}, err
	}
	if alreadyRestored {
		return plan, nil
	}
	if branch.SHA == meta.RewrittenHead {
		plan.Action = dateRollbackExact
		return plan, nil
	}
	needsReplay, err := gitIsAncestor(a, dir, meta.RewrittenHead, branch.SHA)
	if err != nil {
		return dateRollbackBranchPlan{}, err
	}
	if needsReplay {
		count, err := rewriteDatesReplayCommitCount(a, dir, branch.SHA, meta.RewrittenHead)
		if err != nil {
			return dateRollbackBranchPlan{}, err
		}
		if count == 0 {
			plan.Action = dateRollbackExact
			return plan, nil
		}
		plan.Action = dateRollbackReplay
		plan.ReplayCommits = count
		return plan, nil
	}
	return dateRollbackBranchPlan{}, fmt.Errorf("%s is not at the stored original head, stored rewritten head, or a descendant of either; rollback cannot map ancestry safely", branch.Name)
}

func branchContainsKnownRewrittenCommit(a *app, dir, branchHead string, state rewriteDatesState) (bool, error) {
	for _, commit := range state.Commits {
		if commit.CurrentSHA == "" || commit.OriginalSHA == "" || commit.CurrentSHA == commit.OriginalSHA {
			continue
		}
		contains, err := gitIsAncestor(a, dir, commit.CurrentSHA, branchHead)
		if err != nil {
			return false, err
		}
		if contains {
			return true, nil
		}
	}
	return false, nil
}

func validateRewriteDatesBranchBaselines(a *app, dir string, state rewriteDatesState, branches []dateBranchRef) error {
	byName := rewriteDatesBranchStateByName(state.Branches)
	for _, branch := range branches {
		meta, ok := byName[branch.Name]
		if !ok {
			containsKnown, err := branchContainsKnownRewrittenCommit(a, dir, branch.SHA, state)
			if err != nil {
				return err
			}
			if containsKnown {
				return fmt.Errorf("%s contains rewritten commits but has no matching branch rollback metadata", branch.Name)
			}
			continue
		}
		if err := validateRewriteDatesBranchBaseline(a, dir, branch, meta); err != nil {
			return err
		}
	}
	return nil
}

func validateRewriteDatesBranchBaseline(a *app, dir string, branch dateBranchRef, meta rewriteDatesStateBranch) error {
	if meta.OriginalHead == "" || meta.RewrittenHead == "" || meta.BackupRef == "" {
		return fmt.Errorf("%s has incomplete branch rollback metadata", branch.Name)
	}
	backupHead, err := resolveRewriteDatesCommit(a, dir, meta.BackupRef)
	if err != nil {
		return fmt.Errorf("%s backup ref %s is unavailable: %w", branch.Name, meta.BackupRef, err)
	}
	if backupHead != meta.OriginalHead {
		return fmt.Errorf("%s backup ref %s points to %s, expected %s", branch.Name, meta.BackupRef, prefix(backupHead, 8), prefix(meta.OriginalHead, 8))
	}
	if branch.SHA == meta.OriginalHead || branch.SHA == meta.RewrittenHead {
		return nil
	}
	originalAncestor, err := gitIsAncestor(a, dir, meta.OriginalHead, branch.SHA)
	if err != nil {
		return err
	}
	if originalAncestor {
		return nil
	}
	rewrittenAncestor, err := gitIsAncestor(a, dir, meta.RewrittenHead, branch.SHA)
	if err != nil {
		return err
	}
	if rewrittenAncestor {
		return nil
	}
	return fmt.Errorf("%s is not at or descended from the stored original or rewritten baseline; rewrite cannot update rollback metadata safely", branch.Name)
}

func resolveRewriteDatesCommit(a *app, dir, rev string) (string, error) {
	out, err := a.git.Stdout(a.ctx, dir, nil, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(firstLine(out))
	if sha == "" {
		return "", fmt.Errorf("empty object id")
	}
	return sha, nil
}

func gitIsAncestor(a *app, dir, ancestor, descendant string) (bool, error) {
	out, err := a.git.Capture(a.ctx, dir, nil, "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}
	if strings.TrimSpace(out) == "" {
		return false, nil
	}
	return false, fmt.Errorf("could not compare ancestry between %s and %s: %s", prefix(ancestor, 8), prefix(descendant, 8), strings.TrimSpace(out))
}

func rewriteDatesReplayCommitCount(a *app, dir, head, base string) (int, error) {
	out, err := a.git.Stdout(a.ctx, dir, nil, "rev-list", "--count", head, "--not", base)
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(firstLine(out)))
	if err != nil {
		return 0, fmt.Errorf("malformed replay commit count %q", strings.TrimSpace(out))
	}
	return count, nil
}

func rewriteDatesWorkingTreeClean(a *app, dir string) bool {
	out, err := a.git.StatusPorcelain(a.ctx, dir)
	return err == nil && strings.TrimSpace(out) == ""
}

func collectRewriteDateCommits(a *app, dir string, refs []string) ([]rewriteDateCommit, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	format := "%H%x00%P%x00%at%x00%ai%x00%ct%x00%ci%x00%G?%x1e"
	args := []string{"log", "--topo-order", "--reverse", "--format=" + format}
	args = append(args, refs...)
	args = append(args, "--")
	out, err := a.git.Stdout(a.ctx, dir, nil, args...)
	if err != nil {
		return nil, err
	}
	commits := []rewriteDateCommit{}
	seen := map[string]bool{}
	for _, record := range strings.Split(out, "\x1e") {
		record = strings.Trim(record, "\r\n")
		if record == "" {
			continue
		}
		fields := strings.Split(record, "\x00")
		if len(fields) != 7 {
			return nil, fmt.Errorf("malformed commit metadata record for %q", firstLine(record))
		}
		hash := strings.TrimSpace(fields[0])
		if hash == "" || seen[hash] {
			continue
		}
		seen[hash] = true
		authorEpoch, err := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("malformed author timestamp for %s", hash)
		}
		committerEpoch, err := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("malformed committer timestamp for %s", hash)
		}
		authorTZ := timezoneOffsetFromGitDate(fields[3])
		committerTZ := timezoneOffsetFromGitDate(fields[5])
		commits = append(commits, rewriteDateCommit{
			hash:            hash,
			parents:         strings.Fields(fields[1]),
			authorEpoch:     authorEpoch,
			authorTZ:        authorTZ,
			authorDate:      fmt.Sprintf("%d %s", authorEpoch, authorTZ),
			committerEpoch:  committerEpoch,
			committerTZ:     committerTZ,
			committerDate:   fmt.Sprintf("%d %s", committerEpoch, committerTZ),
			signatureStatus: strings.TrimSpace(fields[6]),
		})
	}
	return commits, nil
}

func timezoneOffsetFromGitDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 5 {
		offset := value[len(value)-5:]
		if timezoneOffsetRe.MatchString(offset) {
			return offset
		}
	}
	return time.Now().Format("-0700")
}

func commitHasSignature(commit rewriteDateCommit) bool {
	status := strings.TrimSpace(commit.signatureStatus)
	return status != "" && status != "N"
}

func rewriteDatesRepoHasTags(a *app, dir string) bool {
	out, err := a.git.Stdout(a.ctx, dir, nil, "for-each-ref", "--format=%(refname)", "refs/tags")
	return err == nil && strings.TrimSpace(out) != ""
}

func readRewriteDatesState(a *app, dir string) (rewriteDatesState, bool, error) {
	out, err := a.git.Stdout(a.ctx, dir, nil, "rev-parse", "--verify", "--quiet", rewriteDatesStateRef)
	if err != nil || strings.TrimSpace(out) == "" {
		return normalizeRewriteDatesState(rewriteDatesState{}), false, nil
	}
	blob := strings.TrimSpace(firstLine(out))
	data, err := a.git.Stdout(a.ctx, dir, nil, "cat-file", "-p", blob)
	if err != nil {
		return rewriteDatesState{}, true, err
	}
	var state rewriteDatesState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		return rewriteDatesState{}, true, err
	}
	if state.Version != rewriteDatesStateVersion {
		return rewriteDatesState{}, true, fmt.Errorf("unsupported rewrite-dates state version %d; remove %s before running rewrite-dates again", state.Version, rewriteDatesStateRef)
	}
	return normalizeRewriteDatesState(state), true, nil
}

func writeRewriteDatesState(a *app, dir string, state rewriteDatesState) error {
	state = normalizeRewriteDatesState(state)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	ctx := run.WithStdin(a.ctx, string(data)+"\n")
	out, err := run.Stdout(ctx, a.runner, dir, nil, "git", "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	blob := strings.TrimSpace(firstLine(out))
	if blob == "" {
		return fmt.Errorf("git hash-object returned an empty object id")
	}
	_, err = a.git.Capture(a.ctx, dir, nil, "update-ref", rewriteDatesStateRef, blob)
	return err
}

func normalizeRewriteDatesState(state rewriteDatesState) rewriteDatesState {
	state.Version = rewriteDatesStateVersion
	sort.Slice(state.Branches, func(i, j int) bool {
		if state.Branches[i].Name == state.Branches[j].Name {
			return state.Branches[i].RunID < state.Branches[j].RunID
		}
		return state.Branches[i].Name < state.Branches[j].Name
	})
	sort.Slice(state.Commits, func(i, j int) bool {
		if state.Commits[i].OriginalSHA == state.Commits[j].OriginalSHA {
			return state.Commits[i].CurrentSHA < state.Commits[j].CurrentSHA
		}
		return state.Commits[i].OriginalSHA < state.Commits[j].OriginalSHA
	})
	return state
}

func applyRewriteDatesStateToCommits(state rewriteDatesState, commits []rewriteDateCommit) {
	byCurrent := map[string]rewriteDatesStateCommit{}
	byOriginal := map[string]rewriteDatesStateCommit{}
	for _, entry := range state.Commits {
		if entry.CurrentSHA != "" {
			byCurrent[entry.CurrentSHA] = entry
		}
		if entry.OriginalSHA != "" {
			byOriginal[entry.OriginalSHA] = entry
		}
	}
	for i := range commits {
		entry, ok := byCurrent[commits[i].hash]
		if !ok {
			entry, ok = byOriginal[commits[i].hash]
		}
		if !ok {
			commits[i].knownInState = false
			commits[i].originalSHA = commits[i].hash
			commits[i].originalAuthorEpoch = commits[i].authorEpoch
			commits[i].originalAuthorTZ = commits[i].authorTZ
			commits[i].originalAuthorDate = commits[i].authorDate
			commits[i].originalCommitterEpoch = commits[i].committerEpoch
			commits[i].originalCommitterTZ = commits[i].committerTZ
			commits[i].originalCommitterDate = commits[i].committerDate
			continue
		}
		commits[i].knownInState = true
		commits[i].originalSHA = entry.OriginalSHA
		commits[i].originalAuthorEpoch = entry.OriginalAuthorEpoch
		commits[i].originalAuthorTZ = entry.OriginalAuthorTZ
		commits[i].originalAuthorDate = entry.OriginalAuthorDate
		commits[i].originalCommitterEpoch = entry.OriginalCommitterEpoch
		commits[i].originalCommitterTZ = entry.OriginalCommitterTZ
		commits[i].originalCommitterDate = entry.OriginalCommitterDate
	}
}

func mergeRewriteDatesState(state rewriteDatesState, commits []rewriteDateCommit) rewriteDatesState {
	state = normalizeRewriteDatesState(state)
	byCurrent := map[string]int{}
	byOriginal := map[string]int{}
	for i, entry := range state.Commits {
		if entry.CurrentSHA != "" {
			byCurrent[entry.CurrentSHA] = i
		}
		if entry.OriginalSHA != "" {
			byOriginal[entry.OriginalSHA] = i
		}
	}
	for _, commit := range commits {
		if _, ok := byCurrent[commit.hash]; ok {
			continue
		}
		if idx, ok := byOriginal[commit.originalSHA]; ok {
			state.Commits[idx].CurrentSHA = commit.hash
			byCurrent[commit.hash] = idx
			continue
		}
		entry := rewriteDatesStateCommit{
			OriginalSHA:            commit.originalSHA,
			CurrentSHA:             commit.hash,
			OriginalAuthorDate:     commit.originalAuthorDate,
			OriginalAuthorEpoch:    commit.originalAuthorEpoch,
			OriginalAuthorTZ:       commit.originalAuthorTZ,
			OriginalCommitterDate:  commit.originalCommitterDate,
			OriginalCommitterEpoch: commit.originalCommitterEpoch,
			OriginalCommitterTZ:    commit.originalCommitterTZ,
		}
		state.Commits = append(state.Commits, entry)
		byCurrent[entry.CurrentSHA] = len(state.Commits) - 1
		byOriginal[entry.OriginalSHA] = len(state.Commits) - 1
	}
	return normalizeRewriteDatesState(state)
}

func rewriteDateCommitSelected(commit rewriteDateCommit, opts rewriteDatesOptions) bool {
	epoch := commit.originalAuthorEpoch
	if opts.hasRewriteAfter && epoch < opts.rewriteAfter {
		return false
	}
	if opts.hasRewriteBefore && epoch >= opts.rewriteBefore {
		return false
	}
	return true
}

func planRewriteDateCandidates(candidates []dateCandidate, opts rewriteDatesOptions) (rewriteDatePlan, error) {
	intensity, _ := rewriteDateIntensityProfile(opts.intensity)
	seed, seedSource := rewriteDateSeed(opts, candidates)
	selected := selectedDateCommits(candidates)
	targetStart, targetEnd, startFixed, endFixed, err := rewriteDateTargetRange(candidates, selected, opts)
	if err != nil {
		return rewriteDatePlan{}, err
	}
	targetStart, targetEnd, err = extendRewriteDateTargetForConstraints(candidates, selected, targetStart, targetEnd, startFixed, endFixed)
	if err != nil {
		return rewriteDatePlan{}, err
	}
	targetStart, targetEnd, err = extendRewriteDateTargetForChains(candidates, targetStart, targetEnd, startFixed, endFixed)
	if err != nil {
		return rewriteDatePlan{}, err
	}
	timestamps := generatePlannedEpochs(len(selected), targetStart, targetEnd, seed, intensity)
	sort.Slice(selected, func(i, j int) bool {
		left := candidates[selected[i].candidate].commits[selected[i].commit]
		right := candidates[selected[j].candidate].commits[selected[j].commit]
		if left.originalAuthorEpoch == right.originalAuthorEpoch {
			if candidates[selected[i].candidate].repo.display == candidates[selected[j].candidate].repo.display {
				return left.hash < right.hash
			}
			return candidates[selected[i].candidate].repo.display < candidates[selected[j].candidate].repo.display
		}
		return left.originalAuthorEpoch < right.originalAuthorEpoch
	})
	for i, ref := range selected {
		candidates[ref.candidate].commits[ref.commit].plannedEpoch = timestamps[i]
	}
	if err := enforceRewriteDateTopology(candidates, targetStart, targetEnd); err != nil {
		return rewriteDatePlan{}, err
	}
	for i := range candidates {
		candidates[i].state.Seed = seed
	}
	return rewriteDatePlan{
		candidates:    candidates,
		seed:          seed,
		seedSource:    seedSource,
		targetStart:   targetStart,
		targetEnd:     targetEnd,
		totalSelected: len(selected),
		intensity:     intensity,
		startFixed:    startFixed,
		endFixed:      endFixed,
	}, nil
}

func rewriteDateIntensityProfile(name string) (rewriteDateIntensity, bool) {
	switch name {
	case "", "medium":
		return rewriteDateIntensity{name: "medium", activeRatio: 0.65, pauseChance: 0.20, maxBurst: 4}, true
	case "low":
		return rewriteDateIntensity{name: "low", activeRatio: 0.45, pauseChance: 0.35, maxBurst: 2}, true
	case "high":
		return rewriteDateIntensity{name: "high", activeRatio: 0.85, pauseChance: 0.08, maxBurst: 8}, true
	default:
		return rewriteDateIntensity{}, false
	}
}

func rewriteDateSeed(opts rewriteDatesOptions, candidates []dateCandidate) (string, string) {
	if opts.seed != "" {
		return opts.seed, "flag"
	}
	for _, candidate := range candidates {
		if candidate.state.Seed != "" {
			return candidate.state.Seed, "state"
		}
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	return fmt.Sprintf("%x", sum[:8]), "generated"
}

func selectedDateCommits(candidates []dateCandidate) []selectedDateCommit {
	selected := []selectedDateCommit{}
	for candidateIndex, candidate := range candidates {
		for _, commitIndex := range candidate.selected {
			selected = append(selected, selectedDateCommit{candidate: candidateIndex, commit: commitIndex})
		}
	}
	return selected
}

func rewriteDateTargetRange(candidates []dateCandidate, selected []selectedDateCommit, opts rewriteDatesOptions) (int64, int64, bool, bool, error) {
	if len(selected) == 0 {
		return 0, 0, false, false, fmt.Errorf("no commits selected")
	}
	startFixed := opts.startDate != "" || opts.days > 0
	endFixed := opts.endDate != "" || opts.days > 0
	if opts.days > 0 {
		until := opts.untilDate
		if until == "" {
			until = time.Now().In(time.Local).Format("2006-01-02")
		}
		end := parseDateEnd(until)
		start := parseDateStart(time.Unix(end, 0).In(time.Local).AddDate(0, 0, -(opts.days - 1)).Format("2006-01-02"))
		return start, end, true, true, nil
	}
	minOriginal := int64(0)
	maxOriginal := int64(0)
	for i, ref := range selected {
		epoch := candidates[ref.candidate].commits[ref.commit].originalAuthorEpoch
		if i == 0 || epoch < minOriginal {
			minOriginal = epoch
		}
		if i == 0 || epoch > maxOriginal {
			maxOriginal = epoch
		}
	}
	start := minOriginal
	end := maxOriginal
	if opts.startDate != "" {
		start = parseDateStart(opts.startDate)
	}
	if opts.endDate != "" {
		end = parseDateEnd(opts.endDate)
	}
	if start > end {
		return 0, 0, startFixed, endFixed, fmt.Errorf("target start date must be on or before target end date")
	}
	return start, end, startFixed, endFixed, nil
}

func extendRewriteDateTargetForConstraints(candidates []dateCandidate, selected []selectedDateCommit, targetStart, targetEnd int64, startFixed, endFixed bool) (int64, int64, error) {
	for {
		changed := false
		for _, ref := range selected {
			candidate := candidates[ref.candidate]
			minFixed, maxFixed := fixedDateConstraints(candidate, ref.commit)
			commit := candidate.commits[ref.commit]
			if minFixed > maxFixed {
				return 0, 0, fmt.Errorf("%s %s has incompatible fixed parent/child dates", candidate.repo.display, prefix(commit.hash, 8))
			}
			if minFixed > targetEnd {
				if endFixed {
					return 0, 0, fmt.Errorf("%s %s needs a target date after fixed parent %s", candidate.repo.display, prefix(commit.hash, 8), formatEpochLocal(minFixed-1))
				}
				targetEnd = minFixed
				changed = true
			}
			if maxFixed < targetStart {
				if startFixed {
					return 0, 0, fmt.Errorf("%s %s needs a target date before fixed child %s", candidate.repo.display, prefix(commit.hash, 8), formatEpochLocal(maxFixed+1))
				}
				targetStart = maxFixed
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	if targetStart > targetEnd {
		return 0, 0, fmt.Errorf("target range is empty after applying fixed commit boundaries")
	}
	return targetStart, targetEnd, nil
}

func extendRewriteDateTargetForChains(candidates []dateCandidate, targetStart, targetEnd int64, startFixed, endFixed bool) (int64, int64, error) {
	needed := int64(maxSelectedChainLength(candidates) - 1)
	if needed < 0 {
		needed = 0
	}
	if targetEnd-targetStart >= needed {
		return targetStart, targetEnd, nil
	}
	if !endFixed {
		return targetStart, targetStart + needed, nil
	}
	if !startFixed {
		return targetEnd - needed, targetEnd, nil
	}
	return 0, 0, fmt.Errorf("target range %s -> %s is too narrow for selected parent/child ordering", formatEpochLocal(targetStart), formatEpochLocal(targetEnd))
}

func fixedDateConstraints(candidate dateCandidate, commitIndex int) (int64, int64) {
	minFixed := int64(math.MinInt64 / 2)
	maxFixed := int64(math.MaxInt64 / 2)
	byHash := commitIndexByHash(candidate.commits)
	selected := selectedIndexSet(candidate.selected)
	for _, parent := range candidate.commits[commitIndex].parents {
		parentIndex, ok := byHash[parent]
		if !ok || selected[parentIndex] {
			continue
		}
		minFixed = maxInt64(minFixed, candidate.commits[parentIndex].originalAuthorEpoch+1)
	}
	children := childCommitIndexes(candidate.commits)
	for _, childIndex := range children[commitIndex] {
		if selected[childIndex] {
			continue
		}
		maxFixed = minInt64(maxFixed, candidate.commits[childIndex].originalAuthorEpoch-1)
	}
	return minFixed, maxFixed
}

func enforceRewriteDateTopology(candidates []dateCandidate, targetStart, targetEnd int64) error {
	for candidateIndex := range candidates {
		candidate := &candidates[candidateIndex]
		selected := selectedIndexSet(candidate.selected)
		byHash := commitIndexByHash(candidate.commits)
		children := childCommitIndexes(candidate.commits)
		mins := make(map[int]int64, len(candidate.selected))
		maxes := make(map[int]int64, len(candidate.selected))
		for _, idx := range candidate.selected {
			minFixed, maxFixed := fixedDateConstraints(*candidate, idx)
			mins[idx] = maxInt64(targetStart, minFixed)
			maxes[idx] = minInt64(targetEnd, maxFixed)
			if mins[idx] > maxes[idx] {
				return fmt.Errorf("%s %s cannot fit between fixed neighboring commits", candidate.repo.display, prefix(candidate.commits[idx].hash, 8))
			}
			if candidate.commits[idx].plannedEpoch < mins[idx] {
				candidate.commits[idx].plannedEpoch = mins[idx]
			}
			if candidate.commits[idx].plannedEpoch > maxes[idx] {
				candidate.commits[idx].plannedEpoch = maxes[idx]
			}
		}
		limit := len(candidate.commits) + 1
		for pass := 0; pass < limit; pass++ {
			changed := false
			for i := range candidate.commits {
				if !selected[i] {
					continue
				}
				minAllowed := mins[i]
				for _, parent := range candidate.commits[i].parents {
					if parentIndex, ok := byHash[parent]; ok && selected[parentIndex] {
						minAllowed = maxInt64(minAllowed, candidate.commits[parentIndex].plannedEpoch+1)
					}
				}
				if candidate.commits[i].plannedEpoch < minAllowed {
					candidate.commits[i].plannedEpoch = minAllowed
					changed = true
				}
				if candidate.commits[i].plannedEpoch > maxes[i] {
					return fmt.Errorf("%s %s needs to be after its selected parent but outside the target range", candidate.repo.display, prefix(candidate.commits[i].hash, 8))
				}
			}
			for i := len(candidate.commits) - 1; i >= 0; i-- {
				if !selected[i] {
					continue
				}
				maxAllowed := maxes[i]
				for _, childIndex := range children[i] {
					if selected[childIndex] {
						maxAllowed = minInt64(maxAllowed, candidate.commits[childIndex].plannedEpoch-1)
					}
				}
				if candidate.commits[i].plannedEpoch > maxAllowed {
					candidate.commits[i].plannedEpoch = maxAllowed
					changed = true
				}
				if candidate.commits[i].plannedEpoch < mins[i] {
					return fmt.Errorf("%s %s needs to be before its selected child but outside the target range", candidate.repo.display, prefix(candidate.commits[i].hash, 8))
				}
			}
			if !changed {
				break
			}
		}
		if err := verifySelectedTopology(*candidate); err != nil {
			return err
		}
	}
	return nil
}

func verifySelectedTopology(candidate dateCandidate) error {
	selected := selectedIndexSet(candidate.selected)
	byHash := commitIndexByHash(candidate.commits)
	children := childCommitIndexes(candidate.commits)
	for _, idx := range candidate.selected {
		commit := candidate.commits[idx]
		for _, parent := range commit.parents {
			if parentIndex, ok := byHash[parent]; ok && selected[parentIndex] && candidate.commits[parentIndex].plannedEpoch+1 > commit.plannedEpoch {
				return fmt.Errorf("%s %s is not at least one second after selected parent %s", candidate.repo.display, prefix(commit.hash, 8), prefix(parent, 8))
			}
		}
		for _, childIndex := range children[idx] {
			if selected[childIndex] && commit.plannedEpoch+1 > candidate.commits[childIndex].plannedEpoch {
				return fmt.Errorf("%s %s is not at least one second before selected child %s", candidate.repo.display, prefix(commit.hash, 8), prefix(candidate.commits[childIndex].hash, 8))
			}
		}
	}
	return nil
}

func selectedIndexSet(indexes []int) map[int]bool {
	set := map[int]bool{}
	for _, idx := range indexes {
		set[idx] = true
	}
	return set
}

func commitIndexByHash(commits []rewriteDateCommit) map[string]int {
	byHash := map[string]int{}
	for i, commit := range commits {
		byHash[commit.hash] = i
	}
	return byHash
}

func childCommitIndexes(commits []rewriteDateCommit) map[int][]int {
	byHash := commitIndexByHash(commits)
	children := map[int][]int{}
	for i, commit := range commits {
		for _, parent := range commit.parents {
			if parentIndex, ok := byHash[parent]; ok {
				children[parentIndex] = append(children[parentIndex], i)
			}
		}
	}
	return children
}

func maxSelectedChainLength(candidates []dateCandidate) int {
	maxChain := 0
	for _, candidate := range candidates {
		selected := selectedIndexSet(candidate.selected)
		byHash := commitIndexByHash(candidate.commits)
		lengths := map[int]int{}
		for i, commit := range candidate.commits {
			if !selected[i] {
				continue
			}
			length := 1
			for _, parent := range commit.parents {
				if parentIndex, ok := byHash[parent]; ok && selected[parentIndex] {
					length = maxInt(length, lengths[parentIndex]+1)
				}
			}
			lengths[i] = length
			maxChain = maxInt(maxChain, length)
		}
	}
	return maxChain
}

func generatePlannedEpochs(n int, startEpoch, endEpoch int64, seed string, intensity rewriteDateIntensity) []int64 {
	if n <= 0 {
		return nil
	}
	if startEpoch > endEpoch {
		startEpoch, endEpoch = endEpoch, startEpoch
	}
	rng := rand.New(rand.NewSource(seedInt64(seed)))
	days := daysInRange(startEpoch, endEpoch)
	activeDays := activePlanningDays(days, intensity, rng)
	if len(activeDays) == 0 {
		activeDays = days
	}
	bursts := []int{}
	remaining := n
	for remaining > 0 {
		burst := 1
		if intensity.maxBurst > 1 {
			burst += rng.Intn(intensity.maxBurst)
		}
		if burst > remaining {
			burst = remaining
		}
		if len(bursts) == 0 && remaining > 1 && len(activeDays) > 1 && burst == remaining {
			burst--
		}
		bursts = append(bursts, burst)
		remaining -= burst
	}
	dayIndexes := make([]int, len(bursts))
	totalWeight := 0
	gapWeights := make([]int, len(bursts)-1)
	for i := range gapWeights {
		weight := 1
		if rng.Float64() < intensity.pauseChance {
			weight += 1 + rng.Intn(2)
		}
		gapWeights[i] = weight
		totalWeight += weight
	}
	cumulativeWeight := 0
	for i := 1; i < len(dayIndexes); i++ {
		cumulativeWeight += gapWeights[i-1]
		dayIndex := int(math.Round(float64(cumulativeWeight) * float64(len(activeDays)-1) / float64(totalWeight)))
		if len(dayIndexes) <= len(activeDays) {
			dayIndex = maxInt(dayIndex, dayIndexes[i-1]+1)
			maxIndex := len(activeDays) - (len(dayIndexes) - i)
			if dayIndex > maxIndex {
				dayIndex = maxIndex
			}
		}
		dayIndexes[i] = dayIndex
	}
	daySlots := make([]int64, 0, n)
	for i, burst := range bursts {
		for range burst {
			daySlots = append(daySlots, activeDays[dayIndexes[i]])
		}
	}
	byDay := map[int64][]int{}
	for i, day := range daySlots {
		byDay[day] = append(byDay[day], i)
	}
	timestamps := make([]int64, n)
	plannedDays := make([]int64, 0, len(byDay))
	for day := range byDay {
		plannedDays = append(plannedDays, day)
	}
	sort.Slice(plannedDays, func(i, j int) bool { return plannedDays[i] < plannedDays[j] })
	for _, day := range plannedDays {
		indexes := byDay[day]
		sort.Ints(indexes)
		startOfWorkday := day + 8*3600 + int64(rng.Intn(60))*60
		endOfWorkday := day + 18*3600
		if startOfWorkday < startEpoch {
			startOfWorkday = startEpoch
		}
		if endOfWorkday > endEpoch {
			endOfWorkday = endEpoch
		}
		if endOfWorkday < startOfWorkday {
			endOfWorkday = startOfWorkday
		}
		spacing := int64(1)
		if len(indexes) > 1 {
			available := endOfWorkday - startOfWorkday
			if available > int64(len(indexes)-1) {
				spacing = available / int64(len(indexes)-1)
			}
		}
		for i, slotIndex := range indexes {
			jitter := int64(0)
			if spacing > 120 {
				jitter = int64(rng.Intn(int(minInt64(spacing/3, 900))))
			}
			ts := startOfWorkday + int64(i)*spacing + jitter
			if ts > endEpoch {
				ts = endEpoch
			}
			if ts < startEpoch {
				ts = startEpoch
			}
			timestamps[slotIndex] = ts
		}
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	return timestamps
}

func daysInRange(startEpoch, endEpoch int64) []int64 {
	startDay := floorDay(startEpoch)
	endDay := floorDay(endEpoch)
	days := []int64{}
	for day := startDay; day <= endDay; day += 86400 {
		days = append(days, day)
	}
	if len(days) == 0 {
		days = append(days, startDay)
	}
	return days
}

func activePlanningDays(days []int64, intensity rewriteDateIntensity, rng *rand.Rand) []int64 {
	active := []int64{}
	for _, day := range days {
		if len(days) == 1 || rng.Float64() <= intensity.activeRatio {
			active = append(active, day)
		}
	}
	if len(active) == 0 {
		active = append(active, days[rng.Intn(len(days))])
	}
	sort.Slice(active, func(i, j int) bool { return active[i] < active[j] })
	return active
}

func seedInt64(seed string) int64 {
	sum := sha256.Sum256([]byte(seed))
	value := int64(0)
	for _, b := range sum[:8] {
		value = (value << 8) | int64(b)
	}
	if value < 0 {
		value = -value
	}
	return value
}

func floorDay(epoch int64) int64 {
	return epoch - positiveMod(epoch, 86400)
}

func positiveMod(value, mod int64) int64 {
	result := value % mod
	if result < 0 {
		result += mod
	}
	return result
}

func renderRewriteDatePlan(a *app, plan rewriteDatePlan, opts rewriteDatesOptions) {
	fmt.Fprintln(a.stdout, "Date Rewrite Plan")
	renderKeyValuesTo(a.stdout, []keyValueRow{
		{key: "Repositories", value: fmt.Sprintf("%d", len(plan.candidates))},
		{key: "Selected commits", value: fmt.Sprintf("%d", plan.totalSelected)},
		{key: "Target range", value: formatEpochLocal(plan.targetStart) + " -> " + formatEpochLocal(plan.targetEnd)},
		{key: "Filters", value: rewriteDateFilterDescription(opts)},
		{key: "Intensity", value: fmt.Sprintf("%s (active days %.0f%%, max burst %d)", plan.intensity.name, plan.intensity.activeRatio*100, plan.intensity.maxBurst)},
		{key: "Seed", value: fmt.Sprintf("%s (%s)", plan.seed, plan.seedSource)},
	})
	fmt.Fprintln(a.stdout)
	for _, candidate := range plan.candidates {
		renderRepoHeader(a, candidate.repo.display)
		fmt.Fprintf(a.stdout, "  Selected commits: %d of %d\n", len(candidate.selected), len(candidate.commits))
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
			fmt.Fprintf(a.stdout, "  %s  %s -> %s\n", prefix(commit.hash, 8), formatEpoch(commit.originalAuthorEpoch, commit.originalAuthorTZ), formatEpoch(commit.plannedEpoch, candidate.tzOffset))
		}
		fmt.Fprintln(a.stdout)
	}
}

func renderRewriteDateRollbackPlan(a *app, candidates []dateCandidate, knownTotal, unknownTotal, exactBranches, replayBranches, replayCommits, skippedBranches int) {
	fmt.Fprintln(a.stdout, "Date Rollback Plan")
	renderKeyValuesTo(a.stdout, []keyValueRow{
		{key: "Repositories", value: fmt.Sprintf("%d", len(candidates))},
		{key: "Known commits", value: fmt.Sprintf("%d", knownTotal)},
		{key: "Unknown/new commits", value: fmt.Sprintf("%d", unknownTotal)},
		{key: "Exact branch restores", value: fmt.Sprintf("%d", exactBranches)},
		{key: "Branches replaying new commits", value: fmt.Sprintf("%d (%d commits)", replayBranches, replayCommits)},
		{key: "Skipped branches", value: fmt.Sprintf("%d", skippedBranches)},
	})
	fmt.Fprintln(a.stdout)
	for _, candidate := range candidates {
		known := 0
		unknown := 0
		for _, commit := range candidate.commits {
			if commit.knownInState {
				known++
			} else {
				unknown++
			}
		}
		renderRepoHeader(a, candidate.repo.display)
		fmt.Fprintf(a.stdout, "  Known commits: %d\n", known)
		fmt.Fprintf(a.stdout, "  Unknown/new commits: %d\n", unknown)
		fmt.Fprintf(a.stdout, "  Branches: %d exact, %d replay, %d skipped\n", candidate.rollbackPlan.exact, candidate.rollbackPlan.replay, candidate.rollbackPlan.skipped)
		warnings := rewriteDateCandidateWarnings(candidate)
		if len(warnings) > 0 {
			fmt.Fprintf(a.stdout, "  Warnings: %s\n", strings.Join(warnings, "; "))
		}
		fmt.Fprintln(a.stdout, "  Sample:")
		shown := 0
		for _, commit := range candidate.commits {
			if !commit.knownInState {
				continue
			}
			if shown >= 3 {
				if known > 3 {
					fmt.Fprintln(a.stdout, "    ...")
				}
				break
			}
			fmt.Fprintf(a.stdout, "  %s  %s -> %s\n", prefix(commit.hash, 8), formatEpoch(commit.authorEpoch, commit.authorTZ), formatEpoch(commit.originalAuthorEpoch, commit.originalAuthorTZ))
			shown++
		}
		fmt.Fprintln(a.stdout)
	}
}

func rewriteDateFilterDescription(opts rewriteDatesOptions) string {
	parts := []string{}
	if opts.rewriteAfterDate != "" {
		parts = append(parts, "after "+opts.rewriteAfterDate)
	}
	if opts.rewriteBeforeDate != "" {
		parts = append(parts, "before "+opts.rewriteBeforeDate)
	}
	if len(parts) == 0 {
		return "all local branch commits"
	}
	return strings.Join(parts, ", ")
}

func rewriteDateCandidateWarnings(candidate dateCandidate) []string {
	warnings := []string{}
	if candidate.hasTags {
		warnings = append(warnings, "tags may still point at old history")
	}
	if candidate.hasSignedObjects {
		if candidate.rollbackPlan.replay > 0 {
			warnings = append(warnings, "replayed commits may lose signatures")
		} else if candidate.rollbackPlan.exact == 0 && candidate.rollbackPlan.skipped == 0 {
			warnings = append(warnings, "signatures may become invalid")
		}
	}
	return warnings
}

func plannedRange(candidate dateCandidate) (int64, int64) {
	first := int64(0)
	last := int64(0)
	for i, commitIndex := range candidate.selected {
		planned := candidate.commits[commitIndex].plannedEpoch
		if i == 0 || planned < first {
			first = planned
		}
		if i == 0 || planned > last {
			last = planned
		}
	}
	return first, last
}

func applyRewriteDateCandidate(a *app, filterCmd []string, candidate dateCandidate) (string, error, error) {
	mapping := map[string]dateCallbackDates{}
	for _, commitIndex := range candidate.selected {
		commit := candidate.commits[commitIndex]
		date := fmt.Sprintf("%d %s", commit.plannedEpoch, candidate.tzOffset)
		mapping[commit.hash] = dateCallbackDates{Author: date, Committer: date}
	}
	return applyDateCallbackCandidate(a, filterCmd, candidate, mapping)
}

func applyRollbackDateCandidate(a *app, candidate dateCandidate) (string, error, error) {
	return applyExactRollbackDateCandidate(a, candidate)
}

func applyExactRollbackDateCandidate(a *app, candidate dateCandidate) (string, error, error) {
	if err := writeRewriteDatesState(a, candidate.repo.dir, candidate.state); err != nil {
		return "", fmt.Errorf("could not save rewrite state: %w", err), nil
	}
	runID := rewriteDatesRunID(candidate.state.Seed + "-rollback")
	backupRefs, err := createRewriteDatesBackupRefs(a, candidate.repo.dir, runID, candidate.branches)
	if err != nil {
		return "", fmt.Errorf("could not create rollback backup refs: %w", err), nil
	}
	replayed := map[string]string{}
	movedRefs := map[string]bool{}
	for _, branch := range candidate.rollbackPlan.branches {
		switch branch.Action {
		case dateRollbackExact:
			if _, err := a.git.Capture(a.ctx, candidate.repo.dir, nil, "update-ref", branch.Name, branch.OriginalHead, branch.CurrentHead); err != nil {
				return "", fmt.Errorf("could not restore %s to %s: %w", branch.Name, prefix(branch.OriginalHead, 8), err), nil
			}
			movedRefs[branch.Name] = true
		case dateRollbackReplay:
			newHead, branchReplayed, err := replayRewriteDateRollbackBranch(a, candidate.repo.dir, candidate.state, branch)
			if err != nil {
				return "", fmt.Errorf("could not replay commits for %s: %w", branch.Name, err), nil
			}
			if _, err := a.git.Capture(a.ctx, candidate.repo.dir, nil, "update-ref", branch.Name, newHead, branch.CurrentHead); err != nil {
				return "", fmt.Errorf("could not move %s to replayed head %s: %w", branch.Name, prefix(newHead, 8), err), nil
			}
			for oldSHA, newSHA := range branchReplayed {
				replayed[oldSHA] = newSHA
			}
			movedRefs[branch.Name] = true
		}
	}
	candidate.state = updateRewriteDatesStateAfterExactRollback(candidate.state, replayed)
	if err := writeRewriteDatesState(a, candidate.repo.dir, candidate.state); err != nil {
		return "", fmt.Errorf("could not update rewrite state: %w", err), nil
	}
	if err := verifyRewriteDateRefs(a, candidate.repo.dir, backupRefs); err != nil {
		return "", err, nil
	}
	if err := resetRewriteDatesCheckedOutBranch(a, candidate.repo.dir, movedRefs); err != nil {
		return "", fmt.Errorf("could not reset worktree after rollback: %w", err), nil
	}
	return "", nil, nil
}

func applyDateCallbackCandidate(a *app, filterCmd []string, candidate dateCandidate, mapping map[string]dateCallbackDates) (string, error, error) {
	if len(mapping) == 0 {
		return "", fmt.Errorf("no commit date mapping generated"), nil
	}
	if !rewriteDatesWorkingTreeClean(a, candidate.repo.dir) {
		return "", fmt.Errorf("working tree must be clean before rewriting dates"), nil
	}
	if err := writeRewriteDatesState(a, candidate.repo.dir, candidate.state); err != nil {
		return "", fmt.Errorf("could not save rewrite state: %w", err), nil
	}
	runID := rewriteDatesRunID(candidate.state.Seed)
	backupRefs, err := createRewriteDatesBackupRefs(a, candidate.repo.dir, runID, candidate.branches)
	if err != nil {
		return "", fmt.Errorf("could not create backup refs: %w", err), nil
	}
	callback, err := writeDateCallbackDates(mapping)
	if err != nil {
		return "", fmt.Errorf("timestamp generation failed: %w", err), nil
	}
	defer os.Remove(callback)
	callbackSource, err := os.ReadFile(callback)
	if err != nil {
		return "", fmt.Errorf("could not read generated timestamp callback: %w", err), nil
	}
	out, runErr, restoreErr := runFilterRepoRestoringOrigin(a, candidate.repo.dir, candidate.repo.gitDir, filterCmd, rewriteDateFilterArgs(candidate.branches, string(callbackSource)), nil)
	if runErr != nil {
		return out, runErr, restoreErr
	}
	commitMap, err := readRewriteDateCommitMap(candidate.repo.gitDir)
	if err != nil {
		return out, fmt.Errorf("could not read commit map: %w", err), restoreErr
	}
	candidate.state = updateRewriteDatesStateFromCommitMap(candidate.state, commitMap)
	candidate.state, err = updateRewriteDatesStateBranchesAfterRewrite(a, candidate.repo.dir, candidate.state, runID, candidate.branches, backupRefs)
	if err != nil {
		return out, fmt.Errorf("could not update branch rewrite state: %w", err), restoreErr
	}
	if err := writeRewriteDatesState(a, candidate.repo.dir, candidate.state); err != nil {
		return out, fmt.Errorf("could not update rewrite state: %w", err), restoreErr
	}
	if err := verifyRewriteDateRefs(a, candidate.repo.dir, backupRefs); err != nil {
		return out, err, restoreErr
	}
	return out, nil, restoreErr
}

type rewriteDateReplayCommit struct {
	SHA            string
	Tree           string
	Parents        []string
	AuthorName     string
	AuthorEmail    string
	AuthorDate     string
	CommitterName  string
	CommitterEmail string
	CommitterDate  string
	Message        string
}

func replayRewriteDateRollbackBranch(a *app, dir string, state rewriteDatesState, branch dateRollbackBranchPlan) (string, map[string]string, error) {
	out, err := a.git.Stdout(a.ctx, dir, nil, "rev-list", "--topo-order", "--reverse", branch.CurrentHead, "--not", branch.RewrittenHead)
	if err != nil {
		return "", nil, err
	}
	shas := splitLines(out)
	if len(shas) == 0 {
		return branch.OriginalHead, map[string]string{}, nil
	}
	parentMap := rewriteDateRollbackParentMap(state)
	parentMap[branch.RewrittenHead] = branch.OriginalHead
	originalDatesByCurrent := rewriteDatesOriginalDatesByCurrent(state)
	replayed := map[string]string{}
	newHead := branch.OriginalHead
	for _, sha := range shas {
		commit, err := readRewriteDateReplayCommit(a, dir, sha)
		if err != nil {
			return "", nil, err
		}
		if dates, ok := originalDatesByCurrent[commit.SHA]; ok {
			commit.AuthorDate = dates.Author
			commit.CommitterDate = dates.Committer
		}
		parentArgs := []string{}
		for _, parent := range commit.Parents {
			mapped, ok := replayed[parent]
			if !ok {
				mapped, ok = parentMap[parent]
			}
			if !ok || mapped == "" {
				return "", nil, fmt.Errorf("parent %s of %s cannot be mapped safely", prefix(parent, 8), prefix(commit.SHA, 8))
			}
			parentArgs = append(parentArgs, "-p", mapped)
		}
		args := []string{"commit-tree", commit.Tree}
		args = append(args, parentArgs...)
		env := []string{
			"GIT_AUTHOR_NAME=" + commit.AuthorName,
			"GIT_AUTHOR_EMAIL=" + commit.AuthorEmail,
			"GIT_AUTHOR_DATE=" + commit.AuthorDate,
			"GIT_COMMITTER_NAME=" + commit.CommitterName,
			"GIT_COMMITTER_EMAIL=" + commit.CommitterEmail,
			"GIT_COMMITTER_DATE=" + commit.CommitterDate,
		}
		ctx := run.WithStdin(a.ctx, commit.Message)
		newSHAOut, err := run.Stdout(ctx, a.runner, dir, env, "git", args...)
		if err != nil {
			return "", nil, err
		}
		newSHA := strings.TrimSpace(firstLine(newSHAOut))
		if newSHA == "" {
			return "", nil, fmt.Errorf("git commit-tree returned an empty object id for %s", prefix(commit.SHA, 8))
		}
		replayed[commit.SHA] = newSHA
		parentMap[commit.SHA] = newSHA
		newHead = newSHA
	}
	return newHead, replayed, nil
}

func rewriteDatesOriginalDatesByCurrent(state rewriteDatesState) map[string]dateCallbackDates {
	byCurrent := map[string]dateCallbackDates{}
	for _, commit := range state.Commits {
		if commit.CurrentSHA == "" {
			continue
		}
		byCurrent[commit.CurrentSHA] = dateCallbackDates{
			Author:    commit.OriginalAuthorDate,
			Committer: commit.OriginalCommitterDate,
		}
	}
	return byCurrent
}

func readRewriteDateReplayCommit(a *app, dir, sha string) (rewriteDateReplayCommit, error) {
	format := "%H%x00%T%x00%P%x00%an%x00%ae%x00%aI%x00%cn%x00%ce%x00%cI"
	out, err := a.git.Stdout(a.ctx, dir, nil, "show", "-s", "--format="+format, sha)
	if err != nil {
		return rewriteDateReplayCommit{}, err
	}
	fields := strings.Split(strings.TrimRight(out, "\r\n"), "\x00")
	if len(fields) != 9 {
		return rewriteDateReplayCommit{}, fmt.Errorf("malformed replay metadata for %s", prefix(sha, 8))
	}
	message, err := a.git.Stdout(a.ctx, dir, nil, "log", "-1", "--format=%B", sha)
	if err != nil {
		return rewriteDateReplayCommit{}, err
	}
	return rewriteDateReplayCommit{
		SHA:            strings.TrimSpace(fields[0]),
		Tree:           strings.TrimSpace(fields[1]),
		Parents:        strings.Fields(fields[2]),
		AuthorName:     fields[3],
		AuthorEmail:    fields[4],
		AuthorDate:     fields[5],
		CommitterName:  fields[6],
		CommitterEmail: fields[7],
		CommitterDate:  fields[8],
		Message:        message,
	}, nil
}

func rewriteDateRollbackParentMap(state rewriteDatesState) map[string]string {
	parentMap := map[string]string{}
	for _, commit := range state.Commits {
		if commit.OriginalSHA != "" {
			parentMap[commit.OriginalSHA] = commit.OriginalSHA
		}
		if commit.CurrentSHA != "" && commit.OriginalSHA != "" {
			parentMap[commit.CurrentSHA] = commit.OriginalSHA
		}
	}
	return parentMap
}

func resetRewriteDatesCheckedOutBranch(a *app, dir string, movedRefs map[string]bool) error {
	if len(movedRefs) == 0 {
		return nil
	}
	out, err := a.git.Stdout(a.ctx, dir, nil, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return nil
	}
	ref := strings.TrimSpace(firstLine(out))
	if ref == "" || !movedRefs[ref] {
		return nil
	}
	_, err = a.git.Capture(a.ctx, dir, nil, "reset", "--hard", "HEAD")
	return err
}

func rewriteDateFilterArgs(branches []dateBranchRef, callback string) []string {
	args := []string{"--partial"}
	if len(branches) > 0 {
		args = append(args, "--refs")
		for _, branch := range branches {
			if strings.HasPrefix(branch.Name, "refs/git-wrangler/") {
				continue
			}
			args = append(args, branch.Name)
		}
	}
	args = append(args, "--commit-callback", callback, "--force")
	return args
}

func createRewriteDatesBackupRefs(a *app, dir, runID string, branches []dateBranchRef) ([]rewriteDatesBackupRef, error) {
	backupRefs := []rewriteDatesBackupRef{}
	for _, branch := range branches {
		suffix := strings.TrimPrefix(branch.Name, "refs/")
		ref := rewriteDatesBackupPrefix + "/" + runID + "/" + suffix
		if _, err := a.git.Capture(a.ctx, dir, nil, "update-ref", ref, branch.SHA); err != nil {
			return backupRefs, err
		}
		backupRefs = append(backupRefs, rewriteDatesBackupRef{Branch: branch, Ref: ref})
	}
	return backupRefs, nil
}

func rewriteDatesRunID(seed string) string {
	base := time.Now().UTC().Format("20060102T150405Z")
	sum := sha256.Sum256([]byte(base + seed))
	return base + "-" + fmt.Sprintf("%x", sum[:4])
}

func readRewriteDateCommitMap(gitDir string) (map[string]string, error) {
	path := filepath.Join(rewriteDatesGitMetadataDir(gitDir), "filter-repo", "commit-map")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	mapping := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] == "old" {
			continue
		}
		if isZeroSHA(fields[1]) {
			continue
		}
		mapping[fields[0]] = fields[1]
	}
	return mapping, nil
}

func updateRewriteDatesStateBranchesAfterRewrite(a *app, dir string, state rewriteDatesState, runID string, originalBranches []dateBranchRef, backupRefs []rewriteDatesBackupRef) (rewriteDatesState, error) {
	rewrittenBranches, err := localBranchRefs(a, dir)
	if err != nil {
		return rewriteDatesState{}, err
	}
	rewrittenByName := map[string]string{}
	for _, branch := range rewrittenBranches {
		rewrittenByName[branch.Name] = branch.SHA
	}
	backupByName := map[string]string{}
	for _, backup := range backupRefs {
		backupByName[backup.Branch.Name] = backup.Ref
	}
	branchesByName := rewriteDatesBranchStateByName(state.Branches)
	for _, branch := range originalBranches {
		rewrittenHead := rewrittenByName[branch.Name]
		if rewrittenHead == "" {
			return rewriteDatesState{}, fmt.Errorf("%s is missing after rewrite", branch.Name)
		}
		backupRef := backupByName[branch.Name]
		if backupRef == "" {
			return rewriteDatesState{}, fmt.Errorf("%s backup ref is missing", branch.Name)
		}
		originalHead := branch.SHA
		originalBackupRef := backupRef
		if existing, ok := branchesByName[branch.Name]; ok {
			if existing.OriginalHead == "" || existing.BackupRef == "" {
				return rewriteDatesState{}, fmt.Errorf("%s has incomplete branch rollback metadata", branch.Name)
			}
			originalHead = existing.OriginalHead
			originalBackupRef = existing.BackupRef
		}
		currentRewrittenHead, ok := rewriteDatesCurrentSHAForOriginal(state, originalHead)
		if !ok || currentRewrittenHead == "" {
			if originalHead != branch.SHA {
				return rewriteDatesState{}, fmt.Errorf("%s original head %s is missing from rewrite state", branch.Name, prefix(originalHead, 8))
			}
			currentRewrittenHead = rewrittenHead
		}
		branchesByName[branch.Name] = rewriteDatesStateBranch{
			Name:          branch.Name,
			OriginalHead:  originalHead,
			RewrittenHead: currentRewrittenHead,
			BackupRef:     originalBackupRef,
			RunID:         runID,
		}
	}
	state.Branches = make([]rewriteDatesStateBranch, 0, len(branchesByName))
	for _, branch := range branchesByName {
		state.Branches = append(state.Branches, branch)
	}
	return normalizeRewriteDatesState(state), nil
}

func rewriteDatesCurrentSHAForOriginal(state rewriteDatesState, originalSHA string) (string, bool) {
	for _, commit := range state.Commits {
		if commit.OriginalSHA == originalSHA {
			return commit.CurrentSHA, true
		}
	}
	return "", false
}

func rewriteDatesGitMetadataDir(gitDir string) string {
	if gitDir == "" {
		return ".git"
	}
	info, err := os.Stat(gitDir)
	if err == nil && info.IsDir() {
		return gitDir
	}
	data, err := os.ReadFile(gitDir)
	if err != nil {
		return gitDir
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return gitDir
	}
	metadataDir := strings.TrimSpace(target)
	if !filepath.IsAbs(metadataDir) {
		metadataDir = filepath.Join(filepath.Dir(gitDir), metadataDir)
	}
	return metadataDir
}

func isZeroSHA(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && strings.Trim(value, "0") == ""
}

func updateRewriteDatesStateFromCommitMap(state rewriteDatesState, commitMap map[string]string) rewriteDatesState {
	for i := range state.Commits {
		if next, ok := commitMap[state.Commits[i].CurrentSHA]; ok && next != "" {
			state.Commits[i].CurrentSHA = next
		}
	}
	return normalizeRewriteDatesState(state)
}

func updateRewriteDatesStateAfterExactRollback(state rewriteDatesState, replayed map[string]string) rewriteDatesState {
	for i := range state.Commits {
		if state.Commits[i].OriginalSHA == "" {
			continue
		}
		previousCurrent := state.Commits[i].CurrentSHA
		state.Commits[i].CurrentSHA = state.Commits[i].OriginalSHA
		if next, ok := replayed[previousCurrent]; ok && next != "" {
			state.Commits[i].CurrentSHA = next
		}
	}
	for i := range state.Branches {
		if state.Branches[i].OriginalHead != "" {
			state.Branches[i].RewrittenHead = state.Branches[i].OriginalHead
		}
	}
	return normalizeRewriteDatesState(state)
}

func verifyRewriteDateRefs(a *app, dir string, backupRefs []rewriteDatesBackupRef) error {
	refs := []string{rewriteDatesStateRef}
	for _, backup := range backupRefs {
		refs = append(refs, backup.Ref)
	}
	for _, ref := range refs {
		if _, err := a.git.Capture(a.ctx, dir, nil, "rev-parse", "--verify", "--quiet", ref); err != nil {
			return fmt.Errorf("expected rewrite ref %s to exist after rewrite", ref)
		}
	}
	return nil
}

func rollbackSelectedIndexes(commits []rewriteDateCommit) []int {
	indexes := []int{}
	for i, commit := range commits {
		if commit.knownInState {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func parseLocalDate(s string) int64 {
	return parseDateStart(s)
}

func parseDateStart(s string) int64 {
	t, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return t.Unix()
}

func parseDateEnd(s string) int64 {
	return parseDateStart(s) + 86399
}

type commitTime struct {
	hash  string
	epoch int64
}

func firstCommitEpoch(a *app, dir string, flags ...string) (int64, error) {
	args := append([]string{"log", "--all"}, flags...)
	args = append(args, "--format=%at")
	if !stringSliceContains(flags, "--reverse") {
		args = append(args, "-1")
	}
	out, err := a.git.Stdout(a.ctx, dir, nil, args...)
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(firstLine(out))
	if value == "" {
		return 0, fmt.Errorf("empty timestamp")
	}
	epoch, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("malformed timestamp %q", value)
	}
	return epoch, nil
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func readCommitTimes(a *app, dir string) ([]commitTime, error) {
	out, err := a.git.Stdout(a.ctx, dir, nil, "log", "--all", "--reverse", "--format=%H %at")
	if err != nil {
		return nil, err
	}
	var commits []commitTime
	for _, line := range splitLines(out) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("malformed commit timestamp line %q", line)
		}
		epoch, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("malformed timestamp %q for %s", fields[1], fields[0])
		}
		commits = append(commits, commitTime{hash: fields[0], epoch: epoch})
	}
	return commits, nil
}

func dominantTimezoneOffset(a *app, dir string) (string, error) {
	out, err := a.git.Stdout(a.ctx, dir, nil, "log", "--all", "--format=%ai")
	if err != nil {
		return "", err
	}
	counts := map[string]int{}
	for _, line := range splitLines(out) {
		if len(line) >= 5 {
			offset := line[len(line)-5:]
			if timezoneOffsetRe.MatchString(offset) {
				counts[offset]++
			}
		}
	}
	return dominantTimezoneOffsetFromCounts(counts)
}

func dominantTimezoneOffsetFromCommits(commits []rewriteDateCommit) string {
	counts := map[string]int{}
	for _, commit := range commits {
		if timezoneOffsetRe.MatchString(commit.originalAuthorTZ) {
			counts[commit.originalAuthorTZ]++
		}
	}
	offset, _ := dominantTimezoneOffsetFromCounts(counts)
	return offset
}

func dominantTimezoneOffsetFromCounts(counts map[string]int) (string, error) {
	best := ""
	bestCount := 0
	for offset, count := range counts {
		if count > bestCount {
			best = offset
			bestCount = count
		}
	}
	if best != "" {
		return best, nil
	}
	return time.Now().Format("-0700"), nil
}

func distributeCommitTimes(commits []commitTime, startEpoch, endEpoch int64) map[string]int64 {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	n := len(commits)
	if n == 0 {
		return map[string]int64{}
	}
	totalRange := float64(endEpoch - startEpoch)
	if totalRange <= 0 {
		totalRange = 86400
	}
	slotWidth := totalRange / float64(n)
	timestamps := make([]int64, n)
	for i := range commits {
		slotStart := float64(startEpoch) + float64(i)*slotWidth
		slotCenter := slotStart + slotWidth/2
		jitter := (rng.Float64()*0.8 - 0.4) * slotWidth
		raw := slotCenter + jitter
		dayStart := int64(raw/86400) * 86400
		hour := sampleBimodalHour(rng)
		ts := dayStart + int64(hour)*3600 + int64(rng.Intn(60))*60 + int64(rng.Intn(60))
		if isWeekend(ts) && rng.Float64() < 0.65 {
			wd := weekdayFromEpoch(ts)
			if wd == 2 {
				ts = dayStart - 86400 + int64(18+rng.Intn(5))*3600 + int64(rng.Intn(60))*60 + int64(rng.Intn(60))
			} else {
				ts = dayStart + 86400 + int64(7+rng.Intn(3))*3600 + int64(rng.Intn(60))*60 + int64(rng.Intn(60))
			}
		}
		timestamps[i] = ts
	}
	dayGroups := map[int64][]int{}
	for i, ts := range timestamps {
		dayGroups[ts/86400] = append(dayGroups[ts/86400], i)
	}
	for day, indices := range dayGroups {
		if len(indices) < 2 {
			continue
		}
		rng.Shuffle(len(indices), func(i, j int) { indices[i], indices[j] = indices[j], indices[i] })
		spacing := int64((25 + rng.Intn(66)) * 60)
		latestStart := 22.0 - float64(len(indices)-1)*float64(spacing)/3600.0
		startHour := 7.0
		if latestStart > 7.0 {
			startHour = 7.0 + rng.Float64()*(latestStart-7.0)
		}
		current := int64(startHour * 3600)
		for _, idx := range indices {
			timestamps[idx] = day*86400 + current + int64(rng.Intn(60))
			current += spacing
		}
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	mapping := map[string]int64{}
	for i, c := range commits {
		mapping[c.hash] = timestamps[i]
	}
	return mapping
}

func sampleBimodalHour(rng *rand.Rand) int {
	peak := 10.0
	if rng.Float64() >= 0.5 {
		peak = 15.0
	}
	u1 := rng.Float64()
	if u1 == 0 {
		u1 = 1e-10
	}
	u2 := rng.Float64()
	z := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
	hour := peak + 2.0*z
	if hour < 7 {
		hour = 7
	}
	if hour > 22 {
		hour = 22
	}
	return int(hour)
}

func weekdayFromEpoch(ts int64) int64 {
	return (ts/86400 + 4) % 7
}

func isWeekend(ts int64) bool {
	wd := weekdayFromEpoch(ts)
	return wd == 0 || wd == 6
}

func formatEpoch(epoch int64, offset string) string {
	sign := 1
	if strings.HasPrefix(offset, "-") {
		sign = -1
	}
	hours, _ := strconv.Atoi(offset[1:3])
	minutes, _ := strconv.Atoi(offset[3:5])
	loc := time.FixedZone(offset, sign*(hours*3600+minutes*60))
	return time.Unix(epoch, 0).In(loc).Format("2006-01-02 15:04:05 ") + offset
}

func formatEpochLocal(epoch int64) string {
	return time.Unix(epoch, 0).In(time.Local).Format("2006-01-02 15:04:05 -0700")
}

func writeDateCallback(mapping map[string]int64, tzOffset string) (string, error) {
	values := map[string]dateCallbackDates{}
	for hash, epoch := range mapping {
		date := fmt.Sprintf("%d %s", epoch, tzOffset)
		values[hash] = dateCallbackDates{Author: date, Committer: date}
	}
	return writeDateCallbackDates(values)
}

func writeDateCallbackDates(mapping map[string]dateCallbackDates) (string, error) {
	f, err := os.CreateTemp("", "git-wrangler-date-callback-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	fmt.Fprintln(f, "mapping = {}")
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := mapping[key]
		fmt.Fprintf(f, "mapping[%s] = (%s, %s)\n", git.PythonBytesLiteral(key), git.PythonBytesLiteral(value.Author), git.PythonBytesLiteral(value.Committer))
	}
	fmt.Fprintln(f, "if commit.original_id in mapping:")
	fmt.Fprintln(f, "    dates = mapping[commit.original_id]")
	fmt.Fprintln(f, "    commit.author_date = dates[0]")
	fmt.Fprintln(f, "    commit.committer_date = dates[1]")
	return f.Name(), nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
