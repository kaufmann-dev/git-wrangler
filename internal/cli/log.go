package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/conventional"
	"github.com/spf13/cobra"
)

type logOptions struct {
	limit     int
	since     string
	until     string
	sinceUnix int64
	untilUnix int64
	hasSince  bool
	hasUntil  bool
	types     map[string]struct{}
	scopes    map[string]struct{}
	summary   bool
}

type logEntry struct {
	repo       repo
	hash       string
	date       time.Time
	rawSubject string
	parsed     conventional.Commit
}

type logResult struct {
	repo            repo
	entries         []logEntry
	defaultFallback bool
	err             error
}

func runLog(a *app, cmd *cobra.Command, args []string) int {
	opts, ok := logOptionsFromFlags(a, cmd)
	if !ok {
		return 1
	}
	if !requireGit(a, "log") {
		return 1
	}
	repos, err := commandRepositoryTargets(cmd)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}

	results := parallelReposProgress(a.ctx, repos, newProgress(a, "Scanning log", len(repos)), func(r repo) logResult {
		return collectLog(a, r, opts)
	})
	if interrupted(a) {
		return 1
	}

	entries := []logEntry{}
	failed := 0
	for _, result := range results {
		if result.defaultFallback {
			renderWarning(a, fmt.Sprintf("%s: origin/HEAD unavailable; using current HEAD", result.repo.display))
		}
		if result.err != nil {
			renderErrorBlock(a, fmt.Sprintf("%s: could not scan log", result.repo.display), result.err.Error())
			failed++
			continue
		}
		entries = append(entries, result.entries...)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].date.Equal(entries[j].date) {
			return entries[i].date.After(entries[j].date)
		}
		if entries[i].repo.display != entries[j].repo.display {
			return entries[i].repo.display < entries[j].repo.display
		}
		return entries[i].hash < entries[j].hash
	})
	if opts.limit > 0 && len(entries) > opts.limit {
		entries = entries[:opts.limit]
	}

	if opts.summary {
		renderLogSummary(a, entries, len(repos), failed)
	}
	if len(entries) == 0 {
		fmt.Fprintln(a.stdout, "No commits found.")
	} else {
		renderLogTable(a, entries, len(repos) > 1)
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func logOptionsFromFlags(a *app, cmd *cobra.Command) (logOptions, bool) {
	limit, _ := cmd.Flags().GetInt("limit")
	if limit < 0 {
		a.plainErrorf("--limit must be 0 or greater.")
		return logOptions{}, false
	}
	since, _ := cmd.Flags().GetString("since")
	until, _ := cmd.Flags().GetString("until")
	opts := logOptions{limit: limit, since: since, until: until}
	if since != "" {
		if !validDate(since) {
			a.plainErrorf("--since must be in YYYY-MM-DD format.")
			return logOptions{}, false
		}
		opts.hasSince = true
		opts.sinceUnix = parseDateStart(since)
	}
	if until != "" {
		if !validDate(until) {
			a.plainErrorf("--until must be in YYYY-MM-DD format.")
			return logOptions{}, false
		}
		opts.hasUntil = true
		opts.untilUnix = parseDateEnd(until)
	}
	if opts.hasSince && opts.hasUntil && opts.sinceUnix > opts.untilUnix {
		a.plainErrorf("--since must be on or before --until.")
		return logOptions{}, false
	}
	types, _ := cmd.Flags().GetStringArray("type")
	opts.types = map[string]struct{}{}
	for _, typ := range types {
		if typ != "other" && !conventional.IsAllowedType(typ) {
			a.plainErrorf("--type must be a standard Conventional Commit type or other.")
			return logOptions{}, false
		}
		opts.types[typ] = struct{}{}
	}
	scopes, _ := cmd.Flags().GetStringArray("scope")
	opts.scopes = map[string]struct{}{}
	for _, scope := range scopes {
		opts.scopes[scope] = struct{}{}
	}
	opts.summary, _ = cmd.Flags().GetBool("summary")
	return opts, true
}

func collectLog(a *app, r repo, opts logOptions) logResult {
	result := logResult{repo: r}
	ref, fallback := logDefaultRef(a, r)
	result.defaultFallback = fallback
	logArgs := []string{"log", "-z", "--format=%H%x00%aI%x00%s", ref}
	result.err = a.git.StreamStdout(a.ctx, r.dir, nil, func(output io.Reader) error {
		entries, err := parseLogOutput(output, r, opts)
		result.entries = entries
		return err
	}, logArgs...)
	if result.err != nil {
		result.entries = nil
	}
	return result
}

func logDefaultRef(a *app, r repo) (string, bool) {
	originHead, err := a.git.Stdout(a.ctx, r.dir, nil, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil || !strings.HasPrefix(strings.TrimSpace(originHead), "origin/") {
		return "HEAD", true
	}
	branch := strings.TrimPrefix(strings.TrimSpace(originHead), "origin/")
	local := "refs/heads/" + branch
	if a.git.VerifyRef(a.ctx, r.dir, local) {
		return local, false
	}
	return "refs/remotes/origin/" + branch, false
}

func parseLogOutput(output io.Reader, r repo, opts logOptions) ([]logEntry, error) {
	reader := bufio.NewReader(output)
	entries := []logEntry{}
	for {
		hash, err := readNULField(reader)
		if errors.Is(err, io.EOF) && hash == "" {
			return entries, nil
		}
		if err != nil {
			return nil, err
		}
		dateText, err := readNULField(reader)
		if err != nil {
			return nil, err
		}
		subject, err := readNULField(reader)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		date, parseErr := time.Parse(time.RFC3339, dateText)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid author date for %s: %w", hash, parseErr)
		}
		parsed := conventional.Parse(subject)
		entry := logEntry{repo: r, hash: hash, date: date, rawSubject: subject, parsed: parsed}
		if logEntryMatches(entry, opts) {
			entries = append(entries, entry)
		}
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
	}
}

func logEntryMatches(entry logEntry, opts logOptions) bool {
	epoch := entry.date.Unix()
	if opts.hasSince && epoch < opts.sinceUnix {
		return false
	}
	if opts.hasUntil && epoch > opts.untilUnix {
		return false
	}
	if len(opts.types) > 0 {
		if _, ok := opts.types[entry.parsed.Type]; !ok {
			return false
		}
	}
	if len(opts.scopes) > 0 {
		if !entry.parsed.Conventional {
			return false
		}
		if _, ok := opts.scopes[entry.parsed.Scope]; !ok {
			return false
		}
	}
	return true
}

func renderLogSummary(a *app, entries []logEntry, repos, failed int) {
	typeCounts := map[string]int{}
	scopeCounts := map[string]int{}
	breaking := 0
	for _, entry := range entries {
		typeCounts[entry.parsed.Type]++
		if entry.parsed.Scope != "" {
			scopeCounts[entry.parsed.Scope]++
		}
		if entry.parsed.Breaking {
			breaking++
		}
	}
	fmt.Fprintf(a.stdout, "Summary: %d commits, %d repositories, %d failed\n", len(entries), repos, failed)
	fmt.Fprintf(a.stdout, "Types: %s\n", formatNamedCounts(typeCounts, conventional.AllowedTypes(), []string{"other"}, 0))
	fmt.Fprintf(a.stdout, "Scopes: %s\n", formatNamedCounts(scopeCounts, nil, nil, 5))
	fmt.Fprintf(a.stdout, "Breaking: %d\n", breaking)
	fmt.Fprintln(a.stdout)
}

func formatNamedCounts(counts map[string]int, preferred []string, suffix []string, limit int) string {
	parts := []string{}
	seen := map[string]struct{}{}
	appendPart := func(name string) {
		if counts[name] == 0 {
			return
		}
		seen[name] = struct{}{}
		parts = append(parts, fmt.Sprintf("%s %d", name, counts[name]))
	}
	for _, name := range preferred {
		appendPart(name)
	}
	names := []string{}
	for name, count := range counts {
		if count == 0 {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		if containsString(suffix, name) {
			continue
		}
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	if limit > 0 && len(names) > limit {
		names = names[:limit]
	}
	for _, name := range names {
		appendPart(name)
	}
	for _, name := range suffix {
		appendPart(name)
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func renderLogTable(a *app, entries []logEntry, multiRepo bool) {
	columns := []tableColumn{
		{header: "Date"},
		{header: "Hash"},
		{header: "Type"},
		{header: "Scope"},
		{header: "Subject", max: 80},
	}
	if multiRepo {
		columns = []tableColumn{
			{header: "Date"},
			{header: "Repository"},
			{header: "Hash"},
			{header: "Type"},
			{header: "Scope"},
			{header: "Subject", max: 80},
		}
	}
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		row := []string{
			entry.date.In(time.Local).Format("2006-01-02"),
			prefix(entry.hash, 8),
			logTypeCell(a, entry.parsed),
			logScopeCell(entry.parsed),
			logSubjectCell(entry),
		}
		if multiRepo {
			row = []string{
				entry.date.In(time.Local).Format("2006-01-02"),
				entry.repo.display,
				prefix(entry.hash, 8),
				logTypeCell(a, entry.parsed),
				logScopeCell(entry.parsed),
				logSubjectCell(entry),
			}
		}
		rows = append(rows, row)
	}
	renderTable(a, columns, rows)
}

func logTypeCell(a *app, commit conventional.Commit) string {
	label := commit.Type
	if commit.Breaking {
		label += "!"
	}
	return logTypeColor(a, commit.Type) + label + a.ui.Reset
}

func logTypeColor(a *app, typ string) string {
	switch typ {
	case "feat", "perf":
		return a.ui.Green
	case "fix", "revert":
		return a.ui.Red
	case "docs", "test":
		return a.ui.Cyan
	case "style", "ci", "build":
		return a.ui.Blue
	case "refactor":
		return a.ui.Yellow
	default:
		return a.ui.Muted
	}
}

func logScopeCell(commit conventional.Commit) string {
	if commit.Scope == "" {
		return "-"
	}
	return commit.Scope
}

func logSubjectCell(entry logEntry) string {
	if entry.parsed.Conventional {
		return entry.parsed.Subject
	}
	return firstLine(entry.rawSubject)
}
