package git

import (
	"context"
	"errors"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func TestFilterRepoCommandPrefersStandaloneExecutable(t *testing.T) {
	if _, err := run.LookPath("git-filter-repo"); err == nil {
		t.Skip("real git-filter-repo is installed")
	}
	restore := run.SetCommandFunc(func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) == 2 && args[0] == "filter-repo" && args[1] == "--version" {
			return "git-filter-repo 2.0", "", nil
		}
		return "", "", errors.New("unexpected command")
	})
	defer restore()
	cmd, ok := FilterRepoCommand(context.Background())
	if !ok {
		t.Fatal("expected git filter-repo fallback")
	}
	if len(cmd) != 2 || cmd[0] != "git" || cmd[1] != "filter-repo" {
		t.Fatalf("cmd = %#v", cmd)
	}
}

func TestFilterRepoCommandRequiresSuccessfulFallback(t *testing.T) {
	if _, err := run.LookPath("git-filter-repo"); err == nil {
		t.Skip("real git-filter-repo is installed")
	}
	restore := run.SetCommandFunc(func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) == 2 && args[0] == "filter-repo" && args[1] == "--version" {
			return "git-filter-repo 2.0", "", errors.New("failed")
		}
		return "", "", errors.New("unexpected command")
	})
	defer restore()
	if cmd, ok := FilterRepoCommand(context.Background()); ok {
		t.Fatalf("expected no fallback command, got %#v", cmd)
	}
}

func TestPythonBytesLiteral(t *testing.T) {
	got := PythonBytesLiteral("emoji 😀 café 'quote' \\slash\n")
	want := `b'emoji \xf0\x9f\x98\x80 caf\xc3\xa9 \'quote\' \\slash\n'`
	if got != want {
		t.Fatalf("literal = %q, want %q", got, want)
	}
}

func TestCatFileBatchCheck(t *testing.T) {
	restore := run.SetCommandFunc(func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) >= 1 && args[0] == "cat-file" {
			stdinInput := run.GetStdin(ctx)
			if stdinInput == "test-hash-1\ntest-hash-2" {
				return "100 test-hash-1 blob\n200 test-hash-2 blob", "", nil
			}
			return "error in input", "", errors.New("invalid stdin")
		}
		return "", "", errors.New("unexpected command")
	})
	defer restore()

	res, err := CatFileBatchCheck(context.Background(), "dummy-dir", "test-hash-1\ntest-hash-2")
	if err != nil {
		t.Fatal(err)
	}
	expected := "100 test-hash-1 blob\n200 test-hash-2 blob"
	if res != expected {
		t.Errorf("expected %q, got %q", expected, res)
	}
}
