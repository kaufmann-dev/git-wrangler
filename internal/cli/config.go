package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
	"github.com/kaufmann-dev/git-wrangler/internal/run"
	"github.com/spf13/cobra"
)

func runConfigShowCommand(a *app, cmd *cobra.Command, args []string) int {
	return runConfigShow(a, configShowOptionsFromCommand(cmd))
}

func runConfigSetCommand(a *app, cmd *cobra.Command, args []string) int {
	return runConfigSet(a, configSetOptionsFromCommand(cmd, args))
}

func runConfigUnsetCommand(a *app, cmd *cobra.Command, args []string) int {
	return runConfigUnset(a, configUnsetOptionsFromCommand(args))
}

func runConfigFileRemoveSecretsPathCommand(a *app, cmd *cobra.Command, args []string) int {
	configPath, err := config.RemoveSecretsPath()
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	fmt.Fprintln(a.stdout, configPath)
	return 0
}

func runConfigFileRemoveSecretsShowCommand(a *app, cmd *cobra.Command, args []string) int {
	paths, err := config.LoadRemoveSecretsPaths()
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	if len(paths) == 0 {
		fmt.Fprintln(a.stdout, "The remove-secrets config file lists no paths; remove-secrets will purge nothing.")
		return 0
	}
	fmt.Fprintln(a.stdout, "Remove-secrets paths")
	for _, path := range paths {
		fmt.Fprintf(a.stdout, "  %s\n", path)
	}
	return 0
}

func runConfigFileRemoveSecretsEditCommand(a *app, cmd *cobra.Command, args []string) int {
	configPath, err := config.EnsureRemoveSecretsStarter()
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	editorName, editorArgs := editorCommand(configPath)
	if err := run.Interactive(a.ctx, a.runner, "", nil, editorName, editorArgs, a.stdin, a.stdout, a.stderr); err != nil {
		a.plainErrorf("editor failed: %s", err)
		return 1
	}
	if _, err := config.LoadRemoveSecretsPaths(); err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	a.ok("Validated " + configPath)
	return 0
}

func configSetArgs(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return errors.New("set requires a key")
	}
	if len(args) > 2 {
		return errors.New("set accepts at most one value")
	}
	return nil
}

func editorCommand(path string) (string, []string) {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "nano"
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return "nano", []string{path}
	}
	return parts[0], append(parts[1:], path)
}

func runConfigShow(a *app, opts configShowOptions) int {
	cfg, err := config.Load()
	if err != nil {
		if opts.json.enabled {
			return writeJSONStatus(a, map[string]any{
				"ok":      false,
				"summary": map[string]any{},
				"error":   jsonError{Message: err.Error()},
			})
		}
		a.plainErrorf("%s", err.Error())
		return 1
	}
	path, _ := config.Path()
	github := credentials.ResolveGitHubToken(a.creds, cfg.GitHub.Host)
	aiKey := credentials.ResolveAIKey(a.creds, cfg.AI.Provider)
	aiHeaders := aiHeaderSummaries(a, cfg)

	if opts.json.enabled {
		return writeJSON(a, map[string]any{
			"ok":      true,
			"summary": map[string]any{"path": path},
			"path":    path,
			"github": map[string]any{
				"host":       cfg.GitHub.Host,
				"username":   cfg.GitHub.Username,
				"authSource": string(github.Source),
				"authSet":    github.Value != "",
			},
			"ai": map[string]any{
				"provider":     cfg.AI.Provider,
				"baseURL":      cfg.AI.BaseURL,
				"model":        cfg.AI.Model,
				"apiKeySource": string(aiKey.Source),
				"apiKeySet":    aiKey.Value != "",
				"headers":      aiHeaders,
			},
		})
	}

	fmt.Fprintln(a.stdout, "Config")
	renderKeyValues(a, []keyValueRow{{key: "Path", value: path}})
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "GitHub")
	renderKeyValues(a, []keyValueRow{
		{key: "Host", value: cfg.GitHub.Host},
		{key: "Username", value: displayUnsetStyled(a, cfg.GitHub.Username)},
		{key: "Auth", value: string(github.Source)},
	})
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "AI")
	renderKeyValues(a, []keyValueRow{
		{key: "Provider", value: displayUnsetStyled(a, cfg.AI.Provider)},
		{key: "Base URL", value: displayUnsetStyled(a, cfg.AI.BaseURL)},
		{key: "Model", value: displayUnsetStyled(a, cfg.AI.Model)},
		{key: "API key", value: string(aiKey.Source)},
		{key: "Headers", value: displayHeaderSummary(a, aiHeaders)},
	})
	return 0
}

func runConfigSet(a *app, opts configSetOptions) int {
	cfg, err := config.Load()
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	if !supportedConfigKey(opts.key) {
		a.plainErrorf("unknown config key %q.", opts.key)
		return 1
	}
	switch opts.key {
	case "github.auth":
		token, ok := secretValue(a, opts.key, opts.hasValue, "GitHub token: ")
		if !ok {
			return 1
		}
		if err := a.creds.Set(credentials.GitHubAccount(cfg.GitHub.Host), token); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "github.host":
		value, ok := configValue(a, opts.key, opts.value, opts.hasValue)
		if !ok {
			return 1
		}
		currentHost := cfg.GitHub.Host
		nextHost := config.NormalizeHost(value)
		if nextHost == "" {
			a.plainErrorf("github.host cannot be empty.")
			return 1
		}
		cfg.GitHub.Host = nextHost
		if nextHost != currentHost {
			cfg.GitHub.Username = ""
		}
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "ai.provider":
		value, ok := configValue(a, opts.key, opts.value, opts.hasValue)
		if !ok {
			return 1
		}
		cfg.AI.Provider = config.NormalizeName(value)
		if cfg.AI.Provider == "" {
			a.plainErrorf("ai.provider cannot be empty.")
			return 1
		}
		config.ApplyDefaults(&cfg)
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "ai.base-url":
		value, ok := configValue(a, opts.key, opts.value, opts.hasValue)
		if !ok {
			return 1
		}
		cfg.AI.BaseURL = value
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "ai.model":
		value, ok := configValue(a, opts.key, opts.value, opts.hasValue)
		if !ok {
			return 1
		}
		cfg.AI.Model = value
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "ai.api-key":
		token, ok := secretValue(a, opts.key, opts.hasValue, "AI API key: ")
		if !ok {
			return 1
		}
		if err := a.creds.Set(credentials.AIAccount(cfg.AI.Provider), token); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	default:
		if strings.HasPrefix(opts.key, "ai.headers.") {
			return runConfigSetAIHeader(a, cfg, opts)
		}
	}
	a.ok("Updated " + opts.key)
	return 0
}

func runConfigUnset(a *app, opts configUnsetOptions) int {
	cfg, err := config.Load()
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	if !supportedConfigKey(opts.key) {
		a.plainErrorf("unknown config key %q.", opts.key)
		return 1
	}
	switch opts.key {
	case "github.auth":
		if err := a.creds.Delete(credentials.GitHubAccount(cfg.GitHub.Host)); err != nil && !errors.Is(err, credentials.ErrNotFound) {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "github.host":
		cfg.GitHub.Host = config.DefaultGitHubHost
		cfg.GitHub.Username = ""
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "ai.api-key":
		if err := a.creds.Delete(credentials.AIAccount(cfg.AI.Provider)); err != nil && !errors.Is(err, credentials.ErrNotFound) {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "ai.provider":
		cfg.AI.Provider = config.DefaultAIProvider
		cfg.AI.BaseURL = ""
		config.ApplyDefaults(&cfg)
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "ai.base-url":
		cfg.AI.BaseURL = ""
		config.ApplyDefaults(&cfg)
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "ai.model":
		cfg.AI.Model = ""
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	default:
		if strings.HasPrefix(opts.key, "ai.headers.") {
			return runConfigUnsetAIHeader(a, cfg, opts.key)
		}
	}
	a.ok("Unset " + opts.key)
	return 0
}

func supportedConfigKey(key string) bool {
	switch key {
	case "github.auth", "github.host", "ai.api-key", "ai.provider", "ai.base-url", "ai.model":
		return true
	default:
		return strings.HasPrefix(key, "ai.headers.")
	}
}

func runConfigSetAIHeader(a *app, cfg config.Config, opts configSetOptions) int {
	header, ok := configHeaderName(a, opts.key)
	if !ok {
		return 1
	}
	if opts.hasValue && opts.value != "" {
		if cfg.AI.Headers == nil {
			cfg.AI.Headers = map[string]string{}
		}
		cfg.AI.Headers[header] = opts.value
		cfg.AI.SecretHeaders = removeHeaderName(cfg.AI.SecretHeaders, header)
		if err := a.creds.Delete(credentials.AIHeaderAccount(cfg.AI.Provider, header)); err != nil && !errors.Is(err, credentials.ErrNotFound) {
			a.plainErrorf("%s", err.Error())
			return 1
		}
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
		a.ok("Updated ai.headers." + header)
		return 0
	}
	token, ok := secretValue(a, opts.key, opts.hasValue, header+": ")
	if !ok {
		return 1
	}
	if err := a.creds.Set(credentials.AIHeaderAccount(cfg.AI.Provider, header), token); err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	if cfg.AI.Headers != nil {
		delete(cfg.AI.Headers, header)
	}
	cfg.AI.SecretHeaders = appendHeaderName(cfg.AI.SecretHeaders, header)
	if err := config.Save(cfg); err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	a.ok("Updated ai.headers." + header)
	return 0
}

func runConfigUnsetAIHeader(a *app, cfg config.Config, key string) int {
	header, ok := configHeaderName(a, key)
	if !ok {
		return 1
	}
	if cfg.AI.Headers != nil {
		delete(cfg.AI.Headers, header)
	}
	cfg.AI.SecretHeaders = removeHeaderName(cfg.AI.SecretHeaders, header)
	if err := a.creds.Delete(credentials.AIHeaderAccount(cfg.AI.Provider, header)); err != nil && !errors.Is(err, credentials.ErrNotFound) {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	if err := config.Save(cfg); err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	a.ok("Unset ai.headers." + header)
	return 0
}

func configHeaderName(a *app, key string) (string, bool) {
	raw := strings.TrimPrefix(key, "ai.headers.")
	header, ok := config.CanonicalHeaderName(raw)
	if !ok {
		a.plainErrorf("invalid AI header name %q.", raw)
		return "", false
	}
	return header, true
}

func appendHeaderName(headers []string, header string) []string {
	headers = removeHeaderName(headers, header)
	headers = append(headers, header)
	sort.Strings(headers)
	return headers
}

func removeHeaderName(headers []string, header string) []string {
	out := headers[:0]
	for _, existing := range headers {
		if !strings.EqualFold(existing, header) {
			out = append(out, existing)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func aiHeaderSummaries(a *app, cfg config.Config) []map[string]any {
	names := map[string]bool{}
	for name := range cfg.AI.Headers {
		names[name] = true
	}
	for _, name := range cfg.AI.SecretHeaders {
		names[name] = true
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	rows := make([]map[string]any, 0, len(sorted))
	for _, name := range sorted {
		source := "config"
		set := cfg.AI.Headers[name] != ""
		if containsHeaderName(cfg.AI.SecretHeaders, name) {
			resolved, err := a.creds.Get(credentials.AIHeaderAccount(cfg.AI.Provider, name))
			source = string(credentials.SourceKeyring)
			set = err == nil && resolved != ""
		}
		rows = append(rows, map[string]any{
			"name":   name,
			"source": source,
			"set":    set,
		})
	}
	return rows
}

func displayHeaderSummary(a *app, headers []map[string]any) string {
	if len(headers) == 0 {
		return displayUnsetStyled(a, "")
	}
	parts := make([]string, 0, len(headers))
	for _, header := range headers {
		parts = append(parts, fmt.Sprintf("%s (%s)", header["name"], header["source"]))
	}
	return strings.Join(parts, ", ")
}

func containsHeaderName(headers []string, header string) bool {
	for _, existing := range headers {
		if strings.EqualFold(existing, header) {
			return true
		}
	}
	return false
}

func configValue(a *app, key, value string, hasValue bool) (string, bool) {
	if !hasValue || value == "" {
		a.plainErrorf("%s requires a value.", key)
		return "", false
	}
	return value, true
}

func secretValue(a *app, key string, hasValue bool, prompt string) (string, bool) {
	if hasValue {
		a.plainErrorf("%s does not accept a plaintext value.", key)
		return "", false
	}
	if !requireInteractive(a, "secret config values") {
		return "", false
	}
	value, err := promptSecret(a, prompt)
	if errors.Is(err, errPromptCancelled) {
		return "", false
	}
	if err != nil || value == "" {
		a.plainErrorf("secret input is required.")
		return "", false
	}
	return value, true
}

func displayUnsetStyled(a *app, value string) string {
	if value == "" {
		return a.ui.Muted + "<unset>" + a.ui.Reset
	}
	return value
}
