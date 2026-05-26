package git

import (
	"context"
	"errors"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func TestFilterRepoCommandPrefersStandaloneExecutable(t *testing.T) {
	if _, err := run.LookPath("git-filter-repo"); err == nil {
		t.Skip("real git-filter-repo is installed")
	}
	restore := run.SetCommandFunc(func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) == 2 && args[0] == "filter-repo" && args[1] == "--version" {
			return "git-filter-repo 2.0", "", nil
		}
		return "", "", errors.New("unexpected command")
	})
	defer restore()
	cmd, ok := FilterRepoCommand(context.Background())
	if !ok {
		t.Fatal("expected git filter-repo fallback")
	}
	if len(cmd) != 2 || cmd[0] != "git" || cmd[1] != "filter-repo" {
		t.Fatalf("cmd = %#v", cmd)
	}
}
