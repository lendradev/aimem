package reflect

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lendradev/aimem/internal/memory"
	"github.com/lendradev/aimem/internal/score"
	"github.com/lendradev/aimem/internal/store"
)

// Lesson is a distilled takeaway written to LESSONS.md.
type Lesson struct {
	Title  string
	Body   string
	Status string
	Scope  string
}

// Report groups items by status for one scope.
type Report struct {
	Preferred   []memory.Item
	Tentative   []memory.Item
	Contested   []memory.Item
	DeadEnds    []memory.Item
	Corrections []Correction
}

type Correction struct {
	ItemID string
	Title  string
	Text   string
	At     time.Time
}

// Build reads every item and its signals in a store and groups them.
func Build(st *store.Store, now time.Time) (*Report, error) {
	items, err := st.List(memory.ScopeProject)
	if err != nil {
		return nil, err
	}
	globalItems, err := st.List(memory.ScopeGlobal)
	if err != nil {
		return nil, err
	}
	items = append(items, globalItems...)

	signals, err := st.AllSignals()
	if err != nil {
		return nil, err
	}
	byItem := map[string][]memory.Signal{}
	for _, s := range signals {
		byItem[s.ItemID] = append(byItem[s.ItemID], s)
	}

	rep := &Report{}
	for _, it := range items {
		sigs := byItem[it.ID]
		it.Status = score.DeriveStatus(sigs)
		it.Score = score.DecayedScore(sigs, now)
		lastCorrection := lastSignal(sigs, memory.OutcomeCorrected)
		switch it.Status {
		case memory.StatusPreferred:
			rep.Preferred = append(rep.Preferred, it)
		case memory.StatusContested:
			rep.Contested = append(rep.Contested, it)
		case memory.StatusDeadEnd:
			rep.DeadEnds = append(rep.DeadEnds, it)
		case memory.StatusCorrected:
			rep.Corrections = append(rep.Corrections, Correction{ItemID: it.ID, Title: it.Title, At: lastCorrection})
		default:
			rep.Tentative = append(rep.Tentative, it)
		}
		for _, s := range sigs {
			if s.Outcome == memory.OutcomeCorrected && s.Correction != "" {
				rep.Corrections = append(rep.Corrections, Correction{ItemID: it.ID, Title: it.Title, Text: s.Correction, At: s.At})
			}
		}
	}

	sortByScore := func(s []memory.Item) {
		sort.SliceStable(s, func(i, j int) bool {
			if s[i].Score != s[j].Score {
				return s[i].Score > s[j].Score
			}
			return s[i].ID < s[j].ID
		})
	}
	sortByScore(rep.Preferred)
	sortByScore(rep.Tentative)
	sortByScore(rep.Contested)
	sortByScore(rep.DeadEnds)
	sort.SliceStable(rep.Corrections, func(i, j int) bool {
		if !rep.Corrections[i].At.Equal(rep.Corrections[j].At) {
			return rep.Corrections[i].At.After(rep.Corrections[j].At)
		}
		return rep.Corrections[i].ItemID < rep.Corrections[j].ItemID
	})
	return rep, nil
}

// Markdown renders the report as a deterministic LESSONS.md document.
func (r *Report) Markdown(now time.Time) string {
	var b strings.Builder
	b.WriteString("# Lessons\n\n")
	b.WriteString("Dihasilkan oleh `aimem reflect`. File ini dibaca agent di awal sesi.\n\n")

	writeSection := func(title string, items []memory.Item, note string) {
		b.WriteString("## " + title + "\n\n")
		if len(items) == 0 {
			b.WriteString("_Tidak ada._\n\n")
			return
		}
		if note != "" {
			b.WriteString(note + "\n\n")
		}
		for _, it := range items {
			b.WriteString(fmt.Sprintf("- **[%s] %s** (score %.2f, id `%s`)\n", it.Kind, it.Title, it.Score, it.ID))
			if it.Body != "" {
				b.WriteString("  " + firstLine(it.Body) + "\n")
			}
		}
		b.WriteString("\n")
	}

	writeSection("Preferred", r.Preferred, "Sudah terkonfirmasi dari beberapa sumber. Pakai lebih dulu.")
	writeSection("Tentative", r.Tentative, "Hanya satu konfirmasi. Perlakukan sebagai hipotesis.")
	writeSection("Contested", r.Contested, "Ada sinyal positif dan negatif. Cek ulang sebelum dipakai.")

	b.WriteString("## Known dead ends\n\n")
	if len(r.DeadEnds) == 0 {
		b.WriteString("_Tidak ada._\n\n")
	} else {
		for _, it := range r.DeadEnds {
			b.WriteString(fmt.Sprintf("- **%s** (id `%s`)\n", it.Title, it.ID))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Corrections\n\n")
	if len(r.Corrections) == 0 {
		b.WriteString("_Tidak ada._\n\n")
	} else {
		for _, c := range r.Corrections {
			if c.Text != "" {
				b.WriteString(fmt.Sprintf("- **%s**: %s (id `%s`)\n", c.Title, c.Text, c.ItemID))
			} else {
				b.WriteString(fmt.Sprintf("- **%s** dikoreksi (id `%s`)\n", c.Title, c.ItemID))
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func lastSignal(sigs []memory.Signal, outcome memory.Outcome) time.Time {
	var last time.Time
	for _, s := range sigs {
		if s.Outcome == outcome && s.At.After(last) {
			last = s.At
		}
	}
	return last
}
