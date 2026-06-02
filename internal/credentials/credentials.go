package credentials

import (
	"errors"
	"os"

	"github.com/zalando/go-keyring"
)

const ServiceName = "git-wrangler"

var ErrNotFound = errors.New("secret not found")

type Source string

const (
	SourceMissing Source = "missing"
	SourceEnv     Source = "env"
	SourceKeyring Source = "keyring"
)

type Store interface {
	Get(account string) (string, error)
	Set(account, secret string) error
	Delete(account string) error
}

type KeyringStore struct{}

func NewKeyringStore() KeyringStore {
	return KeyringStore{}
}

func (KeyringStore) Get(account string) (string, error) {
	secret, err := keyring.Get(ServiceName, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return secret, err
}

func (KeyringStore) Set(account, secret string) error {
	return keyring.Set(ServiceName, account, secret)
}

func (KeyringStore) Delete(account string) error {
	err := keyring.Delete(ServiceName, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

type Resolved struct {
	Value  string
	Source Source
	Err    error
}

func ResolveGitHubToken(store Store, host string) Resolved {
	if value := firstEnv("GIT_WRANGLER_GITHUB_TOKEN", "GH_TOKEN"); value != "" {
		return Resolved{Value: value, Source: SourceEnv}
	}
	return resolveStore(store, GitHubAccount(host))
}

func ResolveAIKey(store Store, provider string) Resolved {
	names := []string{"GIT_WRANGLER_AI_API_KEY"}
	if provider == "openai" {
		names = append(names, "OPENAI_API_KEY")
	}
	if value := firstEnv(names...); value != "" {
		return Resolved{Value: value, Source: SourceEnv}
	}
	return resolveStore(store, AIAccount(provider))
}

func GitHubAccount(host string) string {
	return "github:" + host
}

func AIAccount(provider string) string {
	return "ai:" + provider
}

func KeyringAvailable(store Store) bool {
	if store == nil {
		return false
	}
	_, err := store.Get("__probe__")
	return err == nil || errors.Is(err, ErrNotFound)
}

func resolveStore(store Store, account string) Resolved {
	if store == nil {
		return Resolved{Source: SourceMissing}
	}
	value, err := store.Get(account)
	if err == nil {
		return Resolved{Value: value, Source: SourceKeyring}
	}
	if errors.Is(err, ErrNotFound) {
		return Resolved{Source: SourceMissing}
	}
	return Resolved{Source: SourceMissing, Err: err}
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}
