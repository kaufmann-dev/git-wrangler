package run

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

type CommandFunc func(ctx context.Context, dir string, env []string, name string, args ...string) (stdout string, stderr string, err error)

var commandFunc CommandFunc = realCommand

func SetCommandFunc(fn CommandFunc) func() {
	previous := commandFunc
	if fn == nil {
		commandFunc = realCommand
	} else {
		commandFunc = fn
	}
	return func() { commandFunc = previous }
}

func LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func Capture(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	stdout, stderr, err := commandFunc(ctx, dir, env, name, args...)
	output := stdout + stderr
	if err != nil {
		return output, err
	}
	return output, nil
}

func Stdout(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	stdout, stderr, err := commandFunc(ctx, dir, env, name, args...)
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return stdout, errors.New(strings.TrimSpace(stderr))
		}
		return stdout, err
	}
	return stdout, nil
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
