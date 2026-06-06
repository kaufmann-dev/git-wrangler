package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/auth"
)

func TestAuthorizationWaitInteractiveIsTransientAndClearsBeforeOutput(t *testing.T) {
	var stderr bytes.Buffer
	wait := &authorizationWait{writer: &stderr, interactive: true}
	wait.update(auth.WaitEvent{Remaining: 2 * time.Minute})
	wait.update(auth.WaitEvent{Remaining: 119 * time.Second})
	wait.done()
	stderr.WriteString("Authorized\n")

	out := stderr.String()
	if !strings.Contains(out, "Waiting for GitHub authorization: 2m00s remaining") || !strings.Contains(out, "Waiting for GitHub authorization: 1m59s remaining") {
		t.Fatalf("missing countdown updates:\n%q", out)
	}
	if !strings.Contains(out, "\r\x1b[JAuthorized\n") {
		t.Fatalf("countdown was not cleared before output:\n%q", out)
	}
}

func TestAuthorizationWaitNonInteractiveWritesOneLine(t *testing.T) {
	var stderr bytes.Buffer
	wait := &authorizationWait{writer: &stderr}
	wait.update(auth.WaitEvent{Remaining: 2 * time.Minute})
	wait.update(auth.WaitEvent{Remaining: 119 * time.Second})
	wait.done()

	if got := stderr.String(); got != "Waiting for GitHub authorization: 2m00s remaining\n" {
		t.Fatalf("output = %q", got)
	}
}
