package cli

import (
	"crypto/sha256"
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
	"github.com/spf13/cobra"
)

const (
	rewriteDatesStateRef     = "refs/git-wrangler/state/rewrite-dates"
	rewriteDatesBackupPrefix = "refs/git-wrangler/backup/rewrite-dates"
)

var timezoneOffsetRe = regexp.MustCompile(`^[+-][0-9]{4}$`)

type rewriteDatesOptions struct {
	target       targetOptions
	fetch        fetchOptions
	confirmation confirmationOptions
	startDate    string
	endDate      string
	untilDate    string
	seed         string
	frequency    string
	spread       string
	days         int
	window       string
	timeSchedule *commitTimeSchedule
	bounds       currentRewriteDateBounds
}

type rewriteDateProfile struct {
	frequencyName             string
	frequencyDescription      string
	spreadName                string
	spreadDescription         string
	activeRatio               float64
	commitSpread              float64
	demandScale               float64
	daySigma                  float64
	sessionSigma              float64
	persistence               float64
	weekendActivityMultiplier float64
	eveningChance             float64
	sessionMinDays            int
	sessionMaxDays            int
	gapMinDays                int
	gapMaxDays                int
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
	rawPlannedEpoch        int64
	plannedEpoch           int64
}

type dateRewriteScan struct {
	repo             repo
	err              error
	errLabel         string
	hasHead          bool
	noBranches       bool
	tooFew           bool
	branches         []dateBranchRef
	commits          []rewriteDateCommit
	selected         []int
	tzOffset         string
	hasTags          bool
	hasSignedObjects bool
}

type dateCandidate struct {
	repo               repo
	branches           []dateBranchRef
	commits            []rewriteDateCommit
	selected           []int
	tzOffset           string
	hasTags            bool
	hasSignedObjects   bool
	topologyCompressed bool
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
	profile       rewriteDateProfile
	tzOffset      string
	calendar      rewriteDateCalendarPlan
	startFixed    bool
	endFixed      bool
}

type rewriteDateRestPolicy struct {
	sparseInactiveDays  bool
	softRestBlocks      int
	seasonalYearEnd     bool
	generatedRestBlocks int
	summerVacations     bool
}

type rewriteDateRestBlock struct {
	startDay int64
	endDay   int64
}

type rewriteDateCalendarDayState string

const (
	rewriteDateCalendarInactive     rewriteDateCalendarDayState = "inactive"
	rewriteDateCalendarRest         rewriteDateCalendarDayState = "rest"
	rewriteDateCalendarActive       rewriteDateCalendarDayState = "active"
	rewriteDateCalendarForcedActive rewriteDateCalendarDayState = "forced-active"
)

type rewriteDateCalendarDay struct {
	epoch int64
	state rewriteDateCalendarDayState
	quota int
}

type rewriteDateCalendarPlan struct {
	days        []rewriteDateCalendarDay
	restBlocks  []rewriteDateRestBlock
	tzOffset    string
	targetStart int64
	targetEnd   int64
}

type rewriteDateTopologyCompression struct {
	compressed          bool
	forcedActiveDays    int
	maxOneSecondRun     int
	oneSecondEdges      int
	dailyQuotaDeviation int
	adjustedCommits     int
	selectedEdges       int
	candidateCompressed map[int]bool
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

func runRewriteDates(a *app, cmd *cobra.Command, args []string) int {
	opts, ok := rewriteDatesOptionsFromCommand(a, cmd)
	if !ok {
		return 1
	}
	if !requireGit(a, "rewrite-dates") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-dates")
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
	return runRewriteDatesRewrite(a, repos, filterCmd, opts)
}

func rewriteDatesOptionsFromCommand(a *app, cmd *cobra.Command) (rewriteDatesOptions, bool) {
	opts := rewriteDatesOptions{
		target:       targetOptionsFromCommand(cmd),
		fetch:        fetchOptionsFromCommand(cmd),
		confirmation: confirmationOptionsFromCommand(cmd),
		startDate:    stringFlagValue(cmd, "start-date"),
		endDate:      stringFlagValue(cmd, "end-date"),
		untilDate:    stringFlagValue(cmd, "until"),
		seed:         stringFlagValue(cmd, "seed"),
		frequency:    stringFlagValue(cmd, "frequency"),
		spread:       stringFlagValue(cmd, "spread"),
		days:         intFlagValue(cmd, "days"),
		window:       stringFlagValue(cmd, "window"),
	}
	daysSet := flagChanged(cmd, "days")
	boundOpts, err := rewriteBoundOptionsFromCommand(cmd)
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return rewriteDatesOptions{}, false
	}
	opts.bounds = boundOpts.bounds

	for _, value := range []struct {
		name string
		date string
	}{
		{name: "start-date", date: opts.startDate},
		{name: "end-date", date: opts.endDate},
		{name: "until", date: opts.untilDate},
	} {
		if value.date != "" && !validDate(value.date) {
			a.plainErrorf("--%s must be in YYYY-MM-DD format.", value.name)
			return rewriteDatesOptions{}, false
		}
	}
	if daysSet && opts.days <= 0 {
		a.plainErrorf("--days must be a positive integer.")
		return rewriteDatesOptions{}, false
	}
	if opts.days > 0 && (opts.startDate != "" || opts.endDate != "") {
		a.plainErrorf("--days cannot be combined with --start-date or --end-date.")
		return rewriteDatesOptions{}, false
	}
	if opts.untilDate != "" && opts.days == 0 {
		a.plainErrorf("--until requires --days.")
		return rewriteDatesOptions{}, false
	}
	if !validRewriteDateProfileLevel(opts.frequency) {
		a.plainErrorf("--frequency must be low, medium, or high.")
		return rewriteDatesOptions{}, false
	}
	if !validRewriteDateProfileLevel(opts.spread) {
		a.plainErrorf("--spread must be low, medium, or high.")
		return rewriteDatesOptions{}, false
	}
	if opts.window != "" {
		parsed, err := parseCommitTimeSchedule(opts.window)
		if err != nil {
			a.plainErrorf("--window %s.", err.Error())
			return rewriteDatesOptions{}, false
		}
		opts.timeSchedule = &parsed
	}
	profile, _ := buildRewriteDateProfile(opts.frequency, opts.spread)
	opts.frequency = profile.frequencyName
	opts.spread = profile.spreadName
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
	baseline, _, err := loadRewriteBaseline(r.gitDir)
	if err != nil {
		return dateRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not read rewrite baseline"}
	}
	commits, err := collectRewriteDateCommits(a, r.dir, branchRefNames(branches))
	if err != nil {
		return dateRewriteScan{repo: r, hasHead: true, err: err, errLabel: "could not read commit metadata"}
	}
	if len(commits) < 2 {
		return dateRewriteScan{repo: r, hasHead: true, tooFew: true}
	}
	applyRewriteBaselineToDateCommits(baseline, commits)
	selected := []int{}
	hasSigned := false
	for i := range commits {
		if commitHasSignature(commits[i]) {
			hasSigned = true
		}
		if rewriteDateCommitSelected(commits[i], opts) {
			commits[i].selected = true
			selected = append(selected, i)
		}
	}
	return dateRewriteScan{
		repo:             r,
		hasHead:          true,
		branches:         branches,
		commits:          commits,
		selected:         selected,
		tzOffset:         dominantTimezoneOffsetFromCommits(commits),
		hasTags:          rewriteDatesRepoHasTags(a, r.dir),
		hasSignedObjects: hasSigned,
	}
}

func applyRewriteBaselineToDateCommits(manifest rewriteBaselineManifest, commits []rewriteDateCommit) {
	byCurrent := map[string]rewriteBaselineEntry{}
	byFirst := map[string]rewriteBaselineEntry{}
	for _, entry := range manifest.Entries {
		if entry.CurrentSHA != "" {
			byCurrent[entry.CurrentSHA] = entry
		}
		if entry.FirstSHA != "" {
			byFirst[entry.FirstSHA] = entry
		}
	}
	for i := range commits {
		entry, ok := byCurrent[commits[i].hash]
		if !ok {
			entry, ok = byFirst[commits[i].hash]
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
		commits[i].originalSHA = entry.FirstSHA
		commits[i].originalAuthorEpoch = entry.AuthorEpoch
		commits[i].originalAuthorTZ = entry.AuthorTZ
		commits[i].originalAuthorDate = entry.AuthorDate
		commits[i].originalCommitterEpoch = entry.CommitterEpoch
		commits[i].originalCommitterTZ = entry.CommitterTZ
		commits[i].originalCommitterDate = entry.CommitterDate
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

func rewriteDateCommitSelected(commit rewriteDateCommit, opts rewriteDatesOptions) bool {
	return opts.bounds.selectsCurrentAuthorDate(commit)
}

func planRewriteDateCandidates(candidates []dateCandidate, opts rewriteDatesOptions) (rewriteDatePlan, error) {
	profile, _ := buildRewriteDateProfile(opts.frequency, opts.spread)
	seed, seedSource := rewriteDateSeed(opts, candidates)
	selected := selectedDateCommits(candidates)
	sortSelectedDateCommits(candidates, selected)
	tzOffset := dominantPlanningTimezoneOffset(candidates, selected)
	targetStart, targetEnd, startFixed, endFixed, err := rewriteDateTargetRange(candidates, selected, opts, tzOffset)
	if err != nil {
		return rewriteDatePlan{}, err
	}
	targetStart, targetEnd, err = extendRewriteDateTargetForConstraints(candidates, selected, targetStart, targetEnd, startFixed, endFixed, tzOffset)
	if err != nil {
		return rewriteDatePlan{}, err
	}
	targetStart, targetEnd, err = extendRewriteDateTargetForChains(candidates, targetStart, targetEnd, startFixed, endFixed, tzOffset)
	if err != nil {
		return rewriteDatePlan{}, err
	}
	plannedCandidates, calendar, _, err := planRewriteDateCandidateTimestamps(candidates, selected, targetStart, targetEnd, seed, profile, tzOffset, opts.timeSchedule)
	if err != nil {
		return rewriteDatePlan{}, err
	}
	return rewriteDatePlan{
		candidates:    plannedCandidates,
		seed:          seed,
		seedSource:    seedSource,
		targetStart:   targetStart,
		targetEnd:     targetEnd,
		totalSelected: len(selected),
		profile:       profile,
		tzOffset:      tzOffset,
		calendar:      calendar,
		startFixed:    startFixed,
		endFixed:      endFixed,
	}, nil
}

func buildRewriteDateProfile(frequency, spread string) (rewriteDateProfile, bool) {
	if frequency == "" {
		frequency = "medium"
	}
	if spread == "" {
		spread = "medium"
	}
	profile := rewriteDateProfile{
		frequencyName: frequency,
		spreadName:    spread,
	}
	switch frequency {
	case "low":
		profile.frequencyDescription = "sparse activity with longer pauses"
		profile.activeRatio = 0.18
		profile.commitSpread = 0.25
		profile.demandScale = 12
		profile.weekendActivityMultiplier = 0.65
		profile.eveningChance = 0.05
		profile.sessionMinDays = 1
		profile.sessionMaxDays = 3
		profile.gapMinDays = 5
		profile.gapMaxDays = 14
	case "medium":
		profile.frequencyDescription = "clustered weekday-heavy activity"
		profile.activeRatio = 0.32
		profile.commitSpread = 0.50
		profile.demandScale = 8
		profile.weekendActivityMultiplier = 0.80
		profile.eveningChance = 0.12
		profile.sessionMinDays = 2
		profile.sessionMaxDays = 5
		profile.gapMinDays = 2
		profile.gapMaxDays = 7
	case "high":
		profile.frequencyDescription = "dense activity with shorter pauses"
		profile.activeRatio = 0.55
		profile.commitSpread = 0.72
		profile.demandScale = 5
		profile.weekendActivityMultiplier = 0.95
		profile.eveningChance = 0.24
		profile.sessionMinDays = 3
		profile.sessionMaxDays = 8
		profile.gapMinDays = 1
		profile.gapMaxDays = 4
	default:
		return rewriteDateProfile{}, false
	}
	switch spread {
	case "low":
		profile.spreadDescription = "lower daily quota variance"
		profile.daySigma = 0.35
		profile.sessionSigma = 0.20
		profile.persistence = 0.35
	case "medium":
		profile.spreadDescription = "moderate daily quota variance"
		profile.daySigma = 0.75
		profile.sessionSigma = 0.35
		profile.persistence = 0.55
	case "high":
		profile.spreadDescription = "higher daily quota variance"
		profile.daySigma = 1.00
		profile.sessionSigma = 0.50
		profile.persistence = 0.70
	default:
		return rewriteDateProfile{}, false
	}
	return profile, true
}

func validRewriteDateProfileLevel(name string) bool {
	switch name {
	case "", "low", "medium", "high":
		return true
	default:
		return false
	}
}

func rewriteDateSeed(opts rewriteDatesOptions, candidates []dateCandidate) (string, string) {
	if opts.seed != "" {
		return opts.seed, "flag"
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

func planRewriteDateCandidateTimestamps(candidates []dateCandidate, selected []selectedDateCommit, targetStart, targetEnd int64, seed string, profile rewriteDateProfile, tzOffset string, schedule *commitTimeSchedule) ([]dateCandidate, rewriteDateCalendarPlan, rewriteDateTopologyCompression, error) {
	var bestCandidates []dateCandidate
	var bestCalendar rewriteDateCalendarPlan
	var bestStats rewriteDateTopologyCompression
	var firstErr error
	hasBest := false
	effectiveRepos := effectiveSelectedRepositoryCount(candidates)
	for attempt := 0; attempt < 8; attempt++ {
		attemptCandidates := cloneDateCandidates(candidates)
		for i := range attemptCandidates {
			attemptCandidates[i].tzOffset = tzOffset
			attemptCandidates[i].topologyCompressed = false
		}
		calendarSeed := rewriteDatePlanningSeed(seed, attempt)
		calendar := buildRewriteDateCalendarPlanForRepos(len(selected), targetStart, targetEnd, calendarSeed, profile, tzOffset, effectiveRepos)
		timestamps := plannedEpochsForCalendar(calendar, len(selected), calendarSeed, profile, schedule)
		assignPlannedEpochs(attemptCandidates, selected, timestamps)
		if err := enforceRewriteDateTopologyWithSelected(attemptCandidates, selected, targetStart, targetEnd); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if schedule != nil {
			if err := verifySelectedDateSchedule(attemptCandidates, selected, *schedule); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
		}
		calendar = markCalendarDaysForPlannedCommits(calendar, attemptCandidates, selected)
		stats := rewriteDateTopologyCompressionStats(attemptCandidates, calendar, targetStart, targetEnd, len(selected))
		if !hasBest || rewriteDateCompressionLess(stats, bestStats) {
			bestCandidates = attemptCandidates
			bestCalendar = calendar
			bestStats = stats
			hasBest = true
		}
	}
	if hasBest {
		markTopologyCompressedCandidates(bestCandidates, bestStats)
		return bestCandidates, bestCalendar, bestStats, nil
	}
	if firstErr != nil {
		return nil, rewriteDateCalendarPlan{}, rewriteDateTopologyCompression{}, firstErr
	}
	return cloneDateCandidates(candidates), rewriteDateCalendarPlan{}, rewriteDateTopologyCompression{}, nil
}

func rewriteDatePlanningSeed(seed string, attempt int) string {
	if attempt == 0 {
		return seed
	}
	return seed + "\x00rewrite-dates-topology\x00" + strconv.Itoa(attempt)
}

func cloneDateCandidates(candidates []dateCandidate) []dateCandidate {
	cloned := make([]dateCandidate, len(candidates))
	copy(cloned, candidates)
	for i := range cloned {
		cloned[i].branches = append([]dateBranchRef(nil), candidates[i].branches...)
		cloned[i].commits = append([]rewriteDateCommit(nil), candidates[i].commits...)
		cloned[i].selected = append([]int(nil), candidates[i].selected...)
	}
	return cloned
}

func assignPlannedEpochs(candidates []dateCandidate, selected []selectedDateCommit, timestamps []int64) {
	for i, ref := range selected {
		if i >= len(timestamps) {
			break
		}
		commit := &candidates[ref.candidate].commits[ref.commit]
		commit.rawPlannedEpoch = timestamps[i]
		commit.plannedEpoch = timestamps[i]
	}
}

func rewriteDateCompressionLess(left, right rewriteDateTopologyCompression) bool {
	if left.forcedActiveDays != right.forcedActiveDays {
		return left.forcedActiveDays < right.forcedActiveDays
	}
	if left.compressed != right.compressed {
		return !left.compressed
	}
	if left.maxOneSecondRun != right.maxOneSecondRun {
		return left.maxOneSecondRun < right.maxOneSecondRun
	}
	if left.oneSecondEdges != right.oneSecondEdges {
		return left.oneSecondEdges < right.oneSecondEdges
	}
	if left.dailyQuotaDeviation != right.dailyQuotaDeviation {
		return left.dailyQuotaDeviation < right.dailyQuotaDeviation
	}
	return left.adjustedCommits < right.adjustedCommits
}

func effectiveSelectedRepositoryCount(candidates []dateCandidate) float64 {
	total := 0
	for _, candidate := range candidates {
		total += len(candidate.selected)
	}
	if total == 0 {
		return 1
	}
	concentration := 0.0
	for _, candidate := range candidates {
		share := float64(len(candidate.selected)) / float64(total)
		concentration += share * share
	}
	if concentration <= 0 {
		return 1
	}
	return 1 / concentration
}

func markTopologyCompressedCandidates(candidates []dateCandidate, stats rewriteDateTopologyCompression) {
	for candidateIndex := range stats.candidateCompressed {
		if candidateIndex >= 0 && candidateIndex < len(candidates) {
			candidates[candidateIndex].topologyCompressed = true
		}
	}
}

func sortSelectedDateCommits(candidates []dateCandidate, selected []selectedDateCommit) {
	sort.Slice(selected, func(i, j int) bool {
		left := candidates[selected[i].candidate].commits[selected[i].commit]
		right := candidates[selected[j].candidate].commits[selected[j].commit]
		if left.authorEpoch == right.authorEpoch {
			if candidates[selected[i].candidate].repo.display == candidates[selected[j].candidate].repo.display {
				return left.hash < right.hash
			}
			return candidates[selected[i].candidate].repo.display < candidates[selected[j].candidate].repo.display
		}
		return left.authorEpoch < right.authorEpoch
	})
}

func dominantPlanningTimezoneOffset(candidates []dateCandidate, selected []selectedDateCommit) string {
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

func rewriteDateTargetRange(candidates []dateCandidate, selected []selectedDateCommit, opts rewriteDatesOptions, tzOffset string) (int64, int64, bool, bool, error) {
	if len(selected) == 0 {
		return 0, 0, false, false, fmt.Errorf("no commits selected")
	}
	startFixed := opts.startDate != "" || opts.days > 0
	endFixed := opts.endDate != "" || opts.days > 0
	if opts.days > 0 {
		until := opts.untilDate
		if until == "" {
			until = time.Now().In(locationForTimezoneOffset(tzOffset)).Format("2006-01-02")
		}
		end := parseDateEndInOffset(until, tzOffset)
		start := parseDateStartInOffset(time.Unix(end, 0).In(locationForTimezoneOffset(tzOffset)).AddDate(0, 0, -(opts.days-1)).Format("2006-01-02"), tzOffset)
		return start, end, true, true, nil
	}
	minCurrent := int64(0)
	maxCurrent := int64(0)
	for i, ref := range selected {
		epoch := candidates[ref.candidate].commits[ref.commit].authorEpoch
		if i == 0 || epoch < minCurrent {
			minCurrent = epoch
		}
		if i == 0 || epoch > maxCurrent {
			maxCurrent = epoch
		}
	}
	start := minCurrent
	end := maxCurrent
	if opts.startDate != "" {
		start = parseDateStartInOffset(opts.startDate, tzOffset)
	}
	if opts.endDate != "" {
		end = parseDateEndInOffset(opts.endDate, tzOffset)
	}
	if start > end {
		return 0, 0, startFixed, endFixed, fmt.Errorf("target start date must be on or before target end date")
	}
	return start, end, startFixed, endFixed, nil
}

func extendRewriteDateTargetForConstraints(candidates []dateCandidate, selected []selectedDateCommit, targetStart, targetEnd int64, startFixed, endFixed bool, tzOffset string) (int64, int64, error) {
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
					return 0, 0, fmt.Errorf("%s %s needs a target date after fixed parent %s", candidate.repo.display, prefix(commit.hash, 8), formatEpoch(minFixed-1, tzOffset))
				}
				targetEnd = minFixed
				changed = true
			}
			if maxFixed < targetStart {
				if startFixed {
					return 0, 0, fmt.Errorf("%s %s needs a target date before fixed child %s", candidate.repo.display, prefix(commit.hash, 8), formatEpoch(maxFixed+1, tzOffset))
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

func extendRewriteDateTargetForChains(candidates []dateCandidate, targetStart, targetEnd int64, startFixed, endFixed bool, tzOffset string) (int64, int64, error) {
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
	return 0, 0, fmt.Errorf("target range %s -> %s is too narrow for selected parent/child ordering", formatEpoch(targetStart, tzOffset), formatEpoch(targetEnd, tzOffset))
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
		minFixed = maxInt64(minFixed, candidate.commits[parentIndex].authorEpoch+1)
	}
	children := childCommitIndexes(candidate.commits)
	for _, childIndex := range children[commitIndex] {
		if selected[childIndex] {
			continue
		}
		maxFixed = minInt64(maxFixed, candidate.commits[childIndex].authorEpoch-1)
	}
	return minFixed, maxFixed
}


func enforceRewriteDateTopologyWithSelected(candidates []dateCandidate, selectedRefs []selectedDateCommit, targetStart, targetEnd int64) error {
	limit := totalRewriteDateCommits(candidates) + len(selectedRefs) + 2
	for pass := 0; pass < limit; pass++ {
		before := selectedPlannedEpochs(candidates, selectedRefs)
		if err := enforceRewriteDateRepositoryTopology(candidates, targetStart, targetEnd); err != nil {
			return err
		}
		if err := enforceGlobalSelectedDateOrder(candidates, selectedRefs, targetStart, targetEnd); err != nil {
			return err
		}
		after := selectedPlannedEpochs(candidates, selectedRefs)
		if int64SlicesEqual(before, after) {
			if err := verifyGlobalSelectedDateOrder(candidates, selectedRefs); err != nil {
				return err
			}
			return nil
		}
	}
	if err := verifyGlobalSelectedDateOrder(candidates, selectedRefs); err != nil {
		return err
	}
	return fmt.Errorf("could not satisfy date topology constraints")
}

func enforceRewriteDateRepositoryTopology(candidates []dateCandidate, targetStart, targetEnd int64) error {
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
			}
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
			}
			if !changed {
				break
			}
		}
		for _, idx := range candidate.selected {
			if candidate.commits[idx].plannedEpoch < mins[idx] {
				return fmt.Errorf("%s %s needs to be before its selected child but outside the target range", candidate.repo.display, prefix(candidate.commits[idx].hash, 8))
			}
			if candidate.commits[idx].plannedEpoch > maxes[idx] {
				return fmt.Errorf("%s %s needs to be after its selected parent but outside the target range", candidate.repo.display, prefix(candidate.commits[idx].hash, 8))
			}
		}
		if err := verifySelectedTopology(*candidate); err != nil {
			return err
		}
	}
	return nil
}

func enforceGlobalSelectedDateOrder(candidates []dateCandidate, selectedRefs []selectedDateCommit, targetStart, targetEnd int64) error {
	previous := int64(math.MinInt64 / 2)
	for _, ref := range selectedRefs {
		candidate := &candidates[ref.candidate]
		commit := &candidate.commits[ref.commit]
		minFixed, maxFixed := fixedDateConstraints(*candidate, ref.commit)
		minAllowed := maxInt64(targetStart, minFixed)
		maxAllowed := minInt64(targetEnd, maxFixed)
		if minAllowed > maxAllowed {
			return fmt.Errorf("%s %s cannot fit between fixed neighboring commits", candidate.repo.display, prefix(commit.hash, 8))
		}
		if commit.plannedEpoch < minAllowed {
			commit.plannedEpoch = minAllowed
		}
		if commit.plannedEpoch < previous {
			commit.plannedEpoch = previous
		}
		if commit.plannedEpoch > maxAllowed {
			return fmt.Errorf("%s %s needs to preserve global commit order but is outside the target range", candidate.repo.display, prefix(commit.hash, 8))
		}
		previous = commit.plannedEpoch
	}
	return nil
}

func verifyGlobalSelectedDateOrder(candidates []dateCandidate, selectedRefs []selectedDateCommit) error {
	for i := 1; i < len(selectedRefs); i++ {
		left := candidates[selectedRefs[i-1].candidate].commits[selectedRefs[i-1].commit]
		right := candidates[selectedRefs[i].candidate].commits[selectedRefs[i].commit]
		if right.plannedEpoch < left.plannedEpoch {
			return fmt.Errorf("%s is planned before globally earlier commit %s", prefix(right.hash, 8), prefix(left.hash, 8))
		}
	}
	return nil
}

func verifySelectedDateSchedule(candidates []dateCandidate, selectedRefs []selectedDateCommit, schedule commitTimeSchedule) error {
	for _, ref := range selectedRefs {
		candidate := candidates[ref.candidate]
		commit := candidate.commits[ref.commit]
		day := floorDayInOffset(commit.plannedEpoch, candidate.tzOffset)
		window, ok := schedule.windowForDay(day, candidate.tzOffset)
		if !ok {
			continue
		}
		if !epochInCommitTimeWindow(commit.plannedEpoch, candidate.tzOffset, window) {
			return fmt.Errorf("%s %s cannot fit inside --window %s while preserving topology", candidate.repo.display, prefix(commit.hash, 8), schedule.Text)
		}
	}
	return nil
}

func selectedPlannedEpochs(candidates []dateCandidate, selectedRefs []selectedDateCommit) []int64 {
	epochs := make([]int64, len(selectedRefs))
	for i, ref := range selectedRefs {
		epochs[i] = candidates[ref.candidate].commits[ref.commit].plannedEpoch
	}
	return epochs
}

func int64SlicesEqual(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func totalRewriteDateCommits(candidates []dateCandidate) int {
	total := 0
	for _, candidate := range candidates {
		total += len(candidate.commits)
	}
	return total
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

func rewriteDateTopologyCompressionStats(candidates []dateCandidate, calendar rewriteDateCalendarPlan, targetStart, targetEnd int64, totalSelected int) rewriteDateTopologyCompression {
	stats := rewriteDateTopologyCompression{forcedActiveDays: calendarForcedActiveDayCount(calendar), candidateCompressed: map[int]bool{}}
	stats.dailyQuotaDeviation = calendarDailyQuotaDeviation(calendar, candidates)
	exactRun := 0
	edges := rewriteDateSelectedParentChildEdges(candidates)
	for _, edge := range edges {
		stats.selectedEdges++
		if edge.gap == 1 {
			stats.oneSecondEdges++
			exactRun++
			if exactRun > stats.maxOneSecondRun {
				stats.maxOneSecondRun = exactRun
			}
			stats.candidateCompressed[edge.candidate] = true
			continue
		}
		exactRun = 0
	}
	for _, candidate := range candidates {
		selected := selectedIndexSet(candidate.selected)
		for i, commit := range candidate.commits {
			if selected[i] && commit.rawPlannedEpoch != commit.plannedEpoch {
				stats.adjustedCommits++
			}
		}
	}
	rangeAtLeastOneDay := targetEnd-targetStart >= 86400
	edgeRatioCompressed := stats.selectedEdges > 0 && float64(stats.oneSecondEdges)/float64(stats.selectedEdges) > 0.25
	stats.compressed = rangeAtLeastOneDay && totalSelected >= 8 && (stats.maxOneSecondRun >= 4 || edgeRatioCompressed)
	if !stats.compressed {
		stats.candidateCompressed = map[int]bool{}
	}
	return stats
}

func calendarDailyQuotaDeviation(calendar rewriteDateCalendarPlan, candidates []dateCandidate) int {
	actual := plannedDailyCommitCounts(candidates, calendar.tzOffset)
	deviation := 0
	for _, day := range calendar.days {
		if !calendarDayHasSlots(day.state) {
			continue
		}
		deviation += absInt(actual[day.epoch] - maxInt(1, day.quota))
		delete(actual, day.epoch)
	}
	for _, count := range actual {
		deviation += count
	}
	return deviation
}

func plannedDailyCommitCounts(candidates []dateCandidate, tzOffset string) map[int64]int {
	counts := map[int64]int{}
	for _, candidate := range candidates {
		for _, idx := range candidate.selected {
			counts[floorDayInOffset(candidate.commits[idx].plannedEpoch, tzOffset)]++
		}
	}
	return counts
}

type rewriteDateSelectedEdge struct {
	candidate int
	parent    int64
	child     int64
	gap       int64
}

func rewriteDateSelectedParentChildEdges(candidates []dateCandidate) []rewriteDateSelectedEdge {
	edges := []rewriteDateSelectedEdge{}
	for candidateIndex, candidate := range candidates {
		selected := selectedIndexSet(candidate.selected)
		byHash := commitIndexByHash(candidate.commits)
		for childIndex, commit := range candidate.commits {
			if !selected[childIndex] {
				continue
			}
			for _, parent := range commit.parents {
				parentIndex, ok := byHash[parent]
				if !ok || !selected[parentIndex] {
					continue
				}
				parentEpoch := candidate.commits[parentIndex].plannedEpoch
				childEpoch := commit.plannedEpoch
				edges = append(edges, rewriteDateSelectedEdge{
					candidate: candidateIndex,
					parent:    parentEpoch,
					child:     childEpoch,
					gap:       childEpoch - parentEpoch,
				})
			}
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].child == edges[j].child {
			if edges[i].parent == edges[j].parent {
				return edges[i].candidate < edges[j].candidate
			}
			return edges[i].parent < edges[j].parent
		}
		return edges[i].child < edges[j].child
	})
	return edges
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



func buildRewriteDateCalendarPlanForRepos(selectedCount int, startEpoch, endEpoch int64, seed string, profile rewriteDateProfile, tzOffset string, effectiveRepos float64) rewriteDateCalendarPlan {
	if startEpoch > endEpoch {
		startEpoch, endEpoch = endEpoch, startEpoch
	}
	rng := rand.New(rand.NewSource(seedInt64(seed)))
	days := daysInRangeInOffset(startEpoch, endEpoch, tzOffset)
	if len(days) == 0 {
		days = []int64{floorDayInOffset(startEpoch, tzOffset)}
	}
	restBlocks := syntheticRestBlocks(days, selectedCount, profile, rng, tzOffset)
	restDays := restDaySet(restBlocks)
	calendar := rewriteDateCalendarPlan{
		days:        make([]rewriteDateCalendarDay, len(days)),
		restBlocks:  restBlocks,
		tzOffset:    tzOffset,
		targetStart: startEpoch,
		targetEnd:   endEpoch,
	}
	for i, day := range days {
		state := rewriteDateCalendarInactive
		if restDays[day] {
			state = rewriteDateCalendarRest
		}
		calendar.days[i] = rewriteDateCalendarDay{epoch: day, state: state}
	}
	if selectedCount <= 0 {
		return calendar
	}
	activeTarget := plannedCalendarActiveDayCount(selectedCount, len(days), calendarRestDayCount(calendar), profile)
	if selectedCount <= 3 && len(days) > selectedCount+2 {
		activeTarget = minInt(selectedCount, activeTarget)
		if activeTarget < 1 {
			activeTarget = 1
		}
		activateSmallSelectionCalendarDays(&calendar, activeTarget, profile, rng)
	} else {
		activateCalendarSessions(&calendar, activeTarget, profile, rng)
	}
	ensureCalendarHasActiveDay(&calendar, profile, rng)
	assignCalendarDailyQuotas(&calendar, selectedCount, profile, effectiveRepos, rng)
	return calendar
}

func plannedEpochsForCalendar(calendar rewriteDateCalendarPlan, n int, seed string, profile rewriteDateProfile, schedule *commitTimeSchedule) []int64 {
	if n <= 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(seedInt64(seed + "\x00rewrite-dates-slots")))
	activeDays := activeCalendarDays(calendar)
	if len(activeDays) == 0 {
		days := daysInRangeInOffset(calendar.targetStart, calendar.targetEnd, calendar.tzOffset)
		if len(days) == 0 {
			days = []int64{floorDayInOffset(calendar.targetStart, calendar.tzOffset)}
		}
		day := days[0]
		if len(days) > 2 {
			day = days[1+rng.Intn(len(days)-2)]
		}
		activeDays = []rewriteDateCalendarDay{{epoch: day, state: rewriteDateCalendarActive, quota: n}}
	}
	allocations := allocateCommitsToCalendarDays(n, activeDays, rng)
	timestamps := make([]int64, n)
	slotIndex := 0
	for i, day := range activeDays {
		count := allocations[i]
		if count <= 0 {
			continue
		}
		dayTimes := plannedEpochsForDay(day.epoch, count, calendar.targetStart, calendar.targetEnd, rng, profile, calendar.tzOffset, schedule)
		for _, ts := range dayTimes {
			if slotIndex >= len(timestamps) {
				break
			}
			timestamps[slotIndex] = ts
			slotIndex++
		}
	}
	for slotIndex < len(timestamps) {
		timestamps[slotIndex] = clampInt64(calendar.targetEnd-int64(len(timestamps)-slotIndex-1), calendar.targetStart, calendar.targetEnd)
		slotIndex++
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	return timestamps
}

func plannedCalendarActiveDayCount(selectedCount, dayCount, restDayCount int, profile rewriteDateProfile) int {
	if selectedCount <= 0 || dayCount <= 0 {
		return 0
	}
	nonRestDays := dayCount - restDayCount
	if nonRestDays < 1 {
		nonRestDays = 1
	}
	demand := float64(selectedCount) / float64(nonRestDays)
	activeFraction := profile.activeRatio + (1-profile.activeRatio)*(1-math.Exp(-demand/profile.demandScale))
	densityTarget := int(math.Round(float64(nonRestDays) * activeFraction))
	spreadTarget := int(math.Ceil(float64(selectedCount) * profile.commitSpread))
	active := minInt(nonRestDays, selectedCount)
	active = minInt(active, spreadTarget)
	active = minInt(active, densityTarget)
	if active < 1 {
		active = 1
	}
	if active > nonRestDays {
		active = nonRestDays
	}
	if active < 1 {
		active = 1
	}
	return active
}

func activateSmallSelectionCalendarDays(calendar *rewriteDateCalendarPlan, activeTarget int, profile rewriteDateProfile, rng *rand.Rand) {
	if activeTarget <= 0 {
		return
	}
	eligible := calendarDayIndexesByState(*calendar, false, rewriteDateCalendarInactive)
	if len(calendar.days) > activeTarget+2 {
		trimmed := make([]int, 0, len(eligible))
		for _, idx := range eligible {
			if idx > 0 && idx < len(calendar.days)-1 {
				trimmed = append(trimmed, idx)
			}
		}
		if len(trimmed) > 0 {
			eligible = trimmed
		}
	}
	activateWeightedCalendarDays(calendar, eligible, activeTarget, profile, rng)
}

func activateCalendarSessions(calendar *rewriteDateCalendarPlan, activeTarget int, profile rewriteDateProfile, rng *rand.Rand) {
	if activeTarget <= 0 {
		return
	}
	if len(calendar.days) == 0 {
		return
	}
	sessionLengths := calendarSessionLengths(activeTarget, profile, rng)
	anchors := make([]int, len(sessionLengths))
	for i := range sessionLengths {
		windowStart := i * len(calendar.days) / len(sessionLengths)
		windowEnd := ((i + 1) * len(calendar.days) / len(sessionLengths)) - 1
		if windowEnd < windowStart {
			windowEnd = windowStart
		}
		anchor := calendarSessionAnchorInWindow(calendar, windowStart, windowEnd, profile, rng)
		anchors[i] = anchor
		if anchor < 0 {
			continue
		}
		calendar.days[anchor].state = rewriteDateCalendarActive
	}
	for i, anchor := range anchors {
		if anchor >= 0 {
			continue
		}
		windowStart := i * len(calendar.days) / len(sessionLengths)
		windowEnd := ((i + 1) * len(calendar.days) / len(sessionLengths)) - 1
		anchor = nearestCalendarSessionAnchor(calendar, windowStart, windowEnd, profile, rng)
		anchors[i] = anchor
		if anchor >= 0 {
			calendar.days[anchor].state = rewriteDateCalendarActive
		}
	}
	for i, anchor := range anchors {
		if anchor < 0 {
			continue
		}
		growCalendarSession(calendar, anchor, sessionLengths[i]-1, profile, rng)
	}
	if calendarActiveDayCount(*calendar) < activeTarget {
		eligible := calendarDayIndexesByState(*calendar, true, rewriteDateCalendarInactive)
		activateWeightedCalendarDays(calendar, eligible, activeTarget-calendarActiveDayCount(*calendar), profile, rng)
	}
}

func calendarSessionLengths(activeTarget int, profile rewriteDateProfile, rng *rand.Rand) []int {
	lengths := []int{}
	for remaining := activeTarget; remaining > 0; {
		minLength := maxInt(1, profile.sessionMinDays)
		maxLength := maxInt(minLength, profile.sessionMaxDays)
		if minLength > remaining {
			minLength = remaining
		}
		if maxLength > remaining {
			maxLength = remaining
		}
		length := randomIntBetween(rng, minLength, maxLength)
		if profile.gapMaxDays > 0 {
			shortGapBiasSamples := maxInt(0, (maxLength-profile.gapMaxDays)/2)
			for sample := 0; sample < shortGapBiasSamples; sample++ {
				length = minInt(length, randomIntBetween(rng, minLength, maxLength))
			}
		}
		lengths = append(lengths, length)
		remaining -= length
	}
	return lengths
}

func calendarSessionAnchorInWindow(calendar *rewriteDateCalendarPlan, windowStart, windowEnd int, profile rewriteDateProfile, rng *rand.Rand) int {
	if len(calendar.days) == 0 {
		return -1
	}
	windowStart = maxInt(0, minInt(windowStart, len(calendar.days)-1))
	windowEnd = maxInt(windowStart, minInt(windowEnd, len(calendar.days)-1))
	windowInset := (windowEnd - windowStart + 1) / 4
	centerCandidates := inactiveCalendarDayIndexesInWindow(*calendar, windowStart+windowInset, windowEnd-windowInset)
	centerCandidates = trimCalendarEdgeIndexes(centerCandidates, len(calendar.days))
	if len(centerCandidates) > 0 {
		return centerCandidates[weightedCalendarIndex(calendar, centerCandidates, profile, rng)]
	}
	windowCandidates := inactiveCalendarDayIndexesInWindow(*calendar, windowStart, windowEnd)
	windowCandidates = trimCalendarEdgeIndexes(windowCandidates, len(calendar.days))
	if len(windowCandidates) > 0 {
		return windowCandidates[weightedCalendarIndex(calendar, windowCandidates, profile, rng)]
	}
	return -1
}

func nearestCalendarSessionAnchor(calendar *rewriteDateCalendarPlan, windowStart, windowEnd int, profile rewriteDateProfile, rng *rand.Rand) int {
	if len(calendar.days) == 0 {
		return -1
	}
	windowStart = maxInt(0, minInt(windowStart, len(calendar.days)-1))
	windowEnd = maxInt(windowStart, minInt(windowEnd, len(calendar.days)-1))
	allCandidates := inactiveCalendarDayIndexesInWindow(*calendar, 0, len(calendar.days)-1)
	allCandidates = trimCalendarEdgeIndexes(allCandidates, len(calendar.days))
	if len(allCandidates) == 0 {
		return -1
	}
	nearest := allCandidates[:0]
	nearestDistance := len(calendar.days) + 1
	for _, idx := range allCandidates {
		distance := distanceFromCalendarWindow(idx, windowStart, windowEnd)
		if distance < nearestDistance {
			nearestDistance = distance
			nearest = nearest[:0]
		}
		if distance == nearestDistance {
			nearest = append(nearest, idx)
		}
	}
	return nearest[weightedCalendarIndex(calendar, nearest, profile, rng)]
}

func inactiveCalendarDayIndexesInWindow(calendar rewriteDateCalendarPlan, windowStart, windowEnd int) []int {
	if len(calendar.days) == 0 {
		return nil
	}
	windowStart = maxInt(0, minInt(windowStart, len(calendar.days)-1))
	windowEnd = maxInt(windowStart, minInt(windowEnd, len(calendar.days)-1))
	indexes := []int{}
	for i := windowStart; i <= windowEnd; i++ {
		if calendar.days[i].state == rewriteDateCalendarInactive {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func trimCalendarEdgeIndexes(indexes []int, dayCount int) []int {
	if dayCount <= 2 {
		return indexes
	}
	trimmed := make([]int, 0, len(indexes))
	for _, idx := range indexes {
		if idx > 0 && idx < dayCount-1 {
			trimmed = append(trimmed, idx)
		}
	}
	if len(trimmed) == 0 {
		return indexes
	}
	return trimmed
}

func distanceFromCalendarWindow(index, windowStart, windowEnd int) int {
	if index < windowStart {
		return windowStart - index
	}
	if index > windowEnd {
		return index - windowEnd
	}
	return 0
}

func growCalendarSession(calendar *rewriteDateCalendarPlan, anchor, additionalDays int, profile rewriteDateProfile, rng *rand.Rand) {
	if additionalDays <= 0 || anchor < 0 || anchor >= len(calendar.days) {
		return
	}
	left := anchor - 1
	right := anchor + 1
	leftOpen := left >= 0
	rightOpen := right < len(calendar.days)
	for additionalDays > 0 && (leftOpen || rightOpen) {
		side := chooseCalendarSessionGrowthSide(calendar, left, leftOpen, right, rightOpen, profile, rng)
		if side < 0 {
			day := calendar.days[left]
			if day.state == rewriteDateCalendarRest {
				leftOpen = false
				continue
			}
			if day.state == rewriteDateCalendarInactive && calendarSessionAcceptsDay(day.epoch, profile, rng, calendar.tzOffset) {
				calendar.days[left].state = rewriteDateCalendarActive
				additionalDays--
			}
			left--
			leftOpen = left >= 0
			continue
		}
		day := calendar.days[right]
		if day.state == rewriteDateCalendarRest {
			rightOpen = false
			continue
		}
		if day.state == rewriteDateCalendarInactive && calendarSessionAcceptsDay(day.epoch, profile, rng, calendar.tzOffset) {
			calendar.days[right].state = rewriteDateCalendarActive
			additionalDays--
		}
		right++
		rightOpen = right < len(calendar.days)
	}
}

func chooseCalendarSessionGrowthSide(calendar *rewriteDateCalendarPlan, left int, leftOpen bool, right int, rightOpen bool, profile rewriteDateProfile, rng *rand.Rand) int {
	if leftOpen && !rightOpen {
		return -1
	}
	if rightOpen && !leftOpen {
		return 1
	}
	if !leftOpen && !rightOpen {
		return 0
	}
	leftWeight := planningDayWeight(calendar.days[left].epoch, nil, profile, rng, calendar.tzOffset)
	rightWeight := planningDayWeight(calendar.days[right].epoch, nil, profile, rng, calendar.tzOffset)
	if rng.Float64()*(leftWeight+rightWeight) < leftWeight {
		return -1
	}
	return 1
}

func calendarSessionAcceptsDay(day int64, profile rewriteDateProfile, rng *rand.Rand, tzOffset string) bool {
	if !isWeekendInOffset(day, tzOffset) {
		return true
	}
	return rng.Float64() < profile.weekendActivityMultiplier
}

func activateWeightedCalendarDays(calendar *rewriteDateCalendarPlan, indexes []int, count int, profile rewriteDateProfile, rng *rand.Rand) int {
	activated := 0
	for count > 0 && len(indexes) > 0 {
		position := weightedCalendarIndex(calendar, indexes, profile, rng)
		idx := indexes[position]
		if calendar.days[idx].state == rewriteDateCalendarInactive {
			calendar.days[idx].state = rewriteDateCalendarActive
			activated++
			count--
		}
		indexes = append(indexes[:position], indexes[position+1:]...)
	}
	return activated
}

func weightedCalendarIndex(calendar *rewriteDateCalendarPlan, indexes []int, profile rewriteDateProfile, rng *rand.Rand) int {
	total := 0.0
	weights := make([]float64, len(indexes))
	for i, idx := range indexes {
		weight := planningDayWeight(calendar.days[idx].epoch, nil, profile, rng, calendar.tzOffset)
		weights[i] = weight
		total += weight
	}
	if total <= 0 {
		return rng.Intn(len(indexes))
	}
	pick := rng.Float64() * total
	for i, weight := range weights {
		pick -= weight
		if pick <= 0 {
			return i
		}
	}
	return len(indexes) - 1
}

func ensureCalendarHasActiveDay(calendar *rewriteDateCalendarPlan, profile rewriteDateProfile, rng *rand.Rand) {
	if calendarActiveDayCount(*calendar) == 0 {
		if activateWeightedCalendarDays(calendar, calendarDayIndexesByState(*calendar, true, rewriteDateCalendarInactive), 1, profile, rng) == 0 {
			forceWeightedRestCalendarDay(calendar, profile, rng)
		}
	}
}

func assignCalendarDailyQuotas(calendar *rewriteDateCalendarPlan, selectedCount int, profile rewriteDateProfile, effectiveRepos float64, rng *rand.Rand) {
	activeIndexes := make([]int, 0, calendarActiveDayCount(*calendar))
	for i, day := range calendar.days {
		if calendarDayHasSlots(day.state) {
			activeIndexes = append(activeIndexes, i)
		}
	}
	if len(activeIndexes) == 0 || selectedCount <= 0 {
		return
	}
	weights := workloadWeights(*calendar, activeIndexes, profile, effectiveRepos, rng)
	quotas := proportionalDailyQuotas(selectedCount, weights, rng)
	for i, idx := range activeIndexes {
		calendar.days[idx].quota = quotas[i]
	}
}

func workloadWeights(calendar rewriteDateCalendarPlan, activeIndexes []int, profile rewriteDateProfile, effectiveRepos float64, rng *rand.Rand) []float64 {
	if effectiveRepos < 1 {
		effectiveRepos = 1
	}
	damp := 0.65 + 0.35/math.Sqrt(effectiveRepos)
	daySigma := profile.daySigma * damp
	sessionSigma := profile.sessionSigma * damp
	persistence := profile.persistence * damp
	weights := make([]float64, len(activeIndexes))
	previousIndex := -2
	ar := 0.0
	sessionMultiplier := 1.0
	for i, idx := range activeIndexes {
		if idx != previousIndex+1 {
			sessionMultiplier = math.Exp(rng.NormFloat64()*sessionSigma - sessionSigma*sessionSigma/2)
			ar = rng.NormFloat64()
		} else {
			ar = persistence*ar + math.Sqrt(1-persistence*persistence)*rng.NormFloat64()
		}
		weight := sessionMultiplier * math.Exp(ar*daySigma-daySigma*daySigma/2)
		weights[i] = math.Max(weight, 0.000001)
		previousIndex = idx
	}
	return weights
}

func proportionalDailyQuotas(commitCount int, weights []float64, rng *rand.Rand) []int {
	quotas := make([]int, len(weights))
	if commitCount <= 0 || len(weights) == 0 {
		return quotas
	}
	active := minInt(commitCount, len(weights))
	for i := 0; i < active; i++ {
		quotas[i] = 1
	}
	remaining := commitCount - active
	if remaining <= 0 {
		return quotas
	}
	totalWeight := 0.0
	for i := 0; i < active; i++ {
		totalWeight += weights[i]
	}
	if totalWeight <= 0 {
		totalWeight = float64(active)
		for i := 0; i < active; i++ {
			weights[i] = 1
		}
	}
	type remainder struct {
		index int
		value float64
		tie   float64
	}
	remainders := make([]remainder, 0, active)
	allocated := 0
	for i := 0; i < active; i++ {
		exact := float64(remaining) * weights[i] / totalWeight
		floor := int(math.Floor(exact))
		quotas[i] += floor
		allocated += floor
		remainders = append(remainders, remainder{index: i, value: exact - float64(floor), tie: rng.Float64()})
	}
	sort.Slice(remainders, func(i, j int) bool {
		if remainders[i].value == remainders[j].value {
			return remainders[i].tie < remainders[j].tie
		}
		return remainders[i].value > remainders[j].value
	})
	for i := 0; i < remaining-allocated; i++ {
		quotas[remainders[i].index]++
	}
	return quotas
}

func forceWeightedRestCalendarDay(calendar *rewriteDateCalendarPlan, profile rewriteDateProfile, rng *rand.Rand) int {
	indexes := calendarDayIndexesByState(*calendar, false, rewriteDateCalendarRest)
	if len(indexes) == 0 {
		return 0
	}
	position := weightedCalendarIndex(calendar, indexes, profile, rng)
	idx := indexes[position]
	calendar.days[idx].state = rewriteDateCalendarForcedActive
	calendar.days[idx].quota = 1
	return 1
}

func calendarDayIndexesByState(calendar rewriteDateCalendarPlan, excludeEdges bool, states ...rewriteDateCalendarDayState) []int {
	allowed := map[rewriteDateCalendarDayState]bool{}
	for _, state := range states {
		allowed[state] = true
	}
	indexes := []int{}
	for i, day := range calendar.days {
		if excludeEdges && len(calendar.days) > 2 && (i == 0 || i == len(calendar.days)-1) {
			continue
		}
		if allowed[day.state] {
			indexes = append(indexes, i)
		}
	}
	if len(indexes) == 0 && excludeEdges {
		return calendarDayIndexesByState(calendar, false, states...)
	}
	return indexes
}

func activeCalendarDays(calendar rewriteDateCalendarPlan) []rewriteDateCalendarDay {
	days := []rewriteDateCalendarDay{}
	for _, day := range calendar.days {
		if calendarDayHasSlots(day.state) {
			if day.quota < 1 {
				day.quota = 1
			}
			days = append(days, day)
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i].epoch < days[j].epoch })
	return days
}

func calendarDayHasSlots(state rewriteDateCalendarDayState) bool {
	return state == rewriteDateCalendarActive || state == rewriteDateCalendarForcedActive
}


func calendarActiveDayCount(calendar rewriteDateCalendarPlan) int {
	count := 0
	for _, day := range calendar.days {
		if calendarDayHasSlots(day.state) {
			count++
		}
	}
	return count
}

func calendarRestDayCount(calendar rewriteDateCalendarPlan) int {
	count := 0
	for _, day := range calendar.days {
		if day.state == rewriteDateCalendarRest {
			count++
		}
	}
	return count
}

func calendarForcedActiveDayCount(calendar rewriteDateCalendarPlan) int {
	count := 0
	for _, day := range calendar.days {
		if day.state == rewriteDateCalendarForcedActive {
			count++
		}
	}
	return count
}

func markCalendarDaysForPlannedCommits(calendar rewriteDateCalendarPlan, candidates []dateCandidate, selected []selectedDateCommit) rewriteDateCalendarPlan {
	byDay := map[int64]int{}
	for i, day := range calendar.days {
		byDay[day.epoch] = i
	}
	for _, ref := range selected {
		plannedDay := floorDayInOffset(candidates[ref.candidate].commits[ref.commit].plannedEpoch, calendar.tzOffset)
		idx, ok := byDay[plannedDay]
		if !ok {
			continue
		}
		switch calendar.days[idx].state {
		case rewriteDateCalendarRest:
			calendar.days[idx].state = rewriteDateCalendarForcedActive
			if calendar.days[idx].quota < 1 {
				calendar.days[idx].quota = 1
			}
		case rewriteDateCalendarInactive:
			calendar.days[idx].state = rewriteDateCalendarActive
			if calendar.days[idx].quota < 1 {
				calendar.days[idx].quota = 1
			}
		}
	}
	return calendar
}

func allocateCommitsToCalendarDays(commitCount int, days []rewriteDateCalendarDay, rng *rand.Rand) []int {
	allocations := make([]int, len(days))
	if commitCount <= 0 || len(days) == 0 {
		return allocations
	}
	quotaTotal := 0
	weights := make([]float64, len(days))
	for i, day := range days {
		allocations[i] = maxInt(1, day.quota)
		quotaTotal += allocations[i]
		weights[i] = float64(allocations[i])
	}
	if quotaTotal == commitCount {
		return allocations
	}
	return proportionalDailyQuotas(commitCount, weights, rng)
}

func randomIntBetween(rng *rand.Rand, minValue, maxValue int) int {
	if minValue > maxValue {
		minValue, maxValue = maxValue, minValue
	}
	if minValue < 0 {
		minValue = 0
	}
	if maxValue <= minValue {
		return minValue
	}
	return minValue + rng.Intn(maxValue-minValue+1)
}

func planningDayWeight(day int64, restDays map[int64]bool, profile rewriteDateProfile, rng *rand.Rand, tzOffset string) float64 {
	weight := 1.0
	if isWeekendInOffset(day, tzOffset) {
		weight = profile.weekendActivityMultiplier
	}
	if restDays != nil && restDays[day] {
		weight *= 0.015
	}
	return weight * (0.75 + rng.Float64()*0.5)
}

func plannedEpochsForDay(day int64, count int, startEpoch, endEpoch int64, rng *rand.Rand, profile rewriteDateProfile, tzOffset string, schedule *commitTimeSchedule) []int64 {
	if count <= 0 {
		return nil
	}
	if schedule != nil {
		if window, ok := schedule.windowForDay(day, tzOffset); ok {
			return plannedEpochsForExplicitWindow(day, count, startEpoch, endEpoch, window)
		}
	}
	dayStart := maxInt64(day, startEpoch)
	dayEnd := minInt64(day+86399, endEpoch)
	if dayEnd < dayStart {
		dayStart = maxInt64(minInt64(day, endEpoch), startEpoch)
		dayEnd = dayStart
	}
	if utcStart, utcEnd, ok := sameUTCDateWindowForPlanningDay(day, tzOffset); ok {
		safeStart := maxInt64(dayStart, utcStart)
		safeEnd := minInt64(dayEnd, utcEnd)
		if safeStart <= safeEnd {
			dayStart = safeStart
			dayEnd = safeEnd
		}
	}
	weekend := isWeekendInOffset(day, tzOffset)
	startHour := int64(8 + rng.Intn(3))
	endHour := int64(17 + rng.Intn(2))
	if weekend {
		startHour = int64(10 + rng.Intn(3))
		endHour = int64(14 + rng.Intn(4))
	}
	if rng.Float64() < profile.eveningChance {
		endHour = int64(20 + rng.Intn(4))
	}
	workStart := day + startHour*3600 + int64(rng.Intn(45))*60
	workEnd := day + endHour*3600 + int64(rng.Intn(45))*60
	expansion := 1 - math.Exp(-float64(maxInt(0, count-1))/24)
	workStart -= int64(float64(workStart-dayStart) * expansion)
	workEnd += int64(float64(dayEnd-workEnd) * expansion)
	if workEnd < workStart {
		workEnd = workStart
	}
	if workStart < dayStart {
		workStart = dayStart
	}
	if workEnd > dayEnd {
		workEnd = dayEnd
	}
	if workEnd < workStart {
		workStart = dayStart
		workEnd = dayEnd
	}
	timestamps := make([]int64, count)
	if count == 1 {
		timestamps[0] = clampInt64(workStart+(workEnd-workStart)/2+int64(rng.Intn(3600))-1800, dayStart, dayEnd)
		return timestamps
	}
	spacing := int64(1)
	available := workEnd - workStart
	if available > int64(count-1) {
		spacing = available / int64(count-1)
	}
	for i := 0; i < count; i++ {
		jitter := int64(0)
		if spacing > 180 {
			jitter = int64(rng.Intn(int(minInt64(spacing/3, 1200))))
		}
		timestamps[i] = clampInt64(workStart+int64(i)*spacing+jitter, dayStart, dayEnd)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	return timestamps
}

func sameUTCDateWindowForPlanningDay(day int64, tzOffset string) (int64, int64, bool) {
	loc := locationForTimezoneOffset(tzOffset)
	local := time.Unix(day, 0).In(loc)
	utcStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC).Unix()
	utcEnd := utcStart + 86399
	localStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).Unix()
	localEnd := localStart + 86399
	start := maxInt64(localStart, utcStart)
	end := minInt64(localEnd, utcEnd)
	return start, end, start <= end
}

func syntheticRestBlocks(days []int64, selectedCount int, profile rewriteDateProfile, rng *rand.Rand, tzOffset string) []rewriteDateRestBlock {
	policy := rewriteDateRestPolicyForRange(len(days), selectedCount)
	if len(days) == 0 || (!policy.sparseInactiveDays && policy.softRestBlocks == 0 && !policy.seasonalYearEnd && policy.generatedRestBlocks == 0 && !policy.summerVacations) {
		return nil
	}
	blocks := []rewriteDateRestBlock{}
	for i := 0; i < policy.softRestBlocks; i++ {
		blocks = append(blocks, randomRestBlock(days, restDuration("soft", profile, rng), rng))
	}
	if policy.seasonalYearEnd {
		blocks = append(blocks, yearEndRestBlocks(days, profile, tzOffset)...)
	}
	for i := 0; i < policy.generatedRestBlocks; i++ {
		blocks = append(blocks, randomRestBlock(days, restDuration("generated", profile, rng), rng))
	}
	if policy.summerVacations {
		blocks = append(blocks, summerVacationRestBlocks(days, profile, rng, tzOffset)...)
	}
	return clipAndSortRestBlocks(days, blocks)
}

func rewriteDateRestPolicyForRange(dayCount, selectedCount int) rewriteDateRestPolicy {
	policy := rewriteDateRestPolicy{}
	if dayCount < 14 {
		return policy
	}
	policy.sparseInactiveDays = true
	if dayCount >= 60 && dayCount < 180 && selectedCount >= 10 {
		policy.softRestBlocks = 1
	}
	if dayCount >= 180 && selectedCount >= 20 {
		policy.seasonalYearEnd = true
		policy.generatedRestBlocks = 1
	}
	if dayCount >= 365 && selectedCount >= 30 {
		policy.summerVacations = true
	}
	return policy
}

func randomRestBlock(days []int64, duration int, rng *rand.Rand) rewriteDateRestBlock {
	if len(days) == 0 {
		return rewriteDateRestBlock{}
	}
	if duration < 1 {
		duration = 1
	}
	if duration > len(days) {
		duration = len(days)
	}
	startIndex := 0
	if len(days) > duration {
		startIndex = rng.Intn(len(days) - duration + 1)
	}
	return rewriteDateRestBlock{startDay: days[startIndex], endDay: days[startIndex+duration-1]}
}

func restDuration(kind string, profile rewriteDateProfile, rng *rand.Rand) int {
	switch profile.frequencyName {
	case "low":
		if kind == "soft" {
			return 5 + rng.Intn(4)
		}
		return 7 + rng.Intn(5)
	case "high":
		if kind == "soft" {
			return 2 + rng.Intn(2)
		}
		return 3 + rng.Intn(3)
	default:
		if kind == "soft" {
			return 3 + rng.Intn(3)
		}
		return 5 + rng.Intn(4)
	}
}

func yearEndRestBlocks(days []int64, profile rewriteDateProfile, tzOffset string) []rewriteDateRestBlock {
	if len(days) == 0 {
		return nil
	}
	loc := locationForTimezoneOffset(tzOffset)
	startYear := time.Unix(days[0], 0).In(loc).Year()
	endYear := time.Unix(days[len(days)-1], 0).In(loc).Year()
	blocks := []rewriteDateRestBlock{}
	for year := startYear - 1; year <= endYear; year++ {
		startDay := time.Date(year, time.December, 24, 0, 0, 0, 0, loc).Unix()
		endDay := time.Date(year+1, time.January, 2, 0, 0, 0, 0, loc).Unix()
		switch profile.frequencyName {
		case "low":
			startDay = time.Date(year, time.December, 22, 0, 0, 0, 0, loc).Unix()
			endDay = time.Date(year+1, time.January, 4, 0, 0, 0, 0, loc).Unix()
		case "high":
			startDay = time.Date(year, time.December, 26, 0, 0, 0, 0, loc).Unix()
			endDay = time.Date(year+1, time.January, 1, 0, 0, 0, 0, loc).Unix()
		}
		blocks = append(blocks, rewriteDateRestBlock{startDay: startDay, endDay: endDay})
	}
	return blocks
}

func summerVacationRestBlocks(days []int64, profile rewriteDateProfile, rng *rand.Rand, tzOffset string) []rewriteDateRestBlock {
	if len(days) == 0 {
		return nil
	}
	loc := locationForTimezoneOffset(tzOffset)
	startYear := time.Unix(days[0], 0).In(loc).Year()
	endYear := time.Unix(days[len(days)-1], 0).In(loc).Year()
	blocks := []rewriteDateRestBlock{}
	duration := 8
	switch profile.frequencyName {
	case "low":
		duration = 12
	case "high":
		duration = 5
	}
	for year := startYear; year <= endYear; year++ {
		june := time.Date(year, time.June, 1, 0, 0, 0, 0, loc).Unix()
		augustEnd := time.Date(year, time.August, 31, 0, 0, 0, 0, loc).Unix()
		if augustEnd < days[0] || june > days[len(days)-1] {
			continue
		}
		windowDays := daysBetweenInOffset(june, augustEnd, tzOffset) + 1
		yearDuration := duration
		if windowDays < yearDuration {
			yearDuration = windowDays
		}
		startOffset := 0
		if windowDays > yearDuration {
			startOffset = rng.Intn(windowDays - yearDuration + 1)
		}
		startDay := time.Unix(june, 0).In(loc).AddDate(0, 0, startOffset).Unix()
		endDay := time.Unix(startDay, 0).In(loc).AddDate(0, 0, yearDuration-1).Unix()
		blocks = append(blocks, rewriteDateRestBlock{startDay: startDay, endDay: endDay})
	}
	return blocks
}

func clipAndSortRestBlocks(days []int64, blocks []rewriteDateRestBlock) []rewriteDateRestBlock {
	if len(days) == 0 {
		return nil
	}
	first := days[0]
	last := days[len(days)-1]
	clipped := []rewriteDateRestBlock{}
	for _, block := range blocks {
		if block.endDay < first || block.startDay > last {
			continue
		}
		block.startDay = maxInt64(block.startDay, first)
		block.endDay = minInt64(block.endDay, last)
		if block.startDay <= block.endDay {
			clipped = append(clipped, block)
		}
	}
	sort.Slice(clipped, func(i, j int) bool {
		if clipped[i].startDay == clipped[j].startDay {
			return clipped[i].endDay < clipped[j].endDay
		}
		return clipped[i].startDay < clipped[j].startDay
	})
	return clipped
}

func restDaySet(blocks []rewriteDateRestBlock) map[int64]bool {
	days := map[int64]bool{}
	for _, block := range blocks {
		for day := block.startDay; day <= block.endDay; day += 86400 {
			days[day] = true
		}
	}
	return days
}

func daysInRangeInOffset(startEpoch, endEpoch int64, tzOffset string) []int64 {
	loc := locationForTimezoneOffset(tzOffset)
	start := time.Unix(startEpoch, 0).In(loc)
	end := time.Unix(endEpoch, 0).In(loc)
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	last := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc)
	days := []int64{}
	for !day.After(last) {
		days = append(days, day.Unix())
		day = day.AddDate(0, 0, 1)
	}
	if len(days) == 0 {
		days = append(days, floorDayInOffset(startEpoch, tzOffset))
	}
	return days
}

func daysBetweenInOffset(startEpoch, endEpoch int64, tzOffset string) int {
	days := daysInRangeInOffset(startEpoch, endEpoch, tzOffset)
	if len(days) == 0 {
		return 0
	}
	return len(days) - 1
}

func floorDayInOffset(epoch int64, tzOffset string) int64 {
	loc := locationForTimezoneOffset(tzOffset)
	t := time.Unix(epoch, 0).In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).Unix()
}

func isWeekendInOffset(epoch int64, tzOffset string) bool {
	weekday := time.Unix(epoch, 0).In(locationForTimezoneOffset(tzOffset)).Weekday()
	return weekday == time.Saturday || weekday == time.Sunday
}

func clampInt64(value, minValue, maxValue int64) int64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
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

func renderRewriteDatePlan(a *app, plan rewriteDatePlan, opts rewriteDatesOptions) {
	medianLoad, p90Load, maxLoad := plannedDailyLoadStats(plan.candidates, plan.tzOffset)
	fmt.Fprintln(a.stdout, "Date Rewrite Plan")
	renderKeyValuesTo(a.stdout, []keyValueRow{
		{key: "Repositories", value: fmt.Sprintf("%d", len(plan.candidates))},
		{key: "Selected commits", value: fmt.Sprintf("%d", plan.totalSelected)},
		{key: "Target range", value: formatEpoch(plan.targetStart, plan.tzOffset) + " -> " + formatEpoch(plan.targetEnd, plan.tzOffset)},
		{key: "Planning timezone", value: plan.tzOffset},
		{key: "Active days", value: fmt.Sprintf("%d", calendarActiveDayCount(plan.calendar))},
		{key: "Rest days", value: fmt.Sprintf("%d", calendarRestDayCount(plan.calendar))},
		{key: "Forced-active days", value: fmt.Sprintf("%d", calendarForcedActiveDayCount(plan.calendar))},
		{key: "Median commits/day", value: fmt.Sprintf("%d", medianLoad)},
		{key: "P90 commits/day", value: fmt.Sprintf("%d", p90Load)},
		{key: "Maximum commits/day", value: fmt.Sprintf("%d", maxLoad)},
		{key: "Filters", value: rewriteDateFilterDescription(opts)},
		{key: "Frequency", value: fmt.Sprintf("%s (%s)", plan.profile.frequencyName, plan.profile.frequencyDescription)},
		{key: "Spread", value: fmt.Sprintf("%s (%s)", plan.profile.spreadName, plan.profile.spreadDescription)},
		{key: "Time window", value: rewriteDateTimeWindowDescription(opts)},
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
			fmt.Fprintf(a.stdout, "  %s  %s -> %s\n", prefix(commit.hash, 8), formatEpoch(commit.authorEpoch, commit.authorTZ), formatEpoch(commit.plannedEpoch, candidate.tzOffset))
		}
		fmt.Fprintln(a.stdout)
	}
}

func plannedDailyLoadStats(candidates []dateCandidate, tzOffset string) (int, int, int) {
	countsByDay := plannedDailyCommitCounts(candidates, tzOffset)
	counts := make([]int, 0, len(countsByDay))
	for _, count := range countsByDay {
		counts = append(counts, count)
	}
	if len(counts) == 0 {
		return 0, 0, 0
	}
	sort.Ints(counts)
	median := counts[(len(counts)-1)/2]
	p90 := counts[int(math.Ceil(float64(len(counts))*0.90))-1]
	return median, p90, counts[len(counts)-1]
}

func rewriteDateFilterDescription(opts rewriteDatesOptions) string {
	return currentRewriteDateBoundsDescription(opts.bounds)
}

func rewriteDateTimeWindowDescription(opts rewriteDatesOptions) string {
	if opts.timeSchedule == nil {
		return "generated per active day"
	}
	return opts.timeSchedule.Text
}

func rewriteDateCandidateWarnings(candidate dateCandidate) []string {
	warnings := []string{}
	if candidate.topologyCompressed {
		warnings = append(warnings, "topology constraints compressed some planned timestamps")
	}
	if candidate.hasTags {
		warnings = append(warnings, "tags may still point at old history")
	}
	if candidate.hasSignedObjects {
		warnings = append(warnings, "signatures may become invalid")
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

func applyDateCallbackCandidate(a *app, filterCmd []string, candidate dateCandidate, mapping map[string]dateCallbackDates) (string, error, error) {
	if len(mapping) == 0 {
		return "", fmt.Errorf("no commit date mapping generated"), nil
	}
	if !rewriteDatesWorkingTreeClean(a, candidate.repo.dir) {
		return "", fmt.Errorf("working tree must be clean before rewriting dates"), nil
	}
	baselineHashes := make([]string, 0, len(candidate.selected))
	for _, commitIndex := range candidate.selected {
		baselineHashes = append(baselineHashes, candidate.commits[commitIndex].hash)
	}
	if err := captureRewriteBaselineForHashes(a, candidate.repo, baselineHashes); err != nil {
		return "", fmt.Errorf("could not save rewrite baseline: %w", err), nil
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
	if err := updateRewriteBaselineFromCommitMap(candidate.repo.gitDir, commitMap); err != nil {
		return out, fmt.Errorf("could not update rewrite baseline: %w", err), restoreErr
	}
	return out, nil, restoreErr
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

func readRewriteDateCommitMap(gitDir string) (map[string]string, error) {
	return readFilterRepoCommitMap(gitDir)
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

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func parseDateStart(s string) int64 {
	t, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return t.Unix()
}

func parseDateEnd(s string) int64 {
	return parseDateStart(s) + 86399
}

func parseDateStartInOffset(s string, offset string) int64 {
	t, _ := time.ParseInLocation("2006-01-02", s, locationForTimezoneOffset(offset))
	return t.Unix()
}

func parseDateEndInOffset(s string, offset string) int64 {
	return parseDateStartInOffset(s, offset) + 86399
}

func locationForTimezoneOffset(offset string) *time.Location {
	if !timezoneOffsetRe.MatchString(offset) {
		return time.Local
	}
	sign := 1
	if strings.HasPrefix(offset, "-") {
		sign = -1
	}
	hours, _ := strconv.Atoi(offset[1:3])
	minutes, _ := strconv.Atoi(offset[3:5])
	return time.FixedZone(offset, sign*(hours*3600+minutes*60))
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
	offsets := make([]string, 0, len(counts))
	for offset := range counts {
		offsets = append(offsets, offset)
	}
	sort.Strings(offsets)
	for _, offset := range offsets {
		count := counts[offset]
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

func formatEpoch(epoch int64, offset string) string {
	loc := locationForTimezoneOffset(offset)
	return time.Unix(epoch, 0).In(loc).Format("2006-01-02 15:04:05 ") + offset
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

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
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
