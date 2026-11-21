package reflect

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lendradev/aimem/internal/memory"
	"github.com/lendradev/aimem/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func addItem(t *testing.T, st *store.Store, id, title string) {
	t.Helper()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	it := memory.Item{
		ID: id, Kind: memory.KindFact, Title: title, Body: "detail for " + title,
		Scope: memory.ScopeProject, Source: "cli", Status: memory.StatusTentative,
		CreatedAt: now, UpdatedAt: now, LastUsed: now,
	}
	if err := st.Insert(it); err != nil {
		t.Fatal(err)
	}
}

func TestBuildGroupsItemsByStatus(t *testing.T) {
	st := newStore(t)
	addItem(t, st, "aaa11111", "confirmed fact")
	addItem(t, st, "bbb22222", "single source fact")
	addItem(t, st, "ccc33333", "contested fact")
	addItem(t, st, "ddd44444", "failed approach")

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := st.AddSignal(memory.Signal{ItemID: "aaa11111", Outcome: memory.OutcomeUseful, Source: "s1", At: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddSignal(memory.Signal{ItemID: "aaa11111", Outcome: memory.OutcomeUseful, Source: "s2", At: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddSignal(memory.Signal{ItemID: "bbb22222", Outcome: memory.OutcomeUseful, Source: "s1", At: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddSignal(memory.Signal{ItemID: "ccc33333", Outcome: memory.OutcomeUseful, Source: "s1", At: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddSignal(memory.Signal{ItemID: "ccc33333", Outcome: memory.OutcomeDeadEnd, Source: "s2", At: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddSignal(memory.Signal{ItemID: "ddd44444", Outcome: memory.OutcomeDeadEnd, Source: "s1", At: now}); err != nil {
		t.Fatal(err)
	}

	rep, err := Build(st, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Preferred) != 1 || rep.Preferred[0].Title != "confirmed fact" {
		t.Fatalf("preferred: %+v", rep.Preferred)
	}
	if len(rep.Tentative) != 1 || rep.Tentative[0].Title != "single source fact" {
		t.Fatalf("tentative: %+v", rep.Tentative)
	}
	if len(rep.Contested) != 1 || rep.Contested[0].Title != "contested fact" {
		t.Fatalf("contested: %+v", rep.Contested)
	}
	if len(rep.DeadEnds) != 1 || rep.DeadEnds[0].Title != "failed approach" {
		t.Fatalf("dead ends: %+v", rep.DeadEnds)
	}
}

func TestMarkdownIsDeterministic(t *testing.T) {
	st := newStore(t)
	addItem(t, st, "aaa11111", "one")
	addItem(t, st, "bbb22222", "two")
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	rep, err := Build(st, now)
	if err != nil {
		t.Fatal(err)
	}
	first := rep.Markdown(now)
	rep2, err := Build(st, now)
	if err != nil {
		t.Fatal(err)
	}
	second := rep2.Markdown(now)
	if first != second {
		t.Fatal("markdown output is not stable across runs")
	}
	for _, want := range []string{"# Lessons", "## Preferred", "## Tentative", "## Contested", "## Known dead ends", "## Corrections"} {
		if !strings.Contains(first, want) {
			t.Fatalf("missing section %q in output:\n%s", want, first)
		}
	}
}

func TestMarkdownIncludesCorrectionText(t *testing.T) {
	st := newStore(t)
	addItem(t, st, "aaa11111", "wrong fact")
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := st.AddSignal(memory.Signal{
		ItemID: "aaa11111", Outcome: memory.OutcomeCorrected,
		Correction: "the real answer is Go", Source: "s1", At: now,
	}); err != nil {
		t.Fatal(err)
	}
	rep, err := Build(st, now)
	if err != nil {
		t.Fatal(err)
	}
	md := rep.Markdown(now)
	if !strings.Contains(md, "the real answer is Go") {
		t.Fatalf("correction text missing:\n%s", md)
	}
}
