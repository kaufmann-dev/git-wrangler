package githubcli

import (
	"context"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

type Client struct {
	runner run.Runner
}

func New(runner run.Runner) Client {
	if runner == nil {
		runner = run.New()
	}
	return Client{runner: runner}
}

func (c Client) Capture(ctx context.Context, dir string, args ...string) (string, error) {
	return run.Capture(ctx, c.runner, dir, nil, "gh", args...)
}

func (c Client) CaptureEnv(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	return run.Capture(ctx, c.runner, dir, env, "gh", args...)
}

func (c Client) Stdout(ctx context.Context, dir string, args ...string) (string, error) {
	return run.Stdout(ctx, c.runner, dir, nil, "gh", args...)
}

func (c Client) StdoutEnv(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	return run.Stdout(ctx, c.runner, dir, env, "gh", args...)
}

func (c Client) Installed() bool {
	_, err := c.runner.LookPath("gh")
	return err == nil
}

func Env(token, host string) []string {
	if token == "" {
		return nil
	}
	env := []string{"GH_TOKEN=" + token}
	if host != "" {
		env = append(env, "GH_HOST="+host)
	}
	return env
}

func RepoListArgs(user, visibility, limit string) []string {
	args := []string{"repo", "list", user, "--limit", limit}
	if visibility == "public" || visibility == "private" {
		args = []string{"repo", "list", user, "--visibility", visibility, "--limit", limit}
	}
	return args
}
