package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
)

const (
	SchemaVersion = 1

	DefaultGitHubHost = "github.com"
	DefaultAIProvider = "openai"
	DefaultAIBaseURL  = "https://api.openai.com/v1"
)

type Config struct {
	SchemaVersion int          `json:"schema_version"`
	GitHub        GitHubConfig `json:"github"`
	AI            AIConfig     `json:"ai"`
}

type GitHubConfig struct {
	Host     string `json:"host"`
	Username string `json:"username,omitempty"`
}

type AIConfig struct {
	Provider      string            `json:"provider"`
	BaseURL       string            `json:"base_url"`
	Model         string            `json:"model,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	SecretHeaders []string          `json:"secret_headers,omitempty"`
}

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "git-wrangler", "config.json"), nil
}

func Defaults() Config {
	return Config{
		SchemaVersion: SchemaVersion,
		GitHub: GitHubConfig{
			Host: DefaultGitHubHost,
		},
		AI: AIConfig{
			Provider: DefaultAIProvider,
			BaseURL:  DefaultAIBaseURL,
		},
	}
}

func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	return LoadPath(path)
}

func LoadPath(path string) (Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = SchemaVersion
	}
	if cfg.SchemaVersion != SchemaVersion {
		return Config{}, fmt.Errorf("unsupported config schema version %d", cfg.SchemaVersion)
	}
	ApplyDefaults(&cfg)
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return SavePath(path, cfg)
}

func SavePath(path string, cfg Config) error {
	ApplyDefaults(&cfg)
	cfg.SchemaVersion = SchemaVersion
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func ApplyDefaults(cfg *Config) {
	cfg.GitHub.Host = NormalizeHost(cfg.GitHub.Host)
	if cfg.GitHub.Host == "" {
		cfg.GitHub.Host = DefaultGitHubHost
	}
	cfg.AI.Provider = NormalizeName(cfg.AI.Provider)
	if cfg.AI.Provider == "" {
		cfg.AI.Provider = DefaultAIProvider
	}
	if cfg.AI.BaseURL == "" && cfg.AI.Provider == DefaultAIProvider {
		cfg.AI.BaseURL = DefaultAIBaseURL
	}
	cfg.AI.Headers = canonicalHeaderMap(cfg.AI.Headers)
	cfg.AI.SecretHeaders = canonicalHeaderList(cfg.AI.SecretHeaders)
}

func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	return strings.TrimRight(host, "/")
}

func NormalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func CanonicalHeaderName(name string) (string, bool) {
	name = textproto.TrimString(name)
	if !validHeaderName(name) {
		return "", false
	}
	return textproto.CanonicalMIMEHeaderKey(name), true
}

func canonicalHeaderMap(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := map[string]string{}
	for name, value := range headers {
		if canonical, ok := CanonicalHeaderName(name); ok {
			out[canonical] = value
		} else {
			out[name] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func canonicalHeaderList(headers []string) []string {
	if len(headers) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	for _, name := range headers {
		canonical, ok := CanonicalHeaderName(name)
		if !ok {
			out = append(out, name)
			continue
		}
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		out = append(out, canonical)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !isHeaderTokenRune(r) {
			return false
		}
	}
	return true
}

func isHeaderTokenRune(r rune) bool {
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
		return true
	}
	switch r {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}
