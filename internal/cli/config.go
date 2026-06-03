package cli

import (
	"errors"
	"fmt"

	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
	"github.com/spf13/cobra"
)

func configCommand(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Show and edit Git Wrangler setup.",
		GroupID: "utility",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	show := &cobra.Command{
		Use:   "show",
		Short: "Show non-secret configuration and credential sources.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a.json = jsonFlagValue(cmd)
			if code := runConfigShow(a); code != 0 {
				return exitError{code: code}
			}
			return nil
		},
	}
	show.Flags().Bool("json", false, "Emit one JSON document.")
	cmd.AddCommand(
		show,
		configSetCommand(a),
		configUnsetCommand(a),
	)
	return cmd
}

func configSetCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> [value]",
		Short: "Set a configuration value.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("set requires a key")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := runConfigSet(a, args); code != 0 {
				return exitError{code: code}
			}
			return nil
		},
	}
}

func configUnsetCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "unset <key>",
		Short: "Unset a stored credential.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := runConfigUnset(a, args[0]); code != 0 {
				return exitError{code: code}
			}
			return nil
		},
	}
}

func runConfigShow(a *app) int {
	cfg, err := config.Load()
	if err != nil {
		if a.json {
			return writeJSONStatus(a, map[string]any{
				"ok":      false,
				"summary": map[string]any{},
				"error":   jsonError{Message: err.Error()},
			}, 1)
		}
		a.plainErrorf("%s", err.Error())
		return 1
	}
	path, _ := config.Path()
	github := credentials.ResolveGitHubToken(a.creds, cfg.GitHub.Host)
	aiKey := credentials.ResolveAIKey(a.creds, cfg.AI.Provider)

	if a.json {
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
			},
		})
	}

	fmt.Fprintf(a.stdout, "Config: %s\n", path)
	fmt.Fprintln(a.stdout, "GitHub:")
	fmt.Fprintf(a.stdout, "  Host: %s\n", cfg.GitHub.Host)
	fmt.Fprintf(a.stdout, "  Username: %s\n", displayUnset(cfg.GitHub.Username))
	fmt.Fprintf(a.stdout, "  Auth: %s\n", github.Source)
	fmt.Fprintln(a.stdout, "AI:")
	fmt.Fprintf(a.stdout, "  Provider: %s\n", displayUnset(cfg.AI.Provider))
	fmt.Fprintf(a.stdout, "  Base URL: %s\n", displayUnset(cfg.AI.BaseURL))
	fmt.Fprintf(a.stdout, "  Model: %s\n", displayUnset(cfg.AI.Model))
	fmt.Fprintf(a.stdout, "  API key: %s\n", aiKey.Source)
	return 0
}

func runConfigSet(a *app, args []string) int {
	cfg, err := config.Load()
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	key := args[0]
	switch key {
	case "github.auth":
		token, ok := secretValue(a, args, "GitHub token: ")
		if !ok {
			return 1
		}
		if err := a.creds.Set(credentials.GitHubAccount(cfg.GitHub.Host), token); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "github.host":
		value, ok := configValue(a, args, key)
		if !ok {
			return 1
		}
		cfg.GitHub.Host = config.NormalizeHost(value)
		if cfg.GitHub.Host == "" {
			a.plainErrorf("github.host cannot be empty.")
			return 1
		}
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "ai.provider":
		value, ok := configValue(a, args, key)
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
		value, ok := configValue(a, args, key)
		if !ok {
			return 1
		}
		cfg.AI.BaseURL = value
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "ai.model":
		value, ok := configValue(a, args, key)
		if !ok {
			return 1
		}
		cfg.AI.Model = value
		if err := config.Save(cfg); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	case "ai.api-key":
		token, ok := secretValue(a, args, "AI API key: ")
		if !ok {
			return 1
		}
		if err := a.creds.Set(credentials.AIAccount(cfg.AI.Provider), token); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
	default:
		a.plainErrorf("unknown config key %q.", key)
		return 1
	}
	a.ok("Updated " + key)
	return 0
}

func runConfigUnset(a *app, key string) int {
	cfg, err := config.Load()
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	var account string
	switch key {
	case "github.auth":
		account = credentials.GitHubAccount(cfg.GitHub.Host)
	case "ai.api-key":
		account = credentials.AIAccount(cfg.AI.Provider)
	default:
		a.plainErrorf("unknown config key %q.", key)
		return 1
	}
	if err := a.creds.Delete(account); err != nil && !errors.Is(err, credentials.ErrNotFound) {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	a.ok("Unset " + key)
	return 0
}

func configValue(a *app, args []string, key string) (string, bool) {
	if len(args) != 2 || args[1] == "" {
		a.plainErrorf("%s requires a value.", key)
		return "", false
	}
	return args[1], true
}

func secretValue(a *app, args []string, prompt string) (string, bool) {
	if len(args) > 1 {
		a.plainErrorf("%s does not accept a plaintext value.", args[0])
		return "", false
	}
	value, err := promptSecret(a, prompt)
	if err != nil || value == "" {
		a.plainErrorf("secret input is required.")
		return "", false
	}
	return value, true
}

func displayUnset(value string) string {
	if value == "" {
		return "<unset>"
	}
	return value
}
