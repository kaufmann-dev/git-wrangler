package githubcli

import (
	"context"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func Capture(ctx context.Context, dir string, args ...string) (string, error) {
	return run.Capture(ctx, dir, nil, "gh", args...)
}

func Stdout(ctx context.Context, dir string, args ...string) (string, error) {
	return run.Stdout(ctx, dir, nil, "gh", args...)
}

func Installed() bool {
	_, err := run.LookPath("gh")
	return err == nil
}

func RepoListArgs(user, visibility, limit string) []string {
	args := []string{"repo", "list", user, "--limit", limit}
	if visibility == "public" || visibility == "private" {
		args = []string{"repo", "list", user, "--visibility", visibility, "--limit", limit}
	}
	return args
}
