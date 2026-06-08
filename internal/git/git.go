package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

const RemoteTimeout = 30 * time.Second

type Client struct {
	runner run.Runner
}

type RemoteTimeoutError struct {
	Operation string
	Timeout   time.Duration
}

func (e RemoteTimeoutError) Error() string {
	return fmt.Sprintf("git %s timed out after %s", e.Operation, e.Timeout)
}

func New(runner run.Runner) Client {
	if runner == nil {
		runner = run.New()
	}
	return Client{runner: runner}
}

func (c Client) IsZero() bool {
	return c.runner == nil
}

func (c Client) Capture(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	return run.Capture(ctx, c.runner, dir, env, "git", args...)
}

func (c Client) CaptureRemote(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	remoteCtx, cancel := context.WithTimeout(ctx, RemoteTimeout)
	defer cancel()
	out, err := c.Capture(remoteCtx, dir, env, args...)
	if err != nil && errors.Is(remoteCtx.Err(), context.DeadlineExceeded) {
		return out, RemoteTimeoutError{Operation: remoteOperation(args), Timeout: RemoteTimeout}
	}
	return out, err
}

func (c Client) Stdout(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	return run.Stdout(ctx, c.runner, dir, env, "git", args...)
}

func (c Client) StreamStdout(ctx context.Context, dir string, env []string, consume func(io.Reader) error, args ...string) error {
	return run.StreamStdout(ctx, c.runner, dir, env, "git", args, consume)
}

func (c Client) Installed() bool {
	_, err := c.runner.LookPath("git")
	return err == nil
}

func (c Client) FilterRepoCommand(ctx context.Context) ([]string, bool) {
	if path, err := c.runner.LookPath("git-filter-repo"); err == nil {
		return []string{path}, true
	}
	if _, err := c.Capture(ctx, "", nil, "filter-repo", "--version"); err == nil {
		return []string{"git", "filter-repo"}, true
	}
	return nil, false
}

func (c Client) CatFileBatchCheck(ctx context.Context, dir, input string) (string, error) {
	ctx = run.WithStdin(ctx, input)
	return c.Capture(ctx, dir, nil, "cat-file", "--batch-check=%(objectsize) %(objectname) %(rest)")
}

func (c Client) CatFileBatchCheckAllObjects(ctx context.Context, dir string, consume func(io.Reader) error) error {
	return c.runner.Pipe(ctx, dir, nil,
		run.Command{Name: "git", Args: []string{"rev-list", "--objects", "--all"}},
		run.Command{Name: "git", Args: []string{"cat-file", "--batch-check=%(objectsize) %(objectname) %(rest)"}},
		consume,
	)
}

func (c Client) StatusPorcelain(ctx context.Context, dir string) (string, error) {
	return c.Stdout(ctx, dir, nil, "status", "--porcelain")
}

func (c Client) StatusPorcelainBranch(ctx context.Context, dir string) (string, error) {
	return c.Stdout(ctx, dir, nil, "status", "--porcelain=v2", "--branch")
}

func (c Client) CurrentBranch(ctx context.Context, dir string) (string, error) {
	return c.Stdout(ctx, dir, nil, "rev-parse", "--abbrev-ref", "HEAD")
}

func (c Client) HasHead(ctx context.Context, dir string) bool {
	_, err := c.Capture(ctx, dir, nil, "rev-parse", "HEAD")
	return err == nil
}

func (c Client) VerifyRef(ctx context.Context, dir, ref string) bool {
	_, err := c.Capture(ctx, dir, nil, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}

func (c Client) RemoteURL(ctx context.Context, dir, name string) string {
	out, err := c.Stdout(ctx, dir, nil, "remote", "get-url", name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (c Client) RestoreRemote(ctx context.Context, dir, name, remoteURL string) error {
	if remoteURL == "" {
		return nil
	}
	if _, err := c.Capture(ctx, dir, nil, "remote", "get-url", name); err == nil {
		return nil
	}
	_, err := c.Capture(ctx, dir, nil, "remote", "add", name, remoteURL)
	return err
}

func remoteOperation(args []string) string {
	if len(args) == 0 || args[0] == "" {
		return "remote command"
	}
	return args[0]
}
