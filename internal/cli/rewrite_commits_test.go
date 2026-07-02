package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/ai"
)

func TestRunFilterRepoRestoresOriginAfterFailure(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	gitDir := filepath.Join(t.TempDir(), ".git")
	marker := filepath.Join(gitDir, "filter-repo", "already_ran")
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("old run"), 0o644); err != nil {
		t.Fatal(err)
	}
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
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				return "", "", errors.New("stale git-filter-repo marker was not removed")
			}
			return "partial rewrite output", "", errors.New("filter failed")
		case "git remote add origin https://example.test/repo.git":
			restored = true
			return "", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}}
	a := newApp(context.Background(), runner, strings.NewReader(""), io.Discard, io.Discard)

	out, runErr, restoreErr := runFilterRepoRestoringOrigin(a, "repo", gitDir, []string{"git-filter-repo"}, []string{"--force"}, nil)
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

func TestRemoveFilterRepoAlreadyRanResolvesLinkedWorktreeGitDir(t *testing.T) {
	root := t.TempDir()
	metadataDir := filepath.Join(root, "metadata")
	marker := filepath.Join(metadataDir, "filter-repo", "already_ran")
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("old run"), 0o644); err != nil {
		t.Fatal(err)
	}
	worktreeDir := filepath.Join(root, "worktree")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(worktreeDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: ../metadata\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := removeFilterRepoAlreadyRan(gitFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker still exists: %v", err)
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
	if !strings.Contains(out, "Summary: 3 commit messages rewritten, 2 repositories updated, 0 failed") {
		t.Fatalf("missing aggregate rewrite summary:\n%s", out)
	}
	if strings.Contains(out, "for repo-a") || strings.Contains(out, "for repo-b") {
		t.Fatalf("per-repository success output was printed:\n%s", out)
	}
}

func TestApplyAIPlanReportsUnderlyingErrorWhenOutputIsEmpty(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		if strings.HasPrefix(joined, "git update-ref refs/git-wrangler/baseline-capture/") {
			return "", "", errors.New("update-ref failed")
		}
		return "", "", errors.New("unexpected command: " + joined)
	}}
	var stdout, stderr bytes.Buffer
	gitDir := filepath.Join(t.TempDir(), ".git")
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)

	status := applyAIPlan(a, &ai.Plan{Repos: []ai.RepoPlan{{
		Dir:           "repo",
		Name:          "repo",
		GitDir:        gitDir,
		CallbackFile:  "callback.py",
		ChangedCount:  1,
		ChangedHashes: []string{"abcdef1234567890"},
	}}}, []string{"git-filter-repo"})
	if status == 0 {
		t.Fatal("expected apply failure")
	}
	if !strings.Contains(stderr.String(), "update-ref failed") {
		t.Fatalf("missing underlying error:\n%s", stderr.String())
	}
}
