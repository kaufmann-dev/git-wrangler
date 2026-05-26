package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/ai"
	"github.com/spf13/cobra"
)

func runRewriteCommitsAI(a *app, cmd *cobra.Command, args []string) int {
	baseURL, _ := cmd.Flags().GetString("base-url")
	model, _ := cmd.Flags().GetString("model")
	apiKey, _ := cmd.Flags().GetString("api-key")
	apiKeyEnv, _ := cmd.Flags().GetString("api-key-env")
	batchSize, _ := cmd.Flags().GetString("batch-size")
	maxChars, _ := cmd.Flags().GetString("max-chars-per-commit")
	timeoutSeconds, _ := cmd.Flags().GetString("timeout")
	skipConventional, _ := cmd.Flags().GetBool("skip-conventional")

	if !positiveInt(batchSize) {
		a.plainErrorf("--batch-size must be a positive integer.")
		return 1
	}
	batch, _ := strconv.Atoi(batchSize)
	if batch > 50 {
		a.plainErrorf("--batch-size must be 50 or less.")
		return 1
	}
	if !positiveInt(maxChars) {
		a.plainErrorf("--max-chars-per-commit must be a positive integer.")
		return 1
	}
	if !positiveInt(timeoutSeconds) {
		a.plainErrorf("--timeout must be a positive integer.")
		return 1
	}
	maxCharsInt, _ := strconv.Atoi(maxChars)
	timeoutInt, _ := strconv.Atoi(timeoutSeconds)
	if baseURL == "" {
		a.plainErrorf("--base-url is required.")
		return 1
	}
	if model == "" {
		a.plainErrorf("--model is required.")
		return 1
	}
	if envKey := os.Getenv(apiKeyEnv); envKey != "" {
		apiKey = envKey
	}
	if apiKey == "" {
		a.plainErrorf("API key is required. Set %s or pass --api-key.", apiKeyEnv)
		return 1
	}

	if !requireGit(a, "rewrite-commits-ai") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-commits-ai")
	if !ok {
		return 1
	}

	repos, err := findGitRepositories(".")
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
	plan, err := ai.Generate(context.Background(), aiRepos, ai.Config{
		BaseURL:           baseURL,
		Model:             model,
		APIKey:            apiKey,
		BatchSize:         batch,
		MaxCharsPerCommit: maxCharsInt,
		Timeout:           time.Duration(timeoutInt) * time.Second,
		SkipConventional:  skipConventional,
		WorkDir:           workDir,
	}, a.stdout, func(question string) bool {
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
	if !confirm(a, "Apply these generated commit messages to all listed repositories?") {
		fmt.Fprintf(a.stdout, "%sRewrite cancelled. Generated AI messages were temporary and have been discarded.%s\n", a.ui.Yellow, a.ui.Reset)
		return 1
	}
	return applyAIPlan(a, plan, filterCmd)
}

func positiveInt(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0
}

func applyAIPlan(a *app, plan *ai.Plan, filterCmd []string) int {
	hadError := false
	for _, repoPlan := range plan.Repos {
		remoteURL := strings.TrimSpace(mustStdout(repoPlan.Dir, "git", "remote", "get-url", "origin"))
		out, err := runFilterRepo(repoPlan.Dir, filterCmd, []string{"--partial", "--commit-callback", repoPlan.CallbackFile, "--force"}, nil)
		if err == nil {
			if remoteURL != "" {
				if _, err := runCapture(repoPlan.Dir, nil, "git", "remote", "get-url", "origin"); err != nil {
					if restore, err := runCapture(repoPlan.Dir, nil, "git", "remote", "add", "origin", remoteURL); err != nil {
						fmt.Fprintf(a.stderr, "%sWarning: Commit rewrite completed for %s, but origin could not be restored:\n%s%s\n\n", a.ui.Red, repoPlan.Name, restore, a.ui.Reset)
						hadError = true
						continue
					}
				}
			}
			fmt.Fprintf(a.stdout, "%sRewrote %d commit message(s) for %s%s\n", a.ui.Green, repoPlan.ChangedCount, repoPlan.Name, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not rewrite commit messages for %s:\n%s%s\n\n", a.ui.Red, repoPlan.Name, out, a.ui.Reset)
			hadError = true
		}
	}
	if hadError {
		return 1
	}
	return 0
}
