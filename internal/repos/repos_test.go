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

func TestDiscoverFindsLinkedWorktreeGitFile(t *testing.T) {
	temp := t.TempDir()
	mainDir := filepath.Join(temp, "main")
	root := filepath.Join(temp, "worktrees_root")
	commonGitDir := filepath.Join(mainDir, ".git", "worktrees", "linked")
	worktree := filepath.Join(root, "linked")
	if err := os.MkdirAll(commonGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: ../../main/.git/worktrees/linked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repositories, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(repositories) != 1 {
		t.Fatalf("got %d repos", len(repositories))
	}
	if repositories[0].Dir != worktree {
		t.Fatalf("dir = %q, want %q", repositories[0].Dir, worktree)
	}
	if repositories[0].GitDir != filepath.Join(worktree, ".git") {
		t.Fatalf("gitDir = %q", repositories[0].GitDir)
	}
}

func TestResolveExactFindsGitDirectory(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveExact(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got.Dir != repo {
		t.Fatalf("dir = %q, want %q", got.Dir, repo)
	}
	if got.GitDir != filepath.Join(repo, ".git") {
		t.Fatalf("gitDir = %q", got.GitDir)
	}
}

func TestResolveExactFindsLinkedWorktreeGitFile(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "linked")
	commonGitDir := filepath.Join(root, "main", ".git", "worktrees", "linked")
	if err := os.MkdirAll(commonGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: ../main/.git/worktrees/linked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveExact(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if got.Dir != worktree {
		t.Fatalf("dir = %q, want %q", got.Dir, worktree)
	}
}

func TestResolveExactRejectsNonRepository(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveExact(root); err == nil {
		t.Fatal("expected non-repository error")
	}
}

func TestResolveExactRejectsBareRepository(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveExact(root)
	if err == nil || !strings.Contains(err.Error(), "bare Git repository") {
		t.Fatalf("expected bare repository error, got %v", err)
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
