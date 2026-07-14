package cli

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/ai"
	"github.com/kaufmann-dev/git-wrangler/internal/git"
	"github.com/kaufmann-dev/git-wrangler/internal/trailers"
	"github.com/spf13/cobra"
)

type coauthorOperation string

const (
	coauthorAdd     coauthorOperation = "add"
	coauthorReplace coauthorOperation = "replace"
	coauthorRemove  coauthorOperation = "remove"
)

type rewriteCoauthorsOptions struct {
	target       targetOptions
	fetch        fetchOptions
	confirmation confirmationOptions
	bounds       currentRewriteDateBounds
	operation    coauthorOperation
	coauthors    []trailers.Identity
	oldEmail     string
	emails       []string
	removeAll    bool
}

type coauthorCommit struct {
	hash        string
	parents     []string
	authorEpoch int64
	message     string
}

type coauthorMessageMapping struct {
	hash    string
	message string
}

type coauthorScan struct {
	repo             repo
	branches         []dateBranchRef
	mappings         []coauthorMessageMapping
	skippedGenerated int
	skipped          bool
	err              error
}

type coauthorApply struct {
	repo     repo
	branches []dateBranchRef
	mappings []coauthorMessageMapping
}

type coauthorApplyResult struct {
	apply      coauthorApply
	output     string
	err        error
	restoreErr error
}

func runRewriteCoauthorsAdd(a *app, cmd *cobra.Command, args []string) int {
	return runRewriteCoauthors(a, cmd, coauthorAdd)
}

func runRewriteCoauthorsReplace(a *app, cmd *cobra.Command, args []string) int {
	return runRewriteCoauthors(a, cmd, coauthorReplace)
}

func runRewriteCoauthorsRemove(a *app, cmd *cobra.Command, args []string) int {
	return runRewriteCoauthors(a, cmd, coauthorRemove)
}

func rewriteCoauthorsOptionsFromCommand(a *app, cmd *cobra.Command, operation coauthorOperation) (rewriteCoauthorsOptions, bool) {
	boundOpts, err := rewriteBoundOptionsFromCommand(cmd)
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return rewriteCoauthorsOptions{}, false
	}
	opts := rewriteCoauthorsOptions{
		target:       targetOptionsFromCommand(cmd),
		fetch:        fetchOptionsFromCommand(cmd),
		confirmation: confirmationOptionsFromCommand(cmd),
		bounds:       boundOpts.bounds,
		operation:    operation,
	}

	switch operation {
	case coauthorAdd:
		values := stringArrayFlagValues(cmd, "coauthor")
		if len(values) == 0 {
			a.plainErrorf("--coauthor is required.")
			return rewriteCoauthorsOptions{}, false
		}
		opts.coauthors, err = trailers.ValidateIdentities(values)
	case coauthorReplace:
		opts.oldEmail = stringFlagValue(cmd, "email")
		if opts.oldEmail == "" {
			a.plainErrorf("--email is required.")
			return rewriteCoauthorsOptions{}, false
		}
		replacement := stringFlagValue(cmd, "coauthor")
		if replacement == "" {
			a.plainErrorf("--coauthor is required.")
			return rewriteCoauthorsOptions{}, false
		}
		if err = trailers.ValidateEmail(opts.oldEmail); err == nil {
			var identity trailers.Identity
			identity, err = trailers.ParseIdentity(replacement)
			if err == nil {
				opts.coauthors = []trailers.Identity{identity}
			}
		}
	case coauthorRemove:
		opts.emails = stringArrayFlagValues(cmd, "email")
		opts.removeAll = boolFlagValue(cmd, "all")
		if opts.removeAll == (len(opts.emails) > 0) {
			a.plainErrorf("specify either --email or --all.")
			return rewriteCoauthorsOptions{}, false
		}
		seen := map[string]bool{}
		for _, email := range opts.emails {
			if err = trailers.ValidateEmail(email); err != nil {
				break
			}
			key := strings.ToLower(email)
			if seen[key] {
				err = fmt.Errorf("duplicate email %q", email)
				break
			}
			seen[key] = true
		}
	}
	if err != nil {
		a.plainErrorf("%s.", err.Error())
		return rewriteCoauthorsOptions{}, false
	}
	return opts, true
}

func runRewriteCoauthors(a *app, cmd *cobra.Command, operation coauthorOperation) int {
	opts, ok := rewriteCoauthorsOptionsFromCommand(a, cmd, operation)
	if !ok {
		return 1
	}
	if !requireGit(a, "rewrite-coauthors") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-coauthors")
	if !ok {
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
	if !refreshOriginForRewriteOptions(a, opts.fetch, repos) {
		return 1
	}

	scans := parallelReposProgress(a.ctx, repos, newProgress(a, "Preparing coauthor rewrites", len(repos)), func(r repo) coauthorScan {
		return scanCoauthorMessages(a, r, opts)
	})
	if interrupted(a) {
		return 1
	}

	applies := []coauthorApply{}
	skippedRepos := 0
	skippedGenerated := 0
	failed := 0
	for _, scan := range scans {
		skippedGenerated += scan.skippedGenerated
		if scan.err != nil {
			renderErrorBlock(a, scan.repo.display+": could not prepare coauthor rewrite", scan.err.Error())
			failed++
			continue
		}
		if scan.skipped || len(scan.mappings) == 0 {
			skippedRepos++
			continue
		}
		applies = append(applies, coauthorApply{repo: scan.repo, branches: scan.branches, mappings: scan.mappings})
	}

	if len(applies) == 0 {
		renderSummary(a,
			summaryCount{label: "commit messages rewritten", value: 0, color: a.ui.Green},
			summaryCount{label: "repositories updated", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: skippedRepos, color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		if failed > 0 {
			return 1
		}
		return 0
	}

	affected := 0
	for _, apply := range applies {
		affected += len(apply.mappings)
	}
	renderNotice(a, "Coauthor Rewrite", coauthorPreviewRows(opts, len(applies), affected, skippedGenerated), nil)
	renderWarning(a, fmt.Sprintf("This operation rewrites Git history in %d repositories. A force push will be required to update any remote. Tags may still point at old history, and commit or tag signatures may become invalid.", len(applies)))
	confirmation := confirmOrSkip(a, opts.confirmation.yes, fmt.Sprintf("Rewrite coauthor trailers in %d repositories?", len(applies)))
	if confirmation == confirmationUnavailable || confirmation == confirmationCancelled {
		return 1
	}
	if confirmation == confirmationDeclined {
		renderSummary(a,
			summaryCount{label: "commit messages rewritten", value: 0, color: a.ui.Green},
			summaryCount{label: "repositories updated", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: skippedRepos + len(applies), color: a.ui.Yellow},
			summaryCount{label: "failed", value: failed, color: a.ui.Red},
		)
		if failed > 0 {
			return 1
		}
		return 0
	}

	results := parallelItemsWithWorkersProgress(a.ctx, applies, gitMutationWorkerCount(len(applies)), newProgress(a, "Rewriting coauthors", len(applies)), func(apply coauthorApply) (string, string) {
		return apply.repo.display, apply.repo.display
	}, func(apply coauthorApply) coauthorApplyResult {
		hashes := make([]string, 0, len(apply.mappings))
		for _, mapping := range apply.mappings {
			hashes = append(hashes, mapping.hash)
		}
		if err := captureRewriteBaselineForHashes(a, apply.repo, hashes); err != nil {
			return coauthorApplyResult{apply: apply, err: err}
		}
		callback, err := writeCoauthorMessageCallback(apply.mappings)
		if err != nil {
			return coauthorApplyResult{apply: apply, err: fmt.Errorf("could not create coauthor callback: %w", err)}
		}
		defer os.Remove(callback)
		out, err, restoreErr := runFilterRepoRestoringOrigin(a, apply.repo.dir, apply.repo.gitDir, filterCmd, coauthorFilterArgs(apply.branches, callback), nil)
		if err == nil {
			if updateErr := updateRewriteBaselineFromFilterRepoMap(apply.repo.gitDir); updateErr != nil {
				return coauthorApplyResult{apply: apply, output: out, err: fmt.Errorf("could not update rewrite baseline: %w", updateErr), restoreErr: restoreErr}
			}
		}
		return coauthorApplyResult{apply: apply, output: out, err: err, restoreErr: restoreErr}
	})
	if interrupted(a) {
		return 1
	}

	rewritten := 0
	updated := 0
	for _, result := range results {
		if result.err == nil && result.restoreErr == nil {
			rewritten += len(result.apply.mappings)
			updated++
			continue
		}
		if result.err != nil {
			renderErrorBlock(a, result.apply.repo.display+": could not rewrite coauthor trailers", outputOrError(result.output, result.err))
		} else {
			renderErrorBlock(a, result.apply.repo.display+": coauthor rewrite completed, but origin could not be restored", result.restoreErr.Error())
		}
		if result.err != nil && result.restoreErr != nil {
			renderErrorBlock(a, result.apply.repo.display+": coauthor rewrite failed, and origin could not be restored", result.restoreErr.Error())
		}
		failed++
	}
	renderSummary(a,
		summaryCount{label: "commit messages rewritten", value: rewritten, color: a.ui.Green},
		summaryCount{label: "repositories updated", value: updated, color: a.ui.Green},
		summaryCount{label: "skipped", value: skippedRepos, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	if failed > 0 {
		return 1
	}
	return 0
}

func scanCoauthorMessages(a *app, r repo, opts rewriteCoauthorsOptions) coauthorScan {
	scan := coauthorScan{repo: r}
	if !a.git.HasHead(a.ctx, r.dir) {
		scan.skipped = true
		return scan
	}
	branches, err := localBranchRefs(a, r.dir)
	if err != nil {
		scan.err = fmt.Errorf("could not list local branches: %w", err)
		return scan
	}
	if len(branches) == 0 {
		scan.skipped = true
		return scan
	}
	scan.branches = branches
	commits, err := collectCoauthorCommits(a, r.dir, branchRefNames(branches))
	if err != nil {
		scan.err = fmt.Errorf("could not read commit messages: %w", err)
		return scan
	}
	for _, commit := range commits {
		if (opts.bounds.hasAfter && commit.authorEpoch < opts.bounds.after) || (opts.bounds.hasBefore && commit.authorEpoch >= opts.bounds.before) {
			continue
		}
		if len(commit.parents) > 1 || ai.IsAutoGeneratedMessage(commit.message) {
			scan.skippedGenerated++
			continue
		}
		message, changed := rewriteCoauthorMessage(commit.message, opts)
		if !changed || message == commit.message {
			continue
		}
		scan.mappings = append(scan.mappings, coauthorMessageMapping{hash: commit.hash, message: message})
	}
	if len(scan.mappings) == 0 {
		scan.skipped = true
	}
	return scan
}

func collectCoauthorCommits(a *app, dir string, refs []string) ([]coauthorCommit, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	args := []string{"log", "--topo-order", "--reverse", "--format=%H%x00%P%x00%at%x00%B%x1e"}
	args = append(args, refs...)
	args = append(args, "--")
	out, err := a.git.Stdout(a.ctx, dir, nil, args...)
	if err != nil {
		return nil, err
	}
	commits := []coauthorCommit{}
	seen := map[string]bool{}
	for _, record := range strings.Split(out, "\x1e") {
		record = strings.Trim(record, "\r\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\x00", 4)
		if len(fields) != 4 {
			return nil, fmt.Errorf("malformed commit message record")
		}
		hash := strings.TrimSpace(fields[0])
		if hash == "" || seen[hash] {
			continue
		}
		seen[hash] = true
		authorEpoch, err := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("malformed author timestamp for %s", hash)
		}
		commits = append(commits, coauthorCommit{
			hash:        hash,
			parents:     strings.Fields(fields[1]),
			authorEpoch: authorEpoch,
			message:     strings.TrimSpace(fields[3]),
		})
	}
	return commits, nil
}

func rewriteCoauthorMessage(message string, opts rewriteCoauthorsOptions) (string, bool) {
	switch opts.operation {
	case coauthorAdd:
		return trailers.AddCoauthors(message, opts.coauthors)
	case coauthorReplace:
		return trailers.ReplaceCoauthor(message, opts.oldEmail, opts.coauthors[0])
	case coauthorRemove:
		return trailers.RemoveCoauthors(message, opts.emails, opts.removeAll)
	default:
		return message, false
	}
}

func coauthorPreviewRows(opts rewriteCoauthorsOptions, repositories, affected, skippedGenerated int) []keyValueRow {
	rows := []keyValueRow{
		{key: "Repositories", value: fmt.Sprintf("%d", repositories)},
		{key: "Affected messages", value: fmt.Sprintf("%d", affected)},
		{key: "Current author date filter", value: currentRewriteDateBoundsDescription(opts.bounds)},
		{key: "Operation", value: string(opts.operation)},
	}
	switch opts.operation {
	case coauthorAdd:
		values := make([]string, 0, len(opts.coauthors))
		for _, identity := range opts.coauthors {
			values = append(values, identity.Display)
		}
		rows = append(rows, keyValueRow{key: "Coauthors", value: strings.Join(values, ", ")})
	case coauthorReplace:
		rows = append(rows,
			keyValueRow{key: "Old email", value: opts.oldEmail},
			keyValueRow{key: "Replacement", value: opts.coauthors[0].Display},
		)
	case coauthorRemove:
		selector := "all coauthors"
		if !opts.removeAll {
			selector = strings.Join(opts.emails, ", ")
		}
		rows = append(rows, keyValueRow{key: "Selectors", value: selector})
	}
	rows = append(rows, keyValueRow{key: "Skipped generated commits", value: fmt.Sprintf("%d", skippedGenerated)})
	return rows
}

func coauthorFilterArgs(branches []dateBranchRef, callback string) []string {
	args := []string{"--partial", "--force", "--refs"}
	for _, branch := range branches {
		args = append(args, branch.Name)
	}
	return append(args, "--commit-callback", callback)
}

func writeCoauthorMessageCallback(mappings []coauthorMessageMapping) (string, error) {
	f, err := os.CreateTemp("", "git-wrangler-coauthor-callback-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	fmt.Fprintln(f, "mapping = {}")
	sort.Slice(mappings, func(i, j int) bool { return mappings[i].hash < mappings[j].hash })
	for _, mapping := range mappings {
		fmt.Fprintf(f, "mapping[%s] = %s\n", git.PythonBytesLiteral(mapping.hash), git.PythonBytesLiteral(mapping.message+"\n"))
	}
	fmt.Fprintln(f, "if commit.original_id in mapping:")
	fmt.Fprintln(f, "    commit.message = mapping[commit.original_id]")
	return f.Name(), nil
}
