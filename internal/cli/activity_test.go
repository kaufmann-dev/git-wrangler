package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func activityRecord(hash, date, name, email string) string {
	return strings.Join([]string{hash, date, name, email, ""}, "\x00")
}

func TestParseActivityLogFiltersUsersYearAndUsesUTC(t *testing.T) {
	t.Parallel()
	log := activityRecord("one", "2025-12-31T23:30:00-02:00", "Alice", "alice@example.test") +
		activityRecord("two", "2026-02-03T12:00:00Z", "ALICE", "other@example.test") +
		activityRecord("three", "2026-02-04T12:00:00Z", "Bob", "bob@example.test")
	commits := map[string]activityDay{}
	err := parseActivityLog(strings.NewReader(log), 2026, map[string]struct{}{"alice": {}}, commits)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]activityDay{"one": "2026-01-01", "two": "2026-02-03"}
	if len(commits) != len(want) {
		t.Fatalf("commits = %#v", commits)
	}
	for hash, day := range want {
		if commits[hash] != day {
			t.Fatalf("%s day = %q, want %q", hash, commits[hash], day)
		}
	}
}

func TestActivityDefaultRefsPreferLocalAndIncludeGhPages(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		switch strings.Join(args, " ") {
		case "symbolic-ref --quiet --short refs/remotes/origin/HEAD":
			return "origin/main\n", "", nil
		case "rev-parse --verify --quiet refs/heads/main":
			return "", "", nil
		case "rev-parse --verify --quiet refs/heads/gh-pages":
			return "", "", errors.New("missing")
		case "rev-parse --verify --quiet refs/remotes/origin/gh-pages":
			return "", "", nil
		default:
			return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
		}
	}}
	a := newApp(context.Background(), runner, strings.NewReader(""), io.Discard, io.Discard)
	refs, fallback := activityDefaultRefs(a, repo{dir: "repo"})
	if fallback {
		t.Fatal("unexpected fallback")
	}
	if got, want := strings.Join(refs, ","), "refs/heads/main,refs/remotes/origin/gh-pages"; got != want {
		t.Fatalf("refs = %q, want %q", got, want)
	}
}

func TestActivityDefaultRefsFallBackToRemoteTrackingBranch(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		switch strings.Join(args, " ") {
		case "symbolic-ref --quiet --short refs/remotes/origin/HEAD":
			return "origin/trunk\n", "", nil
		case "rev-parse --verify --quiet refs/heads/trunk",
			"rev-parse --verify --quiet refs/heads/gh-pages",
			"rev-parse --verify --quiet refs/remotes/origin/gh-pages":
			return "", "", errors.New("missing")
		default:
			return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
		}
	}}
	a := newApp(context.Background(), runner, strings.NewReader(""), io.Discard, io.Discard)
	refs, fallback := activityDefaultRefs(a, repo{dir: "repo"})
	if fallback {
		t.Fatal("unexpected fallback")
	}
	if got, want := strings.Join(refs, ","), "refs/remotes/origin/trunk"; got != want {
		t.Fatalf("refs = %q, want %q", got, want)
	}
}

func TestActivityCommandDeduplicatesAndReportsPartialFailure(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one", "two", "failed")
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "origin/main\n", "", nil
			case "rev-parse":
				if args[len(args)-1] == "refs/heads/main" {
					return "", "", nil
				}
				return "", "", errors.New("missing")
			case "log":
				if filepath.Base(dir) == "failed" {
					return "", "log failed", errors.New("failed")
				}
				common := activityRecord("shared", "2026-01-04T12:00:00Z", "Alice", "alice@example.test")
				if filepath.Base(dir) == "one" {
					return common + activityRecord("unique", "2026-01-05T12:00:00Z", "Alice", "alice@example.test"), "", nil
				}
				return common, "", nil
			default:
				return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
			}
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"activity", "--year", "2026"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if !strings.Contains(stdout.String(), "2026  2 commits") ||
		!strings.Contains(stdout.String(), "Summary: 2 commits, 3 repositories, 1 failed") {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "failed: could not scan activity") || !strings.Contains(stderr.String(), "log failed") {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}

func TestActivityAllUsesNormalRefsAndRepeatedUsers(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one")
	t.Chdir(root)
	var logArgs string
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if args[0] != "log" {
				return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
			}
			logArgs = strings.Join(args, " ")
			return activityRecord("one", "2026-01-04T12:00:00Z", "Alice", "alice@example.test") +
				activityRecord("two", "2026-01-05T12:00:00Z", "Bob", "bob@example.test") +
				activityRecord("three", "2026-01-06T12:00:00Z", "Carol", "carol@example.test"), "", nil
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"activity", "--all", "--year", "2026", "--user", "alice", "--user", "BOB@EXAMPLE.TEST"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("activity returned error: %v\n%s", err, stderr.String())
	}
	if !strings.Contains(logArgs, "--exclude=refs/git-wrangler/* --all") {
		t.Fatalf("log args = %q", logArgs)
	}
	if strings.Contains(logArgs, "--since") || strings.Contains(logArgs, "--until") {
		t.Fatalf("log args contain committer-date filter: %q", logArgs)
	}
	if !strings.Contains(stdout.String(), "2026  2 commits") {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
}

func TestActivityExactRepoTargeting(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	parent := makeGitDir(t, root, "parent")
	makeGitDir(t, parent, "nested")
	var scanned []string
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "origin/main\n", "", nil
			case "rev-parse":
				if args[len(args)-1] == "refs/heads/main" {
					return "", "", nil
				}
				return "", "", errors.New("missing")
			case "log":
				scanned = append(scanned, filepath.Base(dir))
				return "", "", nil
			default:
				return "", "", errors.New("unexpected command")
			}
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"activity", "--repo", parent}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(scanned, ","); got != "parent" {
		t.Fatalf("scanned = %q, want parent", got)
	}
}

func TestActivityFallbackWarningAndEmptyRequestedYear(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one")
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "", "", errors.New("missing")
			case "rev-parse":
				return "", "", errors.New("missing")
			case "log":
				if args[len(args)-1] != "HEAD" {
					t.Fatalf("log args = %v, want HEAD fallback", args)
				}
				return "", "", nil
			default:
				return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
			}
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"activity", "--year", "2024"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("activity returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "origin/HEAD unavailable; using current HEAD") {
		t.Fatalf("missing fallback warning:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "2024  0 commits  Max: 0/day") || !strings.Contains(stdout.String(), "Sun") {
		t.Fatalf("empty requested year did not render:\n%s", stdout.String())
	}
}

func TestActivityScalingAndCalendarLayout(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), nil, strings.NewReader(""), &stdout, &stderr)
	days := map[activityDay]int{
		"2025-01-01": 8,
		"2026-01-04": 2,
	}
	renderActivity(a, days, 0, false, nil, false)
	perYear := stdout.String()
	if !strings.Contains(perYear, "2026  2 commits  Max: 2/day") || !strings.Contains(perYear, "2025  8 commits  Max: 8/day") {
		t.Fatalf("per-year scale missing:\n%s", perYear)
	}
	if strings.Index(perYear, "2026  ") > strings.Index(perYear, "2025  ") {
		t.Fatalf("years not newest first:\n%s", perYear)
	}
	if !strings.Contains(perYear, "Sun    4") {
		t.Fatalf("Sunday-first activity missing:\n%s", perYear)
	}
	stdout.Reset()
	renderActivity(a, days, 0, false, nil, true)
	if got := strings.Count(stdout.String(), "Max: 8/day"); got != 3 {
		t.Fatalf("global max count = %d, want heading plus both years:\n%s", got, stdout.String())
	}
}

func TestActivityScansWithReadOnlyWorkerCap(t *testing.T) {
	t.Parallel()
	repos := make([]repo, 64)
	var active atomic.Int32
	var maximum atomic.Int32
	var started atomic.Int32
	var wg sync.WaitGroup
	release := make(chan struct{})
	for i := range repos {
		repos[i].display = fmt.Sprintf("%d", i)
	}
	wg.Add(readOnlyWorkerCount(len(repos)))
	done := make(chan struct{})
	go func() {
		parallelRepos(context.Background(), repos, func(r repo) struct{} {
			current := active.Add(1)
			for {
				max := maximum.Load()
				if current <= max || maximum.CompareAndSwap(max, current) {
					break
				}
			}
			if started.Add(1) <= int32(readOnlyWorkerCount(len(repos))) {
				wg.Done()
			}
			<-release
			active.Add(-1)
			return struct{}{}
		})
		close(done)
	}()
	wg.Wait()
	close(release)
	<-done
	if got := maximum.Load(); got != int32(readOnlyWorkerCount(len(repos))) || got > 32 {
		t.Fatalf("maximum workers = %d, want %d and <= 32", got, readOnlyWorkerCount(len(repos)))
	}
}

func TestActivityLevel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		count int
		max   int
		want  int
	}{
		{0, 8, 0},
		{1, 8, 1},
		{3, 8, 2},
		{5, 8, 3},
		{8, 8, 4},
	} {
		if got := activityLevel(tc.count, tc.max); got != tc.want {
			t.Fatalf("activityLevel(%d, %d) = %d, want %d", tc.count, tc.max, got, tc.want)
		}
	}
}

func TestActivityYearValidation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"activity", "--year", "0"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if !strings.Contains(stderr.String(), "--year must be from 1 through 9999") {
		t.Fatalf("stderr:\n%s", stderr.String())
	}
}

func TestActivityNoMatches(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one")
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "origin/main\n", "", nil
			case "rev-parse":
				if args[len(args)-1] == "refs/heads/main" {
					return "", "", nil
				}
				return "", "", errors.New("missing")
			case "log":
				return "", "", nil
			default:
				return "", "", errors.New("unexpected command")
			}
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"activity"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "No activity found.") || !strings.Contains(stdout.String(), "Summary: 0 commits, 1 repositories, 0 failed") {
		t.Fatalf("stdout:\n%s", stdout.String())
	}
}

func TestParseActivityLogStreamsRecords(t *testing.T) {
	t.Parallel()
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	commits := map[string]activityDay{}
	go func() {
		done <- parseActivityLog(reader, 0, nil, commits)
	}()
	for i := 0; i < 100; i++ {
		_, err := io.WriteString(writer, activityRecord(fmt.Sprintf("%040d", i), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339), "A", "a@example.test"))
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(commits) != 100 {
		t.Fatalf("commits = %d, want 100", len(commits))
	}
}
