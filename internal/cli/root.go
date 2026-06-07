package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
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
	ctx          context.Context
	stdout       io.Writer
	stderr       io.Writer
	stdin        io.Reader
	prompts      *promptSession
	ui           ui.Theme
	runner       run.Runner
	git          git.Client
	gh           githubcli.Client
	creds        credentials.Store
	auth         auth.GitHubAuthenticator
	json         bool
	promptFailed bool
	cancelOnce   sync.Once
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
	appCtx, cancel := context.WithCancel(ctx)
	a := &app{
		ctx:    appCtx,
		stdout: stdout,
		stderr: stderr,
		stdin:  stdin,
		ui:     ui.New(stdout),
		runner: runner,
		git:    git.New(runner),
		gh:     githubcli.New(runner),
		creds:  credentials.NewKeyringStore(),
		auth:   auth.NewGitHubDeviceAuthenticator(),
	}
	a.prompts = newPromptSession(appCtx, cancel, stdin, stderr)
	return a
}

func execute(ctx context.Context, runner run.Runner, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	a := newApp(ctx, runner, stdin, stdout, stderr)
	root := newRootCommand(a)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		if errors.Is(err, errPromptCancelled) || a.ctx.Err() != nil {
			renderCancellation(a)
		}
		if _, ok := err.(exitError); !ok {
			if errors.Is(err, errPromptCancelled) {
				return err
			}
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
		&cobra.Group{ID: "history", Title: "History Rewriting:"},
		&cobra.Group{ID: "utility", Title: "Utility:"},
	)
	root.SetHelpCommandGroupID("utility")
	root.SetCompletionCommandGroupID("utility")

	root.AddCommand(
		command(a, "activity", "Show an aggregated commit activity calendar.", "utility", runActivity, flags{
			repoFlag(),
			intFlag("year", 0, "Only include UTC author dates in YYYY."),
			stringArrayFlag("user", "Only include an exact author name or email. Repeatable."),
			boolFlag("all", "Include commits reachable from all normal refs."),
			boolFlag("global-scale", "Use one activity scale across all rendered years."),
		}),
		command(a, "clone", "Clone multiple GitHub repositories for a user.", "remote", runClone, flags{
			stringFlag("visibility", "all", "Repository visibility: all, public, or private."),
			stringFlag("user", "", "GitHub user or organization to clone from."),
			intFlag("limit", 100, "Maximum repositories to list."),
			stringFlag("into", "", "Directory to clone repositories into."),
		}),
		command(a, "pull", "Pull the latest changes for target repositories.", "remote", runPull, flags{
			repoFlag(),
			boolFlag("rebase", "Rebase local commits while pulling."),
			boolFlag("force", "Pass --force to git pull."),
		}),
		command(a, "fetch", "Fetch origin updates for target repositories.", "remote", runFetch, flags{
			repoFlag(),
			boolFlag("prune", "Prune remote-tracking branches that no longer exist on origin."),
		}),
		command(a, "push", "Push local commits to origin HEAD.", "remote", runPush, flags{
			repoFlag(),
			boolFlag("force", "Use --force-with-lease."),
			boolFlag("force-unsafe", "Use raw --force after confirmation."),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "rename-repo", "Rename GitHub repositories with gh.", "remote", runRenameRepo, flags{
			repoFlag(),
			boolFlag("description", "Prompt for repository description updates."),
		}),
		command(a, "commit", "Generate and create one Conventional Commit per changed repository.", "local", runCommit, flags{
			repoFlag(),
			intFlag("max-chars-per-commit", 3000, "Maximum redacted context characters per commit."),
			intFlag("rpm", 300, "Maximum API requests to start per minute."),
			intFlag("timeout", 90, "API timeout in seconds."),
			boolFlag("body", "Generate commit message bodies."),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "fix-gitignore", "Add missing common generated-file patterns to .gitignore.", "local", runFixGitignore, flags{
			repoFlag(),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "license", "Add or replace MIT LICENSE files.", "local", runLicense, flags{
			repoFlag(),
			stringFlag("name", "", "Copyright holder name."),
			boolFlag("overwrite", "Replace an existing LICENSE file."),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "rename-branch", "Rename a branch across repositories.", "local", runRenameBranch, flags{
			repoFlag(),
			stringFlag("oldbranch", "", "Existing branch name."),
			stringFlag("newbranch", "", "New branch name."),
		}),
		command(a, "reset", "Reset current branches to their origin counterparts.", "local", runReset, flags{
			repoFlag(),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "review", "Review unpushed changes across repositories.", "local", runReview, flags{
			repoFlag(),
			jsonFlag(),
			noFetchFlag(),
		}),
		command(a, "untrack", "Stop tracking files already covered by .gitignore.", "local", runUntrack, flags{
			repoFlag(),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "remove-secrets", "Purge sensitive files from Git history.", "history", runRemoveSecrets, flags{
			repoFlag(),
			noFetchFlag(),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "rewrite-authors", "Rewrite author and committer identity.", "history", runRewriteAuthors, flags{
			stringFlag("name", "", "New author and committer name."),
			stringFlag("email", "", "New author and committer email."),
			repoFlag(),
			noFetchFlag(),
			boolFlag("force", "Pass --force to git-filter-repo."),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "rewrite-commits", "Generate Conventional Commit messages with an OpenAI-compatible endpoint.", "history", runRewriteCommits, flags{
			repoFlag(),
			noFetchFlag(),
			intFlag("batch-size", 10, "Commits per API request."),
			intFlag("max-chars-per-commit", 3000, "Maximum redacted context characters per commit."),
			intFlag("rpm", 300, "Maximum API requests to start per minute."),
			intFlag("timeout", 90, "API timeout in seconds."),
			boolFlag("skip-conventional", "Skip commits that already use Conventional Commits."),
			boolFlag("body", "Generate commit message bodies."),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "rewrite-dates", "Redistribute commit timestamps.", "history", runRewriteDates, flags{
			repoFlag(),
			noFetchFlag(),
			stringFlag("start-date", "", "Earliest target date in YYYY-MM-DD format."),
			stringFlag("end-date", "", "Latest target date in YYYY-MM-DD format."),
			stringFlag("rewrite-before", "", "Rewrite commits with original author dates before YYYY-MM-DD."),
			stringFlag("rewrite-after", "", "Rewrite commits with original author dates on or after YYYY-MM-DD."),
			intFlag("days", 0, "Target the last N days."),
			stringFlag("until", "", "End date for --days in YYYY-MM-DD format. Defaults to today."),
			stringFlag("seed", "", "Deterministic planner seed."),
			stringFlag("intensity", "medium", "Planning intensity: low, medium, or high."),
			boolFlag("rollback", "Restore known history from stored rewrite state."),
			boolFlag("yes", "Skip confirmation prompts."),
		}),
		command(a, "info", "Show detailed repository information.", "utility", runInfo, flags{
			repoFlag(),
			jsonFlag(),
			noFetchFlag(),
		}),
		command(a, "doctor", "Check Git Wrangler runtime dependencies.", "utility", runDoctor, flags{
			jsonFlag(),
		}),
		initCommand(a),
		configCommand(a),
		command(a, "status", "Show clean, dirty, and tracking state.", "utility", runStatus, flags{
			repoFlag(),
			jsonFlag(),
			noFetchFlag(),
		}),
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

type jsonError struct {
	Message string `json:"message"`
}

func writeJSON(a *app, payload any) int {
	enc := json.NewEncoder(a.stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		fmt.Fprintf(a.stderr, "Error: %s\n", err)
		return 1
	}
	return 0
}

func writeJSONStatus(a *app, payload any, code int) int {
	if errCode := writeJSON(a, payload); errCode != 0 {
		return errCode
	}
	return code
}

func command(a *app, use, short, group string, runFn func(*app, *cobra.Command, []string) int, specs flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     use,
		Short:   short,
		GroupID: group,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a.json = jsonFlagValue(cmd)
			a.promptFailed = false
			if err := runGuidedSetup(a, cmd); err != nil {
				if errors.Is(err, errPromptCancelled) {
					return commandExitError(a, 1)
				}
				return err
			}
			if code := runFn(a, cmd, args); code != 0 {
				return commandExitError(a, code)
			}
			if a.promptFailed {
				return commandExitError(a, 1)
			}
			return nil
		},
	}
	for _, spec := range specs {
		switch spec.kind {
		case "bool":
			if spec.name == "yes" {
				cmd.Flags().BoolP(spec.name, "y", false, spec.description)
			} else {
				cmd.Flags().Bool(spec.name, false, spec.description)
			}
		case "int":
			cmd.Flags().Int(spec.name, spec.intValue, spec.description)
		case "stringArray":
			cmd.Flags().StringArray(spec.name, nil, spec.description)
		default:
			cmd.Flags().String(spec.name, spec.stringValue, spec.description)
		}
	}
	if _, ok := guidedPrompts[cmd.Name()]; ok || cmd.Name() == "rewrite-dates" {
		cmd.Flags().Bool("guided", false, "Interactively configure command options.")
	}
	return cmd
}

func commandExitError(a *app, code int) error {
	if a != nil && a.ctx.Err() != nil {
		renderCancellation(a)
	}
	return exitError{code: code}
}

func versionCommand(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Short:   "Print version metadata.",
		GroupID: "utility",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			a.json = jsonFlagValue(cmd)
			if a.json {
				_ = writeJSON(a, map[string]any{
					"ok":      true,
					"summary": map[string]any{"version": version.Version},
					"version": version.Version,
					"commit":  version.Commit,
					"date":    version.Date,
				})
				return
			}
			fmt.Fprintln(a.stdout, version.Full())
		},
	}
	cmd.Flags().Bool("json", false, "Emit one JSON document.")
	return cmd
}
