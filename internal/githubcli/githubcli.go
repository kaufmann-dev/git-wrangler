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

func (c Client) ValidateAuth(ctx context.Context, env []string) error {
	_, err := c.StdoutEnv(ctx, "", env, "api", "user", "-q", ".login")
	return err
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

// UnauthenticatedEnv prevents inherited token variables from turning public
// operations into authenticated requests.
func UnauthenticatedEnv() []string {
	return []string{
		"GH_TOKEN=",
		"GITHUB_TOKEN=",
		"GH_ENTERPRISE_TOKEN=",
		"GITHUB_ENTERPRISE_TOKEN=",
	}
}

func RepoListArgs(user, visibility, limit string) []string {
	args := []string{"repo", "list", user, "--limit", limit}
	if visibility == "public" || visibility == "private" {
		args = []string{"repo", "list", user, "--visibility", visibility, "--limit", limit}
	}
	return args
}
