package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/auth"
	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
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
	if !strings.Contains(stdout.String(), "API key   keyring") {
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

func TestConfigSetAIHeadersUsesConfigAndKeyring(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store := &fakeCredentialStore{}
	var stdout, stderr bytes.Buffer

	a := newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)
	a.creds = store
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"config", "set", "ai.headers.X-Project-ID", "corp-dev-99"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config set plaintext header returned error: %v\nstderr: %s", err, stderr.String())
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Headers["X-Project-Id"] != "corp-dev-99" {
		t.Fatalf("headers = %#v", cfg.AI.Headers)
	}

	stdout.Reset()
	stderr.Reset()
	a = newApp(context.Background(), fakeRunner{}, strings.NewReader("azure-secret\n"), &stdout, &stderr)
	a.creds = store
	cmd = newRootCommand(a)
	cmd.SetArgs([]string{"config", "set", "ai.headers.api-key"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config set secret header returned error: %v\nstderr: %s", err, stderr.String())
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AI.SecretHeaders) != 1 || cfg.AI.SecretHeaders[0] != "Api-Key" {
		t.Fatalf("secret headers = %#v", cfg.AI.SecretHeaders)
	}
	if got, err := store.Get(credentials.AIHeaderAccount("openai", "Api-Key")); err != nil || got != "azure-secret" {
		t.Fatalf("stored header = %q, %v", got, err)
	}
	if strings.Contains(stdout.String()+stderr.String(), "azure-secret") {
		t.Fatalf("secret leaked:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
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
	if strings.Contains(stdout.String(), "azure-secret") {
		t.Fatalf("config show --json leaked secret:\n%s", stdout.String())
	}
	var shown map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout.String())
	}
	aiSection := shown["ai"].(map[string]any)
	headers := aiSection["headers"].([]any)
	foundSecretHeader := false
	for _, header := range headers {
		row := header.(map[string]any)
		if row["name"] == "Api-Key" && row["source"] == "keyring" && row["set"] == true {
			foundSecretHeader = true
		}
	}
	if !foundSecretHeader {
		t.Fatalf("config show --json missing header metadata:\n%s", stdout.String())
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

func TestInitWithoutKeyringSkipsSecretPromptsAndSavesConfig(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_WRANGLER_GITHUB_TOKEN", "")
	t.Setenv("GIT_WRANGLER_AI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	stdin := strings.NewReader("example.com\ncustom\nhttps://ai.example.test/v1\nmodel-test\n")
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, stdin, &stdout, &stderr)
	a.creds = &fakeCredentialStore{err: errors.New("keyring unavailable")}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"init"})
	cmd.SetIn(stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init returned error: %v\nstderr: %s", err, stderr.String())
	}
	if strings.Contains(stderr.String(), "Authenticate GitHub now?") || strings.Contains(stderr.String(), "Store an AI API key now?") || strings.Contains(stderr.String(), "AI API key:") {
		t.Fatalf("secret prompt was shown:\n%s", stderr.String())
	}
	for _, want := range []string{
		"Secure credential storage is unavailable, so Git Wrangler skipped GitHub authentication setup. Set GIT_WRANGLER_GITHUB_TOKEN instead.",
		"Secure credential storage is unavailable, so Git Wrangler skipped AI API key setup. Set GIT_WRANGLER_AI_API_KEY instead.",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("missing %q guidance:\n%s", want, stderr.String())
		}
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHub.Host != "example.com" || cfg.AI.Provider != "custom" || cfg.AI.BaseURL != "https://ai.example.test/v1" || cfg.AI.Model != "model-test" {
		t.Fatalf("config = %#v", cfg)
	}
	if !strings.Contains(stdout.String(), "Setup complete") || !strings.Contains(stdout.String(), "missing") {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
}

func TestInitWithoutKeyringShowsOpenAIEnvironmentGuidance(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_WRANGLER_GITHUB_TOKEN", "")
	t.Setenv("GIT_WRANGLER_AI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	stdin := strings.NewReader("\n\n\n\n")
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, stdin, &stdout, &stderr)
	a.creds = &fakeCredentialStore{err: errors.New("keyring unavailable")}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"init"})
	cmd.SetIn(stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init returned error: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Secure credential storage is unavailable, so Git Wrangler skipped AI API key setup. Set GIT_WRANGLER_AI_API_KEY or OPENAI_API_KEY instead.") {
		t.Fatalf("missing OpenAI guidance:\n%s", stderr.String())
	}
}
