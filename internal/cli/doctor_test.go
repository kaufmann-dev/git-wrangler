package cli

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDoctorReportsInstalledDependencies(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch {
			case name == "/usr/bin/git" && joined(args) == "--version":
				return "git version 2.50.0\n", "", nil
			case name == "/usr/bin/gh" && joined(args) == "--version":
				return "gh version 2.70.0\n", "", nil
			case name == "/usr/bin/git-filter-repo" && joined(args) == "--version":
				return "git-filter-repo 2.47.0\n", "", nil
			default:
				return "", "", errors.New("unexpected command")
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"doctor"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("doctor returned error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"Git Wrangler Doctor",
		"Runtime",
		"Version     git-wrangler dev",
		"Platform",
		"Executable",
		"git              OK",
		"/usr/bin/git (git version 2.50.0)",
		"gh               OK",
		"git-filter-repo  OK",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Source installs do not include runtime dependencies") {
		t.Fatalf("doctor printed missing dependency note with all dependencies present:\n%s", out)
	}
	if stderr.String() != "" {
		t.Fatalf("doctor wrote stderr: %q", stderr.String())
	}
}

func TestDoctorFailsWhenGitIsMissing(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" {
				return "", exec.ErrNotFound
			}
			return "/usr/bin/" + name, nil
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name == "/usr/bin/gh" && joined(args) == "--version" {
				return "gh version 2.70.0\n", "", nil
			}
			if name == "/usr/bin/git-filter-repo" && joined(args) == "--version" {
				return "git-filter-repo 2.47.0\n", "", nil
			}
			return "", "", errors.New("unexpected command")
		},
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"doctor"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected doctor to fail when git is missing")
	}
	var exit exitError
	if !errors.As(err, &exit) || exit.code != 1 {
		t.Fatalf("expected exitError(1), got %T %v", err, err)
	}
	out := stdout.String()
	for _, want := range []string{
		"git              ERROR",
		"not found; needed for most Git Wrangler commands",
		"Source installs do not include runtime dependencies",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorWarnsForMissingOptionalDependencies(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			if name == "git" {
				return "/usr/bin/git", nil
			}
			return "", exec.ErrNotFound
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name == "/usr/bin/git" && joined(args) == "--version" {
				return "git version 2.50.0\n", "", nil
			}
			if name == "git" && joined(args) == "filter-repo --version" {
				return "", "", errors.New("missing filter-repo")
			}
			return "", "", errors.New("unexpected command")
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"doctor"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("doctor returned error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"gh               WARN",
		"not found; needed for clone and rename-repo",
		"git-filter-repo  WARN",
		"not found; needed for history rewrite commands",
		"Source installs do not include runtime dependencies",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorReportsGitFilterRepoFallback(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	runner := fakeRunner{
		lookPath: func(name string) (string, error) {
			switch name {
			case "git":
				return "/usr/bin/git", nil
			case "gh":
				return "/usr/bin/gh", nil
			default:
				return "", exec.ErrNotFound
			}
		},
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			switch {
			case name == "/usr/bin/git" && joined(args) == "--version":
				return "git version 2.50.0\n", "", nil
			case name == "/usr/bin/gh" && joined(args) == "--version":
				return "gh version 2.70.0\n", "", nil
			case name == "git" && joined(args) == "filter-repo --version":
				return "git-filter-repo 2.47.0\n", "", nil
			default:
				return "", "", errors.New("unexpected command")
			}
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"doctor"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("doctor returned error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "git-filter-repo  OK     git filter-repo (git-filter-repo 2.47.0)") {
		t.Fatalf("doctor did not report git filter-repo fallback:\n%s", out)
	}
}

func joined(args []string) string {
	return strings.Join(args, " ")
}
