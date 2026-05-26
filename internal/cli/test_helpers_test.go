package cli

import (
	"context"
	"errors"
	"io"
	"os/exec"

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
