package git

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

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

func CatFileBatchCheck(dir, input string) string {
	cmd := exec.Command("git", "cat-file", "--batch-check=%(objectsize) %(objectname) %(rest)")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()
	return out.String()
}
