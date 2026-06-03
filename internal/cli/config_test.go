package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/auth"
)

func TestConfigSetShowUnsetSecretDoesNotPrintSecret(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store := &fakeCredentialStore{}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader("secret-token\n"), &stdout, &stderr)
	a.creds = store
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"config", "set", "ai.api-key"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config set returned error: %v\nstderr: %s", err, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "secret-token") {
		t.Fatalf("secret leaked:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	a = newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)
	a.creds = store
	cmd = newRootCommand(a)
	cmd.SetArgs([]string{"config", "show"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config show returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "API key: keyring") {
		t.Fatalf("config show missing keyring source:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "secret-token") {
		t.Fatalf("config show leaked secret:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	a = newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)
	a.creds = store
	cmd = newRootCommand(a)
	cmd.SetArgs([]string{"config", "show", "--json"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config show --json returned error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("config show --json wrote stderr: %s", stderr.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout.String())
	}
	if strings.Contains(stdout.String(), "secret-token") {
		t.Fatalf("config show --json leaked secret:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	a = newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)
	a.creds = store
	cmd = newRootCommand(a)
	cmd.SetArgs([]string{"config", "unset", "ai.api-key"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config unset returned error: %v", err)
	}
	if _, err := store.Get("ai:openai"); err == nil {
		t.Fatal("secret was not deleted")
	}
}

func TestConfigSetSecretRequiresPromptInput(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)
	a.creds = &fakeCredentialStore{}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"config", "set", "ai.api-key"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected config set to fail without secret input")
	}
	if !strings.Contains(stderr.String(), "secret input is required") {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}

func TestInitUsesFakeAuthAndDoesNotPrintSecrets(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	stdin := strings.NewReader("\ny\n\n\ngpt-test\ny\nai-secret\n")
	var stdout, stderr bytes.Buffer
	store := &fakeCredentialStore{}
	a := newApp(context.Background(), fakeRunner{}, stdin, &stdout, &stderr)
	a.creds = store
	a.auth = fakeGitHubAuth{result: auth.GitHubResult{Token: "github-secret", Username: "octo"}}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"init"})
	cmd.SetIn(stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init returned error: %v\nstderr: %s", err, stderr.String())
	}
	out := stdout.String() + stderr.String()
	if strings.Contains(out, "github-secret") || strings.Contains(out, "ai-secret") {
		t.Fatalf("secret leaked:\n%s", out)
	}
	if got, err := store.Get("github:github.com"); err != nil || got != "github-secret" {
		t.Fatalf("github secret = %q, %v", got, err)
	}
	if got, err := store.Get("ai:openai"); err != nil || got != "ai-secret" {
		t.Fatalf("ai secret = %q, %v", got, err)
	}
}
