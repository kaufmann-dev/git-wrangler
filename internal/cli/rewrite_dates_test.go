package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func TestWeekdayAndWeekend(t *testing.T) {
	// Thursday (Epoch 0)
	if wd := weekdayFromEpoch(0); wd != 4 {
		t.Errorf("expected 4 for epoch 0, got %d", wd)
	}
	if isWeekend(0) {
		t.Error("expected epoch 0 (Thursday) to not be a weekend")
	}

	// Saturday (1970-01-03: epoch 172800)
	if wd := weekdayFromEpoch(172800); wd != 6 {
		t.Errorf("expected 6 for epoch 172800, got %d", wd)
	}
	if !isWeekend(172800) {
		t.Error("expected epoch 172800 (Saturday) to be a weekend")
	}

	// Sunday (1970-01-04: epoch 259200)
	if wd := weekdayFromEpoch(259200); wd != 0 {
		t.Errorf("expected 0 for epoch 259200, got %d", wd)
	}
	if !isWeekend(259200) {
		t.Error("expected epoch 259200 (Sunday) to be a weekend")
	}

	// Monday (1970-01-05: epoch 345600)
	if wd := weekdayFromEpoch(345600); wd != 1 {
		t.Errorf("expected 1 for epoch 345600, got %d", wd)
	}
	if isWeekend(345600) {
		t.Error("expected epoch 345600 (Monday) to not be a weekend")
	}
}

func TestWriteDateCallbackUsesBytesLiterals(t *testing.T) {
	path, err := writeDateCallback(map[string]int64{"abc123": 1600000000}, "+0200")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `mapping[b'abc123'] = (b'1600000000 +0200', b'1600000000 +0200')`) {
		t.Fatalf("unexpected callback:\n%s", text)
	}
}

func TestFirstCommitEpochChecksMalformedOutput(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) >= 1 && args[0] == "log" {
			return "not-a-timestamp\n", "", nil
		}
		return "", "", nil
	}}
	a := newApp(context.Background(), runner, strings.NewReader(""), io.Discard, io.Discard)
	if _, err := firstCommitEpoch(a, "repo", "--reverse"); err == nil {
		t.Fatal("expected malformed timestamp error")
	}
}

func TestDistributeCommitTimes(t *testing.T) {
	commits := []commitTime{
		{hash: "a", epoch: 100},
		{hash: "b", epoch: 200},
		{hash: "c", epoch: 300},
	}
	// Use realistic Unix epochs (September 2020)
	start := int64(1600000000)
	end := int64(1600864000) // 10 days later

	mapping := distributeCommitTimes(commits, start, end)
	if len(mapping) != 3 {
		t.Fatalf("expected mapping length 3, got %d", len(mapping))
	}

	timeA, okA := mapping["a"]
	timeB, okB := mapping["b"]
	timeC, okC := mapping["c"]

	if !okA || !okB || !okC {
		t.Fatal("missing mapped hashes in result")
	}

	// Given date snap, hour shifts, and potential weekend shifts,
	// timestamps should fall roughly within [start - 2 days, end + 2 days].
	margin := int64(2 * 86400)
	if timeA < start-margin || timeA > end+margin {
		t.Errorf("timeA %d out of bounds [%d, %d]", timeA, start-margin, end+margin)
	}
	if timeB < start-margin || timeB > end+margin {
		t.Errorf("timeB %d out of bounds [%d, %d]", timeB, start-margin, end+margin)
	}
	if timeC < start-margin || timeC > end+margin {
		t.Errorf("timeC %d out of bounds [%d, %d]", timeC, start-margin, end+margin)
	}

	// The distributed times should be strictly sorted chronologically (monotonically increasing)
	if timeA >= timeB || timeB >= timeC {
		t.Errorf("expected strictly sorted times (A < B < C), got: A=%d, B=%d, C=%d", timeA, timeB, timeC)
	}
}

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
		{[]string{"rewrite-dates", "--intensity", "extreme"}, "--intensity must be low, medium, or high"},
		{[]string{"rewrite-dates", "--rewrite-after", "2024-02-01", "--rewrite-before", "2024-02-01"}, "--rewrite-after must be before --rewrite-before"},
		{[]string{"rewrite-dates", "--rollback", "--seed", "x"}, "--rollback cannot be combined"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithIO(tc.args, strings.NewReader(""), &stdout, &stderr)
		assertExitCode(t, err, 1)
		if !strings.Contains(stderr.String(), tc.want) {
			t.Fatalf("%v stderr:\n%s", tc.args, stderr.String())
		}
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
		intensity: "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.targetStart != parseDateStart("2024-01-25") {
		t.Fatalf("targetStart = %s", formatEpochLocal(plan.targetStart))
	}
	if plan.targetEnd != parseDateEnd("2024-01-31") {
		t.Fatalf("targetEnd = %s", formatEpochLocal(plan.targetEnd))
	}
}

func TestRewriteDateSeedSource(t *testing.T) {
	candidates := []dateCandidate{{state: rewriteDatesState{Seed: "state-seed"}}}
	if seed, source := rewriteDateSeed(rewriteDatesOptions{seed: "flag-seed"}, candidates); seed != "flag-seed" || source != "flag" {
		t.Fatalf("flag seed = %q/%q", seed, source)
	}
	if seed, source := rewriteDateSeed(rewriteDatesOptions{}, candidates); seed != "state-seed" || source != "state" {
		t.Fatalf("state seed = %q/%q", seed, source)
	}
	if seed, source := rewriteDateSeed(rewriteDatesOptions{}, []dateCandidate{{}}); seed == "" || source != "generated" {
		t.Fatalf("generated seed = %q/%q", seed, source)
	}
}

func TestRewriteDatesStateDoesNotOverwriteOriginalDates(t *testing.T) {
	state := rewriteDatesState{Commits: []rewriteDatesStateCommit{{
		OriginalSHA:            "old",
		CurrentSHA:             "current",
		OriginalAuthorDate:     "100 +0000",
		OriginalAuthorEpoch:    100,
		OriginalAuthorTZ:       "+0000",
		OriginalCommitterDate:  "100 +0000",
		OriginalCommitterEpoch: 100,
		OriginalCommitterTZ:    "+0000",
	}}}
	commits := []rewriteDateCommit{testRewriteDateCommit("current", 999)}
	applyRewriteDatesStateToCommits(state, commits)
	state = mergeRewriteDatesState(state, commits)
	if len(state.Commits) != 1 {
		t.Fatalf("state commits = %d", len(state.Commits))
	}
	if state.Commits[0].OriginalAuthorEpoch != 100 || state.Commits[0].OriginalAuthorDate != "100 +0000" {
		t.Fatalf("original author date was overwritten: %+v", state.Commits[0])
	}
}

func TestRewriteDatesCommitMapUpdatesOnlyCurrentSHA(t *testing.T) {
	state := rewriteDatesState{Commits: []rewriteDatesStateCommit{{
		OriginalSHA:            "original",
		CurrentSHA:             "old-current",
		OriginalAuthorDate:     "100 +0000",
		OriginalAuthorEpoch:    100,
		OriginalAuthorTZ:       "+0000",
		OriginalCommitterDate:  "100 +0000",
		OriginalCommitterEpoch: 100,
		OriginalCommitterTZ:    "+0000",
	}}}
	updated := updateRewriteDatesStateFromCommitMap(state, map[string]string{"old-current": "new-current"})
	if updated.Commits[0].CurrentSHA != "new-current" {
		t.Fatalf("CurrentSHA = %q", updated.Commits[0].CurrentSHA)
	}
	if updated.Commits[0].OriginalSHA != "original" || updated.Commits[0].OriginalAuthorEpoch != 100 {
		t.Fatalf("original fields changed: %+v", updated.Commits[0])
	}
}

func TestRewriteDatesExactRollbackStateUpdates(t *testing.T) {
	state := rewriteDatesState{Commits: []rewriteDatesStateCommit{
		{
			OriginalSHA:            "original",
			CurrentSHA:             "rewritten",
			OriginalAuthorDate:     "100 +0000",
			OriginalAuthorEpoch:    100,
			OriginalAuthorTZ:       "+0000",
			OriginalCommitterDate:  "100 +0000",
			OriginalCommitterEpoch: 100,
			OriginalCommitterTZ:    "+0000",
		},
		{
			OriginalSHA:            "new-before-rollback",
			CurrentSHA:             "new-before-rollback",
			OriginalAuthorDate:     "200 +0000",
			OriginalAuthorEpoch:    200,
			OriginalAuthorTZ:       "+0000",
			OriginalCommitterDate:  "200 +0000",
			OriginalCommitterEpoch: 200,
			OriginalCommitterTZ:    "+0000",
		},
	}}
	updated := updateRewriteDatesStateAfterExactRollback(state, map[string]string{"new-before-rollback": "new-after-rollback"})
	byOriginal := map[string]rewriteDatesStateCommit{}
	for _, commit := range updated.Commits {
		byOriginal[commit.OriginalSHA] = commit
	}
	if byOriginal["original"].CurrentSHA != "original" {
		t.Fatalf("known commit current SHA = %q", byOriginal["original"].CurrentSHA)
	}
	if byOriginal["new-before-rollback"].CurrentSHA != "new-after-rollback" {
		t.Fatalf("replayed commit current SHA = %q", byOriginal["new-before-rollback"].CurrentSHA)
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
		intensity: "medium",
	})
	if err == nil {
		t.Fatal("expected planning error")
	}
	if !strings.Contains(err.Error(), "repo") || !strings.Contains(err.Error(), "child123") {
		t.Fatalf("error lacks concrete repo/commit: %v", err)
	}
}

func TestRewriteDateRollbackSelectsOnlyKnownCommits(t *testing.T) {
	commits := []rewriteDateCommit{
		{hash: "known", knownInState: true},
		{hash: "unknown"},
	}
	got := rollbackSelectedIndexes(commits)
	if len(got) != 1 || got[0] != 0 {
		t.Fatalf("rollback indexes = %#v", got)
	}
}

func TestRewriteDateRollbackBranchClassification(t *testing.T) {
	t.Parallel()
	meta := rewriteDatesStateBranch{
		Name:          "refs/heads/main",
		OriginalHead:  "original",
		RewrittenHead: "rewritten",
		BackupRef:     "refs/git-wrangler/backup/rewrite-dates/run/heads/main",
		RunID:         "run",
	}
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git rev-parse --verify --quiet refs/git-wrangler/backup/rewrite-dates/run/heads/main^{commit}":
			return "original\n", "", nil
		case "git merge-base --is-ancestor original rewritten",
			"git merge-base --is-ancestor original child",
			"git merge-base --is-ancestor original diverged",
			"git merge-base --is-ancestor rewritten diverged":
			return "", "", errors.New("not ancestor")
		case "git merge-base --is-ancestor rewritten child":
			return "", "", nil
		case "git rev-list --count child --not rewritten":
			return "2\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}}
	a := newApp(context.Background(), runner, strings.NewReader(""), io.Discard, io.Discard)

	exact, err := classifyRewriteDatesRollbackBranch(a, "repo", dateBranchRef{Name: "refs/heads/main", SHA: "rewritten"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if exact.Action != dateRollbackExact {
		t.Fatalf("exact action = %s", exact.Action)
	}
	replay, err := classifyRewriteDatesRollbackBranch(a, "repo", dateBranchRef{Name: "refs/heads/main", SHA: "child"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Action != dateRollbackReplay || replay.ReplayCommits != 2 {
		t.Fatalf("replay plan = %+v", replay)
	}
	skip, err := classifyRewriteDatesRollbackBranch(a, "repo", dateBranchRef{Name: "refs/heads/main", SHA: "original"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if skip.Action != dateRollbackSkip {
		t.Fatalf("skip action = %s", skip.Action)
	}
	if _, err := classifyRewriteDatesRollbackBranch(a, "repo", dateBranchRef{Name: "refs/heads/main", SHA: "diverged"}, meta); err == nil {
		t.Fatal("expected unsafe divergent branch to fail")
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

func TestRewriteDatesStateBlobRefReadWrite(t *testing.T) {
	t.Run("write", func(t *testing.T) {
		var stdin string
		var updatedRef string
		runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := name + " " + strings.Join(args, " ")
			switch joined {
			case "git hash-object -w --stdin":
				stdin = run.GetStdin(ctx)
				return "blob123\n", "", nil
			case "git update-ref refs/git-wrangler/state/rewrite-dates blob123":
				updatedRef = args[1]
				return "", "", nil
			default:
				return "", "", nil
			}
		}}
		a := newApp(context.Background(), runner, strings.NewReader(""), io.Discard, io.Discard)
		err := writeRewriteDatesState(a, "repo", rewriteDatesState{
			Seed: "seed",
			Branches: []rewriteDatesStateBranch{{
				Name:          "refs/heads/main",
				OriginalHead:  "a",
				RewrittenHead: "b",
				BackupRef:     "refs/git-wrangler/backup/rewrite-dates/run/heads/main",
				RunID:         "run",
			}},
			Commits: []rewriteDatesStateCommit{{
				OriginalSHA:            "a",
				CurrentSHA:             "b",
				OriginalAuthorDate:     "100 +0000",
				OriginalAuthorEpoch:    100,
				OriginalAuthorTZ:       "+0000",
				OriginalCommitterDate:  "101 +0000",
				OriginalCommitterEpoch: 101,
				OriginalCommitterTZ:    "+0000",
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdin, `"schema_version": 2`) || !strings.Contains(stdin, `"seed": "seed"`) || !strings.Contains(stdin, `"original_sha": "a"`) || !strings.Contains(stdin, `"backup_ref": "refs/git-wrangler/backup/rewrite-dates/run/heads/main"`) {
			t.Fatalf("unexpected state stdin:\n%s", stdin)
		}
		if updatedRef != rewriteDatesStateRef {
			t.Fatalf("updated ref = %q", updatedRef)
		}
	})

	t.Run("read", func(t *testing.T) {
		runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := name + " " + strings.Join(args, " ")
			switch joined {
			case "git rev-parse --verify --quiet refs/git-wrangler/state/rewrite-dates":
				return "blob123\n", "", nil
			case "git cat-file -p blob123":
				return `{"schema_version":1,"seed":"seed","commits":[{"original_sha":"a","current_sha":"b","original_author_date":"100 +0000","original_author_epoch":100,"original_author_tz":"+0000","original_committer_date":"101 +0000","original_committer_epoch":101,"original_committer_tz":"+0000"}]}` + "\n", "", nil
			default:
				return "", "", nil
			}
		}}
		a := newApp(context.Background(), runner, strings.NewReader(""), io.Discard, io.Discard)
		state, found, err := readRewriteDatesState(a, "repo")
		if err != nil {
			t.Fatal(err)
		}
		if !found || state.SchemaVersion != rewriteDatesStateVersion || state.Seed != "seed" || len(state.Commits) != 1 || state.Commits[0].CurrentSHA != "b" || len(state.Branches) != 0 {
			t.Fatalf("state = found:%v %+v", found, state)
		}
	})
}

func TestRewriteDatesExactRollbackDoesNotRequireFilterRepo(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stateJSON := `{"schema_version":2,"seed":"seed","branches":[{"name":"refs/heads/main","original_head":"original","rewritten_head":"rewritten","backup_ref":"refs/git-wrangler/backup/rewrite-dates/run/heads/main","run_id":"run"}],"commits":[{"original_sha":"original","current_sha":"rewritten","original_author_date":"100 +0000","original_author_epoch":100,"original_author_tz":"+0000","original_committer_date":"100 +0000","original_committer_epoch":100,"original_committer_tz":"+0000"}]}`
	filterRepoLookups := 0
	filterRepoRuns := 0
	branchRestored := false
	hashObjects := 0
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
				return "rewritten\n", "", nil
			case joined == "git for-each-ref --format=%(refname)%00%(objectname) refs/heads":
				return "refs/heads/main\x00rewritten\n", "", nil
			case joined == "git rev-parse --verify --quiet refs/git-wrangler/state/rewrite-dates":
				return "stateblob\n", "", nil
			case joined == "git cat-file -p stateblob":
				return stateJSON + "\n", "", nil
			case len(args) > 0 && args[0] == "log" && strings.Contains(joined, "--topo-order"):
				return "rewritten\x00\x00100\x001970-01-01 00:01:40 +0000\x00100\x001970-01-01 00:01:40 +0000\x00N\x1e", "", nil
			case joined == "git rev-parse --verify --quiet refs/git-wrangler/backup/rewrite-dates/run/heads/main^{commit}":
				return "original\n", "", nil
			case joined == "git merge-base --is-ancestor original rewritten":
				return "", "", errors.New("not ancestor")
			case joined == "git status --porcelain":
				return "", "", nil
			case joined == "git for-each-ref --format=%(refname) refs/tags":
				return "", "", nil
			case joined == "git hash-object -w --stdin":
				hashObjects++
				return fmt.Sprintf("stateblob%d\n", hashObjects), "", nil
			case len(args) >= 3 && args[0] == "update-ref" && args[1] == rewriteDatesStateRef:
				return "", "", nil
			case len(args) >= 3 && args[0] == "update-ref" && strings.HasPrefix(args[1], rewriteDatesBackupPrefix+"/"):
				return "", "", nil
			case joined == "git update-ref refs/heads/main original rewritten":
				branchRestored = true
				return "", "", nil
			case len(args) >= 4 && args[0] == "rev-parse" && args[1] == "--verify" && args[2] == "--quiet" && strings.HasPrefix(args[3], rewriteDatesBackupPrefix+"/"):
				return "rewritten\n", "", nil
			case joined == "git symbolic-ref --quiet HEAD":
				return "refs/heads/main\n", "", nil
			case joined == "git reset --hard HEAD":
				return "", "", nil
			default:
				return "", "", errors.New("unexpected command: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"rewrite-dates", "--repo", repoDir, "--no-fetch", "--rollback", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("rewrite-dates rollback failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	if filterRepoLookups != 0 || filterRepoRuns != 0 {
		t.Fatalf("filter-repo was used: lookups=%d runs=%d", filterRepoLookups, filterRepoRuns)
	}
	if !branchRestored {
		t.Fatal("branch ref was not restored to original head")
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
	rewrittenDates := commitAuthorDatesBySubject(t, repoDir)
	for _, subject := range []string{"first", "second", "third"} {
		if rewrittenDates[subject] < parseDateStart("2024-01-01") || rewrittenDates[subject] > parseDateEnd("2024-01-10") {
			t.Fatalf("%s date outside target range: %s", subject, formatEpochLocal(rewrittenDates[subject]))
		}
	}

	commitEmptyForTest(t, repoDir, "new", "2025-01-01T10:00:00 +0000")
	newBeforeRollback := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD"))
	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithIO([]string{"rewrite-dates", "--repo", repoDir, "--no-fetch", "--rollback", "--yes"}, strings.NewReader(""), &stdout, &stderr)
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

func testRewriteDateCandidate(name string, commits []rewriteDateCommit, selected []int) dateCandidate {
	for _, idx := range selected {
		commits[idx].selected = true
	}
	state := mergeRewriteDatesState(rewriteDatesState{}, commits)
	return dateCandidate{
		repo:     repo{display: name},
		state:    state,
		branches: []dateBranchRef{{Name: "refs/heads/main", SHA: "head"}},
		commits:  commits,
		selected: selected,
		tzOffset: "+0000",
	}
}
