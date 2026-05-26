package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
	"github.com/kaufmann-dev/git-wrangler/internal/repos"
	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func (a *app) status(stream io.Writer, color, symbol string, parts ...string) {
	message := ""
	if len(parts) == 1 {
		message = parts[0]
	} else if len(parts) >= 2 {
		message = parts[0] + ": " + parts[1]
	}
	fmt.Fprintf(stream, "%s%s%s %s\n", color, symbol, a.ui.Reset, message)
}

func (a *app) ok(parts ...string) { a.status(a.stdout, a.ui.Green, a.ui.OKSymbol, parts...) }

func (a *app) warn(parts ...string) { a.status(a.stdout, a.ui.Yellow, a.ui.WarnSymbol, parts...) }

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
	if _, err := run.LookPath(name); err != nil {
		a.errorf("'%s' is required for %s. Install it and make sure it is on PATH.", name, context)
		return false
	}
	return true
}

func requireGit(a *app, context string) bool {
	return requireCommand(a, "git", context)
}

func filterRepoCommand(a *app, commandContext string) ([]string, bool) {
	if cmd, ok := git.FilterRepoCommand(context.Background()); ok {
		return cmd, true
	}
	a.errorf("'git-filter-repo' or 'git filter-repo' is required for %s.", commandContext)
	return nil, false
}

func runCapture(dir string, env []string, name string, args ...string) (string, error) {
	return run.Capture(context.Background(), dir, env, name, args...)
}

func runStdout(dir string, env []string, name string, args ...string) (string, error) {
	return run.Stdout(context.Background(), dir, env, name, args...)
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

func confirm(a *app, question string) bool {
	var input io.Reader = a.stdin
	var output io.Writer = a.stdout

	if a.stdin == os.Stdin && a.stdout == os.Stdout {
		if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
			defer tty.Close()
			input = tty
			output = tty
		}
	}

	fmt.Fprintf(output, "%s [y/N] ", question)
	reader := bufio.NewReader(input)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimRight(answer, "\r\n")
	return answer == "y" || answer == "Y"
}

func promptRead(a *app, prompt string) (string, error) {
	var input io.Reader = a.stdin
	var output io.Writer = a.stdout

	if a.stdin == os.Stdin && a.stdout == os.Stdout {
		if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
			defer tty.Close()
			fmt.Fprint(tty, prompt)
			reader := bufio.NewReader(tty)
			answer, err := reader.ReadString('\n')
			return strings.TrimRight(answer, "\r\n"), err
		}
	}

	fmt.Fprint(output, prompt)
	reader := bufio.NewReader(input)
	answer, err := reader.ReadString('\n')
	return strings.TrimRight(answer, "\r\n"), err
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

func mustStdout(dir, name string, args ...string) string {
	out, _ := runStdout(dir, nil, name, args...)
	return out
}
