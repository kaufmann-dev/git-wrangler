package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAuthenticateGitHubOpensBrowserAndAuthorizes(t *testing.T) {
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
	var openedURL string
	var stderr bytes.Buffer
	got, err := (GitHubDeviceAuthenticator{
		ClientID:   "client-id",
		HTTPClient: client,
		BrowseURL: func(url string) error {
			openedURL = url
			return nil
		},
	}).AuthenticateGitHub(context.Background(), "github.com", strings.NewReader("\n"), &stderr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "oauth-token" || got.Username != "octo" {
		t.Fatalf("result = %#v", got)
	}
	if openedURL != "https://github.com/login/device" {
		t.Fatalf("opened URL = %q", openedURL)
	}
	for _, want := range []string{"user-code", "https://github.com/login/device", "Press Enter"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestAuthenticateGitHubContinuesWhenBrowserFails(t *testing.T) {
	client := successfulAuthClient(t)
	var stderr bytes.Buffer
	got, err := (GitHubDeviceAuthenticator{
		ClientID:   "client-id",
		HTTPClient: client,
		BrowseURL:  func(string) error { return errors.New("raw launcher failure") },
	}).AuthenticateGitHub(context.Background(), "github.com", strings.NewReader("\n"), &stderr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "oauth-token" {
		t.Fatalf("result = %#v", got)
	}
	if !strings.Contains(stderr.String(), "Could not open a browser") || !strings.Contains(stderr.String(), "manually") {
		t.Fatalf("missing browser fallback warning:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "raw launcher failure") {
		t.Fatalf("raw launcher output leaked:\n%s", stderr.String())
	}
}

func TestAuthenticateGitHubExpirationIsActionable(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/login/device/code") {
			return formResponse("device_code=device-code&user_code=user-code&verification_uri=https%3A%2F%2Fgithub.com%2Flogin%2Fdevice&expires_in=1&interval=1"), nil
		}
		t.Fatalf("unexpected request after device code: %s", req.URL)
		return nil, nil
	})}
	_, err := (GitHubDeviceAuthenticator{
		ClientID:   "client-id",
		HTTPClient: client,
		BrowseURL:  func(string) error { return nil },
	}).AuthenticateGitHub(context.Background(), "github.com", strings.NewReader("\n"), io.Discard, nil)
	if err == nil || !strings.Contains(err.Error(), "authorization code expired") || !strings.Contains(err.Error(), "git-wrangler init") {
		t.Fatalf("error = %v", err)
	}
}

func TestAuthenticateGitHubCancellationStopsPolling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var tokenRequests int
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/login/device/code") {
			return formResponse("device_code=device-code&user_code=user-code&verification_uri=https%3A%2F%2Fgithub.com%2Flogin%2Fdevice&expires_in=900&interval=5"), nil
		}
		tokenRequests++
		return formResponse("error=authorization_pending"), nil
	})}
	_, err := (GitHubDeviceAuthenticator{
		ClientID:   "client-id",
		HTTPClient: client,
		BrowseURL: func(string) error {
			cancel()
			return nil
		},
	}).AuthenticateGitHub(ctx, "github.com", strings.NewReader("\n"), io.Discard, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if tokenRequests != 0 {
		t.Fatalf("token requests = %d, want 0", tokenRequests)
	}
}

func TestWaitEventsDecreaseAndStop(t *testing.T) {
	var mu sync.Mutex
	var events []WaitEvent
	stop := emitWaitEvents(3, func(event WaitEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	time.Sleep(1100 * time.Millisecond)
	stop()
	mu.Lock()
	count := len(events)
	if count < 2 || events[0].Remaining <= events[count-1].Remaining {
		t.Fatalf("events = %#v", events)
	}
	mu.Unlock()
	time.Sleep(1100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(events) != count {
		t.Fatalf("events continued after stop: %#v", events)
	}
}

func TestHTTPClientDefaultsToNonNilClient(t *testing.T) {
	if got := (GitHubDeviceAuthenticator{}).httpClient(); got == nil {
		t.Fatal("expected default HTTP client")
	}
}

func successfulAuthClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/login/device/code"):
			return formResponse("device_code=device-code&user_code=user-code&verification_uri=https%3A%2F%2Fgithub.com%2Flogin%2Fdevice&expires_in=900&interval=0"), nil
		case strings.HasSuffix(req.URL.Path, "/login/oauth/access_token"):
			return formResponse("access_token=oauth-token&token_type=bearer&scope=repo%20read%3Aorg"), nil
		case req.URL.String() == "https://api.github.com/user":
			return jsonResponse(`{"login":"octo"}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})}
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
