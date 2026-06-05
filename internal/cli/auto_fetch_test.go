package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func TestRemoteAwareCommandsFetchOriginBeforeInspection(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, tc := range []struct {
		name       string
		args       []string
		inspection string
	}{
		{name: "status", args: []string{"status"}, inspection: "status --porcelain=v2 --branch"},
		{name: "info", args: []string{"info"}, inspection: "status --porcelain"},
		{name: "review", args: []string{"review"}, inspection: "rev-list HEAD --not --remotes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := tempGitRepos(t, "repo")
			t.Chdir(root)
			commands := []string{}
			runner := fakeRunner{
				lookPath: fakeGitLookPath,
				run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
					joined := strings.Join(args, " ")
					commands = append(commands, joined)
					return remoteAwareCommandOutput(joined)
				},
				pipe: fakeLargestFilesPipe,
			}

			var stdout, stderr bytes.Buffer
			if err := ExecuteWithRunner(context.Background(), runner, tc.args, strings.NewReader(""), &stdout, &stderr); err != nil {
				t.Fatalf("%s returned error: %v\nstdout: %s\nstderr: %s", tc.name, err, stdout.String(), stderr.String())
			}
			if len(commands) < 2 {
				t.Fatalf("commands = %v, want fetch and inspection", commands)
			}
			if commands[0] != "fetch --prune origin" {
				t.Fatalf("first command = %q, want auto-fetch", commands[0])
			}
			if commands[1] != tc.inspection {
				t.Fatalf("second command = %q, want %q; all commands: %v", commands[1], tc.inspection, commands)
			}
		})
	}
}

func TestRemoteAwareNoFetchSkipsOriginRefresh(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "status", args: []string{"status", "--no-fetch"}},
		{name: "info", args: []string{"info", "--no-fetch"}},
		{name: "review", args: []string{"review", "--no-fetch"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := tempGitRepos(t, "repo")
			t.Chdir(root)
			runner := fakeRunner{
				lookPath: fakeGitLookPath,
				run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
					joined := strings.Join(args, " ")
					if joined == "fetch --prune origin" {
						return "", "", errors.New("fetch should not run")
					}
					return remoteAwareCommandOutput(joined)
				},
				pipe: fakeLargestFilesPipe,
			}

			var stdout, stderr bytes.Buffer
			if err := ExecuteWithRunner(context.Background(), runner, tc.args, strings.NewReader(""), &stdout, &stderr); err != nil {
				t.Fatalf("%s returned error: %v\nstdout: %s\nstderr: %s", tc.name, err, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRemoteAwareJSONFetchFailureRowsAreSilent(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "status", args: []string{"status", "--json"}},
		{name: "info", args: []string{"info", "--json"}},
		{name: "review", args: []string{"review", "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := tempGitRepos(t, "repo")
			t.Chdir(root)
			inspected := false
			runner := fakeRunner{
				lookPath: fakeGitLookPath,
				run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
					joined := strings.Join(args, " ")
					if joined == "fetch --prune origin" {
						return "", "offline", errors.New("fetch failed")
					}
					inspected = true
					return remoteAwareCommandOutput(joined)
				},
				pipe: fakeLargestFilesPipe,
			}

			var stdout, stderr bytes.Buffer
			err := ExecuteWithRunner(context.Background(), runner, tc.args, strings.NewReader(""), &stdout, &stderr)
			assertExitCode(t, err, 1)
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if inspected {
				t.Fatal("repository inspection should not run after fetch failure")
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
			if !strings.Contains(doc.Repositories[0].Error.Message, "git fetch failed") || !strings.Contains(doc.Repositories[0].Error.Message, "offline") {
				t.Fatalf("unexpected fetch error message: %+v", doc.Repositories[0].Error)
			}
		})
	}
}

func TestRemoveSecretsFetchFailureStopsBeforeScan(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	scanned := false
	runner := fakeRunner{
		lookPath: fakeGitAndFilterRepoLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := name + " " + strings.Join(args, " ")
			if joined == "git fetch --prune origin" {
				return "", "offline", errors.New("fetch failed")
			}
			scanned = true
			return "", "", errors.New("unexpected command: " + joined)
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"remove-secrets", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if scanned {
		t.Fatal("history scan should not run after fetch failure")
	}
	if !strings.Contains(stderr.String(), "git fetch failed") {
		t.Fatalf("missing fetch failure output:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
}

func TestRewriteCommitsFetchFailureStopsBeforeAIGeneration(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("GIT_WRANGLER_AI_API_KEY", "test-key")
	configPath := filepath.Join(configDir, "git-wrangler", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"schema_version":1,"ai":{"base_url":"https://example.test/v1","model":"test-model"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	aiScanned := false
	runner := fakeRunner{
		lookPath: fakeGitAndFilterRepoLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := name + " " + strings.Join(args, " ")
			if joined == "git fetch --prune origin" {
				return "", "offline", errors.New("fetch failed")
			}
			aiScanned = true
			return "", "", errors.New("unexpected command: " + joined)
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"rewrite-commits", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if aiScanned {
		t.Fatal("AI scanning or generation should not start after fetch failure")
	}
	if !strings.Contains(stderr.String(), "git fetch failed") {
		t.Fatalf("missing fetch failure output:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
}

func TestRewriteDatesNoFetchWarnsAndSkipsFetch(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	fetched := false
	runner := fakeRunner{
		lookPath: fakeGitAndFilterRepoLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			joined := strings.Join(args, " ")
			if joined == "fetch --prune origin" {
				fetched = true
				return "", "", errors.New("fetch should not run")
			}
			if joined == "rev-parse HEAD" {
				return "", "", errors.New("unborn branch")
			}
			return "", "", errors.New("unexpected git args: " + joined)
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"rewrite-dates", "--no-fetch", "--yes"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("rewrite-dates returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if fetched {
		t.Fatal("fetch should not run with --no-fetch")
	}
	if !strings.Contains(stderr.String(), "remote-only commits may be missed") {
		t.Fatalf("missing no-fetch warning:\n%s", stderr.String())
	}
}

func remoteAwareCommandOutput(joined string) (string, string, error) {
	switch joined {
	case "fetch --prune origin":
		return "fetched\n", "", nil
	case "status --porcelain=v2 --branch":
		return "# branch.upstream origin/main\n# branch.ab +0 -0\n", "", nil
	case "status --porcelain":
		return "", "", nil
	case "rev-parse --abbrev-ref HEAD":
		return "main\n", "", nil
	case "rev-parse HEAD":
		return "head\n", "", nil
	case "rev-list --left-right --count HEAD...@{u}":
		return "0 0\n", "", nil
	case "branch -a --no-color":
		return "* main\n", "", nil
	case "remote -v":
		return "", "", nil
	case "log --reverse --format=%ci - %s":
		return "2024-01-01 00:00:00 +0000 - first\n", "", nil
	case "rev-list --all --count":
		return "1\n", "", nil
	case "log --since=1 month ago --format=%ci":
		return "", "", nil
	case "log -1 --format=%ci - %s":
		return "2024-01-02 00:00:00 +0000 - last\n", "", nil
	case "log --format=%an <%ae>":
		return "A <a@example.test>\n", "", nil
	case "rev-list HEAD --not --remotes":
		return "", "", nil
	default:
		return "", "", errors.New("unexpected git args: " + joined)
	}
}

func fakeLargestFilesPipe(ctx context.Context, dir string, env []string, left run.Command, right run.Command, consume func(io.Reader) error) error {
	return consume(strings.NewReader("100 hash file.txt\n"))
}

func fakeGitAndFilterRepoLookPath(name string) (string, error) {
	if name == "git" || name == "git-filter-repo" {
		return "/usr/bin/" + name, nil
	}
	return "", errors.New("unexpected command")
}
