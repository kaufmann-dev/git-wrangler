package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func runRewriteAuthors(a *app, cmd *cobra.Command, args []string) int {
	force, _ := cmd.Flags().GetBool("force")
	yes := yesFlag(cmd)
	newName, ok := requiredStringFlag(a, cmd, "name", "New author and committer name: ")
	if !ok {
		return 1
	}
	newEmail, ok := requiredStringFlag(a, cmd, "email", "New author and committer email: ")
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
	repos, err := commandRepositoryTargets(cmd)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	filterArgs := []string{"--partial"}
	if force {
		filterArgs = append(filterArgs, "--force")
	}
	filterArgs = append(filterArgs,
		"--email-callback", `import os; return os.environ["NEW_EMAIL_ENV"].encode("utf-8")`,
		"--name-callback", `import os; return os.environ["NEW_NAME_ENV"].encode("utf-8")`,
	)
	status := 0
	type authorApply struct {
		repo repo
	}
	type authorApplyResult struct {
		apply      authorApply
		output     string
		err        error
		restoreErr error
	}
	applies := []authorApply{}
	for _, r := range repos {
		applies = append(applies, authorApply{repo: r})
	}
	renderNotice(a, "Author Rewrite", []keyValueRow{
		{key: "Repositories", value: fmt.Sprintf("%d", len(applies))},
		{key: "New name", value: newName},
		{key: "New email", value: newEmail},
	}, nil)
	renderWarning(a, fmt.Sprintf("This operation rewrites Git history in %d repositories. A force push will be required to update any remote.", len(applies)))
	if !confirmOrSkip(a, yes, fmt.Sprintf("Rewrite author and committer identity in %d repositories?", len(applies))) {
		renderSummary(a,
			summaryCount{label: "rewritten", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: len(applies), color: a.ui.Yellow},
			summaryCount{label: "failed", value: 0, color: a.ui.Red},
		)
		return status
	}
	results := parallelItemsWithWorkersProgress(applies, gitMutationWorkerCount(len(applies)), newProgress(a, "Rewriting authors", len(applies)), func(apply authorApply) (string, string) {
		return apply.repo.display, apply.repo.display
	}, func(apply authorApply) authorApplyResult {
		out, err, restoreErr := runFilterRepoRestoringOrigin(a, apply.repo.dir, apply.repo.gitDir, filterCmd, filterArgs, []string{"NEW_EMAIL_ENV=" + newEmail, "NEW_NAME_ENV=" + newName})
		return authorApplyResult{apply: apply, output: out, err: err, restoreErr: restoreErr}
	})
	rewritten := 0
	failed := 0
	for _, result := range results {
		r := result.apply.repo
		if result.err == nil {
			if result.restoreErr != nil {
				renderErrorBlock(a, r.display+": author rewrite completed, but origin could not be restored", result.restoreErr.Error())
				status = 1
				failed++
				continue
			}
			rewritten++
			continue
		}
		renderErrorBlock(a, r.display+": could not update git author and committer information", result.output)
		if result.restoreErr != nil {
			renderErrorBlock(a, r.display+": author rewrite failed, and origin could not be restored", result.restoreErr.Error())
		}
		status = 1
		failed++
	}
	renderSummary(a,
		summaryCount{label: "rewritten", value: rewritten, color: a.ui.Green},
		summaryCount{label: "skipped", value: 0, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	return status
}
