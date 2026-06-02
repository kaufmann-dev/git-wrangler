package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/ai"
)

func TestProgressWritesPlainLinesForNonTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)

	progress := newProgress(a, "Testing progress", 2)
	progress.advance("repo-a")
	progress.advance("repo-b")
	progress.done()

	out := stderr.String()
	for _, want := range []string{
		"Testing progress: 1/2 repo-a",
		"Testing progress: 2/2 repo-b",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestProgressThrottlesPlainLinesForNonTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)

	progress := newProgress(a, "Testing progress", 25)
	for i := 0; i < 25; i++ {
		progress.advance("repo")
	}
	progress.done()

	out := stderr.String()
	for _, want := range []string{
		"Testing progress: 1/25 repo",
		"Testing progress: 10/25 repo",
		"Testing progress: 20/25 repo",
		"Testing progress: 25/25 repo",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Testing progress: 2/25") {
		t.Fatalf("progress was not throttled:\n%s", out)
	}
}

func TestProgressLogWritesStandaloneLineForNonTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)

	progress := newProgress(a, "Testing progress", 3)
	progress.advance("")
	progress.log("Retrying 1 commit(s) after failed batch attempt 1: missing or invalid message.")
	progress.advance("")
	progress.done()

	out := stderr.String()
	if !strings.Contains(out, "\nRetrying 1 commit(s) after failed batch attempt 1: missing or invalid message.\n") {
		t.Fatalf("retry log was not standalone:\n%s", out)
	}
	if strings.Contains(out, "Testing progress: 1/3 Retrying") {
		t.Fatalf("retry log was mixed into progress line:\n%s", out)
	}
}

func TestProgressLogClearsAndRedrawsInteractiveLine(t *testing.T) {
	var stderr bytes.Buffer
	progress := &progress{
		writer:      &stderr,
		interactive: true,
		label:       "Testing progress",
		total:       3,
	}

	progress.advance("")
	progress.log("Retrying 1 commit(s) after failed batch attempt 1: missing or invalid message.")

	out := stderr.String()
	if !strings.Contains(out, "\n\rTesting progress: [######--------------] 1/3 ") {
		t.Fatalf("progress line was not redrawn after log:\n%q", out)
	}
	if strings.Contains(out, "1/3 Retrying") {
		t.Fatalf("retry log was mixed into progress line:\n%q", out)
	}
}

func TestAIRequestProgressWritesInlineColoredRetryDetail(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)

	apiProgress := (*progress)(nil)
	updateAIRequestProgress(a, &apiProgress, ai.ProgressEvent{
		Phase:   "Sending API requests",
		Current: 0,
		Total:   2,
		Detail:  "Retrying 1 commit(s) after failed batch attempt 1: missing or invalid message.",
		Error:   true,
	})
	updateAIRequestProgress(a, &apiProgress, ai.ProgressEvent{
		Phase:   "Sending API requests",
		Current: 1,
		Total:   2,
		Detail:  "batch 1 completed",
	})
	apiProgress.done()

	out := stderr.String()
	if !strings.Contains(out, "Sending API requests: 0/2 \033[31mRetrying 1 commit(s)") {
		t.Fatalf("retry detail was not inline and red:\n%q", out)
	}
	if !strings.Contains(out, "Sending API requests: 1/2 batch 1 completed") {
		t.Fatalf("completion detail was not inline:\n%q", out)
	}
}
