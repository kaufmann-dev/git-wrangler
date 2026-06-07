package cli

import (
	"bytes"
	"context"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestPromptSessionRequiresStdinAndStderrTTY(t *testing.T) {
	p := newPromptSession(strings.NewReader(""), io.Discard)
	if p.available() {
		t.Fatal("buffered streams should not be interactive")
	}
	p.interactive = func() bool { return true }
	if !p.available() {
		t.Fatal("injected TTY eligibility was ignored")
	}
}

func TestPromptHelpers(t *testing.T) {
	var stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader("\n\nno\n2\nalpha, beta\nsecret\n"), io.Discard, &stderr)
	makeInteractive(a)

	value, err := guidedStringValue(a, "Name", "current")
	if err != nil || value != "current" {
		t.Fatalf("string value = %q, %v", value, err)
	}
	integer, err := guidedPositiveIntegerValue(a, "Count", "12")
	if err != nil || integer != "12" {
		t.Fatalf("integer value = %q, %v", integer, err)
	}
	boolean, err := guidedBooleanValue(a, "Enabled", true)
	if err != nil || boolean {
		t.Fatalf("boolean value = %v, %v", boolean, err)
	}
	enum, err := guidedEnumValue(a, "Mode", "one", []string{"one", "two"})
	if err != nil || enum != "two" {
		t.Fatalf("enum value = %q, %v", enum, err)
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringArray("item", nil, "")
	if err := applyGuidedPrompt(a, cmd, guidedRepeatable("item", "Items")); err != nil {
		t.Fatal(err)
	}
	items, _ := cmd.Flags().GetStringArray("item")
	if strings.Join(items, ",") != "alpha,beta" {
		t.Fatalf("items = %#v", items)
	}
	secret, err := promptSecret(a, "Secret: ")
	if err != nil || secret != "secret" {
		t.Fatalf("secret = %q, %v", secret, err)
	}
}

func TestGuidedRequiredStringRejectsEmptyWithoutDefault(t *testing.T) {
	var stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader("\nrequired\n"), io.Discard, &stderr)
	makeInteractive(a)
	value, err := guidedRequiredStringValue(a, "Name", "")
	if err != nil || value != "required" {
		t.Fatalf("required string = %q, %v", value, err)
	}
	if !strings.Contains(stderr.String(), "A value is required.") {
		t.Fatalf("missing required-value guidance:\n%s", stderr.String())
	}
}

func TestConfirmationResultsAndNonTTYGuidance(t *testing.T) {
	var stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader("y\n"), io.Discard, &stderr)
	if confirm(a, "Proceed?") != confirmationUnavailable {
		t.Fatal("non-TTY confirmation should not be accepted")
	}
	if !a.promptFailed || !strings.Contains(stderr.String(), "pass --yes") {
		t.Fatalf("missing non-TTY guidance:\n%s", stderr.String())
	}

	stderr.Reset()
	a = newApp(context.Background(), fakeRunner{}, strings.NewReader("n\ny\n"), io.Discard, &stderr)
	makeInteractive(a)
	if confirm(a, "First?") != confirmationDeclined {
		t.Fatal("expected decline")
	}
	if confirm(a, "Second?") != confirmationAccepted {
		t.Fatal("expected acceptance")
	}
}

func TestNonTTYCommandConfirmationFailsWithoutDeclineSummary(t *testing.T) {
	root := tempGitRepos(t, "repo")
	t.Chdir(root)
	runner := fakeRunner{lookPath: fakeGitLookPath}
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), runner, []string{"push", "--force-unsafe"}, strings.NewReader("y\n"), &stdout, &stderr)
	if err == nil || !strings.Contains(stderr.String(), "pass --yes") {
		t.Fatalf("error = %v, stderr:\n%s", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "declined") || strings.Contains(stdout.String(), "Summary:") {
		t.Fatalf("non-TTY confirmation was treated as a decline:\n%s", stdout.String())
	}
}

func TestRequiredValuePromptsWithYesAndFailsOutsideTTY(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("name", "", "")
	cmd.Flags().Bool("yes", false, "")
	_ = cmd.Flags().Set("yes", "true")

	var stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader("Ada\n"), io.Discard, &stderr)
	makeInteractive(a)
	value, ok := requiredStringFlag(a, cmd, "name", "Name: ")
	if !ok || value != "Ada" {
		t.Fatalf("required value = %q, %v", value, ok)
	}

	a = newApp(context.Background(), fakeRunner{}, strings.NewReader("Ada\n"), io.Discard, &stderr)
	if _, ok := requiredStringFlag(a, cmd, "name", "Name: "); ok {
		t.Fatal("non-TTY required value should fail")
	}

	var stdout bytes.Buffer
	stderr.Reset()
	err := ExecuteWithIO([]string{"license", "--yes"}, strings.NewReader("Ada\n"), &stdout, &stderr)
	if err == nil || !strings.Contains(stderr.String(), "--name is required") {
		t.Fatalf("license --yes error = %v, stderr:\n%s", err, stderr.String())
	}
}

func TestGuidedCommandAndYesSurfaces(t *testing.T) {
	root := newRootCommand(newApp(context.Background(), fakeRunner{}, strings.NewReader(""), io.Discard, io.Discard))
	guidedWant := strings.Fields("activity clone commit fetch fix-gitignore info license pull push remove-secrets rename-branch reset review rewrite-authors rewrite-commits rewrite-dates status untrack")
	yesWant := strings.Fields("commit fix-gitignore license push remove-secrets reset rewrite-authors rewrite-commits rewrite-dates untrack")
	assertFlagSurface(t, root, "guided", guidedWant)
	assertFlagSurface(t, root, "yes", yesWant)
	for _, name := range yesWant {
		cmd := commandByName(t, root, name)
		if flag := cmd.Flags().ShorthandLookup("y"); flag == nil || flag.Name != "yes" {
			t.Fatalf("%s does not expose -y for --yes", name)
		}
	}
}

func TestGuidedSchemasCoverEveryBehaviorFlagAndExcludeMetaFlags(t *testing.T) {
	root := newRootCommand(newApp(context.Background(), fakeRunner{}, strings.NewReader(""), io.Discard, io.Discard))
	for _, cmd := range root.Commands() {
		if cmd.Flags().Lookup("guided") == nil {
			continue
		}
		want := []string{}
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			switch flag.Name {
			case "guided", "yes", "json":
				return
			default:
				want = append(want, flag.Name)
			}
		})
		got := []string{}
		for _, prompt := range guidedSummaryPrompts(cmd) {
			got = append(got, prompt.flag)
		}
		sort.Strings(want)
		sort.Strings(got)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("%s guided flags = %q, want %q", cmd.Name(), got, want)
		}
	}
}

func TestRepresentativeGuidedFlows(t *testing.T) {
	t.Run("clone", func(t *testing.T) {
		cmd, a, stderr := guidedTestCommand(t, "clone", "octo\n2\n\n\n")
		if err := runGuidedSetup(a, cmd); err != nil {
			t.Fatal(err)
		}
		assertFlagValue(t, cmd, "user", "octo")
		assertFlagValue(t, cmd, "visibility", "public")
		assertFlagValue(t, cmd, "limit", "100")
		assertFlagValue(t, cmd, "into", "octo")
		if !strings.Contains(stderr.String(), "Selected configuration") {
			t.Fatalf("missing summary:\n%s", stderr.String())
		}
	})

	t.Run("push mode", func(t *testing.T) {
		cmd, a, _ := guidedTestCommand(t, "push", "\n3\n")
		if err := runGuidedSetup(a, cmd); err != nil {
			t.Fatal(err)
		}
		assertFlagValue(t, cmd, "force", "false")
		assertFlagValue(t, cmd, "force-unsafe", "true")
	})

	t.Run("activity keeps optional zero year unset", func(t *testing.T) {
		cmd, a, _ := guidedTestCommand(t, "activity", "\n\n\n\n\n")
		if err := runGuidedSetup(a, cmd); err != nil {
			t.Fatal(err)
		}
		if cmd.Flags().Changed("year") {
			t.Fatal("accepting the default optional year should not make --year explicit")
		}
	})

	t.Run("rewrite dates rollback", func(t *testing.T) {
		cmd, a, stderr := guidedTestCommand(t, "rewrite-dates", "2\n\ny\n")
		if err := runGuidedSetup(a, cmd); err != nil {
			t.Fatal(err)
		}
		assertFlagValue(t, cmd, "rollback", "true")
		assertFlagValue(t, cmd, "no-fetch", "true")
		if strings.Contains(stderr.String(), "Target range mode") {
			t.Fatalf("rollback prompted for planning options:\n%s", stderr.String())
		}
	})

	t.Run("json rejected", func(t *testing.T) {
		cmd, a, _ := guidedTestCommand(t, "status", "")
		_ = cmd.Flags().Set("json", "true")
		err := runGuidedSetup(a, cmd)
		if err == nil || !strings.Contains(err.Error(), "--guided cannot be combined with --json") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestInteractiveOnlyCommandsFailOutsideTTY(t *testing.T) {
	for _, args := range [][]string{{"init"}, {"rename-repo"}, {"config", "set", "ai.api-key"}} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithIO(args, strings.NewReader("ignored\n"), &stdout, &stderr)
		if err == nil || !strings.Contains(stderr.String(), "requires an interactive terminal") {
			t.Fatalf("%v error = %v, stderr:\n%s", args, err, stderr.String())
		}
	}
}

func guidedTestCommand(t *testing.T, name, input string) (*cobra.Command, *app, *bytes.Buffer) {
	t.Helper()
	var stderr bytes.Buffer
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader(input), io.Discard, &stderr)
	makeInteractive(a)
	cmd := commandByName(t, newRootCommand(a), name)
	if err := cmd.Flags().Set("guided", "true"); err != nil {
		t.Fatal(err)
	}
	return cmd, a, &stderr
}

func commandByName(t *testing.T, root *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, cmd := range root.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	t.Fatalf("command %q not found", name)
	return nil
}

func assertFlagSurface(t *testing.T, root *cobra.Command, name string, want []string) {
	t.Helper()
	got := []string{}
	for _, cmd := range root.Commands() {
		if cmd.Flags().Lookup(name) != nil {
			got = append(got, cmd.Name())
		}
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("--%s commands = %q, want %q", name, got, want)
	}
}

func assertFlagValue(t *testing.T, cmd *cobra.Command, name, want string) {
	t.Helper()
	flag := cmd.Flags().Lookup(name)
	if flag == nil || flag.Value.String() != want {
		t.Fatalf("--%s = %v, want %q", name, flag, want)
	}
}
