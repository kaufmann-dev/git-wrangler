package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRootCommandShowsLanding(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithIO([]string{}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("root command returned error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"____ _ _",
		"Orchestrate Git operations across many repositories.",
		"Common commands:",
		"git-wrangler status",
		"git-wrangler help",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("landing missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Remote Operations:") || strings.Contains(out, "History Rewriting:") {
		t.Fatalf("landing should not include Cobra command groups:\n%s", out)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRootHelpUsesCobraGroups(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithIO([]string{"--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("--help returned error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"Remote Operations:",
		"Local Operations:",
		"AI Commands:",
		"History Rewriting:",
		"Utility:",
		"commit-ai",
		"rewrite-commits-ai",
		"completion",
		"doctor",
		"version",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "update") || strings.Contains(out, "uninstall") {
		t.Fatalf("removed commands appeared in help:\n%s", out)
	}
	if strings.Index(out, "AI Commands:") < strings.Index(out, "Local Operations:") || strings.Index(out, "AI Commands:") > strings.Index(out, "History Rewriting:") {
		t.Fatalf("AI Commands group is not in the expected order:\n%s", out)
	}
}

func TestHelpCommandUsesCobraGeneratedHelp(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithIO([]string{"help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("help returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Help about any command") || !strings.Contains(stdout.String(), "Utility:") {
		t.Fatalf("root help missing generated help command:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := ExecuteWithIO([]string{"help", "status"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("help status returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Show clean, dirty, and tracking state.") {
		t.Fatalf("command help missing status text:\n%s", stdout.String())
	}
}

func TestVersionCommand(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithIO([]string{"version"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("version returned error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"git-wrangler dev", "commit: unknown", "built: unknown"} {
		if !strings.Contains(out, want) {
			t.Fatalf("version missing %q:\n%s", want, out)
		}
	}
}

func TestCommandsRejectPositionalArgs(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, args := range [][]string{
		{"version", "extra"},
		{"doctor", "extra"},
		{"status", "extra"},
		{"commit", "extra", "--message", "test"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithIO(args, strings.NewReader(""), &stdout, &stderr)
		if err == nil {
			t.Fatalf("%v returned nil error", args)
		}
		if !strings.Contains(stderr.String(), "accepts 0 arg(s)") && !strings.Contains(stderr.String(), "unknown command") {
			t.Fatalf("%v stderr = %q", args, stderr.String())
		}
	}
}

func TestCompletionCommandIsPresent(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithIO([]string{"completion", "--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("completion help returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Available Commands:") || !strings.Contains(stdout.String(), "bash") {
		t.Fatalf("completion help missing shells:\n%s", stdout.String())
	}
}

func TestRewriteCommitsAIFlagValidation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"rewrite-commits-ai", "--batch-size", "51"}, "--batch-size must be 50 or less"},
		{[]string{"rewrite-commits-ai", "--requests-per-minute", "0"}, "--requests-per-minute must be a positive integer"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithIO(tc.args, strings.NewReader(""), &stdout, &stderr)
		if err == nil {
			t.Fatalf("%v returned nil error", tc.args)
		}
		var exit exitError
		if !errors.As(err, &exit) || exit.code != 1 {
			t.Fatalf("expected exitError(1), got %T %v", err, err)
		}
		if !strings.Contains(stderr.String(), tc.want) {
			t.Fatalf("%v stderr:\n%s", tc.args, stderr.String())
		}
	}
}

func TestCommitAIFlagValidation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"commit-ai", "--max-chars-per-commit", "0"}, "--max-chars-per-commit must be a positive integer"},
		{[]string{"commit-ai", "--requests-per-minute", "0"}, "--requests-per-minute must be a positive integer"},
		{[]string{"commit-ai", "--timeout", "0"}, "--timeout must be a positive integer"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithIO(tc.args, strings.NewReader(""), &stdout, &stderr)
		if err == nil {
			t.Fatalf("%v returned nil error", tc.args)
		}
		if !strings.Contains(stderr.String(), tc.want) {
			t.Fatalf("%v stderr:\n%s", tc.args, stderr.String())
		}
	}
}

func TestConfirmUsesInjectedStreams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader("y\n"), &stdout, &stderr)
	if !confirm(a, "Proceed?") {
		t.Fatal("expected yes confirmation")
	}
	if stdout.String() != "" {
		t.Fatalf("prompt should not be written to stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Proceed? [y/N]") {
		t.Fatalf("prompt not written to injected stderr: %q", stderr.String())
	}
}

func TestRequiredFlagFailsNonInteractive(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), fakeRunner{}, []string{"commit"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected missing flag failure")
	}
	if !strings.Contains(stderr.String(), "--message is required") {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}

func TestRewriteCommitsAIMissingConfigDoesNotPrompt(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stderr bytes.Buffer
	cmd := newRootCommand(newApp(context.Background(), fakeRunner{}, strings.NewReader("ignored\n"), io.Discard, &stderr))
	cmd.SetArgs([]string{"rewrite-commits-ai", "--yes"})
	cmd.SetIn(strings.NewReader("ignored\n"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing flag failure")
	}
	if strings.Contains(stderr.String(), "OpenAI-compatible API base URL:") {
		t.Fatalf("rewrite-commits-ai should not prompt for removed flags:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "AI model is required") {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}
