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
	if !requireInteractive(a, "init") {
		return 1
	}
	cfg, err := config.Load()
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	fmt.Fprintln(a.stdout, "Git Wrangler Setup")
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "GitHub")
	keyringAvailable := credentials.KeyringAvailable(a.creds)

	host, err := promptDefault(a, "GitHub host", cfg.GitHub.Host)
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	cfg.GitHub.Host = config.NormalizeHost(host)
	if cfg.GitHub.Host == "" {
		cfg.GitHub.Host = config.DefaultGitHubHost
	}
	if keyringAvailable {
		confirmation := confirm(a, "Authenticate GitHub now?")
		if confirmation == confirmationUnavailable {
			return 1
		}
		if confirmation == confirmationAccepted {
			wait := newAuthorizationWait(a)
			result, err := a.auth.AuthenticateGitHub(a.ctx, cfg.GitHub.Host, a.input, a.stderr, wait.update)
			wait.done()
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
	}
	if !keyringAvailable && credentials.ResolveGitHubToken(a.creds, cfg.GitHub.Host).Value == "" {
		a.warn("Secure credential storage is unavailable, so Git Wrangler skipped GitHub authentication setup. Set GIT_WRANGLER_GITHUB_TOKEN instead.")
	}

	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "AI")
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
	if keyringAvailable {
		confirmation := confirm(a, "Store an AI API key now?")
		if confirmation == confirmationUnavailable {
			return 1
		}
		if confirmation == confirmationAccepted {
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
	}
	if !keyringAvailable && credentials.ResolveAIKey(a.creds, cfg.AI.Provider).Value == "" {
		guidance := "Secure credential storage is unavailable, so Git Wrangler skipped AI API key setup. Set GIT_WRANGLER_AI_API_KEY instead."
		if cfg.AI.Provider == config.DefaultAIProvider {
			guidance = "Secure credential storage is unavailable, so Git Wrangler skipped AI API key setup. Set GIT_WRANGLER_AI_API_KEY or OPENAI_API_KEY instead."
		}
		a.warn(guidance)
	}
	if err := config.Save(cfg); err != nil {
		a.plainErrorf("%s", err.Error())
		return 1
	}
	a.ok("Setup complete")
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Recap")
	github := credentials.ResolveGitHubToken(a.creds, cfg.GitHub.Host)
	aiKey := credentials.ResolveAIKey(a.creds, cfg.AI.Provider)
	renderKeyValues(a, []keyValueRow{
		{key: "GitHub host", value: cfg.GitHub.Host},
		{key: "GitHub auth", value: string(github.Source)},
		{key: "AI provider", value: displayUnsetStyled(a, cfg.AI.Provider)},
		{key: "AI base URL", value: displayUnsetStyled(a, cfg.AI.BaseURL)},
		{key: "AI model", value: displayUnsetStyled(a, cfg.AI.Model)},
		{key: "AI API key", value: string(aiKey.Source)},
	})
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
