package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/ai"
	"github.com/spf13/cobra"
)

type commitAIChange struct {
	repo  repo
	input ai.GenerationInput
}

type commitAICommitResult struct {
	change   commitAIChange
	out      string
	err      error
	stageErr bool
}

func runCommit(a *app, cmd *cobra.Command, args []string) int {
	maxCharsInt, _ := cmd.Flags().GetInt("max-chars-per-commit")
	rpm, _ := cmd.Flags().GetInt("rpm")
	timeoutInt, _ := cmd.Flags().GetInt("timeout")
	body, _ := cmd.Flags().GetBool("body")
	yes := yesFlag(cmd)

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
	if !requireGit(a, "commit") {
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

	changes, skipped, failed := collectCommitChanges(a, repos, maxCharsInt)
	if a.ctx.Err() != nil {
		return 1
	}
	if failed > 0 {
		renderSummary(a,
			summaryCount{label: "committed", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		return 1
	}
	if len(changes) == 0 {
		renderSummary(a,
			summaryCount{label: "committed", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: 0, color: a.ui.Red},
		)
		return 0
	}

	bodyLines := []string{
		"Content: file paths, stats, and redacted staged diff snippets",
		"Secrets: API keys are not sent in commit context",
	}
	if body {
		bodyLines = append(bodyLines, "Messages: subject and body")
	}
	renderNotice(a, "Data Send Notice", []keyValueRow{
		{key: "Endpoint", value: settings.Config.AI.BaseURL},
		{key: "Model", value: settings.Config.AI.Model},
		{key: "Repositories", value: fmt.Sprintf("%d", len(changes))},
		{key: "Context budget", value: fmt.Sprintf("%d characters per commit", maxCharsInt)},
	}, bodyLines)
	confirmation := confirmOrSkip(a, yes, "Send this data to the configured API endpoint?")
	if confirmation == confirmationUnavailable || confirmation == confirmationCancelled {
		return 1
	}
	if confirmation == confirmationDeclined {
		renderStatusLine(a, a.stdout, statusSkip, "stopped before sending any data", "")
		return 0
	}

	inputs := make([]ai.GenerationInput, 0, len(changes))
	for _, change := range changes {
		inputs = append(inputs, change.input)
	}
	apiProgress := (*progress)(nil)
	messages, generationFailures := ai.GenerateMessages(a.ctx, inputs, ai.Config{
		BaseURL:           settings.Config.AI.BaseURL,
		Model:             settings.Config.AI.Model,
		APIKey:            settings.APIKey,
		Headers:           settings.Headers,
		BatchSize:         4,
		MaxCharsPerCommit: maxCharsInt,
		RPM:               rpm,
		Timeout:           time.Duration(timeoutInt) * time.Second,
		Body:              body,
		Git:               a.git,
		Progress: func(event ai.ProgressEvent) {
			if event.Phase != "Sending API requests" {
				return
			}
			updateAIRequestProgress(a, &apiProgress, event)
		},
	}, a.stderr)
	apiProgress.done()
	if len(generationFailures) > 0 {
		if allGenerationFailuresAre(generationFailures, ai.ErrAPICancelled.Error()) {
			renderStatusLine(a, a.stdout, statusSkip, "stopped while sending API requests", "no commit was created")
			return 1
		}
		for _, failure := range generationFailures {
			renderStatusLine(a, a.stderr, statusError, failure.RepoName, failure.Reason)
		}
		renderSummary(a,
			summaryCount{label: "committed", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: len(generationFailures), color: a.ui.Red},
		)
		return 1
	}

	commitResults := parallelItemsWithWorkersProgress(a.ctx, changes, gitMutationWorkerCount(len(changes)), newProgress(a, "Creating AI commits", len(changes)), func(change commitAIChange) (string, string) {
		return change.repo.display, change.repo.display
	}, func(change commitAIChange) commitAICommitResult {
		message := messages[change.input.ID]
		if _, err := a.git.Capture(a.ctx, change.repo.dir, nil, "add", "-A"); err != nil {
			return commitAICommitResult{change: change, err: err, stageErr: true}
		}
		if body {
			if out, err := a.git.Capture(a.ctx, change.repo.dir, nil, "commit", "-m", message.Subject, "-m", message.Body); err == nil {
				return commitAICommitResult{change: change, out: out}
			} else {
				return commitAICommitResult{change: change, out: out, err: err}
			}
		}
		if out, err := a.git.Capture(a.ctx, change.repo.dir, nil, "commit", "-m", message.Subject); err == nil {
			return commitAICommitResult{change: change, out: out}
		} else {
			return commitAICommitResult{change: change, out: out, err: err}
		}
	})
	if interrupted(a) {
		return 1
	}
	committed := 0
	for _, result := range commitResults {
		if result.err == nil {
			committed++
			continue
		}
		if result.stageErr {
			renderStatusLine(a, a.stderr, statusError, result.change.repo.display, "could not stage changes")
			failed++
			continue
		}
		renderErrorBlock(a, result.change.repo.display+": could not commit changes", result.out)
		failed++
	}
	renderSummary(a,
		summaryCount{label: "committed", value: committed, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	if failed > 0 {
		return 1
	}
	return 0
}

func allGenerationFailuresAre(failures []ai.GenerationFailure, reason string) bool {
	if len(failures) == 0 {
		return false
	}
	for _, failure := range failures {
		if failure.Reason != reason {
			return false
		}
	}
	return true
}

func collectCommitChanges(a *app, repos []repo, maxChars int) ([]commitAIChange, int, int) {
	type commitAICollectResult struct {
		change       commitAIChange
		repo         repo
		err          error
		stageFailed  bool
		contextError bool
		skipped      bool
	}
	results := parallelGitMutationsProgress(a.ctx, repos, newProgress(a, "Preparing AI commits", len(repos)), func(r repo) commitAICollectResult {
		tempDir, err := os.MkdirTemp("", "git-wrangler-commit-index-*")
		if err != nil {
			return commitAICollectResult{repo: r, err: err, stageFailed: true}
		}
		defer os.RemoveAll(tempDir)
		env := []string{"GIT_INDEX_FILE=" + filepath.Join(tempDir, "index")}
		if _, err := a.git.Capture(a.ctx, r.dir, nil, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
			if _, err := a.git.Capture(a.ctx, r.dir, env, "read-tree", "HEAD"); err != nil {
				return commitAICollectResult{repo: r, err: err, stageFailed: true}
			}
		}
		if _, err := a.git.Capture(a.ctx, r.dir, env, "add", "-A"); err != nil {
			return commitAICollectResult{repo: r, err: err, stageFailed: true}
		}
		if _, err := a.git.Capture(a.ctx, r.dir, env, "diff", "--cached", "--quiet"); err == nil {
			return commitAICollectResult{repo: r, skipped: true}
		}
		contextText, err := ai.BuildStagedContextWithEnv(a.ctx, a.git, r.dir, r.display, maxChars, env)
		if err != nil {
			return commitAICollectResult{repo: r, err: err, contextError: true}
		}
		if contextText == "" {
			return commitAICollectResult{repo: r, skipped: true}
		}
		return commitAICollectResult{
			repo: r,
			change: commitAIChange{
				repo: r,
				input: ai.GenerationInput{
					RepoDir:  r.dir,
					RepoName: r.display,
					Ref:      "staged",
					Context:  contextText,
				},
			},
		}
	})
	if interrupted(a) {
		return nil, 0, 1
	}
	changes := []commitAIChange{}
	skipped := 0
	failed := 0
	for _, result := range results {
		switch {
		case result.stageFailed:
			renderStatusLine(a, a.stderr, statusError, result.repo.display, "could not stage changes")
			failed++
		case result.contextError:
			renderErrorBlock(a, result.repo.display+": could not prepare commit context", result.err.Error())
			failed++
		case result.skipped:
			renderStatusLine(a, a.stdout, statusSkip, result.repo.display, "no changes to commit")
			skipped++
		default:
			result.change.input.ID = fmt.Sprintf("c%06d", len(changes)+1)
			changes = append(changes, result.change)
		}
	}
	return changes, skipped, failed
}
