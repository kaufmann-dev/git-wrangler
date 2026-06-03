package cli

import "testing"

func TestReadOnlyWorkerCountIsBounded(t *testing.T) {
	t.Parallel()
	if got := readOnlyWorkerCount(1000); got < 1 || got > 32 {
		t.Fatalf("worker count = %d, want 1..32", got)
	}
	if got := readOnlyWorkerCount(2); got != 2 {
		t.Fatalf("worker count for two repos = %d, want 2", got)
	}
}

func TestGitMutationWorkerCountIsBounded(t *testing.T) {
	t.Parallel()
	if got := gitMutationWorkerCount(1000); got < 1 || got > 4 {
		t.Fatalf("worker count = %d, want 1..4", got)
	}
	if got := gitMutationWorkerCount(2); got != 2 {
		t.Fatalf("worker count for two repos = %d, want 2", got)
	}
}

func TestParallelReposPreservesOrder(t *testing.T) {
	t.Parallel()
	repos := []repo{{display: "a"}, {display: "b"}, {display: "c"}}
	got := parallelRepos(repos, func(r repo) string {
		return r.display + "!"
	})
	want := []string{"a!", "b!", "c!"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("result[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
