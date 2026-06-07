package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func runPull(a *app, cmd *cobra.Command, args []string) int {
	rebase, _ := cmd.Flags().GetBool("rebase")
	force, _ := cmd.Flags().GetBool("force")
	if !requireGit(a, "pull") {
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
	type pullResult struct {
		repo    repo
		out     string
		err     error
		skipped bool
	}
	status := 0
	updated := 0
	skipped := 0
	failed := 0
	results := parallelGitMutationsProgress(a.ctx, repos, newProgress(a, "Pulling repositories", len(repos)), func(r repo) pullResult {
		pullArgs := []string{"pull"}
		if rebase {
			pullArgs = append(pullArgs, "--rebase")
		}
		if force {
			pullArgs = append(pullArgs, "--force")
		}
		out, err := a.git.Capture(a.ctx, r.dir, nil, pullArgs...)
		return pullResult{repo: r, out: out, err: err, skipped: err == nil && strings.Contains(out, "Already up to date")}
	})
	if interrupted(a) {
		return 1
	}
	for _, result := range results {
		if result.err != nil {
			renderErrorBlock(a, result.repo.display+": git pull failed", outputOrError(result.out, result.err))
			status = 1
			failed++
		} else if result.skipped {
			renderStatusLine(a, a.stdout, statusSkip, result.repo.display, "already up to date")
			skipped++
		} else {
			updated++
		}
	}
	renderSummary(a,
		summaryCount{label: "updated", value: updated, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	return status
}

func runFetch(a *app, cmd *cobra.Command, args []string) int {
	prune, _ := cmd.Flags().GetBool("prune")
	if !requireGit(a, "fetch") {
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
	type fetchResult struct {
		repo repo
		out  string
		err  error
	}
	status := 0
	fetched := 0
	failed := 0
	results := parallelGitMutationsProgress(a.ctx, repos, newProgress(a, "Fetching repositories", len(repos)), func(r repo) fetchResult {
		fetchArgs := []string{"fetch", "origin"}
		if prune {
			fetchArgs = []string{"fetch", "--prune", "origin"}
		}
		out, err := a.git.Capture(a.ctx, r.dir, nil, fetchArgs...)
		return fetchResult{repo: r, out: out, err: err}
	})
	if interrupted(a) {
		return 1
	}
	for _, result := range results {
		if result.err != nil {
			renderErrorBlock(a, result.repo.display+": git fetch failed", outputOrError(result.out, result.err))
			status = 1
			failed++
			continue
		}
		fetched++
	}
	renderSummary(a,
		summaryCount{label: "fetched", value: fetched, color: a.ui.Green},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	return status
}

func runPush(a *app, cmd *cobra.Command, args []string) int {
	force, _ := cmd.Flags().GetBool("force")
	forceUnsafe, _ := cmd.Flags().GetBool("force-unsafe")
	if force && forceUnsafe {
		a.error("Use either --force or --force-unsafe, not both.")
		return 1
	}
	if !requireGit(a, "push") {
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
	type pushResult struct {
		repo    repo
		out     string
		err     error
		skipped bool
	}
	status := 0
	pushed := 0
	skipped := 0
	failed := 0
	if !forceUnsafe {
		results := parallelGitMutationsProgress(a.ctx, repos, newProgress(a, "Pushing repositories", len(repos)), func(r repo) pushResult {
			pushArgs := []string{"push", "origin", "HEAD"}
			if force {
				pushArgs = []string{"push", "--force-with-lease", "origin", "HEAD"}
			}
			out, err := a.git.Capture(a.ctx, r.dir, nil, pushArgs...)
			return pushResult{repo: r, out: out, err: err, skipped: err == nil && strings.Contains(out, "Everything up-to-date")}
		})
		if interrupted(a) {
			return 1
		}
		for _, result := range results {
			if result.err != nil {
				renderErrorBlock(a, result.repo.display+": git push failed", outputOrError(result.out, result.err))
				status = 1
				failed++
			} else if result.skipped {
				renderStatusLine(a, a.stdout, statusSkip, result.repo.display, "nothing to push")
				skipped++
			} else {
				pushed++
			}
		}
		renderSummary(a,
			summaryCount{label: "pushed", value: pushed, color: a.ui.Green},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		return status
	}
	confirmation := confirmOrSkip(a, yesFlag(cmd), fmt.Sprintf("Raw force push %d repositories with --force?", len(repos)))
	if confirmation == confirmationUnavailable || confirmation == confirmationCancelled {
		return 1
	}
	if confirmation == confirmationDeclined {
		skipped = len(repos)
		renderStatusLine(a, a.stdout, statusSkip, "unsafe force push declined", "")
		renderSummary(a,
			summaryCount{label: "pushed", value: pushed, color: a.ui.Green},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		return status
	}
	progress := newProgress(a, "Pushing repositories", len(repos))
	results := []pushResult{}
	for _, r := range repos {
		progress.start(r.display)
		pushArgs := []string{"push", "origin", "HEAD"}
		pushArgs = []string{"push", "--force", "origin", "HEAD"}
		out, err := a.git.Capture(a.ctx, r.dir, nil, pushArgs...)
		progress.advance(r.display)
		results = append(results, pushResult{repo: r, out: out, err: err, skipped: err == nil && strings.Contains(out, "Everything up-to-date")})
	}
	finishProgressBeforeOutput(progress)
	for _, result := range results {
		r := result.repo
		out := result.out
		err := result.err
		if err == nil {
			if strings.Contains(out, "Everything up-to-date") {
				renderStatusLine(a, a.stdout, statusSkip, r.display, "nothing to push")
				skipped++
			} else {
				pushed++
			}
		} else {
			renderErrorBlock(a, r.display+": git push failed", outputOrError(out, err))
			status = 1
			failed++
		}
	}
	renderSummary(a,
		summaryCount{label: "pushed", value: pushed, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	return status
}

func outputOrError(output string, err error) string {
	if strings.TrimSpace(output) != "" || err == nil {
		return output
	}
	return err.Error()
}
