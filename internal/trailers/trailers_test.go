package trailers

import (
	"os/exec"
	"strings"
	"testing"
)

func TestParsePreservesTrailerOrderCasingAndFolding(t *testing.T) {
	message := "subject\n\nbody\n\nreviewed-BY: A <a@example.test>\nCo-AUTHORED-by: B <b@example.test>\nToken: first\n second line\n\tthird line"
	parsed := Parse(message)
	if len(parsed.Entries) != 3 {
		t.Fatalf("entries = %#v", parsed.Entries)
	}
	if parsed.Entries[0].Raw != "reviewed-BY: A <a@example.test>" || parsed.Entries[1].Key != "Co-AUTHORED-by" {
		t.Fatalf("raw trailers changed: %#v", parsed.Entries)
	}
	if parsed.Entries[2].Value != "first second line third line" || parsed.Entries[2].Raw != "Token: first\n second line\n\tthird line" {
		t.Fatalf("folded trailer = %#v", parsed.Entries[2])
	}
}

func TestParseRejectsBodyMatchesAndNoDividerParagraphs(t *testing.T) {
	for _, message := range []string{
		"fix: ordinary conventional subject",
		"subject\nCo-authored-by: A <a@example.test>",
		"subject\n\nKey: body prose\nmore body prose",
		"subject\n\n---\nToken: value",
	} {
		if entries := Parse(message).Entries; len(entries) != 0 {
			t.Fatalf("Parse(%q) entries = %#v", message, entries)
		}
	}
}

func TestParseMatchesGitInterpretTrailersNoDivider(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	for _, message := range []string{
		"subject\n\nSigned-off-by: A <a@example.test>\nCo-authored-by: B <b@example.test>\n",
		"subject\n\nToken: first\n second\nOther: value\n",
		"subject\n\nKey: body prose\nmore body prose\n",
		"subject\n\n---\nToken: value\n",
	} {
		cmd := exec.Command("git", "interpret-trailers", "--parse", "--no-divider")
		cmd.Stdin = strings.NewReader(message)
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		gotRows := []string{}
		for _, entry := range Parse(message).Entries {
			gotRows = append(gotRows, entry.Key+": "+entry.Value)
		}
		got := strings.Join(gotRows, "\n")
		want := strings.TrimSpace(string(out))
		if got != want {
			t.Fatalf("message %q\ngot:  %q\nwant: %q", message, got, want)
		}
	}
}

func TestIdentityValidation(t *testing.T) {
	identity, err := ParseIdentity("  Jane Q. Doe <Jane@Example.test>  ")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Display != "Jane Q. Doe <Jane@Example.test>" || identity.Email != "Jane@Example.test" {
		t.Fatalf("identity = %#v", identity)
	}
	for _, value := range []string{"Jane", "<jane@example.test>", "Jane <>", "Jane <not-an-email>", "Jane<jane@example.test>"} {
		if _, err := ParseIdentity(value); err == nil {
			t.Fatalf("ParseIdentity(%q) succeeded", value)
		}
	}
	if _, err := ValidateIdentities([]string{"A <Same@Example.test>", "B <same@example.test>"}); err == nil {
		t.Fatal("case-insensitive duplicate email was accepted")
	}
}

func TestCoauthorMutationsPreserveUnrelatedTrailers(t *testing.T) {
	message := "subject\n\nReviewed-by: Reviewer <reviewer@example.test>\nco-AUTHORED-BY: Old Name <old@example.test>\nToken: first\n folded"
	added, changed := AddCoauthors(message, []Identity{{Display: "New Name <new@example.test>", Email: "new@example.test"}})
	if !changed || !strings.HasSuffix(added, "Co-authored-by: New Name <new@example.test>") {
		t.Fatalf("add result = %q", added)
	}
	replaced, changed := ReplaceCoauthor(added, "OLD@EXAMPLE.TEST", Identity{Display: "Current Name <new@example.test>", Email: "new@example.test"})
	if !changed || strings.Count(strings.ToLower(replaced), "new@example.test") != 1 || !strings.Contains(replaced, "Co-authored-by: Current Name <new@example.test>") {
		t.Fatalf("replace result = %q", replaced)
	}
	removed, changed := RemoveCoauthors(replaced, []string{"NEW@EXAMPLE.TEST"}, false)
	if !changed || strings.Contains(strings.ToLower(removed), "co-authored-by") {
		t.Fatalf("remove result = %q", removed)
	}
	if !strings.Contains(removed, "Reviewed-by: Reviewer <reviewer@example.test>") || !strings.Contains(removed, "Token: first\n folded") {
		t.Fatalf("unrelated trailers changed: %q", removed)
	}
}

func TestRemoveAllDropsMalformedCoauthors(t *testing.T) {
	message := "subject\n\nCo-authored-by: not an identity\nSigned-off-by: A <a@example.test>"
	got, changed := RemoveCoauthors(message, nil, true)
	if !changed || strings.Contains(got, "Co-authored-by") || !strings.Contains(got, "Signed-off-by") {
		t.Fatalf("result = %q", got)
	}
}

func TestMergeGeneratedPreservesOrRemovesOnlyCoauthors(t *testing.T) {
	old := "old subject\n\nold body\n\nSigned-off-by: A <a@example.test>\nco-authored-BY: B <b@example.test>"
	got := MergeGenerated(old, "feat(cli): new subject\n\nnew body", false)
	want := "feat(cli): new subject\n\nnew body\n\nSigned-off-by: A <a@example.test>\nco-authored-BY: B <b@example.test>"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	got = MergeGenerated(old, "feat(cli): new subject", true)
	if got != "feat(cli): new subject\n\nSigned-off-by: A <a@example.test>" {
		t.Fatalf("remove-coauthors result = %q", got)
	}
}
