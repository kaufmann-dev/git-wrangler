package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/kaufmann-dev/git-wrangler/internal/auth"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
	"github.com/kaufmann-dev/git-wrangler/internal/git"
	"github.com/kaufmann-dev/git-wrangler/internal/githubcli"
	"github.com/kaufmann-dev/git-wrangler/internal/run"
	"github.com/kaufmann-dev/git-wrangler/internal/ui"
	"github.com/kaufmann-dev/git-wrangler/internal/version"
	"github.com/spf13/cobra"
)

type app struct {
	ctx    context.Context
	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
	input  *bufio.Reader
	ui     ui.Theme
	runner run.Runner
	git    git.Client
	gh     githubcli.Client
	creds  credentials.Store
	auth   auth.GitHubAuthenticator
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

const rootBanner = `  ██████╗ ██╗████████╗    ██╗    ██╗██████╗  █████╗ ███╗   ██╗ ██████╗ ██╗     ███████╗██████╗
 ██╔════╝ ██║╚══██╔══╝    ██║    ██║██╔══██╗██╔══██╗████╗  ██║██╔════╝ ██║     ██╔════╝██╔══██╗
 ██║  ███╗██║   ██║       ██║ █╗ ██║██████╔╝███████║██╔██╗ ██║██║  ███╗██║     █████╗  ██████╔╝
 ██║   ██║██║   ██║       ██║███╗██║██╔══██╗██╔══██║██║╚██╗██║██║   ██║██║     ██╔══╝  ██╔══██╗
 ╚██████╔╝██║   ██║       ╚███╔███╔╝██║  ██║██║  ██║██║ ╚████║╚██████╔╝███████╗███████╗██║  ██║
  ╚═════╝ ╚═╝   ╚═╝        ╚══╝╚══╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝ ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝`

var rootBannerGradient = []string{
	"\033[38;2;0;245;255m",
	"\033[38;2;0;190;255m",
	"\033[38;2;87;132;255m",
	"\033[38;2;148;93;255m",
	"\033[38;2;214;84;255m",
	"\033[38;2;255;91;184m",
}

func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return execute(ctx, run.New(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

func ExecuteWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return execute(context.Background(), run.New(), args, stdin, stdout, stderr)
}

func ExecuteWithRunner(ctx context.Context, runner run.Runner, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return execute(ctx, runner, args, stdin, stdout, stderr)
}

func newApp(ctx context.Context, runner run.Runner, stdin io.Reader, stdout, stderr io.Writer) *app {
	if ctx == nil {
		ctx = context.Background()
	}
	if runner == nil {
		runner = run.New()
	}
	return &app{
		ctx:    ctx,
		stdout: stdout,
		stderr: stderr,
		stdin:  stdin,
		input:  bufio.NewReader(stdin),
		ui:     ui.New(stdout),
		runner: runner,
		git:    git.New(runner),
		gh:     githubcli.New(runner),
		creds:  credentials.NewKeyringStore(),
		auth:   auth.NewGitHubDeviceAuthenticator(),
	}
}

func execute(ctx context.Context, runner run.Runner, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	a := newApp(ctx, runner, stdin, stdout, stderr)
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
			printRootLanding(a)
			return nil
		},
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.CompletionOptions.DisableDescriptions = false
	root.AddGroup(
		&cobra.Group{ID: "remote", Title: "Remote Operations:"},
		&cobra.Group{ID: "local", Title: "Local Operations:"},
		&cobra.Group{ID: "ai", Title: "AI Commands:"},
		&cobra.Group{ID: "history", Title: "History Rewriting:"},
		&cobra.Group{ID: "utility", Title: "Utility:"},
	)
	root.SetHelpCommandGroupID("utility")
	root.SetCompletionCommandGroupID("utility")

	root.AddCommand(
		command(a, "clone", "Clone multiple GitHub repositories for a user.", "remote", runClone, flags{
			stringFlag("visibility", "all", "Repository visibility: all, public, or private."),
			stringFlag("user", "", "GitHub user or organization to clone from."),
			intFlag("limit", 100, "Maximum repositories to list."),
			stringFlag("into", "", "Directory to clone repositories into."),
		}),
		command(a, "pull", "Pull the latest changes for every discovered repository.", "remote", runPull, flags{
			boolFlag("rebase", "Rebase local commits while pulling."),
			boolFlag("force", "Pass --force to git pull."),
		}),
		command(a, "push", "Push local commits to origin HEAD.", "remote", runPush, flags{
			boolFlag("force", "Use --force-with-lease."),
			boolFlag("force-unsafe", "Use raw --force after confirmation."),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "rename-repo", "Rename GitHub repositories with gh.", "remote", runRenameRepo, flags{
			boolFlag("description", "Prompt for repository description updates."),
		}),
		command(a, "commit", "Stage all changes and create a commit in every repository.", "local", runCommit, flags{
			stringFlag("message", "", "Commit message."),
		}),
		command(a, "commit-ai", "Generate and create one Conventional Commit per changed repository.", "ai", runCommitAI, flags{
			intFlag("max-chars-per-commit", 3000, "Maximum redacted context characters per commit."),
			intFlag("rpm", 300, "Maximum API requests to start per minute."),
			intFlag("timeout", 90, "API timeout in seconds."),
			boolFlag("body", "Generate commit message bodies."),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "fix-gitignore", "Add missing common generated-file patterns to .gitignore.", "local", runFixGitignore, flags{
			boolFlag("yes", "Skip confirmation prompts."),
		}),
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
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "review", "Review unpushed changes across repositories.", "local", runReview, nil),
		command(a, "untrack", "Stop tracking files already covered by .gitignore.", "local", runUntrack, flags{
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "remove-secrets", "Purge sensitive files from Git history.", "history", runRemoveSecrets, flags{
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "rewrite-authors", "Rewrite author and committer identity.", "history", runRewriteAuthors, flags{
			stringFlag("name", "", "New author and committer name."),
			stringFlag("email", "", "New author and committer email."),
			stringFlag("repo", "", "Repository directory to target."),
			boolFlag("force", "Pass --force to git-filter-repo."),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "rewrite-commits", "Rewrite commit messages to Conventional Commits.", "history", runRewriteCommits, flags{
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "rewrite-commits-ai", "Generate Conventional Commit messages with an OpenAI-compatible endpoint.", "ai", runRewriteCommitsAI, flags{
			intFlag("batch-size", 10, "Commits per API request."),
			intFlag("max-chars-per-commit", 3000, "Maximum redacted context characters per commit."),
			intFlag("rpm", 300, "Maximum API requests to start per minute."),
			intFlag("timeout", 90, "API timeout in seconds."),
			boolFlag("skip-conventional", "Skip commits that already use Conventional Commits."),
			boolFlag("body", "Generate commit message bodies."),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "rewrite-dates", "Redistribute commit timestamps.", "history", runRewriteDates, flags{
			stringFlag("start-date", "", "Earliest date in YYYY-MM-DD format."),
			stringFlag("end-date", "", "Latest date in YYYY-MM-DD format."),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "info", "Show detailed repository information.", "utility", runInfo, flags{
			stringFlag("repo", "", "Repository directory to target."),
		}),
		command(a, "doctor", "Check Git Wrangler runtime dependencies.", "utility", runDoctor, nil),
		initCommand(a),
		configCommand(a),
		command(a, "status", "Show clean, dirty, and tracking state.", "utility", runStatus, nil),
		versionCommand(a),
	)
	return root
}

func printRootLanding(a *app) {
	printRootBanner(a)
	fmt.Fprintln(a.stdout, "Orchestrate Git operations across many repositories.")
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Common commands:")
	fmt.Fprintln(a.stdout, "  git-wrangler status          Show repository state")
	fmt.Fprintln(a.stdout, "  git-wrangler pull --rebase   Refresh every repo")
	fmt.Fprintln(a.stdout, "  git-wrangler review          Review unpushed work")
	fmt.Fprintln(a.stdout, "  git-wrangler help            Show all commands")
}

func printRootBanner(a *app) {
	if a.ui.Reset == "" {
		fmt.Fprintf(a.stdout, "%s\n\n", rootBanner)
		return
	}
	for i, line := range strings.Split(rootBanner, "\n") {
		color := rootBannerGradient[i%len(rootBannerGradient)]
		fmt.Fprintf(a.stdout, "%s%s%s%s\n", a.ui.Bold, color, line, a.ui.Reset)
	}
	fmt.Fprintln(a.stdout)
}

func command(a *app, use, short, group string, runFn func(*app, *cobra.Command, []string) int, specs flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     use,
		Short:   short,
		GroupID: group,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := runFn(a, cmd, args); code != 0 {
				return exitError{code: code}
			}
			return nil
		},
	}
	for _, spec := range specs {
		switch spec.kind {
		case "bool":
			cmd.Flags().Bool(spec.name, false, spec.description)
		case "int":
			cmd.Flags().Int(spec.name, spec.intValue, spec.description)
		default:
			cmd.Flags().String(spec.name, spec.stringValue, spec.description)
		}
	}
	return cmd
}

func versionCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print version metadata.",
		GroupID: "utility",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(a.stdout, version.Full())
		},
	}
}
