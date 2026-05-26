package git

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

type fakeRunner struct {
	run      run.CommandFunc
	lookPath func(string) (string, error)
	pipe     func(context.Context, string, []string, run.Command, run.Command, func(io.Reader) error) error
}

func (f fakeRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
	if f.run == nil {
		return "", "", errors.New("unexpected command")
	}
	return f.run(ctx, dir, env, name, args...)
}

func (f fakeRunner) LookPath(name string) (string, error) {
	if f.lookPath == nil {
		return "", exec.ErrNotFound
	}
	return f.lookPath(name)
}

func (f fakeRunner) Pipe(ctx context.Context, dir string, env []string, left run.Command, right run.Command, consume func(io.Reader) error) error {
	if f.pipe == nil {
		return errors.New("unexpected pipe")
	}
	return f.pipe(ctx, dir, env, left, right, consume)
}

func TestFilterRepoCommandPrefersStandaloneExecutable(t *testing.T) {
	t.Parallel()
	client := New(fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git-filter-repo" {
				return "/usr/bin/git-filter-repo", nil
			}
			return "", exec.ErrNotFound
		},
	})
	cmd, ok := client.FilterRepoCommand(context.Background())
	if !ok {
		t.Fatal("expected standalone git-filter-repo")
	}
	if len(cmd) != 1 || cmd[0] != "/usr/bin/git-filter-repo" {
		t.Fatalf("cmd = %#v", cmd)
	}
}

func TestFilterRepoCommandUsesGitFallback(t *testing.T) {
	t.Parallel()
	client := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) == 2 && args[0] == "filter-repo" && args[1] == "--version" {
			return "git-filter-repo 2.0", "", nil
		}
		return "", "", errors.New("unexpected command")
	}})
	cmd, ok := client.FilterRepoCommand(context.Background())
	if !ok {
		t.Fatal("expected git filter-repo fallback")
	}
	if len(cmd) != 2 || cmd[0] != "git" || cmd[1] != "filter-repo" {
		t.Fatalf("cmd = %#v", cmd)
	}
}

func TestFilterRepoCommandRequiresSuccessfulFallback(t *testing.T) {
	t.Parallel()
	client := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) == 2 && args[0] == "filter-repo" && args[1] == "--version" {
			return "git-filter-repo 2.0", "", errors.New("failed")
		}
		return "", "", errors.New("unexpected command")
	}})
	if cmd, ok := client.FilterRepoCommand(context.Background()); ok {
		t.Fatalf("expected no fallback command, got %#v", cmd)
	}
}

func TestPythonBytesLiteral(t *testing.T) {
	t.Parallel()
	got := PythonBytesLiteral("emoji 😀 café 'quote' \\slash\n")
	want := `b'emoji \xf0\x9f\x98\x80 caf\xc3\xa9 \'quote\' \\slash\n'`
	if got != want {
		t.Fatalf("literal = %q, want %q", got, want)
	}
}

func TestCatFileBatchCheck(t *testing.T) {
	t.Parallel()
	client := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) >= 1 && args[0] == "cat-file" {
			if run.GetStdin(ctx) == "test-hash-1\ntest-hash-2" {
				return "100 test-hash-1 blob\n200 test-hash-2 blob", "", nil
			}
			return "error in input", "", errors.New("invalid stdin")
		}
		return "", "", errors.New("unexpected command")
	}})

	res, err := client.CatFileBatchCheck(context.Background(), "dummy-dir", "test-hash-1\ntest-hash-2")
	if err != nil {
		t.Fatal(err)
	}
	expected := "100 test-hash-1 blob\n200 test-hash-2 blob"
	if res != expected {
		t.Errorf("expected %q, got %q", expected, res)
	}
}
