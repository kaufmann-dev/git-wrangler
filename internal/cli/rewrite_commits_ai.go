package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/ai"
	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
	"github.com/spf13/cobra"
)

func runRewriteCommitsAI(a *app, cmd *cobra.Command, args []string) int {
	batch, _ := cmd.Flags().GetInt("batch-size")
	maxCharsInt, _ := cmd.Flags().GetInt("max-chars-per-commit")
	timeoutInt, _ := cmd.Flags().GetInt("timeout")
	skipConventional, _ := cmd.Flags().GetBool("skip-conventional")
	yes := yesFlag(cmd)

	if batch <= 0 {
		a.plainErrorf("--batch-size must be a positive integer.")
		return 1
	}
	if batch > 50 {
		a.plainErrorf("--batch-size must be 50 or less.")
		return 1
	}
	if maxCharsInt <= 0 {
		a.plainErrorf("--max-chars-per-commit must be a positive integer.")
		return 1
	}
	if timeoutInt <= 0 {
		a.plainErrorf("--timeout must be a positive integer.")
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	if cfg.AI.BaseURL == "" {
		a.plainErrorf("AI base URL is required. Run 'git-wrangler config set ai.base-url <url>'.")
		return 1
	}
	if cfg.AI.Model == "" {
		a.plainErrorf("AI model is required. Run 'git-wrangler config set ai.model <model>'.")
		return 1
	}
	apiKey := credentials.ResolveAIKey(a.creds, cfg.AI.Provider)
	if apiKey.Err != nil {
		a.plainErrorf("AI API key could not be read: %s", apiKey.Err.Error())
		return 1
	}
	if apiKey.Value == "" {
		a.plainErrorf("AI API key is required. Run 'git-wrangler config set ai.api-key' or set GIT_WRANGLER_AI_API_KEY.")
		return 1
	}

	if !requireGit(a, "rewrite-commits-ai") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-commits-ai")
	if !ok {
		return 1
	}

	repos, err := resolveRepositoryTargets("")
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
	plan, err := ai.Generate(a.ctx, aiRepos, ai.Config{
		BaseURL:           cfg.AI.BaseURL,
		Model:             cfg.AI.Model,
		APIKey:            apiKey.Value,
		BatchSize:         batch,
		MaxCharsPerCommit: maxCharsInt,
		Timeout:           time.Duration(timeoutInt) * time.Second,
		SkipConventional:  skipConventional,
		WorkDir:           workDir,
		Git:               a.git,
	}, a.stderr, func(question string) bool {
		if yes {
			return true
		}
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
	if !yes && !confirm(a, "Apply these generated commit messages to all listed repositories?") {
		fmt.Fprintf(a.stdout, "%sRewrite cancelled. Generated AI messages were temporary and have been discarded.%s\n", a.ui.Yellow, a.ui.Reset)
		return 1
	}
	return applyAIPlan(a, plan, filterCmd)
}

func applyAIPlan(a *app, plan *ai.Plan, filterCmd []string) int {
	hadError := false
	for _, repoPlan := range plan.Repos {
		out, err, restoreErr := runFilterRepoRestoringOrigin(a, repoPlan.Dir, filterCmd, []string{"--partial", "--commit-callback", repoPlan.CallbackFile, "--force"}, nil)
		if err == nil {
			if restoreErr != nil {
				fmt.Fprintf(a.stderr, "%sWarning: Commit rewrite completed for %s, but origin could not be restored:\n%s%s\n\n", a.ui.Red, repoPlan.Name, restoreErr.Error(), a.ui.Reset)
				hadError = true
				continue
			}
			fmt.Fprintf(a.stdout, "%sRewrote %d commit message(s) for %s%s\n", a.ui.Green, repoPlan.ChangedCount, repoPlan.Name, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not rewrite commit messages for %s:\n%s%s\n\n", a.ui.Red, repoPlan.Name, out, a.ui.Reset)
			if restoreErr != nil {
				fmt.Fprintf(a.stderr, "%sWarning: Commit rewrite failed for %s, and origin could not be restored:\n%s%s\n\n", a.ui.Red, repoPlan.Name, restoreErr.Error(), a.ui.Reset)
			}
			hadError = true
		}
	}
	if hadError {
		return 1
	}
	return 0
}
