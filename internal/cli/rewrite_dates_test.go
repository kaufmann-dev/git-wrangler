package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRewriteDatesFlagValidation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"rewrite-dates", "--start-date", "bad"}, "--start-date must be in YYYY-MM-DD format"},
		{[]string{"rewrite-dates", "--days", "0"}, "--days must be a positive integer"},
		{[]string{"rewrite-dates", "--days", "7", "--start-date", "2024-01-01"}, "--days cannot be combined"},
		{[]string{"rewrite-dates", "--until", "2024-01-31"}, "--until requires --days"},
		{[]string{"rewrite-dates", "--frequency", "extreme"}, "--frequency must be low, medium, or high"},
		{[]string{"rewrite-dates", "--spread", "extreme"}, "--spread must be low, medium, or high"},
		{[]string{"rewrite-dates", "--rewrite-after", "2024-02-01", "--rewrite-before", "2024-02-01"}, "--rewrite-after must be before --rewrite-before"},
		{[]string{"rewrite-dates", "--window", "bad"}, "--window window must be in HH:MM-HH:MM format"},
		{[]string{"rewrite-dates", "--window", "18:00-09:00"}, "--window window start must be before window end"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithIO(tc.args, strings.NewReader(""), &stdout, &stderr)
		assertExitCode(t, err, 1)
		if !strings.Contains(stderr.String(), tc.want) {
			t.Fatalf("%v stderr:\n%s", tc.args, stderr.String())
		}
	}
}

func TestRewriteDatesRollbackFlagRemoved(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	err := ExecuteWithIO([]string{"rewrite-dates", "--rollback"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil || !strings.Contains(stderr.String(), "unknown flag: --rollback") {
		t.Fatalf("rewrite-dates --rollback error = %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
}

func TestRewriteHoursRequiresWindow(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	err := ExecuteWithIO([]string{"rewrite-hours"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if !strings.Contains(stderr.String(), "--window is required") {
		t.Fatalf("missing window error:\nstdout:%s\nstderr:%s", stdout.String(), stderr.String())
	}
}

func TestCommitTimeWindowParsing(t *testing.T) {
	window, err := parseCommitTimeWindow("09:15-17:45")
	if err != nil {
		t.Fatal(err)
	}
	if window.StartMinute != 9*60+15 || window.EndMinute != 17*60+45 || window.Text != "09:15-17:45" {
		t.Fatalf("window = %+v", window)
	}
	for _, value := range []string{"9:00-17:00", "24:00-25:00", "18:00-09:00", "09:00-09:00"} {
		if _, err := parseCommitTimeWindow(value); err == nil {
			t.Fatalf("%q was accepted", value)
		}
	}
}

func TestExplicitWindowEpochsAndOverflow(t *testing.T) {
	window, err := parseCommitTimeWindow("09:00-09:00")
	if err == nil {
		t.Fatalf("zero-length window accepted: %+v", window)
	}
	window, err = parseCommitTimeWindow("09:00-09:01")
	if err != nil {
		t.Fatal(err)
	}
	day := parseDateStartInOffset("2024-01-01", "+0000")
	got := plannedEpochsForExplicitWindow(day, 3, day, day+86399, window)
	want := []int64{day + 9*3600, day + 9*3600 + 29, day + 9*3600 + 59}
	assertEpochsEqual(t, got, want)

	overflow := plannedEpochsForExplicitWindow(day, 62, day, day+86399, window)
	assertSortedEpochsInRange(t, overflow, day+9*3600, day+9*3600+59)
	duplicates := 0
	for i := 1; i < len(overflow); i++ {
		if overflow[i] == overflow[i-1] {
			duplicates++
		}
	}
	if duplicates == 0 {
		t.Fatal("overflow did not produce duplicate seconds")
	}
}

func TestRewriteDateProfileDefaultsAndMappings(t *testing.T) {
	defaultProfile, ok := buildRewriteDateProfile("", "")
	if !ok {
		t.Fatal("empty profile values were not normalized")
	}
	if defaultProfile.frequencyName != "medium" || defaultProfile.spreadName != "medium" {
		t.Fatalf("default profile = %s/%s", defaultProfile.frequencyName, defaultProfile.spreadName)
	}

	for _, tc := range []struct {
		name                      string
		activeRatio               float64
		commitSpread              float64
		demandScale               float64
		weekendActivityMultiplier float64
		eveningChance             float64
		sessionMinDays            int
		sessionMaxDays            int
		gapMinDays                int
		gapMaxDays                int
	}{
		{name: "low", activeRatio: 0.18, commitSpread: 0.25, demandScale: 12, weekendActivityMultiplier: 0.65, eveningChance: 0.05, sessionMinDays: 1, sessionMaxDays: 3, gapMinDays: 5, gapMaxDays: 14},
		{name: "medium", activeRatio: 0.32, commitSpread: 0.50, demandScale: 8, weekendActivityMultiplier: 0.80, eveningChance: 0.12, sessionMinDays: 2, sessionMaxDays: 5, gapMinDays: 2, gapMaxDays: 7},
		{name: "high", activeRatio: 0.55, commitSpread: 0.72, demandScale: 5, weekendActivityMultiplier: 0.95, eveningChance: 0.24, sessionMinDays: 3, sessionMaxDays: 8, gapMinDays: 1, gapMaxDays: 4},
	} {
		profile, ok := buildRewriteDateProfile(tc.name, "medium")
		if !ok {
			t.Fatalf("frequency %q rejected", tc.name)
		}
		if profile.frequencyName != tc.name || profile.activeRatio != tc.activeRatio || profile.commitSpread != tc.commitSpread || profile.demandScale != tc.demandScale || profile.weekendActivityMultiplier != tc.weekendActivityMultiplier || profile.eveningChance != tc.eveningChance || profile.sessionMinDays != tc.sessionMinDays || profile.sessionMaxDays != tc.sessionMaxDays || profile.gapMinDays != tc.gapMinDays || profile.gapMaxDays != tc.gapMaxDays {
			t.Fatalf("frequency profile %q = %+v", tc.name, profile)
		}
	}

	for _, tc := range []struct {
		name         string
		daySigma     float64
		sessionSigma float64
		persistence  float64
	}{
		{name: "low", daySigma: 0.35, sessionSigma: 0.20, persistence: 0.35},
		{name: "medium", daySigma: 0.75, sessionSigma: 0.35, persistence: 0.55},
		{name: "high", daySigma: 1.00, sessionSigma: 0.50, persistence: 0.70},
	} {
		profile, ok := buildRewriteDateProfile("medium", tc.name)
		if !ok {
			t.Fatalf("spread %q rejected", tc.name)
		}
		if profile.spreadName != tc.name || profile.daySigma != tc.daySigma || profile.sessionSigma != tc.sessionSigma || profile.persistence != tc.persistence {
			t.Fatalf("spread profile %q = %+v", tc.name, profile)
		}
	}

	if _, ok := buildRewriteDateProfile("extreme", "medium"); ok {
		t.Fatal("invalid frequency was accepted")
	}
	if _, ok := buildRewriteDateProfile("medium", "extreme"); ok {
		t.Fatal("invalid spread was accepted")
	}
}

func TestRewriteDateSelectionFiltersUseOriginalAuthorDates(t *testing.T) {
	opts := rewriteDatesOptions{
		hasRewriteAfter:  true,
		rewriteAfter:     parseDateStart("2024-01-10"),
		hasRewriteBefore: true,
		rewriteBefore:    parseDateStart("2024-01-20"),
	}
	before := testRewriteDateCommit("a", parseDateStart("2024-01-09"))
	first := testRewriteDateCommit("b", parseDateStart("2024-01-10"))
	last := testRewriteDateCommit("c", parseDateStart("2024-01-19"))
	after := testRewriteDateCommit("d", parseDateStart("2024-01-20"))

	if rewriteDateCommitSelected(before, opts) {
		t.Fatal("commit before --rewrite-after was selected")
	}
	if !rewriteDateCommitSelected(first, opts) || !rewriteDateCommitSelected(last, opts) {
		t.Fatal("commits inside after <= commit < before window were not selected")
	}
	if rewriteDateCommitSelected(after, opts) {
		t.Fatal("commit at --rewrite-before boundary was selected")
	}
}

func TestRewriteDateTargetRangeDays(t *testing.T) {
	candidates := []dateCandidate{testRewriteDateCandidate("repo", []rewriteDateCommit{
		testRewriteDateCommit("a", parseDateStart("2020-01-01")),
		testRewriteDateCommit("b", parseDateStart("2020-01-02"), "a"),
	}, []int{0, 1})}
	plan, err := planRewriteDateCandidates(candidates, rewriteDatesOptions{
		days:      7,
		untilDate: "2024-01-31",
		seed:      "seed",
		frequency: "medium",
		spread:    "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.targetStart != parseDateStartInOffset("2024-01-25", "+0000") {
		t.Fatalf("targetStart = %s", formatEpochLocal(plan.targetStart))
	}
	if plan.targetEnd != parseDateEndInOffset("2024-01-31", "+0000") {
		t.Fatalf("targetEnd = %s", formatEpochLocal(plan.targetEnd))
	}
}

func TestRewriteDateExplicitWindowPlanning(t *testing.T) {
	window, err := parseCommitTimeWindow("09:00-11:00")
	if err != nil {
		t.Fatal(err)
	}
	commits := []rewriteDateCommit{
		testRewriteDateCommit("a", parseDateStart("2020-01-01")),
		testRewriteDateCommit("b", parseDateStart("2020-01-02"), "a"),
		testRewriteDateCommit("c", parseDateStart("2020-01-03"), "b"),
		testRewriteDateCommit("d", parseDateStart("2020-01-04"), "c"),
	}
	candidates := []dateCandidate{testRewriteDateCandidate("repo", commits, []int{0, 1, 2, 3})}
	plan, err := planRewriteDateCandidates(candidates, rewriteDatesOptions{
		startDate:  "2024-01-01",
		endDate:    "2024-01-10",
		seed:       "window-seed",
		frequency:  "medium",
		spread:     "medium",
		window:     window.Text,
		timeWindow: &window,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range plan.candidates {
		for _, idx := range candidate.selected {
			commit := candidate.commits[idx]
			if !epochInCommitTimeWindow(commit.plannedEpoch, candidate.tzOffset, window) {
				t.Fatalf("%s planned outside window: %s", commit.hash, formatEpoch(commit.plannedEpoch, candidate.tzOffset))
			}
		}
	}
}

func TestGeneratePlannedEpochsDeterministicBySeed(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	end := parseDateEndInOffset("2024-06-30", "+0000")
	profile, _ := buildRewriteDateProfile("medium", "medium")

	got := generatePlannedEpochs(80, start, end, "seed-a", profile, "+0000")
	again := generatePlannedEpochs(80, start, end, "seed-a", profile, "+0000")
	different := generatePlannedEpochs(80, start, end, "seed-b", profile, "+0000")
	assertEpochsEqual(t, got, again)
	if epochsEqual(got, different) {
		t.Fatal("different seed produced the same plan")
	}
	assertSortedEpochsInRange(t, got, start, end)
}

func TestRewriteDateCalendarDeterministicBySeed(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	end := parseDateEndInOffset("2024-12-31", "+0000")
	profile, _ := buildRewriteDateProfile("medium", "medium")

	got := buildRewriteDateCalendarPlan(160, start, end, "seed-a", profile, "+0000")
	again := buildRewriteDateCalendarPlan(160, start, end, "seed-a", profile, "+0000")
	different := buildRewriteDateCalendarPlan(160, start, end, "seed-b", profile, "+0000")
	if calendarSignature(got) != calendarSignature(again) {
		t.Fatalf("same seed produced different calendars:\n%s\n%s", calendarSignature(got), calendarSignature(again))
	}
	if calendarSignature(got) == calendarSignature(different) {
		t.Fatal("different seed produced the same calendar")
	}
}

func TestRewriteDateCalendarSessionsCoverLongTargetRange(t *testing.T) {
	start := parseDateStartInOffset("2021-01-01", "+0000")
	end := parseDateEndInOffset("2026-06-30", "+0000")
	seeds := []string{"coverage-a", "coverage-b", "coverage-c"}

	for _, frequencyName := range []string{"low", "medium", "high"} {
		t.Run(frequencyName, func(t *testing.T) {
			profile, _ := buildRewriteDateProfile(frequencyName, "medium")
			for _, seed := range seeds {
				calendar := buildRewriteDateCalendarPlan(71, start, end, seed, profile, "+0000")
				activeTarget := plannedCalendarActiveDayCount(71, len(calendar.days), calendarRestDayCount(calendar), profile)
				if calendarActiveDayCount(calendar) != activeTarget {
					t.Fatalf("%s active days = %d, want %d", seed, calendarActiveDayCount(calendar), activeTarget)
				}
				if covered := activeCalendarCoverageWindowCount(calendar, 4); covered != 4 {
					t.Fatalf("%s covered %d of 4 range quarters; active days: %s", seed, covered, activeCalendarDayLabels(calendar))
				}
			}
		})
	}
}

func TestRewriteDateCalendarSingleSessionUsesInteriorAnchor(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	end := parseDateEndInOffset("2024-01-31", "+0000")
	low, _ := buildRewriteDateProfile("low", "medium")

	calendar := buildRewriteDateCalendarPlan(4, start, end, "single-session", low, "+0000")
	activeIndexes := []int{}
	for i, day := range calendar.days {
		if calendarDayHasSlots(day.state) {
			activeIndexes = append(activeIndexes, i)
		}
	}
	if len(activeIndexes) != 1 {
		t.Fatalf("active indexes = %v, want one active day", activeIndexes)
	}
	if activeIndexes[0] == 0 || activeIndexes[0] == len(calendar.days)-1 {
		t.Fatalf("single session landed on range edge: %s", formatEpoch(calendar.days[activeIndexes[0]].epoch, "+0000"))
	}
}

func TestRewriteDatePlanningSeedSalt(t *testing.T) {
	if got := rewriteDatePlanningSeed("seed", 0); got != "seed" {
		t.Fatalf("attempt 0 seed = %q", got)
	}
	if got := rewriteDatePlanningSeed("seed", 3); got != "seed\x00rewrite-dates-topology\x003" {
		t.Fatalf("attempt 3 seed = %q", got)
	}
}

func TestRewriteDateRestPolicyThresholds(t *testing.T) {
	for _, tc := range []struct {
		name            string
		days            int
		selected        int
		sparse          bool
		soft            int
		seasonal        bool
		generated       int
		summerVacations bool
	}{
		{name: "13 days", days: 13, selected: 30},
		{name: "14 days", days: 14, selected: 30, sparse: true},
		{name: "59 days", days: 59, selected: 30, sparse: true},
		{name: "60 days below 10 commits", days: 60, selected: 9, sparse: true},
		{name: "60 days at 10 commits", days: 60, selected: 10, sparse: true, soft: 1},
		{name: "179 days", days: 179, selected: 10, sparse: true, soft: 1},
		{name: "180 days below 20 commits", days: 180, selected: 19, sparse: true},
		{name: "180 days at 20 commits", days: 180, selected: 20, sparse: true, seasonal: true, generated: 1},
		{name: "364 days", days: 364, selected: 20, sparse: true, seasonal: true, generated: 1},
		{name: "365 days below 30 commits", days: 365, selected: 29, sparse: true, seasonal: true, generated: 1},
		{name: "365 days at 30 commits", days: 365, selected: 30, sparse: true, seasonal: true, generated: 1, summerVacations: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteDateRestPolicyForRange(tc.days, tc.selected)
			if got.sparseInactiveDays != tc.sparse || got.softRestBlocks != tc.soft || got.seasonalYearEnd != tc.seasonal || got.generatedRestBlocks != tc.generated || got.summerVacations != tc.summerVacations {
				t.Fatalf("policy = %+v", got)
			}
		})
	}
}

func TestSyntheticRestBlocksIncludeJanuaryYearEndIntersection(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	end := parseDateEndInOffset("2024-06-29", "+0000")
	days := daysInRangeInOffset(start, end, "+0000")
	profile, _ := buildRewriteDateProfile("medium", "medium")

	blocks := syntheticRestBlocks(days, 20, profile, rand.New(rand.NewSource(seedInt64("rest"))), "+0000")
	jan1 := parseDateStartInOffset("2024-01-01", "+0000")
	found := false
	for _, block := range blocks {
		if block.startDay <= jan1 && block.endDay >= jan1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("year-end rest did not include January intersection: %+v", blocks)
	}
}

func TestGeneratePlannedEpochsSmallSelectionsAvoidEdges(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	end := parseDateEndInOffset("2024-01-10", "+0000")
	profile, _ := buildRewriteDateProfile("medium", "medium")

	one := generatePlannedEpochs(1, start, end, "one", profile, "+0000")
	if len(one) != 1 {
		t.Fatalf("one timestamp count = %d", len(one))
	}
	if floorDayInOffset(one[0], "+0000") == floorDayInOffset(start, "+0000") || floorDayInOffset(one[0], "+0000") == floorDayInOffset(end, "+0000") {
		t.Fatalf("single commit landed on an edge day: %s", formatEpoch(one[0], "+0000"))
	}

	for _, n := range []int{2, 3} {
		got := generatePlannedEpochs(n, start, end, fmt.Sprintf("small-%d", n), profile, "+0000")
		assertSortedEpochsInRange(t, got, start, end)
		if floorDayInOffset(got[0], "+0000") == floorDayInOffset(start, "+0000") && floorDayInOffset(got[len(got)-1], "+0000") == floorDayInOffset(end, "+0000") {
			t.Fatalf("%d commits were forced to target endpoints: %s -> %s", n, formatEpoch(got[0], "+0000"), formatEpoch(got[len(got)-1], "+0000"))
		}
	}
}

func TestGeneratePlannedEpochsSingleDayHighVolume(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	end := parseDateEndInOffset("2024-01-01", "+0000")
	profile, _ := buildRewriteDateProfile("high", "medium")

	got := generatePlannedEpochs(200, start, end, "seed", profile, "+0000")
	if len(got) != 200 {
		t.Fatalf("timestamps = %d, want 200", len(got))
	}
	assertSortedEpochsInRange(t, got, start, end)
}

func TestGeneratePlannedEpochsActivityInvariants(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	end := parseDateEndInOffset("2024-12-31", "+0000")
	low, _ := buildRewriteDateProfile("low", "medium")
	medium, _ := buildRewriteDateProfile("medium", "medium")
	high, _ := buildRewriteDateProfile("high", "medium")

	lowPlan := generatePlannedEpochs(120, start, end, "activity", low, "+0000")
	mediumPlan := generatePlannedEpochs(120, start, end, "activity", medium, "+0000")
	highPlan := generatePlannedEpochs(120, start, end, "activity", high, "+0000")

	if maxInactiveGapDays(mediumPlan, "+0000") < 2 {
		t.Fatalf("medium plan lacks a multi-day inactive gap")
	}
	if multiDayInactiveGapCount(mediumPlan, "+0000", 2) < 3 {
		t.Fatalf("medium plan lacks multiple multi-day inactive gaps")
	}
	if maxInactiveGapDays(highPlan, "+0000") > maxInactiveGapDays(mediumPlan, "+0000") {
		t.Fatalf("high frequency has a longer max gap than medium")
	}
	if activeDayCount(lowPlan, "+0000") >= activeDayCount(mediumPlan, "+0000") {
		t.Fatalf("low active days = %d, medium active days = %d", activeDayCount(lowPlan, "+0000"), activeDayCount(mediumPlan, "+0000"))
	}
	if activeDayCount(highPlan, "+0000") <= activeDayCount(mediumPlan, "+0000") {
		t.Fatalf("high active days = %d, medium active days = %d", activeDayCount(highPlan, "+0000"), activeDayCount(mediumPlan, "+0000"))
	}
	denseMedium := generatePlannedEpochs(1000, start, end, "dense-activity", medium, "+0000")
	if activeDayCount(denseMedium, "+0000") >= 360 {
		t.Fatalf("medium dense plan used nearly every day: active=%d", activeDayCount(denseMedium, "+0000"))
	}
	weekend := weekendActiveDayCount(mediumPlan, "+0000")
	total := activeDayCount(mediumPlan, "+0000")
	if weekend*2 >= total {
		t.Fatalf("weekend active days are not a minority: weekend=%d total=%d", weekend, total)
	}
}

func TestRewriteDateDailyQuotasScaleWithDemand(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	end := parseDateEndInOffset("2024-12-31", "+0000")
	medium, _ := buildRewriteDateProfile("medium", "medium")

	smallCalendar := buildRewriteDateCalendarPlan(400, start, end, "demand", medium, "+0000")
	largeCalendar := buildRewriteDateCalendarPlan(40000, start, end, "demand", medium, "+0000")
	if calendarQuotaTotal(smallCalendar) != 400 || calendarQuotaTotal(largeCalendar) != 40000 {
		t.Fatalf("quota sums = %d and %d", calendarQuotaTotal(smallCalendar), calendarQuotaTotal(largeCalendar))
	}
	if calendarForcedActiveDayCount(smallCalendar) != 0 || calendarForcedActiveDayCount(largeCalendar) != 0 {
		t.Fatalf("volume activated rest days: small=%d large=%d", calendarForcedActiveDayCount(smallCalendar), calendarForcedActiveDayCount(largeCalendar))
	}
	if calendarRestDayCount(smallCalendar) == 0 || calendarRestDayCount(largeCalendar) == 0 {
		t.Fatal("expected synchronized rest periods")
	}
	smallRest := restCalendarDaySet(smallCalendar)
	largeRest := restCalendarDaySet(largeCalendar)
	if len(smallRest) != len(largeRest) {
		t.Fatalf("rest periods changed with volume: small=%d large=%d", len(smallRest), len(largeRest))
	}
	for day := range smallRest {
		if !largeRest[day] {
			t.Fatalf("rest day %s changed with volume", formatEpoch(day, "+0000"))
		}
	}

	smallMedian, smallP90, smallMax := calendarDailyLoadStats(smallCalendar)
	largeMedian, largeP90, largeMax := calendarDailyLoadStats(largeCalendar)
	if smallMedian > 4 || smallMax <= 4 || smallMedian == smallP90 {
		t.Fatalf("small load shape = median %d p90 %d max %d", smallMedian, smallP90, smallMax)
	}
	if largeMedian <= smallMedian*10 || largeP90 <= smallP90*10 || largeMax <= smallMax*10 {
		t.Fatalf("large load did not scale: small=%d/%d/%d large=%d/%d/%d", smallMedian, smallP90, smallMax, largeMedian, largeP90, largeMax)
	}
}

func TestRewriteDateSpreadControlsQuotaVarianceOnly(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	end := parseDateEndInOffset("2024-12-31", "+0000")
	low, _ := buildRewriteDateProfile("medium", "low")
	medium, _ := buildRewriteDateProfile("medium", "medium")
	high, _ := buildRewriteDateProfile("medium", "high")

	lowCalendar := buildRewriteDateCalendarPlan(4000, start, end, "spread", low, "+0000")
	mediumCalendar := buildRewriteDateCalendarPlan(4000, start, end, "spread", medium, "+0000")
	highCalendar := buildRewriteDateCalendarPlan(4000, start, end, "spread", high, "+0000")
	if calendarPlacementSignature(lowCalendar) != calendarPlacementSignature(mediumCalendar) || calendarPlacementSignature(mediumCalendar) != calendarPlacementSignature(highCalendar) {
		t.Fatal("spread changed calendar active/rest placement")
	}
	if calendarLoadRange(lowCalendar) >= calendarLoadRange(mediumCalendar) || calendarLoadRange(mediumCalendar) >= calendarLoadRange(highCalendar) {
		t.Fatalf("quota variance did not increase with spread: low=%d medium=%d high=%d", calendarLoadRange(lowCalendar), calendarLoadRange(mediumCalendar), calendarLoadRange(highCalendar))
	}
}

func TestRewriteDateWorkloadWeightsIgnoreWeekendActivityMultiplier(t *testing.T) {
	lowFrequency, _ := buildRewriteDateProfile("low", "medium")
	highFrequency, _ := buildRewriteDateProfile("high", "medium")
	calendar := rewriteDateCalendarPlan{
		tzOffset: "+0000",
		days: []rewriteDateCalendarDay{
			{epoch: parseDateStart("2024-01-01"), state: rewriteDateCalendarActive},
			{epoch: parseDateStart("2024-01-06"), state: rewriteDateCalendarActive},
			{epoch: parseDateStart("2024-01-08"), state: rewriteDateCalendarActive},
		},
	}
	lowWeights := workloadWeights(calendar, []int{0, 1, 2}, lowFrequency, 1, rand.New(rand.NewSource(seedInt64("weights"))))
	highWeights := workloadWeights(calendar, []int{0, 1, 2}, highFrequency, 1, rand.New(rand.NewSource(seedInt64("weights"))))
	for i := range lowWeights {
		if lowWeights[i] != highWeights[i] {
			t.Fatalf("workload weight %d changed with frequency: low=%v high=%v", i, lowWeights, highWeights)
		}
	}
}

func TestRewriteDateAllRestFallbackUsesOneDay(t *testing.T) {
	medium, _ := buildRewriteDateProfile("medium", "medium")
	calendar := rewriteDateCalendarPlan{
		tzOffset: "+0000",
		days: []rewriteDateCalendarDay{
			{epoch: parseDateStart("2024-01-01"), state: rewriteDateCalendarRest},
			{epoch: parseDateStart("2024-01-02"), state: rewriteDateCalendarRest},
			{epoch: parseDateStart("2024-01-03"), state: rewriteDateCalendarRest},
		},
	}
	rng := rand.New(rand.NewSource(seedInt64("all-rest")))
	ensureCalendarHasActiveDay(&calendar, medium, rng)
	assignCalendarDailyQuotas(&calendar, 1000, medium, 1, rng)
	if calendarForcedActiveDayCount(calendar) != 1 || calendarActiveDayCount(calendar) != 1 {
		t.Fatalf("all-rest fallback activated %d forced days and %d total days", calendarForcedActiveDayCount(calendar), calendarActiveDayCount(calendar))
	}
	if calendarQuotaTotal(calendar) != 1000 {
		t.Fatalf("all-rest fallback quota total = %d", calendarQuotaTotal(calendar))
	}
}

func TestRewriteDateAttemptRankingUsesQuotaDeviationBeforeAdjustments(t *testing.T) {
	left := rewriteDateTopologyCompression{dailyQuotaDeviation: 2, adjustedCommits: 20}
	right := rewriteDateTopologyCompression{dailyQuotaDeviation: 3, adjustedCommits: 1}
	if !rewriteDateCompressionLess(left, right) {
		t.Fatal("lower daily quota deviation did not rank first")
	}
	left = rewriteDateTopologyCompression{forcedActiveDays: 1}
	right = rewriteDateTopologyCompression{compressed: true}
	if !rewriteDateCompressionLess(right, left) {
		t.Fatal("fewer forced-active days did not rank first")
	}
	left = rewriteDateTopologyCompression{}
	right = rewriteDateTopologyCompression{compressed: true}
	if !rewriteDateCompressionLess(left, right) {
		t.Fatal("uncompressed attempt did not rank first after forced-active days")
	}
}

func TestRewriteDateRepositoryConcentrationDampsWorkloadVariation(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	end := parseDateEndInOffset("2024-12-31", "+0000")
	medium, _ := buildRewriteDateProfile("medium", "medium")

	dominant := buildRewriteDateCalendarPlanForRepos(4000, start, end, "repos", medium, "+0000", 1)
	balanced := buildRewriteDateCalendarPlanForRepos(4000, start, end, "repos", medium, "+0000", 8)
	if calendarQuotaTotal(dominant) != 4000 || calendarQuotaTotal(balanced) != 4000 {
		t.Fatal("repository concentration changed selected commit totals")
	}
	if calendarPlacementSignature(dominant) != calendarPlacementSignature(balanced) {
		t.Fatal("repository concentration changed temporal placement")
	}
	if calendarLoadRange(dominant) <= calendarLoadRange(balanced) {
		t.Fatalf("dominant repository did not retain stronger variation: dominant=%d balanced=%d", calendarLoadRange(dominant), calendarLoadRange(balanced))
	}
}

func TestRewriteDateSeedSource(t *testing.T) {
	candidates := []dateCandidate{{}}
	if seed, source := rewriteDateSeed(rewriteDatesOptions{seed: "flag-seed"}, candidates); seed != "flag-seed" || source != "flag" {
		t.Fatalf("flag seed = %q/%q", seed, source)
	}
	if seed, source := rewriteDateSeed(rewriteDatesOptions{}, candidates); seed == "" || source != "generated" {
		t.Fatalf("generated seed = %q/%q", seed, source)
	}
}

func TestRewriteDateTopologyConstraintsIncludeMerges(t *testing.T) {
	candidate := testRewriteDateCandidate("repo", []rewriteDateCommit{
		testRewriteDateCommit("a", 100),
		testRewriteDateCommit("b", 100, "a"),
		testRewriteDateCommit("c", 100, "a"),
		testRewriteDateCommit("d", 100, "b", "c"),
	}, []int{0, 1, 2, 3})
	for i := range candidate.selected {
		candidate.commits[candidate.selected[i]].plannedEpoch = 100
	}
	candidates := []dateCandidate{candidate}
	if err := enforceRewriteDateTopology(candidates, 100, 200); err != nil {
		t.Fatal(err)
	}
	commits := candidates[0].commits
	if commits[0].plannedEpoch >= commits[1].plannedEpoch || commits[0].plannedEpoch >= commits[2].plannedEpoch {
		t.Fatalf("parent was not before children: %+v", commits)
	}
	if commits[1].plannedEpoch >= commits[3].plannedEpoch || commits[2].plannedEpoch >= commits[3].plannedEpoch {
		t.Fatalf("merge commit was not after both parents: %+v", commits)
	}
}

func TestRewriteDateExplicitTargetReportsFixedBoundary(t *testing.T) {
	parent := testRewriteDateCommit("parent123", 200000)
	child := testRewriteDateCommit("child123", 300000, "parent123")
	child.selected = true
	candidates := []dateCandidate{testRewriteDateCandidate("repo", []rewriteDateCommit{parent, child}, []int{1})}
	_, err := planRewriteDateCandidates(candidates, rewriteDatesOptions{
		startDate: "1970-01-01",
		endDate:   "1970-01-01",
		seed:      "seed",
		frequency: "medium",
		spread:    "medium",
	})
	if err == nil {
		t.Fatal("expected planning error")
	}
	if !strings.Contains(err.Error(), "repo") || !strings.Contains(err.Error(), "child123") {
		t.Fatalf("error lacks concrete repo/commit: %v", err)
	}
}

func TestRewriteDatePlanningUsesDominantTimezone(t *testing.T) {
	commits := []rewriteDateCommit{
		testRewriteDateCommit("a", parseDateStartInOffset("2020-01-01", "+0530")),
		testRewriteDateCommit("b", parseDateStartInOffset("2020-01-02", "+0530"), "a"),
		testRewriteDateCommit("c", parseDateStartInOffset("2020-01-03", "-0700"), "b"),
	}
	commits[0].originalAuthorTZ = "+0530"
	commits[0].authorTZ = "+0530"
	commits[1].originalAuthorTZ = "+0530"
	commits[1].authorTZ = "+0530"
	commits[2].originalAuthorTZ = "-0700"
	commits[2].authorTZ = "-0700"
	candidates := []dateCandidate{testRewriteDateCandidate("repo", commits, []int{0, 1, 2})}

	plan, err := planRewriteDateCandidates(candidates, rewriteDatesOptions{
		startDate: "2024-01-01",
		endDate:   "2024-01-02",
		seed:      "seed",
		frequency: "medium",
		spread:    "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.tzOffset != "+0530" || plan.candidates[0].tzOffset != "+0530" {
		t.Fatalf("planning timezone = plan %q candidate %q", plan.tzOffset, plan.candidates[0].tzOffset)
	}
	if plan.targetStart != parseDateStartInOffset("2024-01-01", "+0530") || plan.targetEnd != parseDateEndInOffset("2024-01-02", "+0530") {
		t.Fatalf("target range = %s -> %s", formatEpoch(plan.targetStart, "+0530"), formatEpoch(plan.targetEnd, "+0530"))
	}

	var stdout bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, io.Discard)
	renderRewriteDatePlan(a, plan, rewriteDatesOptions{frequency: "medium", spread: "medium"})
	if !strings.Contains(stdout.String(), "2024-01-01 00:00:00 +0530") || !strings.Contains(stdout.String(), "Timezone: +0530") {
		t.Fatalf("preview did not use planning timezone:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Planning timezone") || !strings.Contains(stdout.String(), "Active days") || !strings.Contains(stdout.String(), "Rest days") || !strings.Contains(stdout.String(), "Forced-active days") {
		t.Fatalf("preview did not include calendar metadata:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Median commits/day") || !strings.Contains(stdout.String(), "P90 commits/day") || !strings.Contains(stdout.String(), "Maximum commits/day") {
		t.Fatalf("preview did not include final daily load statistics:\n%s", stdout.String())
	}

	mapping := map[string]dateCallbackDates{}
	for _, idx := range plan.candidates[0].selected {
		commit := plan.candidates[0].commits[idx]
		date := fmt.Sprintf("%d %s", commit.plannedEpoch, plan.candidates[0].tzOffset)
		mapping[commit.hash] = dateCallbackDates{Author: date, Committer: date}
	}
	callback, err := writeDateCallbackDates(mapping)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(callback)
	data, err := os.ReadFile(callback)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "+0530") {
		t.Fatalf("callback did not use planning timezone:\n%s", string(data))
	}
}

func TestRewriteDateTopologyCompressionWarning(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	commits := make([]rewriteDateCommit, 0, 9)
	for i := 0; i < 8; i++ {
		parents := []string{}
		if i > 0 {
			parents = append(parents, fmt.Sprintf("c%d", i-1))
		}
		commits = append(commits, testRewriteDateCommit(fmt.Sprintf("c%d", i), start+1000+int64(i), parents...))
	}
	commits = append(commits, testRewriteDateCommit("fixed-child", start+8, "c7"))
	candidates := []dateCandidate{testRewriteDateCandidate("repo", commits, []int{0, 1, 2, 3, 4, 5, 6, 7})}

	plan, err := planRewriteDateCandidates(candidates, rewriteDatesOptions{
		startDate: "2024-01-01",
		endDate:   "2024-01-03",
		seed:      "seed",
		frequency: "medium",
		spread:    "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.candidates[0].topologyCompressed {
		t.Fatal("expected topology compression warning flag")
	}
	warnings := strings.Join(rewriteDateCandidateWarnings(plan.candidates[0]), "; ")
	if !strings.Contains(warnings, "topology constraints compressed some planned timestamps") {
		t.Fatalf("missing compression warning: %s", warnings)
	}
	for i := 1; i < 8; i++ {
		left := plan.candidates[0].commits[i-1].plannedEpoch
		right := plan.candidates[0].commits[i].plannedEpoch
		if right-left != 1 {
			t.Fatalf("gap %d = %d, want 1", i, right-left)
		}
	}
}

func TestRewriteDateSharedCalendarKeepsRestDaysBlankAcrossRepositories(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "+0000")
	candidates := []dateCandidate{
		testRewriteDateCandidate("repo-a", testRewriteDateCommits("a", 90, start, 86400*4), indexesForCount(90)),
		testRewriteDateCandidate("repo-b", testRewriteDateCommits("b", 90, start+43200, 86400*4), indexesForCount(90)),
	}

	plan, err := planRewriteDateCandidates(candidates, rewriteDatesOptions{
		startDate: "2024-01-01",
		endDate:   "2024-12-31",
		seed:      "shared-rest",
		frequency: "medium",
		spread:    "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	restDays := restCalendarDaySet(plan.calendar)
	if len(restDays) == 0 {
		t.Fatal("calendar did not include rest days")
	}
	for _, candidate := range plan.candidates {
		for _, day := range plannedCommitDays(candidate, plan.tzOffset) {
			if restDays[day] {
				t.Fatalf("%s planned a selected commit on rest day %s", candidate.repo.display, formatEpoch(day, plan.tzOffset))
			}
		}
	}
}

func TestRewriteDateRestDaysStayBlankInUTCActivityModel(t *testing.T) {
	start := parseDateStartInOffset("2024-01-01", "-0700")
	commits := testRewriteDateCommits("c", 120, start, 86400*3)
	for i := range commits {
		commits[i].authorTZ = "-0700"
		commits[i].originalAuthorTZ = "-0700"
	}
	candidates := []dateCandidate{testRewriteDateCandidate("repo", commits, indexesForCount(len(commits)))}

	plan, err := planRewriteDateCandidates(candidates, rewriteDatesOptions{
		startDate: "2024-01-01",
		endDate:   "2024-12-31",
		seed:      "utc-rest",
		frequency: "medium",
		spread:    "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	restDays := restCalendarDaySet(plan.calendar)
	if len(restDays) == 0 {
		t.Fatal("calendar did not include rest days")
	}
	utcActiveDays := map[string]bool{}
	for _, candidate := range plan.candidates {
		for _, commitIndex := range candidate.selected {
			utcActiveDays[time.Unix(candidate.commits[commitIndex].plannedEpoch, 0).UTC().Format("2006-01-02")] = true
		}
	}
	loc := locationForTimezoneOffset(plan.tzOffset)
	for day := range restDays {
		activityDay := time.Unix(day, 0).In(loc).Format("2006-01-02")
		if utcActiveDays[activityDay] {
			t.Fatalf("rest day %s has activity in UTC day model", activityDay)
		}
	}
}

func TestRewriteDateFilterArgsExcludeGitWranglerRefs(t *testing.T) {
	args := rewriteDateFilterArgs([]dateBranchRef{
		{Name: "refs/heads/main"},
		{Name: "refs/git-wrangler/state/rewrite-dates"},
	}, "callback.py")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "refs/heads/main") {
		t.Fatalf("missing local branch ref: %v", args)
	}
	if strings.Contains(joined, "refs/git-wrangler") {
		t.Fatalf("included internal refs: %v", args)
	}
}

func TestRollbackRewritesDoesNotRequireFilterRepo(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	filterRepoLookups := 0
	filterRepoRuns := 0
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			switch name {
			case "git":
				return "/usr/bin/git", nil
			case "git-filter-repo":
				filterRepoLookups++
				return "", exec.ErrNotFound
			default:
				return "", exec.ErrNotFound
			}
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name == "git-filter-repo" || (name == "git" && len(args) > 0 && args[0] == "filter-repo") {
				filterRepoRuns++
				return "", "", errors.New("filter-repo should not run")
			}
			joined := name + " " + strings.Join(args, " ")
			switch {
			case joined == "git rev-parse HEAD":
				return "", "", errors.New("unborn")
			default:
				return "", "", errors.New("unexpected command: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"rollback-rewrites", "--repo", repoDir, "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("rollback-rewrites failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	if filterRepoLookups != 0 || filterRepoRuns != 0 {
		t.Fatalf("filter-repo was used: lookups=%d runs=%d", filterRepoLookups, filterRepoRuns)
	}
}

func TestRollbackRewritesInvalidBaselineFailsBeforeMutation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	baselineDir := filepath.Join(repoDir, ".git", "git-wrangler", "baseline")
	if err := os.MkdirAll(baselineDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baselineDir, "manifest.json"), []byte(`{"version":1,"entries":[{"first_sha":"original","current_sha":"rewritten","tree_sha":"tree","author_date":"100 +0000","committer_date":"100 +0000","capture_id":"capture","bundle_path":"bundles/missing.bundle"}]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mutated := false
	filterRepoLookups := 0
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			switch name {
			case "git":
				return "/usr/bin/git", nil
			case "git-filter-repo":
				filterRepoLookups++
				return "", exec.ErrNotFound
			default:
				return "", exec.ErrNotFound
			}
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name == "git-filter-repo" || (name == "git" && len(args) > 0 && args[0] == "filter-repo") {
				mutated = true
				return "", "", errors.New("filter-repo should not run")
			}
			joined := name + " " + strings.Join(args, " ")
			switch {
			case joined == "git rev-parse HEAD":
				return "rewritten\n", "", nil
			case len(args) > 0 && args[0] == "update-ref":
				mutated = true
				return "", "", errors.New("update-ref should not run")
			case joined == "git hash-object -w --stdin":
				mutated = true
				return "", "", errors.New("hash-object should not run")
			default:
				return "", "", errors.New("unexpected command: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"rollback-rewrites", "--repo", repoDir, "--yes"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if mutated {
		t.Fatalf("rollback mutated refs or state after missing branch metadata\nstdout:%s\nstderr:%s", stdout.String(), stderr.String())
	}
	if filterRepoLookups != 0 {
		t.Fatalf("rollback looked up filter-repo after missing branch metadata: %d", filterRepoLookups)
	}
	if !strings.Contains(stderr.String(), "rewrite baseline bundle") {
		t.Fatalf("missing baseline error not reported:\nstdout:%s\nstderr:%s", stdout.String(), stderr.String())
	}
}

func TestRewriteDatesDirtyRepoFailsBeforeMutation(t *testing.T) {
	requireGitFilterRepoForTest(t)
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	runGitForTest(t, "", "init", repoDir)
	runGitForTest(t, repoDir, "config", "user.name", "Test User")
	runGitForTest(t, repoDir, "config", "user.email", "test@example.test")
	commitEmptyForTest(t, repoDir, "first", "2020-01-01T10:00:00 +0000")
	commitEmptyForTest(t, repoDir, "second", "2020-01-02T10:00:00 +0000")
	originalHead := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD"))
	dirtyPath := filepath.Join(repoDir, "dirty.txt")
	if err := os.WriteFile(dirtyPath, []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithIO([]string{"rewrite-dates", "--repo", repoDir, "--no-fetch", "--start-date", "2024-01-01", "--end-date", "2024-01-10", "--seed", "test-seed", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if !strings.Contains(stderr.String(), "working tree must be clean before rewriting dates") {
		t.Fatalf("missing dirty working tree error:\nstdout:%s\nstderr:%s", stdout.String(), stderr.String())
	}
	if head := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD")); head != originalHead {
		t.Fatalf("HEAD changed from %s to %s", originalHead, head)
	}
	if data, err := os.ReadFile(dirtyPath); err != nil || string(data) != "keep me\n" {
		t.Fatalf("dirty file was changed or removed: data=%q err=%v", string(data), err)
	}
}

func TestRewriteHoursTempRepoWindowPreservesCalendarDates(t *testing.T) {
	requireGitFilterRepoForTest(t)
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	runGitForTest(t, "", "init", repoDir)
	runGitForTest(t, repoDir, "config", "user.name", "Test User")
	runGitForTest(t, repoDir, "config", "user.email", "test@example.test")
	commitEmptyForTest(t, repoDir, "first", "2024-02-01T02:00:00 +0000")
	commitEmptyForTest(t, repoDir, "second", "2024-02-02T22:00:00 +0000")
	commitEmptyForTest(t, repoDir, "third", "2024-02-03T23:30:00 +0000")
	before := commitAuthorDatesBySubject(t, repoDir)

	var stdout, stderr bytes.Buffer
	err := ExecuteWithIO([]string{"rewrite-hours", "--repo", repoDir, "--no-fetch", "--window", "09:00-10:00", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("rewrite-hours failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	after := commitAuthorDatesBySubject(t, repoDir)
	for subject, beforeEpoch := range before {
		afterEpoch := after[subject]
		if floorDayInOffset(afterEpoch, "+0000") != floorDayInOffset(beforeEpoch, "+0000") {
			t.Fatalf("%s day changed from %s to %s", subject, formatEpoch(beforeEpoch, "+0000"), formatEpoch(afterEpoch, "+0000"))
		}
		if !epochInCommitTimeWindow(afterEpoch, "+0000", commitTimeWindow{StartMinute: 9 * 60, EndMinute: 10 * 60, Text: "09:00-10:00"}) {
			t.Fatalf("%s not inside window: %s", subject, formatEpoch(afterEpoch, "+0000"))
		}
	}
	committerDates := commitCommitterDatesBySubject(t, repoDir)
	for subject, committerEpoch := range committerDates {
		if committerEpoch != after[subject] {
			t.Fatalf("%s committer date %s does not match author date %s", subject, formatEpoch(committerEpoch, "+0000"), formatEpoch(after[subject], "+0000"))
		}
	}
}

func TestRewriteDatesTempRepoRewriteAndRollback(t *testing.T) {
	requireGitFilterRepoForTest(t)
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	runGitForTest(t, "", "init", repoDir)
	runGitForTest(t, repoDir, "config", "user.name", "Test User")
	runGitForTest(t, repoDir, "config", "user.email", "test@example.test")
	commitEmptyForTest(t, repoDir, "first", "2020-01-01T10:00:00 +0000")
	commitEmptyForTest(t, repoDir, "second", "2020-01-02T10:00:00 +0000")
	commitEmptyForTest(t, repoDir, "third", "2020-01-03T10:00:00 +0000")
	originalSHAs := commitSHAsBySubject(t, repoDir)

	var stdout, stderr bytes.Buffer
	err := ExecuteWithIO([]string{"rewrite-dates", "--repo", repoDir, "--no-fetch", "--start-date", "2024-01-01", "--end-date", "2024-01-10", "--seed", "test-seed", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("rewrite-dates failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Planning timezone") || !strings.Contains(stdout.String(), "Active days") {
		t.Fatalf("--repo preview did not use calendar model:\nstdout:%s\nstderr:%s", stdout.String(), stderr.String())
	}
	rewrittenDates := commitAuthorDatesBySubject(t, repoDir)
	for _, subject := range []string{"first", "second", "third"} {
		if rewrittenDates[subject] < parseDateStartInOffset("2024-01-01", "+0000") || rewrittenDates[subject] > parseDateEndInOffset("2024-01-10", "+0000") {
			t.Fatalf("%s date outside target range: %s", subject, formatEpochLocal(rewrittenDates[subject]))
		}
	}
	if rewrittenDates["first"] > rewrittenDates["second"] || rewrittenDates["second"] > rewrittenDates["third"] {
		t.Fatalf("applied dates are not ordered: first=%s second=%s third=%s", formatEpochLocal(rewrittenDates["first"]), formatEpochLocal(rewrittenDates["second"]), formatEpochLocal(rewrittenDates["third"]))
	}

	commitEmptyForTest(t, repoDir, "new", "2025-01-01T10:00:00 +0000")
	newBeforeRollback := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD"))
	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithIO([]string{"rollback-rewrites", "--repo", repoDir, "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("rewrite-dates rollback failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	rolledBackDates := commitAuthorDatesBySubject(t, repoDir)
	want := map[string]int64{
		"first":  parseGitDateForTest(t, "2020-01-01T10:00:00 +0000"),
		"second": parseGitDateForTest(t, "2020-01-02T10:00:00 +0000"),
		"third":  parseGitDateForTest(t, "2020-01-03T10:00:00 +0000"),
		"new":    parseGitDateForTest(t, "2025-01-01T10:00:00 +0000"),
	}
	for subject, wantEpoch := range want {
		if rolledBackDates[subject] != wantEpoch {
			t.Fatalf("%s date = %s, want %s", subject, formatEpochLocal(rolledBackDates[subject]), formatEpochLocal(wantEpoch))
		}
	}
	rolledBackSHAs := commitSHAsBySubject(t, repoDir)
	for _, subject := range []string{"first", "second", "third"} {
		if rolledBackSHAs[subject] != originalSHAs[subject] {
			t.Fatalf("%s SHA = %s, want original %s", subject, rolledBackSHAs[subject], originalSHAs[subject])
		}
	}
	if rolledBackSHAs["new"] == newBeforeRollback {
		t.Fatal("new commit was not replayed onto the restored original base")
	}
	parents := commitParentsBySubject(t, repoDir)
	if got, want := parents["new"], []string{originalSHAs["third"]}; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("new parents = %v, want %v", got, want)
	}
}

func TestRewriteDatesTempRepoRepeatedRewriteRollbackRestoresOriginalBaseline(t *testing.T) {
	requireGitFilterRepoForTest(t)
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	runGitForTest(t, "", "init", repoDir)
	runGitForTest(t, repoDir, "config", "user.name", "Test User")
	runGitForTest(t, repoDir, "config", "user.email", "test@example.test")
	commitEmptyForTest(t, repoDir, "first", "2020-01-01T10:00:00 +0000")
	commitEmptyForTest(t, repoDir, "second", "2020-01-02T10:00:00 +0000")
	commitEmptyForTest(t, repoDir, "third", "2020-01-03T10:00:00 +0000")
	originalSHAs := commitSHAsBySubject(t, repoDir)
	originalHead := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD"))

	var stdout, stderr bytes.Buffer
	err := ExecuteWithIO([]string{"rewrite-dates", "--repo", repoDir, "--no-fetch", "--start-date", "2024-01-01", "--end-date", "2024-01-10", "--seed", "first-seed", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("first rewrite failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithIO([]string{"rewrite-dates", "--repo", repoDir, "--no-fetch", "--start-date", "2025-01-01", "--end-date", "2025-01-10", "--seed", "second-seed", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("second rewrite failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	if head := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD")); head == originalHead {
		t.Fatal("second rewrite did not change HEAD")
	}

	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithIO([]string{"rollback-rewrites", "--repo", repoDir, "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("rollback failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	rolledBackSHAs := commitSHAsBySubject(t, repoDir)
	for _, subject := range []string{"first", "second", "third"} {
		if rolledBackSHAs[subject] != originalSHAs[subject] {
			t.Fatalf("%s SHA = %s, want original %s", subject, rolledBackSHAs[subject], originalSHAs[subject])
		}
	}
	rolledBackDates := commitAuthorDatesBySubject(t, repoDir)
	wantDates := map[string]int64{
		"first":  parseGitDateForTest(t, "2020-01-01T10:00:00 +0000"),
		"second": parseGitDateForTest(t, "2020-01-02T10:00:00 +0000"),
		"third":  parseGitDateForTest(t, "2020-01-03T10:00:00 +0000"),
	}
	for subject, wantEpoch := range wantDates {
		if rolledBackDates[subject] != wantEpoch {
			t.Fatalf("%s date = %s, want %s", subject, formatEpochLocal(rolledBackDates[subject]), formatEpochLocal(wantEpoch))
		}
	}

	headAfterRollback := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD"))
	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithIO([]string{"rollback-rewrites", "--repo", repoDir, "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("second rollback failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	if head := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD")); head != headAfterRollback {
		t.Fatalf("second rollback changed HEAD from %s to %s", headAfterRollback, head)
	}
}

func TestRewriteDatesTempRepoRollbackReplaysCommitIncludedInSecondRewrite(t *testing.T) {
	requireGitFilterRepoForTest(t)
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	runGitForTest(t, "", "init", repoDir)
	runGitForTest(t, repoDir, "config", "user.name", "Test User")
	runGitForTest(t, repoDir, "config", "user.email", "test@example.test")
	commitEmptyForTest(t, repoDir, "first", "2020-01-01T10:00:00 +0000")
	commitEmptyForTest(t, repoDir, "second", "2020-01-02T10:00:00 +0000")
	commitEmptyForTest(t, repoDir, "third", "2020-01-03T10:00:00 +0000")
	originalSHAs := commitSHAsBySubject(t, repoDir)

	var stdout, stderr bytes.Buffer
	err := ExecuteWithIO([]string{"rewrite-dates", "--repo", repoDir, "--no-fetch", "--start-date", "2024-01-01", "--end-date", "2024-01-10", "--seed", "first-seed", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("first rewrite failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	commitEmptyForTest(t, repoDir, "new", "2025-01-01T10:00:00 +0000")
	newBeforeSecondRewrite := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD"))

	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithIO([]string{"rewrite-dates", "--repo", repoDir, "--no-fetch", "--start-date", "2026-01-01", "--end-date", "2026-01-10", "--seed", "second-seed", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("second rewrite failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithIO([]string{"rollback-rewrites", "--repo", repoDir, "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("rollback failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	rolledBackDates := commitAuthorDatesBySubject(t, repoDir)
	want := map[string]int64{
		"first":  parseGitDateForTest(t, "2020-01-01T10:00:00 +0000"),
		"second": parseGitDateForTest(t, "2020-01-02T10:00:00 +0000"),
		"third":  parseGitDateForTest(t, "2020-01-03T10:00:00 +0000"),
		"new":    parseGitDateForTest(t, "2025-01-01T10:00:00 +0000"),
	}
	for subject, wantEpoch := range want {
		if rolledBackDates[subject] != wantEpoch {
			t.Fatalf("%s date = %s, want %s", subject, formatEpochLocal(rolledBackDates[subject]), formatEpochLocal(wantEpoch))
		}
	}
	rolledBackSHAs := commitSHAsBySubject(t, repoDir)
	for _, subject := range []string{"first", "second", "third"} {
		if rolledBackSHAs[subject] != originalSHAs[subject] {
			t.Fatalf("%s SHA = %s, want original %s", subject, rolledBackSHAs[subject], originalSHAs[subject])
		}
	}
	if rolledBackSHAs["new"] == newBeforeSecondRewrite {
		t.Fatal("new commit was not replayed onto the restored original base")
	}
	parents := commitParentsBySubject(t, repoDir)
	if got, wantParents := parents["new"], []string{originalSHAs["third"]}; strings.Join(got, " ") != strings.Join(wantParents, " ") {
		t.Fatalf("new parents = %v, want %v", got, wantParents)
	}

	headAfterRollback := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD"))
	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithIO([]string{"rollback-rewrites", "--repo", repoDir, "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("second rollback failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	if head := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD")); head != headAfterRollback {
		t.Fatalf("second rollback changed HEAD from %s to %s", headAfterRollback, head)
	}
}

func testRewriteDateCommit(hash string, epoch int64, parents ...string) rewriteDateCommit {
	return rewriteDateCommit{
		hash:                   hash,
		parents:                parents,
		authorEpoch:            epoch,
		authorTZ:               "+0000",
		authorDate:             fmt.Sprintf("%d +0000", epoch),
		committerEpoch:         epoch,
		committerTZ:            "+0000",
		committerDate:          fmt.Sprintf("%d +0000", epoch),
		originalSHA:            hash,
		originalAuthorEpoch:    epoch,
		originalAuthorTZ:       "+0000",
		originalAuthorDate:     fmt.Sprintf("%d +0000", epoch),
		originalCommitterEpoch: epoch,
		originalCommitterTZ:    "+0000",
		originalCommitterDate:  fmt.Sprintf("%d +0000", epoch),
	}
}

func requireGitFilterRepoForTest(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git-filter-repo"); err == nil {
		return
	}
	cmd := exec.Command("git", "filter-repo", "--version")
	if err := cmd.Run(); err != nil {
		t.Skip("git-filter-repo is not installed")
	}
}

func runGitForTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func commitEmptyForTest(t *testing.T, dir, subject, date string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", subject)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_DATE="+date,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git commit %s failed: %v\n%s", subject, err, string(out))
	}
}

func commitAuthorDatesBySubject(t *testing.T, dir string) map[string]int64 {
	t.Helper()
	out := runGitForTest(t, dir, "log", "--format=%s%x00%at")
	result := map[string]int64{}
	for _, line := range splitLines(out) {
		fields := strings.Split(line, "\x00")
		if len(fields) != 2 {
			t.Fatalf("malformed git log line %q", line)
		}
		epoch, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			t.Fatalf("malformed epoch %q: %v", fields[1], err)
		}
		result[fields[0]] = epoch
	}
	return result
}

func commitCommitterDatesBySubject(t *testing.T, dir string) map[string]int64 {
	t.Helper()
	out := runGitForTest(t, dir, "log", "--format=%s%x00%ct")
	result := map[string]int64{}
	for _, line := range splitLines(out) {
		fields := strings.Split(line, "\x00")
		if len(fields) != 2 {
			t.Fatalf("malformed git log line %q", line)
		}
		epoch, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			t.Fatalf("malformed epoch %q: %v", fields[1], err)
		}
		result[fields[0]] = epoch
	}
	return result
}

func commitSHAsBySubject(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := runGitForTest(t, dir, "log", "--format=%s%x00%H")
	result := map[string]string{}
	for _, line := range splitLines(out) {
		fields := strings.Split(line, "\x00")
		if len(fields) != 2 {
			t.Fatalf("malformed git log line %q", line)
		}
		result[fields[0]] = fields[1]
	}
	return result
}

func commitParentsBySubject(t *testing.T, dir string) map[string][]string {
	t.Helper()
	out := runGitForTest(t, dir, "log", "--format=%s%x00%P")
	result := map[string][]string{}
	for _, line := range splitLines(out) {
		fields := strings.Split(line, "\x00")
		if len(fields) != 2 {
			t.Fatalf("malformed git log line %q", line)
		}
		result[fields[0]] = strings.Fields(fields[1])
	}
	return result
}

func parseGitDateForTest(t *testing.T, value string) int64 {
	t.Helper()
	parsed, err := time.Parse("2006-01-02T15:04:05 -0700", value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Unix()
}

func assertEpochsEqual(t *testing.T, left, right []int64) {
	t.Helper()
	if !epochsEqual(left, right) {
		t.Fatalf("epochs differ:\nleft:  %v\nright: %v", left, right)
	}
}

func epochsEqual(left, right []int64) bool {
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

func assertSortedEpochsInRange(t *testing.T, epochs []int64, start, end int64) {
	t.Helper()
	for i, epoch := range epochs {
		if epoch < start || epoch > end {
			t.Fatalf("timestamp %d outside range: %s", i, formatEpochLocal(epoch))
		}
		if i > 0 && epoch < epochs[i-1] {
			t.Fatalf("timestamps are not sorted at index %d: %s before %s", i, formatEpochLocal(epoch), formatEpochLocal(epochs[i-1]))
		}
	}
}

func activeDayCount(epochs []int64, tzOffset string) int {
	return len(activeDaySet(epochs, tzOffset))
}

func weekendActiveDayCount(epochs []int64, tzOffset string) int {
	count := 0
	for day := range activeDaySet(epochs, tzOffset) {
		if isWeekendInOffset(day, tzOffset) {
			count++
		}
	}
	return count
}

func maxInactiveGapDays(epochs []int64, tzOffset string) int {
	days := make([]int64, 0, len(activeDaySet(epochs, tzOffset)))
	for day := range activeDaySet(epochs, tzOffset) {
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
	maxGap := 0
	for i := 1; i < len(days); i++ {
		gap := int((days[i]-days[i-1])/86400) - 1
		if gap > maxGap {
			maxGap = gap
		}
	}
	return maxGap
}

func activeDaySet(epochs []int64, tzOffset string) map[int64]bool {
	days := map[int64]bool{}
	for _, epoch := range epochs {
		days[floorDayInOffset(epoch, tzOffset)] = true
	}
	return days
}

func multiDayInactiveGapCount(epochs []int64, tzOffset string, minGap int) int {
	days := make([]int64, 0, len(activeDaySet(epochs, tzOffset)))
	for day := range activeDaySet(epochs, tzOffset) {
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
	count := 0
	for i := 1; i < len(days); i++ {
		gap := int((days[i]-days[i-1])/86400) - 1
		if gap >= minGap {
			count++
		}
	}
	return count
}

func calendarSignature(calendar rewriteDateCalendarPlan) string {
	parts := make([]string, 0, len(calendar.days))
	for _, day := range calendar.days {
		parts = append(parts, fmt.Sprintf("%d:%s:%d", day.epoch, day.state, day.quota))
	}
	return strings.Join(parts, "|")
}

func calendarPlacementSignature(calendar rewriteDateCalendarPlan) string {
	parts := make([]string, 0, len(calendar.days))
	for _, day := range calendar.days {
		parts = append(parts, fmt.Sprintf("%d:%s", day.epoch, day.state))
	}
	return strings.Join(parts, "|")
}

func activeCalendarCoverageWindowCount(calendar rewriteDateCalendarPlan, windowCount int) int {
	if len(calendar.days) == 0 || windowCount <= 0 {
		return 0
	}
	covered := map[int]bool{}
	for i, day := range calendar.days {
		if calendarDayHasSlots(day.state) {
			window := i * windowCount / len(calendar.days)
			if window >= windowCount {
				window = windowCount - 1
			}
			covered[window] = true
		}
	}
	return len(covered)
}

func activeCalendarDayLabels(calendar rewriteDateCalendarPlan) string {
	loc := locationForTimezoneOffset(calendar.tzOffset)
	labels := []string{}
	for _, day := range calendar.days {
		if calendarDayHasSlots(day.state) {
			labels = append(labels, time.Unix(day.epoch, 0).In(loc).Format("2006-01-02"))
		}
	}
	return strings.Join(labels, ", ")
}

func calendarDailyLoadStats(calendar rewriteDateCalendarPlan) (int, int, int) {
	loads := make([]int, 0, calendarActiveDayCount(calendar))
	for _, day := range calendar.days {
		if calendarDayHasSlots(day.state) {
			loads = append(loads, day.quota)
		}
	}
	sort.Ints(loads)
	if len(loads) == 0 {
		return 0, 0, 0
	}
	return loads[(len(loads)-1)/2], loads[int(math.Ceil(float64(len(loads))*0.90))-1], loads[len(loads)-1]
}

func calendarLoadRange(calendar rewriteDateCalendarPlan) int {
	_, _, maximum := calendarDailyLoadStats(calendar)
	minimum := maximum
	for _, day := range calendar.days {
		if calendarDayHasSlots(day.state) && day.quota < minimum {
			minimum = day.quota
		}
	}
	return maximum - minimum
}

func restCalendarDaySet(calendar rewriteDateCalendarPlan) map[int64]bool {
	days := map[int64]bool{}
	for _, day := range calendar.days {
		if day.state == rewriteDateCalendarRest {
			days[day.epoch] = true
		}
	}
	return days
}

func plannedCommitDays(candidate dateCandidate, tzOffset string) []int64 {
	days := make([]int64, 0, len(candidate.selected))
	for _, commitIndex := range candidate.selected {
		days = append(days, floorDayInOffset(candidate.commits[commitIndex].plannedEpoch, tzOffset))
	}
	return days
}

func testRewriteDateCommits(prefix string, count int, start, step int64) []rewriteDateCommit {
	commits := make([]rewriteDateCommit, 0, count)
	for i := 0; i < count; i++ {
		commits = append(commits, testRewriteDateCommit(fmt.Sprintf("%s%03d", prefix, i), start+int64(i)*step))
	}
	return commits
}

func indexesForCount(count int) []int {
	indexes := make([]int, count)
	for i := range indexes {
		indexes[i] = i
	}
	return indexes
}

func testRewriteDateCandidate(name string, commits []rewriteDateCommit, selected []int) dateCandidate {
	for _, idx := range selected {
		commits[idx].selected = true
	}
	return dateCandidate{
		repo:     repo{display: name},
		branches: []dateBranchRef{{Name: "refs/heads/main", SHA: "head"}},
		commits:  commits,
		selected: selected,
		tzOffset: "+0000",
	}
}
