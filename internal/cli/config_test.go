package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	makeInteractive(a)
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
	makeInteractive(a)
	a.creds = &fakeCredentialStore{}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"config", "set", "ai.api-key"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected config set to fail without secret input")
	}
	if strings.Contains(stderr.String(), "secret input is required") {
		t.Fatalf("EOF was reported as missing secret input:\n%s", stderr.String())
	}
	if strings.Count(stdout.String(), "SKIP stopped: operation cancelled") != 1 {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
}

func TestConfigFileRemoveSecretsPathAndShow(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	wantPath := filepath.Join(configDir, "git-wrangler", "remove-secrets.toml")

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), nil, []string{"config", "file", "remove-secrets", "path"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("config file remove-secrets path returned error: %v\nstderr: %s", err, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != wantPath {
		t.Fatalf("path output = %q, want %q", strings.TrimSpace(stdout.String()), wantPath)
	}

	stdout.Reset()
	stderr.Reset()
	if err := ExecuteWithRunner(context.Background(), nil, []string{"config", "file", "remove-secrets", "show"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("config file remove-secrets show returned error: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No extra remove-secrets paths configured.") {
		t.Fatalf("missing none configured message:\n%s", stdout.String())
	}

	if err := os.MkdirAll(filepath.Dir(wantPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wantPath, []byte("paths = [\"private/*.json\", \".env.local\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := ExecuteWithRunner(context.Background(), nil, []string{"config", "file", "remove-secrets", "show"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("config file remove-secrets show returned error: %v\nstderr: %s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, ".env.local") || !strings.Contains(output, "private/*.json") {
		t.Fatalf("show output missing configured paths:\n%s", output)
	}
	if strings.Index(output, ".env.local") > strings.Index(output, "private/*.json") {
		t.Fatalf("show output is not sorted:\n%s", output)
	}
}

func TestConfigFileRemoveSecretsEditCreatesInvokesAndValidates(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("VISUAL", "test-editor --flag")
	var editedPath string
	runner := fakeRunner{
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "test-editor" {
				return "", "", errors.New("unexpected editor: " + name)
			}
			if len(args) != 2 || args[0] != "--flag" {
				return "", "", errors.New("unexpected editor args: " + strings.Join(args, " "))
			}
			editedPath = args[1]
			return "", "", os.WriteFile(editedPath, []byte("paths = [\"private/*.json\"]\n"), 0o600)
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"config", "file", "remove-secrets", "edit"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("config file remove-secrets edit returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if editedPath == "" {
		t.Fatal("editor was not invoked")
	}
	if !strings.Contains(stdout.String(), "OK Validated "+editedPath) {
		t.Fatalf("missing validation success:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
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
	makeInteractive(a)
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

func TestConfigSetGitHubHostClearsUsernameOnChange(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save(config.Config{
		GitHub: config.GitHubConfig{Host: "github.com", Username: "octo"},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), nil, []string{"config", "set", "github.host", "https://github.enterprise.test/"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("config set github.host returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHub.Host != "github.enterprise.test" || cfg.GitHub.Username != "" {
		t.Fatalf("config = %#v", cfg.GitHub)
	}
}

func TestConfigUnsetSupportsAllConfigKeys(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save(config.Config{
		GitHub: config.GitHubConfig{Host: "github.enterprise.test", Username: "octo"},
		AI: config.AIConfig{
			Provider: "custom",
			BaseURL:  "https://ai.example.test/v1",
			Model:    "model-test",
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := &fakeCredentialStore{values: map[string]string{
		"github:github.enterprise.test": "github-secret",
		"ai:custom":                     "ai-secret",
	}}

	for _, args := range [][]string{
		{"config", "unset", "github.auth"},
		{"config", "unset", "ai.api-key"},
		{"config", "unset", "ai.model"},
		{"config", "unset", "github.host"},
		{"config", "unset", "ai.provider"},
		{"config", "unset", "ai.base-url"},
	} {
		var stdout, stderr bytes.Buffer
		a := newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)
		a.creds = store
		cmd := newRootCommand(a)
		cmd.SetArgs(args)
		cmd.SetIn(a.stdin)
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v returned error: %v\nstdout: %s\nstderr: %s", args, err, stdout.String(), stderr.String())
		}
	}

	if _, err := store.Get("github:github.enterprise.test"); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("github credential still present: %v", err)
	}
	if _, err := store.Get("ai:custom"); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("AI credential still present: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHub.Host != config.DefaultGitHubHost || cfg.GitHub.Username != "" {
		t.Fatalf("github config = %#v", cfg.GitHub)
	}
	if cfg.AI.Provider != config.DefaultAIProvider || cfg.AI.BaseURL != config.DefaultAIBaseURL || cfg.AI.Model != "" {
		t.Fatalf("ai config = %#v", cfg.AI)
	}
}

func TestConfigUnsetAIBaseURLLeavesCustomProviderEmpty(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save(config.Config{
		AI: config.AIConfig{Provider: "custom", BaseURL: "https://ai.example.test/v1"},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), nil, []string{"config", "unset", "ai.base-url"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("config unset ai.base-url returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Provider != "custom" || cfg.AI.BaseURL != "" {
		t.Fatalf("ai config = %#v", cfg.AI)
	}
}

func TestConfigUnsetAIHeaderRemovesConfigAndCredential(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save(config.Config{
		AI: config.AIConfig{
			Provider:      "openai",
			Headers:       map[string]string{"X-Project-Id": "corp-dev-99"},
			SecretHeaders: []string{"Api-Key"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := &fakeCredentialStore{values: map[string]string{
		credentials.AIHeaderAccount("openai", "Api-Key"): "azure-secret",
	}}

	for _, key := range []string{"ai.headers.X-Project-ID", "ai.headers.api-key"} {
		var stdout, stderr bytes.Buffer
		a := newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)
		a.creds = store
		cmd := newRootCommand(a)
		cmd.SetArgs([]string{"config", "unset", key})
		cmd.SetIn(a.stdin)
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("config unset %s returned error: %v\nstdout: %s\nstderr: %s", key, err, stdout.String(), stderr.String())
		}
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AI.Headers) != 0 || len(cfg.AI.SecretHeaders) != 0 {
		t.Fatalf("headers = %#v secret = %#v", cfg.AI.Headers, cfg.AI.SecretHeaders)
	}
	if _, err := store.Get(credentials.AIHeaderAccount("openai", "Api-Key")); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("secret header credential still present: %v", err)
	}
}

func TestConfigSetRejectsExtraValuesBeforeMutation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, args := range [][]string{
		{"config", "set", "ai.model", "gpt-test", "extra"},
		{"config", "set", "ai.headers.X-Project-ID", "corp-dev-99", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithRunner(context.Background(), nil, args, strings.NewReader(""), &stdout, &stderr)
		if err == nil || !strings.Contains(stderr.String(), "set accepts at most one value") {
			t.Fatalf("%v error = %v, stderr:\n%s", args, err, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("%v wrote stdout:\n%s", args, stdout.String())
		}
	}
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid config set wrote config file %q: %v", path, err)
	}
}

func TestConfigSetSecretPlaintextValueIsRejected(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), nil, []string{"config", "set", "ai.api-key", "super-secret"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil || !strings.Contains(stderr.String(), "ai.api-key does not accept a plaintext value") {
		t.Fatalf("config set ai.api-key plaintext error = %v, stderr:\n%s", err, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "super-secret") {
		t.Fatalf("plaintext secret leaked:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("plaintext secret set wrote config file %q: %v", path, err)
	}
}

func TestInitUsesFakeAuthAndDoesNotPrintSecrets(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	stdin := strings.NewReader("\ny\n\n\ngpt-test\ny\nai-secret\n")
	var stdout, stderr bytes.Buffer
	store := &fakeCredentialStore{}
	a := newApp(context.Background(), fakeRunner{}, stdin, &stdout, &stderr)
	makeInteractive(a)
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
	makeInteractive(a)
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
	makeInteractive(a)
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
