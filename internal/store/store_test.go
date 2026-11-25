package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/lendradev/aimem/internal/memory"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func testItem(id string) memory.Item {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	return memory.Item{
		ID: id, Kind: memory.KindFact, Title: "item " + id, Body: "body " + id,
		Tags: []string{"tag"}, Scope: memory.ScopeProject, Source: "cli",
		Status: memory.StatusTentative, CreatedAt: now, UpdatedAt: now, LastUsed: now,
	}
}

func TestInsertGetDelete(t *testing.T) {
	st := newStore(t)
	it := testItem("abcd1234")
	if err := st.Insert(it); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get("abcd1234")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != it.Title || got.Body != it.Body {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "tag" {
		t.Fatalf("tags mismatch: %v", got.Tags)
	}
	if err := st.Delete("abcd1234"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get("abcd1234"); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestSearchIndexesItems(t *testing.T) {
	st := newStore(t)
	if err := st.Insert(testItem("aaa11111")); err != nil {
		t.Fatal(err)
	}
	hits, err := st.Search(`"docker"`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("unrelated query matched: %+v", hits)
	}
	hits, err = st.Search(`"body"`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "aaa11111" {
		t.Fatalf("got %+v", hits)
	}
}

// A title match must outrank a body match, because the title states what the
// item is about while the body only mentions the term somewhere.
func TestSearchRanksTitleAboveBody(t *testing.T) {
	st := newStore(t)
	titleItem := testItem("titl0001")
	titleItem.Title = "storage write policy"
	titleItem.Body = "short note"
	if err := st.Insert(titleItem); err != nil {
		t.Fatal(err)
	}
	bodyItem := testItem("body00001")
	bodyItem.Title = "unrelated heading"
	bodyItem.Body = "this body mentions storage write policy once in passing"
	if err := st.Insert(bodyItem); err != nil {
		t.Fatal(err)
	}
	hits, err := st.Search(`"storage" OR "write" OR "policy"`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2: %+v", len(hits), hits)
	}
	if hits[0].ID != "titl0001" {
		t.Fatalf("body match outranked the title match: %+v", hits)
	}
}

func TestSearchSurvivesUpdate(t *testing.T) {
	st := newStore(t)
	it := testItem("bbb22222")
	it.Title = "original title"
	if err := st.Insert(it); err != nil {
		t.Fatal(err)
	}
	it.Title = "replaced title"
	it.Body = "replaced body"
	if err := st.Update(it); err != nil {
		t.Fatal(err)
	}
	hits, err := st.Search(`"replaced"`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("updated body not indexed: %+v", hits)
	}
	hits, err = st.Search(`"original"`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("stale text still indexed: %+v", hits)
	}
}

func TestSignalsRoundTrip(t *testing.T) {
	st := newStore(t)
	if err := st.Insert(testItem("ccc33333")); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := st.AddSignal(memory.Signal{ItemID: "ccc33333", Outcome: memory.OutcomeUseful, Source: "a", At: at}); err != nil {
		t.Fatal(err)
	}
	sigs, err := st.Signals("ccc33333")
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 || sigs[0].Outcome != memory.OutcomeUseful || sigs[0].Source != "a" {
		t.Fatalf("got %+v", sigs)
	}
}

func TestDeletingItemRemovesSignals(t *testing.T) {
	st := newStore(t)
	if err := st.Insert(testItem("ddd44444")); err != nil {
		t.Fatal(err)
	}
	if err := st.AddSignal(memory.Signal{ItemID: "ddd44444", Outcome: memory.OutcomeUseful, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := st.Delete("ddd44444"); err != nil {
		t.Fatal(err)
	}
	sigs, err := st.AllSignals()
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 0 {
		t.Fatalf("orphan signals left: %+v", sigs)
	}
}

func TestPruneRemovesIdleItemsOnly(t *testing.T) {
	st := newStore(t)
	old := testItem("old11111")
	old.CreatedAt = time.Now().AddDate(0, 0, -400)
	old.UpdatedAt = old.CreatedAt
	old.LastUsed = old.CreatedAt
	if err := st.Insert(old); err != nil {
		t.Fatal(err)
	}
	fresh := testItem("new11111")
	if err := st.Insert(fresh); err != nil {
		t.Fatal(err)
	}
	n, err := st.Prune(time.Now(), 180, 365, -1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pruned %d items, want 1", n)
	}
	if _, err := st.Get("old11111"); err != ErrNotFound {
		t.Fatal("idle item was not pruned")
	}
	if _, err := st.Get("new11111"); err != nil {
		t.Fatalf("fresh item was pruned")
	}
}

// Fundamental knowledge stays even when it is old and never used. The stale
// window drops everything else.
func TestPruneKeepsFundamentalItems(t *testing.T) {
	st := newStore(t)
	fund := testItem("fund1111")
	fund.Fundamental = true
	fund.CreatedAt = time.Now().AddDate(0, 0, -3000)
	fund.UpdatedAt = fund.CreatedAt
	fund.LastUsed = fund.CreatedAt
	if err := st.Insert(fund); err != nil {
		t.Fatal(err)
	}
	plain := testItem("plai1111")
	plain.CreatedAt = time.Now().AddDate(0, 0, -3000)
	plain.UpdatedAt = plain.CreatedAt
	plain.LastUsed = plain.CreatedAt
	if err := st.Insert(plain); err != nil {
		t.Fatal(err)
	}

	n, err := st.Prune(time.Now(), 180, 365, 365)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pruned %d items, want 1", n)
	}
	if _, err := st.Get("fund1111"); err != nil {
		t.Fatalf("fundamental item was pruned")
	}
	if _, err := st.Get("plai1111"); err != ErrNotFound {
		t.Fatalf("stale item was not pruned")
	}
}

// The stale window drops items that were used once but never recalled again.
func TestPruneDropsStaleUsedItems(t *testing.T) {
	st := newStore(t)
	used := testItem("used1111")
	used.UseCount = 3
	used.CreatedAt = time.Now().AddDate(0, 0, -3000)
	used.UpdatedAt = used.CreatedAt
	used.LastUsed = used.CreatedAt
	if err := st.Insert(used); err != nil {
		t.Fatal(err)
	}
	n, err := st.Prune(time.Now(), 180, 365, 365)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pruned %d items, want 1", n)
	}
	if _, err := st.Get("used1111"); err != ErrNotFound {
		t.Fatal("stale used item was not pruned")
	}
}

// A database created before the fundamental column exists must still open
// and keep its rows.
func TestOpenMigratesLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE items (
	  id TEXT PRIMARY KEY, kind TEXT NOT NULL, title TEXT NOT NULL,
	  body TEXT NOT NULL DEFAULT '', tags TEXT NOT NULL DEFAULT '', scope TEXT NOT NULL,
	  source TEXT NOT NULL DEFAULT 'cli', status TEXT NOT NULL DEFAULT 'tentative',
	  score REAL NOT NULL DEFAULT 0, use_count INTEGER NOT NULL DEFAULT 0,
	  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, last_used INTEGER NOT NULL DEFAULT 0)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO items (id, kind, title, scope, created_at, updated_at, last_used)
	  VALUES ('legacy01', 'fact', 'legacy item', 'project', 1, 1, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	it, err := st.Get("legacy01")
	if err != nil {
		t.Fatal(err)
	}
	if it.Title != "legacy item" {
		t.Fatalf("legacy item mismatch: %+v", it)
	}
	if it.Fundamental {
		t.Fatal("legacy item must not be fundamental")
	}
}

func TestStatsReportsCounts(t *testing.T) {
	st := newStore(t)
	if err := st.Insert(testItem("eee55555")); err != nil {
		t.Fatal(err)
	}
	rows, size, err := st.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if rows["items"] != 1 {
		t.Fatalf("items count %d", rows["items"])
	}
	if size <= 0 {
		t.Fatalf("size %d", size)
	}
}
