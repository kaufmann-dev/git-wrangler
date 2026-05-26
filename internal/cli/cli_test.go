package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

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
		"History Rewriting:",
		"Utility:",
		"completion",
		"version",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "update") || strings.Contains(out, "uninstall") {
		t.Fatalf("removed commands appeared in help:\n%s", out)
	}
	if strings.Contains(out, "\n  doctor") || strings.Contains(out, "\n  help") {
		t.Fatalf("removed commands appeared in help:\n%s", out)
	}
}

func TestHelpCommandIsRemoved(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	err := ExecuteWithIO([]string{"help"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected help command to return an error")
	}
	if !strings.Contains(stderr.String(), `unknown command "help"`) {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
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
	var stdout, stderr bytes.Buffer
	err := ExecuteWithIO([]string{"rewrite-commits-ai", "--batch-size", "51"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected validation error")
	}
	var exit exitError
	if !errors.As(err, &exit) || exit.code != 1 {
		t.Fatalf("expected exitError(1), got %T %v", err, err)
	}
	if !strings.Contains(stderr.String(), "--batch-size must be 50 or less") {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}

func TestConfirmUsesInjectedStreams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	a := newApp(strings.NewReader("y\n"), &stdout, &stderr)
	if !confirm(a, "Proceed?") {
		t.Fatal("expected yes confirmation")
	}
	if !strings.Contains(stdout.String(), "Proceed? [y/N]") {
		t.Fatalf("prompt not written to injected stdout: %q", stdout.String())
	}
}
