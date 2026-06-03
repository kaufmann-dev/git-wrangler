package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/ai"
)

func TestRunFilterRepoRestoresOriginAfterFailure(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	remoteGetCalls := 0
	restored := false
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if dir != "repo" {
			return "", "", errors.New("unexpected dir")
		}
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git remote get-url origin":
			remoteGetCalls++
			if remoteGetCalls == 1 {
				return "https://example.test/repo.git\n", "", nil
			}
			return "", "", errors.New("origin removed")
		case "git-filter-repo --force":
			return "partial rewrite output", "", errors.New("filter failed")
		case "git remote add origin https://example.test/repo.git":
			restored = true
			return "", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}}
	a := newApp(context.Background(), runner, strings.NewReader(""), io.Discard, io.Discard)

	out, runErr, restoreErr := runFilterRepoRestoringOrigin(a, "repo", []string{"git-filter-repo"}, []string{"--force"}, nil)
	if runErr == nil {
		t.Fatal("expected filter failure")
	}
	if restoreErr != nil {
		t.Fatalf("unexpected restore error: %v", restoreErr)
	}
	if out != "partial rewrite output" {
		t.Fatalf("output = %q", out)
	}
	if !restored {
		t.Fatal("expected origin restore after filter failure")
	}
}

func TestApplyAIPlanRunsFilterRepoInParallelWithOrderedOutput(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var mu sync.Mutex
	activeFilters := 0
	maxActiveFilters := 0
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case joined == "git remote get-url origin":
			return "https://example.test/" + dir + ".git\n", "", nil
		case strings.HasPrefix(joined, "git-filter-repo --partial --commit-callback"):
			mu.Lock()
			activeFilters++
			if activeFilters > maxActiveFilters {
				maxActiveFilters = activeFilters
			}
			mu.Unlock()
			if dir == "repo-a" {
				time.Sleep(50 * time.Millisecond)
			}
			time.Sleep(25 * time.Millisecond)
			mu.Lock()
			activeFilters--
			mu.Unlock()
			return "rewritten\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}}
	var stdout bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, io.Discard)

	status := applyAIPlan(a, &ai.Plan{Repos: []ai.RepoPlan{
		{Dir: "repo-a", Name: "repo-a", CallbackFile: "callback-a.py", ChangedCount: 1},
		{Dir: "repo-b", Name: "repo-b", CallbackFile: "callback-b.py", ChangedCount: 2},
	}}, []string{"git-filter-repo"})
	if status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	if maxActiveFilters < 2 {
		t.Fatalf("git-filter-repo runs did not overlap; max active = %d", maxActiveFilters)
	}
	out := stdout.String()
	if !strings.Contains(out, "Rewrote 3 commit message(s) across 2 repositories.") {
		t.Fatalf("missing aggregate rewrite summary:\n%s", out)
	}
	if strings.Contains(out, "for repo-a") || strings.Contains(out, "for repo-b") {
		t.Fatalf("per-repository success output was printed:\n%s", out)
	}
}
