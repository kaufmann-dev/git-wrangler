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
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
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
