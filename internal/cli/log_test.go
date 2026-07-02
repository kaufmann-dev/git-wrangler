package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func logRecord(hash, date, subject string) string {
	return strings.Join([]string{hash, date, subject, ""}, "\x00")
}

func TestParseLogOutputFiltersAndParsesConventionalSubjects(t *testing.T) {
	t.Parallel()
	output := logRecord("one11111", "2026-01-04T12:00:00Z", "feat(cli): add log") +
		logRecord("two22222", "2026-01-05T12:00:00Z", "fix(core)!: repair bug") +
		logRecord("three333", "2026-01-06T12:00:00Z", "plain message")
	opts := logOptions{
		hasSince:  true,
		sinceUnix: parseDateStart("2026-01-05"),
		hasUntil:  true,
		untilUnix: parseDateEnd("2026-01-06"),
		types:     map[string]struct{}{"fix": {}, "other": {}},
		scopes:    map[string]struct{}{},
	}
	entries, err := parseLogOutput(strings.NewReader(output), repo{display: "repo"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].parsed.Type != "fix" || entries[0].parsed.Scope != "core" || !entries[0].parsed.Breaking || entries[0].parsed.Subject != "repair bug" {
		t.Fatalf("conventional entry = %#v", entries[0])
	}
	if entries[1].parsed.Type != "other" || entries[1].parsed.Subject != "plain message" {
		t.Fatalf("other entry = %#v", entries[1])
	}

	opts.scopes = map[string]struct{}{"core": {}}
	entries, err = parseLogOutput(strings.NewReader(output), repo{display: "repo"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].hash != "two22222" {
		t.Fatalf("scope-filtered entries = %#v", entries)
	}
}

func TestLogDefaultRefPrefersLocalDefaultBranch(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		switch strings.Join(args, " ") {
		case "symbolic-ref --quiet --short refs/remotes/origin/HEAD":
			return "origin/main\n", "", nil
		case "rev-parse --verify --quiet refs/heads/main":
			return "", "", nil
		default:
			return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
		}
	}}
	a := newApp(context.Background(), runner, strings.NewReader(""), io.Discard, io.Discard)
	ref, fallback := logDefaultRef(a, repo{dir: "repo"})
	if fallback {
		t.Fatal("unexpected fallback")
	}
	if ref != "refs/heads/main" {
		t.Fatalf("ref = %q, want local main", ref)
	}
}

func TestLogFallbackToHeadWarns(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one")
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "", "", errors.New("missing")
			case "log":
				if args[len(args)-1] != "HEAD" {
					t.Fatalf("log args = %v, want HEAD fallback", args)
				}
				return logRecord("abc123456", "2026-01-04T12:00:00Z", "feat: add thing"), "", nil
			default:
				return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
			}
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"log"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("log returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "origin/HEAD unavailable; using current HEAD") {
		t.Fatalf("missing fallback warning:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "feat") || !strings.Contains(stdout.String(), "add thing") {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
}

func TestLogMultiRepoSortsAndLimitsGlobally(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one", "two")
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "origin/main\n", "", nil
			case "rev-parse":
				return "", "", nil
			case "log":
				switch filepath.Base(dir) {
				case "one":
					return logRecord("old111111", "2026-01-01T12:00:00Z", "feat: older") +
						logRecord("new111111", "2026-01-03T12:00:00Z", "fix: newest"), "", nil
				case "two":
					return logRecord("mid222222", "2026-01-02T12:00:00Z", "docs: middle"), "", nil
				}
			}
			return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"log", "--limit", "2"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("log returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Repository") {
		t.Fatalf("multi-repo table missing repository column:\n%s", out)
	}
	if strings.Contains(out, "older") {
		t.Fatalf("global limit did not trim oldest entry:\n%s", out)
	}
	if strings.Index(out, "newest") > strings.Index(out, "middle") {
		t.Fatalf("entries are not date-desc sorted:\n%s", out)
	}
}

func TestLogLimitZeroShowsAllAndSingleRepoOmitsRepositoryColumn(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one")
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "origin/main\n", "", nil
			case "rev-parse":
				return "", "", nil
			case "log":
				return logRecord("one11111", "2026-01-01T12:00:00Z", "feat: one") +
					logRecord("two22222", "2026-01-02T12:00:00Z", "fix: two"), "", nil
			}
			return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"log", "--limit", "0"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("log returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if strings.Contains(firstLine(out), "Repository") {
		t.Fatalf("single-repo table should omit repository column:\n%s", out)
	}
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Fatalf("--limit 0 did not show all entries:\n%s", out)
	}
}

func TestLogFiltersTypeScopeAndDate(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one")
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "origin/main\n", "", nil
			case "rev-parse":
				return "", "", nil
			case "log":
				return logRecord("one11111", "2026-01-01T12:00:00Z", "feat(cli): before") +
					logRecord("two22222", "2026-01-02T12:00:00Z", "feat(api): wrong scope") +
					logRecord("three333", "2026-01-03T12:00:00Z", "feat(cli): match") +
					logRecord("four4444", "2026-01-04T12:00:00Z", "plain message"), "", nil
			}
			return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"log", "--type", "feat", "--scope", "cli", "--since", "2026-01-02", "--until", "2026-01-03"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("log returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "match") || strings.Contains(out, "before") || strings.Contains(out, "wrong scope") || strings.Contains(out, "plain message") {
		t.Fatalf("filters produced unexpected output:\n%s", out)
	}
}

func TestLogTypeOtherAndSummary(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one")
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "origin/main\n", "", nil
			case "rev-parse":
				return "", "", nil
			case "log":
				return logRecord("one11111", "2026-01-01T12:00:00Z", "feat(cli)!: add log") +
					logRecord("two22222", "2026-01-02T12:00:00Z", "plain message"), "", nil
			}
			return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"log", "--summary"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("log returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Summary: 2 commits, 1 repository, 0 failed, 1 breaking", "Types", "Top scopes", "feat   1  ", "other  1  ", "cli  1  "} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "feat   1") > strings.Index(out, "other  1") {
		t.Fatalf("other should sort after feat:\n%s", out)
	}
	if !strings.Contains(out, strings.Repeat("#", 30)) {
		t.Fatalf("summary missing full-width ascii bar:\n%s", out)
	}

	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithRunner(context.Background(), runner, []string{"log", "--type", "other"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("log --type other returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "plain message") || strings.Contains(stdout.String(), "add log") {
		t.Fatalf("--type other output:\n%s", stdout.String())
	}
}

func TestLogPerRepoFailureShowsSuccessesAndExitsNonzero(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "failed", "ok")
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch args[0] {
			case "symbolic-ref":
				return "origin/main\n", "", nil
			case "rev-parse":
				return "", "", nil
			case "log":
				if filepath.Base(dir) == "failed" {
					return "", "log failed", errors.New("failed")
				}
				return logRecord("abc123456", "2026-01-04T12:00:00Z", "feat: success"), "", nil
			}
			return "", "", fmt.Errorf("unexpected git args: %s", strings.Join(args, " "))
		},
	}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"log", "--summary"}, strings.NewReader(""), &stdout, &stderr)
	assertExitCode(t, err, 1)
	if !strings.Contains(stdout.String(), "success") || !strings.Contains(stdout.String(), "Summary: 1 commit, 2 repositories, 1 failed, 0 breaking") {
		t.Fatalf("stdout missing successful partial output:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "failed: could not scan log") || !strings.Contains(stderr.String(), "log failed") {
		t.Fatalf("stderr missing failure block:\n%s", stderr.String())
	}
}
