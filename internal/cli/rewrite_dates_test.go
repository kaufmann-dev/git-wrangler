package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

func TestWeekdayAndWeekend(t *testing.T) {
	// Thursday (Epoch 0)
	if wd := weekdayFromEpoch(0); wd != 4 {
		t.Errorf("expected 4 for epoch 0, got %d", wd)
	}
	if isWeekend(0) {
		t.Error("expected epoch 0 (Thursday) to not be a weekend")
	}

	// Saturday (1970-01-03: epoch 172800)
	if wd := weekdayFromEpoch(172800); wd != 6 {
		t.Errorf("expected 6 for epoch 172800, got %d", wd)
	}
	if !isWeekend(172800) {
		t.Error("expected epoch 172800 (Saturday) to be a weekend")
	}

	// Sunday (1970-01-04: epoch 259200)
	if wd := weekdayFromEpoch(259200); wd != 0 {
		t.Errorf("expected 0 for epoch 259200, got %d", wd)
	}
	if !isWeekend(259200) {
		t.Error("expected epoch 259200 (Sunday) to be a weekend")
	}

	// Monday (1970-01-05: epoch 345600)
	if wd := weekdayFromEpoch(345600); wd != 1 {
		t.Errorf("expected 1 for epoch 345600, got %d", wd)
	}
	if isWeekend(345600) {
		t.Error("expected epoch 345600 (Monday) to not be a weekend")
	}
}

func TestWriteDateCallbackUsesBytesLiterals(t *testing.T) {
	path, err := writeDateCallback(map[string]int64{"abc123": 1600000000}, "+0200")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `mapping[b'abc123'] = b'1600000000 +0200'`) {
		t.Fatalf("unexpected callback:\n%s", text)
	}
}

func TestFirstCommitEpochChecksMalformedOutput(t *testing.T) {
	restore := run.SetCommandFunc(func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) >= 1 && args[0] == "log" {
			return "not-a-timestamp\n", "", nil
		}
		return "", "", nil
	})
	defer restore()
	if _, err := firstCommitEpoch("repo", "--reverse"); err == nil {
		t.Fatal("expected malformed timestamp error")
	}
}

func TestDistributeCommitTimes(t *testing.T) {
	commits := []commitTime{
		{hash: "a", epoch: 100},
		{hash: "b", epoch: 200},
		{hash: "c", epoch: 300},
	}
	// Use realistic Unix epochs (September 2020)
	start := int64(1600000000)
	end := int64(1600864000) // 10 days later

	mapping := distributeCommitTimes(commits, start, end)
	if len(mapping) != 3 {
		t.Fatalf("expected mapping length 3, got %d", len(mapping))
	}

	timeA, okA := mapping["a"]
	timeB, okB := mapping["b"]
	timeC, okC := mapping["c"]

	if !okA || !okB || !okC {
		t.Fatal("missing mapped hashes in result")
	}

	// Given date snap, hour shifts, and potential weekend shifts,
	// timestamps should fall roughly within [start - 2 days, end + 2 days].
	margin := int64(2 * 86400)
	if timeA < start-margin || timeA > end+margin {
		t.Errorf("timeA %d out of bounds [%d, %d]", timeA, start-margin, end+margin)
	}
	if timeB < start-margin || timeB > end+margin {
		t.Errorf("timeB %d out of bounds [%d, %d]", timeB, start-margin, end+margin)
	}
	if timeC < start-margin || timeC > end+margin {
		t.Errorf("timeC %d out of bounds [%d, %d]", timeC, start-margin, end+margin)
	}

	// The distributed times should be strictly sorted chronologically (monotonically increasing)
	if timeA >= timeB || timeB >= timeC {
		t.Errorf("expected strictly sorted times (A < B < C), got: A=%d, B=%d, C=%d", timeA, timeB, timeC)
	}
}
