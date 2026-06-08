package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
)

func renderRemoteGitFailure(a *app, r repo, operation, output string, err error) {
	if timeoutMessage, ok := remoteTimeoutMessage(err); ok {
		renderErrorBlock(a, fmt.Sprintf("%s: %s", r.display, timeoutMessage), output)
		return
	}
	renderErrorBlock(a, fmt.Sprintf("%s: git %s failed", r.display, operation), outputOrError(output, err))
}

func remoteGitFailureMessage(operation, output string, err error) string {
	if timeoutMessage, ok := remoteTimeoutMessage(err); ok {
		output = strings.TrimSpace(output)
		if output == "" {
			return timeoutMessage
		}
		return timeoutMessage + ": " + output
	}
	return fmt.Sprintf("git %s failed: %s", operation, outputOrError(output, err))
}

func remoteTimeoutMessage(err error) (string, bool) {
	var timeout git.RemoteTimeoutError
	if errors.As(err, &timeout) {
		return timeout.Error(), true
	}
	return "", false
}
