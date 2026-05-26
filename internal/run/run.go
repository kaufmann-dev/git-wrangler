package run

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CommandFunc func(ctx context.Context, dir string, env []string, name string, args ...string) (stdout string, stderr string, err error)

const DefaultTimeout = 5 * time.Minute

type Command struct {
	Name string
	Args []string
}

type Runner interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) (stdout string, stderr string, err error)
	LookPath(name string) (string, error)
	Pipe(ctx context.Context, dir string, env []string, left Command, right Command, consume func(io.Reader) error) error
}

type RealRunner struct{}

func New() Runner {
	return RealRunner{}
}

func LookPath(name string) (string, error) {
	return RealRunner{}.LookPath(name)
}

func Capture(ctx context.Context, r Runner, dir string, env []string, name string, args ...string) (string, error) {
	if r == nil {
		r = RealRunner{}
	}
	stdout, stderr, err := r.Run(ctx, dir, env, name, args...)
	output := stdout + stderr
	if err != nil {
		return output, err
	}
	return output, nil
}

func Stdout(ctx context.Context, r Runner, dir string, env []string, name string, args ...string) (string, error) {
	if r == nil {
		r = RealRunner{}
	}
	stdout, stderr, err := r.Run(ctx, dir, env, name, args...)
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

func (RealRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func (RealRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
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

func (r RealRunner) Pipe(ctx context.Context, dir string, env []string, left Command, right Command, consume func(io.Reader) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	leftCmd := exec.CommandContext(ctx, left.Name, left.Args...)
	rightCmd := exec.CommandContext(ctx, right.Name, right.Args...)
	if dir != "" {
		leftCmd.Dir = dir
		rightCmd.Dir = dir
	}
	if env != nil {
		fullEnv := append(os.Environ(), env...)
		leftCmd.Env = fullEnv
		rightCmd.Env = fullEnv
	}
	pipe, err := leftCmd.StdoutPipe()
	if err != nil {
		return err
	}
	rightCmd.Stdin = pipe
	var leftErr, rightErr bytes.Buffer
	leftCmd.Stderr = &leftErr
	rightCmd.Stderr = &rightErr
	output, err := rightCmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := rightCmd.Start(); err != nil {
		return err
	}
	if err := leftCmd.Start(); err != nil {
		if rightCmd.Process != nil {
			_ = rightCmd.Process.Kill()
		}
		_ = rightCmd.Wait()
		return err
	}
	if consume == nil {
		consume = func(output io.Reader) error {
			_, err := io.Copy(io.Discard, output)
			return err
		}
	}
	consumeErr := consume(output)
	leftRunErr := leftCmd.Wait()
	rightRunErr := rightCmd.Wait()
	if consumeErr != nil {
		return consumeErr
	}
	if leftRunErr != nil {
		if strings.TrimSpace(leftErr.String()) != "" {
			return errors.New(strings.TrimSpace(leftErr.String()))
		}
		return leftRunErr
	}
	if rightRunErr != nil {
		if strings.TrimSpace(rightErr.String()) != "" {
			return errors.New(strings.TrimSpace(rightErr.String()))
		}
		return rightRunErr
	}
	return nil
}
