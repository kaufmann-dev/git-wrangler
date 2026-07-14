package trailers

import (
	"fmt"
	"strings"
	"unicode"
)

const coauthorKey = "co-authored-by"

// Identity is a validated Git identity in Name <email> form. Display preserves
// the caller-supplied spelling and casing after surrounding whitespace is
// removed.
type Identity struct {
	Display string
	Name    string
	Email   string
}

// Entry is one parsed trailer, including any folded continuation lines.
type Entry struct {
	Key   string
	Value string
	Raw   string
}

type segment struct {
	entry *Entry
	raw   string
}

// Message describes the final Git trailer block in Raw. A message without a
// blank line between its title/body and final paragraph has no trailer block,
// matching `git interpret-trailers --parse --no-divider`.
type Message struct {
	Raw      string
	Prefix   string
	Divider  string
	Trailing string
	Entries  []Entry
	segments []segment
	hasBlock bool
}

type messageLine struct {
	start int
	end   int
	text  string
}

// Parse locates and parses Git's final trailer paragraph without treating a
// patch divider specially. Raw trailer spelling, casing, order, and folding are
// retained in Entry.Raw.
func Parse(raw string) Message {
	parsed := Message{Raw: raw}
	lines := splitMessageLines(raw)
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last].text) == "" {
		last--
	}
	if last < 0 {
		return parsed
	}

	blank := last - 1
	for blank >= 0 && strings.TrimSpace(lines[blank].text) != "" {
		blank--
	}
	// Git never treats the title paragraph itself as a trailer block.
	if blank < 0 {
		return parsed
	}

	candidate := lines[blank+1 : last+1]
	segments, entries, valid := parseCandidate(candidate)
	if !valid {
		return parsed
	}

	prefixEnd := lines[blank].start
	for prefixEnd > 0 && (raw[prefixEnd-1] == '\n' || raw[prefixEnd-1] == '\r') {
		prefixEnd--
	}
	candidateStart := candidate[0].start
	contentEnd := candidate[len(candidate)-1].end
	parsed.Prefix = raw[:prefixEnd]
	parsed.Divider = raw[prefixEnd:candidateStart]
	parsed.Trailing = raw[contentEnd:]
	parsed.Entries = entries
	parsed.segments = segments
	parsed.hasBlock = true
	return parsed
}

func splitMessageLines(raw string) []messageLine {
	if raw == "" {
		return nil
	}
	lines := []messageLine{}
	start := 0
	for start <= len(raw) {
		newline := strings.IndexByte(raw[start:], '\n')
		end := len(raw)
		next := len(raw) + 1
		if newline >= 0 {
			end = start + newline
			next = end + 1
		}
		text := strings.TrimSuffix(raw[start:end], "\r")
		lines = append(lines, messageLine{start: start, end: end, text: text})
		if next > len(raw) {
			break
		}
		start = next
		if start == len(raw) {
			lines = append(lines, messageLine{start: start, end: start, text: ""})
			break
		}
	}
	return lines
}

func parseCandidate(lines []messageLine) ([]segment, []Entry, bool) {
	segments := []segment{}
	entries := []Entry{}
	trailerLines := 0
	nonBlankLines := 0
	signedOff := false
	allTrailerContent := true

	for i := 0; i < len(lines); {
		line := lines[i].text
		if strings.TrimSpace(line) != "" {
			nonBlankLines++
		}
		key, value, ok := parseTrailerLine(line)
		if !ok {
			allTrailerContent = false
			segments = append(segments, segment{raw: line})
			i++
			continue
		}

		rawLines := []string{line}
		values := []string{strings.TrimSpace(value)}
		trailerLines++
		if strings.EqualFold(key, "signed-off-by") {
			signedOff = true
		}
		i++
		for i < len(lines) && isContinuation(lines[i].text) {
			rawLines = append(rawLines, lines[i].text)
			values = append(values, strings.TrimSpace(lines[i].text))
			nonBlankLines++
			i++
		}
		entry := Entry{Key: key, Value: strings.TrimSpace(strings.Join(values, " ")), Raw: strings.Join(rawLines, "\n")}
		entries = append(entries, entry)
		entryCopy := entry
		segments = append(segments, segment{entry: &entryCopy, raw: entry.Raw})
	}

	valid := trailerLines > 0 && (allTrailerContent || (signedOff && trailerLines*4 >= nonBlankLines))
	return segments, entries, valid
}

func parseTrailerLine(line string) (string, string, bool) {
	separator := strings.IndexByte(line, ':')
	if separator <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:separator])
	if key == "" {
		return "", "", false
	}
	for _, r := range key {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == ':' {
			return "", "", false
		}
	}
	return key, line[separator+1:], true
}

func isContinuation(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

// ParseIdentity validates and preserves a Name <email> identity.
func ParseIdentity(value string) (Identity, error) {
	display := strings.TrimSpace(value)
	if display == "" || strings.ContainsAny(display, "\r\n") || !strings.HasSuffix(display, ">") {
		return Identity{}, fmt.Errorf("identity must be in Name <email> form")
	}
	open := strings.LastIndexByte(display, '<')
	if open <= 0 || open == len(display)-1 || !unicode.IsSpace(rune(display[open-1])) {
		return Identity{}, fmt.Errorf("identity must be in Name <email> form")
	}
	name := strings.TrimSpace(display[:open])
	email := display[open+1 : len(display)-1]
	if name == "" || strings.ContainsAny(name, "<>") {
		return Identity{}, fmt.Errorf("identity must include a name")
	}
	if err := ValidateEmail(email); err != nil {
		return Identity{}, err
	}
	return Identity{Display: display, Name: name, Email: email}, nil
}

// ValidateEmail validates an email selector without changing its display form.
func ValidateEmail(email string) error {
	if email == "" || email != strings.TrimSpace(email) || strings.ContainsAny(email, "<>\r\n\t ") {
		return fmt.Errorf("email must be a non-empty address")
	}
	if strings.Count(email, "@") != 1 {
		return fmt.Errorf("email must contain one @")
	}
	local, domain, _ := strings.Cut(email, "@")
	if local == "" || domain == "" {
		return fmt.Errorf("email must include text before and after @")
	}
	return nil
}

// ValidateIdentities parses identities and rejects duplicate emails without
// regard to case.
func ValidateIdentities(values []string) ([]Identity, error) {
	identities := make([]Identity, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		identity, err := ParseIdentity(value)
		if err != nil {
			return nil, fmt.Errorf("invalid coauthor %q: %w", value, err)
		}
		key := strings.ToLower(identity.Email)
		if seen[key] {
			return nil, fmt.Errorf("duplicate coauthor email %q", identity.Email)
		}
		seen[key] = true
		identities = append(identities, identity)
	}
	return identities, nil
}

// CoauthorLine returns the canonical spelling used for new and replaced
// coauthor trailers.
func CoauthorLine(identity Identity) string {
	return "Co-authored-by: " + identity.Display
}

func isCoauthor(entry Entry) bool {
	return strings.EqualFold(entry.Key, coauthorKey)
}

func entryIdentity(entry Entry) (Identity, bool) {
	if !isCoauthor(entry) {
		return Identity{}, false
	}
	identity, err := ParseIdentity(entry.Value)
	return identity, err == nil
}

// MergeGenerated appends the original valid trailer entries to a generated
// subject/body. When removeCoauthors is set, only Co-authored-by entries are
// omitted.
func MergeGenerated(oldMessage, generated string, removeCoauthors bool) string {
	parsed := Parse(oldMessage)
	rows := []string{}
	for _, entry := range parsed.Entries {
		if removeCoauthors && isCoauthor(entry) {
			continue
		}
		rows = append(rows, entry.Raw)
	}
	generated = strings.TrimSpace(generated)
	if len(rows) == 0 {
		return generated
	}
	return generated + "\n\n" + strings.Join(rows, "\n")
}

// AddCoauthors appends identities whose email is not already represented by a
// parseable coauthor trailer.
func AddCoauthors(message string, identities []Identity) (string, bool) {
	parsed := Parse(message)
	existing := map[string]bool{}
	for _, entry := range parsed.Entries {
		if identity, ok := entryIdentity(entry); ok {
			existing[strings.ToLower(identity.Email)] = true
		}
	}
	additions := []string{}
	for _, identity := range identities {
		key := strings.ToLower(identity.Email)
		if existing[key] {
			continue
		}
		existing[key] = true
		additions = append(additions, CoauthorLine(identity))
	}
	if len(additions) == 0 {
		return message, false
	}
	return parsed.render(nil, additions), true
}

// ReplaceCoauthor replaces matching oldEmail trailers. When the replacement
// email is already present, all matching old/replacement entries collapse into
// the explicitly supplied identity at the first affected position.
func ReplaceCoauthor(message, oldEmail string, replacement Identity) (string, bool) {
	parsed := Parse(message)
	oldKey := strings.ToLower(oldEmail)
	replacementKey := strings.ToLower(replacement.Email)
	hasOld := false
	for _, entry := range parsed.Entries {
		if identity, ok := entryIdentity(entry); ok && strings.ToLower(identity.Email) == oldKey {
			hasOld = true
			break
		}
	}
	if !hasOld {
		return message, false
	}
	inserted := false
	transform := func(entry Entry) (string, bool) {
		identity, ok := entryIdentity(entry)
		if !ok {
			return entry.Raw, true
		}
		key := strings.ToLower(identity.Email)
		if key != oldKey && key != replacementKey {
			return entry.Raw, true
		}
		if inserted {
			return "", false
		}
		inserted = true
		return CoauthorLine(replacement), true
	}
	return parsed.render(transform, nil), true
}

// RemoveCoauthors removes all coauthor trailers or only parseable identities
// whose emails match selectors case-insensitively.
func RemoveCoauthors(message string, emails []string, all bool) (string, bool) {
	parsed := Parse(message)
	selectors := map[string]bool{}
	for _, email := range emails {
		selectors[strings.ToLower(email)] = true
	}
	changed := false
	transform := func(entry Entry) (string, bool) {
		if !isCoauthor(entry) {
			return entry.Raw, true
		}
		remove := all
		if !remove {
			identity, ok := entryIdentity(entry)
			remove = ok && selectors[strings.ToLower(identity.Email)]
		}
		if remove {
			changed = true
			return "", false
		}
		return entry.Raw, true
	}
	result := parsed.render(transform, nil)
	if !changed {
		return message, false
	}
	return result, true
}

func (parsed Message) render(transform func(Entry) (string, bool), additions []string) string {
	if !parsed.hasBlock {
		if len(additions) == 0 {
			return parsed.Raw
		}
		base := strings.TrimRight(parsed.Raw, "\r\n")
		if base == "" {
			return strings.Join(additions, "\n")
		}
		return base + "\n\n" + strings.Join(additions, "\n")
	}

	rows := []string{}
	for _, segment := range parsed.segments {
		if segment.entry == nil || transform == nil {
			rows = append(rows, segment.raw)
			continue
		}
		if raw, keep := transform(*segment.entry); keep {
			rows = append(rows, raw)
		}
	}
	rows = append(rows, additions...)
	if len(rows) == 0 {
		return parsed.Prefix + parsed.Trailing
	}
	return parsed.Prefix + parsed.Divider + strings.Join(rows, "\n") + parsed.Trailing
}
