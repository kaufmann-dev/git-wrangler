package cli

import (
	"bytes"
	"context"
	"encoding/json"
	gitclient "github.com/kaufmann-dev/git-wrangler/internal/git"
	"github.com/kaufmann-dev/git-wrangler/internal/run"
	"github.com/kaufmann-dev/git-wrangler/internal/ui"
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

func TestDiffstatScale(t *testing.T) {
	tests := []struct {
		add, del, width, maxChange int
		wantAdd, wantDel           int
	}{
		{5, 2, 40, 10, 5, 2},
		{795, 0, 40, 795, 40, 0},
		{1, 1, 40, 795, 1, 1},
		{0, 4, 40, 795, 0, 1},
		{100, 100, 40, 200, 20, 20},
		{150, 50, 40, 200, 30, 10},
	}
	for _, tt := range tests {
		add, del := diffstatScale(tt.add, tt.del, tt.width, tt.maxChange)
		if add != tt.wantAdd || del != tt.wantDel {
			t.Errorf("diffstatScale(%d, %d, %d, %d) = %d, %d; want %d, %d", tt.add, tt.del, tt.width, tt.maxChange, add, del, tt.wantAdd, tt.wantDel)
		}
	}
}

func reviewTestFiles() []gitclient.FileChange {
	n := func(v int) *int { return &v }
	return []gitclient.FileChange{
		{Status: "M", Path: "src/format.ts", Additions: n(30), Deletions: n(9)},
		{Status: "A", Path: "drizzle/snapshot.json", Additions: n(120), Deletions: n(0)},
		{Status: "R", Path: "b.go", PreviousPath: "a.go", Additions: n(0), Deletions: n(0)},
		{Status: "A", Path: "logo.png", Binary: true},
		{Status: "U", Path: "conflict.txt"},
	}
}

func TestRenderReviewSectionPlain(t *testing.T) {
	var out bytes.Buffer
	a := &app{stdout: &out}
	renderReviewSection(a, "Staged", reviewTestFiles(), false)
	want := "" +
		"  Staged — 5 files, +150 -9\n" +
		"    Edited      src/format.ts         |  39 ++++++++++---\n" +
		"    Added       drizzle/snapshot.json | 120 ++++++++++++++++++++++++++++++++++++++++\n" +
		"    Renamed     a.go → b.go           |   0\n" +
		"    Added       logo.png              | Bin\n" +
		"    Conflicted  conflict.txt          | unavailable\n"
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRenderReviewSectionEmptyForced(t *testing.T) {
	var out bytes.Buffer
	a := &app{stdout: &out}
	renderReviewSection(a, "Unpushed", []gitclient.FileChange{}, true)
	want := "  Unpushed — 0 files, +0 -0\n    No net file changes\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}

func TestRenderReviewSectionColors(t *testing.T) {
	var out bytes.Buffer
	a := &app{stdout: &out, ui: ui.Theme{Red: "<r>", Green: "<g>", Yellow: "<y>", Cyan: "<c>", Muted: "<m>", Bold: "<b>", Reset: "</>"}}
	n := func(v int) *int { return &v }
	renderReviewSection(a, "Staged", []gitclient.FileChange{{Status: "M", Path: "a.go", Additions: n(2), Deletions: n(1)}}, false)
	want := "  <b>Staged</> — 1 file, <g>+2</> <r>-1</>\n" +
		"    <y>Edited</>  a.go |   3 <g>++</><r>-</>\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
