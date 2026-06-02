package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
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
