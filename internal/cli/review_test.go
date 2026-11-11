package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/kaufmann-dev/git-wrangler/internal/run"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewLocalChangesJSON(t *testing.T) {
	dir := t.TempDir()
	runGitForTest(t, dir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitForTest(t, dir, "add", ".")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("one\ntwo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), run.New(), []string{"review", "--repo", dir, "--no-fetch", "--json"}, bytes.NewReader(nil), &out, &stderr)
	if err != nil {
		t.Fatalf("%v: %s %s", err, out.String(), stderr.String())
	}
	var result struct {
		Repositories []reviewResult `json:"repositories"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	r := result.Repositories[0]
	if len(r.Staged) != 1 || len(r.Unstaged) != 1 || r.Comparison.State != "unborn" {
		t.Fatalf("unexpected result: %+v", r)
	}
	if *r.Staged[0].Additions != 1 || *r.Unstaged[0].Additions != 1 {
		t.Fatalf("wrong counts: %s", out.String())
	}
	if stderr.Len() != 0 {
		t.Fatal(stderr.String())
	}
}

func TestReviewRetainsLocalFilesOnFetchFailure(t *testing.T) {
	dir := t.TempDir()
	runGitForTest(t, dir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), run.New(), []string{"review", "--repo", dir, "--json"}, bytes.NewReader(nil), &out, &stderr)
	assertExitCode(t, err, 1)
	var result struct {
		Repositories []reviewResult `json:"repositories"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	r := result.Repositories[0]
	if len(r.Untracked) != 1 || r.Unpushed != nil || r.Error == nil {
		t.Fatalf("unexpected result: %s", out.String())
	}
	if stderr.Len() != 0 {
		t.Fatal(stderr.String())
	}
}
