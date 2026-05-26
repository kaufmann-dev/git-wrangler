package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func TestPrintLargestFilesStreamsTopThree(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	runner := fakeRunner{pipe: func(ctx context.Context, dir string, env []string, left run.Command, right run.Command, consume func(io.Reader) error) error {
		if left.Name != "git" || strings.Join(left.Args, " ") != "rev-list --objects --all" {
			t.Fatalf("unexpected left command: %#v", left)
		}
		if right.Name != "git" || !strings.HasPrefix(strings.Join(right.Args, " "), "cat-file --batch-check") {
			t.Fatalf("unexpected right command: %#v", right)
		}
		return consume(strings.NewReader(strings.Join([]string{
			"100 hash-a small.txt",
			"900 hash-b large.bin",
			"300 hash-c medium.dat",
			"1200 hash-d largest.iso",
			"900 hash-e large.bin",
			"700 hash-f third.mov",
		}, "\n")))
	}}
	a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)

	if err := printLargestFiles(a, "repo"); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, want := range []string{"1.17 KB - largest.iso", "900 bytes - large.bin", "700 bytes - third.mov"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "small.txt") || strings.Count(out, "large.bin") != 1 {
		t.Fatalf("unexpected largest file output:\n%s", out)
	}
}
