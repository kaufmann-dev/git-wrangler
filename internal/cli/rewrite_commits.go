package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func runRewriteCommits(a *app, cmd *cobra.Command, args []string) int {
	if len(args) > 0 {
		a.errorf("Unknown option: %s", args[0])
		return 1
	}
	if !requireGit(a, "rewrite-commits") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-commits")
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
	for _, r := range repos {
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "HEAD"); err != nil {
			fmt.Fprintf(a.stdout, "%sRepository has no commits in %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		remoteURL := strings.TrimSpace(mustStdout(r.dir, "git", "remote", "get-url", "origin"))
		mapping, err := buildCommitMessageMapping(r.dir)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not inspect commits for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			continue
		}
		if len(mapping) == 0 {
			fmt.Fprintf(a.stdout, "%sNo commits require rewriting in %s (already format compliant). Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		callback, err := writeCommitCallback(mapping)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not prepare commit callback for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			continue
		}
		out, err := runFilterRepo(r.dir, filterCmd, []string{"--partial", "--commit-callback", callback, "--force"}, nil)
		_ = os.Remove(callback)
		if err == nil {
			fmt.Fprintf(a.stdout, "%sRewrote commit messages for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
			if remoteURL != "" {
				_, _ = runCapture(r.dir, nil, "git", "remote", "add", "origin", remoteURL)
			}
		} else {
			fmt.Fprintf(a.stderr, "%sError: Could not update commit messages for %s:\n%s%s\n\n", a.ui.Red, r.display, out, a.ui.Reset)
		}
	}
	return 0
}

var (
	conventionalRe   = regexp.MustCompile(`^(feat|fix|docs|chore|test|build|ci|perf|refactor|style)(\(.*\))?: `)
	docsRe           = regexp.MustCompile(`(\.md$|\.txt$|\.rst$|^LICENSE|^docs/)`)
	testsRe          = regexp.MustCompile(`(^test/|^spec/|_test\.|spec\.|\.test\.)`)
	configRe         = regexp.MustCompile(`(^\.github/|^Makefile$|^Dockerfile$|\.yml$|^\w+\.json$)`)
	timezoneOffsetRe = regexp.MustCompile(`^[+-][0-9]{4}$`)
)

func buildCommitMessageMapping(dir string) (map[string]string, error) {
	out, err := runStdout(dir, nil, "git", "rev-list", "--all")
	if err != nil {
		return nil, err
	}
	mapping := map[string]string{}
	for _, commit := range splitLines(out) {
		msg, _ := runStdout(dir, nil, "git", "log", "-1", "--format=%B", commit)
		if conventionalRe.MatchString(firstLine(msg)) {
			continue
		}
		diff, _ := runStdout(dir, nil, "git", "diff-tree", "--root", "--no-commit-id", "--name-status", "-r", commit)
		if strings.TrimSpace(diff) == "" {
			continue
		}
		newMsg := categorizeCommit(diff)
		if newMsg != "" && newMsg != strings.TrimRight(msg, "\n") {
			mapping[commit] = newMsg
		}
	}
	return mapping, nil
}

func categorizeCommit(diff string) string {
	firstFile := ""
	hasDocs, hasTests, hasConfig, hasSrc := false, false, false, false
	additions, deletions, modifications := 0, 0, 0
	for _, line := range splitLines(diff) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		status := fields[0][0]
		file := fields[1]
		if firstFile == "" {
			firstFile = file
		}
		switch status {
		case 'A':
			additions++
		case 'D':
			deletions++
		default:
			modifications++
		}
		switch {
		case docsRe.MatchString(file):
			hasDocs = true
		case testsRe.MatchString(file):
			hasTests = true
		case configRe.MatchString(file):
			hasConfig = true
		default:
			hasSrc = true
		}
	}
	total := additions + deletions + modifications
	if total == 0 {
		return ""
	}
	typ := "chore"
	switch {
	case !hasSrc && !hasConfig && !hasTests && hasDocs:
		typ = "docs"
	case !hasSrc && !hasConfig && !hasDocs && hasTests:
		typ = "test"
	case !hasSrc && !hasTests && !hasDocs && hasConfig:
		typ = "chore"
	case additions > 0 && deletions == 0 && hasSrc:
		typ = "feat"
	case deletions > 0 && additions == 0 && modifications == 0:
		typ = "chore"
	case hasSrc && (modifications > 0 || (additions > 0 && deletions > 0)):
		typ = "fix"
	}
	target := firstFile
	if total > 1 {
		if strings.Contains(firstFile, "/") {
			target = firstFile[:strings.LastIndex(firstFile, "/")+1]
		} else {
			target = filepath.Base(firstFile)
		}
	}
	action := "update"
	if additions > 0 && deletions == 0 && modifications == 0 {
		action = "add"
	} else if deletions > 0 && additions == 0 && modifications == 0 {
		action = "remove"
	}
	return fmt.Sprintf("%s: %s %s", typ, action, target)
}

func writeCommitCallback(mapping map[string]string) (string, error) {
	f, err := os.CreateTemp("", "git-wrangler-commit-callback-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	fmt.Fprintln(f, "mapping = {}")
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(f, "mapping[b%q] = b%q\n", key, mapping[key]+"\n")
	}
	fmt.Fprintln(f, "if commit.original_id in mapping:")
	fmt.Fprintln(f, "    commit.message = mapping[commit.original_id]")
	return f.Name(), nil
}
