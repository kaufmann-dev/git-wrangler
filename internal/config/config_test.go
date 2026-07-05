package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenConfigIsMissing(t *testing.T) {
	cfg, err := LoadPath(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHub.Host != DefaultGitHubHost {
		t.Fatalf("github host = %q", cfg.GitHub.Host)
	}
	if cfg.AI.Provider != DefaultAIProvider || cfg.AI.BaseURL != DefaultAIBaseURL {
		t.Fatalf("AI defaults = %#v", cfg.AI)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	want := Defaults()
	want.GitHub.Host = "git.example.com"
	want.GitHub.Username = "octo"
	want.AI.Model = "gpt-test"
	want.AI.Headers = map[string]string{"x-project-id": "corp-dev-99"}
	want.AI.SecretHeaders = []string{"api-key", "Api-Key"}
	if err := SavePath(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.GitHub.Host != want.GitHub.Host || got.GitHub.Username != want.GitHub.Username || got.AI.Model != want.AI.Model {
		t.Fatalf("config = %#v, want %#v", got, want)
	}
	if got.AI.Headers["X-Project-Id"] != "corp-dev-99" {
		t.Fatalf("headers = %#v", got.AI.Headers)
	}
	if len(got.AI.SecretHeaders) != 1 || got.AI.SecretHeaders[0] != "Api-Key" {
		t.Fatalf("secret headers = %#v", got.AI.SecretHeaders)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestCanonicalHeaderName(t *testing.T) {
	got, ok := CanonicalHeaderName("x-project-id")
	if !ok || got != "X-Project-Id" {
		t.Fatalf("canonical = %q/%v", got, ok)
	}
	if _, ok := CanonicalHeaderName("bad header"); ok {
		t.Fatal("header with spaces should be invalid")
	}
}

func TestLoadMalformedConfigFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPath(path); err == nil {
		t.Fatal("expected malformed config error")
	}
}

func TestLoadRemoveSecretsPathsMissingFileUsesDefaults(t *testing.T) {
	paths, usingDefaults, err := LoadRemoveSecretsPathsPath(filepath.Join(t.TempDir(), "remove-secrets.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !usingDefaults {
		t.Fatal("expected defaults when config file is missing")
	}
	want, err := ValidateRemoveSecretsPaths(DefaultRemoveSecretsPaths())
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 || len(paths) != len(want) {
		t.Fatalf("paths = %#v, want defaults %#v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths = %#v, want defaults %#v", paths, want)
		}
	}
}

func TestLoadRemoveSecretsPathsValidatesNormalizesAndSorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "remove-secrets.toml")
	data := []byte("paths = [\"private\\\\*.json\", \".env.local\", \".env.local\"]\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	paths, usingDefaults, err := LoadRemoveSecretsPathsPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if usingDefaults {
		t.Fatal("expected file-backed paths, not defaults")
	}
	want := []string{".env.local", "private/*.json"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths = %#v, want %#v", paths, want)
		}
	}
}

func TestLoadRemoveSecretsPathsRejectsInvalidConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		toml string
	}{
		{name: "unknown field", toml: "paths = [\"ok\"]\nregexes = [\"no\"]\n"},
		{name: "empty path", toml: "paths = [\"\"]\n"},
		{name: "absolute path", toml: "paths = [\"/tmp/secret\"]\n"},
		{name: "windows absolute path", toml: "paths = [\"C:/tmp/secret\"]\n"},
		{name: "traversal path", toml: "paths = [\"../secret\"]\n"},
		{name: "invalid glob", toml: "paths = [\"secret[\"]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "remove-secrets.toml")
			if err := os.WriteFile(path, []byte(tc.toml), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := LoadRemoveSecretsPathsPath(path); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestEnsureRemoveSecretsStarterCreatesPrivateFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := EnsureRemoveSecretsStarter()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
	if _, _, err := LoadRemoveSecretsPaths(); err != nil {
		t.Fatalf("starter TOML should validate: %v", err)
	}
}
