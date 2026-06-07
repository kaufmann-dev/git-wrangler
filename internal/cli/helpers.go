package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/repos"
	"github.com/kaufmann-dev/git-wrangler/internal/run"
	"github.com/spf13/cobra"
)

func (a *app) status(stream io.Writer, color, symbol string, parts ...string) {
	state := statusInfo
	switch symbol {
	case a.ui.OKSymbol:
		state = statusOK
	case a.ui.WarnSymbol:
		state = statusWarn
	case a.ui.ErrSymbol:
		state = statusError
	case a.ui.SkipSymbol:
		state = statusSkip
	}
	_ = color
	if len(parts) == 1 {
		renderStatusLine(a, stream, state, parts[0], "")
		return
	}
	if len(parts) >= 2 {
		renderStatusLine(a, stream, state, parts[0], parts[1])
	}
}

func (a *app) ok(parts ...string) { a.status(a.stdout, a.ui.Green, a.ui.OKSymbol, parts...) }

func (a *app) warn(parts ...string) { a.status(a.stderr, a.ui.Yellow, a.ui.WarnSymbol, parts...) }

func (a *app) info(parts ...string) { a.status(a.stdout, a.ui.Cyan, a.ui.InfoSymbol, parts...) }

func (a *app) step(parts ...string) {
	a.status(a.stdout, a.ui.Bold+a.ui.Cyan, a.ui.StepSymbol, parts...)
}

func (a *app) skip(parts ...string) { a.status(a.stdout, a.ui.Yellow, a.ui.SkipSymbol, parts...) }

func (a *app) error(parts ...string) { a.status(a.stderr, a.ui.Red, a.ui.ErrSymbol, parts...) }

func (a *app) errorf(format string, args ...any) {
	a.error(fmt.Sprintf(format, args...))
}

func (a *app) plainErrorf(format string, args ...any) {
	fmt.Fprintf(a.stderr, "%sError: %s%s\n", a.ui.Red, fmt.Sprintf(format, args...), a.ui.Reset)
}

func requireValue(a *app, option string, args []string) (string, bool) {
	if len(args) < 2 || args[1] == "" || strings.HasPrefix(args[1], "--") {
		a.errorf("%s requires a value.", option)
		return "", false
	}
	return args[1], true
}

func requireCommand(a *app, name, context string) bool {
	if _, err := a.runner.LookPath(name); err != nil {
		a.errorf("'%s' is required for %s. Install it and make sure it is on PATH.", name, context)
		return false
	}
	return true
}

func requireGit(a *app, context string) bool {
	return requireCommand(a, "git", context)
}

func filterRepoCommand(a *app, commandContext string) ([]string, bool) {
	if cmd, ok := a.git.FilterRepoCommand(a.ctx); ok {
		return cmd, true
	}
	a.errorf("'git-filter-repo' or 'git filter-repo' is required for %s.", commandContext)
	return nil, false
}

func (a *app) runCapture(dir string, env []string, name string, args ...string) (string, error) {
	return run.Capture(a.ctx, a.runner, dir, env, name, args...)
}

func (a *app) runStdout(dir string, env []string, name string, args ...string) (string, error) {
	return run.Stdout(a.ctx, a.runner, dir, env, name, args...)
}

func findGitRepositories(root string) ([]repo, error) {
	discovered, err := repos.Discover(root)
	if err != nil {
		return nil, err
	}
	result := make([]repo, 0, len(discovered))
	for _, r := range discovered {
		result = append(result, repo{gitDir: r.GitDir, dir: r.Dir, display: r.Display})
	}
	return result, nil
}

func resolveRepositoryTargets(repoName string) ([]repo, error) {
	if repoName == "" {
		return findGitRepositories(".")
	}
	r, err := repos.ResolveExact(repoName)
	if err != nil {
		return nil, err
	}
	return []repo{{gitDir: r.GitDir, dir: r.Dir, display: r.Display}}, nil
}

func commandRepositoryTargets(cmd *cobra.Command) ([]repo, error) {
	repoName := ""
	if cmd.Flags().Lookup("repo") != nil {
		repoName, _ = cmd.Flags().GetString("repo")
	}
	return resolveRepositoryTargets(repoName)
}

func repoDirFromGitDir(gitDir string) string {
	return repos.DirFromGitDir(gitDir)
}

func repoDisplayName(repoDir string) string {
	return repos.DisplayName(repoDir)
}

func noRepos(a *app) int {
	a.warn("No Git repositories found in the specified directory.")
	return 0
}

func confirm(a *app, question string) confirmationResult {
	result := a.prompts.confirm(question)
	if result == confirmationUnavailable {
		a.plainErrorf("confirmation requires an interactive terminal; pass --yes to confirm noninteractively.")
		a.promptFailed = true
	}
	return result
}

func confirmOrSkip(a *app, yes bool, question string) confirmationResult {
	if yes {
		return confirmationAccepted
	}
	return confirm(a, question)
}

func promptRead(a *app, prompt string) (string, error) {
	return a.prompts.read(prompt)
}

func promptSecret(a *app, prompt string) (string, error) {
	return a.prompts.secret(prompt)
}

func interactive(a *app) bool {
	return a.prompts.available()
}

func requireInteractive(a *app, command string) bool {
	if interactive(a) {
		return true
	}
	a.plainErrorf("%s requires an interactive terminal for stdin and stderr.", command)
	return false
}

func yesFlag(cmd *cobra.Command) bool {
	yes, _ := cmd.Flags().GetBool("yes")
	return yes
}

func jsonFlagValue(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Flags().Lookup("json") == nil {
		return false
	}
	value, _ := cmd.Flags().GetBool("json")
	return value
}

func requiredStringFlag(a *app, cmd *cobra.Command, name, prompt string) (string, bool) {
	value, _ := cmd.Flags().GetString(name)
	if value != "" {
		return value, true
	}
	if !interactive(a) {
		a.plainErrorf("--%s is required.", name)
		return "", false
	}
	answer, err := promptRead(a, prompt)
	if errors.Is(err, errPromptCancelled) {
		return "", false
	}
	if err != nil || answer == "" {
		a.plainErrorf("--%s is required.", name)
		return "", false
	}
	return answer, true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func prefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func lineCount(s string) int {
	count := 0
	for _, line := range splitLines(s) {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func sortedUnique(lines []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	sort.Strings(out)
	return out
}

func originURL(a *app, dir string) string {
	return a.git.RemoteURL(a.ctx, dir, "origin")
}

func restoreOrigin(a *app, dir, remoteURL string) error {
	return a.git.RestoreRemote(a.ctx, dir, "origin", remoteURL)
}
