package conventional

import (
	"strings"
	"testing"
)

func TestParseValidSubjects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		subject  string
		typ      string
		scope    string
		breaking bool
		parsed   string
	}{
		{subject: "feat: add thing", typ: "feat", parsed: "add thing"},
		{subject: "fix(cli): handle case", typ: "fix", scope: "cli", parsed: "handle case"},
		{subject: "feat!: change contract", typ: "feat", breaking: true, parsed: "change contract"},
		{subject: "refactor(core)!: replace flow", typ: "refactor", scope: "core", breaking: true, parsed: "replace flow"},
	}
	for _, tc := range tests {
		got := Parse(tc.subject)
		if !got.Conventional || got.Type != tc.typ || got.Scope != tc.scope || got.Breaking != tc.breaking || got.Subject != tc.parsed {
			t.Fatalf("Parse(%q) = %#v", tc.subject, got)
		}
		if !ValidSubject(tc.subject) {
			t.Fatalf("ValidSubject(%q) = false", tc.subject)
		}
	}
}

func TestParseInvalidSubjectsFallBackToOther(t *testing.T) {
	t.Parallel()
	tests := []string{
		"feature: add thing",
		"feat add thing",
		"feat: ",
		"feat(): add thing",
		strings.Repeat("a", 121),
		"feat: add thing\nbody",
	}
	for _, subject := range tests {
		got := Parse(subject)
		if got.Conventional || got.Type != "other" || got.Subject != subject {
			t.Fatalf("Parse(%q) = %#v, want non-conventional other", subject, got)
		}
		if ValidSubject(subject) {
			t.Fatalf("ValidSubject(%q) = true", subject)
		}
	}
}

func TestIsConventionalMessageUsesTrimmedFirstLine(t *testing.T) {
	t.Parallel()
	if !IsConventionalMessage("  feat(cli): add thing\n\nbody") {
		t.Fatal("expected first line to be conventional")
	}
	if IsConventionalMessage("not conventional\nfeat: later") {
		t.Fatal("expected non-conventional first line")
	}
}
