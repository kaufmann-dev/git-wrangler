package config

import (
	"encoding/json"
	"errors"
	"fmt"
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
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model,omitempty"`
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
