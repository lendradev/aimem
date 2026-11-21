package store

import (
	"testing"
)

func TestNormalizeQueryDropsPunctuationAndDuplicates(t *testing.T) {
	got := NormalizeQuery("Go-build: cache! cache, GOCACHE")
	want := []string{"go", "build", "cache", "gocache"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestFTS5MatchQuotesEveryTerm(t *testing.T) {
	got := FTS5Match("go-build cache")
	want := `"go" OR "build" OR "cache"`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFTS5MatchEscapesInternalQuotes(t *testing.T) {
	got := FTS5Match(`say "hi"`)
	if got != `"say" OR "hi"` {
		t.Fatalf("got %q", got)
	}
}

func TestFTS5MatchDropsStopwords(t *testing.T) {
	got := FTS5Match("the cache of the project")
	if got != `"cache" OR "project"` {
		t.Fatalf("got %q", got)
	}
}

func TestFTS5MatchEmptyQuery(t *testing.T) {
	if got := FTS5Match("   "); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestFTS5MatchKeepsAllStopwordsWhenNothingElse(t *testing.T) {
	got := FTS5Match("the and")
	if got != `"the" OR "and"` {
		t.Fatalf("got %q", got)
	}
}
