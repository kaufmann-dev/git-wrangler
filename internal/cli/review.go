package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func runReview(a *app, cmd *cobra.Command, args []string) int {
	if len(args) > 0 {
		a.errorf("Unknown option: %s", args[0])
		return 1
	}
	if !requireGit(a, "review") {
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
		unpushed, _ := runStdout(r.dir, nil, "git", "rev-list", "HEAD", "--not", "--remotes")
		commits := splitLines(unpushed)
		if len(commits) == 0 {
			fmt.Fprintf(a.stdout, "%sNo unpushed changes for %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		oldest := commits[len(commits)-1]
		base, err := runStdout(r.dir, nil, "git", "rev-parse", "--verify", oldest+"^")
		base = strings.TrimSpace(base)
		if err != nil || base == "" {
			base = strings.TrimSpace(mustStdout(r.dir, "git", "hash-object", "-t", "tree", "/dev/null"))
		}
		diff, _ := runStdout(r.dir, nil, "git", "diff", "--name-status", "-z", "--no-renames", base+"..HEAD")
		added, modified, deleted := parseNameStatusZ(diff)
		if len(added) == 0 && len(modified) == 0 && len(deleted) == 0 {
			fmt.Fprintf(a.stdout, "%sNo unpushed changes for %s. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		deletedFolders, individualDeleted := groupDeletedFiles(r.dir, deleted)
		fmt.Fprintf(a.stdout, "%s%s:%s\n", a.ui.RepoColor, r.display, a.ui.Reset)
		for _, f := range added {
			fmt.Fprintf(a.stdout, "  %sAdded:%s    %s\n", a.ui.Green, a.ui.Reset, f)
		}
		for _, f := range modified {
			fmt.Fprintf(a.stdout, "  %sEdited:%s   %s\n", a.ui.Yellow, a.ui.Reset, f)
		}
		for _, f := range deletedFolders {
			fmt.Fprintf(a.stderr, "  %sRemoved:%s  %s/ (entire folder)\n", a.ui.Red, a.ui.Reset, f)
		}
		for _, f := range individualDeleted {
			fmt.Fprintf(a.stderr, "  %sRemoved:%s  %s\n", a.ui.Red, a.ui.Reset, f)
		}
		fmt.Fprintln(a.stdout)
	}
	return 0
}

func parseNameStatusZ(data string) (added, modified, deleted []string) {
	parts := strings.Split(strings.TrimRight(data, "\x00"), "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		status := parts[i]
		file := parts[i+1]
		if status == "" {
			continue
		}
		switch status[0] {
		case 'A':
			added = append(added, file)
		case 'M':
			modified = append(modified, file)
		case 'D':
			deleted = append(deleted, file)
		}
	}
	return
}

func groupDeletedFiles(dir string, deleted []string) ([]string, []string) {
	deletedFolders := []string{}
	individual := []string{}
	for _, file := range deleted {
		covered := false
		for _, folder := range deletedFolders {
			if strings.HasPrefix(file, folder+"/") {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		parent := filepath.ToSlash(filepath.Dir(file))
		if parent == "." {
			individual = append(individual, file)
			continue
		}
		current := ""
		found := ""
		for _, part := range strings.Split(parent, "/") {
			if current == "" {
				current = part
			} else {
				current += "/" + part
			}
			if out, _ := runStdout(dir, nil, "git", "ls-tree", "-d", "HEAD", current); strings.TrimSpace(out) == "" {
				found = current
				break
			}
		}
		if found != "" {
			deletedFolders = append(deletedFolders, found)
		} else {
			individual = append(individual, file)
		}
	}
	return deletedFolders, individual
}
