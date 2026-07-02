package ui

import (
	"io"
	"os"

	"golang.org/x/term"
)

type Theme struct {
	Red        string
	Green      string
	Yellow     string
	Blue       string
	Cyan       string
	Muted      string
	Bold       string
	Reset      string
	Dark       bool
	RepoColor  string
	OKSymbol   string
	WarnSymbol string
	ErrSymbol  string
	InfoSymbol string
	StepSymbol string
	SkipSymbol string
}

func New(stdout io.Writer) Theme {
	t := Theme{}
	color := ColorEnabled(stdout)
	if color {
		t.Red = "\033[31m"
		t.Green = "\033[32m"
		t.Yellow = "\033[33m"
		t.Blue = "\033[34m"
		t.Cyan = "\033[36m"
		t.Muted = "\033[2m"
		t.Bold = "\033[1m"
		t.Reset = "\033[0m"
		t.Dark = DarkBackground(stdout)
	}
	t.RepoColor = t.Bold + t.Blue
	if UnicodeEnabled(stdout) {
		t.OKSymbol = "✔"
		t.WarnSymbol = "⚠"
		t.ErrSymbol = "✖"
		t.InfoSymbol = "ℹ"
		t.StepSymbol = "▸"
		t.SkipSymbol = "↷"
	} else {
		t.OKSymbol = "OK"
		t.WarnSymbol = "WARN"
		t.ErrSymbol = "ERROR"
		t.InfoSymbol = "INFO"
		t.StepSymbol = ">"
		t.SkipSymbol = "SKIP"
	}
	return t
}

func ColorEnabled(stdout io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("CLICOLOR") == "0" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if force := os.Getenv("CLICOLOR_FORCE"); force != "" && force != "0" {
		return true
	}
	return IsTerminal(stdout)
}

func UnicodeEnabled(stdout io.Writer) bool {
	return os.Getenv("TERM") != "dumb" && IsTerminal(stdout)
}

func IsTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}
