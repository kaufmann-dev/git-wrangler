package cli

import (
	"context"
	"errors"
	"io"
	"os/exec"

	"github.com/kaufmann-dev/git-wrangler/internal/auth"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
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

type fakeCredentialStore struct {
	values map[string]string
	err    error
}

func (s *fakeCredentialStore) Get(account string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.values == nil {
		return "", credentials.ErrNotFound
	}
	value, ok := s.values[account]
	if !ok {
		return "", credentials.ErrNotFound
	}
	return value, nil
}

func (s *fakeCredentialStore) Set(account, secret string) error {
	if s.err != nil {
		return s.err
	}
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[account] = secret
	return nil
}

func (s *fakeCredentialStore) Delete(account string) error {
	if s.err != nil {
		return s.err
	}
	if s.values != nil {
		delete(s.values, account)
	}
	return nil
}

type fakeGitHubAuth struct {
	result     auth.GitHubResult
	err        error
	waitEvents []auth.WaitEvent
}

func makeInteractive(a *app) {
	a.prompts.interactive = func() bool { return true }
}

func executeInteractive(t interface {
	Helper()
	Fatalf(string, ...any)
}, ctx context.Context, runner run.Runner, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	t.Helper()
	a := newApp(ctx, runner, stdin, stdout, stderr)
	makeInteractive(a)
	root := newRootCommand(a)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	return root.Execute()
}

func (f fakeGitHubAuth) AuthenticateGitHub(ctx context.Context, host string, stdin io.Reader, stderr io.Writer, onWait func(auth.WaitEvent)) (auth.GitHubResult, error) {
	if onWait != nil {
		for _, event := range f.waitEvents {
			onWait(event)
		}
	}
	return f.result, f.err
}
