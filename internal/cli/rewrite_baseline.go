package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

const (
	rewriteBaselineVersion = 1
	rewriteBaselineRelDir  = "git-wrangler/baseline"
)

var commitIdentityRe = regexp.MustCompile(`^(.*) <([^<>]*)> ([0-9]+ [+-][0-9]{4})$`)

type rewriteBaselineManifest struct {
	Version   int                    `json:"version"`
	Entries   []rewriteBaselineEntry `json:"entries"`
	UpdatedAt string                 `json:"updated_at,omitempty"`
}

type rewriteBaselineEntry struct {
	FirstSHA       string   `json:"first_sha"`
	CurrentSHA     string   `json:"current_sha"`
	KnownSHAs      []string `json:"known_shas,omitempty"`
	TreeSHA        string   `json:"tree_sha"`
	ParentSHAs     []string `json:"parent_shas"`
	AuthorName     string   `json:"author_name"`
	AuthorEmail    string   `json:"author_email"`
	AuthorDate     string   `json:"author_date"`
	AuthorEpoch    int64    `json:"author_epoch"`
	AuthorTZ       string   `json:"author_tz"`
	CommitterName  string   `json:"committer_name"`
	CommitterEmail string   `json:"committer_email"`
	CommitterDate  string   `json:"committer_date"`
	CommitterEpoch int64    `json:"committer_epoch"`
	CommitterTZ    string   `json:"committer_tz"`
	Message        string   `json:"message"`
	CaptureID      string   `json:"capture_id"`
	BundlePath     string   `json:"bundle_path"`
	CreatedAt      string   `json:"created_at"`
}

type rewriteBaselineCommitData struct {
	SHA            string
	Tree           string
	Parents        []string
	AuthorName     string
	AuthorEmail    string
	AuthorDate     string
	AuthorEpoch    int64
	AuthorTZ       string
	CommitterName  string
	CommitterEmail string
	CommitterDate  string
	CommitterEpoch int64
	CommitterTZ    string
	Message        string
}

func captureRewriteBaselineForHashes(a *app, r repo, hashes []string) error {
	hashes = sortedUniqueNonEmpty(hashes)
	if len(hashes) == 0 {
		_, _, err := loadRewriteBaseline(r.gitDir)
		return err
	}
	manifest, found, err := loadRewriteBaseline(r.gitDir)
	if err != nil {
		return err
	}
	if !found {
		manifest = rewriteBaselineManifest{Version: rewriteBaselineVersion}
	}
	represented := rewriteBaselineRepresentedSHAs(manifest)
	newHashes := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		if represented[hash] {
			continue
		}
		newHashes = append(newHashes, hash)
	}
	if len(newHashes) == 0 {
		return nil
	}
	captureID := rewriteBaselineCaptureID()
	bundleRelPath := filepath.ToSlash(filepath.Join("bundles", captureID+".bundle"))
	baselineDir := rewriteBaselineDir(r.gitDir)
	bundlePath := filepath.Join(baselineDir, filepath.FromSlash(bundleRelPath))
	if err := os.MkdirAll(filepath.Dir(bundlePath), 0o755); err != nil {
		return fmt.Errorf("could not create baseline bundle directory: %w", err)
	}
	bundlePathForGit, err := absoluteRewriteBaselinePath(bundlePath)
	if err != nil {
		return fmt.Errorf("could not resolve baseline bundle path: %w", err)
	}
	if err := createRewriteBaselineBundle(a, r.dir, bundlePathForGit, captureID, newHashes); err != nil {
		return fmt.Errorf("could not create baseline bundle: %w", err)
	}
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	for _, hash := range newHashes {
		data, err := readRewriteBaselineCommit(a, r.dir, hash)
		if err != nil {
			return fmt.Errorf("could not read baseline commit %s: %w", prefix(hash), err)
		}
		manifest.Entries = append(manifest.Entries, rewriteBaselineEntry{
			FirstSHA:       data.SHA,
			CurrentSHA:     data.SHA,
			KnownSHAs:      []string{data.SHA},
			TreeSHA:        data.Tree,
			ParentSHAs:     append([]string(nil), data.Parents...),
			AuthorName:     data.AuthorName,
			AuthorEmail:    data.AuthorEmail,
			AuthorDate:     data.AuthorDate,
			AuthorEpoch:    data.AuthorEpoch,
			AuthorTZ:       data.AuthorTZ,
			CommitterName:  data.CommitterName,
			CommitterEmail: data.CommitterEmail,
			CommitterDate:  data.CommitterDate,
			CommitterEpoch: data.CommitterEpoch,
			CommitterTZ:    data.CommitterTZ,
			Message:        data.Message,
			CaptureID:      captureID,
			BundlePath:     bundleRelPath,
			CreatedAt:      createdAt,
		})
	}
	return writeRewriteBaseline(r.gitDir, manifest)
}

func createRewriteBaselineBundle(a *app, dir, bundlePath, captureID string, hashes []string) error {
	tempRefs := make([]string, 0, len(hashes))
	for i, hash := range hashes {
		ref := fmt.Sprintf("refs/git-wrangler/baseline-capture/%s/%06d", captureID, i+1)
		if _, err := a.git.Capture(a.ctx, dir, nil, "update-ref", ref, hash); err != nil {
			return err
		}
		tempRefs = append(tempRefs, ref)
	}
	defer func() {
		for _, ref := range tempRefs {
			_, _ = a.git.Capture(a.ctx, dir, nil, "update-ref", "-d", ref)
		}
	}()
	args := append([]string{"bundle", "create", bundlePath}, tempRefs...)
	_, err := a.git.Capture(a.ctx, dir, nil, args...)
	return err
}

func loadRewriteBaseline(gitDir string) (rewriteBaselineManifest, bool, error) {
	path := rewriteBaselineManifestPath(gitDir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return rewriteBaselineManifest{Version: rewriteBaselineVersion}, false, nil
	}
	if err != nil {
		return rewriteBaselineManifest{}, true, fmt.Errorf("could not read rewrite baseline manifest: %w", err)
	}
	var manifest rewriteBaselineManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return rewriteBaselineManifest{}, true, fmt.Errorf("could not parse rewrite baseline manifest: %w", err)
	}
	if err := validateRewriteBaseline(gitDir, manifest); err != nil {
		return rewriteBaselineManifest{}, true, err
	}
	return normalizeRewriteBaseline(manifest), true, nil
}

func validateRewriteBaseline(gitDir string, manifest rewriteBaselineManifest) error {
	if manifest.Version != rewriteBaselineVersion {
		return fmt.Errorf("unsupported rewrite baseline version %d", manifest.Version)
	}
	byFirst := map[string]bool{}
	byCurrent := map[string]bool{}
	for _, entry := range manifest.Entries {
		if entry.FirstSHA == "" || entry.CurrentSHA == "" || entry.TreeSHA == "" {
			return fmt.Errorf("rewrite baseline contains an incomplete commit entry")
		}
		if byFirst[entry.FirstSHA] {
			return fmt.Errorf("rewrite baseline contains duplicate first SHA %s", prefix(entry.FirstSHA))
		}
		if byCurrent[entry.CurrentSHA] {
			return fmt.Errorf("rewrite baseline contains duplicate current SHA %s", prefix(entry.CurrentSHA))
		}
		byFirst[entry.FirstSHA] = true
		byCurrent[entry.CurrentSHA] = true
		if entry.AuthorDate == "" || entry.CommitterDate == "" || entry.BundlePath == "" || entry.CaptureID == "" {
			return fmt.Errorf("rewrite baseline entry %s is missing required metadata", prefix(entry.FirstSHA))
		}
		bundlePath := rewriteBaselineEntryBundlePath(gitDir, entry)
		if _, err := os.Stat(bundlePath); err != nil {
			return fmt.Errorf("rewrite baseline bundle for %s is unavailable: %w", prefix(entry.FirstSHA), err)
		}
	}
	return nil
}

func writeRewriteBaseline(gitDir string, manifest rewriteBaselineManifest) error {
	manifest = normalizeRewriteBaseline(manifest)
	manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	baselineDir := rewriteBaselineDir(gitDir)
	if err := os.MkdirAll(baselineDir, 0o755); err != nil {
		return fmt.Errorf("could not create rewrite baseline directory: %w", err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := rewriteBaselineManifestPath(gitDir)
	temp, err := os.CreateTemp(baselineDir, "manifest-*.json")
	if err != nil {
		return fmt.Errorf("could not create temporary rewrite baseline manifest: %w", err)
	}
	tempName := temp.Name()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("could not save rewrite baseline manifest: %w", err)
	}
	return nil
}

func normalizeRewriteBaseline(manifest rewriteBaselineManifest) rewriteBaselineManifest {
	manifest.Version = rewriteBaselineVersion
	for i := range manifest.Entries {
		manifest.Entries[i].KnownSHAs = sortedUniqueNonEmpty(append(manifest.Entries[i].KnownSHAs, manifest.Entries[i].FirstSHA, manifest.Entries[i].CurrentSHA))
	}
	sort.Slice(manifest.Entries, func(i, j int) bool {
		if manifest.Entries[i].FirstSHA == manifest.Entries[j].FirstSHA {
			return manifest.Entries[i].CurrentSHA < manifest.Entries[j].CurrentSHA
		}
		return manifest.Entries[i].FirstSHA < manifest.Entries[j].FirstSHA
	})
	return manifest
}

func updateRewriteBaselineFromCommitMap(gitDir string, commitMap map[string]string) error {
	if len(commitMap) == 0 {
		return nil
	}
	manifest, found, err := loadRewriteBaseline(gitDir)
	if err != nil || !found {
		return err
	}
	changed := false
	for i := range manifest.Entries {
		if next := commitMap[manifest.Entries[i].CurrentSHA]; next != "" {
			manifest.Entries[i].KnownSHAs = append(manifest.Entries[i].KnownSHAs, manifest.Entries[i].CurrentSHA, next)
			manifest.Entries[i].CurrentSHA = next
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writeRewriteBaseline(gitDir, manifest)
}

func updateRewriteBaselineFromFilterRepoMap(gitDir string) error {
	_, found, err := loadRewriteBaseline(gitDir)
	if err != nil || !found {
		return err
	}
	commitMap, err := readFilterRepoCommitMap(gitDir)
	if err != nil {
		return err
	}
	return updateRewriteBaselineFromCommitMap(gitDir, commitMap)
}

func readFilterRepoCommitMap(gitDir string) (map[string]string, error) {
	path := filepath.Join(rewriteDatesGitMetadataDir(gitDir), "filter-repo", "commit-map")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	mapping := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] == "old" {
			continue
		}
		if isZeroSHA(fields[1]) {
			continue
		}
		mapping[fields[0]] = fields[1]
	}
	return mapping, nil
}

func clearRewriteBaselineStorage(gitDir string) error {
	err := os.RemoveAll(rewriteBaselineDir(gitDir))
	if err != nil {
		return fmt.Errorf("could not clear rewrite baseline: %w", err)
	}
	return nil
}

func clearManagedRewriteMetadata(a *app, dir, gitDir string) error {
	if err := clearRewriteBaselineStorage(gitDir); err != nil {
		return err
	}
	_, _ = a.git.Capture(a.ctx, dir, nil, "update-ref", "-d", rewriteDatesStateRef)
	out, err := a.git.Stdout(a.ctx, dir, nil, "for-each-ref", "--format=%(refname)", rewriteDatesBackupPrefix)
	if err != nil {
		return nil
	}
	for _, ref := range splitLines(out) {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		if _, err := a.git.Capture(a.ctx, dir, nil, "update-ref", "-d", strings.TrimSpace(ref)); err != nil {
			return err
		}
	}
	return nil
}

func readRewriteBaselineCommit(a *app, dir, sha string) (rewriteBaselineCommitData, error) {
	data, err := a.git.Stdout(a.ctx, dir, nil, "cat-file", "-p", sha)
	if err != nil {
		return rewriteBaselineCommitData{}, err
	}
	headers, message, ok := strings.Cut(data, "\n\n")
	if !ok {
		headers = strings.TrimRight(data, "\n")
		message = ""
	}
	commit := rewriteBaselineCommitData{SHA: sha, Message: message}
	for _, line := range strings.Split(headers, "\n") {
		if strings.HasPrefix(line, "tree ") {
			commit.Tree = strings.TrimSpace(strings.TrimPrefix(line, "tree "))
			continue
		}
		if strings.HasPrefix(line, "parent ") {
			commit.Parents = append(commit.Parents, strings.TrimSpace(strings.TrimPrefix(line, "parent ")))
			continue
		}
		if strings.HasPrefix(line, "author ") {
			name, email, date, epoch, tz, err := parseCommitIdentity(strings.TrimPrefix(line, "author "))
			if err != nil {
				return rewriteBaselineCommitData{}, err
			}
			commit.AuthorName = name
			commit.AuthorEmail = email
			commit.AuthorDate = date
			commit.AuthorEpoch = epoch
			commit.AuthorTZ = tz
			continue
		}
		if strings.HasPrefix(line, "committer ") {
			name, email, date, epoch, tz, err := parseCommitIdentity(strings.TrimPrefix(line, "committer "))
			if err != nil {
				return rewriteBaselineCommitData{}, err
			}
			commit.CommitterName = name
			commit.CommitterEmail = email
			commit.CommitterDate = date
			commit.CommitterEpoch = epoch
			commit.CommitterTZ = tz
		}
	}
	if commit.SHA == "" || commit.Tree == "" || commit.AuthorDate == "" || commit.CommitterDate == "" {
		return rewriteBaselineCommitData{}, fmt.Errorf("commit %s is missing required metadata", prefix(sha))
	}
	return commit, nil
}

func parseCommitIdentity(value string) (string, string, string, int64, string, error) {
	matches := commitIdentityRe.FindStringSubmatch(value)
	if matches == nil {
		return "", "", "", 0, "", fmt.Errorf("malformed commit identity %q", value)
	}
	date := matches[3]
	fields := strings.Fields(date)
	if len(fields) != 2 {
		return "", "", "", 0, "", fmt.Errorf("malformed commit date %q", date)
	}
	epoch, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return "", "", "", 0, "", err
	}
	return matches[1], matches[2], date, epoch, fields[1], nil
}

func rewriteBaselineDir(gitDir string) string {
	return filepath.Join(rewriteDatesGitMetadataDir(gitDir), filepath.FromSlash(rewriteBaselineRelDir))
}

func rewriteBaselineManifestPath(gitDir string) string {
	return filepath.Join(rewriteBaselineDir(gitDir), "manifest.json")
}

func rewriteBaselineEntryBundlePath(gitDir string, entry rewriteBaselineEntry) string {
	if filepath.IsAbs(entry.BundlePath) {
		return entry.BundlePath
	}
	return filepath.Join(rewriteBaselineDir(gitDir), filepath.FromSlash(entry.BundlePath))
}

func absoluteRewriteBaselinePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

func rewriteBaselineRepresentedSHAs(manifest rewriteBaselineManifest) map[string]bool {
	represented := map[string]bool{}
	for _, entry := range manifest.Entries {
		if entry.FirstSHA != "" {
			represented[entry.FirstSHA] = true
		}
		if entry.CurrentSHA != "" {
			represented[entry.CurrentSHA] = true
		}
		for _, sha := range entry.KnownSHAs {
			if sha != "" {
				represented[sha] = true
			}
		}
	}
	return represented
}

func rewriteBaselineCaptureID() string {
	base := time.Now().UTC().Format("20060102T150405.000000000Z")
	return strings.NewReplacer(":", "", ".", "").Replace(base)
}

func importRewriteBaselineBundles(a *app, r repo, manifest rewriteBaselineManifest) error {
	seen := map[string]bool{}
	for _, entry := range manifest.Entries {
		bundlePath := rewriteBaselineEntryBundlePath(r.gitDir, entry)
		bundlePathForGit, err := absoluteRewriteBaselinePath(bundlePath)
		if err != nil {
			return fmt.Errorf("could not resolve baseline bundle %s: %w", filepath.Base(bundlePath), err)
		}
		if seen[bundlePathForGit] {
			continue
		}
		seen[bundlePathForGit] = true
		if _, err := a.git.Capture(a.ctx, r.dir, nil, "bundle", "unbundle", bundlePathForGit); err != nil {
			return fmt.Errorf("could not import baseline bundle %s: %w", filepath.Base(bundlePath), err)
		}
	}
	return nil
}

func createCommitFromBaselineData(a *app, dir string, data rewriteBaselineCommitData, parents []string) (string, error) {
	args := []string{"commit-tree", data.Tree}
	for _, parent := range parents {
		if parent != "" {
			args = append(args, "-p", parent)
		}
	}
	env := []string{
		"GIT_AUTHOR_NAME=" + data.AuthorName,
		"GIT_AUTHOR_EMAIL=" + data.AuthorEmail,
		"GIT_AUTHOR_DATE=" + data.AuthorDate,
		"GIT_COMMITTER_NAME=" + data.CommitterName,
		"GIT_COMMITTER_EMAIL=" + data.CommitterEmail,
		"GIT_COMMITTER_DATE=" + data.CommitterDate,
	}
	ctx := run.WithStdin(a.ctx, data.Message)
	out, err := run.Stdout(ctx, a.runner, dir, env, "git", args...)
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(firstLine(out))
	if sha == "" {
		return "", fmt.Errorf("git commit-tree returned an empty object id")
	}
	return sha, nil
}

func baselineEntryCommitData(entry rewriteBaselineEntry) rewriteBaselineCommitData {
	return rewriteBaselineCommitData{
		SHA:            entry.FirstSHA,
		Tree:           entry.TreeSHA,
		Parents:        append([]string(nil), entry.ParentSHAs...),
		AuthorName:     entry.AuthorName,
		AuthorEmail:    entry.AuthorEmail,
		AuthorDate:     entry.AuthorDate,
		AuthorEpoch:    entry.AuthorEpoch,
		AuthorTZ:       entry.AuthorTZ,
		CommitterName:  entry.CommitterName,
		CommitterEmail: entry.CommitterEmail,
		CommitterDate:  entry.CommitterDate,
		CommitterEpoch: entry.CommitterEpoch,
		CommitterTZ:    entry.CommitterTZ,
		Message:        entry.Message,
	}
}

func sortedUniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
