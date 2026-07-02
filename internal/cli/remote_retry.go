package cli

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
)

const remoteRetryAttempts = 3

var remoteRetryBackoffs = []time.Duration{
	100 * time.Millisecond,
	300 * time.Millisecond,
}

func captureRemoteGitWithRetry(a *app, dir string, env []string, args ...string) (string, error) {
	return retryTransientRemote(a.ctx, func() (string, error) {
		return a.git.CaptureRemote(a.ctx, dir, env, args...)
	})
}

func stdoutGitHubWithRetry(a *app, dir string, env []string, args ...string) (string, error) {
	return retryTransientRemote(a.ctx, func() (string, error) {
		return a.gh.StdoutEnv(a.ctx, dir, env, args...)
	})
}

func retryTransientRemote(ctx context.Context, operation func() (string, error)) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var out string
	var err error
	for attempt := 1; attempt <= remoteRetryAttempts; attempt++ {
		out, err = operation()
		if err == nil || ctx.Err() != nil || !isTransientRemoteFailure(out, err) || attempt == remoteRetryAttempts {
			return out, err
		}
		if !waitRemoteRetry(ctx, attempt) {
			return out, ctx.Err()
		}
	}
	return out, err
}

func waitRemoteRetry(ctx context.Context, attempt int) bool {
	backoff := time.Duration(0)
	index := attempt - 1
	if index >= 0 && index < len(remoteRetryBackoffs) {
		backoff = remoteRetryBackoffs[index]
	}
	if backoff <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func isTransientRemoteFailure(output string, err error) bool {
	if err == nil {
		return false
	}
	var timeout git.RemoteTimeoutError
	if errors.As(err, &timeout) {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(output + " " + err.Error()))
	if message == "" {
		return false
	}
	for _, marker := range []string{
		"bad gateway",
		"connection aborted",
		"connection refused",
		"connection reset",
		"connection timed out",
		"could not connect to server",
		"could not resolve host",
		"early eof",
		"failed to connect",
		"gateway timeout",
		"http 500",
		"http 502",
		"http 503",
		"http 504",
		"i/o timeout",
		"name resolution",
		"network is unreachable",
		"operation timed out",
		"remote end hung up unexpectedly",
		"service unavailable",
		"temporary failure",
		"the remote end hung up unexpectedly",
		"tls handshake timeout",
		"unexpected eof",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
