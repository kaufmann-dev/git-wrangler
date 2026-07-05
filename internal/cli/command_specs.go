package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/version"
	"github.com/spf13/cobra"
)

type commandRunFunc func(*app, *cobra.Command, []string) int

type commandSpec struct {
	use      string
	short    string
	group    string
	args     cobra.PositionalArgs
	run      commandRunFunc
	flags    flags
	guided   guidedSpec
	children []commandSpec
	helpOnly bool
}

type guidedSpec struct {
	prompts []guidedPrompt
	summary []guidedPrompt
	setup   guidedSetupFunc
}

type guidedSetupFunc func(*app, *cobra.Command) error

func rootCommands(a *app) []*cobra.Command {
	specs := commandSpecs()
	commands := make([]*cobra.Command, 0, len(specs))
	for _, spec := range specs {
		commands = append(commands, command(a, spec))
	}
	return commands
}

func commandSpecs() []commandSpec {
	return []commandSpec{
		{
			use:   "activity",
			short: "Show an aggregated commit activity calendar.",
			group: "utility",
			run:   runActivity,
			flags: joinFlags(targetFlags(), flags{
				intFlag("year", 0, "Only include UTC author dates in YYYY."),
				stringArrayFlag("user", "Only include an exact author name or email. Repeatable."),
				boolFlag("all", "Include commits reachable from all normal refs."),
				boolFlag("global-scale", "Use one activity scale across all rendered years."),
			}),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedString("year", "Year"), guidedRepeatable("user", "Author filters"), guidedBool("all", "Include all refs"), guidedBool("global-scale", "Use global scale")}},
		},
		{
			use:   "log",
			short: "Show compact Conventional Commit-aware history.",
			group: "utility",
			run:   runLog,
			flags: joinFlags(targetFlags(), flags{
				intFlag("limit", 50, "Maximum commits to display after merging repositories; 0 means unlimited."),
				stringFlag("since", "", "Include commits with author date on or after YYYY-MM-DD."),
				stringFlag("until", "", "Include commits with author date on or before YYYY-MM-DD."),
				stringArrayFlag("type", "Only include a Conventional Commit type or other. Repeatable."),
				stringArrayFlag("scope", "Only include an exact Conventional Commit scope. Repeatable."),
				boolFlag("summary", "Print compact history counts before entries."),
			}),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedNonNegativeInt("limit", "Commit limit"), guidedString("since", "Author date on or after"), guidedString("until", "Author date on or before"), guidedRepeatable("type", "Types"), guidedRepeatable("scope", "Scopes"), guidedBool("summary", "Print summary")}},
		},
		{
			use:   "clone",
			short: "Clone multiple GitHub repositories for a user.",
			group: "remote",
			run:   runClone,
			flags: flags{
				stringFlag("visibility", "all", "Repository visibility: all, public, or private."),
				stringFlag("user", "", "GitHub user or organization to clone from."),
				intFlag("limit", 100, "Maximum repositories to list."),
				stringFlag("into", "", "Directory to clone repositories into."),
			},
			guided: guidedSpec{
				prompts: []guidedPrompt{guidedString("user", "GitHub user or organization"), guidedEnum("visibility", "Visibility", "all", "public", "private"), guidedPositiveInt("limit", "Repository limit"), guidedString("into", "Destination directory")},
				setup:   guideClone,
			},
		},
		{
			use:   "pull",
			short: "Pull the latest changes for target repositories.",
			group: "remote",
			run:   runPull,
			flags: joinFlags(targetFlags(), flags{
				boolFlag("rebase", "Rebase local commits while pulling."),
			}),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedBool("rebase", "Rebase while pulling")}},
		},
		{
			use:   "fetch",
			short: "Fetch origin updates for target repositories.",
			group: "remote",
			run:   runFetch,
			flags: joinFlags(targetFlags(), flags{
				boolFlag("prune", "Prune remote-tracking branches that no longer exist on origin."),
			}),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedBool("prune", "Prune removed origin branches")}},
		},
		{
			use:   "push",
			short: "Push local commits to origin HEAD.",
			group: "remote",
			run:   runPush,
			flags: joinFlags(targetFlags(), flags{
				boolFlag("force", "Use --force-with-lease."),
				boolFlag("force-unsafe", "Use raw --force after confirmation."),
			}, confirmationFlags()),
			guided: guidedSpec{
				prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedBool("force", "Force with lease"), guidedBool("force-unsafe", "Raw force")},
				setup:   guidePush,
			},
		},
		{
			use:   "rename-repo",
			short: "Rename GitHub repositories with gh.",
			group: "remote",
			run:   runRenameRepo,
			flags: joinFlags(targetFlags(), flags{
				boolFlag("description", "Prompt for repository description updates."),
			}),
		},
		{
			use:   "commit",
			short: "Generate and create one Conventional Commit per changed repository.",
			group: "local",
			run:   runCommit,
			flags: joinFlags(targetFlags(), aiRequestFlags(), flags{
				boolFlag("body", "Generate commit message bodies."),
			}, confirmationFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedPositiveInt("rpm", "Requests per minute"), guidedPositiveInt("concurrency", "Concurrent API requests"), guidedPositiveInt("timeout", "Timeout seconds"), guidedBool("body", "Generate message bodies")}},
		},
		{
			use:    "fix-gitignore",
			short:  "Add missing common generated-file patterns to .gitignore.",
			group:  "local",
			run:    runFixGitignore,
			flags:  joinFlags(targetFlags(), confirmationFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository")}},
		},
		{
			use:   "license",
			short: "Add or replace LICENSE files.",
			group: "local",
			run:   runLicense,
			flags: joinFlags(targetFlags(), flags{
				stringFlag("type", "", "License type ID."),
				intFlag("year", time.Now().Year(), "Copyright year."),
				stringFlag("name", "", "Copyright holder name."),
				boolFlag("overwrite", "Replace an existing LICENSE file."),
			}, confirmationFlags()),
			guided: guidedSpec{
				prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedEnum("type", "License type", supportedLicenseIDs()...), guidedPositiveInt("year", "Copyright year"), guidedString("name", "Copyright holder name"), guidedBool("overwrite", "Overwrite existing licenses")},
				setup:   guideLicense,
			},
		},
		{
			use:   "rename-branch",
			short: "Rename a branch across repositories.",
			group: "local",
			run:   runRenameBranch,
			flags: joinFlags(targetFlags(), flags{
				stringFlag("oldbranch", "", "Existing branch name."),
				stringFlag("newbranch", "", "New branch name."),
			}, confirmationFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedRequiredString("oldbranch", "Existing branch name"), guidedRequiredString("newbranch", "New branch name")}},
		},
		{
			use:    "reset",
			short:  "Reset current branches to their origin counterparts.",
			group:  "local",
			run:    runReset,
			flags:  joinFlags(targetFlags(), confirmationFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository")}},
		},
		{
			use:    "review",
			short:  "Review unpushed changes across repositories.",
			group:  "local",
			run:    runReview,
			flags:  joinFlags(targetFlags(), jsonFlags(), fetchControlFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedBool("no-fetch", "Skip origin fetch")}},
		},
		{
			use:    "untrack",
			short:  "Stop tracking files already covered by .gitignore.",
			group:  "local",
			run:    runUntrack,
			flags:  joinFlags(targetFlags(), confirmationFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository")}},
		},
		{
			use:    "remove-secrets",
			short:  "Purge sensitive files from Git history.",
			group:  "history",
			run:    runRemoveSecrets,
			flags:  joinFlags(targetFlags(), fetchControlFlags(), confirmationFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedBool("no-fetch", "Skip origin fetch")}},
		},
		{
			use:   "rewrite-authors",
			short: "Rewrite author and committer identity.",
			group: "history",
			run:   runRewriteAuthors,
			flags: joinFlags(flags{
				stringFlag("name", "", "New author and committer name."),
				stringFlag("email", "", "New author and committer email."),
			}, targetFlags(), fetchControlFlags(), rewriteDateBoundFlags(), confirmationFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedRequiredString("name", "New author and committer name"), guidedRequiredString("email", "New author and committer email"), guidedBool("no-fetch", "Skip origin fetch"), guidedString("rewrite-after", "Current author date on or after"), guidedString("rewrite-before", "Current author date before")}},
		},
		{
			use:   "rewrite-commits",
			short: "Generate Conventional Commit messages with an OpenAI-compatible endpoint.",
			group: "history",
			run:   runRewriteCommits,
			flags: joinFlags(targetFlags(), fetchControlFlags(), rewriteDateBoundFlags(), flags{
				intFlag("batch-size", 10, "Maximum commits per API request."),
			}, aiRequestFlags(), flags{
				boolFlag("skip-conventional", "Skip commits that already use Conventional Commits."),
				boolFlag("require-scope", "Skip only Conventional Commits that also have a scope; implies --skip-conventional."),
				boolFlag("body", "Generate commit message bodies."),
			}, confirmationFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedBool("no-fetch", "Skip origin fetch"), guidedString("rewrite-after", "Current author date on or after"), guidedString("rewrite-before", "Current author date before"), guidedPositiveInt("batch-size", "Maximum commits per API request"), guidedPositiveInt("rpm", "Requests per minute"), guidedPositiveInt("concurrency", "Concurrent API requests"), guidedPositiveInt("timeout", "Timeout seconds"), guidedBool("skip-conventional", "Skip conventional commits"), guidedBool("require-scope", "Skip only scoped conventional commits"), guidedBool("body", "Generate message bodies")}},
		},
		{
			use:   "rewrite-hours",
			short: "Move commit timestamps into a uniform daily time window.",
			group: "history",
			run:   runRewriteHours,
			flags: joinFlags(targetFlags(), fetchControlFlags(), rewriteDateBoundFlags(), flags{
				stringFlag("window", "", "Required same-day time window or day-of-week schedule."),
			}, confirmationFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedBool("no-fetch", "Skip origin fetch"), guidedString("rewrite-after", "Current author date on or after"), guidedString("rewrite-before", "Current author date before"), guidedRequiredString("window", "Time window")}},
		},
		{
			use:   "rewrite-dates",
			short: "Redistribute commit timestamps.",
			group: "history",
			run:   runRewriteDates,
			flags: joinFlags(targetFlags(), fetchControlFlags(), flags{
				stringFlag("start-date", "", "Earliest target date in YYYY-MM-DD format."),
				stringFlag("end-date", "", "Latest target date in YYYY-MM-DD format."),
				stringFlag("rewrite-before", "", "Rewrite commits with current author dates before YYYY-MM-DD."),
				stringFlag("rewrite-after", "", "Rewrite commits with current author dates on or after YYYY-MM-DD."),
				intFlag("days", 0, "Target the last N days."),
				stringFlag("until", "", "End date for --days in YYYY-MM-DD format. Defaults to today."),
				stringFlag("seed", "", "Deterministic planner seed."),
				stringFlag("frequency", "medium", "Planning frequency: low, medium, or high."),
				stringFlag("spread", "medium", "Planning spread: low, medium, or high."),
				stringFlag("window", "", "Use one same-day time window or day-of-week schedule."),
			}, confirmationFlags()),
			guided: guidedSpec{
				prompts: rewriteDatesGuidedSummaryPrompts(),
				setup:   guideRewriteDates,
			},
		},
		{
			use:    "rollback-rewrites",
			short:  "Roll back Git Wrangler history rewrites from the shared baseline.",
			group:  "history",
			run:    runRollbackRewrites,
			flags:  joinFlags(targetFlags(), confirmationFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository")}},
		},
		{
			use:    "info",
			short:  "Show detailed repository information.",
			group:  "utility",
			run:    runInfo,
			flags:  joinFlags(targetFlags(), jsonFlags(), fetchControlFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedBool("no-fetch", "Skip origin fetch")}},
		},
		{
			use:   "doctor",
			short: "Check Git Wrangler runtime dependencies.",
			group: "utility",
			run:   runDoctor,
			flags: jsonFlags(),
		},
		{
			use:   "init",
			short: "Set up GitHub and AI credentials.",
			group: "utility",
			run:   runInitCommand,
		},
		{
			use:      "config",
			short:    "Show and edit Git Wrangler setup.",
			group:    "utility",
			helpOnly: true,
			children: []commandSpec{
				{
					use:   "show",
					short: "Show non-secret configuration and credential sources.",
					run:   runConfigShowCommand,
					flags: jsonFlags(),
				},
				{
					use:   "set <key> [value]",
					short: "Set a configuration value.",
					args:  configSetArgs,
					run:   runConfigSetCommand,
				},
				{
					use:   "unset <key>",
					short: "Unset a stored credential.",
					args:  cobra.ExactArgs(1),
					run:   runConfigUnsetCommand,
				},
				{
					use:      "file",
					short:    "Show and edit file-backed configuration.",
					helpOnly: true,
					children: []commandSpec{
						{
							use:      "remove-secrets",
							short:    "Show and edit remove-secrets path globs.",
							helpOnly: true,
							children: []commandSpec{
								{
									use:   "path",
									short: "Print the remove-secrets config file path.",
									run:   runConfigFileRemoveSecretsPathCommand,
								},
								{
									use:   "show",
									short: "Show the configured remove-secrets path globs.",
									run:   runConfigFileRemoveSecretsShowCommand,
								},
								{
									use:   "edit",
									short: "Edit the remove-secrets path globs.",
									run:   runConfigFileRemoveSecretsEditCommand,
								},
							},
						},
					},
				},
			},
		},
		{
			use:    "status",
			short:  "Show clean, dirty, and tracking state.",
			group:  "utility",
			run:    runStatus,
			flags:  joinFlags(targetFlags(), jsonFlags(), fetchControlFlags()),
			guided: guidedSpec{prompts: []guidedPrompt{guidedString("repo", "Repository"), guidedBool("no-fetch", "Skip origin fetch")}},
		},
		{
			use:   "version",
			short: "Print version metadata.",
			group: "utility",
			run:   runVersion,
			flags: jsonFlags(),
		},
	}
}

func command(a *app, spec commandSpec) *cobra.Command {
	args := spec.args
	if args == nil {
		args = cobra.NoArgs
	}
	cmd := &cobra.Command{
		Use:     spec.use,
		Short:   spec.short,
		GroupID: spec.group,
		Args:    args,
		RunE: func(cmd *cobra.Command, args []string) error {
			a.json = jsonOptionsFromCommand(cmd).enabled
			a.promptFailed = false
			if err := runGuidedSetup(a, cmd, spec.guided); err != nil {
				if errors.Is(err, errPromptCancelled) {
					return commandExitError(a, 1)
				}
				return err
			}
			if spec.helpOnly {
				return cmd.Help()
			}
			if code := spec.run(a, cmd, args); code != 0 {
				return commandExitError(a, code)
			}
			if a.promptFailed {
				return commandExitError(a, 1)
			}
			return nil
		},
	}
	for _, flag := range spec.flags {
		switch flag.kind {
		case flagKindBool:
			if flag.shorthand != "" {
				cmd.Flags().BoolP(flag.name, flag.shorthand, false, flag.description)
			} else {
				cmd.Flags().Bool(flag.name, false, flag.description)
			}
		case flagKindInt:
			cmd.Flags().Int(flag.name, flag.intValue, flag.description)
		case flagKindStringArray:
			cmd.Flags().StringArray(flag.name, nil, flag.description)
		default:
			cmd.Flags().String(flag.name, flag.stringValue, flag.description)
		}
	}
	if spec.guided.enabled() {
		cmd.Flags().Bool("guided", false, "Interactively configure command options.")
	}
	for _, child := range spec.children {
		cmd.AddCommand(command(a, child))
	}
	return cmd
}

func (spec guidedSpec) enabled() bool {
	return len(spec.prompts) > 0 || len(spec.summary) > 0 || spec.setup != nil
}

func runVersion(a *app, cmd *cobra.Command, args []string) int {
	opts := versionOptionsFromCommand(cmd)
	if opts.json.enabled {
		return writeJSON(a, map[string]any{
			"ok":      true,
			"summary": map[string]any{"version": version.Version},
			"version": version.Version,
			"commit":  version.Commit,
			"date":    version.Date,
		})
	}
	fmt.Fprintln(a.stdout, version.Full())
	return 0
}
