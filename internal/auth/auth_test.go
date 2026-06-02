package auth

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAuthenticateGitHubUsesProvidedHTTPClient(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.String() == "https://github.com/login/device/code":
			return formResponse("device_code=device-code&user_code=user-code&verification_uri=https%3A%2F%2Fgithub.com%2Flogin%2Fdevice&expires_in=900&interval=0"), nil
		case req.Method == http.MethodPost && req.URL.String() == "https://github.com/login/oauth/access_token":
			return formResponse("access_token=oauth-token&token_type=bearer&scope=repo%20read%3Aorg"), nil
		case req.Method == http.MethodGet && req.URL.String() == "https://api.github.com/user":
			if got := req.Header.Get("Authorization"); got != "Bearer oauth-token" {
				t.Fatalf("Authorization = %q", got)
			}
			return jsonResponse(`{"login":"octo"}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})}
	var stdout bytes.Buffer
	got, err := (GitHubDeviceAuthenticator{
		ClientID:   "client-id",
		HTTPClient: client,
		BrowseURL:  func(string) error { return nil },
	}).AuthenticateGitHub(context.Background(), "github.com", strings.NewReader("\n"), &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "oauth-token" || got.Username != "octo" {
		t.Fatalf("result = %#v", got)
	}
}

func TestHTTPClientDefaultsToNonNilClient(t *testing.T) {
	if got := (GitHubDeviceAuthenticator{}).httpClient(); got == nil {
		t.Fatal("expected default HTTP client")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func formResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
