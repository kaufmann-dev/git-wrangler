package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cli/oauth"
)

// GitHubOAuthClientID is the public client ID for the Git Wrangler OAuth app.
var GitHubOAuthClientID = "Ov23liuC24uOtSA6s6it"

type GitHubResult struct {
	Token    string
	Username string
}

type GitHubAuthenticator interface {
	AuthenticateGitHub(ctx context.Context, host string, stdin io.Reader, stdout io.Writer) (GitHubResult, error)
}

type GitHubDeviceAuthenticator struct {
	ClientID   string
	HTTPClient *http.Client
	BrowseURL  func(string) error
}

func NewGitHubDeviceAuthenticator() GitHubDeviceAuthenticator {
	return GitHubDeviceAuthenticator{ClientID: GitHubOAuthClientID}
}

func (a GitHubDeviceAuthenticator) AuthenticateGitHub(ctx context.Context, host string, stdin io.Reader, stdout io.Writer) (GitHubResult, error) {
	if a.ClientID == "" {
		return GitHubResult{}, errors.New("GitHub OAuth client ID is not configured")
	}
	oaHost, err := oauth.NewGitHubHost("https://" + host)
	if err != nil {
		return GitHubResult{}, err
	}
	flow := oauth.Flow{
		Host:       oaHost,
		Scopes:     []string{"repo", "read:org"},
		ClientID:   a.ClientID,
		HTTPClient: a.HTTPClient,
		Stdin:      stdin,
		Stdout:     stdout,
		BrowseURL:  a.BrowseURL,
	}
	token, err := flow.DeviceFlow()
	if err != nil {
		return GitHubResult{}, err
	}
	username, err := fetchGitHubUsername(ctx, host, token.Token, a.HTTPClient)
	if err != nil {
		return GitHubResult{}, err
	}
	return GitHubResult{Token: token.Token, Username: username}, nil
}

func fetchGitHubUsername(ctx context.Context, host, token string, client *http.Client) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	url := "https://api.github.com/user"
	if host != "github.com" {
		url = fmt.Sprintf("https://%s/api/v3/user", host)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub user lookup failed: %s", resp.Status)
	}
	var body struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.Login == "" {
		return "", errors.New("GitHub user lookup did not return a username")
	}
	return body.Login, nil
}
