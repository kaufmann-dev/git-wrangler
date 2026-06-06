package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cli/oauth"
	"github.com/cli/oauth/api"
	"github.com/cli/oauth/device"
)

// GitHubOAuthClientID is the public client ID for the Git Wrangler OAuth app.
var GitHubOAuthClientID = "Ov23liuC24uOtSA6s6it"

type GitHubResult struct {
	Token    string
	Username string
}

type WaitEvent struct {
	Remaining time.Duration
}

type GitHubAuthenticator interface {
	AuthenticateGitHub(ctx context.Context, host string, stdin io.Reader, stderr io.Writer, onWait func(WaitEvent)) (GitHubResult, error)
}

type GitHubDeviceAuthenticator struct {
	ClientID   string
	HTTPClient *http.Client
	BrowseURL  func(string) error
}

func NewGitHubDeviceAuthenticator() GitHubDeviceAuthenticator {
	return GitHubDeviceAuthenticator{ClientID: GitHubOAuthClientID}
}

func (a GitHubDeviceAuthenticator) AuthenticateGitHub(ctx context.Context, host string, stdin io.Reader, stderr io.Writer, onWait func(WaitEvent)) (GitHubResult, error) {
	if a.ClientID == "" {
		return GitHubResult{}, errors.New("GitHub OAuth client ID is not configured")
	}
	httpClient := a.httpClient()
	oaHost, err := oauth.NewGitHubHost("https://" + host)
	if err != nil {
		return GitHubResult{}, err
	}
	code, err := device.RequestCode(contextHTTPClient{ctx: ctx, client: httpClient}, oaHost.DeviceCodeURL, a.ClientID, []string{"repo", "read:org"})
	if err != nil {
		return GitHubResult{}, err
	}
	fmt.Fprintf(stderr, "GitHub one-time code: %s\n", code.UserCode)
	fmt.Fprintf(stderr, "GitHub verification URL: %s\n", code.VerificationURI)
	fmt.Fprintln(stderr, "Press Enter to open the verification URL in a browser...")
	if err := waitForEnter(stdin); err != nil {
		return GitHubResult{}, err
	}
	browseURL := a.BrowseURL
	if browseURL == nil {
		browseURL = openBrowser
	}
	if err := browseURL(code.VerificationURI); err != nil {
		fmt.Fprintf(stderr, "Warning: Could not open a browser. Open %s manually.\n", code.VerificationURI)
	}

	stopWaitEvents := emitWaitEvents(code.ExpiresIn, onWait)
	waitCtx, cancelWait := context.WithTimeout(ctx, time.Duration(code.ExpiresIn)*time.Second)
	token, err := device.Wait(waitCtx, contextHTTPClient{ctx: waitCtx, client: httpClient}, oaHost.TokenURL, device.WaitOptions{
		ClientID:   a.ClientID,
		DeviceCode: code,
	})
	cancelWait()
	stopWaitEvents()
	if err != nil {
		if ctx.Err() != nil {
			return GitHubResult{}, ctx.Err()
		}
		var apiErr *api.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &apiErr) && apiErr.Code == "expired_token" {
			return GitHubResult{}, errors.New("GitHub authorization code expired; run 'git-wrangler init' to try again")
		}
		return GitHubResult{}, err
	}
	username, err := fetchGitHubUsername(ctx, host, token.Token, httpClient)
	if err != nil {
		return GitHubResult{}, err
	}
	return GitHubResult{Token: token.Token, Username: username}, nil
}

type contextHTTPClient struct {
	ctx    context.Context
	client *http.Client
}

func (c contextHTTPClient) PostForm(endpoint string, values url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.client.Do(req)
}

func waitForEnter(r io.Reader) error {
	if reader, ok := r.(*bufio.Reader); ok {
		_, err := reader.ReadString('\n')
		return err
	}
	_, err := bufio.NewReader(r).ReadString('\n')
	return err
}

func emitWaitEvents(expiresIn int, onWait func(WaitEvent)) func() {
	if onWait == nil {
		return func() {}
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		emit := func() {
			remaining := time.Until(deadline).Round(time.Second)
			if remaining < 0 {
				remaining = 0
			}
			onWait(WaitEvent{Remaining: remaining})
		}
		emit()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				emit()
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}

func openBrowser(url string) error {
	var commands [][]string
	switch runtime.GOOS {
	case "darwin":
		commands = [][]string{{"open", url}}
	case "windows":
		commands = [][]string{{"cmd", "/c", "start", url}}
	default:
		commands = [][]string{{"xdg-open", url}, {"wslview", url}}
	}
	var lastErr error
	for _, command := range commands {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

func (a GitHubDeviceAuthenticator) httpClient() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return http.DefaultClient
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
