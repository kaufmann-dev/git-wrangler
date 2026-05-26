package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func runRenameBranch(a *app, cmd *cobra.Command, args []string) int {
	oldBranch, ok := requiredStringFlag(a, cmd, "oldbranch", "Existing branch name: ")
	if !ok {
		return 1
	}
	newBranch, ok := requiredStringFlag(a, cmd, "newbranch", "New branch name: ")
	if !ok {
		return 1
	}
	if !requireGit(a, "rename-branch") {
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
	status := 0
	for _, r := range repos {
		if _, err := os.Stat(r.dir); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Directory is inaccessible: %s%s\n", a.ui.Red, r.display, a.ui.Reset)
			status = 1
			continue
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "rev-parse", "--is-inside-work-tree"); err != nil {
			fmt.Fprintf(a.stderr, "%sError: Not a valid git repository for %s:\n%s%s\n", a.ui.Red, r.display, out, a.ui.Reset)
			status = 1
			continue
		}
		if !a.git.VerifyRef(a.ctx, r.dir, "refs/heads/"+oldBranch) {
			fmt.Fprintf(a.stdout, "%sOld branch '%s' does not exist in %s. Skipping...%s\n", a.ui.Yellow, oldBranch, r.display, a.ui.Reset)
			continue
		}
		if a.git.VerifyRef(a.ctx, r.dir, "refs/heads/"+newBranch) {
			fmt.Fprintf(a.stdout, "%sNew branch '%s' already exists in %s. Skipping...%s\n", a.ui.Yellow, newBranch, r.display, a.ui.Reset)
			continue
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "branch", "-m", oldBranch, newBranch); err == nil {
			fmt.Fprintf(a.stdout, "%sBranch renamed from '%s' to '%s' for %s%s\n", a.ui.Green, oldBranch, newBranch, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Failed to rename branch in %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			status = 1
		}
	}
	return status
}

func runRenameRepo(a *app, cmd *cobra.Command, args []string) int {
	editDescription, _ := cmd.Flags().GetBool("description")
	if !requireGit(a, "rename-repo") || !requireCommand(a, "gh", "rename-repo") {
		return 1
	}
	repos, err := resolveRepositoryTargets("")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		a.warn("No Git repositories found under the current directory.")
		return 0
	}
	status := 0
	for _, r := range repos {
		oldName, err := a.gh.Stdout(a.ctx, r.dir, "repo", "view", "--json", "name", "-q", ".name")
		if err != nil {
			fmt.Fprintf(a.stdout, "%sSkipping %s: No remote or not a GitHub repository.%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		oldName = strings.TrimSpace(oldName)
		fmt.Fprintf(a.stdout, "\n%sRepository: %s%s\n", a.ui.RepoColor, oldName, a.ui.Reset)
		newName, _ := promptRead(a, "Enter new name (leave blank to skip): ")
		newDesc := ""
		if editDescription {
			oldDesc, _ := a.gh.Stdout(a.ctx, r.dir, "repo", "view", "--json", "description", "-q", ".description")
			oldDesc = strings.TrimSpace(oldDesc)
			if oldDesc == "" {
				fmt.Fprintln(a.stdout, "Current description: <None>")
			} else {
				fmt.Fprintf(a.stdout, "Current description: %s\n", oldDesc)
			}
			newDesc, _ = promptRead(a, "Enter new description (leave blank to skip): ")
		}
		if editDescription && newDesc != "" {
			if out, err := a.gh.Capture(a.ctx, r.dir, "repo", "edit", "--description", newDesc); err == nil {
				fmt.Fprintf(a.stdout, "%sSuccessfully updated description for %s%s\n", a.ui.Green, oldName, a.ui.Reset)
			} else {
				fmt.Fprintf(a.stderr, "%sError: Failed to update description for %s:\n%s%s\n", a.ui.Red, oldName, out, a.ui.Reset)
				status = 1
			}
		}
		if newName != "" {
			if out, err := a.gh.Capture(a.ctx, r.dir, "repo", "rename", newName, "--yes"); err == nil {
				fmt.Fprintf(a.stdout, "%sSuccessfully renamed %s to %s%s\n", a.ui.Green, oldName, newName, a.ui.Reset)
			} else {
				fmt.Fprintf(a.stderr, "%sError: Failed to rename %s:\n%s%s\n", a.ui.Red, oldName, out, a.ui.Reset)
				status = 1
			}
		}
	}
	return status
}
