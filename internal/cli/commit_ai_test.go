package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
)

func TestCommitAIStagesSkipsCommitsAndReportsSummary(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := aiResponseServer(t, `{"messages":[{"id":"c000001","subject":"feat(dirty): update file"}]}`)
	defer server.Close()
	saveAIConfig(t, server.URL)

	root := t.TempDir()
	makeGitDir(t, root, "dirty")
	makeGitDir(t, root, "clean")
	var commits []string
	realAdds := 0
	tempAdds := 0
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" {
				return "/usr/bin/git", nil
			}
			return "", errors.New("unexpected lookpath")
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" {
				return "", "", errors.New("unexpected command: " + name)
			}
			joined := strings.Join(args, " ")
			repoName := filepath.Base(dir)
			tempIndex := hasGitIndexEnv(env)
			switch {
			case joined == "rev-parse --verify --quiet HEAD":
				return "head\n", "", nil
			case tempIndex && joined == "read-tree HEAD":
				return "", "", nil
			case tempIndex && joined == "add -A":
				tempAdds++
				return "", "", nil
			case !tempIndex && joined == "add -A":
				realAdds++
				return "", "", nil
			case tempIndex && repoName == "clean" && joined == "diff --cached --quiet":
				return "", "", nil
			case tempIndex && repoName == "dirty" && joined == "diff --cached --quiet":
				return "", "", errors.New("dirty")
			case tempIndex && repoName == "dirty" && joined == "diff --cached --name-status":
				return "M\tfile.txt\n", "", nil
			case tempIndex && repoName == "dirty" && joined == "diff --cached --numstat":
				return "1\t0\tfile.txt\n", "", nil
			case tempIndex && repoName == "dirty" && joined == "diff --cached --no-color --no-ext-diff --find-renames --find-copies --unified=3 -- file.txt":
				return "diff --git a/file.txt b/file.txt\n+hello\n", "", nil
			case !tempIndex && repoName == "dirty" && joined == "commit -m feat(dirty): update file":
				commits = append(commits, joined)
				return "committed\n", "", nil
			default:
				return "", "", errors.New("unexpected git command in " + dir + ": " + joined)
			}
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
	a.creds = &fakeCredentialStore{values: map[string]string{credentials.AIAccount("openai"): "test-key"}}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"commit-ai", "--yes"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	t.Chdir(root)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit-ai returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if len(commits) != 1 {
		t.Fatalf("commits = %#v", commits)
	}
	if tempAdds != 2 {
		t.Fatalf("temp-index adds = %d, want 2", tempAdds)
	}
	if realAdds != 1 {
		t.Fatalf("real-index adds = %d, want 1", realAdds)
	}
	if !strings.Contains(stdout.String(), "Summary: 1 committed, 1 skipped, 0 failed") {
		t.Fatalf("missing summary:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestCommitAIWithBodyUsesSecondCommitMessageFlag(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := aiResponseServer(t, `{"messages":[{"id":"c000001","subject":"feat(dirty): update file","body":"Explain why this change was made."}]}`)
	defer server.Close()
	saveAIConfig(t, server.URL)

	root := t.TempDir()
	makeGitDir(t, root, "dirty")
	var commitArgs []string
	realAdds := 0
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" {
				return "/usr/bin/git", nil
			}
			return "", errors.New("unexpected lookpath")
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := strings.Join(args, " ")
			tempIndex := hasGitIndexEnv(env)
			switch {
			case joined == "rev-parse --verify --quiet HEAD":
				return "head\n", "", nil
			case tempIndex && joined == "read-tree HEAD":
				return "", "", nil
			case tempIndex && joined == "add -A":
				return "", "", nil
			case !tempIndex && joined == "add -A":
				realAdds++
				return "", "", nil
			case tempIndex && joined == "diff --cached --quiet":
				return "", "", errors.New("dirty")
			case tempIndex && joined == "diff --cached --name-status":
				return "M\tfile.txt\n", "", nil
			case tempIndex && joined == "diff --cached --numstat":
				return "1\t0\tfile.txt\n", "", nil
			case tempIndex && joined == "diff --cached --no-color --no-ext-diff --find-renames --find-copies --unified=3 -- file.txt":
				return "diff --git a/file.txt b/file.txt\n+hello\n", "", nil
			case !tempIndex && strings.HasPrefix(joined, "commit "):
				commitArgs = append(commitArgs, joined)
				return "committed\n", "", nil
			default:
				return "", "", errors.New("unexpected git command in " + dir + ": " + joined)
			}
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
	a.creds = &fakeCredentialStore{values: map[string]string{credentials.AIAccount("openai"): "test-key"}}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"commit-ai", "--yes", "--body"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	t.Chdir(root)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("commit-ai returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if len(commitArgs) != 1 || commitArgs[0] != "commit -m feat(dirty): update file -m Explain why this change was made." {
		t.Fatalf("commit args = %#v", commitArgs)
	}
	if realAdds != 1 {
		t.Fatalf("real-index adds = %d, want 1", realAdds)
	}
}

func TestCommitAIInvalidOutputRetriesAndDoesNotCommit(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	requests := 0
	server := aiResponseServerFunc(t, func() string {
		requests++
		return `{"messages":[{"id":"c000001","subject":"not conventional"}]}`
	})
	defer server.Close()
	saveAIConfig(t, server.URL)

	root := t.TempDir()
	makeGitDir(t, root, "dirty")
	committed := false
	realAdd := false
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" {
				return "/usr/bin/git", nil
			}
			return "", errors.New("unexpected lookpath")
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := strings.Join(args, " ")
			tempIndex := hasGitIndexEnv(env)
			switch {
			case joined == "rev-parse --verify --quiet HEAD":
				return "head\n", "", nil
			case tempIndex && joined == "read-tree HEAD":
				return "", "", nil
			case tempIndex && joined == "add -A":
				return "", "", nil
			case !tempIndex && joined == "add -A":
				realAdd = true
				return "", "", errors.New("should not stage real index")
			case tempIndex && joined == "diff --cached --quiet":
				return "", "", errors.New("dirty")
			case tempIndex && joined == "diff --cached --name-status":
				return "M\tfile.txt\n", "", nil
			case tempIndex && joined == "diff --cached --numstat":
				return "1\t0\tfile.txt\n", "", nil
			case tempIndex && joined == "diff --cached --no-color --no-ext-diff --find-renames --find-copies --unified=3 -- file.txt":
				return "diff --git a/file.txt b/file.txt\n+hello\n", "", nil
			case !tempIndex && strings.HasPrefix(joined, "commit "):
				committed = true
				return "", "", errors.New("should not commit")
			default:
				return "", "", errors.New("unexpected git command: " + joined)
			}
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
	a.creds = &fakeCredentialStore{values: map[string]string{credentials.AIAccount("openai"): "test-key"}}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"commit-ai", "--yes"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	t.Chdir(root)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected generation failure")
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
	if committed {
		t.Fatal("commit should not run after invalid AI output")
	}
	if realAdd {
		t.Fatal("real index should not be staged after invalid AI output")
	}
	if !strings.Contains(stdout.String(), "Summary: 0 committed, 0 skipped, 1 failed") {
		t.Fatalf("missing summary:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestCommitAICancelBeforeAPIDoesNotStageRealIndex(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := aiResponseServer(t, `{"messages":[{"id":"c000001","subject":"feat(dirty): update file"}]}`)
	defer server.Close()
	saveAIConfig(t, server.URL)

	root := t.TempDir()
	makeGitDir(t, root, "dirty")
	realAdd := false
	committed := false
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" {
				return "/usr/bin/git", nil
			}
			return "", errors.New("unexpected lookpath")
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := strings.Join(args, " ")
			tempIndex := hasGitIndexEnv(env)
			switch {
			case joined == "rev-parse --verify --quiet HEAD":
				return "head\n", "", nil
			case tempIndex && joined == "read-tree HEAD":
				return "", "", nil
			case tempIndex && joined == "add -A":
				return "", "", nil
			case !tempIndex && joined == "add -A":
				realAdd = true
				return "", "", errors.New("should not stage real index")
			case tempIndex && joined == "diff --cached --quiet":
				return "", "", errors.New("dirty")
			case tempIndex && joined == "diff --cached --name-status":
				return "M\tfile.txt\n", "", nil
			case tempIndex && joined == "diff --cached --numstat":
				return "1\t0\tfile.txt\n", "", nil
			case tempIndex && joined == "diff --cached --no-color --no-ext-diff --find-renames --find-copies --unified=3 -- file.txt":
				return "diff --git a/file.txt b/file.txt\n+hello\n", "", nil
			case !tempIndex && strings.HasPrefix(joined, "commit "):
				committed = true
				return "", "", errors.New("should not commit")
			default:
				return "", "", errors.New("unexpected git command: " + joined)
			}
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader("n\n"), &stdout, &stderr)
	a.creds = &fakeCredentialStore{values: map[string]string{credentials.AIAccount("openai"): "test-key"}}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"commit-ai"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	t.Chdir(root)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected cancellation failure")
	}
	if realAdd {
		t.Fatal("real index should not be staged before API-send confirmation")
	}
	if committed {
		t.Fatal("commit should not run before API-send confirmation")
	}
	if !strings.Contains(stdout.String(), "Stopped before sending any data.") {
		t.Fatalf("missing cancellation output:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestCommitAIPreparationUsesMutationWorkerCap(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	repos := make([]repo, 8)
	for i := range repos {
		dir := makeGitDir(t, root, "repo-"+string(rune('a'+i)))
		repos[i] = repo{dir: dir, display: filepath.Base(dir)}
	}
	var mu sync.Mutex
	activeAdds := 0
	maxActiveAdds := 0
	runner := fakeRunner{
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := strings.Join(args, " ")
			tempIndex := hasGitIndexEnv(env)
			switch {
			case joined == "rev-parse --verify --quiet HEAD":
				return "head\n", "", nil
			case tempIndex && joined == "read-tree HEAD":
				return "", "", nil
			case tempIndex && joined == "add -A":
				mu.Lock()
				activeAdds++
				if activeAdds > maxActiveAdds {
					maxActiveAdds = activeAdds
				}
				mu.Unlock()
				time.Sleep(20 * time.Millisecond)
				mu.Lock()
				activeAdds--
				mu.Unlock()
				return "", "", nil
			case tempIndex && joined == "diff --cached --quiet":
				return "", "", nil
			default:
				return "", "", errors.New("unexpected git command: " + joined)
			}
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)

	changes, skipped, failed := collectCommitAIChanges(a, repos, 3000)
	if len(changes) != 0 || skipped != len(repos) || failed != 0 {
		t.Fatalf("changes=%d skipped=%d failed=%d", len(changes), skipped, failed)
	}
	if maxActiveAdds > gitMutationWorkerCount(len(repos)) {
		t.Fatalf("active temp-index adds = %d, want <= %d", maxActiveAdds, gitMutationWorkerCount(len(repos)))
	}
}

func saveAIConfig(t *testing.T, baseURL string) {
	t.Helper()
	cfg := config.Defaults()
	cfg.AI.BaseURL = baseURL
	cfg.AI.Model = "gpt-test"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func hasGitIndexEnv(env []string) bool {
	for _, entry := range env {
		if strings.HasPrefix(entry, "GIT_INDEX_FILE=") {
			return true
		}
	}
	return false
}

func makeGitDir(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func aiResponseServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return aiResponseServerFunc(t, func() string { return content })
}

func aiResponseServerFunc(t *testing.T, content func() string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Fatal(err)
		}
		envelope := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": content()}},
			},
		}
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(envelope); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buf.Bytes())
	}))
}
