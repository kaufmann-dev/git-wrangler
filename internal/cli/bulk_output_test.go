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
