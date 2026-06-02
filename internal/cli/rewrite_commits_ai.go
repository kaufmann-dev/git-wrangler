package cli

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/ai"
	"github.com/spf13/cobra"
)

func runRewriteCommitsAI(a *app, cmd *cobra.Command, args []string) int {
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
			if event.Total <= 1 {
				return
			}
			switch event.Phase {
			case "Scanning repositories":
				if event.Current == 0 {
					return
				}
				scanProgress.advance(event.RepoName)
			case "Scanning commits":
				if event.Current == 0 {
					return
				}
				scanProgress.message(fmt.Sprintf("%s %d/%d", event.RepoName, event.Current, event.Total))
			case "Sending API requests":
				if apiProgress == nil {
					apiProgress = newProgress(a, event.Phase, event.Total)
				}
				if event.Detail != "" {
					apiProgress.log(event.Detail)
					return
				}
				if event.Current == 0 {
					return
				}
				apiProgress.advance("")
			default:
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
		return 1
	}
	if errors.Is(err, ai.ErrAPICancelled) {
		fmt.Fprintf(a.stdout, "%sStopped while sending API requests. No history was changed.%s\n", a.ui.Yellow, a.ui.Reset)
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

type aiApplyResult struct {
	plan       ai.RepoPlan
	output     string
	err        error
	restoreErr error
}

func applyAIPlan(a *app, plan *ai.Plan, filterCmd []string) int {
	progress := newProgress(a, "Applying AI rewrites", len(plan.Repos))
	results := make([]aiApplyResult, len(plan.Repos))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := gitMutationWorkerCount(len(plan.Repos))
	if workers < 1 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				repoPlan := plan.Repos[index]
				out, err, restoreErr := runFilterRepoRestoringOrigin(a, repoPlan.Dir, filterCmd, []string{"--partial", "--commit-callback", repoPlan.CallbackFile, "--force"}, nil)
				results[index] = aiApplyResult{plan: repoPlan, output: out, err: err, restoreErr: restoreErr}
				progress.advance(aiApplyProgressDetail(results[index]))
			}
		}()
	}
	for index := range plan.Repos {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	progress.done()

	hadError := false
	for _, result := range results {
		if result.err == nil {
			if result.restoreErr != nil {
				fmt.Fprintf(a.stderr, "%sWarning: Commit rewrite completed for %s, but origin could not be restored:\n%s%s\n\n", a.ui.Red, result.plan.Name, result.restoreErr.Error(), a.ui.Reset)
				hadError = true
				continue
			}
			fmt.Fprintf(a.stdout, "%sRewrote %d commit message(s) for %s%s\n", a.ui.Green, result.plan.ChangedCount, result.plan.Name, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stderr, "%sError: Could not rewrite commit messages for %s:\n%s%s\n\n", a.ui.Red, result.plan.Name, result.output, a.ui.Reset)
		if result.restoreErr != nil {
			fmt.Fprintf(a.stderr, "%sWarning: Commit rewrite failed for %s, and origin could not be restored:\n%s%s\n\n", a.ui.Red, result.plan.Name, result.restoreErr.Error(), a.ui.Reset)
		}
		hadError = true
	}
	if hadError {
		return 1
	}
	return 0
}

func aiApplyProgressDetail(result aiApplyResult) string {
	if result.err != nil {
		return result.plan.Name + " failed"
	}
	if result.restoreErr != nil {
		return result.plan.Name + " rewrite done, origin restore failed"
	}
	return fmt.Sprintf("%s rewrote %d commit message(s)", result.plan.Name, result.plan.ChangedCount)
}
