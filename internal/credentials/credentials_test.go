package credentials

import (
	"errors"
	"testing"
)

type memoryStore struct {
	values map[string]string
	err    error
}

func (s memoryStore) Get(account string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	value, ok := s.values[account]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (s memoryStore) Set(account, secret string) error {
	return nil
}

func (s memoryStore) Delete(account string) error {
	return nil
}

func TestResolveGitHubTokenPrefersEnv(t *testing.T) {
	t.Setenv("GIT_WRANGLER_GITHUB_TOKEN", "env-token")
	got := ResolveGitHubToken(memoryStore{values: map[string]string{GitHubAccount("github.com"): "keyring-token"}}, "github.com")
	if got.Value != "env-token" || got.Source != SourceEnv {
		t.Fatalf("resolved = %#v", got)
	}
}

func TestResolveGitHubTokenIgnoresGHToken(t *testing.T) {
	t.Setenv("GIT_WRANGLER_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "gh-token")
	got := ResolveGitHubToken(memoryStore{values: map[string]string{GitHubAccount("github.com"): "keyring-token"}}, "github.com")
	if got.Value != "keyring-token" || got.Source != SourceKeyring {
		t.Fatalf("resolved = %#v", got)
	}
}

func TestResolveAIKeyUsesOpenAIEnvFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-token")
	got := ResolveAIKey(memoryStore{}, "openai")
	if got.Value != "openai-token" || got.Source != SourceEnv {
		t.Fatalf("resolved = %#v", got)
	}
}

func TestResolveKeyringAndMissing(t *testing.T) {
	store := memoryStore{values: map[string]string{AIAccount("openai"): "stored"}}
	got := ResolveAIKey(store, "openai")
	if got.Value != "stored" || got.Source != SourceKeyring {
		t.Fatalf("resolved = %#v", got)
	}
	got = ResolveGitHubToken(store, "github.com")
	if got.Value != "" || got.Source != SourceMissing {
		t.Fatalf("missing resolved = %#v", got)
	}
}

func TestKeyringAvailability(t *testing.T) {
	if !KeyringAvailable(memoryStore{}) {
		t.Fatal("ErrNotFound should mean keyring is available")
	}
	if KeyringAvailable(memoryStore{err: errors.New("unavailable")}) {
		t.Fatal("expected unavailable keyring")
	}
}
