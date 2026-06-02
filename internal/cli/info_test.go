package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
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

func TestInfoRunsConcurrentlyAndPreservesOutputOrder(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "a-slow", "b-fast")
	t.Chdir(root)

	var mu sync.Mutex
	activeStatus := 0
	maxActiveStatus := 0
	release := make(chan struct{})
	var releaseOnce sync.Once
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name != "git" {
				return "", "", errors.New("unexpected command")
			}
			joined := strings.Join(args, " ")
			switch joined {
			case "status --porcelain":
				done := trackConcurrentStart(&mu, &activeStatus, &maxActiveStatus, release, &releaseOnce)
				defer done()
				return "", "", nil
			case "rev-parse --abbrev-ref HEAD":
				return "main\n", "", nil
			case "rev-parse HEAD":
				return "head\n", "", nil
			case "rev-list --left-right --count HEAD...@{u}":
				return "1 2\n", "", nil
			case "branch -a --no-color":
				return "* main\n", "", nil
			case "remote -v":
				return "", "", nil
			case "log --reverse --format=%ci - %s":
				return "2024-01-01 00:00:00 +0000 - first\n", "", nil
			case "rev-list --all --count":
				return "1\n", "", nil
			case "log --since=1 month ago --format=%ci":
				return "", "", nil
			case "log -1 --format=%ci - %s":
				return "2024-01-02 00:00:00 +0000 - last\n", "", nil
			case "log --format=%an <%ae>":
				return "A <a@example.test>\n", "", nil
			default:
				return "", "", errors.New("unexpected git args: " + joined)
			}
		},
		pipe: func(ctx context.Context, dir string, env []string, left run.Command, right run.Command, consume func(io.Reader) error) error {
			return consume(strings.NewReader("100 hash file.txt\n"))
		},
	}

	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), runner, []string{"info"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("info returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if maxActiveStatus < 2 {
		t.Fatalf("info did not run concurrently; max active status = %d", maxActiveStatus)
	}
	out := stdout.String()
	first := strings.Index(out, "Repository:         a-slow")
	second := strings.Index(out, "Repository:         b-fast")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("info output not in repo order:\n%s", out)
	}
}
