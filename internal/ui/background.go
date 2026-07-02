package ui

import (
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

var (
	backgroundOnce sync.Once
	backgroundDark bool

	queryTerminalBackground = queryOSC11
)

// DarkBackground reports whether the terminal background is dark. Detection
// runs at most once per process; non-terminal streams and undetectable
// terminals default to dark.
func DarkBackground(stdout io.Writer) bool {
	if !IsTerminal(stdout) {
		return true
	}
	backgroundOnce.Do(func() {
		backgroundDark = detectDarkBackground()
	})
	return backgroundDark
}

func detectDarkBackground() bool {
	if reply, ok := queryTerminalBackground(); ok {
		if r, g, b, ok := parseOSC11Reply(reply); ok {
			return 0.2126*r+0.7152*g+0.0722*b < 0.5
		}
	}
	if dark, ok := colorfgbgDark(os.Getenv("COLORFGBG")); ok {
		return dark
	}
	return true
}

func queryOSC11() (string, bool) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", false
	}
	defer tty.Close()
	if err := tty.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		return "", false
	}
	fd := int(tty.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return "", false
	}
	defer term.Restore(fd, state)
	if _, err := tty.WriteString("\x1b]11;?\x1b\\"); err != nil {
		return "", false
	}
	reply := make([]byte, 0, 64)
	buf := make([]byte, 1)
	for {
		n, err := tty.Read(buf)
		if err != nil {
			return "", false
		}
		if n == 0 {
			continue
		}
		reply = append(reply, buf[0])
		if buf[0] == '\a' {
			return string(reply), true
		}
		if len(reply) >= 2 && buf[0] == '\\' && reply[len(reply)-2] == '\x1b' {
			return string(reply), true
		}
		if len(reply) > 128 {
			return "", false
		}
	}
}

func parseOSC11Reply(reply string) (r, g, b float64, ok bool) {
	start := strings.Index(reply, "rgb:")
	if start < 0 {
		return 0, 0, 0, false
	}
	value := reply[start+len("rgb:"):]
	value = strings.TrimSuffix(value, "\a")
	value = strings.TrimSuffix(value, "\x1b\\")
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	channels := [3]float64{}
	for i, part := range parts {
		if len(part) < 1 || len(part) > 4 {
			return 0, 0, 0, false
		}
		parsed, err := strconv.ParseUint(part, 16, 16)
		if err != nil {
			return 0, 0, 0, false
		}
		max := float64(uint64(1)<<(4*len(part))) - 1
		channels[i] = float64(parsed) / max
	}
	return channels[0], channels[1], channels[2], true
}

func colorfgbgDark(value string) (dark, ok bool) {
	parts := strings.Split(value, ";")
	if len(parts) < 2 {
		return false, false
	}
	background, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return false, false
	}
	switch {
	case background >= 0 && background <= 6 || background == 8:
		return true, true
	case background == 7 || background == 15:
		return false, true
	default:
		return false, false
	}
}
