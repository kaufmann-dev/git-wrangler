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
	rpm, _ := cmd.Flags().GetInt("rpm")
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	timeoutInt, _ := cmd.Flags().GetInt("timeout")
	skipConventional, _ := cmd.Flags().GetBool("skip-conventional")
	requireScope, _ := cmd.Flags().GetBool("require-scope")
	if requireScope {
		skipConventional = true
	}
	body, _ := cmd.Flags().GetBool("body")
	yes := yesFlag(cmd)
	bounds, err := currentRewriteDateBoundsFromFlags(cmd)
	if err != nil {
		a.error(err.Error())
		return 1
	}

	if batch <= 0 {
		a.plainErrorf("--batch-size must be a positive integer.")
		return 1
	}
	if batch > 50 {
		a.plainErrorf("--batch-size must be 50 or less.")
		return 1
	}
	if rpm <= 0 {
		a.plainErrorf("--rpm must be a positive integer.")
		return 1
	}
	if concurrency <= 0 {
		a.plainErrorf("--concurrency must be a positive integer.")
		return 1
	}
	if concurrency > 64 {
		a.plainErrorf("--concurrency must be 64 or less.")
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
	if !preflightAI(a, settings, time.Duration(timeoutInt)*time.Second) {
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
	if !refreshOriginForRewrite(a, cmd, repos) {
		return 1
	}
	aiRepos, ok := rewriteCommitAIRepositories(a, repos, bounds)
	if !ok {
		return 1
	}
	workDir, err := os.MkdirTemp("", "git-wrangler-ai-*")
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	defer os.RemoveAll(workDir)

	scanProgress := newProgress(a, "Scanning repositories", len(aiRepos))
	apiProgress := (*progress)(nil)
	plan, err := ai.Generate(a.ctx, aiRepos, ai.Config{
		BaseURL:          settings.Config.AI.BaseURL,
		Model:            settings.Config.AI.Model,
		APIKey:           settings.APIKey,
		Headers:          settings.Headers,
		BatchSize:        batch,
		RPM:              rpm,
		Concurrency:      concurrency,
		Timeout:          time.Duration(timeoutInt) * time.Second,
		SkipConventional: skipConventional,
		RequireScope:     requireScope,
		Body:             body,
		WorkDir:          workDir,
		Git:              a.git,
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
		return confirm(a, question) == confirmationAccepted
	})
	scanProgress.done()
	apiProgress.done()
	if a.promptFailed {
		return 1
	}
	if errors.Is(err, ai.ErrCancelled) {
		renderStatusLine(a, a.stdout, statusSkip, "stopped before sending any data", "")
		return 0
	}
	if errors.Is(err, ai.ErrAPICancelled) {
		renderStatusLine(a, a.stdout, statusSkip, "stopped while sending API requests", "no history was changed")
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
	renderWarning(a, "This operation rewrites Git history. A force push will be required to update remotes.")
	confirmation := confirmOrSkip(a, yes, "Apply these generated commit messages to all listed repositories?")
	if confirmation == confirmationUnavailable || confirmation == confirmationCancelled {
		return 1
	}
	if confirmation == confirmationDeclined {
		renderStatusLine(a, a.stdout, statusSkip, "rewrite cancelled", "generated AI messages were temporary and have been discarded")
		return 0
	}
	return applyAIPlan(a, plan, filterCmd)
}

func rewriteCommitAIRepositories(a *app, repos []repo, bounds currentRewriteDateBounds) ([]ai.Repository, bool) {
	if !bounds.enabled() {
		aiRepos := make([]ai.Repository, 0, len(repos))
		for _, r := range repos {
			aiRepos = append(aiRepos, ai.Repository{Dir: r.dir, Name: r.display, GitDir: r.gitDir})
		}
		return aiRepos, true
	}
	scans := parallelReposProgress(a.ctx, repos, newProgress(a, "Selecting commits by current author date", len(repos)), func(r repo) currentRewriteDateSelectionScan {
		return scanCurrentRewriteDateSelection(a, r, bounds)
	})
	if interrupted(a) {
		return nil, false
	}
	aiRepos := make([]ai.Repository, 0, len(repos))
	for _, scan := range scans {
		if scan.err != nil {
			renderErrorBlock(a, scan.repo.display+": "+scan.errLabel, scan.err.Error())
			return nil, false
		}
		if !scan.hasHead || scan.noBranches || scan.noCommits || len(scan.selected) == 0 {
			continue
		}
		aiRepos = append(aiRepos, ai.Repository{
			Dir:            scan.repo.dir,
			Name:           scan.repo.display,
			GitDir:         scan.repo.gitDir,
			SelectedHashes: selectedRewriteDateHashSet(scan.commits, scan.selected),
		})
	}
	return aiRepos, true
}

type aiApplyResult struct {
	plan       ai.RepoPlan
	output     string
	err        error
	restoreErr error
}

func applyAIPlan(a *app, plan *ai.Plan, filterCmd []string) int {
	progress := newProgress(a, "Applying AI rewrites", len(plan.Repos))
	results := parallelItemsWithWorkersProgress(a.ctx, plan.Repos, gitMutationWorkerCount(len(plan.Repos)), progress, func(repoPlan ai.RepoPlan) (string, string) {
		return repoPlan.Name, repoPlan.Name
	}, func(repoPlan ai.RepoPlan) aiApplyResult {
		if err := captureRewriteBaselineForHashes(a, repo{dir: repoPlan.Dir, gitDir: repoPlan.GitDir, display: repoPlan.Name}, repoPlan.ChangedHashes); err != nil {
			return aiApplyResult{plan: repoPlan, err: err}
		}
		out, err, restoreErr := runFilterRepoRestoringOrigin(a, repoPlan.Dir, repoPlan.GitDir, filterCmd, []string{"--partial", "--commit-callback", repoPlan.CallbackFile, "--force"}, nil)
		if err == nil {
			if updateErr := updateRewriteBaselineFromFilterRepoMap(repoPlan.GitDir); updateErr != nil {
				return aiApplyResult{plan: repoPlan, output: out, err: fmt.Errorf("could not update rewrite baseline: %w", updateErr), restoreErr: restoreErr}
			}
		}
		return aiApplyResult{plan: repoPlan, output: out, err: err, restoreErr: restoreErr}
	})
	if interrupted(a) {
		return 1
	}

	hadError := false
	succeededRepos := 0
	succeededCommits := 0
	failed := 0
	for _, result := range results {
		if result.err == nil {
			if result.restoreErr != nil {
				renderErrorBlock(a, result.plan.Name+": commit rewrite completed, but origin could not be restored", result.restoreErr.Error())
				hadError = true
				failed++
				continue
			}
			succeededRepos++
			succeededCommits += result.plan.ChangedCount
			continue
		}
		renderErrorBlock(a, result.plan.Name+": could not rewrite commit messages", outputOrError(result.output, result.err))
		if result.restoreErr != nil {
			renderErrorBlock(a, result.plan.Name+": commit rewrite failed, and origin could not be restored", result.restoreErr.Error())
		}
		hadError = true
		failed++
	}
	renderSummary(a,
		summaryCount{label: "commit messages rewritten", value: succeededCommits, color: a.ui.Green},
		summaryCount{label: "repositories updated", value: succeededRepos, color: a.ui.Green},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
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
