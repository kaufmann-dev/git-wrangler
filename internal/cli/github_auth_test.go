package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
)

func TestClonePassesGitWranglerAuthToGhAndSkipsGhAuthStatus(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	into := t.TempDir()
	var ghCalls []string
	var ghEnvs [][]string
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "gh" {
				return "", "", errors.New("unexpected command: " + name)
			}
			joinedArgs := strings.Join(args, " ")
			ghCalls = append(ghCalls, joinedArgs)
			ghEnvs = append(ghEnvs, append([]string{}, env...))
			if joinedArgs == "auth status" {
				return "", "", errors.New("gh auth status should not run")
			}
			if strings.HasPrefix(joinedArgs, "repo list octo") {
				return "octo/repo\tprivate\n", "", nil
			}
			if strings.HasPrefix(joinedArgs, "repo clone octo/repo ") {
				return "", "", nil
			}
			return "", "", errors.New("unexpected gh args: " + joinedArgs)
		},
	}
	store := &fakeCredentialStore{values: map[string]string{credentials.GitHubAccount("github.com"): "github-token"}}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
	a.creds = store
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"clone", "--user", "octo", "--visibility", "all", "--into", into})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("clone returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if len(ghCalls) != 3 {
		t.Fatalf("gh calls = %#v", ghCalls)
	}
	for _, env := range ghEnvs {
		joined := strings.Join(env, "\n")
		if !strings.Contains(joined, "GH_TOKEN=github-token") || !strings.Contains(joined, "GH_HOST=github.com") {
			t.Fatalf("missing auth env in %#v", env)
		}
	}
}

func TestCloneIgnoresInboundGHToken(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_WRANGLER_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "foreign-token")
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			return "", "", errors.New("gh should not run")
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
	a.creds = &fakeCredentialStore{}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"clone", "--user", "octo", "--visibility", "all"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected missing Git Wrangler auth failure")
	}
	if !strings.Contains(stderr.String(), "Git Wrangler GitHub auth is required") {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}

func TestCloneHidesUnavailableCredentialStorageError(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_WRANGLER_GITHUB_TOKEN", "")
	backendErr := "org.freedesktop.secrets was not provided"
	ghCalled := false
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			ghCalled = true
			return "", "", errors.New("gh should not run")
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
	a.creds = &fakeCredentialStore{err: errors.New(backendErr)}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"clone", "--user", "octo"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected unavailable credential storage failure")
	}
	if ghCalled {
		t.Fatal("clone invoked gh without required authentication")
	}
	for _, want := range []string{"Secure credential storage is unavailable", "GIT_WRANGLER_GITHUB_TOKEN", "--visibility public"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("missing %q guidance:\n%s", want, stderr.String())
		}
	}
	if strings.Contains(stderr.String(), backendErr) {
		t.Fatalf("backend error was exposed:\n%s", stderr.String())
	}
}

func TestPublicCloneContinuesWithoutCredentialStorage(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_WRANGLER_GITHUB_TOKEN", "invalid-configured-token")
	t.Setenv("GH_TOKEN", "foreign-token")
	t.Setenv("GITHUB_TOKEN", "foreign-token")
	into := filepath.Join(t.TempDir(), "clones")
	var ghEnvs [][]string
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "gh" {
				return "", "", errors.New("unexpected command: " + name)
			}
			ghEnvs = append(ghEnvs, append([]string{}, env...))
			joinedArgs := strings.Join(args, " ")
			if strings.HasPrefix(joinedArgs, "repo list octo") {
				return "octo/repo\tpublic\n", "", nil
			}
			if strings.HasPrefix(joinedArgs, "repo clone octo/repo ") {
				return "", "", nil
			}
			return "", "", errors.New("unexpected gh args: " + joinedArgs)
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
	a.creds = &fakeCredentialStore{err: errors.New("keyring unavailable")}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"clone", "--user", "octo", "--visibility", "public", "--into", into})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("public clone returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	for _, env := range ghEnvs {
		joined := strings.Join(env, "\n")
		for _, want := range []string{"GH_TOKEN=", "GITHUB_TOKEN=", "GH_ENTERPRISE_TOKEN=", "GITHUB_ENTERPRISE_TOKEN="} {
			if !strings.Contains(joined, want) {
				t.Fatalf("public clone did not mask %s: %#v", want, env)
			}
		}
	}
}

func TestRenameRepoRequiresGitWranglerAuthAndPassesEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	var ghEnv []string
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name == "gh" && strings.Join(args, " ") == "api user -q .login" {
				return "octo\n", "", nil
			}
			if name == "gh" && strings.Join(args, " ") == "repo view --json name -q .name" {
				ghEnv = append([]string{}, env...)
				return "repo\n", "", nil
			}
			return "", "", errors.New("unexpected command")
		},
	}
	store := &fakeCredentialStore{values: map[string]string{credentials.GitHubAccount("github.com"): "github-token"}}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader("\n"), &stdout, &stderr)
	makeInteractive(a)
	a.creds = store
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"rename-repo"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	t.Chdir(root)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rename-repo returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	joined := strings.Join(ghEnv, "\n")
	if !strings.Contains(joined, "GH_TOKEN=github-token") || !strings.Contains(joined, "GH_HOST=github.com") {
		t.Fatalf("missing auth env in %#v", ghEnv)
	}
}

func TestRenameRepoCancellationStopsBeforeMutation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	rootDir := t.TempDir()
	repo := filepath.Join(rootDir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	mutated := false
	runner := fakeRunner{
		lookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name == "gh" && strings.Join(args, " ") == "api user -q .login" {
				return "octo\n", "", nil
			}
			if name == "gh" && strings.Join(args, " ") == "repo view --json name -q .name" {
				return "repo\n", "", nil
			}
			if name == "gh" {
				mutated = true
			}
			return "", "", errors.New("unexpected command")
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := newBlockingPromptReader()
	var stdout, stderr bytes.Buffer
	a := newApp(ctx, runner, reader, &stdout, &stderr)
	makeInteractive(a)
	a.creds = &fakeCredentialStore{values: map[string]string{credentials.GitHubAccount("github.com"): "github-token"}}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"rename-repo"})
	t.Chdir(rootDir)
	result := make(chan error, 1)
	go func() { result <- cmd.Execute() }()
	<-reader.started
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected cancellation failure")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("rename-repo cancellation waited for input")
	}
	if mutated {
		t.Fatal("rename-repo mutated after prompt cancellation")
	}
	if got := strings.Count(stdout.String(), "SKIP stopped: operation cancelled"); got != 1 {
		t.Fatalf("cancellation status count = %d\n%s", got, stdout.String())
	}
}

func TestRenameRepoFailsBeforeTargetDiscoveryWhenAuthenticationIsInvalid(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "repo", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	var calls []string
	runner := fakeRunner{
		lookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return "", "invalid token", errors.New("invalid token")
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
	makeInteractive(a)
	a.creds = &fakeCredentialStore{values: map[string]string{credentials.GitHubAccount("github.com"): "bad-token"}}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"rename-repo"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	t.Chdir(root)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected authentication failure")
	}
	if got, want := calls, []string{"gh api user -q .login"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("gh calls = %#v, want %#v", got, want)
	}
	if !strings.Contains(stderr.String(), "GitHub authentication failed") {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}

func TestRenameRepoHidesUnavailableCredentialStorageError(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_WRANGLER_GITHUB_TOKEN", "")
	backendErr := "org.freedesktop.secrets was not provided"
	ghCalled := false
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			ghCalled = true
			return "", "", errors.New("gh should not run")
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
	makeInteractive(a)
	a.creds = &fakeCredentialStore{err: errors.New(backendErr)}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"rename-repo"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected unavailable credential storage failure")
	}
	if ghCalled {
		t.Fatal("rename-repo invoked gh without required authentication")
	}
	for _, want := range []string{"Secure credential storage is unavailable", "GIT_WRANGLER_GITHUB_TOKEN"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("missing %q guidance:\n%s", want, stderr.String())
		}
	}
	if strings.Contains(stderr.String(), backendErr) {
		t.Fatalf("backend error was exposed:\n%s", stderr.String())
	}
}

func TestRewriteCommitsFailsBeforeRepositoryScanWhenKeyMissing(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.Defaults()
	cfg.AI.Model = "gpt-test"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	var lookedUp bool
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			lookedUp = true
			return "", errors.New("unexpected lookpath")
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
	a.creds = &fakeCredentialStore{}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"rewrite-commits"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing model failure")
	}
	if lookedUp {
		t.Fatal("rewrite-commits checked dependencies before config credentials")
	}
	if !strings.Contains(stderr.String(), "AI API key is required") {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}

func TestCommitFailsBeforeRepositoryScanWhenKeyMissing(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.Defaults()
	cfg.AI.Model = "gpt-test"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	var lookedUp bool
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			lookedUp = true
			return "", errors.New("unexpected lookpath")
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
	a.creds = &fakeCredentialStore{}
	cmd := newRootCommand(a)
	cmd.SetArgs([]string{"commit"})
	cmd.SetIn(a.stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing key failure")
	}
	if lookedUp {
		t.Fatal("commit checked dependencies before config credentials")
	}
	if !strings.Contains(stderr.String(), "AI API key is required") {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}
