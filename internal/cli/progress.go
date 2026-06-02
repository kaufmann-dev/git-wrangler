package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/kaufmann-dev/git-wrangler/internal/ui"
	"golang.org/x/term"
)

type progress struct {
	mu          sync.Mutex
	writer      io.Writer
	interactive bool
	label       string
	total       int
	current     int
	lastWidth   int
	closed      bool
}

var termGetSize = term.GetSize

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

func (p *progress) log(message string) {
	if p == nil || message == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if p.interactive {
		if p.lastWidth > 0 {
			fmt.Fprintf(p.writer, "\r%s\r", strings.Repeat(" ", p.lastWidth))
		}
		fmt.Fprintln(p.writer, message)
		if p.current > 0 {
			p.write("")
		}
		return
	}
	fmt.Fprintln(p.writer, message)
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
		prefix := fmt.Sprintf("%s: [%s] %d/%d ", p.label, p.bar(20), p.current, p.total)
		w := 0
		if file, ok := p.writer.(*os.File); ok {
			if tw, _, err := termGetSize(int(file.Fd())); err == nil && tw > 0 {
				w = tw
			}
		}

		if w > 0 {
			// Leave a safety margin of at least 1 column.
			maxDetailWidth := w - len(prefix) - 1
			if maxDetailWidth < 0 {
				maxDetailWidth = 0
			}
			detail = truncateToVisibleWidth(detail, maxDetailWidth, "\033[0m")

			if p.lastWidth > w-1 {
				p.lastWidth = w - 1
			}
		}

		line := prefix + detail
		if len(line) < p.lastWidth {
			line += strings.Repeat(" ", p.lastWidth-len(line))
		}
		p.lastWidth = len(line)
		fmt.Fprintf(p.writer, "\r%s", line)
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

func visibleWidth(s string) int {
	inSeq := false
	width := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inSeq = true
			continue
		}
		if inSeq {
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				inSeq = false
			}
			continue
		}
		width++
	}
	return width
}

func truncateToVisibleWidth(s string, maxW int, resetCode string) string {
	if maxW <= 0 {
		return ""
	}
	inSeq := false
	hasSeq := false
	width := 0
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inSeq = true
			hasSeq = true
			sb.WriteByte(s[i])
			continue
		}
		if inSeq {
			sb.WriteByte(s[i])
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				inSeq = false
			}
			continue
		}
		if width >= maxW {
			if hasSeq && resetCode != "" {
				sb.WriteString(resetCode)
			}
			break
		}
		sb.WriteByte(s[i])
		width++
	}
	return sb.String()
}
