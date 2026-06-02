package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
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
		case "git rev-list --reverse --all":
			return "abc123\n", "", nil
		case "git log -1 --format=%B abc123":
			return "old message\n", "", nil
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

	accepted, failures := processBatch(context.Background(), []item{
		{ID: "c000001", RepoName: "repo", Hash: "abcdef123456", Context: "context"},
		{ID: "c000002", RepoName: "repo", Hash: "123456abcdef", Context: "context"},
	}, Config{
		BaseURL: server.URL,
		Model:   "test-model",
		APIKey:  "test-key",
		Timeout: time.Second,
	}, io.Discard)
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
}
