package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type pullOptions struct {
	target targetOptions
	rebase bool
	force  bool
}

type fetchCommandOptions struct {
	target targetOptions
	prune  bool
}

type pushOptions struct {
	target       targetOptions
	confirmation confirmationOptions
	force        bool
	forceUnsafe  bool
}

func pullOptionsFromCommand(cmd *cobra.Command) pullOptions {
	return pullOptions{
		target: targetOptionsFromCommand(cmd),
		rebase: boolFlagValue(cmd, "rebase"),
		force:  boolFlagValue(cmd, "force"),
	}
}

func fetchCommandOptionsFromCommand(cmd *cobra.Command) fetchCommandOptions {
	return fetchCommandOptions{
		target: targetOptionsFromCommand(cmd),
		prune:  boolFlagValue(cmd, "prune"),
	}
}

func pushOptionsFromCommand(cmd *cobra.Command) pushOptions {
	return pushOptions{
		target:       targetOptionsFromCommand(cmd),
		confirmation: confirmationOptionsFromCommand(cmd),
		force:        boolFlagValue(cmd, "force"),
		forceUnsafe:  boolFlagValue(cmd, "force-unsafe"),
	}
}

func runPull(a *app, cmd *cobra.Command, args []string) int {
	opts := pullOptionsFromCommand(cmd)
	if !requireGit(a, "pull") {
		return 1
	}
	repos, err := opts.target.repositories()
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
		if opts.rebase {
			pullArgs = append(pullArgs, "--rebase")
		}
		if opts.force {
			pullArgs = append(pullArgs, "--force")
		}
		out, err := a.git.CaptureRemote(a.ctx, r.dir, nil, pullArgs...)
		return pullResult{repo: r, out: out, err: err, skipped: err == nil && strings.Contains(out, "Already up to date")}
	})
	if interrupted(a) {
		return 1
	}
	for _, result := range results {
		if result.err != nil {
			renderRemoteGitFailure(a, result.repo, "pull", result.out, result.err)
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
	opts := fetchCommandOptionsFromCommand(cmd)
	if !requireGit(a, "fetch") {
		return 1
	}
	repos, err := opts.target.repositories()
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
		if opts.prune {
			fetchArgs = []string{"fetch", "--prune", "origin"}
		}
		out, err := captureRemoteGitWithRetry(a, r.dir, nil, fetchArgs...)
		return fetchResult{repo: r, out: out, err: err}
	})
	if interrupted(a) {
		return 1
	}
	for _, result := range results {
		if result.err != nil {
			renderRemoteGitFailure(a, result.repo, "fetch", result.out, result.err)
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
	opts := pushOptionsFromCommand(cmd)
	if opts.force && opts.forceUnsafe {
		a.plainErrorf("Use either --force or --force-unsafe, not both.")
		return 1
	}
	if !requireGit(a, "push") {
		return 1
	}
	repos, err := opts.target.repositories()
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
	if !opts.forceUnsafe {
		results := parallelGitMutationsProgress(a.ctx, repos, newProgress(a, "Pushing repositories", len(repos)), func(r repo) pushResult {
			pushArgs := []string{"push", "origin", "HEAD"}
			if opts.force {
				pushArgs = []string{"push", "--force-with-lease", "origin", "HEAD"}
			}
			out, err := a.git.CaptureRemote(a.ctx, r.dir, nil, pushArgs...)
			return pushResult{repo: r, out: out, err: err, skipped: err == nil && strings.Contains(out, "Everything up-to-date")}
		})
		if interrupted(a) {
			return 1
		}
		for _, result := range results {
			if result.err != nil {
				renderRemoteGitFailure(a, result.repo, "push", result.out, result.err)
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
	confirmation := confirmOrSkip(a, opts.confirmation.yes, fmt.Sprintf("Raw force push %d repositories with --force?", len(repos)))
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
		out, err := a.git.CaptureRemote(a.ctx, r.dir, nil, "push", "--force", "origin", "HEAD")
		progress.advance(r.display)
		results = append(results, pushResult{repo: r, out: out, err: err, skipped: err == nil && strings.Contains(out, "Everything up-to-date")})
	}
	finishProgressBeforeOutput(progress)
	for _, result := range results {
		if result.err != nil {
			renderRemoteGitFailure(a, result.repo, "push", result.out, result.err)
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
