package cli

import (
	"fmt"
	"io"
	"sync"

	"github.com/kaufmann-dev/git-wrangler/internal/ui"
)

type progress struct {
	mu          sync.Mutex
	writer      io.Writer
	interactive bool
	label       string
	total       int
	current     int
	closed      bool
}

func newProgress(a *app, label string, total int) *progress {
	if total <= 1 {
		return nil
	}
	return &progress{
		writer:      a.stderr,
		interactive: ui.IsTerminal(a.stderr),
		label:       label,
		total:       total,
	}
}

func (p *progress) advance(detail string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.current++
	if p.shouldWrite() {
		p.write(detail)
	}
}

func (p *progress) message(detail string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.write(detail)
}

func (p *progress) done() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if p.interactive {
		fmt.Fprintln(p.writer)
	}
	p.closed = true
}

func (p *progress) write(detail string) {
	if p.interactive {
		fmt.Fprintf(p.writer, "\r%s: [%s] %d/%d %s", p.label, p.bar(20), p.current, p.total, detail)
		return
	}
	if detail == "" {
		fmt.Fprintf(p.writer, "%s: %d/%d\n", p.label, p.current, p.total)
	} else {
		fmt.Fprintf(p.writer, "%s: %d/%d %s\n", p.label, p.current, p.total, detail)
	}
}

func (p *progress) shouldWrite() bool {
	return p.interactive || p.current == 1 || p.current == p.total || p.current%10 == 0
}

func (p *progress) bar(width int) string {
	filled := p.current * width / p.total
	bar := make([]byte, width)
	for i := range bar {
		if i < filled {
			bar[i] = '#'
		} else {
			bar[i] = '-'
		}
	}
	return string(bar)
}
