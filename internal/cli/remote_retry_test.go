package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
)

func TestRetryTransientRemoteRetriesTimeoutsAndTransportErrors(t *testing.T) {
	attempts := 0
	out, err := retryTransientRemote(context.Background(), func() (string, error) {
		attempts++
		if attempts == 1 {
			return "", git.RemoteTimeoutError{Operation: "fetch", Timeout: git.RemoteTimeout}
		}
		if attempts == 2 {
			return "", errors.New("unexpected EOF")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" || attempts != 3 {
		t.Fatalf("out = %q, attempts = %d; want ok after 3 attempts", out, attempts)
	}
}

func TestRetryTransientRemoteDoesNotRetryAuthFailures(t *testing.T) {
	attempts := 0
	out, err := retryTransientRemote(context.Background(), func() (string, error) {
		attempts++
		return "", errors.New("authentication failed")
	})
	if err == nil {
		t.Fatal("expected auth failure")
	}
	if out != "" || attempts != 1 {
		t.Fatalf("out = %q, attempts = %d; want one attempt", out, attempts)
	}
}

func TestAutomaticOriginRefreshRetriesTransientFetchFailure(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	fetches := 0
	inspected := false
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := strings.Join(args, " ")
			if joined == "fetch --prune origin" {
				fetches++
				if fetches == 1 {
					return "", "unexpected EOF", errors.New("unexpected EOF")
				}
			}
			if joined == "status --porcelain=v2 --branch" {
				inspected = true
			}
			return remoteAwareCommandOutput(joined)
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"status"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("status returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if fetches != 2 || !inspected {
		t.Fatalf("fetches = %d, inspected = %v; want retried fetch and inspection", fetches, inspected)
	}
}

func TestFetchCommandRetriesTransientFetchFailure(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	fetches := 0
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" || strings.Join(args, " ") != "fetch origin" {
				return "", "", errors.New("unexpected command")
			}
			fetches++
			if fetches == 1 {
				return "", "Failed to connect to github.com", errors.New("fetch failed")
			}
			return "fetched\n", "", nil
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"fetch"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("fetch returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if fetches != 2 {
		t.Fatalf("fetches = %d, want 2", fetches)
	}
}

func TestResetRetriesPreparationFetch(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	fetches := 0
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := strings.Join(args, " ")
			switch joined {
			case "rev-parse --abbrev-ref HEAD":
				return "main\n", "", nil
			case "fetch origin main":
				fetches++
				if fetches == 1 {
					return "", "connection reset by peer", errors.New("fetch failed")
				}
				return "fetched\n", "", nil
			case "rev-parse --verify --quiet origin/main":
				return "origin/main\n", "", nil
			case "rev-list --count origin/main..main", "rev-list --count main..origin/main":
				return "0\n", "", nil
			default:
				return "", "", errors.New("unexpected git args: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"reset"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("reset returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if fetches != 2 {
		t.Fatalf("fetches = %d, want 2", fetches)
	}
}

func TestCloneRetriesRepositoryListing(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	t.Chdir(root)
	listOne := 0
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" || name == "gh" {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("unexpected command")
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := strings.Join(args, " ")
			switch joined {
			case "repo list user --visibility public --limit 1":
				listOne++
				if listOne == 1 {
					return "", "unexpected EOF", errors.New("unexpected EOF")
				}
				return "owner/repo\n", "", nil
			case "repo clone owner/repo clones/repo":
				return "cloned\n", "", nil
			default:
				return "", "", errors.New("unexpected gh args: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"clone", "--user", "user", "--visibility", "public", "--limit", "1", "--into", "clones"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("clone returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if listOne != 3 {
		t.Fatalf("one-item/full list calls = %d, want 3 including retry and full list", listOne)
	}
}

func TestCloneEmptyRepositoryListIsSuccessfulNoop(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" || name == "gh" {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("unexpected command")
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "gh" || strings.Join(args, " ") != "repo list user --visibility public --limit 1" {
				return "", "", errors.New("unexpected command")
			}
			return "", "", nil
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"clone", "--user", "user", "--visibility", "public", "--limit", "1", "--into", "clones"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("clone empty list returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	for _, want := range []string{"SKIP no public repositories found for 'user'", "Summary: 0 cloned, 0 skipped, 0 failed"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("missing %q:\nstdout:%s\nstderr:%s", want, stdout.String(), stderr.String())
		}
	}
}

func TestCloneRetriesTransientCloneFailureAndCleansPartialTarget(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	t.Chdir(root)
	cloneCalls := 0
	target := filepath.Join(root, "clones", "repo")
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" || name == "gh" {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("unexpected command")
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := strings.Join(args, " ")
			switch joined {
			case "repo list user --visibility public --limit 1":
				return "owner/repo\n", "", nil
			case "repo clone owner/repo clones/repo":
				cloneCalls++
				if cloneCalls == 1 {
					if err := os.MkdirAll(filepath.Join(target, ".git"), 0o755); err != nil {
						t.Fatal(err)
					}
					return "", "unexpected EOF", errors.New("unexpected EOF")
				}
				if fileExists(target) {
					t.Fatal("partial clone target was not removed before retry")
				}
				return "cloned\n", "", nil
			default:
				return "", "", errors.New("unexpected gh args: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"clone", "--user", "user", "--visibility", "public", "--limit", "1", "--into", "clones"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("clone returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if cloneCalls != 2 {
		t.Fatalf("clone calls = %d, want 2", cloneCalls)
	}
}

func TestCloneDoesNotRetryAuthenticationFailure(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	t.Chdir(root)
	cloneCalls := 0
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" || name == "gh" {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("unexpected command")
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := strings.Join(args, " ")
			switch joined {
			case "repo list user --visibility public --limit 1":
				return "owner/repo\n", "", nil
			case "repo clone owner/repo clones/repo":
				cloneCalls++
				return "", "authentication failed", errors.New("authentication failed")
			default:
				return "", "", errors.New("unexpected gh args: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"clone", "--user", "user", "--visibility", "public", "--limit", "1", "--into", "clones"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if cloneCalls != 1 {
		t.Fatalf("clone calls = %d, want 1", cloneCalls)
	}
}
