package git

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func recordGit(calls *[][]string, reply func(args []string) (string, string, error)) fakeRunner {
	return fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		*calls = append(*calls, append([]string{name}, args...))
		return reply(args)
	}}
}

func TestIsZero(t *testing.T) {
	if !(Client{}).IsZero() {
		t.Fatal("zero-value client should report IsZero")
	}
	if New(nil).IsZero() {
		t.Fatal("New(nil) should install a default runner")
	}
	if New(fakeRunner{}).IsZero() {
		t.Fatal("constructed client should not be zero")
	}
}

func TestReadCommandsSendExpectedArgs(t *testing.T) {
	cases := []struct {
		name string
		call func(Client) (string, error)
		want string
	}{
		{"StatusPorcelain", func(c Client) (string, error) { return c.StatusPorcelain(context.Background(), "/r") }, "git status --porcelain"},
		{"StatusPorcelainBranch", func(c Client) (string, error) { return c.StatusPorcelainBranch(context.Background(), "/r") }, "git status --porcelain=v2 --branch"},
		{"CurrentBranch", func(c Client) (string, error) { return c.CurrentBranch(context.Background(), "/r") }, "git rev-parse --abbrev-ref HEAD"},
		{"Stdout", func(c Client) (string, error) { return c.Stdout(context.Background(), "/r", nil, "log", "--oneline") }, "git log --oneline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			c := New(recordGit(&calls, func(args []string) (string, string, error) { return "output\n", "", nil }))
			out, err := tc.call(c)
			if err != nil {
				t.Fatal(err)
			}
			if out != "output\n" {
				t.Fatalf("out = %q", out)
			}
			if len(calls) != 1 || strings.Join(calls[0], " ") != tc.want {
				t.Fatalf("calls = %v, want %q", calls, tc.want)
			}
		})
	}
}

func TestHasHeadAndVerifyRefReflectRunnerResult(t *testing.T) {
	ok := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		return "abc\n", "", nil
	}})
	fail := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		return "", "fatal: bad revision", errors.New("exit 128")
	}})
	if !ok.HasHead(context.Background(), "/r") || ok.VerifyRef(context.Background(), "/r", "main") == false {
		t.Fatal("success runner should report HasHead/VerifyRef true")
	}
	if fail.HasHead(context.Background(), "/r") || fail.VerifyRef(context.Background(), "/r", "main") {
		t.Fatal("failing runner should report HasHead/VerifyRef false")
	}
}

func TestRemoteURLTrimsAndFallsBackToEmpty(t *testing.T) {
	ok := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		return "git@example.com:o/r.git\n", "", nil
	}})
	if got := ok.RemoteURL(context.Background(), "/r", "origin"); got != "git@example.com:o/r.git" {
		t.Fatalf("url = %q", got)
	}
	fail := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		return "", "no such remote", errors.New("exit 2")
	}})
	if got := fail.RemoteURL(context.Background(), "/r", "origin"); got != "" {
		t.Fatalf("expected empty url on error, got %q", got)
	}
}

func TestRestoreRemote(t *testing.T) {
	noop := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		t.Fatalf("runner must not run for an empty url: %v", args)
		return "", "", nil
	}})
	if err := noop.RestoreRemote(context.Background(), "/r", "origin", ""); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	existing := New(recordGit(&calls, func(args []string) (string, string, error) { return "url\n", "", nil }))
	if err := existing.RestoreRemote(context.Background(), "/r", "origin", "git@x:o/r.git"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || strings.Join(calls[0], " ") != "git remote get-url origin" {
		t.Fatalf("existing remote should only probe get-url, got %v", calls)
	}

	calls = nil
	missing := New(recordGit(&calls, func(args []string) (string, string, error) {
		if len(args) >= 2 && args[1] == "get-url" {
			return "", "", errors.New("no such remote")
		}
		return "", "", nil
	}))
	if err := missing.RestoreRemote(context.Background(), "/r", "origin", "git@x:o/r.git"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || strings.Join(calls[1], " ") != "git remote add origin git@x:o/r.git" {
		t.Fatalf("missing remote should be re-added, got %v", calls)
	}
}

func TestCaptureRemoteWrapsDeadlineAsTimeout(t *testing.T) {
	parent, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	c := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		return "", "", context.DeadlineExceeded
	}})
	_, err := c.CaptureRemote(parent, "/r", nil, "fetch", "--prune")
	var timeout RemoteTimeoutError
	if !errors.As(err, &timeout) {
		t.Fatalf("expected RemoteTimeoutError, got %v", err)
	}
	if timeout.Operation != "fetch" {
		t.Fatalf("operation = %q, want fetch", timeout.Operation)
	}
}

func TestCaptureRemotePassesThroughOrdinaryResults(t *testing.T) {
	fail := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		return "", "boom", errors.New("boom")
	}})
	_, err := fail.CaptureRemote(context.Background(), "", nil, "push")
	var timeout RemoteTimeoutError
	if err == nil || errors.As(err, &timeout) {
		t.Fatalf("non-timeout failure should pass through, got %v", err)
	}

	ok := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		return "done\n", "", nil
	}})
	out, err := ok.CaptureRemote(context.Background(), "", nil, "fetch")
	if err != nil || out != "done\n" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestRemoteTimeoutErrorAndOperation(t *testing.T) {
	if got := (RemoteTimeoutError{Operation: "fetch", Timeout: 30 * time.Second}).Error(); got != "git fetch timed out after 30s" {
		t.Fatalf("Error() = %q", got)
	}
	if remoteOperation(nil) != "remote command" || remoteOperation([]string{""}) != "remote command" {
		t.Fatal("empty args should describe a generic remote command")
	}
	if remoteOperation([]string{"fetch", "--prune"}) != "fetch" {
		t.Fatal("first arg should name the operation")
	}
}

func TestCatFileBatchCheckAllObjectsWiresPipe(t *testing.T) {
	var left, right run.Command
	c := New(fakeRunner{pipe: func(ctx context.Context, dir string, env []string, l run.Command, r run.Command, consume func(io.Reader) error) error {
		left, right = l, r
		return consume(strings.NewReader("123 abc file\n"))
	}})
	var got string
	err := c.CatFileBatchCheckAllObjects(context.Background(), "/r", func(reader io.Reader) error {
		data, err := io.ReadAll(reader)
		got = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if left.Name != "git" || strings.Join(left.Args, " ") != "rev-list --objects --all" {
		t.Fatalf("left command = %+v", left)
	}
	if right.Name != "git" || len(right.Args) == 0 || right.Args[0] != "cat-file" {
		t.Fatalf("right command = %+v", right)
	}
	if got != "123 abc file\n" {
		t.Fatalf("consumed = %q", got)
	}
}
