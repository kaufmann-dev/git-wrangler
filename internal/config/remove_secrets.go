package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// DefaultRemoveSecretsPaths is the built-in seed of sensitive path globs that
// git-wrangler remove-secrets purges when the config file does not exist yet.
// Once the config file is created it fully defines the list; these defaults
// only apply while no file is present.
func DefaultRemoveSecretsPaths() []string {
	return []string{
		".env", ".env.*", ".npmrc", ".pypirc", ".netrc", ".git-credentials",
		"*.pem", "*.key", "*.p12", "*.pfx", "*.asc", "*.gpg", "*.crt", "*.cer", "*.cert",
		"id_rsa", "id_rsa.pub", "id_ed25519", "id_ed25519.pub", "*_rsa", "*_ed25519",
		"secrets.json", "credentials.json", "*secret*.json", "*credential*.json", "*.secret",
		"config/credentials.yml.enc", ".docker/config.json", ".kube/config", "kubeconfig",
		".aws/credentials", ".aws/config", ".config/gcloud/*", "application_default_credentials.json",
		"azureProfile.json", "accessTokens.json",
	}
}

type RemoveSecretsConfig struct {
	Paths []string `toml:"paths"`
}

func RemoveSecretsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "git-wrangler", "remove-secrets.toml"), nil
}

// LoadRemoveSecretsPaths returns the effective remove-secrets globs. When the
// config file is absent it returns the built-in defaults and reports
// usingDefaults=true; otherwise the file's paths are the complete list.
func LoadRemoveSecretsPaths() (paths []string, usingDefaults bool, err error) {
	configPath, err := RemoveSecretsPath()
	if err != nil {
		return nil, false, err
	}
	return LoadRemoveSecretsPathsPath(configPath)
}

func LoadRemoveSecretsPathsPath(configPath string) (paths []string, usingDefaults bool, err error) {
	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		defaults, err := ValidateRemoveSecretsPaths(DefaultRemoveSecretsPaths())
		if err != nil {
			return nil, false, err
		}
		return defaults, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	var cfg RemoveSecretsConfig
	decoder := toml.NewDecoder(strings.NewReader(string(data))).DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return nil, false, fmt.Errorf("read remove-secrets config: %w", err)
	}
	paths, err = ValidateRemoveSecretsPaths(cfg.Paths)
	if err != nil {
		return nil, false, fmt.Errorf("read remove-secrets config: %w", err)
	}
	return paths, false, nil
}

// EnsureRemoveSecretsStarter creates the config file seeded with the built-in
// default paths when it does not already exist, then returns its path.
func EnsureRemoveSecretsStarter() (string, error) {
	configPath, err := RemoveSecretsPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return "", err
	}
	return configPath, os.WriteFile(configPath, []byte(removeSecretsStarter()), 0o600)
}

func removeSecretsStarter() string {
	paths, _ := ValidateRemoveSecretsPaths(DefaultRemoveSecretsPaths())
	var b strings.Builder
	b.WriteString("# Path globs purged from Git history by git-wrangler remove-secrets.\n")
	b.WriteString("# This file is the complete list: remove-secrets purges exactly these globs.\n")
	b.WriteString("# It is seeded with the built-in defaults below. Add your own paths, or\n")
	b.WriteString("# delete any you do not want purged.\n\n")
	b.WriteString("paths = [\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "  %q,\n", p)
	}
	b.WriteString("]\n")
	return b.String()
}

func ValidateRemoveSecretsPaths(paths []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		normalized, err := normalizeRemoveSecretsPath(raw)
		if err != nil {
			return nil, err
		}
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeRemoveSecretsPath(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("remove-secrets paths cannot be empty")
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" || path.IsAbs(value) || looksLikeWindowsAbs(value) {
		return "", fmt.Errorf("remove-secrets path %q must be relative", raw)
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return "", fmt.Errorf("remove-secrets path %q must not contain .. traversal", raw)
		}
	}
	if _, err := path.Match(value, ""); err != nil {
		return "", fmt.Errorf("remove-secrets path %q has invalid glob syntax: %w", raw, err)
	}
	return value, nil
}

func looksLikeWindowsAbs(value string) bool {
	if len(value) < 3 || value[1] != ':' || value[2] != '/' {
		return false
	}
	c := value[0]
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}
