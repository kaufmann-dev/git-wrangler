package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type confirmationResult int

const (
	confirmationDeclined confirmationResult = iota
	confirmationAccepted
	confirmationUnavailable
	confirmationCancelled
)

var errPromptCancelled = errors.New("prompt cancelled")

type promptSession struct {
	ctx         context.Context
	cancel      context.CancelFunc
	stdin       io.Reader
	stderr      io.Writer
	input       *bufio.Reader
	interactive func() bool
	readSecret  func() ([]byte, error)
	restore     func() error
}

func newPromptSession(ctx context.Context, cancel context.CancelFunc, stdin io.Reader, stderr io.Writer) *promptSession {
	p := &promptSession{ctx: ctx, cancel: cancel, stdin: stdin, stderr: stderr, input: bufio.NewReader(stdin)}
	p.interactive = func() bool {
		in, inOK := stdin.(*os.File)
		out, outOK := stderr.(*os.File)
		return inOK && outOK && term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
	}
	return p
}

func (p *promptSession) available() bool {
	return p != nil && p.interactive != nil && p.interactive()
}

func (p *promptSession) read(prompt string) (string, error) {
	fmt.Fprint(p.stderr, prompt)
	answer, err := p.readWithContext(func() (string, error) {
		return p.input.ReadString('\n')
	})
	return strings.TrimRight(answer, "\r\n"), err
}

func (p *promptSession) secret(prompt string) (string, error) {
	fmt.Fprint(p.stderr, prompt)
	if p.readSecret != nil {
		answer, err := p.readSecretWithContext(p.readSecret, p.restore)
		if !errors.Is(err, errPromptCancelled) {
			fmt.Fprintln(p.stderr)
		}
		return strings.TrimRight(string(answer), "\r\n"), err
	}
	if f, ok := p.stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		state, err := term.GetState(int(f.Fd()))
		if err != nil {
			return "", err
		}
		readSecret := func() ([]byte, error) { return term.ReadPassword(int(f.Fd())) }
		restore := func() error { return term.Restore(int(f.Fd()), state) }
		answer, err := p.readSecretWithContext(readSecret, restore)
		if !errors.Is(err, errPromptCancelled) {
			fmt.Fprintln(p.stderr)
		}
		return strings.TrimRight(string(answer), "\r\n"), err
	}
	answer, err := p.readWithContext(func() (string, error) {
		return p.input.ReadString('\n')
	})
	return strings.TrimRight(answer, "\r\n"), err
}

func (p *promptSession) readSecretWithContext(readSecret func() ([]byte, error), restore func() error) ([]byte, error) {
	type result struct {
		value []byte
		err   error
	}
	results := make(chan result, 1)
	go func() {
		value, err := readSecret()
		results <- result{value: value, err: err}
	}()
	select {
	case <-p.ctx.Done():
		if restore != nil {
			_ = restore()
		}
		p.closeInput()
		return nil, errPromptCancelled
	case result := <-results:
		if p.ctx.Err() != nil {
			if restore != nil {
				_ = restore()
			}
			p.closeInput()
			return nil, errPromptCancelled
		}
		if errors.Is(result.err, io.EOF) {
			p.cancel()
			return nil, errPromptCancelled
		}
		if result.err != nil {
			return nil, result.err
		}
		return result.value, nil
	}
}

func (p *promptSession) readWithContext(read func() (string, error)) (string, error) {
	type result struct {
		value string
		err   error
	}
	results := make(chan result, 1)
	go func() {
		value, err := read()
		results <- result{value: value, err: err}
	}()
	select {
	case <-p.ctx.Done():
		p.closeInput()
		return "", errPromptCancelled
	case result := <-results:
		if p.ctx.Err() != nil {
			return "", errPromptCancelled
		}
		if errors.Is(result.err, io.EOF) {
			p.cancel()
			return "", errPromptCancelled
		}
		if result.err != nil {
			return "", result.err
		}
		return result.value, nil
	}
}

func (p *promptSession) closeInput() {
	if closer, ok := p.stdin.(io.Closer); ok {
		go func() { _ = closer.Close() }()
	}
}

func (p *promptSession) confirm(question string) confirmationResult {
	if !p.available() {
		return confirmationUnavailable
	}
	answer, err := p.read(question + " [y/N] ")
	if errors.Is(err, errPromptCancelled) {
		return confirmationCancelled
	}
	if answer == "y" || answer == "Y" {
		return confirmationAccepted
	}
	return confirmationDeclined
}

type guidedPrompt struct {
	flag    string
	label   string
	kind    string
	choices []string
}

func guidedString(flag, label string) guidedPrompt {
	return guidedPrompt{flag: flag, label: label, kind: "string"}
}

func guidedRequiredString(flag, label string) guidedPrompt {
	return guidedPrompt{flag: flag, label: label, kind: "required-string"}
}

func guidedPositiveInt(flag, label string) guidedPrompt {
	return guidedPrompt{flag: flag, label: label, kind: "positive-int"}
}

func guidedNonNegativeInt(flag, label string) guidedPrompt {
	return guidedPrompt{flag: flag, label: label, kind: "non-negative-int"}
}

func guidedBool(flag, label string) guidedPrompt {
	return guidedPrompt{flag: flag, label: label, kind: "bool"}
}

func guidedRepeatable(flag, label string) guidedPrompt {
	return guidedPrompt{flag: flag, label: label, kind: "repeatable"}
}

func guidedEnum(flag, label string, choices ...string) guidedPrompt {
	return guidedPrompt{flag: flag, label: label, kind: "enum", choices: choices}
}

func runGuidedSetup(a *app, cmd *cobra.Command, spec guidedSpec) error {
	if !boolFlagValue(cmd, "guided") {
		return nil
	}
	if jsonOptionsFromCommand(cmd).enabled {
		return fmt.Errorf("--guided cannot be combined with --json")
	}
	if !a.prompts.available() {
		return fmt.Errorf("--guided requires an interactive terminal for stdin and stderr")
	}
	if !spec.enabled() {
		return fmt.Errorf("--guided is not configured for %s", cmd.Name())
	}

	var err error
	if spec.setup != nil {
		err = spec.setup(a, cmd)
	} else {
		for _, prompt := range spec.prompts {
			if err = applyGuidedPrompt(a, cmd, prompt); err != nil {
				break
			}
		}
	}
	if err != nil {
		return err
	}
	renderGuidedSummary(a, cmd, spec)
	return nil
}

func applyGuidedPrompt(a *app, cmd *cobra.Command, prompt guidedPrompt) error {
	flag := cmd.Flags().Lookup(prompt.flag)
	if flag == nil {
		return fmt.Errorf("guided prompt references unknown flag --%s", prompt.flag)
	}
	current := flag.Value.String()
	label := prompt.label
	switch prompt.kind {
	case "bool":
		value, err := guidedBooleanValue(a, label, current == "true")
		if err != nil {
			return err
		}
		return setGuidedFlag(cmd, prompt.flag, strconv.FormatBool(value))
	case "positive-int":
		value, err := guidedPositiveIntegerValue(a, label, current)
		if err != nil {
			return err
		}
		return setGuidedFlag(cmd, prompt.flag, value)
	case "non-negative-int":
		value, err := guidedNonNegativeIntegerValue(a, label, current)
		if err != nil {
			return err
		}
		return setGuidedFlag(cmd, prompt.flag, value)
	case "enum":
		value, err := guidedEnumValue(a, label, current, prompt.choices)
		if err != nil {
			return err
		}
		return setGuidedFlag(cmd, prompt.flag, value)
	case "repeatable":
		value, err := a.prompts.read(fmt.Sprintf("%s (comma-separated) [%s]: ", label, strings.Trim(current, "[]")))
		if err != nil {
			return err
		}
		if value == "" {
			return nil
		}
		for _, item := range strings.Split(value, ",") {
			if err := cmd.Flags().Set(prompt.flag, strings.TrimSpace(item)); err != nil {
				return err
			}
		}
		return nil
	case "required-string":
		value, err := guidedRequiredStringValue(a, label, current)
		if err != nil {
			return err
		}
		return setGuidedFlag(cmd, prompt.flag, value)
	default:
		value, err := guidedStringValue(a, label, current)
		if err != nil {
			return err
		}
		return setGuidedFlag(cmd, prompt.flag, value)
	}
}

func setGuidedFlag(cmd *cobra.Command, name, value string) error {
	flag := cmd.Flags().Lookup(name)
	if flag != nil && !flag.Changed && flag.Value.String() == value {
		return nil
	}
	return cmd.Flags().Set(name, value)
}

func clearGuidedFlag(cmd *cobra.Command, name, value string) error {
	if err := cmd.Flags().Set(name, value); err != nil {
		return err
	}
	cmd.Flags().Lookup(name).Changed = false
	return nil
}

func guidedRequiredStringValue(a *app, label, current string) (string, error) {
	for {
		value, err := guidedStringValue(a, label, current)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
		fmt.Fprintln(a.stderr, "A value is required.")
	}
}

func guidedStringValue(a *app, label, current string) (string, error) {
	prompt := label + ": "
	if current != "" {
		prompt = fmt.Sprintf("%s [%s]: ", label, current)
	}
	value, err := a.prompts.read(prompt)
	if value == "" {
		value = current
	}
	return value, err
}

func guidedPositiveIntegerValue(a *app, label, current string) (string, error) {
	for {
		value, err := guidedStringValue(a, label, current)
		if err != nil {
			return "", err
		}
		number, err := strconv.Atoi(value)
		if err == nil && number > 0 {
			return value, nil
		}
		fmt.Fprintln(a.stderr, "Enter a positive integer.")
	}
}

func guidedNonNegativeIntegerValue(a *app, label, current string) (string, error) {
	for {
		value, err := guidedStringValue(a, label, current)
		if err != nil {
			return "", err
		}
		number, err := strconv.Atoi(value)
		if err == nil && number >= 0 {
			return value, nil
		}
		fmt.Fprintln(a.stderr, "Enter 0 or a positive integer.")
	}
}

func guidedBooleanValue(a *app, label string, current bool) (bool, error) {
	defaultText := "y/N"
	if current {
		defaultText = "Y/n"
	}
	for {
		value, err := a.prompts.read(fmt.Sprintf("%s [%s]: ", label, defaultText))
		if err != nil {
			return false, err
		}
		switch strings.ToLower(value) {
		case "":
			return current, nil
		case "y", "yes", "true":
			return true, nil
		case "n", "no", "false":
			return false, nil
		default:
			fmt.Fprintln(a.stderr, "Enter yes or no.")
		}
	}
}

func guidedEnumValue(a *app, label, current string, choices []string) (string, error) {
	for {
		fmt.Fprintf(a.stderr, "%s:\n", label)
		for i, choice := range choices {
			marker := ""
			if choice == current {
				marker = " (current)"
			}
			fmt.Fprintf(a.stderr, "  %d. %s%s\n", i+1, choice, marker)
		}
		value, err := a.prompts.read("Choice: ")
		if err != nil {
			return "", err
		}
		if value == "" && current != "" {
			return current, nil
		}
		index, err := strconv.Atoi(value)
		if err == nil && index >= 1 && index <= len(choices) {
			return choices[index-1], nil
		}
		fmt.Fprintln(a.stderr, "Enter one of the listed numbers.")
	}
}

func renderGuidedSummary(a *app, cmd *cobra.Command, spec guidedSpec) {
	fmt.Fprintln(a.stderr)
	fmt.Fprintln(a.stderr, "Selected configuration")
	for _, prompt := range guidedSummaryPrompts(spec) {
		flag := cmd.Flags().Lookup(prompt.flag)
		if flag != nil {
			fmt.Fprintf(a.stderr, "  %s: %s\n", prompt.label, displayGuidedValue(flag.Value.String()))
		}
	}
	fmt.Fprintln(a.stderr)
}

func displayGuidedValue(value string) string {
	if value == "" || value == "[]" {
		return "<unset>"
	}
	return value
}

func guidedSummaryPrompts(spec guidedSpec) []guidedPrompt {
	if len(spec.summary) > 0 {
		return spec.summary
	}
	return spec.prompts
}

func rewriteDatesGuidedSummaryPrompts() []guidedPrompt {
	return []guidedPrompt{
		guidedString("repo", "Repository"),
		guidedBool("no-fetch", "Skip origin fetch"),
		guidedString("rewrite-after", "Current author date on or after"),
		guidedString("rewrite-before", "Current author date before"),
		guidedString("start-date", "Target start date"),
		guidedString("end-date", "Target end date"),
		guidedPositiveInt("days", "Target last N days"),
		guidedString("until", "Last-N-days end date"),
		guidedString("seed", "Seed"),
		guidedEnum("frequency", "Frequency", "low", "medium", "high"),
		guidedEnum("spread", "Spread", "low", "medium", "high"),
		guidedString("window", "Time window"),
	}
}

func guidePush(a *app, cmd *cobra.Command) error {
	if err := applyGuidedPrompt(a, cmd, guidedString("repo", "Repository")); err != nil {
		return err
	}
	current := "normal"
	force := boolFlagValue(cmd, "force")
	forceUnsafe := boolFlagValue(cmd, "force-unsafe")
	if force {
		current = "force-with-lease"
	}
	if forceUnsafe {
		current = "force"
	}
	mode, err := guidedEnumValue(a, "Push mode", current, []string{"normal", "force-with-lease", "force"})
	if err != nil {
		return err
	}
	if err := cmd.Flags().Set("force", strconv.FormatBool(mode == "force-with-lease")); err != nil {
		return err
	}
	return cmd.Flags().Set("force-unsafe", strconv.FormatBool(mode == "force"))
}

func guideClone(a *app, cmd *cobra.Command) error {
	for _, prompt := range []guidedPrompt{
		guidedRequiredString("user", "GitHub user or organization"),
		guidedEnum("visibility", "Visibility", "all", "public", "private"),
		guidedPositiveInt("limit", "Repository limit"),
	} {
		if err := applyGuidedPrompt(a, cmd, prompt); err != nil {
			return err
		}
	}
	into := stringFlagValue(cmd, "into")
	if into == "" {
		into = stringFlagValue(cmd, "user")
		if err := cmd.Flags().Set("into", into); err != nil {
			return err
		}
	}
	return applyGuidedPrompt(a, cmd, guidedString("into", "Destination directory"))
}

func guideRewriteDates(a *app, cmd *cobra.Command) error {
	if _, ok := rewriteDatesOptionsFromCommand(a, cmd); !ok {
		return exitError{code: 1}
	}
	for _, prompt := range []guidedPrompt{guidedString("repo", "Repository"), guidedBool("no-fetch", "Skip origin fetch")} {
		if err := applyGuidedPrompt(a, cmd, prompt); err != nil {
			return err
		}
	}
	for _, prompt := range []guidedPrompt{
		guidedString("rewrite-after", "Current author date on or after"),
		guidedString("rewrite-before", "Current author date before"),
		guidedEnum("target-range", "Target range mode", "explicit dates", "last N days"),
	} {
		if prompt.flag == "target-range" {
			rangeMode, err := guidedEnumValue(a, prompt.label, rewriteDatesRangeMode(cmd), prompt.choices)
			if err != nil {
				return err
			}
			if rangeMode == "last N days" {
				if err := clearGuidedFlag(cmd, "start-date", ""); err != nil {
					return err
				}
				if err := clearGuidedFlag(cmd, "end-date", ""); err != nil {
					return err
				}
				if err := applyGuidedPrompt(a, cmd, guidedPositiveInt("days", "Target last N days")); err != nil {
					return err
				}
				if err := applyGuidedPrompt(a, cmd, guidedString("until", "Last-N-days end date")); err != nil {
					return err
				}
			} else {
				if err := clearGuidedFlag(cmd, "days", "0"); err != nil {
					return err
				}
				if err := clearGuidedFlag(cmd, "until", ""); err != nil {
					return err
				}
				if err := applyGuidedPrompt(a, cmd, guidedString("start-date", "Target start date")); err != nil {
					return err
				}
				if err := applyGuidedPrompt(a, cmd, guidedString("end-date", "Target end date")); err != nil {
					return err
				}
			}
			continue
		}
		if err := applyGuidedPrompt(a, cmd, prompt); err != nil {
			return err
		}
	}
	for _, prompt := range []guidedPrompt{
		guidedString("seed", "Seed"),
		guidedEnum("frequency", "Frequency", "low", "medium", "high"),
		guidedEnum("spread", "Spread", "low", "medium", "high"),
		guidedString("window", "Time window"),
	} {
		if err := applyGuidedPrompt(a, cmd, prompt); err != nil {
			return err
		}
	}
	return nil
}

func rewriteDatesRangeMode(cmd *cobra.Command) string {
	days := intFlagValue(cmd, "days")
	if days > 0 {
		return "last N days"
	}
	return "explicit dates"
}
