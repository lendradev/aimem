package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lendradev/aimem/internal/memory"
	"github.com/lendradev/aimem/internal/store"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	g, err := store.Open(filepath.Join(dir, "global", "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.Open(filepath.Join(dir, "project", "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		g.Close()
		p.Close()
	})
	eng := NewEngine(g, p)
	eng.Now = func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }
	return eng
}

// remember is a test helper that stores one item and fails the test on error.
func remember(t *testing.T, eng *Engine, scope, kind, title, body string, tags []string) memory.Item {
	t.Helper()
	res, err := eng.Remember(RememberOptions{
		Scope: scope, Kind: kind, Title: title, Body: body, Tags: tags, Source: "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	return res.Item
}

func TestRememberAndShow(t *testing.T) {
	eng := newTestEngine(t)
	it := remember(t, eng, memory.ScopeProject, "fact", "Go is the language", "chosen on 2026-11-21", []string{"Stack"})
	if it.ID == "" || len(it.ID) != 8 {
		t.Fatalf("unexpected id %q", it.ID)
	}
	got, err := eng.Show(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Go is the language" || got.Body != "chosen on 2026-11-21" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "stack" {
		t.Fatalf("tags mismatch: %v", got.Tags)
	}
}

func TestRememberRejectsBadInput(t *testing.T) {
	eng := newTestEngine(t)
	if _, err := eng.Remember(RememberOptions{Scope: memory.ScopeProject, Kind: "nonsense", Title: "title"}); err == nil {
		t.Fatal("expected error for invalid kind")
	}
	if _, err := eng.Remember(RememberOptions{Scope: memory.ScopeProject, Kind: "fact", Title: "  "}); err == nil {
		t.Fatal("expected error for empty title")
	}
	long := make([]byte, MaxTitleBytes+1)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := eng.Remember(RememberOptions{Scope: memory.ScopeProject, Kind: "fact", Title: string(long)}); err == nil {
		t.Fatal("expected error for long title")
	}
}

// A body of 32 KB must fit, one byte more must not.
func TestRememberBodyLimit(t *testing.T) {
	eng := newTestEngine(t)
	body := strings.Repeat("a", MaxBodyBytes)
	if _, err := eng.Remember(RememberOptions{
		Scope: memory.ScopeProject, Kind: "fact", Title: "big body", Body: body,
	}); err != nil {
		t.Fatalf("body at the limit was rejected: %v", err)
	}
	if _, err := eng.Remember(RememberOptions{
		Scope: memory.ScopeProject, Kind: "fact", Title: "too big", Body: body + "a",
	}); err == nil {
		t.Fatal("expected error for oversized body")
	}
}

func TestRememberStoresFundamentalFlag(t *testing.T) {
	eng := newTestEngine(t)
	res, err := eng.Remember(RememberOptions{
		Scope: memory.ScopeProject, Kind: "fact", Title: "physics law", Fundamental: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := eng.Show(res.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Fundamental {
		t.Fatal("fundamental flag was not stored")
	}
}

func TestRememberRejectsDuplicate(t *testing.T) {
	eng := newTestEngine(t)
	remember(t, eng, memory.ScopeProject, "decision", "pakai Go bukan Rust", "keputusan user 2026-11-21", nil)

	res, err := eng.Remember(RememberOptions{
		Scope: memory.ScopeProject, Kind: "decision",
		Title: "pakai Go bukan Rust", Body: "keputusan user 2026-11-21", Source: "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.DuplicateOf == nil {
		t.Fatal("duplicate was not detected")
	}
	if res.Item.ID != "" {
		t.Fatal("duplicate was stored anyway")
	}
	if res.DuplicateScore < DuplicateThreshold {
		t.Fatalf("duplicate score %v is below the threshold", res.DuplicateScore)
	}
}

func TestRememberAllowsDuplicateWhenRequested(t *testing.T) {
	eng := newTestEngine(t)
	remember(t, eng, memory.ScopeProject, "decision", "pakai Go bukan Rust", "keputusan user", nil)

	res, err := eng.Remember(RememberOptions{
		Scope: memory.ScopeProject, Kind: "decision",
		Title: "pakai Go bukan Rust", Body: "keputusan user", Source: "cli", AllowDuplicate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.DuplicateOf != nil {
		t.Fatal("duplicate was reported even though duplicates were allowed")
	}
	if res.Item.ID == "" {
		t.Fatal("item was not stored")
	}
}

func TestRememberKeepsDifferentItems(t *testing.T) {
	eng := newTestEngine(t)
	remember(t, eng, memory.ScopeProject, "decision", "pakai Go bukan Rust", "keputusan user", nil)
	res, err := eng.Remember(RememberOptions{
		Scope: memory.ScopeProject, Kind: "decision",
		Title: "simpan database di SQLite", Body: "pure Go tanpa cgo", Source: "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.DuplicateOf != nil {
		t.Fatalf("unrelated item was flagged as duplicate: %+v", res.DuplicateOf)
	}
}

func TestScopesAreIsolated(t *testing.T) {
	eng := newTestEngine(t)
	remember(t, eng, memory.ScopeProject, "fact", "project only item", "", nil)
	remember(t, eng, memory.ScopeGlobal, "fact", "global only item", "", nil)

	res, err := eng.Recall(RecallOptions{Query: "item", Scope: memory.ScopeProject, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("project scope returned %d items, want 1", len(res))
	}
	if res[0].Title != "project only item" {
		t.Fatalf("project scope leaked: %q", res[0].Title)
	}

	res, err = eng.Recall(RecallOptions{Query: "item", Scope: memory.ScopeGlobal, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Title != "global only item" {
		t.Fatalf("global scope wrong: %+v", res)
	}
}

func TestRecallFindsItem(t *testing.T) {
	eng := newTestEngine(t)
	remember(t, eng, memory.ScopeProject, "decision", "pakai Go bukan Rust", "keputusan user 2026-11-21", []string{"keputusan"})
	res, err := eng.Recall(RecallOptions{Query: "Rust", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d results, want 1", len(res))
	}
	if res[0].Title != "pakai Go bukan Rust" {
		t.Fatalf("got %q", res[0].Title)
	}
}

func TestRecallStemsWords(t *testing.T) {
	eng := newTestEngine(t)
	remember(t, eng, memory.ScopeProject, "fact", "docker caching strategy", "cache invalidation rules", nil)
	res, err := eng.Recall(RecallOptions{Query: "cache", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d results, want 1 (stemming should match)", len(res))
	}
}

func TestRecallRawOperatorQueryDoesNotError(t *testing.T) {
	eng := newTestEngine(t)
	remember(t, eng, memory.ScopeProject, "fact", "docker build cache", "", nil)
	for _, q := range []string{"go-build", "a:b", "(cache", "col:x", `"unterminated`} {
		if _, err := eng.Recall(RecallOptions{Query: q, Limit: 5}); err != nil {
			t.Fatalf("query %q returned an error: %v", q, err)
		}
	}
}

func TestRecallFindsItemByPrefixWhenFTSMisses(t *testing.T) {
	eng := newTestEngine(t)
	remember(t, eng, memory.ScopeProject, "fact", "rebuild mongodb connection pool", "keputusan arsitektur", nil)
	// "pool" is a prefix of "pooling"; the porter tokenizer does not match it,
	// so the trigram fallback has to.
	res, err := eng.Recall(RecallOptions{Query: "pool", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d results, want the trigram fallback to find 1: %+v", len(res), res)
	}
	if res[0].Title != "rebuild mongodb connection pool" {
		t.Fatalf("got %q", res[0].Title)
	}
}

func TestRecallEmptyQueryReturnsNothing(t *testing.T) {
	eng := newTestEngine(t)
	remember(t, eng, memory.ScopeProject, "fact", "something", "", nil)
	res, err := eng.Recall(RecallOptions{Query: "   "})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Fatalf("got %d results, want 0", len(res))
	}
}

func TestRecallRespectsTokenBudget(t *testing.T) {
	eng := newTestEngine(t)
	for i := 0; i < 5; i++ {
		body := ""
		for j := 0; j < 40; j++ {
			body += "cache line padding text here "
		}
		remember(t, eng, memory.ScopeProject, "fact", "cache item "+string(rune('a'+i)), body, nil)
	}
	res, err := eng.Recall(RecallOptions{Query: "cache", Limit: 10, TokenBudget: 60})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) >= 5 {
		t.Fatalf("token budget did not limit results: got %d", len(res))
	}
}

// A body larger than the token budget must still come back partially, so a big
// memory item stays useful without blowing the budget.
func TestRecallKeepsPartialBodyWithinBudget(t *testing.T) {
	eng := newTestEngine(t)
	body := strings.Repeat("detail words here ", 500)
	remember(t, eng, memory.ScopeProject, "fact", "large body item", body, nil)
	res, err := eng.Recall(RecallOptions{Query: "large body", Limit: 5, TokenBudget: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d results", len(res))
	}
	if res[0].Body == "" {
		t.Fatal("body was dropped instead of truncated")
	}
	if !res[0].Truncated {
		t.Fatal("truncated flag was not set")
	}
	if EstimateTokens(res[0].Body)+EstimateTokens(res[0].Title) > 100 {
		t.Fatalf("budget exceeded: %d tokens", EstimateTokens(res[0].Body))
	}
}

func TestRecallHidesDeadEndsByDefault(t *testing.T) {
	eng := newTestEngine(t)
	it := remember(t, eng, memory.ScopeProject, "fact", "postgres migration approach", "tried and failed", nil)
	if _, err := eng.Feedback(it.ID, memory.OutcomeDeadEnd, "", "cli"); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Recall(RecallOptions{Query: "postgres", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Fatalf("dead end was returned: %+v", res)
	}
	res, err = eng.Recall(RecallOptions{Query: "postgres", Limit: 5, IncludeDeadEnds: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("dead end hidden even with the flag: %+v", res)
	}
}

func TestFeedbackUpdatesStatusAndScore(t *testing.T) {
	eng := newTestEngine(t)
	it := remember(t, eng, memory.ScopeProject, "fact", "useful item", "", nil)
	if _, err := eng.Feedback(it.ID, memory.OutcomeUseful, "", "session-a"); err != nil {
		t.Fatal(err)
	}
	got, err := eng.Feedback(it.ID, memory.OutcomeUseful, "", "session-b")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != memory.StatusPreferred {
		t.Fatalf("got status %v, want preferred", got.Status)
	}
	if got.Score <= 0 {
		t.Fatalf("got score %v, want positive", got.Score)
	}
}

func TestFeedbackRejectsUnknownItem(t *testing.T) {
	eng := newTestEngine(t)
	if _, err := eng.Feedback("missing1", memory.OutcomeUseful, "", "cli"); err == nil {
		t.Fatal("expected error for unknown item")
	}
	if _, err := eng.Feedback("missing1", "bogus", "", "cli"); err == nil {
		t.Fatal("expected error for invalid outcome")
	}
}

func TestForgetRemovesItem(t *testing.T) {
	eng := newTestEngine(t)
	it := remember(t, eng, memory.ScopeProject, "fact", "temporary", "", nil)
	if _, err := eng.Forget([]string{it.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Show(it.ID); err == nil {
		t.Fatal("item still present after forget")
	}
	if _, err := eng.Forget([]string{it.ID}); err == nil {
		t.Fatal("expected error forgetting a missing item")
	}
}

func TestProjectScopeOverridesGlobal(t *testing.T) {
	eng := newTestEngine(t)
	remember(t, eng, memory.ScopeGlobal, "preference", "editor preference uses tabs", "", nil)
	remember(t, eng, memory.ScopeProject, "preference", "editor preference uses spaces", "", nil)
	res, err := eng.Recall(RecallOptions{Query: "editor preference", Limit: 10, Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(res), res)
	}
	if res[0].Scope != memory.ScopeProject {
		t.Fatalf("project item did not outrank the global one: %+v", res)
	}
	if res[0].Title != "editor preference uses spaces" {
		t.Fatalf("wrong project item ranked first: %q", res[0].Title)
	}
}

func TestRecallUsageRaisesScoreOverTime(t *testing.T) {
	eng := newTestEngine(t)
	used := remember(t, eng, memory.ScopeProject, "fact", "recall me often", "", nil)
	unused := remember(t, eng, memory.ScopeProject, "fact", "recall me rarely", "", nil)

	for i := 0; i < 3; i++ {
		if _, err := eng.Recall(RecallOptions{Query: "often", Limit: 5}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := eng.Recall(RecallOptions{Query: "rarely", Limit: 5}); err != nil {
		t.Fatal(err)
	}

	usedItem, err := eng.Show(used.ID)
	if err != nil {
		t.Fatal(err)
	}
	unusedItem, err := eng.Show(unused.ID)
	if err != nil {
		t.Fatal(err)
	}
	if usedItem.UseCount != 3 {
		t.Fatalf("use count %d, want 3", usedItem.UseCount)
	}
	if usedItem.Score <= unusedItem.Score {
		t.Fatalf("used item score %v is not above unused item score %v", usedItem.Score, unusedItem.Score)
	}
}

func TestFindPromotionsNeedsTwoSourcesAndTwoRecalls(t *testing.T) {
	eng := newTestEngine(t)
	it := remember(t, eng, memory.ScopeProject, "procedure", "always run the migration script first", "", nil)

	promos, err := eng.FindPromotions()
	if err != nil {
		t.Fatal(err)
	}
	if len(promos) != 0 {
		t.Fatalf("unconfirmed item was proposed: %+v", promos)
	}

	if _, err := eng.Feedback(it.ID, memory.OutcomeUseful, "", "session-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Feedback(it.ID, memory.OutcomeUseful, "", "session-b"); err != nil {
		t.Fatal(err)
	}
	promos, err = eng.FindPromotions()
	if err != nil {
		t.Fatal(err)
	}
	if len(promos) != 0 {
		t.Fatalf("item was proposed before it was recalled twice: %+v", promos)
	}

	for i := 0; i < 2; i++ {
		if _, err := eng.Recall(RecallOptions{Query: "migration", Limit: 5}); err != nil {
			t.Fatal(err)
		}
	}
	promos, err = eng.FindPromotions()
	if err != nil {
		t.Fatal(err)
	}
	if len(promos) != 1 || promos[0].Item.ID != it.ID {
		t.Fatalf("got %d promotions, want 1: %+v", len(promos), promos)
	}
}

func TestPromoteMovesItemToGlobalScope(t *testing.T) {
	eng := newTestEngine(t)
	it := remember(t, eng, memory.ScopeProject, "procedure", "always run the migration script first", "", nil)
	if _, err := eng.Feedback(it.ID, memory.OutcomeUseful, "", "session-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Feedback(it.ID, memory.OutcomeUseful, "", "session-b"); err != nil {
		t.Fatal(err)
	}

	global, err := eng.Promote(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if global.Scope != memory.ScopeGlobal {
		t.Fatalf("scope %v, want global", global.Scope)
	}
	if _, err := eng.Project.Get(it.ID); err == nil {
		t.Fatal("item still in the project scope")
	}
	if _, err := eng.Global.Get(global.ID); err != nil {
		t.Fatal("item missing from the global scope")
	}
	sigs, err := eng.Global.Signals(global.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 2 {
		t.Fatalf("signal history was not carried over: got %d", len(sigs))
	}
}

func TestDefaultPathsUseAimemDir(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	proj := filepath.Join(dir, "proj")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_MEMORY_HOME", filepath.Join(home, ".aimem"))
	t.Setenv("AI_MEMORY_PROJECT", filepath.Join(proj, ".aimem"))

	g, p, err := DefaultPaths(proj)
	if err != nil {
		t.Fatal(err)
	}
	if g != filepath.Join(home, ".aimem", "data.db") {
		t.Fatalf("global path %q", g)
	}
	if p != filepath.Join(proj, ".aimem", "data.db") {
		t.Fatalf("project path %q", p)
	}
}

func TestDefaultPathsMigrateLegacyDir(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	proj := filepath.Join(dir, "proj")
	if err := os.MkdirAll(filepath.Join(home, ".ai-memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, ".ai-memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacyFile := filepath.Join(proj, ".ai-memory", "data.db")
	if err := os.WriteFile(legacyFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_MEMORY_HOME", "")
	t.Setenv("AI_MEMORY_PROJECT", "")

	_, p, err := DefaultPaths(proj)
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(proj, ".aimem", "data.db") {
		t.Fatalf("project path %q", p)
	}
	if _, err := os.Stat(filepath.Join(proj, ".aimem", "data.db")); err != nil {
		t.Fatalf("legacy database was not migrated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, ".ai-memory")); err == nil {
		t.Fatal("legacy directory still exists")
	}
}

func TestDefaultPathsDoNotOverwriteExistingDir(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "proj")
	if err := os.MkdirAll(filepath.Join(proj, ".ai-memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, ".aimem"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_MEMORY_HOME", filepath.Join(dir, "home"))
	t.Setenv("AI_MEMORY_PROJECT", filepath.Join(proj, ".aimem"))

	if _, _, err := DefaultPaths(proj); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(proj, ".ai-memory")); err != nil {
		t.Fatal("legacy directory was removed even though the new one already exists")
	}
}

func TestResolveScopeFindsGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveScope(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("got %q, want %q", got, dir)
	}
}

func TestResolveScopePrefersNestedWorkspaceRepo(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(filepath.Join(inner, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveScope(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != inner {
		t.Fatalf("got %q, want %q", got, inner)
	}
}

func TestResolveScopeReportsNoProjectRoot(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveScope(dir)
	if !errors.Is(err, ErrNoProjectRoot) {
		t.Fatalf("err = %v, want ErrNoProjectRoot", err)
	}
	if got != "" {
		t.Fatalf("root %q, want empty", got)
	}
}

func TestDefaultPathsWithoutProjectRootHasNoProjectPath(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	plain := filepath.Join(dir, "plain")
	for _, d := range []string{home, plain} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("AI_MEMORY_HOME", filepath.Join(home, ".aimem"))
	t.Setenv("AI_MEMORY_PROJECT", "")

	g, p, err := DefaultPaths(plain)
	if err != nil {
		t.Fatal(err)
	}
	if g != filepath.Join(home, ".aimem", "data.db") {
		t.Fatalf("global path %q", g)
	}
	if p != "" {
		t.Fatalf("project path %q, want empty when there is no project root", p)
	}
}

func TestDefaultPathsDropsProjectWhenItCollidesWithGlobal(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_MEMORY_HOME", filepath.Join(home, ".aimem"))
	t.Setenv("AI_MEMORY_PROJECT", "")

	// The home directory has no project marker, so the project scope must be
	// dropped rather than pointing at the global database.
	g, p, err := DefaultPaths(home)
	if err != nil {
		t.Fatal(err)
	}
	if p != "" {
		t.Fatalf("project path %q, want empty to avoid a collision with the global scope", p)
	}
	if g != filepath.Join(home, ".aimem", "data.db") {
		t.Fatalf("global path %q", g)
	}
}
