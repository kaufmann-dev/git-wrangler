package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/ai"
	"github.com/kaufmann-dev/git-wrangler/internal/ui"
	"github.com/spf13/cobra"
)

func runRewriteCommits(a *app, cmd *cobra.Command, args []string) int {
	batch, _ := cmd.Flags().GetInt("batch-size")
	maxCharsInt, _ := cmd.Flags().GetInt("max-chars-per-commit")
	rpm, _ := cmd.Flags().GetInt("rpm")
	timeoutInt, _ := cmd.Flags().GetInt("timeout")
	skipConventional, _ := cmd.Flags().GetBool("skip-conventional")
	body, _ := cmd.Flags().GetBool("body")
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
	if rpm <= 0 {
		a.plainErrorf("--rpm must be a positive integer.")
		return 1
	}
	if timeoutInt <= 0 {
		a.plainErrorf("--timeout must be a positive integer.")
		return 1
	}

	settings, ok := loadAISettings(a)
	if !ok {
		return 1
	}

	if !requireGit(a, "rewrite-commits") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-commits")
	if !ok {
		return 1
	}

	repos, err := commandRepositoryTargets(cmd)
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
	scanProgress := newProgress(a, "Scanning repositories", len(repos))
	apiProgress := (*progress)(nil)
	plan, err := ai.Generate(a.ctx, aiRepos, ai.Config{
		BaseURL:           settings.Config.AI.BaseURL,
		Model:             settings.Config.AI.Model,
		APIKey:            settings.APIKey,
		BatchSize:         batch,
		MaxCharsPerCommit: maxCharsInt,
		RPM:               rpm,
		Timeout:           time.Duration(timeoutInt) * time.Second,
		SkipConventional:  skipConventional,
		Body:              body,
		WorkDir:           workDir,
		Git:               a.git,
		Progress: func(event ai.ProgressEvent) {
			switch event.Phase {
			case "Sending API requests":
				updateAIRequestProgress(a, &apiProgress, event)
			case "Scanning repositories":
				if event.Total <= 1 {
					return
				}
				if event.Current == 0 {
					scanProgress.startWork(progressEventKey(event), progressEventDetail(event))
					return
				}
				scanProgress.finish(progressEventKey(event), progressEventDetail(event))
			case "Scanning commits":
				if event.Total <= 1 {
					return
				}
				if event.Current == 0 {
					return
				}
				scanProgress.update(progressEventKey(event), progressEventDetail(event))
			default:
				if event.Total <= 1 {
					return
				}
				if event.Current == 0 {
					return
				}
				scanProgress.message(fmt.Sprintf("%s %d/%d", event.Phase, event.Current, event.Total))
			}
		},
	}, a.stderr, func(question string) bool {
		if yes {
			return true
		}
		return confirm(a, question)
	})
	scanProgress.done()
	apiProgress.done()
	if errors.Is(err, ai.ErrCancelled) {
		fmt.Fprintf(a.stdout, "%sStopped before sending any data.%s\n", a.ui.Yellow, a.ui.Reset)
		return 0
	}
	if errors.Is(err, ai.ErrAPICancelled) {
		fmt.Fprintf(a.stdout, "%sStopped while sending API requests. No history was changed.%s\n", a.ui.Yellow, a.ui.Reset)
		return 0
	}
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	fmt.Fprint(a.stdout, plan.Summary)
	if plan.GeneratedCount == 0 {
		return 0
	}
	fmt.Fprintln(a.stderr)
	fmt.Fprintf(a.stderr, "%sWARNING: This operation rewrites Git history. A force push will be required to update remotes.%s\n", a.ui.Red, a.ui.Reset)
	if !confirmOrSkip(a, yes, "Apply these generated commit messages to all listed repositories?") {
		fmt.Fprintf(a.stdout, "%sRewrite cancelled. Generated AI messages were temporary and have been discarded.%s\n", a.ui.Yellow, a.ui.Reset)
		return 0
	}
	return applyAIPlan(a, plan, filterCmd)
}

type aiApplyResult struct {
	plan       ai.RepoPlan
	output     string
	err        error
	restoreErr error
}

func applyAIPlan(a *app, plan *ai.Plan, filterCmd []string) int {
	progress := newProgress(a, "Applying AI rewrites", len(plan.Repos))
	results := parallelItemsWithWorkersProgress(plan.Repos, gitMutationWorkerCount(len(plan.Repos)), progress, func(repoPlan ai.RepoPlan) (string, string) {
		return repoPlan.Name, repoPlan.Name
	}, func(repoPlan ai.RepoPlan) aiApplyResult {
		out, err, restoreErr := runFilterRepoRestoringOrigin(a, repoPlan.Dir, filterCmd, []string{"--partial", "--commit-callback", repoPlan.CallbackFile, "--force"}, nil)
		return aiApplyResult{plan: repoPlan, output: out, err: err, restoreErr: restoreErr}
	})

	hadError := false
	succeededRepos := 0
	succeededCommits := 0
	for _, result := range results {
		if result.err == nil {
			if result.restoreErr != nil {
				fmt.Fprintf(a.stderr, "%sWarning: Commit rewrite completed for %s, but origin could not be restored:\n%s%s\n\n", a.ui.Red, result.plan.Name, result.restoreErr.Error(), a.ui.Reset)
				hadError = true
				continue
			}
			succeededRepos++
			succeededCommits += result.plan.ChangedCount
			continue
		}
		fmt.Fprintf(a.stderr, "%sError: Could not rewrite commit messages for %s:\n%s%s\n\n", a.ui.Red, result.plan.Name, result.output, a.ui.Reset)
		if result.restoreErr != nil {
			fmt.Fprintf(a.stderr, "%sWarning: Commit rewrite failed for %s, and origin could not be restored:\n%s%s\n\n", a.ui.Red, result.plan.Name, result.restoreErr.Error(), a.ui.Reset)
		}
		hadError = true
	}
	if succeededRepos > 0 {
		fmt.Fprintf(a.stdout, "%sRewrote %d commit message(s) across %d repositories.%s\n", a.ui.Green, succeededCommits, succeededRepos, a.ui.Reset)
	}
	if hadError {
		return 1
	}
	return 0
}

func updateAIRequestProgress(a *app, apiProgress **progress, event ai.ProgressEvent) {
	if event.Total <= 1 {
		if event.Error && event.Detail != "" {
			fmt.Fprintln(a.stderr, aiProgressDetail(a, event))
		}
		return
	}
	if *apiProgress == nil {
		*apiProgress = newProgress(a, event.Phase, event.Total)
	}
	detail := aiProgressDetail(a, event)
	if event.Error {
		(*apiProgress).log(detail)
		return
	}
	if event.Current == 0 {
		if detail != "" {
			(*apiProgress).startWork(progressEventKey(event), detail)
		}
		return
	}
	(*apiProgress).finish(progressEventKey(event), detail)
}

func aiProgressDetail(a *app, event ai.ProgressEvent) string {
	if event.Detail == "" {
		return ""
	}
	if event.Error {
		theme := ui.New(a.stderr)
		return theme.Red + event.Detail + theme.Reset
	}
	return event.Detail
}

func progressEventKey(event ai.ProgressEvent) string {
	if event.Key != "" {
		return event.Key
	}
	if event.RepoName != "" {
		return event.RepoName
	}
	return event.Detail
}

func progressEventDetail(event ai.ProgressEvent) string {
	if event.Detail != "" {
		return event.Detail
	}
	return event.RepoName
}
