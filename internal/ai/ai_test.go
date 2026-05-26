package ai

import (
	"strings"
	"testing"
)

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
