package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
)

var (
	ErrCancelled    = errors.New("cancelled")
	ErrAPICancelled = errors.New("api generation cancelled")
	conventionalRe  = regexp.MustCompile(`^(feat|fix|docs|style|refactor|test|chore|perf|ci|build|revert)(\([^)]+\))?!?: .+$`)
	secretAssignRe  = regexp.MustCompile("(?i)\\b(password|passwd|pwd|secret|api_key|apikey|auth_token|access_token|private_key)\\b(\\s*(?::=|:|==|=)\\s*)('[^']{8,}'|\\\"[^\\\"]{8,}\\\"|\\x60[^\\x60]{8,}\\x60|[^'\\\"\\s]{8,})")
	secretPatterns  = []*regexp.Regexp{
		regexp.MustCompile(`(['"]?)(sk-[a-zA-Z0-9_-]{20,}|sk_[a-zA-Z0-9_-]{20,})(['"]?)`),
		regexp.MustCompile(`(['"]?)(gh[pousr]_[a-zA-Z0-9_]{20,})(['"]?)`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`(?i)(mongodb(\+srv)?|postgres(ql)?|mysql|redis)://[^@\s]+@[^\s]+`),
		regexp.MustCompile(`(?i)Bearer\s+[a-zA-Z0-9_.-]+`),
	}
)

type Repository struct {
	Dir     string
	Name    string
	GitDir  string
	Ordinal int
}

type Config struct {
	BaseURL           string
	Model             string
	APIKey            string
	Headers           map[string]string
	BatchSize         int
	MaxCharsPerCommit int
	RPM               int
	Timeout           time.Duration
	SkipConventional  bool
	Body              bool
	WorkDir           string
	Git               git.Client
	Progress          func(ProgressEvent)
}

type ProgressEvent struct {
	Phase    string
	Key      string
	RepoName string
	Current  int
	Total    int
	Detail   string
	Error    bool
}

type Plan struct {
	Repos          []RepoPlan
	Summary        string
	GeneratedCount int
}

type RepoPlan struct {
	Dir          string
	Name         string
	GitDir       string
	CallbackFile string
	ChangedCount int
}

type Stats struct {
	RepoCount        int
	TotalCommits     int
	SkippedFormatted int
	SkippedEmpty     int
	SkippedUnborn    int
}

type Message struct {
	Subject string
	Body    string
}

type GenerationInput struct {
	ID       string
	RepoDir  string
	RepoName string
	Ref      string
	Context  string
}

type GenerationFailure struct {
	ID       string
	RepoName string
	Ref      string
	Reason   string
}

type item struct {
	ID         string
	RepoIndex  int
	RepoDir    string
	RepoName   string
	RepoGitDir string
	Hash       string
	OldMessage string
	Context    string
}

type failure struct {
	Item   item
	Reason string
}

type mapping struct {
	hash    string
	message string
}

type generatedMessage struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type generatedResponse struct {
	Messages []generatedMessage `json:"messages"`
}

func Generate(ctx context.Context, repos []Repository, cfg Config, out io.Writer, confirm func(string) bool) (*Plan, error) {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.MaxCharsPerCommit <= 0 {
		cfg.MaxCharsPerCommit = 3000
	}
	if cfg.RPM <= 0 {
		cfg.RPM = 300
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	if cfg.WorkDir == "" {
		return nil, errors.New("missing work directory")
	}
	if cfg.Git.IsZero() {
		cfg.Git = git.New(nil)
	}

	fmt.Fprintln(out, "Scanning repositories and preparing redacted commit context...")
	items, stats, err := collectItems(ctx, repos, cfg.Git, cfg.MaxCharsPerCommit, cfg.SkipConventional, progressFunc(cfg, out))
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		if cfg.SkipConventional {
			return &Plan{Summary: "No commits require AI rewriting. Existing Conventional Commit messages were skipped.\n"}, nil
		}
		return &Plan{Summary: "No commits with usable file context were found for AI rewriting.\n"}, nil
	}

	batches := (len(items) + cfg.BatchSize - 1) / cfg.BatchSize
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Data send notice")
	fmt.Fprintln(out, "----------------")
	fmt.Fprintf(out, "Endpoint: %s\n", cfg.BaseURL)
	fmt.Fprintf(out, "Model: %s\n", cfg.Model)
	fmt.Fprintf(out, "Repositories scanned: %d\n", stats.RepoCount)
	fmt.Fprintf(out, "Total commits found: %d\n", stats.TotalCommits)
	fmt.Fprintf(out, "Commits to process: %d\n", len(items))
	if cfg.SkipConventional {
		fmt.Fprintf(out, "Skipped already Conventional Commits: %d\n", stats.SkippedFormatted)
	}
	fmt.Fprintf(out, "API batches: %d\n", batches)
	fmt.Fprintf(out, "Per-commit context budget: %d characters\n", cfg.MaxCharsPerCommit)
	if cfg.Body {
		fmt.Fprintln(out, "Generated messages will include a subject and body.")
	}
	fmt.Fprintln(out, "The command will send file paths, stats, and redacted diff snippets.")
	fmt.Fprintln(out, "Old commit messages and API keys are not sent in commit context.")
	fmt.Fprintln(out)
	if confirm == nil || !confirm("Send this data to the configured API endpoint?") {
		return nil, ErrCancelled
	}

	results, failures, err := processItems(ctx, items, cfg, out)
	if err != nil {
		return nil, err
	}
	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintf(out, "Failed %s %s: %s\n", f.Item.RepoName, f.Item.Hash[:8], f.Reason)
		}
		return nil, fmt.Errorf("AI generation is incomplete; no history was changed")
	}
	return buildPlan(items, results, stats, cfg.WorkDir)
}

func GenerateMessages(ctx context.Context, changes []GenerationInput, cfg Config, out io.Writer) (map[string]Message, []GenerationFailure) {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 4
	}
	if cfg.MaxCharsPerCommit <= 0 {
		cfg.MaxCharsPerCommit = 3000
	}
	if cfg.RPM <= 0 {
		cfg.RPM = 300
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	items := make([]item, 0, len(changes))
	for _, change := range changes {
		items = append(items, item{
			ID:       change.ID,
			RepoDir:  change.RepoDir,
			RepoName: change.RepoName,
			Hash:     change.Ref,
			Context:  change.Context,
		})
	}
	results, failures, err := processItems(ctx, items, cfg, out)
	publicFailures := make([]GenerationFailure, 0, len(failures))
	if err != nil {
		for _, change := range changes {
			publicFailures = append(publicFailures, GenerationFailure{
				ID:       change.ID,
				RepoName: change.RepoName,
				Ref:      change.Ref,
				Reason:   err.Error(),
			})
		}
		return results, publicFailures
	}
	for _, failure := range failures {
		publicFailures = append(publicFailures, GenerationFailure{
			ID:       failure.Item.ID,
			RepoName: failure.Item.RepoName,
			Ref:      failure.Item.Hash,
			Reason:   failure.Reason,
		})
	}
	return results, publicFailures
}

func progressFunc(cfg Config, out io.Writer) func(ProgressEvent) {
	if cfg.Progress != nil {
		return cfg.Progress
	}
	return func(event ProgressEvent) {
		if event.Total <= 1 || event.Current == 0 {
			return
		}
		if event.Detail != "" {
			fmt.Fprintf(out, "%s: %d/%d %s\n", event.Phase, event.Current, event.Total, event.Detail)
		} else if event.RepoName != "" {
			fmt.Fprintf(out, "%s: %d/%d %s\n", event.Phase, event.Current, event.Total, event.RepoName)
		} else {
			fmt.Fprintf(out, "%s: %d/%d\n", event.Phase, event.Current, event.Total)
		}
	}
}

func collectItems(ctx context.Context, repositories []Repository, gitClient git.Client, charBudget int, skipConventional bool, progress func(ProgressEvent)) ([]item, Stats, error) {
	type repoResult struct {
		items []item
		stats Stats
		err   error
	}
	results := make([]repoResult, len(repositories))
	jobs := make(chan int)
	var wg sync.WaitGroup
	completedRepos := 0
	var completedMu sync.Mutex
	for i := 0; i < aiScanWorkerCount(len(repositories)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repoIndex := range jobs {
				repo := repositories[repoIndex]
				progress(ProgressEvent{Phase: "Scanning repositories", Key: repo.Name, RepoName: repo.Name, Current: 0, Total: len(repositories), Detail: repo.Name})
				items, stats, err := collectRepoItems(ctx, repoIndex, repo, gitClient, charBudget, skipConventional, progress)
				results[repoIndex] = repoResult{items: items, stats: stats, err: err}
				completedMu.Lock()
				completedRepos++
				current := completedRepos
				completedMu.Unlock()
				progress(ProgressEvent{Phase: "Scanning repositories", Key: repo.Name, RepoName: repo.Name, Current: current, Total: len(repositories), Detail: repo.Name})
			}
		}()
	}
	for repoIndex := range repositories {
		jobs <- repoIndex
	}
	close(jobs)
	wg.Wait()

	var items []item
	stats := Stats{}
	for _, result := range results {
		stats.RepoCount += result.stats.RepoCount
		stats.TotalCommits += result.stats.TotalCommits
		stats.SkippedFormatted += result.stats.SkippedFormatted
		stats.SkippedEmpty += result.stats.SkippedEmpty
		stats.SkippedUnborn += result.stats.SkippedUnborn
		if result.err != nil {
			return nil, stats, result.err
		}
		items = append(items, result.items...)
	}
	for i := range items {
		items[i].ID = fmt.Sprintf("c%06d", i+1)
	}
	return items, stats, nil
}

func collectRepoItems(ctx context.Context, repoIndex int, repo Repository, gitClient git.Client, charBudget int, skipConventional bool, progress func(ProgressEvent)) ([]item, Stats, error) {
	stats := Stats{RepoCount: 1}
	if _, err := gitClient.Capture(ctx, repo.Dir, nil, "rev-parse", "HEAD"); err != nil {
		stats.SkippedUnborn++
		return nil, stats, nil
	}
	commits, err := readCommitMessages(ctx, gitClient, repo.Dir)
	if err != nil {
		return nil, stats, fmt.Errorf("%s: list commits: %w", repo.Name, err)
	}
	stats.TotalCommits += len(commits)
	items := []item{}
	for commitIndex, commit := range commits {
		if shouldReportAIProgress(commitIndex+1, len(commits)) {
			progress(ProgressEvent{
				Phase:    "Scanning commits",
				Key:      repo.Name,
				RepoName: repo.Name,
				Current:  commitIndex + 1,
				Total:    len(commits),
				Detail:   fmt.Sprintf("%s %d/%d commits", repo.Name, commitIndex+1, len(commits)),
			})
		}
		if skipConventional && IsConventional(commit.message) {
			stats.SkippedFormatted++
			continue
		}
		contextText, err := buildContext(ctx, gitClient, repo.Dir, repo.Name, commit.hash, charBudget)
		if err != nil {
			return nil, stats, fmt.Errorf("%s %s: build commit context: %w", repo.Name, shortHash(commit.hash, 8), err)
		}
		if strings.TrimSpace(contextText) == "" {
			stats.SkippedEmpty++
			continue
		}
		items = append(items, item{
			RepoIndex:  repoIndex,
			RepoDir:    repo.Dir,
			RepoName:   repo.Name,
			RepoGitDir: repo.GitDir,
			Hash:       commit.hash,
			OldMessage: strings.TrimSpace(commit.message),
			Context:    contextText,
		})
	}
	return items, stats, nil
}

func shouldReportAIProgress(current, total int) bool {
	return total >= 100 && current%100 == 0
}

type commitMessage struct {
	hash    string
	message string
}

func readCommitMessages(ctx context.Context, gitClient git.Client, repoDir string) ([]commitMessage, error) {
	out, err := gitClient.Stdout(ctx, repoDir, nil, "log", "--reverse", "--all", "--format=%H%x1f%B%x1e")
	if err != nil {
		return nil, err
	}
	commits := []commitMessage{}
	for _, record := range strings.Split(out, "\x1e") {
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}
		hash, message, ok := strings.Cut(record, "\x1f")
		if !ok || strings.TrimSpace(hash) == "" {
			return nil, fmt.Errorf("malformed commit log record")
		}
		commits = append(commits, commitMessage{hash: strings.TrimSpace(hash), message: message})
	}
	return commits, nil
}

func aiScanWorkerCount(repoCount int) int {
	workers := runtime.NumCPU()
	if workers > 32 {
		workers = 32
	}
	if workers < 1 {
		workers = 1
	}
	if repoCount > 0 && workers > repoCount {
		workers = repoCount
	}
	return workers
}

func buildContext(ctx context.Context, gitClient git.Client, repoDir, repoName, commitHash string, charBudget int) (string, error) {
	nameStatus, err := gitClient.Stdout(ctx, repoDir, nil, "diff-tree", "--root", "--no-commit-id", "--name-status", "-r", commitHash)
	if err != nil {
		return "", fmt.Errorf("read changed files: %w", err)
	}
	numstat, err := gitClient.Stdout(ctx, repoDir, nil, "diff-tree", "--root", "--no-commit-id", "--numstat", "-r", commitHash)
	if err != nil {
		return "", fmt.Errorf("read change stats: %w", err)
	}
	if strings.TrimSpace(nameStatus) == "" && strings.TrimSpace(numstat) == "" {
		return "", nil
	}
	diffText := hiddenDiffMarker(nameStatus)
	paths := visibleDiffPaths(nameStatus)
	if len(paths) > 0 {
		args := append([]string{"show", "--format=", "--no-color", "--no-ext-diff", "--find-renames", "--find-copies", "--unified=3", commitHash, "--"}, paths...)
		diffText, err = gitClient.Stdout(ctx, repoDir, nil, args...)
		if err != nil {
			return "", fmt.Errorf("read diff: %w", err)
		}
		diffText = RedactDiff(diffText)
	}
	contextText := strings.Join([]string{
		"Repository: " + repoName,
		"Commit: " + shortHash(commitHash, 12),
		"",
		"Files changed:",
		limitedLines(nameStatus, 80),
		"",
		"Stats:",
		limitedLines(numstat, 80),
		"",
		"Redacted diff snippet:",
		diffText,
	}, "\n")
	return truncateText(contextText, charBudget), nil
}

func hiddenDiffMarker(nameStatus string) string {
	for _, paths := range changedPathGroups(nameStatus) {
		sensitive, excluded := pathGroupSafety(paths)
		if !sensitive && excluded {
			return "[EXCLUDED OR SENSITIVE FILE CONTENT HIDDEN]"
		}
	}
	return "[SENSITIVE FILE CONTENT HIDDEN]"
}

func visibleDiffPaths(nameStatus string) []string {
	paths := []string{}
	for _, group := range changedPathGroups(nameStatus) {
		sensitive, excluded := pathGroupSafety(group)
		if !sensitive && !excluded && len(group) > 0 {
			paths = append(paths, group[len(group)-1])
		}
	}
	return paths
}

func changedPaths(nameStatus string) []string {
	paths := []string{}
	for _, group := range changedPathGroups(nameStatus) {
		paths = append(paths, group...)
	}
	return paths
}

func changedPathGroups(nameStatus string) [][]string {
	groups := [][]string{}
	for _, line := range splitLines(nameStatus) {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		groups = append(groups, fields[1:])
	}
	return groups
}

func pathGroupSafety(paths []string) (sensitive bool, excluded bool) {
	for _, path := range paths {
		if IsSensitivePath(path) {
			sensitive = true
		}
		if IsExcludedDiffPath(path) {
			excluded = true
		}
	}
	return sensitive, excluded
}

func BuildStagedContext(ctx context.Context, gitClient git.Client, repoDir, repoName string, charBudget int) (string, error) {
	return BuildStagedContextWithEnv(ctx, gitClient, repoDir, repoName, charBudget, nil)
}

func BuildStagedContextWithEnv(ctx context.Context, gitClient git.Client, repoDir, repoName string, charBudget int, env []string) (string, error) {
	nameStatus, err := gitClient.Stdout(ctx, repoDir, env, "diff", "--cached", "--name-status")
	if err != nil {
		return "", fmt.Errorf("read changed files: %w", err)
	}
	numstat, err := gitClient.Stdout(ctx, repoDir, env, "diff", "--cached", "--numstat")
	if err != nil {
		return "", fmt.Errorf("read change stats: %w", err)
	}
	if strings.TrimSpace(nameStatus) == "" && strings.TrimSpace(numstat) == "" {
		return "", nil
	}
	diffText := hiddenDiffMarker(nameStatus)
	paths := visibleDiffPaths(nameStatus)
	if len(paths) > 0 {
		args := append([]string{"diff", "--cached", "--no-color", "--no-ext-diff", "--find-renames", "--find-copies", "--unified=3", "--"}, paths...)
		diffText, err = gitClient.Stdout(ctx, repoDir, env, args...)
		if err != nil {
			return "", fmt.Errorf("read diff: %w", err)
		}
		diffText = RedactDiff(diffText)
	}
	contextText := strings.Join([]string{
		"Repository: " + repoName,
		"",
		"Files changed:",
		limitedLines(nameStatus, 80),
		"",
		"Stats:",
		limitedLines(numstat, 80),
		"",
		"Redacted staged diff snippet:",
		diffText,
	}, "\n")
	return truncateText(contextText, charBudget), nil
}

func IsConventional(message string) bool {
	first := firstLine(strings.TrimSpace(message))
	return conventionalRe.MatchString(first)
}

func IsSensitivePath(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	base := filepath.Base(normalized)
	if base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".env") {
		return true
	}
	switch base {
	case ".npmrc", ".pypirc", ".netrc", ".git-credentials", "id_rsa", "id_rsa.pub", "id_ed25519", "id_ed25519.pub", "credentials.json", "secrets.json", "kubeconfig":
		return true
	case "application_default_credentials.json", "azureprofile.json", "accesstokens.json":
		return true
	}
	if strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx") || strings.HasSuffix(base, ".asc") || strings.HasSuffix(base, ".gpg") || strings.HasSuffix(base, ".crt") || strings.HasSuffix(base, ".cer") || strings.HasSuffix(base, ".cert") {
		return true
	}
	if strings.Contains(normalized, ".docker/config.json") || strings.Contains(normalized, ".kube/config") {
		return true
	}
	if strings.Contains(normalized, ".aws/") || strings.Contains(normalized, ".config/gcloud/") || strings.Contains(normalized, "cloud/credentials") {
		return true
	}
	if strings.Contains(normalized, "secret") || strings.Contains(normalized, "credential") {
		for _, suffix := range []string{".json", ".yml", ".yaml", ".toml", ".ini", ".env"} {
			if strings.HasSuffix(base, suffix) {
				return true
			}
		}
	}
	return false
}

func IsExcludedDiffPath(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	normalized = strings.Trim(normalized, "/")
	if normalized == "" {
		return false
	}
	ext := filepath.Ext(normalized)
	switch ext {
	case ".avif", ".bmp", ".gif", ".ico", ".jpeg", ".jpg", ".pdf", ".png", ".svg", ".webp",
		".mp3", ".mp4", ".ogg", ".wav", ".webm",
		".eot", ".otf", ".ttf", ".woff", ".woff2",
		".glb", ".gltf":
		return true
	}
	if strings.HasSuffix(normalized, ".min.js") || strings.HasSuffix(normalized, ".min.css") {
		return true
	}
	if strings.Contains("/"+normalized+"/", "/wp-content/uploads/") {
		return true
	}
	for _, segment := range strings.Split(normalized, "/") {
		switch segment {
		case "node_modules", "vendor", "dist", "build", ".next", ".nuxt", ".astro", ".cache", "coverage", "tmp", "temp", "bin", "obj", "target", "wp-admin", "wp-includes", "uploads":
			return true
		}
	}
	return false
}

func RedactDiff(diffText string) string {
	var lines []string
	hiding := false
	for _, line := range strings.Split(diffText, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			paths, ok := diffPaths(line)
			sensitive, excluded := pathGroupSafety(paths)
			hiding = !ok || sensitive || excluded
			lines = append(lines, RedactLine(line))
			if hiding {
				if ok && excluded && !sensitive {
					lines = append(lines, "[EXCLUDED FILE CONTENT HIDDEN]")
				} else {
					lines = append(lines, "[SENSITIVE FILE CONTENT HIDDEN]")
				}
			}
			continue
		}
		if hiding {
			continue
		}
		lines = append(lines, RedactLine(line))
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func RedactLine(line string) string {
	redacted := secretAssignRe.ReplaceAllString(line, "${1}${2}[REDACTED]")
	for _, pattern := range secretPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED]")
	}
	return redacted
}

func processItems(ctx context.Context, items []item, cfg Config, out io.Writer) (map[string]Message, []failure, error) {
	totalBatches := (len(items) + cfg.BatchSize - 1) / cfg.BatchSize
	if totalBatches == 0 {
		return map[string]Message{}, nil, nil
	}
	type batchTask struct {
		index int
		items []item
	}
	type batchResult struct {
		index    int
		results  map[string]Message
		failures []failure
	}
	tasks := make([]batchTask, 0, totalBatches)
	batchResults := make([]batchResult, totalBatches)
	var wg sync.WaitGroup
	completedBatches := 0
	var completedMu sync.Mutex
	output := &lockedWriter{writer: out}
	progress := progressFunc(cfg, output)
	pacer := newRequestPacer(cfg.RPM)
	for batchIndex, start := 0, 0; start < len(items); batchIndex, start = batchIndex+1, start+cfg.BatchSize {
		end := start + cfg.BatchSize
		if end > len(items) {
			end = len(items)
		}
		tasks = append(tasks, batchTask{index: batchIndex, items: items[start:end]})
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(task batchTask) {
			defer wg.Done()
			batchKey := fmt.Sprintf("batch-%d", task.index+1)
			completedMu.Lock()
			progress(ProgressEvent{Phase: "Sending API requests", Key: batchKey, Current: 0, Total: totalBatches, Detail: fmt.Sprintf("batch %d", task.index+1)})
			completedMu.Unlock()
			reportRetry := func(message string) {
				completedMu.Lock()
				progress(ProgressEvent{Phase: "Sending API requests", Key: batchKey, Current: completedBatches, Total: totalBatches, Detail: message, Error: true})
				completedMu.Unlock()
			}
			accepted, batchFailures := processBatchWithProgress(ctx, task.items, cfg, output, reportRetry, pacer)
			batchResults[task.index] = batchResult{index: task.index, results: accepted, failures: batchFailures}
			completedMu.Lock()
			completedBatches++
			progress(ProgressEvent{Phase: "Sending API requests", Key: batchKey, Current: completedBatches, Total: totalBatches, Detail: fmt.Sprintf("batch %d completed", task.index+1)})
			completedMu.Unlock()
		}(task)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil, nil, ErrAPICancelled
	}

	results := map[string]Message{}
	var failures []failure
	for _, batch := range batchResults {
		for id, message := range batch.results {
			results[id] = message
		}
		failures = append(failures, batch.failures...)
	}
	return results, failures, nil
}

func requestInterval(requestsPerMinute int) time.Duration {
	if requestsPerMinute <= 0 {
		requestsPerMinute = 300
	}
	return time.Minute / time.Duration(requestsPerMinute)
}

type requestPacer struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
}

func newRequestPacer(requestsPerMinute int) *requestPacer {
	return &requestPacer{interval: requestInterval(requestsPerMinute)}
}

func (p *requestPacer) wait(ctx context.Context) bool {
	if p == nil {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.last.IsZero() {
		delay := p.last.Add(p.interval).Sub(time.Now())
		if delay > 0 && !sleepContext(ctx, delay) {
			return false
		}
	}
	if ctx.Err() != nil {
		return false
	}
	p.last = time.Now()
	return true
}

type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(p)
}

func processBatch(ctx context.Context, batch []item, cfg Config, out io.Writer) (map[string]Message, []failure) {
	return processBatchWithProgress(ctx, batch, cfg, out, nil, nil)
}

func processBatchWithProgress(ctx context.Context, batch []item, cfg Config, out io.Writer, reportRetry func(string), pacer *requestPacer) (map[string]Message, []failure) {
	accepted := map[string]Message{}
	pending := append([]item(nil), batch...)
	errorsByID := map[string]string{}
	for attempt := 1; attempt <= 3 && len(pending) > 0; attempt++ {
		if !pacer.wait(ctx) {
			break
		}
		returned, err := requestBatch(ctx, pending, cfg, attempt)
		nextPending := []item{}
		if err != nil {
			for _, item := range pending {
				errorsByID[item.ID] = err.Error()
				nextPending = append(nextPending, item)
			}
		} else {
			for _, item := range pending {
				message := returned[item.ID]
				if ValidateGeneratedMessage(message, cfg.Body) {
					accepted[item.ID] = message
				} else {
					errorsByID[item.ID] = "missing or invalid message"
					nextPending = append(nextPending, item)
				}
			}
		}
		pending = nextPending
		if len(pending) > 0 && attempt < 3 {
			if ctx.Err() != nil {
				break
			}
			reportRetryMessage(out, reportRetry, retryBatchMessage(len(pending), attempt, retryReason(pending, errorsByID)))
			if !sleepContext(ctx, time.Duration(attempt)*250*time.Millisecond) {
				break
			}
		}
	}
	if len(pending) > 1 {
		if ctx.Err() != nil {
			return accepted, failuresForPending(pending, errorsByID)
		}
		reportRetryMessage(out, reportRetry, fmt.Sprintf("Retrying %d commit(s) individually after failed batch generation.", len(pending)))
		recovered, individualFailures := processSingleItemRetries(ctx, pending, cfg, out, reportRetry, pacer)
		for id, message := range recovered {
			accepted[id] = message
		}
		return accepted, individualFailures
	}
	failures := make([]failure, 0, len(pending))
	for _, item := range pending {
		failures = append(failures, failure{Item: item, Reason: errorsByID[item.ID]})
	}
	return accepted, failures
}

func reportRetryMessage(out io.Writer, reportRetry func(string), message string) {
	if reportRetry != nil {
		reportRetry(message)
		return
	}
	fmt.Fprintln(out, message)
}

func retryBatchMessage(count, attempt int, reason string) string {
	return fmt.Sprintf("Retrying %d commit(s) after failed batch attempt %d: %s.", count, attempt, reason)
}

func failuresForPending(pending []item, errorsByID map[string]string) []failure {
	failures := make([]failure, 0, len(pending))
	for _, item := range pending {
		reason := errorsByID[item.ID]
		if reason == "" {
			reason = ErrCancelled.Error()
		}
		failures = append(failures, failure{Item: item, Reason: reason})
	}
	return failures
}

func retryReason(pending []item, errorsByID map[string]string) string {
	counts := map[string]int{}
	for _, item := range pending {
		reason := strings.TrimSpace(errorsByID[item.ID])
		if reason == "" {
			reason = "unknown error"
		}
		counts[truncateText(reason, 160)]++
	}
	type row struct {
		reason string
		count  int
	}
	rows := make([]row, 0, len(counts))
	for reason, count := range counts {
		rows = append(rows, row{reason: reason, count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count == rows[j].count {
			return rows[i].reason < rows[j].reason
		}
		return rows[i].count > rows[j].count
	})
	parts := make([]string, 0, min(len(rows), 3))
	for i, row := range rows {
		if i >= 3 {
			break
		}
		if row.count == 1 {
			parts = append(parts, row.reason)
		} else {
			parts = append(parts, fmt.Sprintf("%s (%d commits)", row.reason, row.count))
		}
	}
	if len(rows) > 3 {
		parts = append(parts, fmt.Sprintf("%d more reason(s)", len(rows)-3))
	}
	return strings.Join(parts, "; ")
}

func processSingleItemRetries(ctx context.Context, pending []item, cfg Config, out io.Writer, reportRetry func(string), pacer *requestPacer) (map[string]Message, []failure) {
	accepted := map[string]Message{}
	var failures []failure
	for _, pendingItem := range pending {
		itemAccepted, itemFailures := processBatchWithProgress(ctx, []item{pendingItem}, cfg, out, reportRetry, pacer)
		for id, message := range itemAccepted {
			accepted[id] = message
		}
		failures = append(failures, itemFailures...)
	}
	return accepted, failures
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func requestBatch(ctx context.Context, batch []item, cfg Config, attempt int) (map[string]Message, error) {
	commits := make([]map[string]string, 0, len(batch))
	for _, item := range batch {
		commits = append(commits, map[string]string{
			"id":         item.ID,
			"repository": item.RepoName,
			"commit":     shortHash(item.Hash, 12),
			"context":    item.Context,
		})
	}
	shape := "{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(scope): add thing\"}]}"
	if cfg.Body {
		shape = "{\"messages\":[{\"id\":\"c000001\",\"subject\":\"feat(scope): add thing\",\"body\":\"Explain why this change was made.\"}]}"
	}
	userContent := "Generate one Conventional Commit subject for each commit below.\n" +
		"Return exactly this JSON shape: " + shape + "\n" +
		"Preserve every input id exactly once. Use lowercase subjects, present tense, no trailing period.\n"
	if cfg.Body {
		userContent += "Include a concise non-empty body for every message. Do not repeat the subject in the body.\n"
	}
	userContent += "\nCommits:\n" + mustJSON(commits)
	payload := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "You generate concise Conventional Commit messages. Return valid JSON only. Do not include Markdown or explanations."},
			{"role": "user", "content": userContent},
		},
		"temperature": 0.2,
		"max_tokens":  messageTokenLimit(len(batch), cfg.Body, attempt),
	}
	respBody, err := chatCompletion(ctx, cfg, payload)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Choices) == 0 {
		return nil, errors.New("response has no choices")
	}
	if envelope.Choices[0].FinishReason == "length" {
		return nil, errors.New("AI response was truncated by the output token limit")
	}
	return ExtractMessages(envelope.Choices[0].Message.Content)
}

// Preflight verifies the configured API endpoint, model, and credentials before
// commands perform repository work. It intentionally shares the generation path.
func Preflight(ctx context.Context, cfg Config) error {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	_, err := chatCompletion(ctx, cfg, map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "user", "content": "Respond with OK."},
		},
		"max_tokens": 1,
	})
	return err
}

func chatCompletion(ctx context.Context, cfg Config, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ChatEndpoint(cfg.BaseURL), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range cfg.Headers {
		req.Header.Set(name, value)
	}
	client := &http.Client{Timeout: cfg.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateText(string(respBody), 500))
	}
	return respBody, nil
}

func messageTokenLimit(batchSize int, includeBody bool, attempt int) int {
	perMessage := 500
	if includeBody {
		perMessage = 900
	}
	limit := max(1000, batchSize*perMessage)
	if attempt > 1 {
		limit *= attempt
	}
	cap := 20000
	if includeBody {
		cap = 40000
	}
	return min(limit, cap)
}

func ChatEndpoint(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

func ExtractMessages(content string) (map[string]Message, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = regexp.MustCompile("^```(?:json)?\\s*").ReplaceAllString(content, "")
		content = regexp.MustCompile("\\s*```$").ReplaceAllString(content, "")
	}
	var parsed generatedResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		start := strings.IndexByte(content, '{')
		end := strings.LastIndexByte(content, '}')
		if start < 0 || end <= start {
			return nil, responseJSONError(err)
		}
		if retryErr := json.Unmarshal([]byte(content[start:end+1]), &parsed); retryErr != nil {
			return nil, responseJSONError(retryErr)
		}
	}
	result := map[string]Message{}
	for _, row := range parsed.Messages {
		id := strings.TrimSpace(row.ID)
		if id != "" {
			result[id] = Message{
				Subject: normalizeSubject(row.Subject),
				Body:    normalizeBody(row.Body),
			}
		}
	}
	return result, nil
}

func responseJSONError(err error) error {
	if strings.Contains(err.Error(), "unexpected end of JSON input") {
		return errors.New("AI response was incomplete JSON")
	}
	return fmt.Errorf("AI response was not valid JSON: %w", err)
}

func ValidateMessage(message string) bool {
	return ValidateSubject(message)
}

func ValidateSubject(subject string) bool {
	return subject != "" && len(subject) <= 120 && !strings.ContainsAny(subject, "\r\n") && conventionalRe.MatchString(subject)
}

func ValidateBody(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 1000 {
		return false
	}
	for _, r := range body {
		if r < 0x20 && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}

func ValidateGeneratedMessage(message Message, requireBody bool) bool {
	if !ValidateSubject(message.Subject) {
		return false
	}
	return !requireBody || ValidateBody(message.Body)
}

func FormatMessage(message Message) string {
	subject := strings.TrimSpace(message.Subject)
	body := strings.TrimSpace(message.Body)
	if body == "" {
		return subject
	}
	return subject + "\n\n" + body
}

func buildPlan(items []item, results map[string]Message, stats Stats, workDir string) (*Plan, error) {
	type key struct {
		index  int
		dir    string
		name   string
		gitDir string
	}
	byRepo := map[key][]mapping{}
	var samples []string
	unchanged := 0
	for _, item := range items {
		result := results[item.ID]
		if result.Subject == "" {
			continue
		}
		message := FormatMessage(result)
		if message == strings.TrimSpace(item.OldMessage) {
			unchanged++
			continue
		}
		k := key{index: item.RepoIndex, dir: item.RepoDir, name: item.RepoName, gitDir: item.RepoGitDir}
		byRepo[k] = append(byRepo[k], mapping{hash: item.Hash, message: message})
		if len(samples) < 12 {
			samples = append(samples, fmt.Sprintf("  %s %s: %s", item.RepoName, shortHash(item.Hash, 8), message))
		}
	}
	keys := make([]key, 0, len(byRepo))
	for k := range byRepo {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].index == keys[j].index {
			return keys[i].dir < keys[j].dir
		}
		return keys[i].index < keys[j].index
	})
	plan := &Plan{}
	for _, k := range keys {
		callbackFile := filepath.Join(workDir, fmt.Sprintf("callback-%d.py", k.index))
		if err := writeCommitCallback(callbackFile, byRepo[k]); err != nil {
			return nil, err
		}
		plan.Repos = append(plan.Repos, RepoPlan{Dir: k.dir, Name: k.name, GitDir: k.gitDir, CallbackFile: callbackFile, ChangedCount: len(byRepo[k])})
		plan.GeneratedCount += len(byRepo[k])
	}
	lines := []string{
		"",
		"AI commit rewrite summary",
		"-------------------------",
		fmt.Sprintf("Repositories scanned: %d", stats.RepoCount),
		fmt.Sprintf("Repositories with generated rewrites: %d", len(byRepo)),
		fmt.Sprintf("Total commits found: %d", stats.TotalCommits),
		fmt.Sprintf("Commits selected for processing: %d", len(items)),
		fmt.Sprintf("Commits sent to API: %d", len(items)),
		fmt.Sprintf("Generated rewrites: %d", plan.GeneratedCount),
		fmt.Sprintf("Generated but unchanged: %d", unchanged),
		fmt.Sprintf("Skipped empty/unreadable commits: %d", stats.SkippedEmpty),
	}
	if stats.SkippedFormatted > 0 {
		lines = append(lines, fmt.Sprintf("Skipped already Conventional Commits: %d", stats.SkippedFormatted))
	}
	if stats.SkippedUnborn > 0 {
		lines = append(lines, fmt.Sprintf("Skipped repositories with no commits: %d", stats.SkippedUnborn))
	}
	if len(samples) > 0 {
		lines = append(lines, "", "Sample generated messages:")
		lines = append(lines, samples...)
	}
	if plan.GeneratedCount == 0 {
		lines = append(lines, "", "No generated messages require rewriting.")
	}
	plan.Summary = strings.Join(lines, "\n") + "\n"
	return plan, nil
}

func writeCommitCallback(path string, mappings []mapping) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "mapping = {}")
	sort.Slice(mappings, func(i, j int) bool { return mappings[i].hash < mappings[j].hash })
	for _, row := range mappings {
		fmt.Fprintf(f, "mapping[%s] = %s\n", git.PythonBytesLiteral(row.hash), git.PythonBytesLiteral(row.message+"\n"))
	}
	fmt.Fprintln(f, "if commit.original_id in mapping:")
	fmt.Fprintln(f, "    commit.message = mapping[commit.original_id]")
	return nil
}

func diffPaths(line string) ([]string, bool) {
	rest, ok := strings.CutPrefix(line, "diff --git ")
	if !ok {
		return nil, false
	}
	if strings.HasPrefix(rest, `"`) {
		first, remaining, ok := nextDiffToken(rest)
		if !ok {
			return nil, false
		}
		second, remaining, ok := nextDiffToken(strings.TrimSpace(remaining))
		if !ok || strings.TrimSpace(remaining) != "" {
			return nil, false
		}
		first = trimDiffPrefix(first)
		second = trimDiffPrefix(second)
		return []string{first, second}, first != "" && second != ""
	}
	if strings.HasPrefix(rest, "a/") {
		idx := strings.LastIndex(rest, " b/")
		if idx < 0 {
			return nil, false
		}
		first := trimDiffPrefix(rest[:idx])
		second := trimDiffPrefix(rest[idx+1:])
		return []string{first, second}, first != "" && second != ""
	}
	fields := strings.Fields(rest)
	if len(fields) != 2 {
		return nil, false
	}
	first := trimDiffPrefix(fields[0])
	second := trimDiffPrefix(fields[1])
	return []string{first, second}, first != "" && second != ""
}

func nextDiffToken(s string) (token string, rest string, ok bool) {
	if s == "" {
		return "", "", false
	}
	if s[0] != '"' {
		idx := strings.IndexByte(s, ' ')
		if idx < 0 {
			return s, "", true
		}
		return s[:idx], s[idx+1:], true
	}
	escaped := false
	for i := 1; i < len(s); i++ {
		switch {
		case escaped:
			escaped = false
		case s[i] == '\\':
			escaped = true
		case s[i] == '"':
			unquoted, err := strconv.Unquote(s[:i+1])
			if err != nil {
				return "", "", false
			}
			return unquoted, s[i+1:], true
		}
	}
	return "", "", false
}

func trimDiffPrefix(path string) string {
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return path
}

func limitedLines(text string, maxLines int) string {
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	return strings.Join(append(lines[:maxLines], fmt.Sprintf("[TRUNCATED %d LINES]", len(lines)-maxLines)), "\n")
}

func truncateText(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	suffix := "\n[TRUNCATED TO PER-COMMIT CONTEXT BUDGET]"
	if limit <= len(suffix) {
		return text[:limit]
	}
	return text[:limit-len(suffix)] + suffix
}

func normalizeSubject(subject string) string {
	return strings.Trim(strings.TrimSpace(subject), `"'`)
}

func normalizeBody(body string) string {
	return strings.TrimSpace(body)
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func shortHash(hash string, n int) string {
	if len(hash) <= n {
		return hash
	}
	return hash[:n]
}

func mustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
