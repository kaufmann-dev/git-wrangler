package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPullSummaryCountsOutcomes(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "failed", "skipped", "updated")
	t.Chdir(root)

	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" || strings.Join(args, " ") != "pull --rebase" {
				return "", "", errors.New("unexpected command")
			}
			switch filepath.Base(dir) {
			case "failed":
				return "", "pull failed", errors.New("pull failed")
			case "skipped":
				return "Already up to date.\n", "", nil
			case "updated":
				return "updated\n", "", nil
			default:
				return "", "", errors.New("unexpected repo")
			}
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"pull", "--rebase"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if !strings.Contains(stdout.String(), "Summary: 1 updated, 1 skipped, 1 failed") {
		t.Fatalf("missing pull summary:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestPullRunsConcurrentlyAndPreservesOutputOrder(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "a-slow", "b-fast")
	t.Chdir(root)

	var mu sync.Mutex
	active := 0
	maxActive := 0
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" || strings.Join(args, " ") != "pull" {
				return "", "", errors.New("unexpected command")
			}
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			if filepath.Base(dir) == "a-slow" {
				time.Sleep(50 * time.Millisecond)
			}
			mu.Lock()
			active--
			mu.Unlock()
			return "updated\n", "", nil
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"pull"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("pull returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if maxActive < 2 {
		t.Fatalf("pull did not run concurrently; max active = %d", maxActive)
	}
	out := stdout.String()
	first := strings.Index(out, "a-slow")
	second := strings.Index(out, "b-fast")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("pull output not in repo order:\n%s", out)
	}
}

func TestPushSummaryCountsOutcomes(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "declined", "failed", "pushed", "skipped")
	t.Chdir(root)

	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" || strings.Join(args, " ") != "push --force origin HEAD" {
				return "", "", errors.New("unexpected command")
			}
			switch filepath.Base(dir) {
			case "failed":
				return "", "push failed", errors.New("push failed")
			case "pushed":
				return "pushed\n", "", nil
			case "skipped":
				return "Everything up-to-date\n", "", nil
			default:
				return "", "", errors.New("unexpected repo")
			}
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"push", "--force-unsafe"}, strings.NewReader("n\ny\ny\ny\n"), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if !strings.Contains(stdout.String(), "Summary: 1 pushed, 2 skipped, 1 failed") {
		t.Fatalf("missing push summary:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestCommitSummaryCountsOutcomes(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "committed", "failed", "skipped")
	t.Chdir(root)

	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" {
				return "", "", errors.New("unexpected command")
			}
			repoName := filepath.Base(dir)
			switch strings.Join(args, " ") {
			case "add -A":
				return "", "", nil
			case "diff --cached --quiet":
				if repoName == "skipped" {
					return "", "", nil
				}
				return "", "", errors.New("changes")
			case "commit -m Test commit":
				if repoName == "failed" {
					return "commit failed", "", errors.New("commit failed")
				}
				return "committed\n", "", nil
			default:
				return "", "", errors.New("unexpected git args")
			}
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"commit", "--message", "Test commit"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if !strings.Contains(stdout.String(), "Summary: 1 committed, 1 skipped, 1 failed") {
		t.Fatalf("missing commit summary:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestCommitRunsConcurrentlyAndPreservesOutputOrder(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "a-slow", "b-fast")
	t.Chdir(root)

	var mu sync.Mutex
	activeAdds := 0
	maxActiveAdds := 0
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" {
				return "", "", errors.New("unexpected command")
			}
			switch strings.Join(args, " ") {
			case "add -A":
				mu.Lock()
				activeAdds++
				if activeAdds > maxActiveAdds {
					maxActiveAdds = activeAdds
				}
				mu.Unlock()
				if filepath.Base(dir) == "a-slow" {
					time.Sleep(50 * time.Millisecond)
				}
				mu.Lock()
				activeAdds--
				mu.Unlock()
				return "", "", nil
			case "diff --cached --quiet":
				return "", "", errors.New("changes")
			case "commit -m Test commit":
				return "committed\n", "", nil
			default:
				return "", "", errors.New("unexpected git args")
			}
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"commit", "--message", "Test commit"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("commit returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if maxActiveAdds < 2 {
		t.Fatalf("commit did not run concurrently; max active adds = %d", maxActiveAdds)
	}
	out := stdout.String()
	first := strings.Index(out, "a-slow")
	second := strings.Index(out, "b-fast")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("commit output not in repo order:\n%s", out)
	}
}

func TestStatusSummaryUsesStdoutAndCountsFailures(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "behind", "clean", "dirty", "failed", "noremote")
	t.Chdir(root)

	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" || strings.Join(args, " ") != "status --porcelain=v2 --branch" {
				return "", "", errors.New("unexpected command")
			}
			switch filepath.Base(dir) {
			case "behind":
				return "# branch.upstream origin/main\n# branch.ab +0 -2\n", "", nil
			case "clean":
				return "# branch.upstream origin/main\n# branch.ab +0 -0\n", "", nil
			case "dirty":
				return "# branch.upstream origin/main\n# branch.ab +0 -0\n1 .M N... 100644 100644 100644 abc abc main.go\n", "", nil
			case "failed":
				return "", "status failed", errors.New("status failed")
			case "noremote":
				return "# branch.head main\n", "", nil
			default:
				return "", "", errors.New("unexpected repo")
			}
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"status"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if !strings.Contains(stdout.String(), "Summary: 3 clean, 1 dirty, 1 behind, 1 no remote, 1 failed") {
		t.Fatalf("missing status summary:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "Summary:") {
		t.Fatalf("status summary should be on stdout, got stderr:\n%s", stderr.String())
	}
}

func TestReviewRunsConcurrentlyAndPreservesOutputOrder(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "a-slow", "b-fast")
	t.Chdir(root)

	var mu sync.Mutex
	activeRevLists := 0
	maxActiveRevLists := 0
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" {
				return "", "", errors.New("unexpected command")
			}
			switch {
			case strings.Join(args, " ") == "rev-list HEAD --not --remotes":
				mu.Lock()
				activeRevLists++
				if activeRevLists > maxActiveRevLists {
					maxActiveRevLists = activeRevLists
				}
				mu.Unlock()
				if filepath.Base(dir) == "a-slow" {
					time.Sleep(50 * time.Millisecond)
				}
				mu.Lock()
				activeRevLists--
				mu.Unlock()
				return "commit\n", "", nil
			case strings.HasPrefix(strings.Join(args, " "), "rev-parse --verify commit^"):
				return "", "", errors.New("no parent")
			case len(args) >= 1 && args[0] == "diff":
				return "A\x00file.go\x00", "", nil
			default:
				return "", "", errors.New("unexpected git args")
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"review"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("review returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if maxActiveRevLists < 2 {
		t.Fatalf("review did not run concurrently; max active rev-lists = %d", maxActiveRevLists)
	}
	out := stdout.String()
	first := strings.Index(out, "a-slow")
	second := strings.Index(out, "b-fast")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("review output not in repo order:\n%s", out)
	}
}

func TestCloneRunsGitHubCloneSerially(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	t.Chdir(root)

	var mu sync.Mutex
	activeClones := 0
	maxActiveClones := 0
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" || name == "gh" {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("unexpected command")
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "gh" {
				return "", "", errors.New("unexpected command")
			}
			joined := strings.Join(args, " ")
			switch joined {
			case "repo list user --visibility public --limit 1":
				return "owner/a\n", "", nil
			case "repo list user --visibility public --limit 2":
				return "owner/a\nowner/b\n", "", nil
			case "repo clone owner/a clones/a", "repo clone owner/b clones/b":
				mu.Lock()
				activeClones++
				if activeClones > maxActiveClones {
					maxActiveClones = activeClones
				}
				mu.Unlock()
				time.Sleep(25 * time.Millisecond)
				mu.Lock()
				activeClones--
				mu.Unlock()
				return "cloned\n", "", nil
			default:
				return "", "", errors.New("unexpected gh args: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"clone", "--user", "user", "--visibility", "public", "--limit", "2", "--into", "clones"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("clone returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if maxActiveClones != 1 {
		t.Fatalf("clone operations overlapped; max active = %d", maxActiveClones)
	}
}

func TestRenameRepoRunsGitHubMutationsSerially(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_WRANGLER_GITHUB_TOKEN", "token")
	root := tempGitRepos(t, "a-slow", "b-fast")
	t.Chdir(root)

	var mu sync.Mutex
	activeRenames := 0
	maxActiveRenames := 0
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" || name == "gh" {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("unexpected command")
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "gh" {
				return "", "", errors.New("unexpected command")
			}
			joined := strings.Join(args, " ")
			switch {
			case joined == "repo view --json name -q .name":
				return filepath.Base(dir) + "\n", "", nil
			case strings.HasPrefix(joined, "repo rename "):
				mu.Lock()
				activeRenames++
				if activeRenames > maxActiveRenames {
					maxActiveRenames = activeRenames
				}
				mu.Unlock()
				time.Sleep(25 * time.Millisecond)
				mu.Lock()
				activeRenames--
				mu.Unlock()
				return "renamed\n", "", nil
			default:
				return "", "", errors.New("unexpected gh args: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"rename-repo"}, strings.NewReader("new-a\nnew-b\n"), &stdout, &stderr); err != nil {
		t.Fatalf("rename-repo returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if maxActiveRenames != 1 {
		t.Fatalf("rename-repo mutations overlapped; max active = %d", maxActiveRenames)
	}
}

func tempGitRepos(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		if err := os.MkdirAll(filepath.Join(root, name, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func fakeGitLookPath(name string) (string, error) {
	if name == "git" {
		return "/usr/bin/git", nil
	}
	return "", errors.New("unexpected command")
}

func assertExitCode(t *testing.T, err error, code int) {
	t.Helper()
	var exit exitError
	if !errors.As(err, &exit) || exit.code != code {
		t.Fatalf("expected exitError(%d), got %T %v", code, err, err)
	}
}
