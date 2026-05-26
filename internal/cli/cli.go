package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/kaufmann-dev/git-wrangler/internal/ai"
	"github.com/kaufmann-dev/git-wrangler/internal/git"
	"github.com/kaufmann-dev/git-wrangler/internal/githubcli"
	"github.com/kaufmann-dev/git-wrangler/internal/repos"
	"github.com/kaufmann-dev/git-wrangler/internal/run"
	"github.com/kaufmann-dev/git-wrangler/internal/ui"
	"github.com/kaufmann-dev/git-wrangler/internal/version"
)

type app struct {
	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
	ui     ui.Theme
}

type repo struct {
	gitDir  string
	dir     string
	display string
}

type exitError struct {
	code int
}

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func Execute() error {
	return execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

func ExecuteWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return execute(args, stdin, stdout, stderr)
}

func newApp(stdin io.Reader, stdout, stderr io.Writer) *app {
	return &app{
		stdout: stdout,
		stderr: stderr,
		stdin:  stdin,
		ui:     ui.New(stdout),
	}
}

func execute(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	a := newApp(stdin, stdout, stderr)
	root := newRootCommand(a)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		if _, ok := err.(exitError); !ok {
			fmt.Fprintf(stderr, "Error: %s\n", err)
		}
		return err
	}
	return nil
}

func newRootCommand(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:           "git-wrangler",
		Short:         "Orchestrate Git operations across many repositories.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Full(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.CompletionOptions.DisableDescriptions = false
	root.AddGroup(
		&cobra.Group{ID: "remote", Title: "Remote Operations:"},
		&cobra.Group{ID: "local", Title: "Local Operations:"},
		&cobra.Group{ID: "history", Title: "History Rewriting:"},
		&cobra.Group{ID: "utility", Title: "Utility:"},
	)
	root.SetHelpCommandGroupID("utility")
	root.SetCompletionCommandGroupID("utility")

	root.AddCommand(
		command(a, "clone", "Clone multiple GitHub repositories for a user.", "remote", runClone, flags{
			stringFlag("visibility", "", "Repository visibility: all, public, or private."),
			stringFlag("user", "", "GitHub user or organization to clone from."),
			stringFlag("limit", "", "Maximum repositories to list."),
			stringFlag("into", "", "Directory to clone repositories into."),
		}),
		command(a, "pull", "Pull the latest changes for every discovered repository.", "remote", runPull, flags{
			boolFlag("rebase", "Rebase local commits while pulling."),
			boolFlag("force", "Pass --force to git pull."),
		}),
		command(a, "push", "Push local commits to origin HEAD.", "remote", runPush, flags{
			boolFlag("force", "Use --force-with-lease."),
			boolFlag("force-unsafe", "Use raw --force after confirmation."),
		}),
		command(a, "rename-repo", "Rename GitHub repositories with gh.", "remote", runRenameRepo, flags{
			boolFlag("description", "Prompt for repository description updates."),
		}),
		command(a, "commit", "Stage all changes and create a commit in every repository.", "local", runCommit, flags{
			stringFlag("message", "", "Commit message."),
		}),
		command(a, "fix-gitignore", "Add missing common generated-file patterns to .gitignore.", "local", runFixGitignore, nil),
		command(a, "license", "Add or replace MIT LICENSE files.", "local", runLicense, flags{
			stringFlag("repo", "", "Repository directory to target."),
			stringFlag("name", "", "Copyright holder name."),
			boolFlag("overwrite", "Replace an existing LICENSE file."),
		}),
		command(a, "rename-branch", "Rename a branch across repositories.", "local", runRenameBranch, flags{
			stringFlag("oldbranch", "", "Existing branch name."),
			stringFlag("newbranch", "", "New branch name."),
		}),
		command(a, "reset", "Reset current branches to their origin counterparts.", "local", runReset, flags{
			boolFlag("confirm", "Skip interactive reset confirmation."),
		}),
		command(a, "review", "Review unpushed changes across repositories.", "local", runReview, nil),
		command(a, "untrack", "Stop tracking files already covered by .gitignore.", "local", runUntrack, nil),
		command(a, "remove-secrets", "Purge sensitive files from Git history.", "history", runRemoveSecrets, flags{
			boolFlag("confirm", "Allow history rewriting."),
		}),
		command(a, "rewrite-authors", "Rewrite author and committer identity.", "history", runRewriteAuthors, flags{
			stringFlag("name", "", "New author and committer name."),
			stringFlag("email", "", "New author and committer email."),
			stringFlag("repo", "", "Repository directory to target."),
			boolFlag("force", "Pass --force to git-filter-repo."),
		}),
		command(a, "rewrite-commits", "Rewrite commit messages to Conventional Commits.", "history", runRewriteCommits, nil),
		command(a, "rewrite-commits-ai", "Generate Conventional Commit messages with an OpenAI-compatible endpoint.", "history", runRewriteCommitsAI, flags{
			stringFlag("base-url", "", "OpenAI-compatible API base URL."),
			stringFlag("model", "", "Model name."),
			stringFlag("api-key", "", "API key for this run."),
			stringFlag("api-key-env", "", "Environment variable containing the API key."),
			stringFlag("batch-size", "", "Commits per API request."),
			stringFlag("max-chars-per-commit", "", "Maximum redacted context characters per commit."),
			stringFlag("timeout", "", "API timeout in seconds."),
			boolFlag("skip-conventional", "Skip commits that already use Conventional Commits."),
		}),
		command(a, "rewrite-dates", "Redistribute commit timestamps.", "history", runRewriteDates, flags{
			stringFlag("start-date", "", "Earliest date in YYYY-MM-DD format."),
			stringFlag("end-date", "", "Latest date in YYYY-MM-DD format."),
			boolFlag("confirm", "Skip interactive rewrite confirmation."),
		}),
		command(a, "doctor", "Check runtime dependencies.", "utility", runDoctor, flags{
			boolFlag("summary", "Only print dependency status."),
		}),
		command(a, "info", "Show detailed repository information.", "utility", runInfo, flags{
			stringFlag("repo", "", "Repository directory to target."),
		}),
		command(a, "status", "Show clean, dirty, and tracking state.", "utility", runStatus, nil),
		versionCommand(a),
	)
	return root
}

type flagSpec struct {
	name        string
	value       string
	description string
	boolean     bool
}

type flags []flagSpec

func stringFlag(name, value, description string) flagSpec {
	return flagSpec{name: name, value: value, description: description}
}

func boolFlag(name, description string) flagSpec {
	return flagSpec{name: name, description: description, boolean: true}
}

func command(a *app, use, short, group string, runFn func(*app, []string) int, specs flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     use,
		Short:   short,
		GroupID: group,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := runFn(a, legacyArgs(cmd.Flags(), args)); code != 0 {
				return exitError{code: code}
			}
			return nil
		},
	}
	for _, spec := range specs {
		if spec.boolean {
			cmd.Flags().Bool(spec.name, false, spec.description)
		} else {
			cmd.Flags().String(spec.name, spec.value, spec.description)
		}
	}
	return cmd
}

func legacyArgs(flagSet *pflag.FlagSet, args []string) []string {
	result := make([]string, 0, len(args))
	flagSet.Visit(func(f *pflag.Flag) {
		result = append(result, "--"+f.Name)
		if f.Value.Type() != "bool" {
			result = append(result, f.Value.String())
		}
	})
	return append(result, args...)
}

func versionCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print version metadata.",
		GroupID: "utility",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(a.stdout, version.Full())
		},
	}
}

func (a *app) status(stream io.Writer, color, symbol string, parts ...string) {
	message := ""
	if len(parts) == 1 {
		message = parts[0]
	} else if len(parts) >= 2 {
		message = parts[0] + ": " + parts[1]
	}
	fmt.Fprintf(stream, "%s%s%s %s\n", color, symbol, a.ui.Reset, message)
}

func (a *app) ok(parts ...string)   { a.status(a.stdout, a.ui.Green, a.ui.OKSymbol, parts...) }
func (a *app) warn(parts ...string) { a.status(a.stdout, a.ui.Yellow, a.ui.WarnSymbol, parts...) }
func (a *app) info(parts ...string) { a.status(a.stdout, a.ui.Cyan, a.ui.InfoSymbol, parts...) }
func (a *app) step(parts ...string) {
	a.status(a.stdout, a.ui.Bold+a.ui.Cyan, a.ui.StepSymbol, parts...)
}
func (a *app) skip(parts ...string)  { a.status(a.stdout, a.ui.Yellow, a.ui.SkipSymbol, parts...) }
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
		a.errorf("'%s' is required for %s. Run 'git-wrangler doctor' for more information.", name, context)
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
	a.errorf("'git-filter-repo' is required for %s. Run 'git-wrangler doctor' for more information.", commandContext)
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
	var input *os.File
	var output *os.File
	if _, stdinIsFile := a.stdin.(*os.File); stdinIsFile {
		if _, stdoutIsFile := a.stdout.(*os.File); stdoutIsFile {
			if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
				defer tty.Close()
				input = tty
				output = tty
			}
		}
	}
	if input == nil {
		if f, ok := a.stdin.(*os.File); ok {
			input = f
		}
	}
	if output == nil {
		if f, ok := a.stdout.(*os.File); ok {
			output = f
		}
	}

	if output != nil {
		fmt.Fprintf(output, "%s [y/N] ", question)
	} else {
		fmt.Fprintf(a.stdout, "%s [y/N] ", question)
	}

	reader := bufio.NewReader(a.stdin)
	if input != nil {
		reader = bufio.NewReader(input)
	}
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimRight(answer, "\r\n")
	return answer == "y" || answer == "Y"
}

func promptRead(prompt string) (string, error) {
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer tty.Close()
		fmt.Fprint(tty, prompt)
		reader := bufio.NewReader(tty)
		answer, err := reader.ReadString('\n')
		return strings.TrimRight(answer, "\r\n"), err
	}
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
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

func runStatus(a *app, args []string) int {
	if len(args) > 0 {
		a.errorf("Unknown option: %s", args[0])
		return 1
	}
	if !requireGit(a, "status") {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}

	fmt.Fprintf(a.stdout, "%-30s | %-5s | %s\n", "REPOSITORY", "STATE", "TRACKING")
	fmt.Fprintln(a.stdout, "-------------------------------+-------+------------------------")

	totalDirty := 0
	totalBehind := 0
	totalNoRemote := 0

	for _, r := range repos {
		row, err := statusRow(a, r)
		if err != nil {
			continue
		}
		fmt.Fprintf(a.stdout, "%-30s | %s | %s\n", row.name, row.state, row.tracking)
		totalDirty += row.dirty
		totalBehind += row.behind
		totalNoRemote += row.noRemote
	}

	fmt.Fprintln(a.stdout, "-------------------------------+-------+------------------------")
	fmt.Fprintf(a.stderr, "Summary: %s%d dirty%s, %s%d behind%s, %s%d no remote%s\n",
		a.ui.Yellow, totalDirty, a.ui.Reset,
		a.ui.Red, totalBehind, a.ui.Reset,
		a.ui.Muted, totalNoRemote, a.ui.Reset)
	return 0
}

type statusTableRow struct {
	name     string
	state    string
	tracking string
	dirty    int
	behind   int
	noRemote int
}

func statusRow(a *app, r repo) (statusTableRow, error) {
	out, _ := runStdout(r.dir, nil, "git", "status", "--porcelain=v2", "--branch")
	isDirty := false
	hasUpstream := false
	aheadCount := 0
	behindCount := 0
	for _, line := range splitLines(out) {
		switch {
		case strings.HasPrefix(line, "# branch.upstream "):
			hasUpstream = true
		case strings.HasPrefix(line, "# branch.ab "):
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				aheadCount, _ = strconv.Atoi(strings.TrimPrefix(fields[2], "+"))
				behindCount, _ = strconv.Atoi(strings.TrimPrefix(fields[3], "-"))
			}
		case strings.HasPrefix(line, "#"), line == "":
		default:
			isDirty = true
		}
	}

	name := r.display
	if len(name) > 30 {
		name = truncate(name, 30)
	}
	state := a.ui.Green + "clean" + a.ui.Reset
	dirty := 0
	if isDirty {
		state = a.ui.Yellow + "dirty" + a.ui.Reset
		dirty = 1
	}

	tracking := "up to date"
	behind := 0
	noRemote := 0
	if !hasUpstream {
		tracking = a.ui.Muted + "no remote" + a.ui.Reset
		noRemote = 1
	} else if aheadCount > 0 && behindCount > 0 {
		tracking = fmt.Sprintf("%sahead %d%s, %sbehind %d%s", a.ui.Cyan, aheadCount, a.ui.Reset, a.ui.Red, behindCount, a.ui.Reset)
		behind = 1
	} else if aheadCount > 0 {
		tracking = fmt.Sprintf("%sahead %d%s", a.ui.Cyan, aheadCount, a.ui.Reset)
	} else if behindCount > 0 {
		tracking = fmt.Sprintf("%sbehind %d%s", a.ui.Red, behindCount, a.ui.Reset)
		behind = 1
	}

	return statusTableRow{name: name, state: state, tracking: tracking, dirty: dirty, behind: behind, noRemote: noRemote}, nil
}

func runDoctor(a *app, args []string) int {
	summaryOnly := false
	for len(args) > 0 {
		switch args[0] {
		case "--summary":
			summaryOnly = true
			args = args[1:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}

	printDependencySummary(a)
	if summaryOnly {
		return 0
	}
	printInstallInstructions(a)
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Run 'gh auth login' before private or all GitHub repository operations.")
	return 0
}

func printDependencySummary(a *app) {
	fmt.Fprintf(a.stdout, "\n%sDependencies%s\n\n", a.ui.Bold, a.ui.Reset)
	printDependency(a, "git", "all commands")
	printDependency(a, "gh", "clone, rename-repo")
	printFilterRepoDependency(a)
}

func printDependency(a *app, name, requiredFor string) {
	if path, err := run.LookPath(name); err == nil {
		fmt.Fprintf(a.stdout, "  %sfound%s   %-16s %s\n", a.ui.Green, a.ui.Reset, name, path)
	} else {
		fmt.Fprintf(a.stdout, "  %smissing%s %-16s required for %s\n", a.ui.Red, a.ui.Reset, name, requiredFor)
	}
}

func printFilterRepoDependency(a *app) {
	if path, err := run.LookPath("git-filter-repo"); err == nil {
		fmt.Fprintf(a.stdout, "  %sfound%s   %-16s %s\n", a.ui.Green, a.ui.Reset, "git-filter-repo", path)
	} else if _, ok := filterRepoCommand(&app{stderr: io.Discard, ui: a.ui}, "doctor"); ok {
		fmt.Fprintf(a.stdout, "  %sfound%s   %-16s %s\n", a.ui.Green, a.ui.Reset, "git-filter-repo", "git filter-repo")
	} else {
		fmt.Fprintf(a.stdout, "  %smissing%s %-16s required for %s\n", a.ui.Red, a.ui.Reset, "git-filter-repo", "rewrite-authors, rewrite-commits, rewrite-commits-ai, rewrite-dates, remove-secrets")
	}
}

func installed(name string) bool {
	_, err := run.LookPath(name)
	return err == nil
}

func filterRepoInstalled() bool {
	if installed("git-filter-repo") {
		return true
	}
	_, err := runCapture("", nil, "git", "filter-repo", "--version")
	return err == nil
}

func missingDependencyCount() int {
	count := 0
	for _, name := range []string{"git", "gh"} {
		if !installed(name) {
			count++
		}
	}
	if !filterRepoInstalled() {
		count++
	}
	return count
}

func printInstallInstructions(a *app) {
	if missingDependencyCount() == 0 {
		fmt.Fprintf(a.stdout, "\n%sAll command dependencies are installed.%s\n", a.ui.Green, a.ui.Reset)
		return
	}

	manager := detectPackageManager()
	fmt.Fprintf(a.stdout, "\n%sInstall Instructions%s\n\n", a.ui.Bold, a.ui.Reset)
	fmt.Fprintf(a.stdout, "  Detected: %s", detectOS())
	if manager != "unknown" {
		fmt.Fprintf(a.stdout, " with %s", manager)
	}
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout)

	packages := missingPackages(manager)
	printCommand := func(s string) { fmt.Fprintf(a.stdout, "    %s\n", s) }
	switch manager {
	case "brew":
		printCommand("brew install " + strings.Join(packages, " "))
	case "apt":
		if !installed("gh") {
			fmt.Fprintln(a.stdout, "  Add the GitHub CLI apt repository for gh:")
			printCommand("type -p wget >/dev/null || (sudo apt update && sudo apt install wget -y)")
			printCommand("sudo mkdir -p -m 755 /etc/apt/keyrings")
			printCommand("out=$(mktemp) && wget -nv -O\"$out\" https://cli.github.com/packages/githubcli-archive-keyring.gpg")
			printCommand("cat \"$out\" | sudo tee /etc/apt/keyrings/githubcli-archive-keyring.gpg >/dev/null")
			printCommand("sudo chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg")
			printCommand("sudo mkdir -p -m 755 /etc/apt/sources.list.d")
			printCommand("echo \"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main\" | sudo tee /etc/apt/sources.list.d/github-cli.list >/dev/null")
		}
		printCommand("sudo apt update")
		printCommand("sudo apt install " + strings.Join(packages, " "))
	case "dnf", "yum", "zypper", "pacman", "apk", "xbps":
		prefix := map[string]string{
			"dnf":    "sudo dnf install ",
			"yum":    "sudo yum install ",
			"zypper": "sudo zypper install ",
			"pacman": "sudo pacman -S ",
			"apk":    "sudo apk add ",
			"xbps":   "sudo xbps-install ",
		}[manager]
		printCommand(prefix + strings.Join(packages, " "))
	default:
		fmt.Fprintln(a.stdout, "  No supported package manager was detected. Install manually:")
		if !installed("git") {
			printCommand("git: https://git-scm.com/downloads")
		}
		if !installed("gh") {
			printCommand("gh: https://cli.github.com/")
		}
		if !filterRepoInstalled() {
			printCommand("git-filter-repo: https://github.com/newren/git-filter-repo/blob/main/INSTALL.md")
		}
	}
}

func missingPackages(manager string) []string {
	var packages []string
	add := func(commandName, packageName string) {
		if !installed(commandName) {
			packages = append(packages, packageName)
		}
	}
	add("git", "git")
	switch manager {
	case "pacman", "apk", "xbps":
		add("gh", "github-cli")
	default:
		add("gh", "gh")
	}
	if !filterRepoInstalled() {
		packages = append(packages, "git-filter-repo")
	}
	return packages
}

func detectOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return runtime.GOOS
	}
}

func detectPackageManager() string {
	if runtime.GOOS == "windows" {
		for _, name := range []string{"winget", "scoop", "choco"} {
			if installed(name) {
				if name == "choco" {
					return "chocolatey"
				}
				return name
			}
		}
		return "unknown"
	}
	if runtime.GOOS == "darwin" {
		if installed("brew") {
			return "brew"
		}
		return "unknown"
	}
	for _, name := range []string{"apt", "dnf", "yum", "zypper", "pacman", "apk", "xbps-install", "brew"} {
		if installed(name) {
			if name == "xbps-install" {
				return "xbps"
			}
			return name
		}
	}
	return "unknown"
}

func runClone(a *app, args []string) int {
	visibility := "all"
	user := ""
	limit := "100"
	into := ""

	for len(args) > 0 {
		switch args[0] {
		case "--visibility":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			visibility = value
			args = args[2:]
		case "--user":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			user = value
			args = args[2:]
		case "--limit":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			limit = value
			args = args[2:]
		case "--into":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			into = value
			args = args[2:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}

	if user == "" {
		a.error("The --user option is required.")
		return 1
	}
	if !requireCommand(a, "gh", "clone") || !requireGit(a, "clone") {
		return 1
	}
	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt < 1 {
		a.error("--limit must be 1 or greater.")
		return 1
	}
	if visibility != "all" && visibility != "public" && visibility != "private" {
		a.error("Invalid visibility option. Use 'all', 'public', or 'private'.")
		return 1
	}
	if visibility == "private" || visibility == "all" {
		out, _ := githubcli.Capture(context.Background(), "", "auth", "status")
		if !regexp.MustCompile(`Logged in to .* account ` + regexp.QuoteMeta(user) + ` `).MatchString(out) {
			a.errorf("You are not logged in as the specified user: %s. Set --visibility to 'public' or use 'gh auth login'.", user)
			return 1
		}
	}

	listArgs := githubcli.RepoListArgs(user, visibility, "1")
	out, _ := githubcli.Stdout(context.Background(), "", listArgs...)
	if lineCount(out) == 0 {
		if visibility == "public" || visibility == "private" {
			a.errorf("No %s repositories found for '%s'.", visibility, user)
		} else {
			a.errorf("No repositories found for '%s'.", user)
		}
		return 1
	}

	if into == "" {
		into = user
	}
	if info, err := os.Stat(into); err == nil && !info.IsDir() {
		a.errorf("Unable to create or access the specified directory '%s'.", into)
		return 1
	}
	if err := os.MkdirAll(into, 0o755); err != nil {
		a.errorf("Unable to create or access the specified directory '%s'.", into)
		return 1
	}

	listArgs = githubcli.RepoListArgs(user, visibility, limit)
	reposOut, err := githubcli.Stdout(context.Background(), "", listArgs...)
	if err != nil {
		fmt.Fprintf(a.stderr, "%sError: Failed to list repositories:\n%s%s\n\n", a.ui.Red, err.Error(), a.ui.Reset)
		return 1
	}
	for _, line := range splitLines(reposOut) {
		fields := strings.Split(line, "\t")
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		cloneRepository(a, fields[0], into)
	}
	return 0
}

func cloneRepository(a *app, fullName, into string) {
	repoName := fullName
	if idx := strings.LastIndex(fullName, "/"); idx >= 0 {
		repoName = fullName[idx+1:]
	}
	target := filepath.Join(into, repoName)
	if isDir(target) {
		abs, _ := filepath.Abs(into)
		fmt.Fprintf(a.stdout, "%s%s already exists in %s. Skipping...%s\n", a.ui.Yellow, repoName, abs, a.ui.Reset)
		return
	}
	if out, err := githubcli.Capture(context.Background(), "", "repo", "clone", fullName, target); err == nil {
		abs, _ := filepath.Abs(target)
		fmt.Fprintf(a.stdout, "%sCloned %s into %s%s\n", a.ui.Green, repoName, abs, a.ui.Reset)
	} else {
		fmt.Fprintf(a.stderr, "%sError: Failed to clone %s:\n%s%s\n\n", a.ui.Red, repoName, out, a.ui.Reset)
	}
}

func runCommit(a *app, args []string) int {
	message := ""
	for len(args) > 0 {
		switch args[0] {
		case "--message":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			message = value
			args = args[2:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}
	if message == "" {
		fmt.Fprintf(a.stderr, "%sError: A commit message is required. Use --message <commit_message>.%s\n", a.ui.Red, a.ui.Reset)
		return 1
	}
	if !requireGit(a, "commit") {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		if _, err := runCapture(r.dir, nil, "git", "add", "-A"); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not stage changes for %s%s\n", a.ui.Red, r.display, a.ui.Reset)
			continue
		}
		if _, err := runCapture(r.dir, nil, "git", "diff", "--cached", "--quiet"); err == nil {
			fmt.Fprintf(a.stdout, "%sNo changes to commit for %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		if out, err := runCapture(r.dir, nil, "git", "commit", "-m", message); err == nil {
			fmt.Fprintf(a.stdout, "%sCommit created for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not commit changes for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
		}
	}
	return 0
}

func runFixGitignore(a *app, args []string) int {
	if len(args) > 0 {
		a.errorf("Unknown option: %s", args[0])
		return 1
	}
	if !requireGit(a, "fix-gitignore") {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	candidates := []string{"bin/", "obj/", ".idea/", "vendor/", "node_modules/", "dist/", "build/", "wp-includes/", ".DS_Store", "Thumbs.db", "*.log"}
	for _, r := range repos {
		added := []string{}
		covered := []string{}
		notPresent := []string{}
		for _, entry := range candidates {
			match := findExistingMatch(r.dir, entry)
			if match == "" {
				notPresent = append(notPresent, entry)
				continue
			}
			if _, err := runCapture(r.dir, nil, "git", "check-ignore", "-q", match); err == nil {
				covered = append(covered, entry)
				continue
			}
			if fileContainsLine(filepath.Join(r.dir, ".gitignore"), entry) {
				covered = append(covered, entry)
			} else {
				added = append(added, entry)
			}
		}
		if len(added) > 0 {
			appendGitignoreEntries(filepath.Join(r.dir, ".gitignore"), added)
		}
		fmt.Fprintf(a.stdout, "%s%s:%s\n", a.ui.RepoColor, r.display, a.ui.Reset)
		if len(added) > 0 {
			fmt.Fprintf(a.stdout, "  %sAdded:%s %s\n", a.ui.Green, a.ui.Reset, strings.Join(added, ", "))
			_, _ = runCapture(r.dir, nil, "git", "add", ".gitignore")
			if out, err := runCapture(r.dir, nil, "git", "commit", "-m", "Update .gitignore with missing entries"); err == nil {
				fmt.Fprintf(a.stdout, "  %sCommitted .gitignore updates%s\n", a.ui.Green, a.ui.Reset)
			} else {
				fmt.Fprintf(a.stderr, "  %sError: Could not commit .gitignore:\n%s%s\n", a.ui.Red, out, a.ui.Reset)
			}
		}
		if len(covered) > 0 {
			fmt.Fprintf(a.stdout, "  %sSkipped (already covered):%s %s\n", a.ui.Yellow, a.ui.Reset, strings.Join(covered, ", "))
		}
		if len(notPresent) > 0 {
			fmt.Fprintf(a.stdout, "  %sSkipped (not present on disk):%s %s\n", a.ui.Yellow, a.ui.Reset, strings.Join(notPresent, ", "))
		}
		if len(added) == 0 && len(covered) == 0 && len(notPresent) == 0 {
			fmt.Fprintf(a.stdout, "  %sNo changes needed.%s\n", a.ui.Yellow, a.ui.Reset)
		}
	}
	return 0
}

func findExistingMatch(root, entry string) string {
	var result string
	wantDir := strings.HasSuffix(entry, "/")
	pattern := strings.TrimSuffix(entry, "/")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || result != "" {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if wantDir {
			if d.IsDir() && d.Name() == pattern {
				result = "./" + filepath.ToSlash(rel)
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			if ok, _ := filepath.Match(pattern, d.Name()); ok {
				result = "./" + filepath.ToSlash(rel)
			}
		}
		return nil
	})
	return result
}

func fileContainsLine(path, line string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if scanner.Text() == line {
			return true
		}
	}
	return false
}

func appendGitignoreEntries(path string, entries []string) {
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 && data[len(data)-1] != '\n' {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = f.WriteString("\n")
			_ = f.Close()
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	for _, entry := range entries {
		fmt.Fprintln(f, entry)
	}
}

func runLicense(a *app, args []string) int {
	repoName := ""
	holder := ""
	overwrite := false
	for len(args) > 0 {
		switch args[0] {
		case "--repo":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			repoName = value
			args = args[2:]
		case "--name":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			holder = value
			args = args[2:]
		case "--overwrite":
			overwrite = true
			args = args[1:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}
	if holder == "" {
		a.error("Copyright holder name is required. Use --name <NAME>.")
		return 1
	}
	if !requireGit(a, "license") {
		return 1
	}
	root := "."
	if repoName != "" {
		root = repoName
	}
	repos, err := findGitRepositories(root)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		path := filepath.Join(r.dir, "LICENSE")
		if fileExists(path) && !overwrite {
			fmt.Fprintf(a.stdout, "%sLICENSE file already exists in repository: %s (use --overwrite to replace it)%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		_ = os.WriteFile(path, []byte(mitLicense(holder)), 0o644)
		if overwrite && fileExists(path) {
			fmt.Fprintf(a.stdout, "%sLICENSE file overwritten in repository: %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stdout, "%sLICENSE file created in repository: %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		}
	}
	return 0
}

func mitLicense(holder string) string {
	return "MIT License\n\nCopyright (c) " + holder + "\n\n" + `Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
`
}

func runUntrack(a *app, args []string) int {
	if len(args) > 0 {
		a.errorf("Unknown option: %s", args[0])
		return 1
	}
	if !requireGit(a, "untrack") {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		if !fileExists(filepath.Join(r.dir, ".gitignore")) {
			fmt.Fprintf(a.stdout, "%sNo .gitignore file found for %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		out, _ := runStdout(r.dir, nil, "git", "ls-files", "--ignored", "--cached", "--exclude-standard")
		if strings.TrimSpace(out) == "" {
			fmt.Fprintf(a.stdout, "%sNo currently tracked files match .gitignore in %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		zout, _ := runStdout(r.dir, nil, "git", "ls-files", "--ignored", "--cached", "--exclude-standard", "-z")
		files := strings.Split(strings.TrimRight(zout, "\x00"), "\x00")
		rmArgs := append([]string{"rm", "--cached", "-q", "--"}, files...)
		if out, err := runCapture(r.dir, nil, "git", rmArgs...); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not untrack files for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			continue
		}
		if out, err := runCapture(r.dir, nil, "git", "commit", "-q", "-m", "Stop tracking files defined in .gitignore"); err == nil {
			fmt.Fprintf(a.stdout, "%sStopped tracking and committed ignored files for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not commit changes for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
		}
	}
	return 0
}

func runPull(a *app, args []string) int {
	rebase := false
	force := false
	for len(args) > 0 {
		switch args[0] {
		case "--rebase":
			rebase = true
			args = args[1:]
		case "--force":
			force = true
			args = args[1:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}
	if !requireGit(a, "pull") {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		pullArgs := []string{"pull"}
		if rebase {
			pullArgs = append(pullArgs, "--rebase")
		}
		if force {
			pullArgs = append(pullArgs, "--force")
		}
		out, err := runCapture(r.dir, nil, "git", pullArgs...)
		if err == nil {
			if strings.Contains(out, "Already up to date") {
				a.skip(r.display, "Already up to date. Skipping...")
			} else {
				a.ok(r.display, "Git pull completed")
			}
		} else {
			a.error(r.display, "Git pull failed:")
			fmt.Fprintf(a.stderr, "%s\n\n", out)
		}
	}
	return 0
}

func runPush(a *app, args []string) int {
	force := false
	forceUnsafe := false
	for len(args) > 0 {
		switch args[0] {
		case "--force":
			force = true
			args = args[1:]
		case "--force-unsafe":
			forceUnsafe = true
			args = args[1:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}
	if force && forceUnsafe {
		a.error("Use either --force or --force-unsafe, not both.")
		return 1
	}
	if !requireGit(a, "push") {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		pushArgs := []string{"push", "origin", "HEAD"}
		if force {
			pushArgs = []string{"push", "--force-with-lease", "origin", "HEAD"}
		} else if forceUnsafe {
			if !confirm(a, "Raw force push "+r.display+" with --force?") {
				a.skip(r.display, "Skipping unsafe force push.")
				continue
			}
			pushArgs = []string{"push", "--force", "origin", "HEAD"}
		}
		out, err := runCapture(r.dir, nil, "git", pushArgs...)
		if err == nil {
			if strings.Contains(out, "Everything up-to-date") {
				a.skip(r.display, "No changes to push. Skipping...")
			} else {
				a.ok(r.display, "Git push completed")
			}
		} else {
			a.error(r.display, "Git push failed:")
			fmt.Fprintf(a.stderr, "%s\n\n", out)
		}
	}
	return 0
}

func runReset(a *app, args []string) int {
	confirmed := false
	for len(args) > 0 {
		switch args[0] {
		case "--confirm":
			confirmed = true
			args = args[1:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}
	if !requireGit(a, "reset") {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		branchOut, _ := runStdout(r.dir, nil, "git", "rev-parse", "--abbrev-ref", "HEAD")
		branch := strings.TrimSpace(branchOut)
		if branch == "HEAD" {
			fmt.Fprintf(a.stdout, "%s%s is in detached HEAD state. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		if out, err := runCapture(r.dir, nil, "git", "fetch", "origin", branch); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Fetch failed for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			continue
		}
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "--verify", "--quiet", "origin/"+branch); err != nil {
			fmt.Fprintf(a.stdout, "%sBranch '%s' has no remote counterpart in %s. Skipping...%s\n", a.ui.Yellow, branch, r.display, a.ui.Reset)
			continue
		}
		ahead := strings.TrimSpace(mustStdout(r.dir, "git", "rev-list", "--count", "origin/"+branch+".."+branch))
		behind := strings.TrimSpace(mustStdout(r.dir, "git", "rev-list", "--count", branch+"..origin/"+branch))
		if ahead == "" {
			ahead = "0"
		}
		if behind == "" {
			behind = "0"
		}
		fmt.Fprintf(a.stdout, "%s--- %s (%s) ---%s\n", a.ui.Cyan, r.display, branch, a.ui.Reset)
		if ahead == "0" && behind == "0" {
			fmt.Fprintf(a.stdout, "%sBranch '%s' is already up to date with origin/%s in %s. Nothing to reset.%s\n", a.ui.Yellow, branch, branch, r.display, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stderr, "Divergence: %sahead %s%s, %sbehind %s%s\n", a.ui.Cyan, ahead, a.ui.Reset, a.ui.Red, behind, a.ui.Reset)
		dirty, _ := runStdout(r.dir, nil, "git", "status", "--porcelain")
		if strings.TrimSpace(dirty) != "" {
			fmt.Fprintf(a.stderr, "%sWarning: Working tree has uncommitted changes that will be discarded.%s\n", a.ui.Red, a.ui.Reset)
		}
		fmt.Fprintf(a.stderr, "%sThis will hard reset %s to origin/%s, discarding %s local commit(s).%s\n", a.ui.Red, branch, branch, ahead, a.ui.Reset)
		if !confirmed && !confirm(a, "Proceed with reset for "+r.display+"?") {
			fmt.Fprintf(a.stdout, "%sSkipping %s.%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		if out, err := runCapture(r.dir, nil, "git", "reset", "--hard", "origin/"+branch); err == nil {
			fmt.Fprintf(a.stdout, "%sSuccessfully reset %s to origin/%s%s\n", a.ui.Green, r.display, branch, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Reset failed for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
		}
	}
	return 0
}

func mustStdout(dir, name string, args ...string) string {
	out, _ := runStdout(dir, nil, name, args...)
	return out
}

func runInfo(a *app, args []string) int {
	repoName := ""
	for len(args) > 0 {
		switch args[0] {
		case "--repo":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			repoName = value
			args = args[2:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}
	if !requireGit(a, "info") {
		return 1
	}
	root := "."
	if repoName != "" {
		root = repoName
	}
	repos, err := findGitRepositories(root)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		fmt.Fprintf(a.stdout, "Repository:         %s%s%s\n", a.ui.RepoColor, r.display, a.ui.Reset)
		status, _ := runStdout(r.dir, nil, "git", "status", "--porcelain")
		if strings.TrimSpace(status) == "" {
			fmt.Fprintf(a.stdout, "Status:             %sClean%s\n", a.ui.Green, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stdout, "Status:             %sDirty (uncommitted changes or untracked files)%s\n", a.ui.Yellow, a.ui.Reset)
		}
		printLicenseInfo(a, r.dir)
		branch, _ := runStdout(r.dir, nil, "git", "rev-parse", "--abbrev-ref", "HEAD")
		fmt.Fprintf(a.stdout, "Current branch:     %s\n", strings.TrimSpace(branch))
		hasCommits := false
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "HEAD"); err == nil {
			hasCommits = true
		}
		ab, _ := runStdout(r.dir, nil, "git", "rev-list", "--left-right", "--count", "HEAD...@{u}")
		if fields := strings.Fields(ab); len(fields) >= 2 {
			fmt.Fprintf(a.stdout, "Ahead/behind:       %s ahead, %s behind remote\n", fields[0], fields[1])
		} else {
			fmt.Fprintln(a.stdout, "Ahead/behind:       No upstream set")
		}
		printBranches(a, r.dir)
		printRemotes(a, r.dir)
		initial, _ := runStdout(r.dir, nil, "git", "log", "--reverse", "--format=%ci - %s")
		if hasCommits && strings.TrimSpace(initial) != "" {
			fmt.Fprintf(a.stdout, "Initial commit:     %s\n", firstLine(initial))
		} else {
			fmt.Fprintln(a.stdout, "Initial commit:     None (repository is empty)")
		}
		commitCount, _ := runStdout(r.dir, nil, "git", "rev-list", "--all", "--count")
		fmt.Fprintf(a.stdout, "Total commits:      %s\n", strings.TrimSpace(commitCount))
		lastMonth, _ := runStdout(r.dir, nil, "git", "log", "--since=1 month ago", "--format=%ci")
		fmt.Fprintf(a.stdout, "Commits last month: %d\n", len(splitLines(lastMonth)))
		last, _ := runStdout(r.dir, nil, "git", "log", "-1", "--format=%ci - %s")
		fmt.Fprintf(a.stdout, "Last commit:        %s\n", strings.TrimSpace(last))
		if hasCommits {
			printTopAuthors(a, r.dir)
		} else {
			fmt.Fprintln(a.stdout, "Top authors:        None (repository is empty)")
		}
		printLargestFiles(a, r.dir)
		fmt.Fprintln(a.stdout)
	}
	return 0
}

func printLicenseInfo(a *app, dir string) {
	fmt.Fprint(a.stdout, "License:            ")
	data, err := os.ReadFile(filepath.Join(dir, "LICENSE"))
	if err != nil {
		fmt.Fprintf(a.stdout, "%sNone%s\n", a.ui.Yellow, a.ui.Reset)
		return
	}
	line := firstLine(string(data))
	if line == "" {
		fmt.Fprintf(a.stdout, "%sYes%s\n", a.ui.Green, a.ui.Reset)
	} else {
		fmt.Fprintf(a.stdout, "%s'%s'%s\n", a.ui.Green, truncate(line, 70), a.ui.Reset)
	}
}

func printBranches(a *app, dir string) {
	out, _ := runStdout(dir, nil, "git", "branch", "-a", "--no-color")
	branches := []string{}
	for _, line := range splitLines(out) {
		if strings.Contains(line, "remotes") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "* "))
		if line != "" {
			branches = append(branches, line)
		}
	}
	fmt.Fprintf(a.stdout, "Branches (%d):       ", len(branches))
	for i, branch := range branches {
		if i == 0 {
			fmt.Fprintln(a.stdout, branch)
		} else {
			fmt.Fprintf(a.stdout, "%-20s%s\n", "", branch)
		}
	}
	if len(branches) == 0 {
		fmt.Fprintln(a.stdout)
	}
}

func printRemotes(a *app, dir string) {
	out, _ := runStdout(dir, nil, "git", "remote", "-v")
	seen := map[string]bool{}
	remotes := []string{}
	for _, line := range splitLines(out) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && !seen[fields[1]] {
			seen[fields[1]] = true
			remotes = append(remotes, fields[1])
		}
	}
	sort.Strings(remotes)
	if len(remotes) == 0 {
		fmt.Fprintln(a.stdout, "Remotes:            None")
		return
	}
	for i, remote := range remotes {
		if i == 0 {
			fmt.Fprintf(a.stdout, "Remotes:            %s\n", remote)
		} else {
			fmt.Fprintf(a.stdout, "%-20s%s\n", "", remote)
		}
	}
}

func printTopAuthors(a *app, dir string) {
	out, _ := runStdout(dir, nil, "git", "log", "--format=%an <%ae>")
	counts := map[string]int{}
	for _, line := range splitLines(out) {
		counts[line]++
	}
	type row struct {
		name  string
		count int
	}
	rows := []row{}
	for name, count := range counts {
		rows = append(rows, row{name: name, count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count == rows[j].count {
			return rows[i].name < rows[j].name
		}
		return rows[i].count > rows[j].count
	})
	for i, row := range rows {
		if i >= 3 {
			break
		}
		if i == 0 {
			fmt.Fprintf(a.stdout, "Top authors:        %d - %s\n", row.count, row.name)
		} else {
			fmt.Fprintf(a.stdout, "%-20s%d - %s\n", "", row.count, row.name)
		}
	}
}

func printLargestFiles(a *app, dir string) {
	objects, _ := runStdout(dir, nil, "git", "rev-list", "--objects", "--all")
	if strings.TrimSpace(objects) == "" {
		return
	}
	type row struct {
		size int64
		path string
	}
	rows := []row{}
	seen := map[string]bool{}
	for _, line := range splitLines(git.CatFileBatchCheck(dir, objects)) {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		size, _ := strconv.ParseInt(fields[0], 10, 64)
		path := strings.Join(fields[2:], " ")
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		rows = append(rows, row{size: size, path: path})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].size > rows[j].size })
	for i, row := range rows {
		if i >= 3 {
			break
		}
		if i == 0 {
			fmt.Fprintf(a.stdout, "Largest files:      %s - %s\n", humanSize(row.size), row.path)
		} else {
			fmt.Fprintf(a.stdout, "%-20s%s - %s\n", "", humanSize(row.size), row.path)
		}
	}
}

func humanSize(size int64) string {
	switch {
	case size >= 1073741824:
		return fmt.Sprintf("%.2f GB", float64(size)/1073741824)
	case size >= 1048576:
		return fmt.Sprintf("%.2f MB", float64(size)/1048576)
	case size >= 1024:
		return fmt.Sprintf("%.2f KB", float64(size)/1024)
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}

func runReview(a *app, args []string) int {
	if len(args) > 0 {
		a.errorf("Unknown option: %s", args[0])
		return 1
	}
	if !requireGit(a, "review") {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		unpushed, _ := runStdout(r.dir, nil, "git", "rev-list", "HEAD", "--not", "--remotes")
		commits := splitLines(unpushed)
		if len(commits) == 0 {
			fmt.Fprintf(a.stdout, "%sNo unpushed changes for %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		oldest := commits[len(commits)-1]
		base, err := runStdout(r.dir, nil, "git", "rev-parse", "--verify", oldest+"^")
		base = strings.TrimSpace(base)
		if err != nil || base == "" {
			base = strings.TrimSpace(mustStdout(r.dir, "git", "hash-object", "-t", "tree", "/dev/null"))
		}
		diff, _ := runStdout(r.dir, nil, "git", "diff", "--name-status", "-z", "--no-renames", base+"..HEAD")
		added, modified, deleted := parseNameStatusZ(diff)
		if len(added) == 0 && len(modified) == 0 && len(deleted) == 0 {
			fmt.Fprintf(a.stdout, "%sNo unpushed changes for %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		deletedFolders, individualDeleted := groupDeletedFiles(r.dir, deleted)
		fmt.Fprintf(a.stdout, "%s%s:%s\n", a.ui.RepoColor, r.display, a.ui.Reset)
		for _, f := range added {
			fmt.Fprintf(a.stdout, "  %sAdded:%s    %s\n", a.ui.Green, a.ui.Reset, f)
		}
		for _, f := range modified {
			fmt.Fprintf(a.stdout, "  %sEdited:%s   %s\n", a.ui.Yellow, a.ui.Reset, f)
		}
		for _, f := range deletedFolders {
			fmt.Fprintf(a.stderr, "  %sRemoved:%s  %s/ (entire folder)\n", a.ui.Red, a.ui.Reset, f)
		}
		for _, f := range individualDeleted {
			fmt.Fprintf(a.stderr, "  %sRemoved:%s  %s\n", a.ui.Red, a.ui.Reset, f)
		}
		fmt.Fprintln(a.stdout)
	}
	return 0
}

func parseNameStatusZ(data string) (added, modified, deleted []string) {
	parts := strings.Split(strings.TrimRight(data, "\x00"), "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		status := parts[i]
		file := parts[i+1]
		if status == "" {
			continue
		}
		switch status[0] {
		case 'A':
			added = append(added, file)
		case 'M':
			modified = append(modified, file)
		case 'D':
			deleted = append(deleted, file)
		}
	}
	return
}

func groupDeletedFiles(dir string, deleted []string) ([]string, []string) {
	deletedFolders := []string{}
	individual := []string{}
	for _, file := range deleted {
		covered := false
		for _, folder := range deletedFolders {
			if strings.HasPrefix(file, folder+"/") {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		parent := filepath.ToSlash(filepath.Dir(file))
		if parent == "." {
			individual = append(individual, file)
			continue
		}
		current := ""
		found := ""
		for _, part := range strings.Split(parent, "/") {
			if current == "" {
				current = part
			} else {
				current += "/" + part
			}
			if out, _ := runStdout(dir, nil, "git", "ls-tree", "-d", "HEAD", current); strings.TrimSpace(out) == "" {
				found = current
				break
			}
		}
		if found != "" {
			deletedFolders = append(deletedFolders, found)
		} else {
			individual = append(individual, file)
		}
	}
	return deletedFolders, individual
}

func runRenameBranch(a *app, args []string) int {
	oldBranch := ""
	newBranch := ""
	for len(args) > 0 {
		switch args[0] {
		case "--oldbranch":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			oldBranch = value
			args = args[2:]
		case "--newbranch":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			newBranch = value
			args = args[2:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}
	if oldBranch == "" || newBranch == "" {
		a.error("Both --oldbranch and --newbranch options must be provided.")
		return 1
	}
	if !requireGit(a, "rename-branch") {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		if _, err := os.Stat(r.dir); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Directory is inaccessible: %s%s\n", a.ui.Red, r.display, a.ui.Reset)
			continue
		}
		if out, err := runCapture(r.dir, nil, "git", "rev-parse", "--is-inside-work-tree"); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Not a valid git repository for %s:\n%s%s\n", a.ui.Red, r.display, out, a.ui.Reset)
			continue
		}
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "--verify", "--quiet", "refs/heads/"+oldBranch); err != nil {
			fmt.Fprintf(a.stdout, "%sOld branch '%s' does not exist in %s. Skipping...%s\n", a.ui.Yellow, oldBranch, r.display, a.ui.Reset)
			continue
		}
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "--verify", "--quiet", "refs/heads/"+newBranch); err == nil {
			fmt.Fprintf(a.stdout, "%sNew branch '%s' already exists in %s. Skipping...%s\n", a.ui.Yellow, newBranch, r.display, a.ui.Reset)
			continue
		}
		if out, err := runCapture(r.dir, nil, "git", "branch", "-m", oldBranch, newBranch); err == nil {
			fmt.Fprintf(a.stdout, "%sBranch renamed from '%s' to '%s' for %s%s\n", a.ui.Green, oldBranch, newBranch, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Failed to rename branch in %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
		}
	}
	return 0
}

func runRenameRepo(a *app, args []string) int {
	editDescription := false
	for len(args) > 0 {
		switch args[0] {
		case "--description":
			editDescription = true
			args = args[1:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}
	if !requireGit(a, "rename-repo") || !requireCommand(a, "gh", "rename-repo") {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		a.warn("No Git repositories found under the current directory.")
		return 0
	}
	for _, r := range repos {
		oldName, err := githubcli.Stdout(context.Background(), r.dir, "repo", "view", "--json", "name", "-q", ".name")
		if err != nil {
			fmt.Fprintf(a.stdout, "%sSkipping %s: No remote or not a GitHub repository.%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		oldName = strings.TrimSpace(oldName)
		fmt.Fprintf(a.stdout, "\n%sRepository: %s%s\n", a.ui.RepoColor, oldName, a.ui.Reset)
		newName, _ := promptRead("Enter new name (leave blank to skip): ")
		newDesc := ""
		if editDescription {
			oldDesc, _ := githubcli.Stdout(context.Background(), r.dir, "repo", "view", "--json", "description", "-q", ".description")
			oldDesc = strings.TrimSpace(oldDesc)
			if oldDesc == "" {
				fmt.Fprintln(a.stdout, "Current description: <None>")
			} else {
				fmt.Fprintf(a.stdout, "Current description: %s\n", oldDesc)
			}
			newDesc, _ = promptRead("Enter new description (leave blank to skip): ")
		}
		if editDescription && newDesc != "" {
			if out, err := githubcli.Capture(context.Background(), r.dir, "repo", "edit", "--description", newDesc); err == nil {
				fmt.Fprintf(a.stdout, "%sSuccessfully updated description for %s%s\n", a.ui.Green, oldName, a.ui.Reset)
			} else {
				fmt.Fprintf(a.stderr, "%sError: Failed to update description for %s:\n%s%s\n", a.ui.Red, oldName, out, a.ui.Reset)
			}
		}
		if newName != "" {
			if out, err := githubcli.Capture(context.Background(), r.dir, "repo", "rename", newName, "--yes"); err == nil {
				fmt.Fprintf(a.stdout, "%sSuccessfully renamed %s to %s%s\n", a.ui.Green, oldName, newName, a.ui.Reset)
			} else {
				fmt.Fprintf(a.stderr, "%sError: Failed to rename %s:\n%s%s\n", a.ui.Red, oldName, out, a.ui.Reset)
			}
		}
	}
	return 0
}

func runRemoveSecrets(a *app, args []string) int {
	confirmed := false
	for len(args) > 0 {
		switch args[0] {
		case "--confirm":
			confirmed = true
			args = args[1:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}
	if !requireGit(a, "remove-secrets") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "remove-secrets")
	if !ok {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	patterns := []string{".env", ".env.*", "*.pem", "*.key", "*.p12", "*.pfx", "id_rsa", "id_rsa.pub", "id_ed25519", "id_ed25519.pub", "config.json", "secrets.json", "credentials.json", "*.secret"}
	status := 0
	for _, r := range repos {
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "--is-inside-work-tree"); err != nil {
			fmt.Fprintf(a.stderr, "%sError: %s is not a valid or accessible git repository. Skipping...%s\n", a.ui.Red, r.display, a.ui.Reset)
			status = 1
			continue
		}
		matchedPatterns := []string{}
		matchedFiles := []string{}
		for _, pattern := range patterns {
			out, _ := runStdout(r.dir, nil, "git", "log", "--all", "--oneline", "--", pattern)
			if strings.TrimSpace(out) == "" {
				continue
			}
			matchedPatterns = append(matchedPatterns, pattern)
			files, _ := runStdout(r.dir, nil, "git", "log", "--all", "--format=", "--name-only", "--", pattern)
			matchedFiles = append(matchedFiles, splitLines(files)...)
		}
		matchedFiles = sortedUnique(matchedFiles)
		if len(matchedPatterns) == 0 {
			fmt.Fprintf(a.stdout, "%sNo target patterns found in history. Skipping %s cleanly...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stdout, "%sFound %d sensitive file(s) matching %d pattern(s) in %s:%s\n", a.ui.Yellow, len(matchedFiles), len(matchedPatterns), r.display, a.ui.Reset)
		for _, file := range matchedFiles {
			fmt.Fprintf(a.stdout, "  %s\n", file)
		}
		fmt.Fprintln(a.stdout)
		if !confirmed {
			a.error(r.display, "Refusing to rewrite history without --confirm.")
			status = 1
			continue
		}
		filterArgs := []string{}
		for _, pattern := range matchedPatterns {
			filterArgs = append(filterArgs, "--path-glob", pattern)
		}
		filterArgs = append(filterArgs, "--invert-paths", "--use-base-name", "--partial", "--force")
		remoteURL := strings.TrimSpace(mustStdout(r.dir, "git", "remote", "get-url", "origin"))
		if out, err := runFilterRepo(r.dir, filterCmd, filterArgs, nil); err == nil {
			fmt.Fprintf(a.stdout, "%sSuccessfully purged %d sensitive file(s) from %s%s\n", a.ui.Green, len(matchedFiles), r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Rewrite failed for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			status = 1
			continue
		}
		if remoteURL != "" {
			_, _ = runCapture(r.dir, nil, "git", "remote", "add", "origin", remoteURL)
		}
	}
	return status
}

func runFilterRepo(dir string, filterCmd []string, args []string, env []string) (string, error) {
	if len(filterCmd) == 0 {
		return "", errors.New("missing filter command")
	}
	return runCapture(dir, env, filterCmd[0], append(filterCmd[1:], args...)...)
}

func runRewriteAuthors(a *app, args []string) int {
	force := false
	repoName := ""
	newName := ""
	newEmail := ""
	for len(args) > 0 {
		switch args[0] {
		case "--name":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			newName = value
			args = args[2:]
		case "--email":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			newEmail = value
			args = args[2:]
		case "--force":
			force = true
			args = args[1:]
		case "--repo":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			repoName = value
			args = args[2:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}
	if newName == "" || newEmail == "" {
		fmt.Fprintf(a.stderr, "%sError: Both --name and --email options must be provided.%s\n", a.ui.Red, a.ui.Reset)
		return 1
	}
	if !requireGit(a, "rewrite-authors") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-authors")
	if !ok {
		return 1
	}
	root := "."
	if repoName != "" {
		root = repoName
	}
	repos, err := findGitRepositories(root)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	filterArgs := []string{"--partial"}
	if force {
		filterArgs = append(filterArgs, "--force")
	}
	filterArgs = append(filterArgs,
		"--email-callback", `import os; return os.environ["NEW_EMAIL_ENV"].encode("utf-8")`,
		"--name-callback", `import os; return os.environ["NEW_NAME_ENV"].encode("utf-8")`,
	)
	for _, r := range repos {
		remoteURL := strings.TrimSpace(mustStdout(r.dir, "git", "remote", "get-url", "origin"))
		out, err := runFilterRepo(r.dir, filterCmd, filterArgs, []string{"NEW_EMAIL_ENV=" + newEmail, "NEW_NAME_ENV=" + newName})
		if err == nil {
			if remoteURL != "" {
				if _, err := runCapture(r.dir, nil, "git", "remote", "get-url", "origin"); err != nil {
					if restore, err := runCapture(r.dir, nil, "git", "remote", "add", "origin", remoteURL); err != nil {
						fmt.Fprintf(a.stderr, "%sWarning: Author rewrite completed for %s, but origin could not be restored:\n%s%s\n\n", a.ui.Red, r.display, restore, a.ui.Reset)
						return 1
					}
				}
			}
			fmt.Fprintf(a.stdout, "%sAuthor and commiter information updated for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not update git author and commiter information for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
		}
	}
	return 0
}

func runRewriteCommits(a *app, args []string) int {
	if len(args) > 0 {
		a.errorf("Unknown option: %s", args[0])
		return 1
	}
	if !requireGit(a, "rewrite-commits") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-commits")
	if !ok {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "HEAD"); err != nil {
			fmt.Fprintf(a.stdout, "%sRepository has no commits in %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		remoteURL := strings.TrimSpace(mustStdout(r.dir, "git", "remote", "get-url", "origin"))
		mapping, err := buildCommitMessageMapping(r.dir)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect commits for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			continue
		}
		if len(mapping) == 0 {
			fmt.Fprintf(a.stdout, "%sNo commits require rewriting in %s (already format compliant). Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		callback, err := writeCommitCallback(mapping)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not prepare commit callback for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			continue
		}
		out, err := runFilterRepo(r.dir, filterCmd, []string{"--partial", "--commit-callback", callback, "--force"}, nil)
		_ = os.Remove(callback)
		if err == nil {
			fmt.Fprintf(a.stdout, "%sRewrote commit messages for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
			if remoteURL != "" {
				_, _ = runCapture(r.dir, nil, "git", "remote", "add", "origin", remoteURL)
			}
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not update commit messages for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
		}
	}
	return 0
}

var conventionalRe = regexp.MustCompile(`^(feat|fix|docs|chore|test|build|ci|perf|refactor|style)(\(.*\))?: `)

func buildCommitMessageMapping(dir string) (map[string]string, error) {
	out, err := runStdout(dir, nil, "git", "rev-list", "--all")
	if err != nil {
		return nil, err
	}
	mapping := map[string]string{}
	for _, commit := range splitLines(out) {
		msg, _ := runStdout(dir, nil, "git", "log", "-1", "--format=%B", commit)
		if conventionalRe.MatchString(firstLine(msg)) {
			continue
		}
		diff, _ := runStdout(dir, nil, "git", "diff-tree", "--root", "--no-commit-id", "--name-status", "-r", commit)
		if strings.TrimSpace(diff) == "" {
			continue
		}
		newMsg := categorizeCommit(diff)
		if newMsg != "" && newMsg != strings.TrimRight(msg, "\n") {
			mapping[commit] = newMsg
		}
	}
	return mapping, nil
}

func categorizeCommit(diff string) string {
	firstFile := ""
	hasDocs, hasTests, hasConfig, hasSrc := false, false, false, false
	additions, deletions, modifications := 0, 0, 0
	for _, line := range splitLines(diff) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		status := fields[0][0]
		file := fields[1]
		if firstFile == "" {
			firstFile = file
		}
		switch status {
		case 'A':
			additions++
		case 'D':
			deletions++
		default:
			modifications++
		}
		switch {
		case regexp.MustCompile(`(\.md$|\.txt$|\.rst$|^LICENSE|^docs/)`).MatchString(file):
			hasDocs = true
		case regexp.MustCompile(`(^test/|^spec/|_test\.|spec\.|\\.test\.)`).MatchString(file):
			hasTests = true
		case regexp.MustCompile(`(^\.github/|^Makefile$|^Dockerfile$|\.yml$|^\w+\.json$)`).MatchString(file):
			hasConfig = true
		default:
			hasSrc = true
		}
	}
	total := additions + deletions + modifications
	if total == 0 {
		return ""
	}
	typ := "chore"
	switch {
	case !hasSrc && !hasConfig && !hasTests && hasDocs:
		typ = "docs"
	case !hasSrc && !hasConfig && !hasDocs && hasTests:
		typ = "test"
	case !hasSrc && !hasTests && !hasDocs && hasConfig:
		typ = "chore"
	case additions > 0 && deletions == 0 && hasSrc:
		typ = "feat"
	case deletions > 0 && additions == 0 && modifications == 0:
		typ = "chore"
	case hasSrc && (modifications > 0 || (additions > 0 && deletions > 0)):
		typ = "fix"
	}
	target := firstFile
	if total > 1 {
		if strings.Contains(firstFile, "/") {
			target = firstFile[:strings.LastIndex(firstFile, "/")+1]
		} else {
			target = filepath.Base(firstFile)
		}
	}
	action := "update"
	if additions > 0 && deletions == 0 && modifications == 0 {
		action = "add"
	} else if deletions > 0 && additions == 0 && modifications == 0 {
		action = "remove"
	}
	return fmt.Sprintf("%s: %s %s", typ, action, target)
}

func writeCommitCallback(mapping map[string]string) (string, error) {
	f, err := os.CreateTemp("", "git-wrangler-commit-callback-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	fmt.Fprintln(f, "mapping = {}")
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(f, "mapping[b%q] = b%q\n", key, mapping[key]+"\n")
	}
	fmt.Fprintln(f, "if commit.original_id in mapping:")
	fmt.Fprintln(f, "    commit.message = mapping[commit.original_id]")
	return f.Name(), nil
}

func runRewriteDates(a *app, args []string) int {
	startDate := ""
	endDate := ""
	confirmed := false
	for len(args) > 0 {
		switch args[0] {
		case "--start-date":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			startDate = value
			args = args[2:]
		case "--end-date":
			value, ok := requireValue(a, args[0], args)
			if !ok {
				return 1
			}
			endDate = value
			args = args[2:]
		case "--confirm":
			confirmed = true
			args = args[1:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}
	if startDate != "" && !validDate(startDate) {
		a.error("--start-date must be in YYYY-MM-DD format.")
		return 1
	}
	if endDate != "" && !validDate(endDate) {
		a.error("--end-date must be in YYYY-MM-DD format.")
		return 1
	}
	if !requireGit(a, "rewrite-dates") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-dates")
	if !ok {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "HEAD"); err != nil {
			fmt.Fprintf(a.stdout, "%s%s has no commits. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stdout, "%sProcessing %s...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
		countOut := strings.TrimSpace(mustStdout(r.dir, "git", "rev-list", "--all", "--count"))
		count, _ := strconv.Atoi(countOut)
		if count < 2 {
			fmt.Fprintf(a.stdout, "%s%s has fewer than 2 commits. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		startEpoch := int64(0)
		endEpoch := int64(0)
		if startDate != "" {
			startEpoch = parseLocalDate(startDate)
		} else {
			startEpoch, _ = strconv.ParseInt(strings.TrimSpace(firstLine(mustStdout(r.dir, "git", "log", "--all", "--reverse", "--format=%at"))), 10, 64)
		}
		if endDate != "" {
			endEpoch = parseLocalDate(endDate)
		} else {
			endEpoch, _ = strconv.ParseInt(strings.TrimSpace(firstLine(mustStdout(r.dir, "git", "log", "--all", "--format=%at", "-1"))), 10, 64)
		}
		if startEpoch >= endEpoch {
			fmt.Fprintf(a.stderr, "%sError: start date must be before end date in %s.%s\n", a.ui.Red, r.display, a.ui.Reset)
			continue
		}
		remoteURL := strings.TrimSpace(mustStdout(r.dir, "git", "remote", "get-url", "origin"))
		tzOffset := dominantTimezoneOffset(r.dir)
		commits := readCommitTimes(r.dir)
		mapping := distributeCommitTimes(commits, startEpoch, endEpoch)
		fmt.Fprintln(a.stdout, "Commit summary (old -> new):")
		fmt.Fprintln(a.stdout, strings.Repeat("-", 70))
		for _, c := range commits {
			fmt.Fprintf(a.stdout, "  %s  %s -> %s\n", prefix(c.hash, 8), formatEpoch(c.epoch, tzOffset), formatEpoch(mapping[c.hash], tzOffset))
		}
		fmt.Fprintf(a.stderr, "%s\nWARNING: This operation rewrites Git history. A force push will be required to update any remote.%s\n\n", a.ui.Red, a.ui.Reset)
		if !confirmed && !confirm(a, "Proceed with rewrite for "+r.display+"?") {
			fmt.Fprintf(a.stdout, "%sSkipping %s.%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		callback, err := writeDateCallback(mapping, tzOffset)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: timestamp generation failed for %s:\n%s%s\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			continue
		}
		out, err := runFilterRepo(r.dir, filterCmd, []string{"--partial", "--commit-callback", callback, "--force"}, nil)
		_ = os.Remove(callback)
		if err == nil {
			fmt.Fprintf(a.stdout, "%sSuccessfully rewrote commit dates for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
			if remoteURL != "" {
				_, _ = runCapture(r.dir, nil, "git", "remote", "add", "origin", remoteURL)
			}
		} else {
			fmt.Fprintf(a.stderr, "%sError: rewrite failed for %s:\n%s%s\n", a.ui.Red, r.display, out, a.ui.Reset)
		}
	}
	return 0
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func parseLocalDate(s string) int64 {
	t, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return t.Unix()
}

type commitTime struct {
	hash  string
	epoch int64
}

func readCommitTimes(dir string) []commitTime {
	out, _ := runStdout(dir, nil, "git", "log", "--all", "--reverse", "--format=%H %at")
	var commits []commitTime
	for _, line := range splitLines(out) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		epoch, _ := strconv.ParseInt(fields[1], 10, 64)
		commits = append(commits, commitTime{hash: fields[0], epoch: epoch})
	}
	return commits
}

func dominantTimezoneOffset(dir string) string {
	out, _ := runStdout(dir, nil, "git", "log", "--all", "--format=%ai")
	counts := map[string]int{}
	for _, line := range splitLines(out) {
		if len(line) >= 5 {
			offset := line[len(line)-5:]
			if regexp.MustCompile(`^[+-][0-9]{4}$`).MatchString(offset) {
				counts[offset]++
			}
		}
	}
	best := ""
	bestCount := 0
	for offset, count := range counts {
		if count > bestCount {
			best = offset
			bestCount = count
		}
	}
	if best != "" {
		return best
	}
	return time.Now().Format("-0700")
}

func distributeCommitTimes(commits []commitTime, startEpoch, endEpoch int64) map[string]int64 {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	n := len(commits)
	if n == 0 {
		return map[string]int64{}
	}
	totalRange := float64(endEpoch - startEpoch)
	if totalRange <= 0 {
		totalRange = 86400
	}
	slotWidth := totalRange / float64(n)
	timestamps := make([]int64, n)
	for i := range commits {
		slotStart := float64(startEpoch) + float64(i)*slotWidth
		slotCenter := slotStart + slotWidth/2
		jitter := (rng.Float64()*0.8 - 0.4) * slotWidth
		raw := slotCenter + jitter
		dayStart := int64(raw/86400) * 86400
		hour := sampleBimodalHour(rng)
		ts := dayStart + int64(hour)*3600 + int64(rng.Intn(60))*60 + int64(rng.Intn(60))
		if isWeekend(ts) && rng.Float64() < 0.65 {
			wd := weekdayFromEpoch(ts)
			if wd == 2 {
				ts = dayStart - 86400 + int64(18+rng.Intn(5))*3600 + int64(rng.Intn(60))*60 + int64(rng.Intn(60))
			} else {
				ts = dayStart + 86400 + int64(7+rng.Intn(3))*3600 + int64(rng.Intn(60))*60 + int64(rng.Intn(60))
			}
		}
		timestamps[i] = ts
	}
	dayGroups := map[int64][]int{}
	for i, ts := range timestamps {
		dayGroups[ts/86400] = append(dayGroups[ts/86400], i)
	}
	for day, indices := range dayGroups {
		if len(indices) < 2 {
			continue
		}
		rng.Shuffle(len(indices), func(i, j int) { indices[i], indices[j] = indices[j], indices[i] })
		spacing := int64((25 + rng.Intn(66)) * 60)
		latestStart := 22.0 - float64(len(indices)-1)*float64(spacing)/3600.0
		startHour := 7.0
		if latestStart > 7.0 {
			startHour = 7.0 + rng.Float64()*(latestStart-7.0)
		}
		current := int64(startHour * 3600)
		for _, idx := range indices {
			timestamps[idx] = day*86400 + current + int64(rng.Intn(60))
			current += spacing
		}
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	mapping := map[string]int64{}
	for i, c := range commits {
		mapping[c.hash] = timestamps[i]
	}
	return mapping
}

func sampleBimodalHour(rng *rand.Rand) int {
	peak := 10.0
	if rng.Float64() >= 0.5 {
		peak = 15.0
	}
	u1 := rng.Float64()
	if u1 == 0 {
		u1 = 1e-10
	}
	u2 := rng.Float64()
	z := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
	hour := peak + 2.0*z
	if hour < 7 {
		hour = 7
	}
	if hour > 22 {
		hour = 22
	}
	return int(hour)
}

func weekdayFromEpoch(ts int64) int64 {
	return (ts/86400 + 4) % 7
}

func isWeekend(ts int64) bool {
	wd := weekdayFromEpoch(ts)
	return wd == 2 || wd == 3
}

func formatEpoch(epoch int64, offset string) string {
	sign := 1
	if strings.HasPrefix(offset, "-") {
		sign = -1
	}
	hours, _ := strconv.Atoi(offset[1:3])
	minutes, _ := strconv.Atoi(offset[3:5])
	loc := time.FixedZone(offset, sign*(hours*3600+minutes*60))
	return time.Unix(epoch, 0).In(loc).Format("2006-01-02 15:04:05 ") + offset
}

func writeDateCallback(mapping map[string]int64, tzOffset string) (string, error) {
	f, err := os.CreateTemp("", "git-wrangler-date-callback-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	fmt.Fprintln(f, "mapping = {}")
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(f, "mapping[b%q] = b%q\n", key, fmt.Sprintf("%d %s", mapping[key], tzOffset))
	}
	fmt.Fprintln(f, "if commit.original_id in mapping:")
	fmt.Fprintln(f, "    commit.author_date = mapping[commit.original_id]")
	fmt.Fprintln(f, "    commit.committer_date = mapping[commit.original_id]")
	return f.Name(), nil
}

func runRewriteCommitsAI(a *app, args []string) int {
	baseURL := ""
	model := ""
	apiKey := ""
	apiKeyEnv := "OPENAI_API_KEY"
	batchSize := "10"
	maxChars := "3000"
	timeoutSeconds := "90"
	skipConventional := false

	for len(args) > 0 {
		switch args[0] {
		case "--base-url":
			value, ok := requireAIValue(a, args[0], args)
			if !ok {
				return 1
			}
			baseURL = value
			args = args[2:]
		case "--model":
			value, ok := requireAIValue(a, args[0], args)
			if !ok {
				return 1
			}
			model = value
			args = args[2:]
		case "--api-key":
			value, ok := requireAIValue(a, args[0], args)
			if !ok {
				return 1
			}
			apiKey = value
			args = args[2:]
		case "--api-key-env":
			value, ok := requireAIValue(a, args[0], args)
			if !ok {
				return 1
			}
			apiKeyEnv = value
			args = args[2:]
		case "--batch-size":
			value, ok := requireAIValue(a, args[0], args)
			if !ok {
				return 1
			}
			batchSize = value
			args = args[2:]
		case "--max-chars-per-commit":
			value, ok := requireAIValue(a, args[0], args)
			if !ok {
				return 1
			}
			maxChars = value
			args = args[2:]
		case "--timeout":
			value, ok := requireAIValue(a, args[0], args)
			if !ok {
				return 1
			}
			timeoutSeconds = value
			args = args[2:]
		case "--skip-conventional":
			skipConventional = true
			args = args[1:]
		default:
			a.errorf("Unknown option: %s", args[0])
			return 1
		}
	}

	if !positiveInt(batchSize) {
		a.plainErrorf("--batch-size must be a positive integer.")
		return 1
	}
	batch, _ := strconv.Atoi(batchSize)
	if batch > 50 {
		a.plainErrorf("--batch-size must be 50 or less.")
		return 1
	}
	if !positiveInt(maxChars) {
		a.plainErrorf("--max-chars-per-commit must be a positive integer.")
		return 1
	}
	if !positiveInt(timeoutSeconds) {
		a.plainErrorf("--timeout must be a positive integer.")
		return 1
	}
	maxCharsInt, _ := strconv.Atoi(maxChars)
	timeoutInt, _ := strconv.Atoi(timeoutSeconds)
	if baseURL == "" {
		a.plainErrorf("--base-url is required.")
		return 1
	}
	if model == "" {
		a.plainErrorf("--model is required.")
		return 1
	}
	if envKey := os.Getenv(apiKeyEnv); envKey != "" {
		apiKey = envKey
	}
	if apiKey == "" {
		a.plainErrorf("API key is required. Set %s or pass --api-key.", apiKeyEnv)
		return 1
	}

	if !requireGit(a, "rewrite-commits-ai") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-commits-ai")
	if !ok {
		return 1
	}

	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	workDir, err := os.MkdirTemp("", "git-wrangler-ai-*")
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	defer os.RemoveAll(workDir)

	aiRepos := make([]ai.Repository, 0, len(repos))
	for _, r := range repos {
		aiRepos = append(aiRepos, ai.Repository{Dir: r.dir, Name: r.display, GitDir: r.gitDir})
	}
	plan, err := ai.Generate(context.Background(), aiRepos, ai.Config{
		BaseURL:           baseURL,
		Model:             model,
		APIKey:            apiKey,
		BatchSize:         batch,
		MaxCharsPerCommit: maxCharsInt,
		Timeout:           time.Duration(timeoutInt) * time.Second,
		SkipConventional:  skipConventional,
		WorkDir:           workDir,
	}, a.stdout, func(question string) bool {
		return confirm(a, question)
	})
	if errors.Is(err, ai.ErrCancelled) {
		fmt.Fprintf(a.stdout, "%sStopped before sending any data.%s\n", a.ui.Yellow, a.ui.Reset)
		return 1
	}
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	fmt.Fprint(a.stdout, plan.Summary)
	if plan.GeneratedCount == 0 {
		return 0
	}
	fmt.Fprintf(a.stderr, "%sWARNING: This operation rewrites Git history. A force push will be required to update remotes.%s\n", a.ui.Red, a.ui.Reset)
	if !confirm(a, "Apply these generated commit messages to all listed repositories?") {
		fmt.Fprintf(a.stdout, "%sRewrite cancelled. Generated AI messages were temporary and have been discarded.%s\n", a.ui.Yellow, a.ui.Reset)
		return 1
	}
	return applyAIPlan(a, plan, filterCmd)
}

func requireAIValue(a *app, option string, args []string) (string, bool) {
	if len(args) < 2 || args[1] == "" || strings.HasPrefix(args[1], "--") {
		a.plainErrorf("%s requires a value.", option)
		return "", false
	}
	return args[1], true
}

func positiveInt(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0 && regexp.MustCompile(`^[0-9]+$`).MatchString(s)
}

func applyAIPlan(a *app, plan *ai.Plan, filterCmd []string) int {
	hadError := false
	for _, repoPlan := range plan.Repos {
		remoteURL := strings.TrimSpace(mustStdout(repoPlan.Dir, "git", "remote", "get-url", "origin"))
		out, err := runFilterRepo(repoPlan.Dir, filterCmd, []string{"--partial", "--commit-callback", repoPlan.CallbackFile, "--force"}, nil)
		if err == nil {
			if remoteURL != "" {
				if _, err := runCapture(repoPlan.Dir, nil, "git", "remote", "get-url", "origin"); err != nil {
					if restore, err := runCapture(repoPlan.Dir, nil, "git", "remote", "add", "origin", remoteURL); err != nil {
						fmt.Fprintf(a.stderr, "%sWarning: Commit rewrite completed for %s, but origin could not be restored:\n%s%s\n\n", a.ui.Red, repoPlan.Name, restore, a.ui.Reset)
						hadError = true
						continue
					}
				}
			}
			fmt.Fprintf(a.stdout, "%sRewrote %d commit message(s) for %s%s\n", a.ui.Green, repoPlan.ChangedCount, repoPlan.Name, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not rewrite commit messages for %s:\n%s%s\n\n", a.ui.Red, repoPlan.Name, out, a.ui.Reset)
			hadError = true
		}
	}
	if hadError {
		return 1
	}
	return 0
}
