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

func TestFixGitignoreOnlyModifiesFileWithoutStagingOrCommitting(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	if err := os.MkdirAll(filepath.Join(root, "repo", "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" {
				return "", "", errors.New("unexpected command")
			}
			joined := strings.Join(args, " ")
			switch joined {
			case "check-ignore -q ./dist":
				return "", "", errors.New("not ignored")
			default:
				if strings.HasPrefix(joined, "add ") || strings.HasPrefix(joined, "commit ") {
					t.Fatalf("fix-gitignore should not stage or commit: %s", joined)
				}
				return "", "", errors.New("unexpected git args: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"fix-gitignore", "--yes"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("fix-gitignore returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(root, "repo", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "dist/\n") {
		t.Fatalf(".gitignore missing dist entry:\n%s", string(data))
	}
	if !strings.Contains(stdout.String(), "Summary: 1 updated, 0 skipped, 0 failed") {
		t.Fatalf("missing apply summary:\nstdout:%s\nstderr:%s", stdout.String(), stderr.String())
	}
}
