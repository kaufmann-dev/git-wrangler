package cli

import (
	"fmt"
	"strings"
)

type currentRewriteDateBounds struct {
	afterDate  string
	beforeDate string
	hasAfter   bool
	after      int64
	hasBefore  bool
	before     int64
}

type currentRewriteDateSelectionScan struct {
	repo       repo
	err        error
	errLabel   string
	hasHead    bool
	noBranches bool
	noCommits  bool
	branches   []dateBranchRef
	commits    []rewriteDateCommit
	selected   []int
}

func parseCurrentRewriteDateBounds(afterDate, beforeDate string) (currentRewriteDateBounds, error) {
	bounds := currentRewriteDateBounds{
		afterDate:  afterDate,
		beforeDate: beforeDate,
	}
	if bounds.afterDate != "" {
		if !validDate(bounds.afterDate) {
			return currentRewriteDateBounds{}, fmt.Errorf("--rewrite-after must be in YYYY-MM-DD format.")
		}
		bounds.hasAfter = true
		bounds.after = parseDateStart(bounds.afterDate)
	}
	if bounds.beforeDate != "" {
		if !validDate(bounds.beforeDate) {
			return currentRewriteDateBounds{}, fmt.Errorf("--rewrite-before must be in YYYY-MM-DD format.")
		}
		bounds.hasBefore = true
		bounds.before = parseDateStart(bounds.beforeDate)
	}
	if bounds.hasAfter && bounds.hasBefore && bounds.after >= bounds.before {
		return currentRewriteDateBounds{}, fmt.Errorf("--rewrite-after must be before --rewrite-before.")
	}
	return bounds, nil
}

func (bounds currentRewriteDateBounds) enabled() bool {
	return bounds.hasAfter || bounds.hasBefore
}

func (bounds currentRewriteDateBounds) selectsCurrentAuthorDate(commit rewriteDateCommit) bool {
	epoch := commit.authorEpoch
	if bounds.hasAfter && epoch < bounds.after {
		return false
	}
	if bounds.hasBefore && epoch >= bounds.before {
		return false
	}
	return true
}

func selectCurrentAuthorDateCommitIndexes(commits []rewriteDateCommit, bounds currentRewriteDateBounds) []int {
	selected := []int{}
	for i := range commits {
		if bounds.selectsCurrentAuthorDate(commits[i]) {
			selected = append(selected, i)
		}
	}
	return selected
}

func selectedRewriteDateHashes(commits []rewriteDateCommit, selected []int) []string {
	hashes := make([]string, 0, len(selected))
	for _, index := range selected {
		if index >= 0 && index < len(commits) {
			hashes = append(hashes, commits[index].hash)
		}
	}
	return hashes
}

func selectedRewriteDateHashSet(commits []rewriteDateCommit, selected []int) map[string]bool {
	hashes := map[string]bool{}
	for _, hash := range selectedRewriteDateHashes(commits, selected) {
		hashes[hash] = true
	}
	return hashes
}

func scanCurrentRewriteDateSelection(a *app, r repo, bounds currentRewriteDateBounds) currentRewriteDateSelectionScan {
	if !a.git.HasHead(a.ctx, r.dir) {
		return currentRewriteDateSelectionScan{repo: r}
	}
	branches, err := localBranchRefs(a, r.dir)
	if err != nil {
		return currentRewriteDateSelectionScan{repo: r, hasHead: true, err: err, errLabel: "could not list local branches"}
	}
	if len(branches) == 0 {
		return currentRewriteDateSelectionScan{repo: r, hasHead: true, noBranches: true}
	}
	commits, err := collectRewriteDateCommits(a, r.dir, branchRefNames(branches))
	if err != nil {
		return currentRewriteDateSelectionScan{repo: r, hasHead: true, err: err, errLabel: "could not read commit metadata"}
	}
	if len(commits) == 0 {
		return currentRewriteDateSelectionScan{repo: r, hasHead: true, noCommits: true}
	}
	return currentRewriteDateSelectionScan{
		repo:     r,
		hasHead:  true,
		branches: branches,
		commits:  commits,
		selected: selectCurrentAuthorDateCommitIndexes(commits, bounds),
	}
}

func currentRewriteDateBoundsDescription(bounds currentRewriteDateBounds) string {
	parts := []string{}
	if bounds.afterDate != "" {
		parts = append(parts, "on or after "+bounds.afterDate)
	}
	if bounds.beforeDate != "" {
		parts = append(parts, "before "+bounds.beforeDate)
	}
	if len(parts) == 0 {
		return "all local branch commits"
	}
	return strings.Join(parts, ", ")
}
