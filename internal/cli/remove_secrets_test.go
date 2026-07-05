package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveSecretsUsesConfiguredPathGlobs(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	configPath := filepath.Join(configDir, "git-wrangler", "remove-secrets.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("paths = [\".env\", \"private/*.json\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	var scanArgs []string
	var filterArgs []string
	runner := fakeRunner{
		lookPath: fakeGitAndFilterRepoLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := name + " " + strings.Join(args, " ")
			switch {
			case joined == "git rev-parse --is-inside-work-tree":
				return "true\n", "", nil
			case strings.HasPrefix(joined, "git log --all --format= --name-only -- "):
				scanArgs = append([]string{}, args...)
				return ".env\nprivate/key.json\n", "", nil
			case strings.HasPrefix(joined, "git update-ref -d "):
				return "", "", nil
			case strings.HasPrefix(joined, "git for-each-ref --format=%(refname) "):
				return "", "", nil
			case joined == "git remote get-url origin":
				return "git@example.test:owner/repo.git\n", "", nil
			case name == "/usr/bin/git-filter-repo":
				filterArgs = append([]string{}, args...)
				return "", "", nil
			default:
				return "", "", errors.New("unexpected command: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"remove-secrets", "--no-fetch", "--yes"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("remove-secrets returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if !containsArgAfterSeparator(scanArgs, "--", "private/*.json") || !containsArgAfterSeparator(scanArgs, "--", ".env") {
		t.Fatalf("scan args missing configured globs: %v", scanArgs)
	}
	if containsArgAfterSeparator(scanArgs, "--", ".npmrc") {
		t.Fatalf("scan args should not include defaults absent from the config file: %v", scanArgs)
	}
	if !containsArgAfter(filterArgs, "--path-glob", "private/*.json") || !containsArgAfter(filterArgs, "--path-glob", ".env") {
		t.Fatalf("filter args missing configured globs: %v", filterArgs)
	}
}

func TestRemoveSecretsUsesDefaultsWhenNoConfig(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	var scanArgs []string
	runner := fakeRunner{
		lookPath: fakeGitAndFilterRepoLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := name + " " + strings.Join(args, " ")
			switch {
			case joined == "git rev-parse --is-inside-work-tree":
				return "true\n", "", nil
			case strings.HasPrefix(joined, "git log --all --format= --name-only -- "):
				scanArgs = append([]string{}, args...)
				return "", "", nil
			default:
				return "", "", errors.New("unexpected command: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"remove-secrets", "--no-fetch", "--yes"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("remove-secrets returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if !containsArgAfterSeparator(scanArgs, "--", ".env") {
		t.Fatalf("scan args missing built-in default glob: %v", scanArgs)
	}
}

func TestRemoveSecretsInvalidConfigFailsBeforeDependencies(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	configPath := filepath.Join(configDir, "git-wrangler", "remove-secrets.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("paths = [\"../secret\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lookedUp := false
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			lookedUp = true
			return "", errors.New("dependency lookup should not run")
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"remove-secrets", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if lookedUp {
		t.Fatal("dependency lookup ran before config validation")
	}
	if !strings.Contains(stderr.String(), "read remove-secrets config") {
		t.Fatalf("missing validation error:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
}

func containsArgAfter(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func containsArgAfterSeparator(args []string, separator, value string) bool {
	after := false
	for _, arg := range args {
		if after && arg == value {
			return true
		}
		if arg == separator {
			after = true
		}
	}
	return false
}
