package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestCategorizeCommit(t *testing.T) {
	tests := []struct {
		name     string
		diff     string
		expected string
	}{
		{
			name:     "single doc addition",
			diff:     "A\tREADME.md",
			expected: "docs: add README.md",
		},
		{
			name:     "multiple doc modifications",
			diff:     "M\tdocs/index.md\nM\tLICENSE",
			expected: "docs: update docs/",
		},
		{
			name:     "single test file modification",
			diff:     "M\tinternal/cli/cli_test.go",
			expected: "test: update internal/cli/cli_test.go",
		},
		{
			name:     "javascript test file addition",
			diff:     "A\tindex.test.js",
			expected: "test: add index.test.js",
		},
		{
			name:     "ruby spec file addition",
			diff:     "A\tspec/helper_spec.rb",
			expected: "test: add spec/helper_spec.rb",
		},
		{
			name:     "github workflow config addition",
			diff:     "A\t.github/workflows/ci.yml",
			expected: "chore: add .github/workflows/ci.yml",
		},
		{
			name:     "makefile modification",
			diff:     "M\tMakefile",
			expected: "chore: update Makefile",
		},
		{
			name:     "source file addition (feature)",
			diff:     "A\tmain.go",
			expected: "feat: add main.go",
		},
		{
			name:     "source file modification (fix)",
			diff:     "M\tmain.go",
			expected: "fix: update main.go",
		},
		{
			name:     "source file addition and deletion (fix)",
			diff:     "A\tmain.go\nD\told.go",
			expected: "fix: update main.go",
		},
		{
			name:     "source file pure deletion (chore)",
			diff:     "D\tmain.go",
			expected: "chore: remove main.go",
		},
		{
			name:     "mixed config and test, no src (chore)",
			diff:     "M\tMakefile\nM\tmain_test.go",
			expected: "chore: update Makefile",
		},
		{
			name:     "mixed src and doc (fix or feat based on diff)",
			diff:     "M\tmain.go\nM\tREADME.md",
			expected: "fix: update main.go",
		},
		{
			name:     "empty diff",
			diff:     "",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := categorizeCommit(tc.diff)
			if actual != tc.expected {
				t.Errorf("categorizeCommit(%q) = %q, expected %q", tc.diff, actual, tc.expected)
			}
		})
	}
}

func TestWriteCommitCallbackUsesBytesLiterals(t *testing.T) {
	path, err := writeCommitCallback(map[string]string{
		"abc123": "feat: add café 😀 \"quotes\" \\ slash",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "😀") || strings.Contains(text, "café") {
		t.Fatalf("callback contains raw non-ASCII:\n%s", text)
	}
	for _, want := range []string{`\xc3\xa9`, `\xf0\x9f\x98\x80`, `"quotes"`, `\\ slash`} {
		if !strings.Contains(text, want) {
			t.Fatalf("callback missing %q:\n%s", want, text)
		}
	}
}

func TestRunFilterRepoRestoresOriginAfterFailure(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	remoteGetCalls := 0
	restored := false
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if dir != "repo" {
			return "", "", errors.New("unexpected dir")
		}
		joined := name + " " + strings.Join(args, " ")
		switch joined {
		case "git remote get-url origin":
			remoteGetCalls++
			if remoteGetCalls == 1 {
				return "https://example.test/repo.git\n", "", nil
			}
			return "", "", errors.New("origin removed")
		case "git-filter-repo --force":
			return "partial rewrite output", "", errors.New("filter failed")
		case "git remote add origin https://example.test/repo.git":
			restored = true
			return "", "", nil
		default:
			return "", "", errors.New("unexpected command: " + joined)
		}
	}}
	a := newApp(context.Background(), runner, strings.NewReader(""), io.Discard, io.Discard)

	out, runErr, restoreErr := runFilterRepoRestoringOrigin(a, "repo", []string{"git-filter-repo"}, []string{"--force"}, nil)
	if runErr == nil {
		t.Fatal("expected filter failure")
	}
	if restoreErr != nil {
		t.Fatalf("unexpected restore error: %v", restoreErr)
	}
	if out != "partial rewrite output" {
		t.Fatalf("output = %q", out)
	}
	if !restored {
		t.Fatal("expected origin restore after filter failure")
	}
}
