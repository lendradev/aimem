package score

import (
	"testing"
	"time"

	"github.com/lendradev/aimem/internal/memory"
)

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestDecayedScoreSign(t *testing.T) {
	now := base.AddDate(0, 0, 1)
	sigs := []memory.Signal{
		{Outcome: memory.OutcomeUseful, At: base},
	}
	if got := DecayedScore(sigs, now); got <= 0 {
		t.Fatalf("useful signal gave score %v, want positive", got)
	}

	sigs = []memory.Signal{{Outcome: memory.OutcomeDeadEnd, At: base}}
	if got := DecayedScore(sigs, now); got >= 0 {
		t.Fatalf("dead end signal gave score %v, want negative", got)
	}

	sigs = []memory.Signal{{Outcome: memory.OutcomeCorrected, At: base}}
	if got := DecayedScore(sigs, now); got >= 0 {
		t.Fatalf("corrected signal gave score %v, want negative", got)
	}
}

func TestDecayedScoreHalvesEvery30Days(t *testing.T) {
	now := base.AddDate(0, 0, 30)
	got := DecayedScore([]memory.Signal{{Outcome: memory.OutcomeUseful, At: base}}, now)
	if got < 0.49 || got > 0.51 {
		t.Fatalf("got %v, want about 0.5", got)
	}
}

func TestDecayedScoreZeroForNoSignals(t *testing.T) {
	if got := DecayedScore(nil, base); got != 0 {
		t.Fatalf("got %v, want 0", got)
	}
}

func TestDeriveStatusTentativeWithoutSignals(t *testing.T) {
	if got := DeriveStatus(nil); got != memory.StatusTentative {
		t.Fatalf("got %v", got)
	}
}

func TestDeriveStatusPreferredNeedsTwoSources(t *testing.T) {
	sigs := []memory.Signal{
		{Outcome: memory.OutcomeUseful, Source: "a", At: base},
		{Outcome: memory.OutcomeUseful, Source: "b", At: base.Add(time.Hour)},
	}
	if got := DeriveStatus(sigs); got != memory.StatusPreferred {
		t.Fatalf("got %v, want preferred", got)
	}

	sigs = []memory.Signal{
		{Outcome: memory.OutcomeUseful, Source: "a", At: base},
		{Outcome: memory.OutcomeUseful, Source: "a", At: base.Add(time.Hour)},
	}
	if got := DeriveStatus(sigs); got != memory.StatusTentative {
		t.Fatalf("got %v, want tentative for the same source twice", got)
	}
}

func TestDeriveStatusDeadEnd(t *testing.T) {
	sigs := []memory.Signal{
		{Outcome: memory.OutcomeDeadEnd, Source: "b", At: base},
	}
	if got := DeriveStatus(sigs); got != memory.StatusDeadEnd {
		t.Fatalf("got %v, want dead_end", got)
	}
}

func TestDeriveStatusUsefulThenDeadEndIsContested(t *testing.T) {
	sigs := []memory.Signal{
		{Outcome: memory.OutcomeUseful, Source: "a", At: base},
		{Outcome: memory.OutcomeDeadEnd, Source: "b", At: base.Add(time.Hour)},
	}
	if got := DeriveStatus(sigs); got != memory.StatusContested {
		t.Fatalf("got %v, want contested", got)
	}
}

func TestDeriveStatusUsefulAfterDeadEndIsContested(t *testing.T) {
	sigs := []memory.Signal{
		{Outcome: memory.OutcomeDeadEnd, Source: "b", At: base},
		{Outcome: memory.OutcomeUseful, Source: "a", At: base.Add(time.Hour)},
	}
	if got := DeriveStatus(sigs); got != memory.StatusContested {
		t.Fatalf("got %v, want contested", got)
	}
}

func TestDeriveStatusContested(t *testing.T) {
	sigs := []memory.Signal{
		{Outcome: memory.OutcomeUseful, Source: "a", At: base},
		{Outcome: memory.OutcomeCorrected, Source: "b", At: base.Add(time.Hour)},
		{Outcome: memory.OutcomeUseful, Source: "c", At: base.Add(2 * time.Hour)},
	}
	if got := DeriveStatus(sigs); got != memory.StatusContested {
		t.Fatalf("got %v, want contested", got)
	}
}

func TestDeriveStatusCorrected(t *testing.T) {
	sigs := []memory.Signal{
		{Outcome: memory.OutcomeCorrected, Source: "a", At: base},
	}
	if got := DeriveStatus(sigs); got != memory.StatusCorrected {
		t.Fatalf("got %v, want corrected", got)
	}
}
