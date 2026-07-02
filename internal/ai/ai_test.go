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
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func shrinkRetryBackoff(t *testing.T) {
	t.Helper()
	previous := retryBackoffBase
	retryBackoffBase = 2 * time.Millisecond
	t.Cleanup(func() { retryBackoffBase = previous })
}

type fakeRunner struct {
	run    func(context.Context, string, []string, string, ...string) (string, string, error)
	stream func(context.Context, string, []string, string, []string, func(io.Reader) error) error
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

func (f fakeRunner) Stream(ctx context.Context, dir string, env []string, name string, args []string, consume func(io.Reader) error) error {
	if f.stream == nil {
		stdout, stderr, err := f.Run(ctx, dir, env, name, args...)
		if err != nil {
			if strings.TrimSpace(stderr) != "" {
				return errors.New(strings.TrimSpace(stderr))
			}
			return err
		}
		return consume(strings.NewReader(stdout))
	}
	return f.stream(ctx, dir, env, name, args, consume)
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

func TestRedactLineHidesUnquotedAndBacktickSecrets(t *testing.T) {
	lines := []string{
		"+password: mysecretvalue",
		"+password := `mysecretvalue`",
		`+api_key = "mysecretvalue"`,
	}
	for _, line := range lines {
		redacted := RedactLine(line)
		if strings.Contains(redacted, "mysecretvalue") {
			t.Fatalf("secret leaked for %q: %s", line, redacted)
		}
		if !strings.Contains(redacted, "[REDACTED]") {
			t.Fatalf("missing redaction marker for %q: %s", line, redacted)
		}
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

func TestRedactDiffHidesSensitiveRenameSource(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/.env b/config.txt",
		"+OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz",
	}, "\n")
	redacted := RedactDiff(diff)
	if strings.Contains(redacted, "OPENAI_API_KEY") || strings.Contains(redacted, "sk-abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("sensitive rename source leaked:\n%s", redacted)
	}
	if !strings.Contains(redacted, "[SENSITIVE FILE CONTENT HIDDEN]") {
		t.Fatalf("missing sensitive marker:\n%s", redacted)
	}
}

func TestRedactDiffHidesExcludedRenameSource(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/vendor/pkg/file.go b/main.go",
		"+generated vendor content",
	}, "\n")
	redacted := RedactDiff(diff)
	if strings.Contains(redacted, "generated vendor content") {
		t.Fatalf("excluded rename source leaked:\n%s", redacted)
	}
	if !strings.Contains(redacted, "[EXCLUDED FILE CONTENT HIDDEN]") {
		t.Fatalf("missing excluded marker:\n%s", redacted)
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
	if !ValidateSubject(messages["c000001"].Subject) {
		t.Fatal("expected valid Conventional Commit message")
	}
	if ValidateSubject("this is not conventional") {
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
	if !ValidateGeneratedMessage(valid, true, false) {
		t.Fatal("expected valid message with body")
	}
	for _, message := range []Message{
		{Subject: "feat(cli): add thing"},
		{Subject: "feat(cli): add thing", Body: "   "},
		{Subject: "feat(cli): add thing", Body: strings.Repeat("a", 1001)},
		{Subject: "feat(cli): add thing", Body: "bad\x00body"},
		{Subject: "not conventional", Body: "Body"},
	} {
		if ValidateGeneratedMessage(message, true, false) {
			t.Fatalf("expected invalid message %#v", message)
		}
	}
	if !ValidateGeneratedMessage(Message{Subject: "feat(cli): add thing"}, false, false) {
		t.Fatal("body should be optional in subject-only mode")
	}
}

func TestValidateGeneratedMessageRequiresScopeWhenEnabled(t *testing.T) {
	if !ValidateGeneratedMessage(Message{Subject: "feat(cli): add thing"}, false, true) {
		t.Fatal("expected scoped subject to be valid with requireScope")
	}
	if ValidateGeneratedMessage(Message{Subject: "feat: add thing"}, false, true) {
		t.Fatal("expected scopeless subject to be invalid with requireScope")
	}
	if !ValidateGeneratedMessage(Message{Subject: "feat: add thing"}, false, false) {
		t.Fatal("expected scopeless subject to stay valid without requireScope")
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

func TestRequestBatchSendsCustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Project-Id"); got != "corp-dev-99" {
			t.Fatalf("X-Project-Id = %q, want corp-dev-99", got)
		}
		if got := r.Header.Get("Authorization"); got != "Gateway custom-token" {
			t.Fatalf("Authorization = %q, want custom header", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	messages, err := requestBatch(context.Background(), []item{{
		ID:       "c000001",
		RepoName: "repo",
		Hash:     "abcdef123456",
		Context:  "context",
	}}, Config{
		BaseURL: server.URL,
		Model:   "test-model",
		APIKey:  "default-token",
		Headers: map[string]string{
			"Authorization": "Gateway custom-token",
			"X-Project-Id":  "corp-dev-99",
		},
		Timeout: time.Second,
	}, 1, maxRequestContextChars)
	if err != nil {
		t.Fatal(err)
	}
	if messages["c000001"].Subject != "feat(cli): add thing" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestRequestBatchUsesJSONResponseFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			ResponseFormat map[string]string `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.ResponseFormat["type"] != "json_object" {
			t.Fatalf("response_format = %#v, want json_object", payload.ResponseFormat)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	_, err := requestBatch(context.Background(), []item{{
		ID:       "c000001",
		RepoName: "repo",
		Hash:     "abcdef123456",
		Context:  "context",
	}}, Config{
		BaseURL: server.URL,
		Model:   "test-model",
		APIKey:  "test-key",
		Timeout: time.Second,
	}, 1, maxRequestContextChars)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRequestBatchPromptsForSemanticPurpose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Messages) != 2 {
			t.Fatalf("messages = %d, want 2", len(payload.Messages))
		}
		system := payload.Messages[0].Content
		user := payload.Messages[1].Content
		for _, want := range []string{
			"Prefer the high-level purpose over file names",
			"Return valid JSON only",
		} {
			if !strings.Contains(system, want) {
				t.Fatalf("system prompt missing %q:\n%s", want, system)
			}
		}
		for _, want := range []string{
			"Summarize the semantic purpose of the change",
			"Do not merely name changed files",
			"Choose a concise scope from the product or subsystem area",
			"Do not repeat the subject or list files in the body",
		} {
			if !strings.Contains(user, want) {
				t.Fatalf("user prompt missing %q:\n%s", want, user)
			}
		}
		if strings.Contains(user, "Never omit the scope") {
			t.Fatalf("mandatory-scope instruction should require RequireScope:\n%s", user)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(cli): add capability\",\"body\":\"Explain the user-visible effect.\"}]}"}}]}`)
	}))
	defer server.Close()

	messages, err := requestBatch(context.Background(), []item{{
		ID:       "c000001",
		RepoName: "repo",
		Hash:     "abcdef123456",
		Context:  "context",
	}}, Config{
		BaseURL: server.URL,
		Model:   "test-model",
		APIKey:  "test-key",
		Timeout: time.Second,
		Body:    true,
	}, 1, maxRequestContextChars)
	if err != nil {
		t.Fatal(err)
	}
	if messages["c000001"].Subject != "feat(cli): add capability" || messages["c000001"].Body == "" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestProcessBatchDoesNotRetryPermanentAuthFailure(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "invalid API key", http.StatusUnauthorized)
	}))
	defer server.Close()

	accepted, failures := processBatchWithProgress(context.Background(), []item{{
		ID:       "c000001",
		RepoName: "repo",
		Hash:     "abcdef123456",
		Context:  "context",
	}}, Config{
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKey:    "bad-key",
		BatchSize: 1,
		Timeout:   time.Second,
	}, io.Discard, nil, nil, nil)
	if len(accepted) != 0 || len(failures) != 1 {
		t.Fatalf("accepted = %#v failures = %#v", accepted, failures)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if !strings.Contains(failures[0].Reason, "HTTP 401") {
		t.Fatalf("failure = %#v, want HTTP 401", failures[0])
	}
}

func TestPreflightUsesGenerationAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Gateway custom-token" {
			t.Fatalf("Authorization = %q, want custom header", got)
		}
		if got := r.Header.Get("X-Project-Id"); got != "corp-dev-99" {
			t.Fatalf("X-Project-Id = %q, want corp-dev-99", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "test-model" || payload["max_tokens"] != float64(1) {
			t.Fatalf("unexpected preflight payload: %#v", payload)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	err := Preflight(context.Background(), Config{
		BaseURL: server.URL,
		Model:   "test-model",
		APIKey:  "default-token",
		Headers: map[string]string{
			"Authorization": "Gateway custom-token",
			"X-Project-Id":  "corp-dev-99",
		},
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPreflightReturnsAPIAndTransportErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "invalid API key")
	}))
	err := Preflight(context.Background(), Config{BaseURL: server.URL, Model: "test-model", APIKey: "bad", Timeout: time.Second})
	server.Close()
	if err == nil || !strings.Contains(err.Error(), "HTTP 401: invalid API key") {
		t.Fatalf("preflight error = %v", err)
	}

	err = Preflight(context.Background(), Config{BaseURL: server.URL, Model: "test-model", APIKey: "bad", Timeout: time.Second})
	if err == nil {
		t.Fatal("expected transport error")
	}
}

func TestPreflightHonorsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	err := Preflight(context.Background(), Config{BaseURL: server.URL, Model: "test-model", APIKey: "test-key", Timeout: 10 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestGenerateFailsBeforeConfirmationWhenContextCollectionFails(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git rev-parse HEAD":
			return "abc123\n", "", nil
		case "git log --reverse --all --format=%H%x1f%P%x1f%B%x1e":
			return "abc123\x1f\x1fold message\n\x1e", "", nil
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
		case "git log --reverse --all --format=%H%x1f%P%x1f%B%x1e":
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
				return "a1\x1f\x1ffeat(cli): existing\n\x1ea2\x1fa1\x1fold a\n\x1e", "", nil
			}
			return "b1\x1f\x1fold b\n\x1e", "", nil
		case "git diff-tree --root --no-commit-id --name-status -r a2",
			"git diff-tree --root --no-commit-id --name-status -r b1":
			return "M\tmain.go\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r a2",
			"git diff-tree --root --no-commit-id --numstat -r b1":
			return "1\t0\tmain.go\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 a2 -- main.go":
			return "diff --git a/main.go b/main.go\n+repo a\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 b1 -- main.go":
			return "diff --git a/main.go b/main.go\n+repo b\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	items, stats, err := collectItems(context.Background(), []Repository{
		{Dir: "repo-a", Name: "repo-a", GitDir: "repo-a/.git"},
		{Dir: "repo-b", Name: "repo-b", GitDir: "repo-b/.git"},
	}, gitClient, true, false, func(ProgressEvent) {})
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
		id     string
		repo   string
		gitDir string
		hash   string
	}{
		{"c000001", "repo-a", "repo-a/.git", "a2"},
		{"c000002", "repo-b", "repo-b/.git", "b1"},
	}
	for i, want := range wants {
		if items[i].ID != want.id || items[i].RepoName != want.repo || items[i].RepoGitDir != want.gitDir || items[i].Hash != want.hash {
			t.Fatalf("item[%d] = id=%q repo=%q gitDir=%q hash=%q, want %#v", i, items[i].ID, items[i].RepoName, items[i].RepoGitDir, items[i].Hash, want)
		}
	}
	plan, err := buildPlan(items, map[string]Message{
		"c000001": {Subject: "feat: rewrite repo a"},
		"c000002": {Subject: "fix: rewrite repo b"},
	}, stats, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Repos[0].GitDir != "repo-a/.git" || plan.Repos[1].GitDir != "repo-b/.git" {
		t.Fatalf("plan git dirs = %q, %q", plan.Repos[0].GitDir, plan.Repos[1].GitDir)
	}
}

func TestCollectItemsSelectedHashesSkipUnselectedContext(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git rev-parse HEAD":
			return "head\n", "", nil
		case "git log --reverse --all --format=%H%x1f%P%x1f%B%x1e":
			return "a1\x1f\x1ffeat(cli): existing\n\x1ea2\x1fa1\x1fold selected\n\x1ea3\x1fa2\x1fold unselected\n\x1e", "", nil
		case "git diff-tree --root --no-commit-id --name-status -r a2":
			return "M\tmain.go\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r a2":
			return "1\t0\tmain.go\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 a2 -- main.go":
			return "diff --git a/main.go b/main.go\n+selected\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})
	items, stats, err := collectItems(context.Background(), []Repository{{
		Dir:            "repo",
		Name:           "repo",
		GitDir:         "repo/.git",
		SelectedHashes: map[string]bool{"a2": true},
	}}, gitClient, true, false, func(ProgressEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalCommits != 3 || stats.SkippedFormatted != 0 {
		t.Fatalf("stats = %#v", stats)
	}
	if len(items) != 1 || items[0].Hash != "a2" {
		t.Fatalf("items = %#v, want only a2", items)
	}
}

func TestCollectItemsRequireScopeSkipsOnlyScopedConventionalCommits(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git rev-parse HEAD":
			return "head\n", "", nil
		case "git log --reverse --all --format=%H%x1f%P%x1f%B%x1e":
			return "a1\x1f\x1ffeat(cli): scoped\n\x1ea2\x1fa1\x1ffeat: scopeless\n\x1ea3\x1fa2\x1fplain message\n\x1e", "", nil
		case "git diff-tree --root --no-commit-id --name-status -r a2",
			"git diff-tree --root --no-commit-id --name-status -r a3":
			return "M\tmain.go\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r a2",
			"git diff-tree --root --no-commit-id --numstat -r a3":
			return "1\t0\tmain.go\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 a2 -- main.go",
			"git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 a3 -- main.go":
			return "diff --git a/main.go b/main.go\n+change\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})
	items, stats, err := collectItems(context.Background(), []Repository{
		{Dir: "repo", Name: "repo", GitDir: "repo/.git"},
	}, gitClient, true, true, func(ProgressEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalCommits != 3 || stats.SkippedFormatted != 1 {
		t.Fatalf("stats = %#v, want 1 skipped scoped commit", stats)
	}
	if len(items) != 2 || items[0].Hash != "a2" || items[1].Hash != "a3" {
		t.Fatalf("items = %#v, want scopeless and plain commits", items)
	}
}

func TestRequestBatchRequireScopePromptDemandsScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		user := payload.Messages[1].Content
		for _, want := range []string{
			"Never omit the scope",
			"use a broad scope such as repo for repository-wide changes",
		} {
			if !strings.Contains(user, want) {
				t.Fatalf("user prompt missing %q:\n%s", want, user)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(cli): add capability\"}]}"}}]}`)
	}))
	defer server.Close()

	_, err := requestBatch(context.Background(), []item{{
		ID:       "c000001",
		RepoName: "repo",
		Hash:     "abcdef123456",
		Context:  "context",
	}}, Config{
		BaseURL:      server.URL,
		Model:        "test-model",
		APIKey:       "test-key",
		Timeout:      time.Second,
		RequireScope: true,
	}, 1, maxRequestContextChars)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectItemsSkipsAutoGeneratedCommits(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git rev-parse HEAD":
			return "head\n", "", nil
		case "git log --reverse --all --format=%H%x1f%P%x1f%B%x1e":
			return "a1\x1f\x1fold message\n\x1e" +
				"a2\x1fa1 f1\x1fMerge branch 'main' of https://github.com/sus-amogus/amogus.css into main\n\x1e" +
				"a3\x1fa2\x1fRevert \"feat(cli): add thing\"\n\nThis reverts commit a1.\n\x1e" +
				"a4\x1fa3\x1ffixup! feat(cli): add thing\n\x1e", "", nil
		case "git diff-tree --root --no-commit-id --name-status -r a1":
			return "M\tmain.go\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r a1":
			return "1\t0\tmain.go\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 a1 -- main.go":
			return "diff --git a/main.go b/main.go\n+change\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})
	items, stats, err := collectItems(context.Background(), []Repository{
		{Dir: "repo", Name: "repo", GitDir: "repo/.git"},
	}, gitClient, true, true, func(ProgressEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalCommits != 4 || stats.SkippedAutoGenerated != 3 || stats.SkippedFormatted != 0 || stats.SkippedEmpty != 0 {
		t.Fatalf("stats = %#v, want merge, revert, and fixup counted as skipped auto-generated", stats)
	}
	if len(items) != 1 || items[0].Hash != "a1" {
		t.Fatalf("items = %#v, want only the regular commit", items)
	}
	plan, err := buildPlan(items, map[string]Message{
		"c000001": {Subject: "feat(cli): rewrite"},
	}, stats, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Summary, "Skipped auto-generated commits: 3") {
		t.Fatalf("summary missing auto-generated skip line:\n%s", plan.Summary)
	}
}

func TestAutoGeneratedSubject(t *testing.T) {
	for _, message := range []string{
		"Revert \"feat(cli): add thing\"\n\nThis reverts commit abc123.",
		"Reapply \"feat(cli): add thing\"",
		"fixup! feat(cli): add thing",
		"squash! feat(cli): add thing",
		"amend! feat(cli): add thing",
	} {
		if !autoGeneratedSubject(message) {
			t.Fatalf("expected auto-generated: %q", message)
		}
	}
	for _, message := range []string{
		"feat(cli): add thing",
		"revert: remove broken feature",
		"Revert broken feature",
		"fix squash bug in fixup logic",
	} {
		if autoGeneratedSubject(message) {
			t.Fatalf("expected not auto-generated: %q", message)
		}
	}
}

func TestProcessItemsRequireScopeRetriesScopelessSubject(t *testing.T) {
	shrinkRetryBackoff(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat: add thing\"}]}"}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	var retryDetails []string
	results, failures, err := processItems(context.Background(), []item{
		{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"},
	}, Config{
		BaseURL:      server.URL,
		Model:        "test-model",
		APIKey:       "test-key",
		BatchSize:    1,
		RPM:          60000,
		Timeout:      time.Second,
		RequireScope: true,
		Progress: func(event ProgressEvent) {
			if event.Error {
				retryDetails = append(retryDetails, event.Detail)
			}
		},
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 || results["c000001"].Subject != "feat(cli): add thing" {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if len(retryDetails) != 1 || !strings.Contains(retryDetails[0], "missing or invalid message") {
		t.Fatalf("retry details = %#v, want scopeless subject retried as invalid", retryDetails)
	}
}

func TestProcessBatchRetrySleepRespectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, failures := processBatchWithProgress(ctx, []item{{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"}}, Config{
		BaseURL:   "http://127.0.0.1:1/v1",
		Model:     "test-model",
		APIKey:    "test-key",
		BatchSize: 1,
		Timeout:   time.Second,
	}, io.Discard, nil, nil, nil)
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("canceled retry slept too long: %s", elapsed)
	}
}

func TestProcessBatchRetriesTruncatedBatchIndividually(t *testing.T) {
	shrinkRetryBackoff(t)
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
		if requests <= 4 {
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
	accepted, failures := processBatchWithProgress(context.Background(), []item{
		{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"},
		{ID: "c000002", RepoName: "repo", Hash: "123456abcdef", Context: "context"},
	}, Config{
		BaseURL: server.URL,
		Model:   "test-model",
		APIKey:  "test-key",
		Timeout: time.Second,
	}, &out, nil, nil, nil)
	if len(failures) != 0 {
		t.Fatalf("failures = %#v", failures)
	}
	if len(accepted) != 2 {
		t.Fatalf("accepted = %#v", accepted)
	}
	if requests != 6 {
		t.Fatalf("requests = %d, want 6", requests)
	}
	if len(maxTokens) < 4 || maxTokens[0] >= maxTokens[1] || maxTokens[1] >= maxTokens[2] || maxTokens[2] >= maxTokens[3] {
		t.Fatalf("max_tokens did not increase across retries: %#v", maxTokens)
	}
	if strings.Contains(out.String(), "after failed batch attempt") {
		t.Fatalf("per-attempt retry line should not be printed:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Retrying 2 commit(s) individually after failed batch generation.") {
		t.Fatalf("retry output missing individual escalation:\n%s", out.String())
	}
}

func TestProcessItemsRetrySummaryIncludesHTTPFailureReason(t *testing.T) {
	shrinkRetryBackoff(t)
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

	var retryDetails []string
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
			if event.Error {
				retryDetails = append(retryDetails, event.Detail)
			}
		},
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 || len(results) != 1 {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if len(retryDetails) != 1 {
		t.Fatalf("retry details = %#v, want one summary", retryDetails)
	}
	if !strings.Contains(retryDetails[0], "Retried 1 transient API failure(s)") || !strings.Contains(retryDetails[0], "HTTP 429: rate limit exceeded") {
		t.Fatalf("retry summary missing HTTP reason: %q", retryDetails[0])
	}
}

func TestBuildContextIncludesChangeSummaryMetadata(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git diff-tree --root --no-commit-id --name-status -r abc123":
			return strings.Join([]string{
				"M\tinternal/cli/commit.go",
				"A\tdocs/commands.md",
				"M\t.env",
				"M\tnode_modules/pkg/index.js",
			}, "\n") + "\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r abc123":
			return strings.Join([]string{
				"10\t2\tinternal/cli/commit.go",
				"3\t1\tdocs/commands.md",
				"1\t0\t.env",
				"50\t0\tnode_modules/pkg/index.js",
			}, "\n") + "\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 abc123 -- internal/cli/commit.go docs/commands.md":
			return strings.Join([]string{
				"diff --git a/internal/cli/commit.go b/internal/cli/commit.go",
				"+semantic source change",
				"diff --git a/docs/commands.md b/docs/commands.md",
				"+document behavior",
			}, "\n"), "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := buildContext(context.Background(), gitClient, "repo", "repo", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Change summary:",
		"Total files: 4",
		"Change mix: 1 added, 3 modified",
		"File areas:",
		"source",
		"docs",
		"generated",
		"Line stats: +64 -3",
		"Hidden content: 1 sensitive path(s), 1 excluded/generated path(s)",
		"semantic source change",
		"node_modules/pkg/index.js",
	} {
		if !strings.Contains(contextText, want) {
			t.Fatalf("context missing %q:\n%s", want, contextText)
		}
	}
	if strings.Contains(contextText, "OPENAI_API_KEY") {
		t.Fatalf("sensitive content leaked:\n%s", contextText)
	}
}

func TestBuildStagedContextPreservesSummaryWhenDiffIsTruncated(t *testing.T) {
	largeDiff := "diff --git a/file.txt b/file.txt\n" + strings.Repeat("+very long generated context\n", 3000)
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git diff --cached --name-status":
			return "M\tfile.txt\n", "", nil
		case "git diff --cached --numstat":
			return "2\t1\tfile.txt\n", "", nil
		case "git diff --cached --no-color --no-ext-diff --find-renames --find-copies --unified=3 -- file.txt":
			return largeDiff, "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := BuildStagedContextWithEnv(context.Background(), gitClient, "repo", "repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Change summary:",
		"Files changed:",
		"Stats:",
		"Redacted staged diff snippet:",
		"[TRUNCATED TO SINGLE-COMMIT CONTEXT BUDGET]",
	} {
		if !strings.Contains(contextText, want) {
			t.Fatalf("context missing %q:\n%s", want, contextText)
		}
	}
	if strings.Count(contextText, "very long generated context") >= 3000 {
		t.Fatalf("diff was not truncated:\n%s", contextText)
	}
}

func TestBuildStagedContextIsNotTruncatedAtLegacyDefault(t *testing.T) {
	largeDiff := "diff --git a/file.txt b/file.txt\n" + strings.Repeat("+useful generated context\n", 250)
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git diff --cached --name-status":
			return "M\tfile.txt\n", "", nil
		case "git diff --cached --numstat":
			return "250\t0\tfile.txt\n", "", nil
		case "git diff --cached --no-color --no-ext-diff --find-renames --find-copies --unified=3 -- file.txt":
			return largeDiff, "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := BuildStagedContextWithEnv(context.Background(), gitClient, "repo", "repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(contextText) <= 3000 {
		t.Fatalf("context length = %d, want above legacy 3000 cap", len(contextText))
	}
	if strings.Contains(contextText, "[TRUNCATED") {
		t.Fatalf("context was unexpectedly truncated:\n%s", contextText)
	}
}

func TestBuildContextBoundsStreamedDiffCollection(t *testing.T) {
	reader := &repeatingReader{line: []byte("+large useful context line\n"), remaining: maxSingleCommitContextChars * 20}
	gitClient := git.New(fakeRunner{
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := name + " " + strings.Join(args, " ")
			switch joined {
			case "git diff-tree --root --no-commit-id --name-status -r abc123":
				return "M\tmain.go\n", "", nil
			case "git diff-tree --root --no-commit-id --numstat -r abc123":
				return "100000\t0\tmain.go\n", "", nil
			default:
				return "", "", errors.New("unexpected command: " + joined)
			}
		},
		stream: func(ctx context.Context, dir string, env []string, name string, args []string, consume func(io.Reader) error) error {
			joined := name + " " + strings.Join(args, " ")
			if joined != "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 abc123 -- main.go" {
				return errors.New("unexpected stream command: " + joined)
			}
			return consume(reader)
		},
	})

	contextText, err := buildContext(context.Background(), gitClient, "repo", "repo", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if reader.read > maxSingleCommitContextChars+32768 {
		t.Fatalf("stream read %d bytes, want bounded near %d", reader.read, maxSingleCommitContextChars)
	}
	if !strings.Contains(contextText, "[TRUNCATED TO SINGLE-COMMIT CONTEXT BUDGET]") {
		t.Fatalf("missing truncation marker:\n%s", contextText)
	}
}

type repeatingReader struct {
	line      []byte
	remaining int
	read      int
	offset    int
}

func (r *repeatingReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) && r.remaining > 0 {
		p[n] = r.line[r.offset]
		n++
		r.read++
		r.remaining--
		r.offset = (r.offset + 1) % len(r.line)
	}
	return n, nil
}

func TestBuildContextSkipsShowForSensitiveOnlyCommit(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git diff-tree --root --no-commit-id --name-status -r abc123":
			return "M\t.env\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r abc123":
			return "1\t0\t.env\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := buildContext(context.Background(), gitClient, "repo", "repo", "abc123")
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
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := buildContext(context.Background(), gitClient, "repo", "repo", "abc123")
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

func TestBuildContextShowsOnlyVisiblePathsForMixedStaticCommit(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git diff-tree --root --no-commit-id --name-status -r abc123":
			return "M\tindex.html\nM\tassets/js/pixi.min.js\nA\tassets/img/photo.webp\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r abc123":
			return "1\t0\tindex.html\n5000\t0\tassets/js/pixi.min.js\n-\t-\tassets/img/photo.webp\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 abc123 -- index.html":
			return "diff --git a/index.html b/index.html\n+visible content\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := buildContext(context.Background(), gitClient, "repo", "repo", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextText, "assets/js/pixi.min.js") || !strings.Contains(contextText, "assets/img/photo.webp") {
		t.Fatalf("missing static path context:\n%s", contextText)
	}
	if !strings.Contains(contextText, "visible content") {
		t.Fatalf("missing visible diff content:\n%s", contextText)
	}
}

func TestBuildContextPreservesVisiblePathWithSpaces(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git diff-tree --root --no-commit-id --name-status -r abc123":
			return "M\tpages/about me.html\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r abc123":
			return "1\t0\tpages/about me.html\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 abc123 -- pages/about me.html":
			return "diff --git a/pages/about me.html b/pages/about me.html\n+visible content\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := buildContext(context.Background(), gitClient, "repo", "repo", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextText, "visible content") {
		t.Fatalf("missing visible diff content:\n%s", contextText)
	}
}

func TestBuildContextSkipsDiffForSensitiveRenameSource(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git diff-tree --root --no-commit-id --name-status -r abc123":
			return "R100\t.env\tconfig.txt\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r abc123":
			return "1\t0\tconfig.txt\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := buildContext(context.Background(), gitClient, "repo", "repo", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextText, "[SENSITIVE FILE CONTENT HIDDEN]") {
		t.Fatalf("missing sensitive marker:\n%s", contextText)
	}
}

func TestBuildContextIncludesNormalRenameDiff(t *testing.T) {
	gitClient := git.New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git diff-tree --root --no-commit-id --name-status -r abc123":
			return "R100\told.txt\tnew.txt\n", "", nil
		case "git diff-tree --root --no-commit-id --numstat -r abc123":
			return "1\t0\tnew.txt\n", "", nil
		case "git show --format= --no-color --no-ext-diff --find-renames --find-copies --unified=3 abc123 -- new.txt":
			return "diff --git a/old.txt b/new.txt\n+visible content\n", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := buildContext(context.Background(), gitClient, "repo", "repo", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextText, "visible content") {
		t.Fatalf("missing visible rename diff:\n%s", contextText)
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
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}})

	contextText, err := BuildStagedContextWithEnv(context.Background(), gitClient, "repo", "repo", nil)
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

func TestPackItemsUsesBatchSizeAsMaximumAndSplitsLargeContexts(t *testing.T) {
	items := []item{
		{ID: "c000001", Context: strings.Repeat("a", 30000)},
		{ID: "c000002", Context: strings.Repeat("b", 30000)},
		{ID: "c000003", Context: strings.Repeat("c", 30000)},
		{ID: "c000004", Context: "small"},
	}
	batches := packItems(items, 3, maxRequestContextChars)
	if len(batches) != 2 {
		t.Fatalf("batches = %d, want 2", len(batches))
	}
	if len(batches[0]) != 2 || len(batches[1]) != 2 {
		t.Fatalf("batch sizes = %d, %d; want 2, 2", len(batches[0]), len(batches[1]))
	}

	batches = packItems(items, 2, maxRequestContextChars)
	if len(batches) != 2 || len(batches[0]) != 2 || len(batches[1]) != 2 {
		t.Fatalf("max batch size not respected: %#v", batchLengths(batches))
	}
}

func TestPackItemsSendsOversizedSingleCommitAlone(t *testing.T) {
	batches := packItems([]item{
		{ID: "c000001", Context: strings.Repeat("a", maxRequestContextChars+1000)},
		{ID: "c000002", Context: "small"},
		{ID: "c000003", Context: "small"},
	}, 10, maxRequestContextChars)
	if len(batches) != 2 {
		t.Fatalf("batches = %d, want 2", len(batches))
	}
	if len(batches[0]) != 1 || batches[0][0].ID != "c000001" {
		t.Fatalf("oversized commit was not sent alone: %#v", batchLengths(batches))
	}
	if len(batches[1]) != 2 {
		t.Fatalf("remaining batch size = %d, want 2", len(batches[1]))
	}
}

func batchLengths(batches [][]item) []int {
	lengths := make([]int, len(batches))
	for i, batch := range batches {
		lengths[i] = len(batch)
	}
	return lengths
}

func TestProcessItemsPacesIndividualFallbackRetries(t *testing.T) {
	shrinkRetryBackoff(t)
	var mu sync.Mutex
	var starts []time.Time
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		starts = append(starts, time.Now())
		request := requests
		mu.Unlock()

		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		content := payload.Messages[1].Content
		w.Header().Set("Content-Type", "application/json")
		if request <= 4 {
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"not conventional\"},{\"id\":\"c000002\",\"subject\":\"not conventional\"}]}"}}]}`)
			return
		}
		id := "c000001"
		if strings.Contains(content, "c000002") {
			id = "c000002"
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"`+id+`\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	results, failures, err := processItems(context.Background(), []item{
		{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"},
		{ID: "c000002", RepoName: "repo", Hash: "123456abcdef", Context: "context"},
	}, Config{
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKey:    "test-key",
		BatchSize: 2,
		RPM:       1200,
		Timeout:   time.Second,
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 || len(results) != 2 {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if len(starts) != 6 {
		t.Fatalf("starts = %d, want 6", len(starts))
	}
	for i := 1; i < len(starts); i++ {
		if delta := starts[i].Sub(starts[i-1]); delta < 40*time.Millisecond {
			t.Fatalf("request %d started too soon after previous request: %s", i+1, delta)
		}
	}
}

func TestProcessBatchShrinksContextAfterIncompleteJSON(t *testing.T) {
	var contextLengths []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		content := payload.Messages[1].Content
		contextLengths = append(contextLengths, strings.Count(content, "context line"))
		w.Header().Set("Content-Type", "application/json")
		if len(contextLengths) == 1 {
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":["}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	results, failures := processBatchWithProgress(context.Background(), []item{{
		ID:       "c000001",
		RepoName: "repo",
		Hash:     "abcdef123456",
		Context:  contextWithLargeDiff(strings.Repeat("+context line\n", 7000)),
	}}, Config{
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKey:    "test-key",
		BatchSize: 1,
		RPM:       60000,
		Timeout:   time.Second,
	}, io.Discard, nil, nil, nil)
	if len(failures) != 0 || len(results) != 1 {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if len(contextLengths) != 2 {
		t.Fatalf("requests = %d, want 2", len(contextLengths))
	}
	if contextLengths[1] >= contextLengths[0] {
		t.Fatalf("context was not reduced after incomplete JSON: %#v", contextLengths)
	}
}

func TestProviderContextLimitRetriesWithSmallerContext(t *testing.T) {
	var contextLengths []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		content := payload.Messages[1].Content
		contextLengths = append(contextLengths, strings.Count(content, "context line"))
		w.Header().Set("Content-Type", "application/json")
		if len(contextLengths) < 3 {
			http.Error(w, "maximum context length exceeded", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	results, failures := processBatchWithProgress(context.Background(), []item{{
		ID:       "c000001",
		RepoName: "repo",
		Hash:     "abcdef123456",
		Context:  contextWithLargeDiff(strings.Repeat("+context line\n", 7000)),
	}}, Config{
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKey:    "test-key",
		BatchSize: 1,
		RPM:       60000,
		Timeout:   time.Second,
	}, &out, nil, nil, nil)
	if len(failures) != 0 || len(results) != 1 {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if len(contextLengths) != 3 {
		t.Fatalf("requests = %d, want 3", len(contextLengths))
	}
	if !(contextLengths[0] > contextLengths[1] && contextLengths[1] > contextLengths[2]) {
		t.Fatalf("context lengths did not shrink: %#v", contextLengths)
	}
	if !strings.Contains(out.String(), "Retrying 1 commit(s) with smaller context after provider context limit") {
		t.Fatalf("missing context-limit retry notice:\n%s", out.String())
	}
}

func TestProcessItemsFinalRecoveryRecoversRemainingFailure(t *testing.T) {
	shrinkRetryBackoff(t)
	requests := 0
	var retryDetails []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests <= 4 {
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"not conventional\"}]}"}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	results, failures, err := processItems(context.Background(), []item{{
		ID:       "c000001",
		RepoName: "repo",
		Hash:     "abcdef123456",
		Context:  "context",
	}}, Config{
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKey:    "test-key",
		BatchSize: 1,
		RPM:       60000,
		Timeout:   time.Second,
		Progress: func(event ProgressEvent) {
			if event.Error {
				retryDetails = append(retryDetails, event.Detail)
			}
		},
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 || len(results) != 1 {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if requests != 5 {
		t.Fatalf("requests = %d, want 5", requests)
	}
	foundRecovery := false
	for _, detail := range retryDetails {
		if strings.Contains(detail, "final single-commit recovery") {
			foundRecovery = true
		}
	}
	if !foundRecovery {
		t.Fatalf("retry details missing final recovery: %#v", retryDetails)
	}
}

func TestProcessItemsFinalRecoverySkipsPermanentFailure(t *testing.T) {
	requests := 0
	var retryDetails []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "invalid API key", http.StatusUnauthorized)
	}))
	defer server.Close()

	results, failures, err := processItems(context.Background(), []item{{
		ID:       "c000001",
		RepoName: "repo",
		Hash:     "abcdef123456",
		Context:  "context",
	}}, Config{
		BaseURL:   server.URL,
		Model:     "test-model",
		APIKey:    "bad-key",
		BatchSize: 1,
		RPM:       60000,
		Timeout:   time.Second,
		Progress: func(event ProgressEvent) {
			if event.Error {
				retryDetails = append(retryDetails, event.Detail)
			}
		},
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 || len(failures) != 1 {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	for _, detail := range retryDetails {
		if strings.Contains(detail, "final single-commit recovery") {
			t.Fatalf("permanent failure should not run final recovery: %#v", retryDetails)
		}
	}
}

func contextWithLargeDiff(diff string) string {
	return strings.Join([]string{
		"Repository: repo",
		"",
		"Change summary:",
		"Total files: 1",
		"",
		"Files changed:",
		"M\tmain.go",
		"",
		"Stats:",
		"1\t0\tmain.go",
		"",
		"Redacted diff snippet:",
		diff,
	}, "\n")
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
	started := 0
	var completed []ProgressEvent
	for _, event := range events {
		if event.Current == 0 {
			started++
			if event.Key == "" || !strings.HasPrefix(event.Detail, "batch ") {
				t.Fatalf("start event missing key/detail: %#v", event)
			}
			continue
		}
		completed = append(completed, event)
	}
	if started != 2 {
		t.Fatalf("started = %d, want 2; events = %#v", started, events)
	}
	if len(completed) != 2 {
		t.Fatalf("completed events = %#v, want 2", completed)
	}
	for i, event := range completed {
		if event.Phase != "Sending API requests" || event.Current != i+1 || event.Total != 2 {
			t.Fatalf("event[%d] = %#v", i, event)
		}
		if event.Detail == "" || event.Error {
			t.Fatalf("event[%d] detail/error = %q/%v", i, event.Detail, event.Error)
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
	var retryDetails []string
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
			if event.Error {
				retryDetails = append(retryDetails, event.Detail)
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
	if len(retryDetails) != 1 || !strings.Contains(retryDetails[0], "Retried 1 transient API failure(s): missing or invalid message.") {
		t.Fatalf("retry details = %#v, want one aggregate summary", retryDetails)
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
	if strings.Contains(out.String(), "Retried") {
		t.Fatalf("retry summary was printed for cancellation:\n%s", out.String())
	}
}

func TestWriteGenerationFailuresGroupsReasonsAndPrintsHint(t *testing.T) {
	var out bytes.Buffer
	writeGenerationFailures(&out, []failure{
		{Item: item{RepoName: "repo-a", Hash: "abcdef123456"}, Reason: "unexpected EOF"},
		{Item: item{RepoName: "repo-b", Hash: "123456abcdef"}, Reason: "unexpected EOF"},
		{Item: item{RepoName: "repo-c", Hash: "fedcba654321"}, Reason: "missing or invalid message"},
	})
	text := out.String()
	for _, want := range []string{
		"Failed repo-a abcdef12: unexpected EOF",
		"Failure reasons: unexpected EOF (2 commits); missing or invalid message",
		"Hint: retry the command",
		"lower --concurrency or --rpm",
		"reduce --batch-size",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestProcessItemsBoundsInFlightRequests(t *testing.T) {
	var inFlight, maxInFlight atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			observed := maxInFlight.Load()
			if current <= observed || maxInFlight.CompareAndSwap(observed, current) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)

		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		id := "c000001"
		for _, candidate := range []string{"c000002", "c000003", "c000004", "c000005", "c000006"} {
			if strings.Contains(payload.Messages[1].Content, candidate) {
				id = candidate
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"`+id+`\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	items := make([]item, 0, 6)
	for _, id := range []string{"c000001", "c000002", "c000003", "c000004", "c000005", "c000006"} {
		items = append(items, item{ID: id, RepoName: "repo", Hash: "abcdef123456", Context: "context"})
	}
	results, failures, err := processItems(context.Background(), items, Config{
		BaseURL:     server.URL,
		Model:       "test-model",
		APIKey:      "test-key",
		BatchSize:   1,
		RPM:         60000,
		Concurrency: 2,
		Timeout:     time.Second,
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 || len(results) != 6 {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if observed := maxInFlight.Load(); observed > 2 {
		t.Fatalf("max in-flight requests = %d, want at most 2", observed)
	}
}

func TestRetryBackoffStaysWithinJitterBounds(t *testing.T) {
	for attempt := 1; attempt <= 6; attempt++ {
		expected := retryBackoffBase << (attempt - 1)
		if expected > 8*time.Second {
			expected = 8 * time.Second
		}
		for range 50 {
			delay := retryBackoff(attempt)
			if delay < expected/2 || delay >= expected {
				t.Fatalf("retryBackoff(%d) = %s, want in [%s, %s)", attempt, delay, expected/2, expected)
			}
		}
	}
}

func TestChatCompletionAppliesPerRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer server.Close()

	_, err := chatCompletion(context.Background(), Config{
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: 50 * time.Millisecond,
	}, map[string]any{"model": "test-model"})
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("err = %v, want context deadline exceeded", err)
	}
}

func TestProcessItemsEmitsSingleRetrySummary(t *testing.T) {
	shrinkRetryBackoff(t)
	var mu sync.Mutex
	failedBatches := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		id := "c000001"
		if strings.Contains(payload.Messages[1].Content, "c000002") {
			id = "c000002"
		}
		mu.Lock()
		firstAttempt := !failedBatches[id]
		failedBatches[id] = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if firstAttempt {
			if id == "c000001" {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":["}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"messages\":[{\"id\":\"`+id+`\",\"subject\":\"feat(cli): add thing\"}]}"}}]}`)
	}))
	defer server.Close()

	var retryDetails []string
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
			if event.Error {
				retryDetails = append(retryDetails, event.Detail)
			}
		},
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 || len(results) != 2 {
		t.Fatalf("results = %#v failures = %#v", results, failures)
	}
	if len(retryDetails) != 1 {
		t.Fatalf("retry details = %#v, want exactly one summary", retryDetails)
	}
	if !strings.Contains(retryDetails[0], "Retried 2 transient API failure(s)") ||
		!strings.Contains(retryDetails[0], "HTTP 429: rate limit exceeded") ||
		!strings.Contains(retryDetails[0], "AI response was incomplete JSON") {
		t.Fatalf("retry summary missing aggregated reasons: %q", retryDetails[0])
	}
}
