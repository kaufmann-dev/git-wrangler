package githubcli

import (
	"reflect"
	"testing"
)

func TestRepoListArgs(t *testing.T) {
	got := RepoListArgs("octo", "private", "7")
	want := []string{"repo", "list", "octo", "--visibility", "private", "--limit", "7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
	got = RepoListArgs("octo", "all", "7")
	want = []string{"repo", "list", "octo", "--limit", "7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}
