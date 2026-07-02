package ui

import (
	"bytes"
	"testing"
)

func TestParseOSC11Reply(t *testing.T) {
	t.Parallel()
	tests := []struct {
		reply string
		light bool
		ok    bool
	}{
		{reply: "\x1b]11;rgb:ffff/ffff/ffff\a", light: true, ok: true},
		{reply: "\x1b]11;rgb:ffff/ffff/ffff\x1b\\", light: true, ok: true},
		{reply: "\x1b]11;rgb:1616/1b1b/2222\a", light: false, ok: true},
		{reply: "\x1b]11;rgb:ff/ff/ff\a", light: true, ok: true},
		{reply: "\x1b]11;rgb:0/0/0\a", light: false, ok: true},
		{reply: "\x1b]11;rgb:ffff/ffff\a", ok: false},
		{reply: "\x1b]11;#ffffff\a", ok: false},
		{reply: "garbage", ok: false},
		{reply: "\x1b]11;rgb:zzzz/ffff/ffff\a", ok: false},
	}
	for _, tc := range tests {
		r, g, b, ok := parseOSC11Reply(tc.reply)
		if ok != tc.ok {
			t.Fatalf("parseOSC11Reply(%q) ok = %v, want %v", tc.reply, ok, tc.ok)
		}
		if !ok {
			continue
		}
		light := 0.2126*r+0.7152*g+0.0722*b >= 0.5
		if light != tc.light {
			t.Fatalf("parseOSC11Reply(%q) = %v/%v/%v, light = %v, want %v", tc.reply, r, g, b, light, tc.light)
		}
	}
}

func TestColorfgbgDark(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value string
		dark  bool
		ok    bool
	}{
		{value: "15;0", dark: true, ok: true},
		{value: "0;15", dark: false, ok: true},
		{value: "12;8", dark: true, ok: true},
		{value: "0;default;7", dark: false, ok: true},
		{value: "default;default", ok: false},
		{value: "15", ok: false},
		{value: "", ok: false},
		{value: "0;9", ok: false},
	}
	for _, tc := range tests {
		dark, ok := colorfgbgDark(tc.value)
		if ok != tc.ok || (ok && dark != tc.dark) {
			t.Fatalf("colorfgbgDark(%q) = %v, %v, want %v, %v", tc.value, dark, ok, tc.dark, tc.ok)
		}
	}
}

func TestDarkBackgroundDefaultsToDarkForNonTerminal(t *testing.T) {
	queried := false
	restore := queryTerminalBackground
	queryTerminalBackground = func() (string, bool) {
		queried = true
		return "", false
	}
	defer func() { queryTerminalBackground = restore }()
	if !DarkBackground(&bytes.Buffer{}) {
		t.Fatal("non-terminal stream should default to dark")
	}
	if queried {
		t.Fatal("non-terminal stream must not query the terminal")
	}
}

func TestDetectDarkBackgroundFallbackChain(t *testing.T) {
	restore := queryTerminalBackground
	defer func() { queryTerminalBackground = restore }()

	queryTerminalBackground = func() (string, bool) { return "\x1b]11;rgb:ffff/ffff/ffff\a", true }
	if detectDarkBackground() {
		t.Fatal("white OSC 11 reply should detect light background")
	}

	queryTerminalBackground = func() (string, bool) { return "", false }
	t.Setenv("COLORFGBG", "0;15")
	if detectDarkBackground() {
		t.Fatal("COLORFGBG light background should detect light")
	}

	t.Setenv("COLORFGBG", "")
	if !detectDarkBackground() {
		t.Fatal("undetectable background should default to dark")
	}
}
