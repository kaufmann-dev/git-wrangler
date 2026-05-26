package ui

import (
	"bytes"
	"testing"
)

func TestColorDisabledByNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "1")
	theme := New(&bytes.Buffer{})
	if theme.Red != "" || theme.Reset != "" {
		t.Fatalf("expected no color with NO_COLOR, got red=%q reset=%q", theme.Red, theme.Reset)
	}
}

func TestColorForced(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CLICOLOR_FORCE", "1")
	theme := New(&bytes.Buffer{})
	if theme.Red == "" || theme.Reset == "" {
		t.Fatal("expected forced color")
	}
}
