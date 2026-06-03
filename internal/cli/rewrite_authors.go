package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func runRewriteAuthors(a *app, cmd *cobra.Command, args []string) int {
	force, _ := cmd.Flags().GetBool("force")
	yes := yesFlag(cmd)
	repoName, _ := cmd.Flags().GetString("repo")
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
	repos, err := resolveRepositoryTargets(repoName)
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
		fmt.Fprintf(a.stderr, "%sWARNING: This operation rewrites Git history. A force push will be required to update any remote.%s\n", a.ui.Red, a.ui.Reset)
		if !yes && !confirm(a, "Rewrite author and committer identity for "+r.display+"?") {
			a.error(r.display, "Refusing to rewrite history without confirmation.")
			status = 1
			continue
		}
		applies = append(applies, authorApply{repo: r})
	}
	results := parallelItemsWithWorkersProgress(applies, gitMutationWorkerCount(len(applies)), newProgress(a, "Rewriting authors", len(applies)), func(apply authorApply) (string, string) {
		return apply.repo.display, apply.repo.display
	}, func(apply authorApply) authorApplyResult {
		out, err, restoreErr := runFilterRepoRestoringOrigin(a, apply.repo.dir, filterCmd, filterArgs, []string{"NEW_EMAIL_ENV=" + newEmail, "NEW_NAME_ENV=" + newName})
		return authorApplyResult{apply: apply, output: out, err: err, restoreErr: restoreErr}
	})
	for _, result := range results {
		r := result.apply.repo
		if result.err == nil {
			if result.restoreErr != nil {
				fmt.Fprintf(a.stderr, "%sWarning: Author rewrite completed for %s, but origin could not be restored:\n%s%s\n\n", a.ui.Red, r.display, result.restoreErr.Error(), a.ui.Reset)
				status = 1
				continue
			}
			fmt.Fprintf(a.stdout, "%sAuthor and committer information updated for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stderr, "%sError: Could not update git author and committer information for %s:\n%s%s\n\n", a.ui.Red, r.display, result.output, a.ui.Reset)
		if result.restoreErr != nil {
			fmt.Fprintf(a.stderr, "%sWarning: Author rewrite failed for %s, and origin could not be restored:\n%s%s\n\n", a.ui.Red, r.display, result.restoreErr.Error(), a.ui.Reset)
		}
		status = 1
	}
	return status
}
