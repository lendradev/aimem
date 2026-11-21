package store

import (
	"strings"
	"unicode"
)

// Script identifies the writing system of a text. It is used to pick a
// tokenizer strategy, because the FTS5 unicode61 tokenizer does not segment
// scripts that are written without word separators.
type Script int

const (
	ScriptMixed Script = iota
	ScriptLatin
	ScriptCJK
	ScriptThai
	ScriptCyrillic
	ScriptArabic
	ScriptHebrew
	ScriptGreek
)

// DetectScript reports the dominant script of a text.
func DetectScript(s string) Script {
	var counts [8]int
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			counts[ScriptLatin]++
		case r >= 0x4E00 && r <= 0x9FFF, r >= 0x3040 && r <= 0x30FF, r >= 0xAC00 && r <= 0xD7AF:
			counts[ScriptCJK]++
		case r >= 0x0E00 && r <= 0x0E7F:
			counts[ScriptThai]++
		case r >= 0x0400 && r <= 0x04FF:
			counts[ScriptCyrillic]++
		case r >= 0x0600 && r <= 0x06FF:
			counts[ScriptArabic]++
		case r >= 0x0590 && r <= 0x05FF:
			counts[ScriptHebrew]++
		case r >= 0x0370 && r <= 0x03FF:
			counts[ScriptGreek]++
		}
	}
	best := ScriptMixed
	bestCount := 0
	for i, c := range counts {
		if c > bestCount {
			bestCount = c
			best = Script(i)
		}
	}
	return best
}

// NeedsSegmentation reports whether the text is written without word
// separators. The unicode61 tokenizer treats a whole run as one token, so
// queries for such text need the whole run to match.
func NeedsSegmentation(s string) bool {
	switch DetectScript(s) {
	case ScriptCJK, ScriptThai:
		return true
	}
	return false
}

// stopwordsByLanguage holds common function words for the languages this tool
// is used with. Keeping them small and specific avoids dropping meaningful
// words in languages the tool does not know well.
var stopwordsByLanguage = map[string][]string{
	"en": {"the", "and", "for", "that", "this", "with", "from", "into", "your", "you", "are", "was", "how", "what", "when", "where", "why", "does", "did", "can", "not", "but", "all", "any", "of", "on", "in", "at", "to", "is", "be", "as", "by", "it", "an", "or"},
	"id": {"ada", "adalah", "yang", "dan", "di", "dari", "untuk", "pada", "ke", "itu", "ini", "atau", "dengan", "cara", "bagaimana", "kenapa", "mengapa", "apakah", "sudah", "belum", "harus", "bisa", "kalau", "jika", "akan", "tidak", "oleh", "dalam", "sebagai", "juga", "hanya", "lebih", "sama", "saya", "kamu", "kita", "mereka", "para", "suatu", "sebuah", "dapat", "perlu", "agar", "supaya"},
	"de": {"der", "die", "das", "und", "ist", "nicht", "mit", "für", "auf", "von", "den", "dem", "ein", "eine", "auch", "sich", "dass", "werden", "wird", "wie", "was", "wenn", "aber", "oder"},
	"fr": {"le", "la", "les", "un", "une", "des", "et", "est", "pour", "dans", "que", "qui", "pas", "plus", "avec", "sur", "cette", "comment", "quoi", "mais", "ou"},
	"es": {"el", "la", "los", "las", "un", "una", "y", "es", "para", "con", "que", "no", "más", "como", "pero", "o", "del", "por", "este", "esta"},
	"pt": {"o", "a", "os", "as", "um", "uma", "e", "é", "para", "com", "que", "não", "mais", "como", "mas", "ou", "do", "da", "por", "este", "esta"},
	"it": {"il", "lo", "la", "gli", "le", "un", "una", "e", "è", "per", "con", "che", "non", "più", "come", "ma", "o", "del", "della", "questo", "questa"},
	"nl": {"de", "het", "een", "en", "is", "niet", "met", "voor", "op", "van", "dat", "die", "te", "zijn", "met", "hoe", "wat", "maar", "of"},
	"ja": {"の", "に", "は", "を", "た", "が", "で", "て", "と", "し", "れ", "さ", "ある", "いる", "も", "する", "から", "な", "こと", "として", "い", "や", "れる", "など", "なっ", "ない", "この", "ため", "その"},
	"zh": {"的", "了", "和", "是", "在", "有", "我", "他", "这", "那", "个", "们", "不", "也", "就", "与", "对", "到", "说", "要", "去", "会", "着", "没", "看", "好", "还", "这", "那", "什么", "怎么"},
}

var allStopwords = map[string]bool{}

func init() {
	for _, words := range stopwordsByLanguage {
		for _, w := range words {
			allStopwords[w] = true
		}
	}
}

// Stopwords returns the stopword list for a language code, or the combined
// list when the language is unknown.
func Stopwords(lang string) []string {
	if words, ok := stopwordsByLanguage[lang]; ok {
		return words
	}
	var out []string
	for _, words := range stopwordsByLanguage {
		out = append(out, words...)
	}
	return out
}

// IsStopword reports whether a term is a stopword in the given language.
// An empty language checks every known list.
func IsStopword(term, lang string) bool {
	if lang != "" {
		for _, w := range stopwordsByLanguage[lang] {
			if w == term {
				return true
			}
		}
		return false
	}
	return allStopwords[term]
}

// segmentRuns splits text into runs of characters that are written without
// word separators. A run is kept whole so it can be matched as one token.
func segmentRuns(s string) []string {
	var runs []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			runs = append(runs, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		if isUnsegmented(r) {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return runs
}

func isUnsegmented(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF, // Han
		r >= 0x3040 && r <= 0x30FF, // Hiragana and Katakana
		r >= 0xAC00 && r <= 0xD7AF, // Hangul
		r >= 0x0E00 && r <= 0x0E7F: // Thai
		return true
	}
	return false
}

// Tokenize splits text into search terms. Scripts written without word
// separators are kept as whole runs, everything else is split on punctuation.
func Tokenize(s string) []string {
	var out []string
	for _, field := range strings.FieldsFunc(s, func(r rune) bool {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
		// Combining marks belong to the letter they follow, so they never
		// end a field. Thai and Devanagari use them heavily.
		return !unicode.IsMark(r)
	}) {
		if needsSegmentationFor(field) {
			out = append(out, segmentRuns(field)...)
			continue
		}
		out = append(out, field)
	}
	return out
}

func needsSegmentationFor(s string) bool {
	for _, r := range s {
		if isUnsegmented(r) {
			return true
		}
	}
	return false
}
