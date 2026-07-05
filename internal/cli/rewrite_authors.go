package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
	"github.com/spf13/cobra"
)

type authorApply struct {
	repo     repo
	branches []dateBranchRef
	hashes   []string
}

type authorApplyResult struct {
	apply      authorApply
	output     string
	err        error
	restoreErr error
}

type rewriteAuthorsOptions struct {
	target       targetOptions
	fetch        fetchOptions
	confirmation confirmationOptions
	bounds       currentRewriteDateBounds
	name         string
	email        string
}

func rewriteAuthorsOptionsFromCommand(a *app, cmd *cobra.Command) (rewriteAuthorsOptions, bool) {
	boundOpts, err := rewriteBoundOptionsFromCommand(cmd)
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return rewriteAuthorsOptions{}, false
	}
	newName, ok := requiredStringFlag(a, cmd, "name", "New author and committer name: ")
	if !ok {
		return rewriteAuthorsOptions{}, false
	}
	newEmail, ok := requiredStringFlag(a, cmd, "email", "New author and committer email: ")
	if !ok {
		return rewriteAuthorsOptions{}, false
	}
	return rewriteAuthorsOptions{
		target:       targetOptionsFromCommand(cmd),
		fetch:        fetchOptionsFromCommand(cmd),
		confirmation: confirmationOptionsFromCommand(cmd),
		bounds:       boundOpts.bounds,
		name:         newName,
		email:        newEmail,
	}, true
}

func runRewriteAuthors(a *app, cmd *cobra.Command, args []string) int {
	opts, ok := rewriteAuthorsOptionsFromCommand(a, cmd)
	if !ok {
		return 1
	}
	if !requireGit(a, "rewrite-authors") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-authors")
	if !ok {
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
	if !refreshOriginForRewriteOptions(a, opts.fetch, repos) {
		return 1
	}
	status := 0
	scans := parallelReposProgress(a.ctx, repos, newProgress(a, "Preparing author rewrites", len(repos)), func(r repo) currentRewriteDateSelectionScan {
		return scanCurrentRewriteDateSelection(a, r, opts.bounds)
	})
	if interrupted(a) {
		return 1
	}
	skipped := 0
	scanFailed := 0
	applies := []authorApply{}
	for _, scan := range scans {
		if scan.err != nil {
			renderErrorBlock(a, scan.repo.display+": "+scan.errLabel, scan.err.Error())
			status = 1
			scanFailed++
			continue
		}
		if !scan.hasHead || scan.noBranches || scan.noCommits || len(scan.selected) == 0 {
			skipped++
			continue
		}
		applies = append(applies, authorApply{
			repo:     scan.repo,
			branches: scan.branches,
			hashes:   selectedRewriteDateHashes(scan.commits, scan.selected),
		})
	}
	if len(applies) == 0 {
		renderSummary(a,
			summaryCount{label: "rewritten", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
			summaryCount{label: "failed", value: scanFailed, color: a.ui.Red},
		)
		return status
	}
	renderNotice(a, "Author Rewrite", []keyValueRow{
		{key: "Repositories", value: fmt.Sprintf("%d", len(applies))},
		{key: "New name", value: opts.name},
		{key: "New email", value: opts.email},
		{key: "Current author date filter", value: currentRewriteDateBoundsDescription(opts.bounds)},
	}, nil)
	renderWarning(a, fmt.Sprintf("This operation rewrites Git history in %d repositories. A force push will be required to update any remote.", len(applies)))
	confirmation := confirmOrSkip(a, opts.confirmation.yes, fmt.Sprintf("Rewrite author and committer identity in %d repositories?", len(applies)))
	if confirmation == confirmationUnavailable || confirmation == confirmationCancelled {
		return 1
	}
	if confirmation == confirmationDeclined {
		renderSummary(a,
			summaryCount{label: "rewritten", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: skipped + len(applies), color: a.ui.Yellow},
			summaryCount{label: "failed", value: scanFailed, color: a.ui.Red},
		)
		return status
	}
	results := parallelItemsWithWorkersProgress(a.ctx, applies, gitMutationWorkerCount(len(applies)), newProgress(a, "Rewriting authors", len(applies)), func(apply authorApply) (string, string) {
		return apply.repo.display, apply.repo.display
	}, func(apply authorApply) authorApplyResult {
		if err := captureRewriteBaselineForHashes(a, apply.repo, apply.hashes); err != nil {
			return authorApplyResult{apply: apply, err: err}
		}
		callback, err := writeRewriteAuthorCallback(apply.hashes)
		if err != nil {
			return authorApplyResult{apply: apply, err: fmt.Errorf("could not create author callback: %w", err)}
		}
		defer os.Remove(callback)
		out, err, restoreErr := runFilterRepoRestoringOrigin(a, apply.repo.dir, apply.repo.gitDir, filterCmd, rewriteAuthorFilterArgs(apply.branches, callback), []string{"NEW_EMAIL_ENV=" + opts.email, "NEW_NAME_ENV=" + opts.name})
		if err == nil {
			if updateErr := updateRewriteBaselineFromFilterRepoMap(apply.repo.gitDir); updateErr != nil {
				return authorApplyResult{apply: apply, output: out, err: fmt.Errorf("could not update rewrite baseline: %w", updateErr), restoreErr: restoreErr}
			}
		}
		return authorApplyResult{apply: apply, output: out, err: err, restoreErr: restoreErr}
	})
	if interrupted(a) {
		return 1
	}
	rewritten := 0
	applyFailed := 0
	for _, result := range results {
		r := result.apply.repo
		if result.err == nil {
			if result.restoreErr != nil {
				renderErrorBlock(a, r.display+": author rewrite completed, but origin could not be restored", result.restoreErr.Error())
				status = 1
				applyFailed++
				continue
			}
			rewritten++
			continue
		}
		renderErrorBlock(a, r.display+": could not update git author and committer information", outputOrError(result.output, result.err))
		if result.restoreErr != nil {
			renderErrorBlock(a, r.display+": author rewrite failed, and origin could not be restored", result.restoreErr.Error())
		}
		status = 1
		applyFailed++
	}
	renderSummary(a,
		summaryCount{label: "rewritten", value: rewritten, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: scanFailed + applyFailed, color: a.ui.Red},
	)
	return status
}

func rewriteAuthorFilterArgs(branches []dateBranchRef, callback string) []string {
	args := []string{"--partial", "--force"}
	if len(branches) > 0 {
		args = append(args, "--refs")
		for _, branch := range branches {
			args = append(args, branch.Name)
		}
	}
	args = append(args, "--commit-callback", callback)
	return args
}

func writeRewriteAuthorCallback(hashes []string) (string, error) {
	f, err := os.CreateTemp("", "git-wrangler-author-callback-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	fmt.Fprintln(f, "import os")
	fmt.Fprintln(f, "selected = {}")
	hashes = sortedUniqueNonEmpty(hashes)
	sort.Strings(hashes)
	for _, hash := range hashes {
		fmt.Fprintf(f, "selected[%s] = True\n", git.PythonBytesLiteral(hash))
	}
	fmt.Fprintln(f, `new_name = os.environ["NEW_NAME_ENV"].encode("utf-8")`)
	fmt.Fprintln(f, `new_email = os.environ["NEW_EMAIL_ENV"].encode("utf-8")`)
	fmt.Fprintln(f, "if commit.original_id in selected:")
	fmt.Fprintln(f, "    commit.author_name = new_name")
	fmt.Fprintln(f, "    commit.author_email = new_email")
	fmt.Fprintln(f, "    commit.committer_name = new_name")
	fmt.Fprintln(f, "    commit.committer_email = new_email")
	return f.Name(), nil
}
