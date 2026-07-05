package run

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
)

func TestRealRunnerInteractivePipesStdinToStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX cat")
	}
	var out, errBuf bytes.Buffer
	err := RealRunner{}.Interactive(context.Background(), "", nil, "cat", nil, strings.NewReader("hello world"), &out, &errBuf)
	if err != nil {
		t.Fatalf("interactive cat: %v (stderr %q)", err, errBuf.String())
	}
	if out.String() != "hello world" {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestRealRunnerStreamStreamsStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell")
	}
	var got strings.Builder
	err := RealRunner{}.Stream(context.Background(), "", nil, "sh", []string{"-c", "printf 'a\\nb\\nc\\n'"}, func(r io.Reader) error {
		_, err := io.Copy(&got, r)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "a\nb\nc\n" {
		t.Fatalf("stream = %q", got.String())
	}
}

func TestRealRunnerPipeConnectsLeftIntoRight(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell")
	}
	var got strings.Builder
	err := RealRunner{}.Pipe(context.Background(), "", nil,
		Command{Name: "sh", Args: []string{"-c", "printf 'a\\nb\\nc\\n'"}},
		Command{Name: "wc", Args: []string{"-l"}},
		func(r io.Reader) error {
			_, err := io.Copy(&got, r)
			return err
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.String()) != "3" {
		t.Fatalf("piped wc -l = %q, want 3", got.String())
	}
}

func TestRealRunnerLookPath(t *testing.T) {
	if _, err := (RealRunner{}).LookPath("sh"); err != nil {
		t.Fatalf("expected to resolve sh: %v", err)
	}
	if _, err := (RealRunner{}).LookPath("git-wrangler-definitely-missing-binary"); err == nil {
		t.Fatal("expected error for a missing binary")
	}
}

func TestNewReturnsRealRunner(t *testing.T) {
	if _, ok := New().(RealRunner); !ok {
		t.Fatal("New should return a RealRunner")
	}
}

func TestInteractiveDispatchesToInteractiveRunner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX cat")
	}
	var out bytes.Buffer
	// RealRunner implements InteractiveRunner, so the editor path runs the real command.
	err := Interactive(context.Background(), RealRunner{}, "", nil, "cat", nil, strings.NewReader("x"), &out, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "x" {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestInteractiveFallsBackToRunForPlainRunner(t *testing.T) {
	var gotName string
	plain := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		gotName = name
		return "buffered", "", nil
	}}
	if err := Interactive(context.Background(), plain, "", nil, "editor", []string{"file"}, strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if gotName != "editor" {
		t.Fatalf("fallback should invoke Run with the editor, got %q", gotName)
	}

	failing := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		return "", "editor exploded", errors.New("exit 1")
	}}
	err := Interactive(context.Background(), failing, "", nil, "editor", nil, strings.NewReader(""), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "editor exploded") {
		t.Fatalf("fallback should surface stderr detail, got %v", err)
	}
}
