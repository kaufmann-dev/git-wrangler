package run

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestSetCommandFuncAndRouting(t *testing.T) {
	called := false
	restore := SetCommandFunc(func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		called = true
		if name == "test-cmd" && len(args) == 1 && args[0] == "arg1" {
			return "out1", "err1", nil
		}
		return "", "", errors.New("unexpected name/args")
	})
	defer restore()

	stdout, err := Stdout(context.Background(), "dir1", nil, "test-cmd", "arg1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "out1" {
		t.Errorf("expected stdout 'out1', got %q", stdout)
	}

	output, err := Capture(context.Background(), "dir1", nil, "test-cmd", "arg1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "out1err1" {
		t.Errorf("expected capture output 'out1err1', got %q", output)
	}

	if !called {
		t.Error("expected custom commandFunc to be called")
	}
}

func TestWithStdinAndGetStdin(t *testing.T) {
	ctx := context.Background()
	if input := GetStdin(ctx); input != "" {
		t.Errorf("expected empty stdin, got %q", input)
	}

	ctx = WithStdin(ctx, "hello-world")
	if input := GetStdin(ctx); input != "hello-world" {
		t.Errorf("expected 'hello-world', got %q", input)
	}
}

func TestSetCommandFuncConcurrentUse(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			restore := SetCommandFunc(func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
				return "ok", "", nil
			})
			defer restore()
			_, _ = Capture(context.Background(), "", nil, "cmd")
		}()
	}
	wg.Wait()
}
