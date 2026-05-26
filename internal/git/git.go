package git

import (
	"context"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func Capture(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	return run.Capture(ctx, dir, env, "git", args...)
}

func Stdout(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	return run.Stdout(ctx, dir, env, "git", args...)
}

func Installed() bool {
	_, err := run.LookPath("git")
	return err == nil
}

func FilterRepoCommand(ctx context.Context) ([]string, bool) {
	if path, err := run.LookPath("git-filter-repo"); err == nil {
		return []string{path}, true
	}
	if out, err := Capture(ctx, "", nil, "filter-repo", "--version"); err == nil || out != "" {
		return []string{"git", "filter-repo"}, true
	}
	return nil, false
}

func CatFileBatchCheck(ctx context.Context, dir, input string) string {
	ctx = run.WithStdin(ctx, input)
	out, _ := Capture(ctx, dir, nil, "cat-file", "--batch-check=%(objectsize) %(objectname) %(rest)")
	return out
}
