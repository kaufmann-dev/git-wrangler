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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
)

var (
	ErrCancelled   = errors.New("cancelled")
	conventionalRe = regexp.MustCompile(`^(feat|fix|docs|style|refactor|test|chore|perf|ci|build|revert)(\([^)]+\))?!?: .+`)
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(['"]?)(sk-[a-zA-Z0-9_-]{20,}|sk_[a-zA-Z0-9_-]{20,})(['"]?)`),
		regexp.MustCompile(`(['"]?)(gh[pousr]_[a-zA-Z0-9_]{20,})(['"]?)`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`(?i)(password|passwd|pwd|secret|api_key|apikey|auth_token|access_token|private_key)\s*[:=]\s*['"][^'"]{8,}['"]`),
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
	BatchSize         int
	MaxCharsPerCommit int
	Timeout           time.Duration
	SkipConventional  bool
	WorkDir           string
	Git               git.Client
}

type Plan struct {
	Repos          []RepoPlan
	Summary        string
	GeneratedCount int
}

type RepoPlan struct {
	Dir          string
	Name         string
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

type item struct {
	ID         string
	RepoIndex  int
	RepoDir    string
	RepoName   string
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
	Message string `json:"message"`
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
	items, stats := collectItems(ctx, repos, cfg.Git, cfg.MaxCharsPerCommit, cfg.SkipConventional)
	if len(items) == 0 {
		if cfg.SkipConventional {
			return &Plan{Summary: "No commits require AI rewriting. Existing Conventional Commit messages were skipped.\n"}, nil
		}
		return &Plan{Summary: "No commits with usable file context were found for AI rewriting.\n"}, nil
	}

	batches := (len(items) + cfg.BatchSize - 1) / cfg.BatchSize
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Data send notice")
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
	fmt.Fprintln(out, "The command will send file paths, stats, and redacted diff snippets.")
	fmt.Fprintln(out, "Old commit messages and API keys are not sent in commit context.")
	if confirm == nil || !confirm("Send this data to the configured API endpoint?") {
		return nil, ErrCancelled
	}

	results, failures := processItems(ctx, items, cfg, out)
	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintf(out, "Failed %s %s: %s\n", f.Item.RepoName, f.Item.Hash[:8], f.Reason)
		}
		return nil, fmt.Errorf("AI generation is incomplete; no history was changed")
	}
	return buildPlan(items, results, stats, cfg.WorkDir)
}

func collectItems(ctx context.Context, repositories []Repository, gitClient git.Client, charBudget int, skipConventional bool) ([]item, Stats) {
	var items []item
	stats := Stats{}
	for repoIndex, repo := range repositories {
		stats.RepoCount++
		if _, err := gitClient.Capture(ctx, repo.Dir, nil, "rev-parse", "HEAD"); err != nil {
			stats.SkippedUnborn++
			continue
		}
		commitsOut, _ := gitClient.Stdout(ctx, repo.Dir, nil, "rev-list", "--reverse", "--all")
		commits := splitLines(commitsOut)
		stats.TotalCommits += len(commits)
		for _, commitHash := range commits {
			oldMessage, _ := gitClient.Stdout(ctx, repo.Dir, nil, "log", "-1", "--format=%B", commitHash)
			if skipConventional && IsConventional(oldMessage) {
				stats.SkippedFormatted++
				continue
			}
			contextText := buildContext(ctx, gitClient, repo.Dir, repo.Name, commitHash, charBudget)
			if strings.TrimSpace(contextText) == "" {
				stats.SkippedEmpty++
				continue
			}
			items = append(items, item{
				ID:         fmt.Sprintf("c%06d", len(items)+1),
				RepoIndex:  repoIndex,
				RepoDir:    repo.Dir,
				RepoName:   repo.Name,
				Hash:       commitHash,
				OldMessage: strings.TrimSpace(oldMessage),
				Context:    contextText,
			})
		}
	}
	return items, stats
}

func buildContext(ctx context.Context, gitClient git.Client, repoDir, repoName, commitHash string, charBudget int) string {
	nameStatus, _ := gitClient.Stdout(ctx, repoDir, nil, "diff-tree", "--root", "--no-commit-id", "--name-status", "-r", commitHash)
	numstat, _ := gitClient.Stdout(ctx, repoDir, nil, "diff-tree", "--root", "--no-commit-id", "--numstat", "-r", commitHash)
	if strings.TrimSpace(nameStatus) == "" && strings.TrimSpace(numstat) == "" {
		return ""
	}
	diffText, _ := gitClient.Stdout(ctx, repoDir, nil, "show", "--format=", "--no-color", "--no-ext-diff", "--find-renames", "--find-copies", "--unified=3", commitHash)
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
		RedactDiff(diffText),
	}, "\n")
	return truncateText(contextText, charBudget)
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

func RedactDiff(diffText string) string {
	var lines []string
	hiding := false
	for _, line := range strings.Split(diffText, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			path, ok := diffPath(line)
			hiding = !ok || IsSensitivePath(path)
			lines = append(lines, RedactLine(line))
			if hiding {
				lines = append(lines, "[SENSITIVE FILE CONTENT HIDDEN]")
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
	redacted := line
	for _, pattern := range secretPatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED]")
	}
	return redacted
}

func processItems(ctx context.Context, items []item, cfg Config, out io.Writer) (map[string]string, []failure) {
	results := map[string]string{}
	var failures []failure
	totalBatches := (len(items) + cfg.BatchSize - 1) / cfg.BatchSize
	for batchIndex, start := 0, 0; start < len(items); batchIndex, start = batchIndex+1, start+cfg.BatchSize {
		end := start + cfg.BatchSize
		if end > len(items) {
			end = len(items)
		}
		batch := items[start:end]
		fmt.Fprintf(out, "Generating batch %d/%d (%d commit(s))...\n", batchIndex+1, totalBatches, len(batch))
		accepted, batchFailures := processBatch(ctx, batch, cfg, out)
		for id, message := range accepted {
			results[id] = message
		}
		failures = append(failures, batchFailures...)
	}
	return results, failures
}

func processBatch(ctx context.Context, batch []item, cfg Config, out io.Writer) (map[string]string, []failure) {
	accepted := map[string]string{}
	pending := append([]item(nil), batch...)
	errorsByID := map[string]string{}
	for attempt := 1; attempt <= 3 && len(pending) > 0; attempt++ {
		returned, err := requestBatch(ctx, pending, cfg)
		nextPending := []item{}
		if err != nil {
			for _, item := range pending {
				errorsByID[item.ID] = err.Error()
				nextPending = append(nextPending, item)
			}
		} else {
			for _, item := range pending {
				message := returned[item.ID]
				if ValidateMessage(message) {
					accepted[item.ID] = message
				} else {
					errorsByID[item.ID] = "missing or invalid message"
					nextPending = append(nextPending, item)
				}
			}
		}
		pending = nextPending
		if len(pending) > 0 && attempt < 3 {
			fmt.Fprintf(out, "Retrying %d commit(s) after failed batch attempt %d.\n", len(pending), attempt)
			time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
		}
	}
	failures := make([]failure, 0, len(pending))
	for _, item := range pending {
		failures = append(failures, failure{Item: item, Reason: errorsByID[item.ID]})
	}
	return accepted, failures
}

func requestBatch(ctx context.Context, batch []item, cfg Config) (map[string]string, error) {
	commits := make([]map[string]string, 0, len(batch))
	for _, item := range batch {
		commits = append(commits, map[string]string{
			"id":         item.ID,
			"repository": item.RepoName,
			"commit":     shortHash(item.Hash, 12),
			"context":    item.Context,
		})
	}
	userContent := "Generate one Conventional Commit message for each commit below.\n" +
		"Return exactly this JSON shape: {\"messages\":[{\"id\":\"c000001\",\"message\":\"feat(scope): add thing\"}]}\n" +
		"Preserve every input id exactly once. Use lowercase messages, present tense, no trailing period.\n\n" +
		"Commits:\n" + mustJSON(commits)
	payload := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "You generate concise Conventional Commit messages. Return valid JSON only. Do not include Markdown or explanations."},
			{"role": "user", "content": userContent},
		},
		"temperature": 0.2,
		"max_tokens":  min(max(400, len(batch)*160), 4000),
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ChatEndpoint(cfg.BaseURL), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
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
	var envelope struct {
		Choices []struct {
			Message struct {
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
	return ExtractMessages(envelope.Choices[0].Message.Content)
}

func ChatEndpoint(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

func ExtractMessages(content string) (map[string]string, error) {
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
			return nil, err
		}
		if retryErr := json.Unmarshal([]byte(content[start:end+1]), &parsed); retryErr != nil {
			return nil, retryErr
		}
	}
	result := map[string]string{}
	for _, row := range parsed.Messages {
		id := strings.TrimSpace(row.ID)
		message := normalizeMessage(row.Message)
		if id != "" {
			result[id] = message
		}
	}
	return result, nil
}

func ValidateMessage(message string) bool {
	return message != "" && len(message) <= 120 && conventionalRe.MatchString(message)
}

func buildPlan(items []item, results map[string]string, stats Stats, workDir string) (*Plan, error) {
	type key struct {
		index int
		dir   string
		name  string
	}
	byRepo := map[key][]mapping{}
	var samples []string
	unchanged := 0
	for _, item := range items {
		message := results[item.ID]
		if message == "" {
			continue
		}
		if message == strings.TrimSpace(item.OldMessage) {
			unchanged++
			continue
		}
		k := key{index: item.RepoIndex, dir: item.RepoDir, name: item.RepoName}
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
		plan.Repos = append(plan.Repos, RepoPlan{Dir: k.dir, Name: k.name, CallbackFile: callbackFile, ChangedCount: len(byRepo[k])})
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

func diffPath(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, "diff --git ")
	if !ok {
		return "", false
	}
	if strings.HasPrefix(rest, `"`) {
		first, remaining, ok := nextDiffToken(rest)
		if !ok {
			return "", false
		}
		second, remaining, ok := nextDiffToken(strings.TrimSpace(remaining))
		if !ok || strings.TrimSpace(remaining) != "" {
			return "", false
		}
		_ = first
		return trimDiffPrefix(second), trimDiffPrefix(second) != ""
	}
	if strings.HasPrefix(rest, "a/") {
		idx := strings.LastIndex(rest, " b/")
		if idx < 0 {
			return "", false
		}
		return rest[idx+3:], rest[idx+3:] != ""
	}
	fields := strings.Fields(rest)
	if len(fields) != 2 {
		return "", false
	}
	return trimDiffPrefix(fields[1]), trimDiffPrefix(fields[1]) != ""
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

func normalizeMessage(message string) string {
	message = strings.Trim(strings.TrimSpace(message), `"'`)
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
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
