package run

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type CommandFunc func(ctx context.Context, dir string, env []string, name string, args ...string) (stdout string, stderr string, err error)

const DefaultTimeout = 5 * time.Minute

var (
	commandMu   sync.RWMutex
	commandFunc CommandFunc = realCommand
)

func SetCommandFunc(fn CommandFunc) func() {
	commandMu.Lock()
	previous := commandFunc
	if fn == nil {
		commandFunc = realCommand
	} else {
		commandFunc = fn
	}
	commandMu.Unlock()
	return func() {
		commandMu.Lock()
		commandFunc = previous
		commandMu.Unlock()
	}
}

func LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func Capture(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	fn := currentCommandFunc()
	stdout, stderr, err := fn(ctx, dir, env, name, args...)
	output := stdout + stderr
	if err != nil {
		return output, err
	}
	return output, nil
}

func Stdout(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	fn := currentCommandFunc()
	stdout, stderr, err := fn(ctx, dir, env, name, args...)
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return stdout, errors.New(strings.TrimSpace(stderr))
		}
		return stdout, err
	}
	return stdout, nil
}

func currentCommandFunc() CommandFunc {
	commandMu.RLock()
	fn := commandFunc
	commandMu.RUnlock()
	return fn
}

type stdinKey struct{}

func WithStdin(ctx context.Context, stdin string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, stdinKey{}, stdin)
}

func GetStdin(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(stdinKey{}); v != nil {
		return v.(string)
	}
	return ""
}

func realCommand(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	if stdin := GetStdin(ctx); stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
