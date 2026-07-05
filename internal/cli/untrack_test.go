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

func TestChunkStrings(t *testing.T) {
	chunks := chunkStrings([]string{"a", "b", "c", "d", "e"}, 2)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks", len(chunks))
	}
	if got := chunks[0][0] + chunks[0][1] + chunks[2][0]; got != "abe" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
	if chunks := chunkStrings([]string{""}, 100); len(chunks) != 0 {
		t.Fatalf("empty split should be omitted: %#v", chunks)
	}
}

func TestUntrackOnlyUpdatesIndexWithoutCommitting(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	if err := os.WriteFile(filepath.Join(root, "repo", ".gitignore"), []byte("dist/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rmCalls := 0
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" {
				return "", "", errors.New("unexpected command")
			}
			joined := strings.Join(args, " ")
			switch joined {
			case "ls-files --ignored --cached --exclude-standard":
				return "dist/app.js\n", "", nil
			case "ls-files --ignored --cached --exclude-standard -z":
				return "dist/app.js\x00", "", nil
			case "rm --cached -q -- dist/app.js":
				rmCalls++
				return "", "", nil
			default:
				if strings.HasPrefix(joined, "commit ") {
					t.Fatalf("untrack should not create commits: %s", joined)
				}
				return "", "", errors.New("unexpected git args: " + joined)
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"untrack", "--yes"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("untrack returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if rmCalls != 1 {
		t.Fatalf("rm calls = %d, want 1", rmCalls)
	}
	if !strings.Contains(stdout.String(), "Summary: 1 updated, 0 skipped, 0 failed") {
		t.Fatalf("missing apply summary:\nstdout:%s\nstderr:%s", stdout.String(), stderr.String())
	}
}
