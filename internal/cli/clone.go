package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/githubcli"
	"github.com/spf13/cobra"
)

func runClone(a *app, cmd *cobra.Command, args []string) int {
	visibility, _ := cmd.Flags().GetString("visibility")
	user, _ := cmd.Flags().GetString("user")
	limitInt, _ := cmd.Flags().GetInt("limit")
	into, _ := cmd.Flags().GetString("into")

	var ok bool
	user, ok = requiredStringFlag(a, cmd, "user", "GitHub user or organization: ")
	if !ok {
		return 1
	}
	if !requireCommand(a, "gh", "clone") || !requireGit(a, "clone") {
		return 1
	}
	if limitInt < 1 {
		a.error("--limit must be 1 or greater.")
		return 1
	}
	if visibility != "all" && visibility != "public" && visibility != "private" {
		a.error("Invalid visibility option. Use 'all', 'public', or 'private'.")
		return 1
	}
	if visibility == "private" || visibility == "all" {
		out, err := a.gh.Capture(a.ctx, "", "auth", "status")
		if err != nil {
			a.errorf("Could not verify GitHub authentication for '%s'. Set --visibility to 'public' or use 'gh auth login'.", user)
			return 1
		}
		if !regexp.MustCompile(`Logged in to .* account ` + regexp.QuoteMeta(user) + ` `).MatchString(out) {
			a.errorf("You are not logged in as the specified user: %s. Set --visibility to 'public' or use 'gh auth login'.", user)
			return 1
		}
	}

	listArgs := githubcli.RepoListArgs(user, visibility, "1")
	out, err := a.gh.Stdout(a.ctx, "", listArgs...)
	if err != nil {
		fmt.Fprintf(a.stderr, "%sError: Failed to list repositories:\n%s%s\n\n", a.ui.Red, err.Error(), a.ui.Reset)
		return 1
	}
	if lineCount(out) == 0 {
		if visibility == "public" || visibility == "private" {
			a.errorf("No %s repositories found for '%s'.", visibility, user)
		} else {
			a.errorf("No repositories found for '%s'.", user)
		}
		return 1
	}

	if into == "" {
		into = user
	}
	if info, err := os.Stat(into); err == nil && !info.IsDir() {
		a.errorf("Unable to create or access the specified directory '%s'.", into)
		return 1
	}
	if err := os.MkdirAll(into, 0o755); err != nil {
		a.errorf("Unable to create or access the specified directory '%s'.", into)
		return 1
	}

	listArgs = githubcli.RepoListArgs(user, visibility, strconv.Itoa(limitInt))
	reposOut, err := a.gh.Stdout(a.ctx, "", listArgs...)
	if err != nil {
		fmt.Fprintf(a.stderr, "%sError: Failed to list repositories:\n%s%s\n\n", a.ui.Red, err.Error(), a.ui.Reset)
		return 1
	}
	status := 0
	for _, line := range splitLines(reposOut) {
		fields := strings.Split(line, "\t")
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		if !cloneRepository(a, fields[0], into) {
			status = 1
		}
	}
	return status
}

func cloneRepository(a *app, fullName, into string) bool {
	repoName := fullName
	if idx := strings.LastIndex(fullName, "/"); idx >= 0 {
		repoName = fullName[idx+1:]
	}
	target := filepath.Join(into, repoName)
	if isDir(target) {
		abs, _ := filepath.Abs(into)
		fmt.Fprintf(a.stdout, "%s%s already exists in %s. Skipping...%s\n", a.ui.Yellow, repoName, abs, a.ui.Reset)
		return true
	}
	if out, err := a.gh.Capture(a.ctx, "", "repo", "clone", fullName, target); err == nil {
		abs, _ := filepath.Abs(target)
		fmt.Fprintf(a.stdout, "%sCloned %s into %s%s\n", a.ui.Green, repoName, abs, a.ui.Reset)
		return true
	} else {
		fmt.Fprintf(a.stderr, "%sError: Failed to clone %s:\n%s%s\n\n", a.ui.Red, repoName, out, a.ui.Reset)
		return false
	}
}
