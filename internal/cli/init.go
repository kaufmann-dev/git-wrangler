package cli

import (
	"fmt"

	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
	"github.com/spf13/cobra"
)

func initCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "init",
		Short:   "Set up GitHub and AI credentials.",
		GroupID: "utility",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := runInit(a); code != 0 {
				return exitError{code: code}
			}
			return nil
		},
	}
}

func runInit(a *app) int {
	cfg, err := config.Load()
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	fmt.Fprintln(a.stdout, "Git Wrangler setup")
	fmt.Fprintln(a.stdout)

	host, err := promptDefault(a, "GitHub host", cfg.GitHub.Host)
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	cfg.GitHub.Host = config.NormalizeHost(host)
	if cfg.GitHub.Host == "" {
		cfg.GitHub.Host = config.DefaultGitHubHost
	}
	if confirm(a, "Authenticate GitHub now?") {
		result, err := a.auth.AuthenticateGitHub(a.ctx, cfg.GitHub.Host, a.input, a.stderr)
		if err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
		if err := a.creds.Set(credentials.GitHubAccount(cfg.GitHub.Host), result.Token); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
		cfg.GitHub.Username = result.Username
		a.ok("Stored GitHub authentication for " + result.Username)
	}

	provider, err := promptDefault(a, "AI provider", cfg.AI.Provider)
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	cfg.AI.Provider = config.NormalizeName(provider)
	baseURL, err := promptDefault(a, "AI base URL", cfg.AI.BaseURL)
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	cfg.AI.BaseURL = baseURL
	model, err := promptDefault(a, "AI model", cfg.AI.Model)
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	cfg.AI.Model = model
	if confirm(a, "Store an AI API key now?") {
		token, err := promptSecret(a, "AI API key: ")
		if err != nil || token == "" {
			a.plainErrorf("secret input is required.")
			return 1
		}
		if err := a.creds.Set(credentials.AIAccount(cfg.AI.Provider), token); err != nil {
			a.plainErrorf("%s", err.Error())
			return 1
		}
		a.ok("Stored AI API key")
	}
	if err := config.Save(cfg); err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	a.ok("Setup complete")
	return 0
}

func promptDefault(a *app, label, current string) (string, error) {
	prompt := label + ": "
	if current != "" {
		prompt = fmt.Sprintf("%s [%s]: ", label, current)
	}
	value, err := promptRead(a, prompt)
	if value == "" {
		value = current
	}
	return value, err
}
