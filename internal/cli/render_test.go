package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRenderSummaryPlain(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), nil, strings.NewReader(""), &stdout, &stderr)

	renderSummary(a,
		summaryCount{label: "updated", value: 2},
		summaryCount{label: "skipped", value: 3},
		summaryCount{label: "failed", value: 1},
	)

	if got, want := stdout.String(), "Summary: 2 updated, 3 skipped, 1 failed\n"; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestRenderStatusLinePlainIncludesStateText(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), nil, strings.NewReader(""), &stdout, &stderr)

	renderStatusLine(a, a.stdout, statusSkip, "repo-a", "already up to date")
	renderStatusLine(a, a.stderr, statusError, "repo-b", "git pull failed")

	if got, want := stdout.String(), "SKIP repo-a: already up to date\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "ERROR repo-b: git pull failed\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRenderTableUsesVisibleANSIWidth(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), nil, strings.NewReader(""), &stdout, &stderr)

	renderTable(a, []tableColumn{
		{header: "Repository"},
		{header: "State"},
		{header: "Tracking"},
	}, [][]string{
		{"short", a.ui.Green + "clean" + a.ui.Reset, "up to date"},
		{"long-name", a.ui.Yellow + "dirty" + a.ui.Reset, a.ui.Red + "behind 2" + a.ui.Reset},
	})

	plain := stripColor(stdout.String())
	if !strings.Contains(plain, "short       clean  up to date") {
		t.Fatalf("table did not align ANSI cells:\n%s", plain)
	}
	if strings.Contains(plain, "|") {
		t.Fatalf("table should not use hard separators:\n%s", plain)
	}
}
