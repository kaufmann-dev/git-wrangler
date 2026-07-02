package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandRepositoryTargetsDiscoverAndExactRepo(t *testing.T) {
	root := t.TempDir()
	parent := makeGitDir(t, root, "parent")
	makeGitDir(t, parent, "nested")
	makeGitDir(t, root, "sibling")

	t.Chdir(root)
	discovered, err := resolveRepositoryTargets("")
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 3 {
		t.Fatalf("discovered %d repos, want 3", len(discovered))
	}

	exact, err := resolveRepositoryTargets(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(exact) != 1 {
		t.Fatalf("exact targets = %d, want 1", len(exact))
	}
	if exact[0].dir != parent {
		t.Fatalf("exact target = %q, want %q", exact[0].dir, parent)
	}
}

func TestStatusJSONExactRepoSuppressesHumanOutput(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	root := t.TempDir()
	parent := makeGitDir(t, root, "parent")
	makeGitDir(t, parent, "nested")
	var inspected []string
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if strings.Join(args, " ") == "fetch --prune origin" {
				return "fetched\n", "", nil
			}
			if strings.Join(args, " ") != "status --porcelain=v2 --branch" {
				return "", "", errors.New("unexpected git command")
			}
			inspected = append(inspected, filepath.Base(dir))
			return "# branch.upstream origin/main\n# branch.ab +0 -0\n", "", nil
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"status", "--repo", parent, "--json"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("status --json returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if strings.Contains(stdout.String(), "\x1b[") || strings.Contains(stdout.String(), "Checking status") {
		t.Fatalf("json output contains ANSI or progress text:\n%s", stdout.String())
	}
	if got := strings.Join(inspected, ","); got != "parent" {
		t.Fatalf("inspected = %s, want parent", got)
	}
	var doc struct {
		OK           bool `json:"ok"`
		Repositories []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout.String())
	}
	if !doc.OK || len(doc.Repositories) != 1 || doc.Repositories[0].State != "clean" {
		t.Fatalf("unexpected json document: %+v", doc)
	}
}

func TestStatusJSONPerRepoFailureExitsNonzero(t *testing.T) {
	root := tempGitRepos(t, "failed")
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if strings.Join(args, " ") == "fetch --prune origin" {
				return "fetched\n", "", nil
			}
			return "", "status failed", errors.New("status failed")
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"status", "--json"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	var doc struct {
		OK           bool `json:"ok"`
		Repositories []struct {
			Error *jsonError `json:"error"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout.String())
	}
	if doc.OK || len(doc.Repositories) != 1 || doc.Repositories[0].Error == nil {
		t.Fatalf("unexpected json document: %+v", doc)
	}
}

func TestRewriteAuthorsDeclineSkipsSuccessfully(t *testing.T) {
	root := tempGitRepos(t, "one", "two")
	t.Chdir(root)
	filterRan := false
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			switch name {
			case "git":
				return "/usr/bin/git", nil
			case "git-filter-repo":
				return "/usr/bin/git-filter-repo", nil
			default:
				return "", errors.New("unexpected command")
			}
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := name + " " + strings.Join(args, " ")
			hash := fakeRewriteAuthorHash(dir)
			switch {
			case joined == "git fetch --prune origin":
				return "fetched\n", "", nil
			case joined == "git rev-parse HEAD":
				return hash + "\n", "", nil
			case joined == "git for-each-ref --format=%(refname)%00%(objectname) refs/heads":
				return "refs/heads/main\x00" + hash + "\n", "", nil
			case strings.HasPrefix(joined, "git log --topo-order --reverse --format="):
				return fakeRewriteAuthorLog(hash), "", nil
			case name == "/usr/bin/git-filter-repo":
				filterRan = true
			}
			return "", "", nil
		},
	}
	var stdout, stderr bytes.Buffer
	err := executeInteractive(t, context.Background(), runner, []string{"rewrite-authors", "--name", "New Name", "--email", "new@example.test"}, strings.NewReader("n\n"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("declined rewrite-authors returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if filterRan {
		t.Fatal("filter-repo should not run after declined confirmation")
	}
	if strings.Count(stderr.String(), "Rewrite author and committer identity") != 1 {
		t.Fatalf("expected one confirmation prompt:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Summary: 0 rewritten, 2 skipped, 0 failed") {
		t.Fatalf("missing skip output:\n%s", stdout.String())
	}
}

func TestConfirmationsAreNotInsideRangeLoops(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, data, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			rangeStmt, ok := node.(*ast.RangeStmt)
			if !ok {
				return true
			}
			ast.Inspect(rangeStmt.Body, func(bodyNode ast.Node) bool {
				call, ok := bodyNode.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if ok && (ident.Name == "confirm" || ident.Name == "confirmOrSkip") {
					t.Fatalf("%s asks for confirmation inside a range loop", file)
				}
				return true
			})
			return false
		})
	}
}

func TestDirectFlagReadsStayInOptionsOrGuidedPlumbing(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || file == "options.go" || file == "prompts.go" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, forbidden := range []string{".Flags().Get", ".PersistentFlags().Get"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s reads Cobra flags directly with %q; use an options parser", file, forbidden)
			}
		}
	}
}

func TestGuidedMetadataIsNotStoredInCobraContext(t *testing.T) {
	for _, file := range []string{"command_specs.go", "prompts.go"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, forbidden := range []string{"guidedContextKey", "context.WithValue", ".SetContext(", ".Context().Value"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s stores guided metadata through Cobra context with %q", file, forbidden)
			}
		}
	}
}
