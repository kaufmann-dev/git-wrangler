package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type renameBranchOptions struct {
	target    targetOptions
	oldBranch string
	newBranch string
}

type renameRepoOptions struct {
	target          targetOptions
	editDescription bool
}

func renameBranchOptionsFromCommand(a *app, cmd *cobra.Command) (renameBranchOptions, bool) {
	oldBranch, ok := requiredStringFlag(a, cmd, "oldbranch", "Existing branch name: ")
	if !ok {
		return renameBranchOptions{}, false
	}
	newBranch, ok := requiredStringFlag(a, cmd, "newbranch", "New branch name: ")
	if !ok {
		return renameBranchOptions{}, false
	}
	return renameBranchOptions{
		target:    targetOptionsFromCommand(cmd),
		oldBranch: oldBranch,
		newBranch: newBranch,
	}, true
}

func renameRepoOptionsFromCommand(cmd *cobra.Command) renameRepoOptions {
	return renameRepoOptions{
		target:          targetOptionsFromCommand(cmd),
		editDescription: boolFlagValue(cmd, "description"),
	}
}

func runRenameBranch(a *app, cmd *cobra.Command, args []string) int {
	opts, ok := renameBranchOptionsFromCommand(a, cmd)
	if !ok {
		return 1
	}
	if !requireGit(a, "rename-branch") {
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
	type renameBranchResult struct {
		repo       repo
		out        string
		err        error
		skipReason string
		renamed    bool
		failed     bool
		accessible bool
		validRepo  bool
	}
	status := 0
	renamed := 0
	skipped := 0
	failed := 0
	results := parallelGitMutationsProgress(a.ctx, repos, newProgress(a, "Renaming branches", len(repos)), func(r repo) renameBranchResult {
		if _, err := os.Stat(r.dir); err != nil {
			return renameBranchResult{repo: r, err: err, failed: true}
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "rev-parse", "--is-inside-work-tree"); err != nil {
			return renameBranchResult{repo: r, out: out, err: err, failed: true, accessible: true}
		}
		if !a.git.VerifyRef(a.ctx, r.dir, "refs/heads/"+opts.oldBranch) {
			return renameBranchResult{repo: r, skipReason: fmt.Sprintf("old branch '%s' does not exist", opts.oldBranch), accessible: true, validRepo: true}
		}
		if a.git.VerifyRef(a.ctx, r.dir, "refs/heads/"+opts.newBranch) {
			return renameBranchResult{repo: r, skipReason: fmt.Sprintf("new branch '%s' already exists", opts.newBranch), accessible: true, validRepo: true}
		}
		if out, err := a.git.Capture(a.ctx, r.dir, nil, "branch", "-m", opts.oldBranch, opts.newBranch); err == nil {
			return renameBranchResult{repo: r, renamed: true, accessible: true, validRepo: true}
		} else {
			return renameBranchResult{repo: r, out: out, err: err, failed: true, accessible: true, validRepo: true}
		}
	})
	if interrupted(a) {
		return 1
	}
	for _, result := range results {
		switch {
		case result.failed && !result.accessible:
			renderErrorBlock(a, result.repo.display+": directory is inaccessible", result.err.Error())
			status = 1
			failed++
		case result.failed && !result.validRepo:
			renderErrorBlock(a, result.repo.display+": not a valid git repository", outputOrError(result.out, result.err))
			status = 1
			failed++
		case result.failed:
			renderErrorBlock(a, result.repo.display+": failed to rename branch", outputOrError(result.out, result.err))
			status = 1
			failed++
		case result.renamed:
			renamed++
		default:
			renderStatusLine(a, a.stdout, statusSkip, result.repo.display, result.skipReason)
			skipped++
		}
	}
	renderSummary(a,
		summaryCount{label: "renamed", value: renamed, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	return status
}

func runRenameRepo(a *app, cmd *cobra.Command, args []string) int {
	if !requireInteractive(a, "rename-repo") {
		return 1
	}
	opts := renameRepoOptionsFromCommand(cmd)
	if !requireGit(a, "rename-repo") || !requireCommand(a, "gh", "rename-repo") {
		return 1
	}
	ghEnv, authSource, ok, err := githubAuthEnv(a)
	if err != nil {
		if errors.Is(err, errGitHubCredentialStorageUnavailable) {
			a.error("Secure credential storage is unavailable. Set GIT_WRANGLER_GITHUB_TOKEN.")
		} else {
			a.error(err.Error())
		}
		return 1
	}
	if !ok {
		a.error("Git Wrangler GitHub auth is required for rename-repo. Run 'git-wrangler init' or 'git-wrangler config set github.auth'.")
		return 1
	}
	if err := a.gh.ValidateAuth(a.ctx, ghEnv); err != nil {
		a.plainErrorf("GitHub authentication failed: %s", err.Error())
		return 1
	}
	renderStatusLine(a, a.stdout, statusInfo, "GitHub auth", string(authSource))
	repos, err := opts.target.repositories()
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	status := 0
	renamed := 0
	descriptionUpdated := 0
	skipped := 0
	failed := 0
	for i, r := range repos {
		oldName, err := a.gh.StdoutEnv(a.ctx, r.dir, ghEnv, "repo", "view", "--json", "name", "-q", ".name")
		if err != nil {
			renderStatusLine(a, a.stdout, statusSkip, r.display, "no remote or not a GitHub repository")
			skipped++
			continue
		}
		oldName = strings.TrimSpace(oldName)
		if i > 0 {
			fmt.Fprintln(a.stdout)
		}
		renderRepoHeader(a, oldName)
		newName, err := promptRead(a, "New name (leave blank to skip): ")
		if errors.Is(err, errPromptCancelled) {
			return 1
		}
		newDesc := ""
		if opts.editDescription {
			oldDesc, _ := a.gh.StdoutEnv(a.ctx, r.dir, ghEnv, "repo", "view", "--json", "description", "-q", ".description")
			oldDesc = strings.TrimSpace(oldDesc)
			if oldDesc == "" {
				fmt.Fprintln(a.stdout, "Current description: <none>")
			} else {
				fmt.Fprintf(a.stdout, "Current description: %s\n", oldDesc)
			}
			newDesc, err = promptRead(a, "New description (leave blank to skip): ")
			if errors.Is(err, errPromptCancelled) {
				return 1
			}
		}
		if opts.editDescription && newDesc != "" {
			if out, err := a.gh.CaptureEnv(a.ctx, r.dir, ghEnv, "repo", "edit", "--description", newDesc); err == nil {
				renderStatusLine(a, a.stdout, statusOK, oldName, "description updated")
				descriptionUpdated++
			} else {
				renderErrorBlock(a, oldName+": failed to update description", out)
				status = 1
				failed++
			}
		}
		if newName != "" {
			if out, err := a.gh.CaptureEnv(a.ctx, r.dir, ghEnv, "repo", "rename", newName, "--yes"); err == nil {
				renderStatusLine(a, a.stdout, statusOK, oldName, "renamed to "+newName)
				renamed++
			} else {
				renderErrorBlock(a, oldName+": failed to rename", out)
				status = 1
				failed++
			}
		}
		if newName == "" && (!opts.editDescription || newDesc == "") {
			skipped++
		}
	}
	renderSummary(a,
		summaryCount{label: "renamed", value: renamed, color: a.ui.Green},
		summaryCount{label: "description updated", value: descriptionUpdated, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	return status
}
