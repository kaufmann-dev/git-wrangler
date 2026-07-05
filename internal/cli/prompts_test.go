package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type blockingPromptReader struct {
	started chan struct{}
	release chan string
	once    sync.Once
}

func newBlockingPromptReader() *blockingPromptReader {
	return &blockingPromptReader{started: make(chan struct{}), release: make(chan string, 1)}
}

func (r *blockingPromptReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	value := <-r.release
	copy(p, value)
	return len(value), nil
}

func (r *blockingPromptReader) Close() error { return nil }

func TestPromptSessionRequiresStdinAndStderrTTY(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := newPromptSession(ctx, cancel, strings.NewReader(""), io.Discard)
	if p.available() {
		t.Fatal("buffered streams should not be interactive")
	}
	p.interactive = func() bool { return true }
	if !p.available() {
		t.Fatal("injected TTY eligibility was ignored")
	}
}

func TestPromptCancellationReturnsImmediatelyAcrossPromptKinds(t *testing.T) {
	tests := []struct {
		name string
		call func(*app) error
	}{
		{name: "normal", call: func(a *app) error { _, err := promptRead(a, "Value: "); return err }},
		{name: "guided string", call: func(a *app) error { _, err := guidedStringValue(a, "Name", ""); return err }},
		{name: "guided required", call: func(a *app) error { _, err := guidedRequiredStringValue(a, "Name", ""); return err }},
		{name: "guided boolean", call: func(a *app) error { _, err := guidedBooleanValue(a, "Enabled", false); return err }},
		{name: "guided enum", call: func(a *app) error { _, err := guidedEnumValue(a, "Mode", "one", []string{"one", "two"}); return err }},
		{name: "guided repeatable", call: func(a *app) error {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().StringArray("item", nil, "")
			return applyGuidedPrompt(a, cmd, guidedRepeatable("item", "Items"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			reader := newBlockingPromptReader()
			a := newApp(ctx, fakeRunner{}, reader, io.Discard, io.Discard)
			makeInteractive(a)
			result := make(chan error, 1)
			go func() { result <- test.call(a) }()
			<-reader.started
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, errPromptCancelled) {
					t.Fatalf("error = %v", err)
				}
			case <-time.After(250 * time.Millisecond):
				t.Fatal("prompt cancellation waited for input")
			}
		})
	}
}

func TestInteractiveEOFCancelsPromptWithoutReturningPartialInput(t *testing.T) {
	a := newApp(context.Background(), fakeRunner{}, strings.NewReader("partial"), io.Discard, io.Discard)
	makeInteractive(a)
	value, err := promptRead(a, "Value: ")
	if value != "" || !errors.Is(err, errPromptCancelled) {
		t.Fatalf("value = %q, error = %v", value, err)
	}
	if a.ctx.Err() == nil {
		t.Fatal("EOF did not cancel the application context")
	}
}

func TestConfirmationCancellationIsNotDecline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := newBlockingPromptReader()
	a := newApp(ctx, fakeRunner{}, reader, io.Discard, io.Discard)
	makeInteractive(a)
	result := make(chan confirmationResult, 1)
	go func() { result <- confirm(a, "Proceed?") }()
	<-reader.started
	cancel()
	select {
	case got := <-result:
		if got != confirmationCancelled {
			t.Fatalf("confirmation = %v", got)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("confirmation cancellation waited for input")
	}
	if a.promptFailed {
		t.Fatal("cancelled confirmation was treated as unavailable")
	}
}

func TestCommandConfirmationCancellationPerformsNoMutation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	rootDir := tempGitRepos(t, "repo")
	t.Chdir(rootDir)
	ctx, cancel := context.WithCancel(context.Background())
	reader := newBlockingPromptReader()
	mutated := false
	runner := fakeRunner{
		lookPath: fakeGitLookPath,
		run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
			mutated = true
			return "", "", nil
		},
	}
	var stdout, stderr bytes.Buffer
	a := newApp(ctx, runner, reader, &stdout, &stderr)
	makeInteractive(a)
	root := newRootCommand(a)
	root.SetArgs([]string{"push", "--force-unsafe"})
	result := make(chan error, 1)
	go func() { result <- root.Execute() }()
	<-reader.started
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected cancellation failure")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("confirmation cancellation waited for input")
	}
	if mutated {
		t.Fatal("mutation ran after confirmation cancellation")
	}
	if strings.Contains(stdout.String(), "declined") || strings.Contains(stdout.String(), "Summary:") {
		t.Fatalf("cancellation was treated as a decline:\n%s", stdout.String())
	}
}

func TestSecretCancellationRestoresTerminalState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := newApp(ctx, fakeRunner{}, strings.NewReader(""), io.Discard, io.Discard)
	started := make(chan struct{})
	release := make(chan struct{})
	a.prompts.readSecret = func() ([]byte, error) {
		close(started)
		<-release
		return []byte("late-secret"), nil
	}
	restored := make(chan struct{}, 1)
	a.prompts.restore = func() error {
		restored <- struct{}{}
		return nil
	}
	result := make(chan error, 1)
	go func() {
		_, err := promptSecret(a, "Secret: ")
		result <- err
	}()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, errPromptCancelled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("secret cancellation waited for input")
	}
	select {
	case <-restored:
	default:
		t.Fatal("terminal state was not restored")
	}
	close(release)
}

func TestAbandonedPromptReadCannotMutateGuidedState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := newBlockingPromptReader()
	var stderr bytes.Buffer
	a := newApp(ctx, fakeRunner{}, reader, io.Discard, &stderr)
	makeInteractive(a)
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringArray("item", nil, "")
	result := make(chan error, 1)
	go func() { result <- applyGuidedPrompt(a, cmd, guidedRepeatable("item", "Items")) }()
	<-reader.started
	cancel()
	if err := <-result; !errors.Is(err, errPromptCancelled) {
		t.Fatalf("error = %v", err)
	}
	before := stderr.String()
	reader.release <- "late, values\n"
	time.Sleep(10 * time.Millisecond)
	items, _ := cmd.Flags().GetStringArray("item")
	if len(items) != 0 {
		t.Fatalf("items mutated after cancellation: %#v", items)
	}
	if stderr.String() != before {
		t.Fatalf("abandoned read printed output: %q", stderr.String())
	}
}

func TestGuidedCancellationStopsPromptsAndSummary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := newBlockingPromptReader()
	var stdout, stderr bytes.Buffer
	a := newApp(ctx, fakeRunner{}, reader, &stdout, &stderr)
	makeInteractive(a)
	cmd := commandByName(t, newRootCommand(a), "activity")
	_ = cmd.Flags().Set("guided", "true")
	spec := guidedSpecForCommandName(t, "activity")
	result := make(chan error, 1)
	go func() { result <- runGuidedSetup(a, cmd, spec) }()
	<-reader.started
	cancel()
	if err := <-result; !errors.Is(err, errPromptCancelled) {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(stderr.String(), "Year") || strings.Contains(stderr.String(), "Selected configuration") {
		t.Fatalf("guided setup continued after cancellation:\n%s", stderr.String())
	}
}

func TestCustomAndStandardCommandCancellationRenderOnce(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "standard guided", args: []string{"activity", "--guided"}},
		{name: "custom init", args: []string{"init"}},
		{name: "custom config", args: []string{"config", "set", "ai.api-key"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", "1")
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			ctx, cancel := context.WithCancel(context.Background())
			reader := newBlockingPromptReader()
			var stdout, stderr bytes.Buffer
			a := newApp(ctx, fakeRunner{}, reader, &stdout, &stderr)
			makeInteractive(a)
			a.creds = &fakeCredentialStore{}
			root := newRootCommand(a)
			root.SetArgs(test.args)
			result := make(chan error, 1)
			go func() { result <- root.Execute() }()
			<-reader.started
			cancel()
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("expected nonzero cancellation")
				}
			case <-time.After(250 * time.Millisecond):
				t.Fatal("command cancellation waited for input")
			}
			if got := strings.Count(stdout.String(), "SKIP stopped: operation cancelled"); got != 1 {
				t.Fatalf("cancellation status count = %d\nstdout:\n%s\nstderr:\n%s", got, stdout.String(), stderr.String())
			}
			for _, misleading := range []string{"is required", "Enter one", "Selected configuration"} {
				if strings.Contains(stderr.String(), misleading) {
					t.Fatalf("misleading cancellation output %q:\n%s", misleading, stderr.String())
				}
			}
		})
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
	err := ExecuteWithRunner(context.Background(), nil, []string{"license", "--yes"}, strings.NewReader("Ada\n"), &stdout, &stderr)
	if err == nil || !strings.Contains(stderr.String(), "--name is required") {
		t.Fatalf("license --yes error = %v, stderr:\n%s", err, stderr.String())
	}
}

func TestGuidedCommandAndYesSurfaces(t *testing.T) {
	root := newRootCommand(newApp(context.Background(), fakeRunner{}, strings.NewReader(""), io.Discard, io.Discard))
	guidedWant := strings.Fields("activity clone commit fetch fix-gitignore info license log pull push remove-secrets rename-branch reset review rewrite-authors rewrite-commits rewrite-dates rewrite-hours rollback-rewrites status untrack")
	yesWant := strings.Fields("commit fix-gitignore license push remove-secrets rename-branch reset rewrite-authors rewrite-commits rewrite-dates rewrite-hours rollback-rewrites untrack")
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
		for _, prompt := range guidedSummaryPrompts(guidedSpecForCommandName(t, cmd.Name())) {
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
		if err := runGuidedSetupForTest(t, a, cmd); err != nil {
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
		if err := runGuidedSetupForTest(t, a, cmd); err != nil {
			t.Fatal(err)
		}
		assertFlagValue(t, cmd, "force", "false")
		assertFlagValue(t, cmd, "force-unsafe", "true")
	})

	t.Run("activity keeps optional zero year unset", func(t *testing.T) {
		cmd, a, _ := guidedTestCommand(t, "activity", "\n\n\n\n\n")
		if err := runGuidedSetupForTest(t, a, cmd); err != nil {
			t.Fatal(err)
		}
		if cmd.Flags().Changed("year") {
			t.Fatal("accepting the default optional year should not make --year explicit")
		}
	})

	t.Run("rewrite dates rewrite", func(t *testing.T) {
		cmd, a, stderr := guidedTestCommand(t, "rewrite-dates", "\n\n\n\n\n2024-01-01\n2024-01-31\nseed\n3\n1\n\n")
		if err := runGuidedSetupForTest(t, a, cmd); err != nil {
			t.Fatal(err)
		}
		assertFlagValue(t, cmd, "start-date", "2024-01-01")
		assertFlagValue(t, cmd, "end-date", "2024-01-31")
		assertFlagValue(t, cmd, "seed", "seed")
		assertFlagValue(t, cmd, "frequency", "high")
		assertFlagValue(t, cmd, "spread", "low")
		if !strings.Contains(stderr.String(), "Frequency: high") || !strings.Contains(stderr.String(), "Spread: low") {
			t.Fatalf("rewrite summary did not show frequency and spread:\n%s", stderr.String())
		}
	})

	t.Run("rollback rewrites", func(t *testing.T) {
		cmd, a, stderr := guidedTestCommand(t, "rollback-rewrites", "\n")
		if err := runGuidedSetupForTest(t, a, cmd); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(stderr.String(), "Target range mode") {
			t.Fatalf("rollback-rewrites prompted for date planning options:\n%s", stderr.String())
		}
	})

	t.Run("json rejected", func(t *testing.T) {
		cmd, a, _ := guidedTestCommand(t, "status", "")
		_ = cmd.Flags().Set("json", "true")
		err := runGuidedSetupForTest(t, a, cmd)
		if err == nil || !strings.Contains(err.Error(), "--guided cannot be combined with --json") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestInteractiveOnlyCommandsFailOutsideTTY(t *testing.T) {
	for _, args := range [][]string{{"init"}, {"rename-repo"}, {"config", "set", "ai.api-key"}} {
		var stdout, stderr bytes.Buffer
		err := ExecuteWithRunner(context.Background(), nil, args, strings.NewReader("ignored\n"), &stdout, &stderr)
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

func runGuidedSetupForTest(t *testing.T, a *app, cmd *cobra.Command) error {
	t.Helper()
	return runGuidedSetup(a, cmd, guidedSpecForCommandName(t, cmd.Name()))
}

func guidedSpecForCommandName(t *testing.T, name string) guidedSpec {
	t.Helper()
	for _, spec := range commandSpecs() {
		if commandUseName(spec.use) == name {
			return spec.guided
		}
	}
	t.Fatalf("command spec %q not found", name)
	return guidedSpec{}
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
