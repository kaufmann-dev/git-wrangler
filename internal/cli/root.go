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
	root.AddCommand(rootCommands(a)...)
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

func commandExitError(a *app, code int) error {
	if a != nil && a.ctx.Err() != nil {
		renderCancellation(a)
	}
	return exitError{code: code}
}
