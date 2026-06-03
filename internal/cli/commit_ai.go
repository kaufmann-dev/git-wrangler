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

func runCommitAI(a *app, cmd *cobra.Command, args []string) int {
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
	if !requireGit(a, "commit-ai") {
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

	changes, skipped, failed := collectCommitAIChanges(a, repos, maxCharsInt)
	if failed > 0 {
		fmt.Fprintf(a.stdout, "Summary: 0 committed, %d skipped, %d failed\n", skipped, failed)
		return 1
	}
	if len(changes) == 0 {
		fmt.Fprintf(a.stdout, "Summary: 0 committed, %d skipped, 0 failed\n", skipped)
		return 0
	}

	fmt.Fprintln(a.stderr)
	fmt.Fprintln(a.stderr, "Data send notice")
	fmt.Fprintf(a.stderr, "Endpoint: %s\n", settings.Config.AI.BaseURL)
	fmt.Fprintf(a.stderr, "Model: %s\n", settings.Config.AI.Model)
	fmt.Fprintf(a.stderr, "Repositories with staged changes: %d\n", len(changes))
	fmt.Fprintf(a.stderr, "Per-commit context budget: %d characters\n", maxCharsInt)
	if body {
		fmt.Fprintln(a.stderr, "Generated messages will include a subject and body.")
	}
	fmt.Fprintln(a.stderr, "The command will send file paths, stats, and redacted staged diff snippets.")
	fmt.Fprintln(a.stderr, "API keys are not sent in commit context.")
	if !yes && !confirm(a, "Send this data to the configured API endpoint?") {
		fmt.Fprintf(a.stdout, "%sStopped before sending any data.%s\n", a.ui.Yellow, a.ui.Reset)
		return 1
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
			fmt.Fprintf(a.stdout, "%sStopped while sending API requests. No commit was created.%s\n", a.ui.Yellow, a.ui.Reset)
			return 1
		}
		for _, failure := range generationFailures {
			fmt.Fprintf(a.stderr, "Failed %s: %s\n", failure.RepoName, failure.Reason)
		}
		fmt.Fprintf(a.stdout, "Summary: 0 committed, %d skipped, %d failed\n", skipped, len(generationFailures))
		return 1
	}

	commitResults := parallelItemsWithWorkersProgress(changes, gitMutationWorkerCount(len(changes)), newProgress(a, "Creating AI commits", len(changes)), func(change commitAIChange) (string, string) {
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
	committed := 0
	for _, result := range commitResults {
		if result.err == nil {
			a.ok(result.change.repo.display, "Commit created")
			committed++
			continue
		}
		if result.stageErr {
			a.error(result.change.repo.display, "Could not stage changes")
			failed++
			continue
		}
		a.error(result.change.repo.display, "Could not commit changes:")
		fmt.Fprintf(a.stderr, "%s\n\n", result.out)
		failed++
	}
	fmt.Fprintf(a.stdout, "Summary: %d committed, %d skipped, %d failed\n", committed, skipped, failed)
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

func collectCommitAIChanges(a *app, repos []repo, maxChars int) ([]commitAIChange, int, int) {
	type commitAICollectResult struct {
		change       commitAIChange
		repo         repo
		err          error
		stageFailed  bool
		contextError bool
		skipped      bool
	}
	results := parallelGitMutationsProgress(repos, newProgress(a, "Preparing AI commits", len(repos)), func(r repo) commitAICollectResult {
		tempDir, err := os.MkdirTemp("", "git-wrangler-commit-ai-index-*")
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
	changes := []commitAIChange{}
	skipped := 0
	failed := 0
	for _, result := range results {
		switch {
		case result.stageFailed:
			a.error(result.repo.display, "Could not stage changes")
			failed++
		case result.contextError:
			a.error(result.repo.display, "Could not prepare commit context:")
			fmt.Fprintf(a.stderr, "%s\n\n", result.err.Error())
			failed++
		case result.skipped:
			a.skip(result.repo.display, "No changes to commit. Skipping...")
			skipped++
		default:
			result.change.input.ID = fmt.Sprintf("c%06d", len(changes)+1)
			changes = append(changes, result.change)
		}
	}
	return changes, skipped, failed
}
