package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestRootCommandShowsLanding(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), nil, []string{}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("root command returned error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"██████╗ ██╗████████╗",
		"Orchestrate Git operations across many repositories.",
		"Common commands:",
		"git-wrangler status",
		"git-wrangler help",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("landing missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Remote Operations:") || strings.Contains(out, "History Rewriting:") {
		t.Fatalf("landing should not include Cobra command groups:\n%s", out)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRootCommandUsesGradientWhenColorEnabled(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CLICOLOR_FORCE", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), nil, []string{}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("root command returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "\033[38;2;0;245;255m") {
		t.Fatalf("landing missing banner gradient:\n%q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRootHelpUsesCobraGroups(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), nil, []string{"--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("--help returned error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"Remote Operations:",
		"Local Operations:",
		"History Rewriting:",
		"Utility:",
		"fetch",
		"commit",
		"rewrite-commits",
		"rewrite-coauthors",
		"completion",
		"activity",
		"log",
		"doctor",
		"version",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
	for _, removed := range []string{"AI Commands:", "commit-ai", "rewrite-commits-ai"} {
		if strings.Contains(out, removed) {
			t.Fatalf("removed help entry %q appeared in help:\n%s", removed, out)
		}
	}
	for _, removed := range []string{"update", "uninstall"} {
		if strings.Contains(out, "\n  "+removed+" ") {
			t.Fatalf("removed command %q appeared in help:\n%s", removed, out)
		}
	}
}

func TestCommandSurfaceMatchesExpected(t *testing.T) {
	root := newRootCommand(newApp(context.Background(), fakeRunner{}, strings.NewReader(""), io.Discard, io.Discard))
	for _, tc := range []struct {
		name  string
		group string
		flags []string
	}{
		{name: "activity", group: "utility", flags: strings.Fields("all global-scale guided repo user year")},
		{name: "clone", group: "remote", flags: strings.Fields("guided into limit user visibility")},
		{name: "commit", group: "local", flags: strings.Fields("body coauthor concurrency guided repo rpm timeout yes")},
		{name: "config", group: "utility"},
		{name: "doctor", group: "utility", flags: strings.Fields("json")},
		{name: "fetch", group: "remote", flags: strings.Fields("guided prune repo")},
		{name: "fix-gitignore", group: "local", flags: strings.Fields("guided repo yes")},
		{name: "info", group: "utility", flags: strings.Fields("guided json no-fetch repo")},
		{name: "init", group: "utility"},
		{name: "license", group: "local", flags: strings.Fields("guided name overwrite repo type year yes")},
		{name: "log", group: "utility", flags: strings.Fields("guided limit repo scope since summary type until")},
		{name: "pull", group: "remote", flags: strings.Fields("guided rebase repo")},
		{name: "push", group: "remote", flags: strings.Fields("force force-unsafe guided repo yes")},
		{name: "remove-secrets", group: "history", flags: strings.Fields("guided no-fetch repo yes")},
		{name: "rename-branch", group: "local", flags: strings.Fields("guided newbranch oldbranch repo yes")},
		{name: "rename-repo", group: "remote", flags: strings.Fields("description repo")},
		{name: "reset", group: "local", flags: strings.Fields("guided repo yes")},
		{name: "review", group: "local", flags: strings.Fields("guided json no-fetch repo")},
		{name: "rewrite-authors", group: "history", flags: strings.Fields("email guided name no-fetch repo rewrite-after rewrite-before yes")},
		{name: "rewrite-coauthors", group: "history"},
		{name: "rewrite-commits", group: "history", flags: strings.Fields("batch-size body concurrency guided no-fetch remove-coauthors repo require-scope rewrite-after rewrite-before rpm skip-conventional timeout yes")},
		{name: "rewrite-dates", group: "history", flags: strings.Fields("days end-date frequency guided no-fetch repo rewrite-after rewrite-before seed spread start-date until window yes")},
		{name: "rewrite-hours", group: "history", flags: strings.Fields("guided no-fetch repo rewrite-after rewrite-before window yes")},
		{name: "rollback-rewrites", group: "history", flags: strings.Fields("guided repo yes")},
		{name: "status", group: "utility", flags: strings.Fields("guided json no-fetch repo")},
		{name: "untrack", group: "local", flags: strings.Fields("guided repo yes")},
		{name: "version", group: "utility", flags: strings.Fields("json")},
	} {
		cmd := commandByName(t, root, tc.name)
		if cmd.GroupID != tc.group {
			t.Fatalf("%s group = %q, want %q", tc.name, cmd.GroupID, tc.group)
		}
		got := localFlagNames(cmd)
		sort.Strings(tc.flags)
		if strings.Join(got, " ") != strings.Join(tc.flags, " ") {
			t.Fatalf("%s flags = %q, want %q", tc.name, got, tc.flags)
		}
	}
}

func TestRootAndConfigCommandsComeFromSpecs(t *testing.T) {
	root := newRootCommand(newApp(context.Background(), fakeRunner{}, strings.NewReader(""), io.Discard, io.Discard))
	wantTop := map[string]commandSpec{}
	for _, spec := range commandSpecs() {
		wantTop[commandUseName(spec.use)] = spec
	}
	gotTop := map[string]bool{}
	for _, cmd := range root.Commands() {
		if cmd.Name() == "completion" || cmd.Name() == "help" {
			continue
		}
		gotTop[cmd.Name()] = true
		if _, ok := wantTop[cmd.Name()]; !ok {
			t.Fatalf("root command %q is not in commandSpecs", cmd.Name())
		}
	}
	for name := range wantTop {
		if !gotTop[name] {
			t.Fatalf("commandSpecs command %q was not built on the root command", name)
		}
	}

	configSpec := wantTop["config"]
	configCmd := commandByName(t, root, "config")
	wantChildren := map[string]bool{}
	for _, child := range configSpec.children {
		wantChildren[commandUseName(child.use)] = true
	}
	gotChildren := map[string]bool{}
	for _, child := range configCmd.Commands() {
		gotChildren[child.Name()] = true
		if !wantChildren[child.Name()] {
			t.Fatalf("config subcommand %q is not in commandSpecs", child.Name())
		}
	}
	for name := range wantChildren {
		if !gotChildren[name] {
			t.Fatalf("commandSpecs config subcommand %q was not built", name)
		}
	}
}

func TestRewriteCoauthorSubcommandSurface(t *testing.T) {
	root := newRootCommand(newApp(context.Background(), fakeRunner{}, strings.NewReader(""), io.Discard, io.Discard))
	parent := commandByName(t, root, "rewrite-coauthors")
	for _, tc := range []struct {
		name  string
		flags string
	}{
		{name: "add", flags: "coauthor guided no-fetch repo rewrite-after rewrite-before yes"},
		{name: "replace", flags: "coauthor email guided no-fetch repo rewrite-after rewrite-before yes"},
		{name: "remove", flags: "all email guided no-fetch repo rewrite-after rewrite-before yes"},
	} {
		cmd := commandByName(t, parent, tc.name)
		if got := strings.Join(localFlagNames(cmd), " "); got != tc.flags {
			t.Fatalf("rewrite-coauthors %s flags = %q, want %q", tc.name, got, tc.flags)
		}
		if flag := cmd.Flags().ShorthandLookup("y"); flag == nil || flag.Name != "yes" {
			t.Fatalf("rewrite-coauthors %s does not expose -y", tc.name)
		}
	}
}

func TestConfigSubcommandSurfaceMatchesExpected(t *testing.T) {
	root := newRootCommand(newApp(context.Background(), fakeRunner{}, strings.NewReader(""), io.Discard, io.Discard))
	config := commandByName(t, root, "config")
	for _, tc := range []struct {
		name  string
		flags []string
	}{
		{name: "show", flags: strings.Fields("json")},
		{name: "set"},
		{name: "unset"},
	} {
		cmd := childCommandByName(t, config, tc.name)
		got := localFlagNames(cmd)
		sort.Strings(tc.flags)
		if strings.Join(got, " ") != strings.Join(tc.flags, " ") {
			t.Fatalf("config %s flags = %q, want %q", tc.name, got, tc.flags)
		}
	}
}

func TestLicenseRemoveSubcommandSurfaceMatchesExpected(t *testing.T) {
	root := newRootCommand(newApp(context.Background(), fakeRunner{}, strings.NewReader(""), io.Discard, io.Discard))
	license := commandByName(t, root, "license")
	remove := childCommandByName(t, license, "remove")
	got := localFlagNames(remove)
	want := strings.Fields("guided repo yes")
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("license remove flags = %q, want %q", got, want)
	}
	if flag := remove.Flags().ShorthandLookup("y"); flag == nil || flag.Name != "yes" {
		t.Fatal("license remove does not expose -y for --yes")
	}
}

func commandUseName(use string) string {
	return strings.Fields(use)[0]
}

func localFlagNames(cmd interface{ Flags() *pflag.FlagSet }) []string {
	names := []string{}
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		names = append(names, flag.Name)
	})
	sort.Strings(names)
	return names
}

func childCommandByName(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, cmd := range parent.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	t.Fatalf("command %q not found below %q", name, parent.Name())
	return nil
}

func TestRollbackRewritesCommandSurface(t *testing.T) {
	root := newRootCommand(newApp(context.Background(), fakeRunner{}, strings.NewReader(""), io.Discard, io.Discard))
	cmd := commandByName(t, root, "rollback-rewrites")
	if cmd.GroupID != "history" {
		t.Fatalf("group = %q, want history", cmd.GroupID)
	}
	for _, flag := range []string{"repo", "yes", "guided"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("rollback-rewrites missing --%s", flag)
		}
	}
	if cmd.Flags().Lookup("no-fetch") != nil {
		t.Fatal("rollback-rewrites should not expose --no-fetch")
	}
}

func TestRemovedAICommandNamesAreUnknown(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, args := range [][]string{
		{"commit-ai"},
		{"rewrite-commits-ai"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithRunner(context.Background(), nil, args, strings.NewReader(""), &stdout, &stderr)
		if err == nil {
			t.Fatalf("%v returned nil error", args)
		}
		if !strings.Contains(stderr.String(), "unknown command") {
			t.Fatalf("%v stderr = %q", args, stderr.String())
		}
	}
}

func TestRemovedCommandsDoNotAppearInHelp(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), nil, []string{"--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("--help returned error: %v", err)
	}
	out := stdout.String()
	if strings.Contains(out, "\n  update ") || strings.Contains(out, "\n  uninstall ") {
		t.Fatalf("removed commands appeared in help:\n%s", out)
	}
}

func TestHelpCommandUsesCobraGeneratedHelp(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), nil, []string{"help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("help returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Help about any command") || !strings.Contains(stdout.String(), "Utility:") {
		t.Fatalf("root help missing generated help command:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := ExecuteWithRunner(context.Background(), nil, []string{"help", "status"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("help status returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Show clean, dirty, and tracking state.") {
		t.Fatalf("command help missing status text:\n%s", stdout.String())
	}
}

func TestVersionCommand(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), nil, []string{"version"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("version returned error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"git-wrangler dev", "commit: unknown", "built: unknown"} {
		if !strings.Contains(out, want) {
			t.Fatalf("version missing %q:\n%s", want, out)
		}
	}
}

func TestCommandsRejectPositionalArgs(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, args := range [][]string{
		{"version", "extra"},
		{"doctor", "extra"},
		{"status", "extra"},
		{"commit", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithRunner(context.Background(), nil, args, strings.NewReader(""), &stdout, &stderr)
		if err == nil {
			t.Fatalf("%v returned nil error", args)
		}
		if !strings.Contains(stderr.String(), "accepts 0 arg(s)") && !strings.Contains(stderr.String(), "unknown command") {
			t.Fatalf("%v stderr = %q", args, stderr.String())
		}
	}
}

func TestCompletionCommandIsPresent(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	if err := ExecuteWithRunner(context.Background(), nil, []string{"completion", "--help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("completion help returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Available Commands:") || !strings.Contains(stdout.String(), "bash") {
		t.Fatalf("completion help missing shells:\n%s", stdout.String())
	}
}

func TestRewriteCommitsFlagValidation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"rewrite-commits", "--batch-size", "51"}, "--batch-size must be 50 or less"},
		{[]string{"rewrite-commits", "--rpm", "0"}, "--rpm must be a positive integer"},
		{[]string{"rewrite-commits", "--concurrency", "0"}, "--concurrency must be a positive integer"},
		{[]string{"rewrite-commits", "--concurrency", "65"}, "--concurrency must be 64 or less"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithRunner(context.Background(), nil, tc.args, strings.NewReader(""), &stdout, &stderr)
		if err == nil {
			t.Fatalf("%v returned nil error", tc.args)
		}
		var exit exitError
		if !errors.As(err, &exit) || exit.code != 1 {
			t.Fatalf("expected exitError(1), got %T %v", err, err)
		}
		if !strings.Contains(stderr.String(), tc.want) {
			t.Fatalf("%v stderr:\n%s", tc.args, stderr.String())
		}
	}
}

func TestRemovedAIContextFlagIsRejected(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, args := range [][]string{
		{"commit", "--max-chars-per-commit", "3000"},
		{"rewrite-commits", "--max-chars-per-commit", "3000"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithRunner(context.Background(), nil, args, strings.NewReader(""), &stdout, &stderr)
		if err == nil {
			t.Fatalf("%v returned nil error", args)
		}
		if !strings.Contains(stderr.String(), "unknown flag: --max-chars-per-commit") {
			t.Fatalf("%v stderr:\n%s", args, stderr.String())
		}
	}
}

func TestCommitFlagValidation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"commit", "--rpm", "0"}, "--rpm must be a positive integer"},
		{[]string{"commit", "--concurrency", "0"}, "--concurrency must be a positive integer"},
		{[]string{"commit", "--concurrency", "65"}, "--concurrency must be 64 or less"},
		{[]string{"commit", "--timeout", "0"}, "--timeout must be a positive integer"},
	} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithRunner(context.Background(), nil, tc.args, strings.NewReader(""), &stdout, &stderr)
		if err == nil {
			t.Fatalf("%v returned nil error", tc.args)
		}
		if !strings.Contains(stderr.String(), tc.want) {
			t.Fatalf("%v stderr:\n%s", tc.args, stderr.String())
		}
	}
}

func TestConfirmUsesInjectedStreams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader("y\n"), &stdout, &stderr)
	makeInteractive(a)
	if confirm(a, "Proceed?") != confirmationAccepted {
		t.Fatal("expected yes confirmation")
	}
	if stdout.String() != "" {
		t.Fatalf("prompt should not be written to stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Proceed? [y/N]") {
		t.Fatalf("prompt not written to injected stderr: %q", stderr.String())
	}
}

func TestRewriteCommitsMissingConfigDoesNotPrompt(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stderr bytes.Buffer
	cmd := newRootCommand(newApp(context.Background(), fakeRunner{}, strings.NewReader("ignored\n"), io.Discard, &stderr))
	cmd.SetArgs([]string{"rewrite-commits", "--yes"})
	cmd.SetIn(strings.NewReader("ignored\n"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing flag failure")
	}
	if strings.Contains(stderr.String(), "OpenAI-compatible API base URL:") {
		t.Fatalf("rewrite-commits should not prompt for removed flags:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "AI model is required") {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}
