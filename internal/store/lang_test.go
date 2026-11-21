package store

import "testing"

func TestTokenizeKeepsUnsegmentedRunsWhole(t *testing.T) {
	got := Tokenize("项目管理笔记 with english")
	want := []string{"项目管理笔记", "with", "english"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestTokenizeKeepsThaiRunsWhole(t *testing.T) {
	got := Tokenize("การจัดการหน่วยความจำ test")
	if len(got) != 2 || got[0] != "การจัดการหน่วยความจำ" {
		t.Fatalf("got %v", got)
	}
}

func TestTokenizeSplitsMixedScripts(t *testing.T) {
	got := Tokenize("cache 缓存 strategy")
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	if got[1] != "缓存" {
		t.Fatalf("got %v", got)
	}
}

func TestDetectScript(t *testing.T) {
	cases := map[string]Script{
		"hello world": ScriptLatin,
		"项目管理":        ScriptCJK,
		"こんにちは":       ScriptCJK,
		"안녕하세요":       ScriptCJK,
		"สวัสดี":      ScriptThai,
		"привет":      ScriptCyrillic,
		"مرحبا":       ScriptArabic,
		"שלום":        ScriptHebrew,
		"γεια":        ScriptGreek,
	}
	for text, want := range cases {
		if got := DetectScript(text); got != want {
			t.Fatalf("DetectScript(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestNeedsSegmentation(t *testing.T) {
	if !NeedsSegmentation("项目管理") {
		t.Fatal("CJK should need segmentation")
	}
	if !NeedsSegmentation("สวัสดี") {
		t.Fatal("Thai should need segmentation")
	}
	if NeedsSegmentation("hello world") {
		t.Fatal("Latin should not need segmentation")
	}
}

func TestStopwordsAcrossLanguages(t *testing.T) {
	if !IsStopword("the", "en") {
		t.Fatal("the should be an English stopword")
	}
	if !IsStopword("adalah", "id") {
		t.Fatal("adalah should be an Indonesian stopword")
	}
	if IsStopword("the", "id") {
		t.Fatal("the should not be an Indonesian stopword")
	}
	if IsStopword("cache", "") {
		t.Fatal("cache should not be a stopword in any language")
	}
}

func TestFTS5MatchDropsStopwordsInAnyLanguage(t *testing.T) {
	got := FTS5Match("the cache und der artikel")
	if got != `"cache" OR "artikel"` {
		t.Fatalf("got %q", got)
	}
}

func TestFTS5MatchKeepsUnsegmentedRunWhole(t *testing.T) {
	got := FTS5Match("项目管理笔记")
	if got != `"项目管理笔记"` {
		t.Fatalf("got %q", got)
	}
}

func TestFTS5MatchKeepsCyrillicAndGreek(t *testing.T) {
	got := FTS5Match("привет мир")
	if got != `"привет" OR "мир"` {
		t.Fatalf("got %q", got)
	}
}
