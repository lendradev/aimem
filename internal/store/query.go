package store

import (
	"strings"
)

// NormalizeQuery lowercases, strips punctuation noise, and returns unique
// terms in order of first appearance.
func NormalizeQuery(q string) []string {
	q = strings.ToLower(q)
	fields := Tokenize(q)
	seen := map[string]bool{}
	var out []string
	for _, f := range fields {
		if len(f) < 2 && !isDigit(f) {
			continue
		}
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

func isDigit(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// FTS5Match builds a safe MATCH expression from free-form user text.
// Every term is double quoted so operator characters in user input cannot
// reach the FTS5 parser as syntax.
//
// Terms written in a script without word separators are matched as a whole
// run, because the FTS5 tokenizer treats such a run as a single token.
func FTS5Match(q string) string {
	terms := NormalizeQuery(q)
	var kept []string
	for _, t := range terms {
		if IsStopword(t, "") {
			continue
		}
		kept = append(kept, t)
	}
	if len(kept) == 0 {
		kept = terms
	}
	if len(kept) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(kept))
	for _, t := range kept {
		quoted = append(quoted, `"`+strings.ReplaceAll(t, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " OR ")
}
