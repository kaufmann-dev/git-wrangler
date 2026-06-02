package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

type fakeRunner struct {
	run func(context.Context, string, []string, string, ...string) (string, string, error)
}

func (f fakeRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
	if f.run == nil {
		return "", "", errors.New("unexpected command")
	}
	return f.run(ctx, dir, env, name, args...)
}

func (f fakeRunner) LookPath(name string) (string, error) {
	return "", exec.ErrNotFound
}

func (f fakeRunner) Pipe(ctx context.Context, dir string, env []string, left run.Command, right run.Command, consume func(io.Reader) error) error {
	return errors.New("unexpected pipe")
}

func TestRedactDiffHidesSensitiveFileContents(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/.env b/.env",
		"+OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz",
		"diff --git a/main.go b/main.go",
		"+token := \"ghp_abcdefghijklmnopqrstuvwxyz\"",
	}, "\n")
	redacted := RedactDiff(diff)
	if strings.Contains(redacted, "OPENAI_API_KEY") || strings.Contains(redacted, "sk-abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("sensitive file content leaked:\n%s", redacted)
	}
	if !strings.Contains(redacted, "[SENSITIVE FILE CONTENT HIDDEN]") {
		t.Fatalf("missing sensitive file marker:\n%s", redacted)
	}
	if strings.Contains(redacted, "ghp_abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("token leaked:\n%s", redacted)
	}
}

func TestRedactDiffParsesQuotedAndSpacedPaths(t *testing.T) {
	diff := strings.Join([]string{
		`diff --git a/app/config.json b/app/config.json`,
		`+{"not":"secret"}`,
		`diff --git "a/dir/secret file.json" "b/dir/secret file.json"`,
		`+password="super-secret-value"`,
		`diff --git "a/dir/escaped\"quote.env" "b/dir/escaped\"quote.env"`,
		`+TOKEN=ghp_abcdefghijklmnopqrstuvwxyz`,
	}, "\n")
	redacted := RedactDiff(diff)
	if !strings.Contains(redacted, `+{"not":"secret"}`) {
		t.Fatalf("generic config.json should not be hidden:\n%s", redacted)
	}
	if strings.Contains(redacted, "super-secret-value") || strings.Contains(redacted, "TOKEN=") {
		t.Fatalf("sensitive path content leaked:\n%s", redacted)
	}
	if strings.Count(redacted, "[SENSITIVE FILE CONTENT HIDDEN]") != 2 {
		t.Fatalf("wrong sensitive marker count:\n%s", redacted)
	}
}

func TestRedactDiffFailsClosedForUnparseableHeader(t *testing.T) {
	diff := "diff --git \"unterminated\n+password=\"super-secret-value\""
	redacted := RedactDiff(diff)
	if strings.Contains(redacted, "super-secret-value") {
		t.Fatalf("unparseable block leaked:\n%s", redacted)
	}
}

func TestSensitivePathList(t *testing.T) {
	sensitive := []string{
		".npmrc",
		".pypirc",
		".netrc",
		".git-credentials",
		".docker/config.json",
		".kube/config",
		"kubeconfig",
		".aws/credentials",
		".config/gcloud/application_default_credentials.json",
		"private.asc",
		"secret.gpg",
		"server.crt",
	}
	for _, path := range sensitive {
		if !IsSensitivePath(path) {
			t.Fatalf("expected sensitive path %q", path)
		}
	}
	if IsSensitivePath("app/config.json") {
		t.Fatal("generic config.json must not be sensitive")
	}
}

func TestExcludedDiffPathList(t *testing.T) {
	excluded := []string{
		"node_modules/pkg/index.js",
		"vendor/library/file.go",
		"app/.next/server/page.js",
		"wp-admin/includes/file.php",
		"wp-content/uploads/2026/image.jpg",
		"public/uploads/image.jpg",
	}
	for _, path := range excluded {
		if !IsExcludedDiffPath(path) {
			t.Fatalf("expected excluded path %q", path)
		}
	}
	if IsExcludedDiffPath("app/main.go") {
		t.Fatal("normal source path must not be excluded")
	}
}

func TestExtractMessagesAndValidate(t *testing.T) {
	messages, err := ExtractMessages("```json\n{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(cli): add thing\"}]}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if messages["c000001"].Subject != "feat(cli): add thing" {
		t.Fatalf("message = %q", messages["c000001"])
	}
	if !ValidateMessage(messages["c000001"].Subject) {
		t.Fatal("expected valid Conventional Commit message")
	}
	if ValidateMessage("this is not conventional") {
		t.Fatal("expected invalid message")
	}
}

func TestExtractMessagesReportsIncompleteJSON(t *testing.T) {
	_, err := ExtractMessages(`{"messages":[`)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "AI response was incomplete JSON") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSubjectRejectsMultilineAndLongSubjects(t *testing.T) {
	if !ValidateSubject("feat(cli): add thing") {
		t.Fatal("expected valid subject")
	}
	for _, subject := range []string{
		"feat(cli): add thing\nbody",
		"feat(cli): add thing\rbody",
		strings.Repeat("a", 121),
		"not conventional",
	} {
		if ValidateSubject(subject) {
			t.Fatalf("expected invalid subject %q", subject)
		}
	}
}

func TestValidateGeneratedMessageRequiresBodyWhenEnabled(t *testing.T) {
	valid := Message{Subject: "feat(cli): add thing", Body: "Explain why this change was made."}
	if !ValidateGeneratedMessage(valid, true) {
		t.Fatal("expected valid message with body")
	}
	for _, message := range []Message{
		{Subject: "feat(cli): add thing"},
		{Subject: "feat(cli): add thing", Body: "   "},
		{Subject: "feat(cli): add thing", Body: strings.Repeat("a", 1001)},
		{Subject: "feat(cli): add thing", Body: "bad\x00body"},
		{Subject: "not conventional", Body: "Body"},
	} {
		if ValidateGeneratedMessage(message, true) {
			t.Fatalf("expected invalid message %#v", message)
		}
	}
	if !ValidateGeneratedMessage(Message{Subject: "feat(cli): add thing"}, false) {
		t.Fatal("body should be optional in subject-only mode")
	}
}

func TestWriteCommitCallbackFormatsBodyWithSingleBlankLine(t *testing.T) {
	path := t.TempDir() + "/callback.py"
	err := writeCommitCallback(path, []mapping{{
		hash:    "abc123",
		message: FormatMessage(Message{Subject: "feat(cli): add thing", Body: "Explain why this change was made."}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `b'feat(cli): add thing\n\nExplain why this change was made.\n'`) {
		t.Fatalf("callback did not preserve one blank line:\n%s", string(data))
	}
}

func TestChatEndpoint(t *testing.T) {
	if got := ChatEndpoint("https://example.test/v1"); got != "https://example.test/v1/chat/completions" {
		t.Fatalf("endpoint = %q", got)
	}
	if got := ChatEndpoint("https://example.test/v1/chat/completions"); got != "https://example.test/v1/chat/completions" {
		t.Fatalf("endpoint = %q", got)
	}
}

func TestGenerateFailsBeforeConfirmationWhenContextCollectionFails(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git rev-parse HEAD":
			return "abc123\n", "", nil
		case "git log --reverse --all --format=%H%x1f%B%x1e":
			return "abc123\x1fold message\n\x1e", "", nil
		case "git diff-tree --root --no-commit-id --name-status -r abc123":
			return "", "diff failed", errors.New("diff failed")
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})
	confirmed := false
	_, err := Generate(context.Background(), []Repository{{Dir: "repo", Name: "repo"}}, Config{
		BaseURL: "https://example.test/v1",
		Model:   "test-model",
		APIKey:  "test-key",
		WorkDir: t.TempDir(),
		Git:     gitClient,
	}, io.Discard, func(string) bool {
		confirmed = true
		return true
	})
	if err == nil {
		t.Fatal("expected context collection error")
	}
	if !strings.Contains(err.Error(), "build commit context") {
		t.Fatalf("unexpected error: %v", err)
	}
	if confirmed {
		t.Fatal("confirm should not be called after failed context collection")
	}
}

func TestCollectItemsScansReposConcurrentlyWithStableIDsAndStats(t *testing.T) {
	var mu sync.Mutex
	activeLogs := 0
	maxActiveLogs := 0
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git rev-parse HEAD":
			return "head\n", "", nil
		case "git log --reverse --all --format=%H%x1f%B%x1e":
			mu.Lock()
			activeLogs++
			if activeLogs > maxActiveLogs {
				maxActiveLogs = activeLogs
			}
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			activeLogs--
			mu.Unlock()
			if dir == "repo-a" {
				return "a1\x1ffeat(cli): existing\n\x1ea2\x1fold a\n\x1e", "", nil
			}
			return "b1\x1fold b\n\x1e", "", nil
		case "git diff-tree --root --no-commit-id --name-status -r a2",
			"git diff-tree --root --no-commit-id --name-status -r b1":
			return "M\tmain.go\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r a2",
			"git diff-tree --root --no-commit-id --numstat -r b1":
			return "1\t0\tmain.go\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 a2":
			return "diff --git a/main.go b/main.go\n+repo a\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 b1":
			return "diff --git a/main.go b/main.go\n+repo b\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	items, stats, err := collectItems(context.Background(), []Repository{
		{Dir: "repo-a", Name: "repo-a"},
		{Dir: "repo-b", Name: "repo-b"},
	}, gitClient, 3000, true, func(ProgressEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if maxActiveLogs < 2 {
		t.Fatalf("repo scans did not overlap; max active logs = %d", maxActiveLogs)
	}
	if stats.RepoCount != 2 || stats.TotalCommits != 3 || stats.SkippedFormatted != 1 || stats.SkippedEmpty != 0 || stats.SkippedUnborn != 0 {
		t.Fatalf("stats = %#v", stats)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	wants := []struct {
		id   string
		repo string
		hash string
	}{
		{"c000001", "repo-a", "a2"},
		{"c000002", "repo-b", "b1"},
	}
	for i, want := range wants {
		if items[i].ID != want.id || items[i].RepoName != want.repo || items[i].Hash != want.hash {
			t.Fatalf("item[%d] = id=%q repo=%q hash=%q, want %#v", i, items[i].ID, items[i].RepoName, items[i].Hash, want)
		}
	}
}

func TestProcessBatchRetrySleepRespectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, failures := processBatch(ctx, []item{{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"}}, Config{
		BaseURL:   "http://127.0.0.1:1/v1",
		Model:     "test-model",
		APIKey:    "test-key",
		BatchSize: 1,
		Timeout:   time.Second,
	}, io.Discard)
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("canceled retry slept too long: %s", elapsed)
	}
}

func TestProcessBatchRetriesTruncatedBatchIndividually(t *testing.T) {
	requests := 0
	var maxTokens []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var payload struct {
			MaxTokens int `json:"max_tokens"`
			Messages  []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		maxTokens = append(maxTokens, payload.MaxTokens)
		content := payload.Messages[1].Content
		w.Header().Set("Content-Type", "application/json")
		if requests <= 3 {
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"length","message":{"content":"{\"messages\":["}}]}`)
			return
		}
		id := "c000001"
		if strings.Contains(content, "c000002") {
			id = "c000002"
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"`+id+`\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	accepted, failures := processBatch(context.Background(), []item{
		{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"},
		{ID: "c000002", RepoName: "repo", Hash: "123456abcdef", Context: "context"},
	}, Config{
		BaseURL: server.URL,
		Model:   "test-model",
		APIKey:  "test-key",
		Timeout: time.Second,
	}, &out)
	if len(failures) != 0 {
		t.Fatalf("failures = %#v", failures)
	}
	if len(accepted) != 2 {
		t.Fatalf("accepted = %#v", accepted)
	}
	if requests != 5 {
		t.Fatalf("requests = %d, want 5", requests)
	}
	if len(maxTokens) < 3 || maxTokens[0] >= maxTokens[1] || maxTokens[1] >= maxTokens[2] {
		t.Fatalf("max_tokens did not increase across retries: %#v", maxTokens)
	}
	if !strings.Contains(out.String(), "Retrying 2 commit(s) after failed batch attempt 1: AI response was truncated by the output token limit (2 commits).") {
		t.Fatalf("retry output missing reason:\n%s", out.String())
	}
}

func TestProcessBatchRetryMessageIncludesHTTPFailureReason(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	accepted, failures := processBatch(context.Background(), []item{
		{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"},
	}, Config{
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKey:    "test-key",
		BatchSize: 1,
		Timeout:   time.Second,
	}, &out)
	if len(failures) != 0 || len(accepted) != 1 {
		t.Fatalf("accepted = %#v failures = %#v", accepted, failures)
	}
	if !strings.Contains(out.String(), "Retrying 1 commit(s) after failed batch attempt 1: HTTP 429: rate limit exceeded") {
		t.Fatalf("retry output missing HTTP reason:\n%s", out.String())
	}
}

func TestBuildContextSkipsShowForSensitiveOnlyCommit(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git diff-tree --root --no-commit-id --name-status -r abc123":
			return "M\t.env\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r abc123":
			return "1\t0\t.env\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 abc123":
			return "", "", errors.New("git show should not run")
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := buildContext(context.Background(), gitClient, "repo", "repo", "abc123", 3000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextText, "[SENSITIVE FILE CONTENT HIDDEN]") {
		t.Fatalf("missing sensitive marker:\n%s", contextText)
	}
}

func TestBuildContextSkipsShowForExcludedOnlyCommit(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git diff-tree --root --no-commit-id --name-status -r abc123":
			return "M\tnode_modules/pkg/index.js\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r abc123":
			return "1\t0\tnode_modules/pkg/index.js\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 abc123":
			return "", "", errors.New("git show should not run")
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := buildContext(context.Background(), gitClient, "repo", "repo", "abc123", 3000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextText, "node_modules/pkg/index.js") {
		t.Fatalf("missing excluded path context:\n%s", contextText)
	}
	if !strings.Contains(contextText, "[EXCLUDED OR SENSITIVE FILE CONTENT HIDDEN]") {
		t.Fatalf("missing excluded marker:\n%s", contextText)
	}
}

func TestBuildStagedContextSkipsDiffForExcludedOnlyChanges(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git diff --cached --name-status":
			return "M\tvendor/pkg/file.go\n", "", nil
		case "git diff --cached --numstat":
			return "1\t0\tvendor/pkg/file.go\n", "", nil
		case "git diff --cached --no-color --no-ext-diff --find-renames --find-copies --unified=3":
			return "", "", errors.New("git diff should not run")
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := BuildStagedContext(context.Background(), gitClient, "repo", "repo", 3000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextText, "vendor/pkg/file.go") {
		t.Fatalf("missing excluded path context:\n%s", contextText)
	}
	if !strings.Contains(contextText, "[EXCLUDED OR SENSITIVE FILE CONTENT HIDDEN]") {
		t.Fatalf("missing excluded marker:\n%s", contextText)
	}
}

func TestRedactDiffHidesExcludedPathContentInMixedDiff(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/node_modules/pkg/index.js b/node_modules/pkg/index.js",
		"+large generated content",
		"diff --git a/main.go b/main.go",
		"+useful source content",
	}, "\n")

	redacted := RedactDiff(diff)
	if strings.Contains(redacted, "large generated content") {
		t.Fatalf("excluded content leaked:\n%s", redacted)
	}
	if !strings.Contains(redacted, "[EXCLUDED FILE CONTENT HIDDEN]") {
		t.Fatalf("missing excluded marker:\n%s", redacted)
	}
	if !strings.Contains(redacted, "useful source content") {
		t.Fatalf("normal source content was hidden:\n%s", redacted)
	}
}

func TestProcessItemsPacesRequestStartsAndKeepsResults(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	var starts []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		starts = append(starts, time.Now())
		mu.Unlock()

		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		id := "c000001"
		if strings.Contains(payload.Messages[1].Content, "c000002") {
			id = "c000002"
			time.Sleep(20 * time.Millisecond)
		} else if strings.Contains(payload.Messages[1].Content, "c000003") {
			id = "c000003"
		} else {
			time.Sleep(80 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"`+id+`\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	results, failures, err := processItems(context.Background(), []item{
		{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"},
		{ID: "c000002", RepoName: "repo", Hash: "123456abcdef", Context: "context"},
		{ID: "c000003", RepoName: "repo", Hash: "333333333333", Context: "context"},
	}, Config{
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKey:    "test-key",
		BatchSize: 1,
		RPM:       3000,
		Timeout:   time.Second,
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 || len(results) != 3 {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
	if len(starts) != 3 {
		t.Fatalf("starts = %d, want 3", len(starts))
	}
	for i := 1; i < len(starts); i++ {
		if delta := starts[i].Sub(starts[i-1]); delta < 12*time.Millisecond {
			t.Fatalf("request %d started too soon after previous request: %s", i+1, delta)
		}
	}
	for _, id := range []string{"c000001", "c000002", "c000003"} {
		if results[id].Subject != "feat(cli): add thing" {
			t.Fatalf("missing result for %s: %#v", id, results[id])
		}
	}
}

func TestProcessItemsReportsProgressWithoutBatchSpam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		id := "c000001"
		if strings.Contains(payload.Messages[1].Content, "c000002") {
			id = "c000002"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"`+id+`\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	var events []ProgressEvent
	results, failures, err := processItems(context.Background(), []item{
		{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"},
		{ID: "c000002", RepoName: "repo", Hash: "123456abcdef", Context: "context"},
	}, Config{
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKey:    "test-key",
		BatchSize: 1,
		RPM:       60000,
		Timeout:   time.Second,
		Progress: func(event ProgressEvent) {
			events = append(events, event)
		},
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 || len(results) != 2 {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if strings.Contains(out.String(), "Generating batch") {
		t.Fatalf("old batch spam was printed:\n%s", out.String())
	}
	if len(events) != 2 {
		t.Fatalf("events = %#v, want 2", events)
	}
	for i, event := range events {
		if event.Phase != "Sending API requests" || event.Current != i+1 || event.Total != 2 {
			t.Fatalf("event[%d] = %#v", i, event)
		}
	}
}

func TestProcessItemsReportsRetryDetailThroughProgress(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"not conventional\"}]}"}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	var details []string
	results, failures, err := processItems(context.Background(), []item{
		{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"},
	}, Config{
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKey:    "test-key",
		BatchSize: 1,
		RPM:       60000,
		Timeout:   time.Second,
		Progress: func(event ProgressEvent) {
			if event.Detail != "" {
				details = append(details, event.Detail)
			}
		},
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 || len(results) != 1 {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if strings.Contains(out.String(), "Retrying") {
		t.Fatalf("retry output should use progress detail:\n%s", out.String())
	}
	if len(details) != 1 || !strings.Contains(details[0], "Retrying 1 commit(s) after failed batch attempt 1: missing or invalid message.") {
		t.Fatalf("details = %#v", details)
	}
}

func TestProcessItemsCancellationSuppressesRetryOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
	}))
	defer server.Close()

	var out bytes.Buffer
	_, failures, err := processItems(ctx, []item{
		{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"},
		{ID: "c000002", RepoName: "repo", Hash: "123456abcdef", Context: "context"},
	}, Config{
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKey:    "test-key",
		BatchSize: 1,
		RPM:       60000,
		Timeout:   time.Second,
	}, &out)
	if !errors.Is(err, ErrAPICancelled) {
		t.Fatalf("err = %v, want ErrAPICancelled", err)
	}
	if len(failures) != 0 {
		t.Fatalf("failures = %#v", failures)
	}
	if strings.Contains(out.String(), "Retrying") {
		t.Fatalf("retry output was printed for cancellation:\n%s", out.String())
	}
}
