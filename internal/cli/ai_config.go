package cli

import (
	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
)

type aiSettings struct {
	Config config.Config
	APIKey string
}

func loadAISettings(a *app) (aiSettings, bool) {
	cfg, err := config.Load()
	if err != nil {
		a.plainErrorf("%s", err.Error())
		return aiSettings{}, false
	}
	if cfg.AI.BaseURL == "" {
		a.plainErrorf("AI base URL is required. Run 'git-wrangler config set ai.base-url <url>'.")
		return aiSettings{}, false
	}
	if cfg.AI.Model == "" {
		a.plainErrorf("AI model is required. Run 'git-wrangler config set ai.model <model>'.")
		return aiSettings{}, false
	}
	apiKey := credentials.ResolveAIKey(a.creds, cfg.AI.Provider)
	if apiKey.Err != nil {
		a.plainErrorf("AI API key could not be read: %s", apiKey.Err.Error())
		return aiSettings{}, false
	}
	if apiKey.Value == "" {
		a.plainErrorf("AI API key is required. Run 'git-wrangler config set ai.api-key' or set GIT_WRANGLER_AI_API_KEY.")
		return aiSettings{}, false
	}
	return aiSettings{Config: cfg, APIKey: apiKey.Value}, true
}
