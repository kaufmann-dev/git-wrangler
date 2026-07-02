package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type reviewOptions struct {
	target targetOptions
	json   jsonOptions
	fetch  fetchOptions
}

func reviewOptionsFromCommand(cmd *cobra.Command) reviewOptions {
	return reviewOptions{
		target: targetOptionsFromCommand(cmd),
		json:   jsonOptionsFromCommand(cmd),
		fetch:  fetchOptionsFromCommand(cmd),
	}
}

func runReview(a *app, cmd *cobra.Command, args []string) int {
	if len(args) > 0 {
		a.errorf("Unknown option: %s", args[0])
		return 1
	}
	opts := reviewOptionsFromCommand(cmd)
	if opts.json.enabled {
		return runReviewJSON(a, opts)
	}
	if !requireGit(a, "review") {
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
	fetchFailures := map[string]originRefreshResult{}
	if !opts.fetch.noFetch {
		fetchFailures = refreshFailuresByDir(refreshOrigin(a, repos))
		if interrupted(a) {
			return 1
		}
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
	changed := 0
	clean := 0
	failed := 0
	results := parallelReposProgress(a.ctx, repos, newProgress(a, "Reviewing repositories", len(repos)), func(r repo) reviewResult {
		if failure, ok := fetchFailures[r.dir]; ok {
			return reviewResult{repo: r, err: fmt.Errorf("%s", fetchFailureMessage(failure)), errMessage: "Could not fetch origin"}
		}
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
	if interrupted(a) {
		return 1
	}
	for _, result := range results {
		if result.err != nil {
			renderErrorBlock(a, fmt.Sprintf("%s: %s", result.repo.display, result.errMessage), result.err.Error())
			status = 1
			failed++
			continue
		}
		if len(result.added) == 0 && len(result.modified) == 0 && len(result.deletedFolders) == 0 && len(result.individualDeleted) == 0 {
			clean++
			continue
		}
		changed++
		renderRepoHeader(a, result.repo.display)
		for _, f := range result.added {
			fmt.Fprintf(a.stdout, "  %sAdded:%s    %s\n", a.ui.Green, a.ui.Reset, f)
		}
		for _, f := range result.modified {
			fmt.Fprintf(a.stdout, "  %sEdited:%s   %s\n", a.ui.Yellow, a.ui.Reset, f)
		}
		for _, f := range result.deletedFolders {
			fmt.Fprintf(a.stdout, "  %sRemoved:%s  %s/ (entire folder)\n", a.ui.Red, a.ui.Reset, f)
		}
		for _, f := range result.individualDeleted {
			fmt.Fprintf(a.stdout, "  %sRemoved:%s  %s\n", a.ui.Red, a.ui.Reset, f)
		}
		fmt.Fprintln(a.stdout)
	}
	renderSummary(a,
		summaryCount{label: "with unpushed changes", value: changed, color: a.ui.Yellow},
		summaryCount{label: "clean", value: clean, color: a.ui.Green},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	return status
}

func runReviewJSON(a *app, opts reviewOptions) int {
	if _, err := a.runner.LookPath("git"); err != nil {
		return writeJSONStatus(a, map[string]any{
			"ok":      false,
			"summary": map[string]any{"repositories": 0, "failed": 1},
			"error":   jsonError{Message: "'git' is required for review. Install it and make sure it is on PATH."},
		}, 1)
	}
	repos, err := opts.target.repositories()
	if err != nil {
		return writeJSONStatus(a, map[string]any{
			"ok":      false,
			"summary": map[string]any{"repositories": 0, "failed": 1},
			"error":   jsonError{Message: err.Error()},
		}, 1)
	}
	fetchFailures := map[string]originRefreshResult{}
	if !opts.fetch.noFetch {
		fetchFailures = refreshFailuresByDir(refreshOrigin(a, repos))
		if a.ctx.Err() != nil {
			return writeJSONStatus(a, map[string]any{
				"ok":      false,
				"summary": map[string]any{"repositories": len(repos), "failed": len(repos)},
				"error":   jsonError{Message: "operation cancelled"},
			}, 1)
		}
	}
	type reviewJSONRepo struct {
		Name            string     `json:"name"`
		Path            string     `json:"path"`
		Added           []string   `json:"added,omitempty"`
		Modified        []string   `json:"modified,omitempty"`
		DeletedFolders  []string   `json:"deletedFolders,omitempty"`
		DeletedFiles    []string   `json:"deletedFiles,omitempty"`
		UnpushedChanges bool       `json:"unpushedChanges"`
		Error           *jsonError `json:"error,omitempty"`
	}
	results := parallelRepos(a.ctx, repos, func(r repo) reviewJSONRepo {
		row := reviewJSONRepo{Name: r.display, Path: r.dir}
		if failure, ok := fetchFailures[r.dir]; ok {
			row.Error = &jsonError{Message: fetchFailureMessage(failure)}
			return row
		}
		unpushed, err := a.git.Stdout(a.ctx, r.dir, nil, "rev-list", "HEAD", "--not", "--remotes")
		if err != nil {
			row.Error = &jsonError{Message: "could not list unpushed commits: " + err.Error()}
			return row
		}
		commits := splitLines(unpushed)
		if len(commits) == 0 {
			return row
		}
		oldest := commits[len(commits)-1]
		base, err := a.git.Stdout(a.ctx, r.dir, nil, "rev-parse", "--verify", oldest+"^")
		base = strings.TrimSpace(base)
		if err != nil || base == "" {
			base = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
		}
		diff, err := a.git.Stdout(a.ctx, r.dir, nil, "diff", "--name-status", "-z", "--no-renames", base+"..HEAD")
		if err != nil {
			row.Error = &jsonError{Message: "could not inspect unpushed diff: " + err.Error()}
			return row
		}
		added, modified, deleted := parseNameStatusZ(diff)
		deletedFolders, individualDeleted := groupDeletedFiles(a, r.dir, deleted)
		row.Added = added
		row.Modified = modified
		row.DeletedFolders = deletedFolders
		row.DeletedFiles = individualDeleted
		row.UnpushedChanges = len(added) > 0 || len(modified) > 0 || len(deletedFolders) > 0 || len(individualDeleted) > 0
		return row
	})
	if a.ctx.Err() != nil {
		return writeJSONStatus(a, map[string]any{
			"ok":      false,
			"summary": map[string]any{"repositories": len(repos), "failed": len(repos)},
			"error":   jsonError{Message: "operation cancelled"},
		}, 1)
	}
	failed := 0
	changed := 0
	for _, result := range results {
		if result.Error != nil {
			failed++
		}
		if result.UnpushedChanges {
			changed++
		}
	}
	code := 0
	if failed > 0 {
		code = 1
	}
	_ = writeJSON(a, map[string]any{
		"ok":           code == 0,
		"summary":      map[string]int{"repositories": len(results), "changed": changed, "failed": failed},
		"repositories": results,
	})
	return code
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
