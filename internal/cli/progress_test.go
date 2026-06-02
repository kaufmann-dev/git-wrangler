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
