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
	repos, err := resolveRepositoryTargets("")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	type reviewResult struct {
		repo              repo
		err               error
		errMessage        string
		added             []string
		modified          []string
		deletedFolders    []string
		individualDeleted []string
	}
	status := 0
	results := parallelRepos(repos, func(r repo) reviewResult {
		unpushed, err := a.git.Stdout(a.ctx, r.dir, nil, "rev-list", "HEAD", "--not", "--remotes")
		if err != nil {
			return reviewResult{repo: r, err: err, errMessage: "Could not list unpushed commits"}
		}
		commits := splitLines(unpushed)
		if len(commits) == 0 {
			return reviewResult{repo: r}
		}
		oldest := commits[len(commits)-1]
		base, err := a.git.Stdout(a.ctx, r.dir, nil, "rev-parse", "--verify", oldest+"^")
		base = strings.TrimSpace(base)
		if err != nil || base == "" {
			base = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
		}
		diff, err := a.git.Stdout(a.ctx, r.dir, nil, "diff", "--name-status", "-z", "--no-renames", base+"..HEAD")
		if err != nil {
			return reviewResult{repo: r, err: err, errMessage: "Could not inspect unpushed diff"}
		}
		added, modified, deleted := parseNameStatusZ(diff)
		if len(added) == 0 && len(modified) == 0 && len(deleted) == 0 {
			return reviewResult{repo: r}
		}
		deletedFolders, individualDeleted := groupDeletedFiles(a, r.dir, deleted)
		return reviewResult{repo: r, added: added, modified: modified, deletedFolders: deletedFolders, individualDeleted: individualDeleted}
	})
	for _, result := range results {
		if result.err != nil {
			fmt.Fprintf(a.stderr, "%sError: %s for %s:\n%s%s\n\n", a.ui.Red, result.errMessage, result.repo.display, result.err.Error(), a.ui.Reset)
			status = 1
			continue
		}
		if len(result.added) == 0 && len(result.modified) == 0 && len(result.deletedFolders) == 0 && len(result.individualDeleted) == 0 {
			fmt.Fprintf(a.stdout, "%sNo unpushed changes for %s. Skipping...%s\n", a.ui.Yellow, result.repo.display, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stdout, "%s%s:%s\n", a.ui.RepoColor, result.repo.display, a.ui.Reset)
		for _, f := range result.added {
			fmt.Fprintf(a.stdout, "  %sAdded:%s    %s\n", a.ui.Green, a.ui.Reset, f)
		}
		for _, f := range result.modified {
			fmt.Fprintf(a.stdout, "  %sEdited:%s   %s\n", a.ui.Yellow, a.ui.Reset, f)
		}
		for _, f := range result.deletedFolders {
			fmt.Fprintf(a.stderr, "  %sRemoved:%s  %s/ (entire folder)\n", a.ui.Red, a.ui.Reset, f)
		}
		for _, f := range result.individualDeleted {
			fmt.Fprintf(a.stderr, "  %sRemoved:%s  %s\n", a.ui.Red, a.ui.Reset, f)
		}
		fmt.Fprintln(a.stdout)
	}
	return status
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

func groupDeletedFiles(a *app, dir string, deleted []string) ([]string, []string) {
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
			if out, _ := a.git.Stdout(a.ctx, dir, nil, "ls-tree", "-d", "HEAD", current); strings.TrimSpace(out) == "" {
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
