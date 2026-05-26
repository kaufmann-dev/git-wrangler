package cli

import (
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
