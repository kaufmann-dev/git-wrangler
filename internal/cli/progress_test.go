package cli

import (
	"bytes"
	"context"
	"io"
	"os"
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

func TestProgressStartShowsActiveWorkAtZero(t *testing.T) {
	var stderr bytes.Buffer
	progress := &progress{
		writer:      &stderr,
		interactive: true,
		label:       "Testing progress",
		total:       2,
	}

	progress.start("repo-a")

	want := "Testing progress: [--------------------] 0/2 repo-a"
	if stderr.String() != want {
		t.Fatalf("progress start output = %q, want %q", stderr.String(), want)
	}
	if progress.current != 0 {
		t.Fatalf("current = %d, want 0", progress.current)
	}
}

func TestProgressStartWorkUsesStableKeyAndDisplayDetail(t *testing.T) {
	var stderr bytes.Buffer
	progress := &progress{
		writer:      &stderr,
		interactive: true,
		label:       "Testing progress",
		total:       2,
	}

	progress.startWork("repo-a", "repo-a 100/200 commits")
	progress.update("repo-a", "repo-a 150/200 commits")
	progress.finish("repo-a", "repo-a")

	out := stderr.String()
	for _, want := range []string{"0/2 repo-a 100/200 commits", "0/2 repo-a 150/200 commits", "1/2 "} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in progress output:\n%q", want, out)
		}
	}
}

func TestProgressDoesNotShowStaleCompletedDetail(t *testing.T) {
	var stderr bytes.Buffer
	progress := &progress{
		writer:      &stderr,
		interactive: true,
		label:       "Testing progress",
		total:       2,
	}

	progress.start("repo-a")
	progress.finish("repo-a", "repo-a")
	progress.start("repo-b")
	progress.finish("repo-b", "repo-b")

	out := stderr.String()
	last := out[strings.LastIndex(out, "Testing progress:"):]
	if strings.Contains(last, "repo-a") || strings.Contains(last, "repo-b") {
		t.Fatalf("final progress line included stale detail:\n%q", out)
	}
	for _, want := range []string{"0/2 repo-a", "1/2 repo-b", "2/2 "} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in progress output:\n%q", want, out)
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
	want := "Testing progress: [######--------------] 1/3 \r\x1b[JRetrying 1 commit(s) after failed batch attempt 1: missing or invalid message.\nTesting progress: [######--------------] 1/3 "
	if out != want {
		t.Fatalf("unexpected output:\ngot:  %q\nwant: %q", out, want)
	}
}

func TestAIRequestProgressWritesInlineColoredRetryDetail(t *testing.T) {
	t.Setenv("TERM", "xterm")
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
	if !strings.Contains(out, "\033[31mRetrying 1 commit(s) after failed batch attempt 1: missing or invalid message.\033[0m\n") {
		t.Fatalf("retry detail was not standalone and red:\n%q", out)
	}
	if !strings.Contains(out, "Sending API requests: 1/2 batch 1 completed") {
		t.Fatalf("completion detail was not inline:\n%q", out)
	}
}

func TestAIRequestProgressLogsSingleBatchRetry(t *testing.T) {
	t.Setenv("TERM", "xterm")
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader(""), &stdout, &stderr)

	apiProgress := (*progress)(nil)
	updateAIRequestProgress(a, &apiProgress, ai.ProgressEvent{
		Phase:   "Sending API requests",
		Current: 0,
		Total:   1,
		Detail:  "Retrying 1 commit(s) after failed batch attempt 1: missing or invalid message.",
		Error:   true,
	})

	out := stderr.String()
	if !strings.Contains(out, "\033[31mRetrying 1 commit(s) after failed batch attempt 1: missing or invalid message.\033[0m\n") {
		t.Fatalf("single-batch retry detail was not logged:\n%q", out)
	}
	if apiProgress != nil {
		t.Fatal("single-batch retry should not create a progress bar")
	}
}

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"café", 4},
		{"привет", 6},
		{"\x1b[31mred\x1b[0m", 3},
		{"\x1b[31mcafé\x1b[0m", 4},
		{"\x1b[1m\x1b[34mbold blue\x1b[0m", 9},
		{"", 0},
	}
	for _, tc := range tests {
		got := visibleWidth(tc.input)
		if got != tc.want {
			t.Errorf("visibleWidth(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestTruncateToVisibleWidth(t *testing.T) {
	tests := []struct {
		input     string
		maxW      int
		resetCode string
		want      string
	}{
		{"hello", 10, "\x1b[0m", "hello"},
		{"hello", 3, "\x1b[0m", "hel"},
		{"café", 3, "\x1b[0m", "caf"},
		{"привет", 3, "\x1b[0m", "при"},
		{"\x1b[31mred\x1b[0m", 2, "\x1b[0m", "\x1b[31mre\x1b[0m"},
		{"\x1b[31mcafé\x1b[0m", 3, "\x1b[0m", "\x1b[31mcaf\x1b[0m"},
		{"\x1b[31mred\x1b[0m", 5, "\x1b[0m", "\x1b[31mred\x1b[0m"},
		{"\x1b[31mred\x1b[0m", 0, "\x1b[0m", ""},
	}
	for _, tc := range tests {
		got := truncateToVisibleWidth(tc.input, tc.maxW, tc.resetCode)
		if got != tc.want {
			t.Errorf("truncateToVisibleWidth(%q, %d) = %q, want %q", tc.input, tc.maxW, got, tc.want)
		}
	}
}

func TestProgressTruncatesLongInteractiveDetails(t *testing.T) {
	oldTermGetSize := termGetSize
	defer func() { termGetSize = oldTermGetSize }()

	termGetSize = func(fd int) (int, int, error) {
		return 50, 0, nil
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	progress := &progress{
		writer:      w,
		interactive: true,
		label:       "Test",
		total:       10,
		current:     5,
	}

	progress.write("123456789012345678901234567890")
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	expected := "Test: [##########----------] 5/10 123456789012345"
	if out != expected {
		t.Errorf("expected output %q, got %q", expected, out)
	}
}

func TestProgressClearHandlesMultipleRows(t *testing.T) {
	oldTermGetSize := termGetSize
	defer func() { termGetSize = oldTermGetSize }()

	termGetSize = func(fd int) (int, int, error) {
		return 20, 0, nil
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	progress := &progress{
		writer:      w,
		interactive: true,
		label:       "Test",
		total:       10,
		current:     5,
		lastWidth:   45,
		lastRows:    3,
	}

	progress.clear()
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	expected := "\x1b[2A\r\x1b[J"
	if out != expected {
		t.Errorf("expected clear output %q, got %q", expected, out)
	}
}
