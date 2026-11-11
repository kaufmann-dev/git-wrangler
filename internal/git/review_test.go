package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.test", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return string(out)
}
func writeReviewFile(t *testing.T, dir, path, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, path), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestOriginComparisonAndReview(t *testing.T) {
	ctx := context.Background()
	c := New(nil)
	dir := t.TempDir()
	testGit(t, dir, "init", "-b", "main")
	compare := func() Comparison {
		t.Helper()
		r, e := c.CompareOrigin(ctx, dir)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	if r := compare(); r.State != "unborn" {
		t.Fatal(r)
	}
	writeReviewFile(t, dir, "file", "one\n")
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "initial")
	if r := compare(); r.State != "unpublished" || *r.Ahead != 1 {
		t.Fatal(r)
	}
	testGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	testGit(t, dir, "update-ref", "refs/remotes/other/main", "HEAD")
	testGit(t, dir, "config", "branch.main.remote", "other")
	testGit(t, dir, "config", "branch.main.merge", "refs/heads/main")
	writeReviewFile(t, dir, "file", "one\ntwo\n")
	testGit(t, dir, "commit", "-am", "edit")
	r := compare()
	if *r.Ahead != 1 || *r.Behind != 0 || r.Target != "origin/main" {
		t.Fatal(r)
	}
	files, e := c.DiffChanges(ctx, dir, r.Base, "HEAD")
	if e != nil || len(files) != 1 || *files[0].Additions != 1 {
		t.Fatalf("%+v %v", files, e)
	}
	// A rewritten root with the same final tree has no merge base but no content changes.
	testGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	testGit(t, dir, "checkout", "--orphan", "replacement")
	testGit(t, dir, "commit", "-m", "rewritten")
	testGit(t, dir, "branch", "-M", "main")
	r = compare()
	if r.Method != "tree comparison; no common ancestor" {
		t.Fatal(r)
	}
	files, e = c.DiffChanges(ctx, dir, r.Base, "HEAD")
	if e != nil || len(files) != 0 {
		t.Fatalf("%+v %v", files, e)
	}
	testGit(t, dir, "checkout", "--detach")
	if r := compare(); r.State != "detached" || r.Ahead != nil {
		t.Fatal(r)
	}
}
func TestReviewFilesAndIndexPreservation(t *testing.T) {
	ctx := context.Background()
	c := New(nil)
	dir := t.TempDir()
	testGit(t, dir, "init", "-b", "main")
	writeReviewFile(t, dir, "old", "one\ntwo\n")
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "initial")
	testGit(t, dir, "mv", "old", "new\tname")
	before, e := os.ReadFile(filepath.Join(dir, ".git", "index"))
	if e != nil {
		t.Fatal(e)
	}
	files, e := c.DiffChanges(ctx, dir, "--cached")
	if e != nil || len(files) != 1 || files[0].PreviousPath != "old" || files[0].Path != "new\tname" || *files[0].Additions != 0 {
		t.Fatalf("%+v %v", files, e)
	}
	writeReviewFile(t, dir, "new\tname", "one\ntwo\nthree\n")
	files, e = c.DiffChanges(ctx, dir)
	if e != nil || len(files) != 1 || *files[0].Additions != 1 {
		t.Fatalf("%+v %v", files, e)
	}
	writeReviewFile(t, dir, "binary", "\x00data")
	writeReviewFile(t, dir, "empty", "")
	writeReviewFile(t, dir, "text", "hello\nworld")
	files, e = c.UntrackedChanges(ctx, dir)
	if e != nil || len(files) != 3 {
		t.Fatalf("%+v %v", files, e)
	}
	if !files[0].Binary || *files[1].Additions != 0 || *files[2].Additions != 2 {
		t.Fatal(files)
	}
	after, e := os.ReadFile(filepath.Join(dir, ".git", "index"))
	if e != nil || string(before) != string(after) {
		t.Fatal("index changed")
	}
}
func TestParseReviewConflictsAndBinary(t *testing.T) {
	files, e := parseChanges("U\x00conflict\x00M\x00conflict\x00M\x00binary\x00T\x00type\x00", "0\t0\tconflict\x001\t2\tconflict\x00-\t-\tbinary\x000\t0\ttype\x00")
	if e != nil || len(files) != 3 {
		t.Fatalf("%+v %v", files, e)
	}
	if files[0].Additions != nil || !files[1].Binary || files[2].Status != "T" {
		t.Fatal(files)
	}
}

func TestReviewDivergenceRevertsAndUnpublishedSnapshot(t *testing.T) {
	ctx := context.Background()
	c := New(nil)
	dir := t.TempDir()
	testGit(t, dir, "init", "-b", "main")
	writeReviewFile(t, dir, "file", "base\n")
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "base")
	testGit(t, dir, "branch", "remote-side")
	testGit(t, dir, "checkout", "remote-side")
	writeReviewFile(t, dir, "remote-only", "remote\n")
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "remote")
	testGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	testGit(t, dir, "checkout", "main")
	writeReviewFile(t, dir, "file", "base\nlocal\n")
	testGit(t, dir, "commit", "-am", "local")
	r, e := c.CompareOrigin(ctx, dir)
	if e != nil || *r.Ahead != 1 || *r.Behind != 1 {
		t.Fatalf("%+v %v", r, e)
	}
	files, e := c.DiffChanges(ctx, dir, r.Base, "HEAD")
	if e != nil || len(files) != 1 || files[0].Path != "file" {
		t.Fatalf("%+v %v", files, e)
	}
	testGit(t, dir, "revert", "--no-edit", "HEAD")
	r, e = c.CompareOrigin(ctx, dir)
	if e != nil || *r.Ahead != 2 {
		t.Fatalf("%+v %v", r, e)
	}
	files, e = c.DiffChanges(ctx, dir, r.Base, "HEAD")
	if e != nil || len(files) != 0 {
		t.Fatalf("%+v %v", files, e)
	}
	testGit(t, dir, "checkout", "-b", "new-branch")
	r, e = c.CompareOrigin(ctx, dir)
	if e != nil || r.State != "unpublished" || *r.Ahead != 3 {
		t.Fatalf("%+v %v", r, e)
	}
	files, e = c.DiffChanges(ctx, dir, r.Base, "HEAD")
	if e != nil || len(files) != 1 || *files[0].Additions != 1 {
		t.Fatalf("%+v %v", files, e)
	}
}
