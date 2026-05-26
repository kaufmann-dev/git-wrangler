package cli

import (
	"context"
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
	limit, _ := cmd.Flags().GetString("limit")
	into, _ := cmd.Flags().GetString("into")

	if user == "" {
		a.error("The --user option is required.")
		return 1
	}
	if !requireCommand(a, "gh", "clone") || !requireGit(a, "clone") {
		return 1
	}
	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt < 1 {
		a.error("--limit must be 1 or greater.")
		return 1
	}
	if visibility != "all" && visibility != "public" && visibility != "private" {
		a.error("Invalid visibility option. Use 'all', 'public', or 'private'.")
		return 1
	}
	if visibility == "private" || visibility == "all" {
		out, _ := githubcli.Capture(context.Background(), "", "auth", "status")
		if !regexp.MustCompile(`Logged in to .* account ` + regexp.QuoteMeta(user) + ` `).MatchString(out) {
			a.errorf("You are not logged in as the specified user: %s. Set --visibility to 'public' or use 'gh auth login'.", user)
			return 1
		}
	}

	listArgs := githubcli.RepoListArgs(user, visibility, "1")
	out, _ := githubcli.Stdout(context.Background(), "", listArgs...)
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

	listArgs = githubcli.RepoListArgs(user, visibility, limit)
	reposOut, err := githubcli.Stdout(context.Background(), "", listArgs...)
	if err != nil {
		fmt.Fprintf(a.stderr, "%sError: Failed to list repositories:\n%s%s\n\n", a.ui.Red, err.Error(), a.ui.Reset)
		return 1
	}
	for _, line := range splitLines(reposOut) {
		fields := strings.Split(line, "\t")
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		cloneRepository(a, fields[0], into)
	}
	return 0
}

func cloneRepository(a *app, fullName, into string) {
	repoName := fullName
	if idx := strings.LastIndex(fullName, "/"); idx >= 0 {
		repoName = fullName[idx+1:]
	}
	target := filepath.Join(into, repoName)
	if isDir(target) {
		abs, _ := filepath.Abs(into)
		fmt.Fprintf(a.stdout, "%s%s already exists in %s. Skipping...%s\n", a.ui.Yellow, repoName, abs, a.ui.Reset)
		return
	}
	if out, err := githubcli.Capture(context.Background(), "", "repo", "clone", fullName, target); err == nil {
		abs, _ := filepath.Abs(target)
		fmt.Fprintf(a.stdout, "%sCloned %s into %s%s\n", a.ui.Green, repoName, abs, a.ui.Reset)
	} else {
		fmt.Fprintf(a.stderr, "%sError: Failed to clone %s:\n%s%s\n\n", a.ui.Red, repoName, out, a.ui.Reset)
	}
}
