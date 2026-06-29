package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func TestRewriteBaselineCaptureAddsOnlyMissingEntries(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	runGitForTest(t, "", "init", repoDir)
	runGitForTest(t, repoDir, "config", "user.name", "Test User")
	runGitForTest(t, repoDir, "config", "user.email", "test@example.test")
	commitEmptyForTest(t, repoDir, "first", "2020-01-01T10:00:00 +0000")
	commitEmptyForTest(t, repoDir, "second", "2020-01-02T10:00:00 +0000")
	r := repo{dir: repoDir, gitDir: filepath.Join(repoDir, ".git"), display: "repo"}
	a := newApp(context.Background(), run.New(), strings.NewReader(""), io.Discard, io.Discard)
	hashes := commitSHAsBySubject(t, repoDir)

	if err := captureRewriteBaselineForHashes(a, r, []string{hashes["first"], hashes["second"]}); err != nil {
		t.Fatal(err)
	}
	manifest, found, err := loadRewriteBaseline(r.gitDir)
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(manifest.Entries) != 2 {
		t.Fatalf("baseline found=%v entries=%d", found, len(manifest.Entries))
	}
	bundles := rewriteBaselineBundlesForTest(t, r.gitDir)
	if len(bundles) != 1 {
		t.Fatalf("bundle count = %d, want 1", len(bundles))
	}
	firstEntry := rewriteBaselineEntryByFirstForTest(manifest, hashes["first"])
	originalDate := firstEntry.AuthorDate

	if err := captureRewriteBaselineForHashes(a, r, []string{hashes["first"], hashes["second"]}); err != nil {
		t.Fatal(err)
	}
	manifest, _, err = loadRewriteBaseline(r.gitDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != 2 {
		t.Fatalf("duplicate capture added entries: %d", len(manifest.Entries))
	}
	if got := len(rewriteBaselineBundlesForTest(t, r.gitDir)); got != 1 {
		t.Fatalf("duplicate capture added bundle: %d", got)
	}

	commitEmptyForTest(t, repoDir, "third", "2020-01-03T10:00:00 +0000")
	if err := captureRewriteBaselineForLocalBranches(a, r); err != nil {
		t.Fatal(err)
	}
	manifest, _, err = loadRewriteBaseline(r.gitDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != 3 {
		t.Fatalf("later capture entries = %d, want 3", len(manifest.Entries))
	}
	if got := rewriteBaselineEntryByFirstForTest(manifest, hashes["first"]).AuthorDate; got != originalDate {
		t.Fatalf("existing original date changed from %q to %q", originalDate, got)
	}
	if got := len(rewriteBaselineBundlesForTest(t, r.gitDir)); got != 2 {
		t.Fatalf("new commit did not create exactly one new bundle: %d", got)
	}
}

func TestRewriteBaselineCommitMapUpdatesCurrentOnlyAndKeepsAliases(t *testing.T) {
	manifest := rewriteBaselineManifest{Version: rewriteBaselineVersion, Entries: []rewriteBaselineEntry{{
		FirstSHA:       "original",
		CurrentSHA:     "current",
		KnownSHAs:      []string{"original", "current"},
		TreeSHA:        "tree",
		AuthorDate:     "100 +0000",
		AuthorEpoch:    100,
		AuthorTZ:       "+0000",
		CommitterDate:  "100 +0000",
		CommitterEpoch: 100,
		CommitterTZ:    "+0000",
		CaptureID:      "capture",
		BundlePath:     "bundles/capture.bundle",
	}}}
	tmp := t.TempDir()
	gitDir := filepath.Join(tmp, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "git-wrangler", "baseline", "bundles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "git-wrangler", "baseline", "bundles", "capture.bundle"), []byte("bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeRewriteBaseline(gitDir, manifest); err != nil {
		t.Fatal(err)
	}
	if err := updateRewriteBaselineFromCommitMap(gitDir, map[string]string{"current": "next"}); err != nil {
		t.Fatal(err)
	}
	updated, _, err := loadRewriteBaseline(gitDir)
	if err != nil {
		t.Fatal(err)
	}
	entry := updated.Entries[0]
	if entry.CurrentSHA != "next" {
		t.Fatalf("current SHA = %q", entry.CurrentSHA)
	}
	if entry.FirstSHA != "original" || entry.AuthorDate != "100 +0000" {
		t.Fatalf("original metadata changed: %+v", entry)
	}
	for _, want := range []string{"original", "current", "next"} {
		if !containsStringForBaselineTest(entry.KnownSHAs, want) {
			t.Fatalf("known SHAs %v missing %s", entry.KnownSHAs, want)
		}
	}
}

func TestClearManagedRewriteMetadataClearsBaseline(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	gitDir := filepath.Join(repoDir, ".git")
	baselineDir := filepath.Join(gitDir, "git-wrangler", "baseline")
	if err := os.MkdirAll(baselineDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baselineDir, "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git update-ref -d refs/git-wrangler/state/rewrite-dates":
			return "", "", nil
		case "git for-each-ref --format=%(refname) refs/git-wrangler/backup/rewrite-dates":
			return "", "", nil
		default:
			return "", "", nil
		}
	}}
	a := newApp(context.Background(), runner, strings.NewReader(""), io.Discard, io.Discard)
	if err := clearManagedRewriteMetadata(a, repoDir, gitDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(baselineDir); !os.IsNotExist(err) {
		t.Fatalf("baseline directory still exists: %v", err)
	}
}

func rewriteBaselineBundlesForTest(t *testing.T, gitDir string) []string {
	t.Helper()
	bundles, err := filepath.Glob(filepath.Join(rewriteBaselineDir(gitDir), "bundles", "*.bundle"))
	if err != nil {
		t.Fatal(err)
	}
	return bundles
}

func rewriteBaselineEntryByFirstForTest(manifest rewriteBaselineManifest, sha string) rewriteBaselineEntry {
	for _, entry := range manifest.Entries {
		if entry.FirstSHA == sha {
			return entry
		}
	}
	return rewriteBaselineEntry{}
}

func containsStringForBaselineTest(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
