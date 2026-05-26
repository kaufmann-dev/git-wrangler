package ai

import (
	"context"
	"errors"
	"io"
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
	messages, err := ExtractMessages("```json\n{\"messages\":[{\"id\":\"c000001\",\"message\":\"feat(cli): add thing\"}]}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if messages["c000001"] != "feat(cli): add thing" {
		t.Fatalf("message = %q", messages["c000001"])
	}
	if !ValidateMessage(messages["c000001"]) {
		t.Fatal("expected valid Conventional Commit message")
	}
	if ValidateMessage("this is not conventional") {
		t.Fatal("expected invalid message")
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
