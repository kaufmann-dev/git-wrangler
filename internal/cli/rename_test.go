package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenameBranchDeclineSkipsWithoutMutation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	mutations := 0
	runner := renameBranchRunner(t, &mutations)

	var stdout, stderr bytes.Buffer
	if err := executeInteractive(t, context.Background(), runner, []string{"rename-branch", "--oldbranch", "master", "--newbranch", "main"}, strings.NewReader("n\n"), &stdout, &stderr); err != nil {
		t.Fatalf("rename-branch decline returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if mutations != 0 {
		t.Fatalf("branch rename ran after declined confirmation: %d", mutations)
	}
	if !strings.Contains(stdout.String(), "Summary: 0 renamed, 1 skipped, 0 failed") {
		t.Fatalf("missing decline summary:\nstdout:%s\nstderr:%s", stdout.String(), stderr.String())
	}
}

func TestRenameBranchYesAppliesAfterPreview(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	mutations := 0
	runner := renameBranchRunner(t, &mutations)

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"rename-branch", "--oldbranch", "master", "--newbranch", "main", "--yes"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("rename-branch returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if mutations != 1 {
		t.Fatalf("branch rename calls = %d, want 1", mutations)
	}
	for _, want := range []string{"Repository", "Old branch", "New branch", "Summary: 1 renamed, 0 skipped, 0 failed"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("missing %q in preview/output:\nstdout:%s\nstderr:%s", want, stdout.String(), stderr.String())
		}
	}
}

func renameBranchRunner(t *testing.T, mutations *int) fakeRunner {
	t.Helper()
	return fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" {
				return "", "", errors.New("unexpected command")
			}
			if filepath.Base(dir) != "repo" {
				return "", "", errors.New("unexpected repo")
			}
			joined := strings.Join(args, " ")
			switch joined {
			case "rev-parse --is-inside-work-tree":
				return "true\n", "", nil
			case "rev-parse --verify --quiet refs/heads/master":
				return "abc123\n", "", nil
			case "rev-parse --verify --quiet refs/heads/main":
				return "", "", errors.New("missing")
			case "branch -m master main":
				(*mutations)++
				return "", "", nil
			default:
				return "", "", errors.New("unexpected git args: " + joined)
			}
		},
	}
}
