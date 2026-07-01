package conventional

import (
	"strings"
)

var allowedTypes = map[string]struct{}{
	"feat":     {},
	"fix":      {},
	"docs":     {},
	"style":    {},
	"refactor": {},
	"test":     {},
	"chore":    {},
	"perf":     {},
	"ci":       {},
	"build":    {},
	"revert":   {},
}

type Commit struct {
	Type         string
	Scope        string
	Breaking     bool
	Subject      string
	Conventional bool
}

func AllowedTypes() []string {
	return []string{"feat", "fix", "docs", "style", "refactor", "test", "chore", "perf", "ci", "build", "revert"}
}

func Parse(subject string) Commit {
	commit, ok := parse(subject)
	commit.Conventional = ok
	if !ok {
		commit.Type = "other"
		commit.Subject = subject
	}
	return commit
}

func ValidSubject(subject string) bool {
	_, ok := parse(subject)
	return ok
}

func IsConventionalMessage(message string) bool {
	first := firstLine(strings.TrimSpace(message))
	return ValidSubject(first)
}

func IsAllowedType(commitType string) bool {
	_, ok := allowedTypes[commitType]
	return ok
}

func parse(subject string) (Commit, bool) {
	if subject == "" || len(subject) > 120 || strings.ContainsAny(subject, "\r\n") {
		return Commit{}, false
	}
	prefix, parsedSubject, ok := strings.Cut(subject, ": ")
	if !ok || parsedSubject == "" {
		return Commit{}, false
	}
	breaking := strings.HasSuffix(prefix, "!")
	if breaking {
		prefix = strings.TrimSuffix(prefix, "!")
	}
	commitType := prefix
	scope := ""
	if open := strings.IndexByte(prefix, '('); open >= 0 {
		if !strings.HasSuffix(prefix, ")") {
			return Commit{}, false
		}
		commitType = prefix[:open]
		scope = prefix[open+1 : len(prefix)-1]
		if scope == "" || strings.Contains(scope, ")") {
			return Commit{}, false
		}
	}
	if !IsAllowedType(commitType) {
		return Commit{}, false
	}
	return Commit{
		Type:         commitType,
		Scope:        scope,
		Breaking:     breaking,
		Subject:      parsedSubject,
		Conventional: true,
	}, true
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
