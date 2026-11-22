package score

import (
	"math"
	"strings"
	"unicode"
)

// stopwords are ignored when comparing two texts.
var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "that": true, "this": true, "with": true,
	"from": true, "into": true, "your": true, "you": true, "are": true, "was": true,
	"ada": true, "adalah": true, "yang": true, "dan": true, "di": true, "dari": true,
	"untuk": true, "pada": true, "ke": true, "itu": true, "ini": true, "atau": true,
	"dengan": true, "sudah": true, "belum": true,
}

// Tokens splits text into lowercase word tokens, dropping stopwords and
// short words.
func Tokens(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	seen := map[string]bool{}
	var out []string
	for _, f := range fields {
		if len(f) < 3 || stopwords[f] || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// Similarity returns the Jaccard similarity of two texts, from 0 to 1.
func Similarity(a, b string) float64 {
	return SetSimilarity(TokenSet(a), TokenSet(b))
}

// TokenSet turns text into a set of unique tokens. Callers that compare many
// pairs build the sets once instead of re-tokenizing the same text per pair.
func TokenSet(s string) map[string]bool {
	set := make(map[string]bool)
	for _, t := range Tokens(s) {
		set[t] = true
	}
	return set
}

// SetSimilarity is the Jaccard similarity of two token sets.
func SetSimilarity(setA, setB map[string]bool) float64 {
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}
	inter := 0
	for t := range setA {
		if setB[t] {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// Containment returns how much of the shorter text appears in the longer one.
// It catches the case where a new item restates an old one with extra detail.
func Containment(a, b string) float64 {
	return SetContainment(TokenSet(a), TokenSet(b))
}

// SetContainment is the containment of two token sets.
func SetContainment(setA, setB map[string]bool) float64 {
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}
	short, long := setA, setB
	if len(long) < len(short) {
		short, long = long, short
	}
	hits := 0
	for t := range short {
		if long[t] {
			hits++
		}
	}
	return float64(hits) / float64(len(short))
}

// DuplicateScore combines similarity and containment into one number that
// tells how likely two texts say the same thing.
func DuplicateScore(a, b string) float64 {
	return SetDuplicateScore(TokenSet(a), TokenSet(b))
}

// SetDuplicateScore is DuplicateScore for pre-built token sets.
func SetDuplicateScore(setA, setB map[string]bool) float64 {
	return math.Max(SetSimilarity(setA, setB), SetContainment(setA, setB)*0.9)
}
