package cli

import (
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
)

type aiSettings struct {
	Config  config.Config
	APIKey  string
	Headers map[string]string
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
	headers, ok := resolveAIHeaders(a, cfg)
	if !ok {
		return aiSettings{}, false
	}
	apiKey := credentials.ResolveAIKey(a.creds, cfg.AI.Provider)
	if apiKey.Err != nil {
		a.plainErrorf("AI API key could not be read: %s", apiKey.Err.Error())
		return aiSettings{}, false
	}
	if apiKey.Value == "" && !hasAuthHeader(headers) {
		a.plainErrorf("AI API key is required. Run 'git-wrangler config set ai.api-key' or set GIT_WRANGLER_AI_API_KEY.")
		return aiSettings{}, false
	}
	return aiSettings{Config: cfg, APIKey: apiKey.Value, Headers: headers}, true
}

func resolveAIHeaders(a *app, cfg config.Config) (map[string]string, bool) {
	headers := map[string]string{}
	for name, value := range cfg.AI.Headers {
		canonical, ok := config.CanonicalHeaderName(name)
		if !ok {
			a.plainErrorf("invalid AI header name %q.", name)
			return nil, false
		}
		headers[canonical] = value
	}
	for _, name := range cfg.AI.SecretHeaders {
		canonical, ok := config.CanonicalHeaderName(name)
		if !ok {
			a.plainErrorf("invalid AI header name %q.", name)
			return nil, false
		}
		value, err := a.creds.Get(credentials.AIHeaderAccount(cfg.AI.Provider, canonical))
		if err != nil {
			a.plainErrorf("AI header %s could not be read: %s", canonical, err.Error())
			return nil, false
		}
		headers[canonical] = value
	}
	if len(headers) == 0 {
		return nil, true
	}
	return headers, true
}

func hasAuthHeader(headers map[string]string) bool {
	for name, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		switch strings.ToLower(name) {
		case "authorization", "api-key", "x-api-key", "ocp-apim-subscription-key":
			return true
		}
	}
	return false
}
