package run

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
)

type fakeRunner struct {
	run      CommandFunc
	lookPath func(string) (string, error)
	pipe     func(context.Context, string, []string, Command, Command, func(io.Reader) error) error
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

func (f fakeRunner) Pipe(ctx context.Context, dir string, env []string, left Command, right Command, consume func(io.Reader) error) error {
	if f.pipe == nil {
		return errors.New("unexpected pipe")
	}
	return f.pipe(ctx, dir, env, left, right, consume)
}

func TestRunnerInjectionAndRouting(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if dir == "dir1" && name == "test-cmd" && len(args) == 1 && args[0] == "arg1" {
			return "out1", "err1", nil
		}
		return "", "", errors.New("unexpected name/args")
	}}

	stdout, err := Stdout(context.Background(), runner, "dir1", nil, "test-cmd", "arg1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "out1" {
		t.Errorf("expected stdout 'out1', got %q", stdout)
	}

	output, err := Capture(context.Background(), runner, "dir1", nil, "test-cmd", "arg1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "out1err1" {
		t.Errorf("expected capture output 'out1err1', got %q", output)
	}
}

func TestWithStdinAndGetStdin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if input := GetStdin(ctx); input != "" {
		t.Errorf("expected empty stdin, got %q", input)
	}

	ctx = WithStdin(ctx, "hello-world")
	if input := GetStdin(ctx); input != "hello-world" {
		t.Errorf("expected 'hello-world', got %q", input)
	}
}

func TestInjectedRunnersAreParallelSafe(t *testing.T) {
	t.Parallel()
	for i := 0; i < 20; i++ {
		t.Run("runner", func(t *testing.T) {
			t.Parallel()
			runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
				return "ok", "", nil
			}}
			if out, err := Capture(context.Background(), runner, "", nil, "cmd"); err != nil || out != "ok" {
				t.Fatalf("Capture = %q, %v", out, err)
			}
		})
	}
}

func TestStreamStdoutFallsBackToBufferedRunner(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		return "streamed output", "", nil
	}}
	var output strings.Builder
	err := StreamStdout(context.Background(), runner, "", nil, "cmd", nil, func(reader io.Reader) error {
		_, err := io.Copy(&output, reader)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "streamed output" {
		t.Fatalf("output = %q", output.String())
	}
}
