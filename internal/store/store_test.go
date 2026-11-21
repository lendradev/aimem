package store

import (
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
	n, err := st.Prune(time.Now(), 180, 365)
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
		t.Fatal("fresh item was pruned")
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
