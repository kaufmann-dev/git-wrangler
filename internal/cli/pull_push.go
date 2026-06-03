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
	repos, err := resolveRepositoryTargets("")
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
	results := parallelGitMutationsProgress(repos, newProgress(a, "Pulling repositories", len(repos)), func(r repo) pullResult {
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
	for _, result := range results {
		if result.err != nil {
			a.error(result.repo.display, "Git pull failed:")
			fmt.Fprintf(a.stderr, "%s\n\n", result.out)
			status = 1
			failed++
		} else if result.skipped {
			a.skip(result.repo.display, "Already up to date. Skipping...")
			skipped++
		} else {
			a.ok(result.repo.display, "Git pull completed")
			updated++
		}
	}
	fmt.Fprintf(a.stdout, "Summary: %d updated, %d skipped, %d failed\n", updated, skipped, failed)
	return status
}

func runFetch(a *app, cmd *cobra.Command, args []string) int {
	prune, _ := cmd.Flags().GetBool("prune")
	if !requireGit(a, "fetch") {
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
	type fetchResult struct {
		repo repo
		out  string
		err  error
	}
	status := 0
	fetched := 0
	failed := 0
	results := parallelGitMutationsProgress(repos, newProgress(a, "Fetching repositories", len(repos)), func(r repo) fetchResult {
		fetchArgs := []string{"fetch", "origin"}
		if prune {
			fetchArgs = []string{"fetch", "--prune", "origin"}
		}
		out, err := a.git.Capture(a.ctx, r.dir, nil, fetchArgs...)
		return fetchResult{repo: r, out: out, err: err}
	})
	for _, result := range results {
		if result.err != nil {
			a.error(result.repo.display, "Git fetch failed:")
			fmt.Fprintf(a.stderr, "%s\n\n", result.out)
			status = 1
			failed++
			continue
		}
		a.ok(result.repo.display, "Git fetch completed")
		fetched++
	}
	fmt.Fprintf(a.stdout, "Summary: %d fetched, %d failed\n", fetched, failed)
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
	repos, err := resolveRepositoryTargets("")
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
		results := parallelGitMutationsProgress(repos, newProgress(a, "Pushing repositories", len(repos)), func(r repo) pushResult {
			pushArgs := []string{"push", "origin", "HEAD"}
			if force {
				pushArgs = []string{"push", "--force-with-lease", "origin", "HEAD"}
			}
			out, err := a.git.Capture(a.ctx, r.dir, nil, pushArgs...)
			return pushResult{repo: r, out: out, err: err, skipped: err == nil && strings.Contains(out, "Everything up-to-date")}
		})
		for _, result := range results {
			if result.err != nil {
				a.error(result.repo.display, "Git push failed:")
				fmt.Fprintf(a.stderr, "%s\n\n", result.out)
				status = 1
				failed++
			} else if result.skipped {
				a.skip(result.repo.display, "No changes to push. Skipping...")
				skipped++
			} else {
				a.ok(result.repo.display, "Git push completed")
				pushed++
			}
		}
		fmt.Fprintf(a.stdout, "Summary: %d pushed, %d skipped, %d failed\n", pushed, skipped, failed)
		return status
	}
	for _, r := range repos {
		pushArgs := []string{"push", "origin", "HEAD"}
		if !yesFlag(cmd) && !confirm(a, "Raw force push "+r.display+" with --force?") {
			a.skip(r.display, "Skipping unsafe force push.")
			skipped++
			continue
		}
		pushArgs = []string{"push", "--force", "origin", "HEAD"}
		out, err := a.git.Capture(a.ctx, r.dir, nil, pushArgs...)
		if err == nil {
			if strings.Contains(out, "Everything up-to-date") {
				a.skip(r.display, "No changes to push. Skipping...")
				skipped++
			} else {
				a.ok(r.display, "Git push completed")
				pushed++
			}
		} else {
			a.error(r.display, "Git push failed:")
			fmt.Fprintf(a.stderr, "%s\n\n", out)
			status = 1
			failed++
		}
	}
	fmt.Fprintf(a.stdout, "Summary: %d pushed, %d skipped, %d failed\n", pushed, skipped, failed)
	return status
}
