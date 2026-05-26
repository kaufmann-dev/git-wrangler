package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func runRemoveSecrets(a *app, cmd *cobra.Command, args []string) int {
	confirmed, _ := cmd.Flags().GetBool("confirm")
	if !requireGit(a, "remove-secrets") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "remove-secrets")
	if !ok {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	patterns := []string{".env", ".env.*", "*.pem", "*.key", "*.p12", "*.pfx", "id_rsa", "id_rsa.pub", "id_ed25519", "id_ed25519.pub", "config.json", "secrets.json", "credentials.json", "*.secret"}
	status := 0
	for _, r := range repos {
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "--is-inside-work-tree"); err != nil {
			fmt.Fprintf(a.stderr, "%sError: %s is not a valid or accessible git repository. Skipping...%s\n", a.ui.Red, r.display, a.ui.Reset)
			status = 1
			continue
		}
		matchedPatterns := []string{}
		matchedFiles := []string{}
		for _, pattern := range patterns {
			out, _ := runStdout(r.dir, nil, "git", "log", "--all", "--oneline", "--", pattern)
			if strings.TrimSpace(out) == "" {
				continue
			}
			matchedPatterns = append(matchedPatterns, pattern)
			files, _ := runStdout(r.dir, nil, "git", "log", "--all", "--format=", "--name-only", "--", pattern)
			matchedFiles = append(matchedFiles, splitLines(files)...)
		}
		matchedFiles = sortedUnique(matchedFiles)
		if len(matchedPatterns) == 0 {
			fmt.Fprintf(a.stdout, "%sNo target patterns found in history. Skipping %s cleanly...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stdout, "%sFound %d sensitive file(s) matching %d pattern(s) in %s:%s\n", a.ui.Yellow, len(matchedFiles), len(matchedPatterns), r.display, a.ui.Reset)
		for _, file := range matchedFiles {
			fmt.Fprintf(a.stdout, "  %s\n", file)
		}
		fmt.Fprintln(a.stdout)
		if !confirmed {
			a.error(r.display, "Refusing to rewrite history without --confirm.")
			status = 1
			continue
		}
		filterArgs := []string{}
		for _, pattern := range matchedPatterns {
			filterArgs = append(filterArgs, "--path-glob", pattern)
		}
		filterArgs = append(filterArgs, "--invert-paths", "--use-base-name", "--partial", "--force")
		remoteURL := strings.TrimSpace(mustStdout(r.dir, "git", "remote", "get-url", "origin"))
		if out, err := runFilterRepo(r.dir, filterCmd, filterArgs, nil); err == nil {
			fmt.Fprintf(a.stdout, "%sSuccessfully purged %d sensitive file(s) from %s%s\n", a.ui.Green, len(matchedFiles), r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stderr, "%sError: Rewrite failed for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
			status = 1
			continue
		}
		if remoteURL != "" {
			_, _ = runCapture(r.dir, nil, "git", "remote", "add", "origin", remoteURL)
		}
	}
	return status
}

func runFilterRepo(dir string, filterCmd []string, args []string, env []string) (string, error) {
	if len(filterCmd) == 0 {
		return "", errors.New("missing filter command")
	}
	return runCapture(dir, env, filterCmd[0], append(filterCmd[1:], args...)...)
}
