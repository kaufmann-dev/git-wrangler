package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestStatusRow(t *testing.T) {
	tests := []struct {
		name             string
		gitOutput        string
		expectedState    string // clean or dirty
		expectedTracking string // up to date, no remote, ahead/behind etc.
		expectedDirty    int
		expectedBehind   int
		expectedNoRemote int
	}{
		{
			name: "clean up to date",
			gitOutput: `# branch.oid 92d6e326bd6426466f28b49e1e98d9e7a83efee5
# branch.head main
# branch.upstream origin/main
# branch.ab +0 -0
`,
			expectedState:    "clean",
			expectedTracking: "up to date",
			expectedDirty:    0,
			expectedBehind:   0,
			expectedNoRemote: 0,
		},
		{
			name: "clean no remote",
			gitOutput: `# branch.oid 92d6e326bd6426466f28b49e1e98d9e7a83efee5
# branch.head main
`,
			expectedState:    "clean",
			expectedTracking: "no remote",
			expectedDirty:    0,
			expectedBehind:   0,
			expectedNoRemote: 1,
		},
		{
			name: "dirty up to date",
			gitOutput: `# branch.oid 92d6e326bd6426466f28b49e1e98d9e7a83efee5
# branch.head main
# branch.upstream origin/main
# branch.ab +0 -0
1 .M N... 100644 100644 100644 92d6e326bd6426466f28b49e1e98d9e7a83efee5 92d6e326bd6426466f28b49e1e98d9e7a83efee5 main.go
`,
			expectedState:    "dirty",
			expectedTracking: "up to date",
			expectedDirty:    1,
			expectedBehind:   0,
			expectedNoRemote: 0,
		},
		{
			name: "clean ahead and behind",
			gitOutput: `# branch.oid 92d6e326bd6426466f28b49e1e98d9e7a83efee5
# branch.head main
# branch.upstream origin/main
# branch.ab +2 -3
`,
			expectedState:    "clean",
			expectedTracking: "ahead 2, behind 3",
			expectedDirty:    0,
			expectedBehind:   1,
			expectedNoRemote: 0,
		},
		{
			name: "clean ahead only",
			gitOutput: `# branch.oid 92d6e326bd6426466f28b49e1e98d9e7a83efee5
# branch.head main
# branch.upstream origin/main
# branch.ab +2 -0
`,
			expectedState:    "clean",
			expectedTracking: "ahead 2",
			expectedDirty:    0,
			expectedBehind:   0,
			expectedNoRemote: 0,
		},
		{
			name: "clean behind only",
			gitOutput: `# branch.oid 92d6e326bd6426466f28b49e1e98d9e7a83efee5
# branch.head main
# branch.upstream origin/main
# branch.ab +0 -3
`,
			expectedState:    "clean",
			expectedTracking: "behind 3",
			expectedDirty:    0,
			expectedBehind:   1,
			expectedNoRemote: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
				if name == "git" && len(args) >= 2 && args[0] == "status" && args[1] == "--porcelain=v2" {
					return tc.gitOutput, "", nil
				}
				return "", "", nil
			}}

			var stdoutBuf bytes.Buffer
			a := newApp(context.Background(), runner, strings.NewReader(""), &stdoutBuf, &stdoutBuf)
			r := repo{dir: "dummy-dir", display: "dummy-repo"}

			row, err := statusRow(a, r)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Strip color codes before checking state/tracking
			stateCleaned := stripColor(row.state)
			trackingCleaned := stripColor(row.tracking)

			if stateCleaned != tc.expectedState {
				t.Errorf("state: got %q, expected %q", stateCleaned, tc.expectedState)
			}
			if trackingCleaned != tc.expectedTracking {
				t.Errorf("tracking: got %q, expected %q", trackingCleaned, tc.expectedTracking)
			}
			if row.dirty != tc.expectedDirty {
				t.Errorf("dirty: got %d, expected %d", row.dirty, tc.expectedDirty)
			}
			if row.behind != tc.expectedBehind {
				t.Errorf("behind: got %d, expected %d", row.behind, tc.expectedBehind)
			}
			if row.noRemote != tc.expectedNoRemote {
				t.Errorf("noRemote: got %d, expected %d", row.noRemote, tc.expectedNoRemote)
			}
		})
	}
}

func TestStatusProgressCompletesBeforeTableOutput(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one", "two")
	t.Chdir(root)

	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" {
				return "", "", errors.New("unexpected command")
			}
			switch strings.Join(args, " ") {
			case "fetch --prune origin":
				return "fetched\n", "", nil
			case "status --porcelain=v2 --branch":
				return "# branch.upstream origin/main\n# branch.ab +0 -0\n", "", nil
			default:
				return "", "", errors.New("unexpected git args")
			}
		},
	}

	var combined bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"status"}, strings.NewReader(""), &combined, &combined)
	if err != nil {
		t.Fatalf("status returned error: %v\n%s", err, combined.String())
	}
	out := combined.String()
	lastProgress := strings.LastIndex(out, "Checking status")
	table := strings.Index(out, "Repository")
	if lastProgress < 0 {
		t.Fatalf("missing progress output:\n%s", out)
	}
	if table < 0 {
		t.Fatalf("missing status table:\n%s", out)
	}
	if lastProgress > table {
		t.Fatalf("progress appeared after table started:\n%s", out)
	}
	if strings.Contains(out[table:], "Checking status") {
		t.Fatalf("progress appeared inside durable table output:\n%s", out)
	}
}

// Simple color stripping utility for test assertions
func stripColor(s string) string {
	s = strings.ReplaceAll(s, "\x1b[32m", "") // green
	s = strings.ReplaceAll(s, "\x1b[33m", "") // yellow
	s = strings.ReplaceAll(s, "\x1b[31m", "") // red
	s = strings.ReplaceAll(s, "\x1b[36m", "") // cyan
	s = strings.ReplaceAll(s, "\x1b[2m", "")  // dim/muted
	s = strings.ReplaceAll(s, "\x1b[0m", "")  // reset
	return s
}
