package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func runRewriteAuthors(a *app, cmd *cobra.Command, args []string) int {
	force, _ := cmd.Flags().GetBool("force")
	confirmed, _ := cmd.Flags().GetBool("confirm")
	repoName, _ := cmd.Flags().GetString("repo")
	newName, _ := cmd.Flags().GetString("name")
	newEmail, _ := cmd.Flags().GetString("email")
	if newName == "" || newEmail == "" {
		fmt.Fprintf(a.stderr, "%sError: Both --name and --email options must be provided.%s\n", a.ui.Red, a.ui.Reset)
		return 1
	}
	if !requireGit(a, "rewrite-authors") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-authors")
	if !ok {
		return 1
	}
	root := "."
	if repoName != "" {
		root = repoName
	}
	repos, err := findGitRepositories(root)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	if !confirmed {
		a.error("Refusing to rewrite history without --confirm.")
		return 1
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
	for _, r := range repos {
		fmt.Fprintf(a.stderr, "%sWARNING: This operation rewrites Git history. A force push will be required to update any remote.%s\n", a.ui.Red, a.ui.Reset)
		remoteURL := originURL(r.dir)
		out, err := runFilterRepo(r.dir, filterCmd, filterArgs, []string{"NEW_EMAIL_ENV=" + newEmail, "NEW_NAME_ENV=" + newName})
		if err == nil {
			if err := restoreOrigin(r.dir, remoteURL); err != nil {
				fmt.Fprintf(a.stderr, "%sWarning: Author rewrite completed for %s, but origin could not be restored:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
				status = 1
				continue
			}
			fmt.Fprintf(a.stdout, "%sAuthor and committer information updated for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not update git author and committer information for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			status = 1
		}
	}
	return status
}
