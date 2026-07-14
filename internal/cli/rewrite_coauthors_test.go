package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteCoauthorsAddGuidedCollectsIdentity(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	runner := fakeRunner{
		lookPath: fakeGitAndFilterRepoLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			if name == "git" && strings.Join(args, " ") == "rev-parse HEAD" {
				return "", "", errors.New("unborn branch")
			}
			return "", "", errors.New("unexpected command: " + name + " " + strings.Join(args, " "))
		},
	}
	var stdout, stderr bytes.Buffer
	input := "\nGuided User <guided@example.test>\ny\n\n\n"
	err := executeInteractive(t, context.Background(), runner, []string{
		"rewrite-coauthors", "add", "--guided", "--yes",
	}, strings.NewReader(input), &stdout, &stderr)
	if err != nil {
		t.Fatalf("guided add failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	for _, want := range []string{"Selected configuration", "Guided User <guided@example.test>", "Skip origin fetch: true"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("guided output missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestRewriteCoauthorOptionValidation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: []string{"rewrite-coauthors", "add"}, want: "--coauthor is required"},
		{args: []string{"rewrite-coauthors", "add", "--coauthor", "No Email"}, want: "identity must be in Name <email> form"},
		{args: []string{"rewrite-coauthors", "replace", "--coauthor", "New <new@example.test>"}, want: "--email is required"},
		{args: []string{"rewrite-coauthors", "replace", "--email", "old@example.test"}, want: "--coauthor is required"},
		{args: []string{"rewrite-coauthors", "remove"}, want: "specify either --email or --all"},
		{args: []string{"rewrite-coauthors", "remove", "--all", "--email", "old@example.test"}, want: "specify either --email or --all"},
		{args: []string{"rewrite-coauthors", "remove", "--email", "same@example.test", "--email", "SAME@example.test"}, want: "duplicate email"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithRunner(context.Background(), fakeRunner{}, tc.args, strings.NewReader(""), &stdout, &stderr)
		if err == nil || !strings.Contains(stderr.String(), tc.want) {
			t.Fatalf("args %v: error = %v, stderr = %q, want %q", tc.args, err, stderr.String(), tc.want)
		}
	}
}

func TestRewriteCoauthorsHistoryAndSharedBaselineRollback(t *testing.T) {
	requireGitFilterRepoForTest(t)
	t.Setenv("NO_COLOR", "1")
	repoDir := filepath.Join(t.TempDir(), "repo")
	runGitForTest(t, "", "init", repoDir)
	runGitForTest(t, repoDir, "config", "user.name", "Test User")
	runGitForTest(t, repoDir, "config", "user.email", "test@example.test")
	commitMessageForTest(t, repoDir, "plain first\n\nSigned-off-by: Signer <signer@example.test>\nCo-authored-by: Old Name <old@example.test>", "2024-01-15T10:00:00 +0000")
	commitMessageForTest(t, repoDir, "fixup! plain first\n\nCo-authored-by: Old Name <old@example.test>", "2024-02-15T10:00:00 +0000")
	commitMessageForTest(t, repoDir, "plain second", "2024-03-15T10:00:00 +0000")
	original := commitMessagesForTest(t, repoDir)
	originalHead := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD"))

	var declinedStdout, declinedStderr bytes.Buffer
	err := executeInteractive(t, context.Background(), nil, []string{
		"rewrite-coauthors", "add", "--repo", repoDir, "--no-fetch",
		"--coauthor", "Declined Person <declined@example.test>",
	}, strings.NewReader("n\n"), &declinedStdout, &declinedStderr)
	if err != nil {
		t.Fatalf("declined rewrite returned error: %v\nstdout:\n%s\nstderr:\n%s", err, declinedStdout.String(), declinedStderr.String())
	}
	if head := strings.TrimSpace(runGitForTest(t, repoDir, "rev-parse", "HEAD")); head != originalHead {
		t.Fatalf("declined rewrite moved HEAD from %s to %s", originalHead, head)
	}
	if !strings.Contains(declinedStdout.String(), "1 skipped") {
		t.Fatalf("declined summary missing candidate skips:\n%s", declinedStdout.String())
	}

	stdout, stderr, err := runRealCLIForTest([]string{
		"rewrite-coauthors", "remove", "--repo", repoDir, "--no-fetch", "--yes",
		"--email", "absent@example.test",
	})
	if err != nil {
		t.Fatalf("no-candidate remove failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "0 commit messages rewritten, 0 repositories updated, 1 skipped, 0 failed") {
		t.Fatalf("no-candidate summary missing:\n%s", stdout)
	}

	stdout, stderr, err = runRealCLIForTest([]string{
		"rewrite-coauthors", "replace", "--repo", repoDir, "--no-fetch", "--yes",
		"--rewrite-before", "2024-02-01", "--email", "OLD@example.test",
		"--coauthor", "New Name <new@example.test>",
	})
	if err != nil {
		t.Fatalf("replace failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Summary: 1 commit messages rewritten, 1 repositories updated, 0 skipped, 0 failed") {
		t.Fatalf("replace summary missing:\n%s", stdout)
	}
	messages := commitMessagesForTest(t, repoDir)
	if !strings.Contains(messages[0], "Signed-off-by: Signer <signer@example.test>") || !strings.Contains(messages[0], "Co-authored-by: New Name <new@example.test>") {
		t.Fatalf("first message trailers were not replaced safely: %q", messages[0])
	}
	if !strings.Contains(messages[1], "Co-authored-by: Old Name <old@example.test>") {
		t.Fatalf("generated commit changed: %q", messages[1])
	}
	assertBaselineContainsMessageForTest(t, filepath.Join(repoDir, ".git"), "Old Name <old@example.test>")
	assertBaselineCurrentSHAsReachableForTest(t, repoDir)
	assertBaselineOmitsMessageForTest(t, filepath.Join(repoDir, ".git"), "New Name <new@example.test>")

	stdout, stderr, err = runRealCLIForTest([]string{
		"rewrite-coauthors", "add", "--repo", repoDir, "--no-fetch", "--yes",
		"--rewrite-after", "2024-02-01", "--coauthor", "Extra Person <extra@example.test>",
	})
	if err != nil {
		t.Fatalf("add failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	messages = commitMessagesForTest(t, repoDir)
	if strings.Contains(messages[1], "extra@example.test") || !strings.Contains(messages[2], "Co-authored-by: Extra Person <extra@example.test>") {
		t.Fatalf("generated exclusion/date bound failed: %#v", messages)
	}
	if !strings.Contains(stderr, "Skipped generated commits") || !strings.Contains(stderr, "Skipped generated commits   1") {
		t.Fatalf("preview missing generated skip count:\n%s", stderr)
	}

	stdout, stderr, err = runRealCLIForTest([]string{
		"rewrite-coauthors", "remove", "--repo", repoDir, "--no-fetch", "--yes", "--all",
	})
	if err != nil {
		t.Fatalf("remove failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	messages = commitMessagesForTest(t, repoDir)
	if strings.Contains(strings.ToLower(messages[0]), "co-authored-by") || strings.Contains(strings.ToLower(messages[2]), "co-authored-by") {
		t.Fatalf("remove --all left regular coauthors: %#v", messages)
	}
	if !strings.Contains(messages[1], "Co-authored-by: Old Name <old@example.test>") {
		t.Fatalf("remove --all changed generated commit: %q", messages[1])
	}
	assertBaselineContainsMessageForTest(t, filepath.Join(repoDir, ".git"), "Old Name <old@example.test>")
	assertBaselineCurrentSHAsReachableForTest(t, repoDir)
	assertBaselineOmitsMessageForTest(t, filepath.Join(repoDir, ".git"), "New Name <new@example.test>")

	stdout, stderr, err = runRealCLIForTest([]string{"rollback-rewrites", "--repo", repoDir, "--yes"})
	if err != nil {
		t.Fatalf("rollback failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	rolledBack := commitMessagesForTest(t, repoDir)
	if strings.Join(rolledBack, "\x00") != strings.Join(original, "\x00") {
		t.Fatalf("rollback messages = %#v, want %#v", rolledBack, original)
	}
}

func assertBaselineContainsMessageForTest(t *testing.T, gitDir, want string) {
	t.Helper()
	manifest, found, err := loadRewriteBaseline(gitDir)
	if err != nil || !found {
		t.Fatalf("load baseline: found=%v err=%v", found, err)
	}
	for _, entry := range manifest.Entries {
		if strings.Contains(entry.Message, want) {
			return
		}
	}
	messages := make([]string, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		messages = append(messages, entry.Message)
	}
	t.Fatalf("baseline messages = %#v, want one containing %q", messages, want)
}

func assertBaselineOmitsMessageForTest(t *testing.T, gitDir, unwanted string) {
	t.Helper()
	manifest, found, err := loadRewriteBaseline(gitDir)
	if err != nil || !found {
		t.Fatalf("load baseline: found=%v err=%v", found, err)
	}
	for _, entry := range manifest.Entries {
		if strings.Contains(entry.Message, unwanted) {
			t.Fatalf("baseline unexpectedly captured %q in entry %s; messages=%#v", unwanted, entry.FirstSHA, baselineMessagesForTest(manifest))
		}
	}
}

func baselineMessagesForTest(manifest rewriteBaselineManifest) []string {
	messages := make([]string, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		messages = append(messages, entry.Message)
	}
	return messages
}

func assertBaselineCurrentSHAsReachableForTest(t *testing.T, repoDir string) {
	t.Helper()
	manifest, found, err := loadRewriteBaseline(filepath.Join(repoDir, ".git"))
	if err != nil || !found {
		t.Fatalf("load baseline: found=%v err=%v", found, err)
	}
	reachable := map[string]bool{}
	for _, hash := range strings.Fields(runGitForTest(t, repoDir, "rev-list", "--branches")) {
		reachable[hash] = true
	}
	for _, entry := range manifest.Entries {
		if !reachable[entry.CurrentSHA] {
			t.Fatalf("baseline current SHA %s is not reachable; known=%v", entry.CurrentSHA, entry.KnownSHAs)
		}
	}
}

func TestCoauthorFilterArgsRestrictLocalBranches(t *testing.T) {
	got := coauthorFilterArgs([]dateBranchRef{{Name: "refs/heads/main"}, {Name: "refs/heads/topic"}}, "callback.py")
	want := []string{"--partial", "--force", "--refs", "refs/heads/main", "refs/heads/topic", "--commit-callback", "callback.py"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func runRealCLIForTest(args []string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), nil, args, strings.NewReader(""), &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func commitMessageForTest(t *testing.T, dir, message, date string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", message)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

func commitMessagesForTest(t *testing.T, dir string) []string {
	t.Helper()
	out := runGitForTest(t, dir, "log", "--reverse", "--format=%B%x1e")
	messages := []string{}
	for _, message := range strings.Split(out, "\x1e") {
		message = strings.TrimSpace(message)
		if message != "" {
			messages = append(messages, message)
		}
	}
	return messages
}
