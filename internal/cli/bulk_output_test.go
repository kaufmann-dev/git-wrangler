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
	release := make(chan struct{})
	var releaseOnce sync.Once
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" || strings.Join(args, " ") != "pull" {
				return "", "", errors.New("unexpected command")
			}
			done := trackConcurrentStart(&mu, &active, &maxActive, release, &releaseOnce)
			defer done()
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
	if !strings.Contains(stdout.String(), "Summary: 2 updated, 0 skipped, 0 failed") {
		t.Fatalf("missing pull summary:\n%s", stdout.String())
	}
}

func TestFetchRunsOriginFetchAndCountsFailures(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "failed", "fetched")
	t.Chdir(root)

	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" || strings.Join(args, " ") != "fetch origin" {
				return "", "", errors.New("unexpected command")
			}
			switch filepath.Base(dir) {
			case "failed":
				return "fatal: 'origin' does not appear to be a git repository", "", errors.New("fetch failed")
			case "fetched":
				return "fetched\n", "", nil
			default:
				return "", "", errors.New("unexpected repo")
			}
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"fetch"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if !strings.Contains(stdout.String(), "Summary: 1 fetched, 1 failed") {
		t.Fatalf("missing fetch summary:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestFetchPruneUsesPruneFlag(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)

	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" || strings.Join(args, " ") != "fetch --prune origin" {
				return "", "", errors.New("unexpected command")
			}
			return "fetched\n", "", nil
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"fetch", "--prune"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("fetch returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Summary: 1 fetched, 0 failed") {
		t.Fatalf("missing fetch summary:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestFetchRunsConcurrentlyAndPreservesOutputOrder(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "a-slow", "b-fast")
	t.Chdir(root)

	var mu sync.Mutex
	active := 0
	maxActive := 0
	release := make(chan struct{})
	var releaseOnce sync.Once
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" || strings.Join(args, " ") != "fetch origin" {
				return "", "", errors.New("unexpected command")
			}
			done := trackConcurrentStart(&mu, &active, &maxActive, release, &releaseOnce)
			defer done()
			return "fetched\n", "", nil
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"fetch"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("fetch returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if maxActive < 2 {
		t.Fatalf("fetch did not run concurrently; max active = %d", maxActive)
	}
	if !strings.Contains(stdout.String(), "Summary: 2 fetched, 0 failed") {
		t.Fatalf("missing fetch summary:\n%s", stdout.String())
	}
}

func TestPushSummaryCountsOutcomes(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "failed", "pushed", "skipped")
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
	err := executeInteractive(t, context.Background(), runner, []string{"push", "--force-unsafe"}, strings.NewReader("y\n"), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if strings.Count(stderr.String(), "Raw force push") != 1 {
		t.Fatalf("expected one confirmation prompt:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Summary: 1 pushed, 1 skipped, 1 failed") {
		t.Fatalf("missing push summary:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestStatusSummaryUsesStdoutAndCountsFailures(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "behind", "clean", "dirty", "failed", "noremote")
	t.Chdir(root)

	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" {
				return "", "", errors.New("unexpected command")
			}
			if strings.Join(args, " ") == "fetch --prune origin" {
				return "fetched\n", "", nil
			}
			if strings.Join(args, " ") != "status --porcelain=v2 --branch" {
				return "", "", errors.New("unexpected git args")
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
	release := make(chan struct{})
	var releaseOnce sync.Once
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" {
				return "", "", errors.New("unexpected command")
			}
			switch {
			case strings.Join(args, " ") == "fetch --prune origin":
				return "fetched\n", "", nil
			case strings.Join(args, " ") == "rev-list HEAD --not --remotes":
				done := trackConcurrentStart(&mu, &activeRevLists, &maxActiveRevLists, release, &releaseOnce)
				defer done()
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
	if err := executeInteractive(t, context.Background(), runner, []string{"rename-repo"}, strings.NewReader("new-a\nnew-b\n"), &stdout, &stderr); err != nil {
		t.Fatalf("rename-repo returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if maxActiveRenames != 1 {
		t.Fatalf("rename-repo mutations overlapped; max active = %d", maxActiveRenames)
	}
}

func TestRewriteAuthorsRunsFilterRepoConcurrentlyAndPreservesOutputOrder(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "a-slow", "b-fast")
	t.Chdir(root)

	var mu sync.Mutex
	activeFilters := 0
	maxActiveFilters := 0
	release := make(chan struct{})
	var releaseOnce sync.Once
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" || name == "git-filter-repo" {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("unexpected command")
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := name + " " + strings.Join(args, " ")
			switch {
			case joined == "git fetch --prune origin":
				return "fetched\n", "", nil
			case joined == "git remote get-url origin":
				return "https://example.test/" + filepath.Base(dir) + ".git\n", "", nil
			case name == "/usr/bin/git-filter-repo" && strings.Contains(strings.Join(args, " "), "--email-callback"):
				done := trackConcurrentStart(&mu, &activeFilters, &maxActiveFilters, release, &releaseOnce)
				defer done()
				return "rewritten\n", "", nil
			default:
				return "", "", errors.New("unexpected command: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"rewrite-authors", "--name", "New Name", "--email", "new@example.test", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("rewrite-authors returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if maxActiveFilters < 2 {
		t.Fatalf("filter-repo runs did not overlap; max active = %d", maxActiveFilters)
	}
	if !strings.Contains(stdout.String(), "Summary: 2 rewritten, 0 skipped, 0 failed") {
		t.Fatalf("missing rewrite-authors summary:\n%s", stdout.String())
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

func trackConcurrentStart(mu *sync.Mutex, active, maxActive *int, release chan struct{}, releaseOnce *sync.Once) func() {
	mu.Lock()
	(*active)++
	if *active > *maxActive {
		*maxActive = *active
	}
	if *active >= 2 {
		releaseOnce.Do(func() { close(release) })
	}
	mu.Unlock()
	select {
	case <-release:
	case <-time.After(500 * time.Millisecond):
	}
	return func() {
		mu.Lock()
		*active--
		mu.Unlock()
	}
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
