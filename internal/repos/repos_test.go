package repos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverFindsReposDeterministically(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"b/.git", "a/.git", "nested/c/.git"} {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	repositories, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(repositories))
	for _, repo := range repositories {
		got = append(got, filepath.Base(repo.Dir))
	}
	want := "a,b,c"
	if strings.Join(got, ",") != want {
		t.Fatalf("repos = %v, want %s", got, want)
	}
}

func TestDisplayNameForCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir(".git", 0o755); err != nil {
		t.Fatal(err)
	}
	repositories, err := Discover(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(repositories) != 1 {
		t.Fatalf("got %d repos", len(repositories))
	}
	if repositories[0].Display != filepath.Base(root) {
		t.Fatalf("display = %q, want %q", repositories[0].Display, filepath.Base(root))
	}
}

func TestReposPackageHasNoSubprocessDependency(t *testing.T) {
	data, err := os.ReadFile("repos.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "os/exec") || strings.Contains(string(data), "exec.") {
		t.Fatal("repos package must not execute subprocesses")
	}
}
