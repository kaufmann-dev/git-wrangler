package cli

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/auth"
	"github.com/kaufmann-dev/git-wrangler/internal/ui"
)

type authorizationWait struct {
	mu          sync.Mutex
	writer      io.Writer
	interactive bool
	written     bool
}

func newAuthorizationWait(a *app) *authorizationWait {
	return &authorizationWait{
		writer:      a.stderr,
		interactive: ui.IsTerminal(a.stderr),
	}
}

func (w *authorizationWait) update(event auth.WaitEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.interactive && w.written {
		return
	}
	if w.interactive && w.written {
		fmt.Fprint(w.writer, "\r\x1b[J")
	}
	fmt.Fprintf(w.writer, "Waiting for GitHub authorization: %s remaining", formatRemaining(event.Remaining))
	if !w.interactive {
		fmt.Fprintln(w.writer)
	}
	w.written = true
}

func (w *authorizationWait) done() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.interactive && w.written {
		fmt.Fprint(w.writer, "\r\x1b[J")
	}
	w.written = false
}

func formatRemaining(remaining time.Duration) string {
	if remaining < 0 {
		remaining = 0
	}
	remaining = remaining.Round(time.Second)
	minutes := remaining / time.Minute
	seconds := remaining % time.Minute / time.Second
	if minutes > 0 {
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
