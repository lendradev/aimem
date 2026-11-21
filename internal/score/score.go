package score

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/lendradev/aimem/internal/memory"
)

const (
	// OutcomeHalfLifeDays is how long an outcome signal keeps half its weight.
	OutcomeHalfLifeDays = 30.0
	// MinCorroboration is the number of distinct sources needed to call an item preferred.
	MinCorroboration = 2
	// RecencyHalfLifeDays is how long an untouched item keeps half its recency weight.
	RecencyHalfLifeDays = 60.0
	// UsageHalfLifeDays is how long a recall keeps half its reinforcement weight.
	UsageHalfLifeDays = 45.0
	// UsageReinforcement is the weight one recall adds to an item's score.
	UsageReinforcement = 0.15
	// UsageReinforcementCap bounds the total reinforcement from usage.
	UsageReinforcementCap = 1.5
)

// Reinforcement returns the score contribution from being recalled repeatedly.
//
// Recall alone is weak evidence, so each use adds a small amount. The
// contribution decays with time, and the total is capped so a frequently
// recalled but never confirmed item cannot outrank a confirmed one.
func Reinforcement(useCount int, lastUsed, now time.Time) float64 {
	if useCount <= 0 {
		return 0
	}
	age := now.Sub(lastUsed).Hours() / 24
	if age < 0 {
		age = 0
	}
	decay := math.Pow(0.5, age/UsageHalfLifeDays)
	value := float64(useCount) * UsageReinforcement * decay
	if value > UsageReinforcementCap {
		value = UsageReinforcementCap
	}
	return value
}

// TotalScore combines outcome evidence with usage reinforcement.
//
// Outcome signals dominate, so a confirmed item always outranks one that was
// merely recalled often.
func TotalScore(signals []memory.Signal, useCount int, lastUsed, now time.Time) float64 {
	return round9(DecayedScore(signals, now) + Reinforcement(useCount, lastUsed, now))
}

func signalWeight(o memory.Outcome) float64 {
	switch o {
	case memory.OutcomeUseful:
		return 1
	case memory.OutcomeDeadEnd:
		return -1
	case memory.OutcomeCorrected:
		return -1.5
	}
	return 0
}

// DecayedScore sums outcome signals with exponential time decay.
func DecayedScore(signals []memory.Signal, now time.Time) float64 {
	total := 0.0
	for _, s := range signals {
		age := now.Sub(s.At).Hours() / 24
		if age < 0 {
			age = 0
		}
		total += signalWeight(s.Outcome) * math.Pow(0.5, age/OutcomeHalfLifeDays)
	}
	return round9(total)
}

// DeriveStatus turns the signal history into an item status.
//
// A node with both positive and negative signals is contested regardless of
// order. A node with only negative signals is a dead end or a correction
// depending on which one came last. A node with only positive signals is
// preferred once enough distinct sources corroborated it.
func DeriveStatus(signals []memory.Signal) memory.Status {
	if len(signals) == 0 {
		return memory.StatusTentative
	}
	ordered := append([]memory.Signal(nil), signals...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].At.Before(ordered[j].At) })

	corroborators := map[string]bool{}
	hasUseful := false
	hasNegative := false
	lastNegative := memory.Outcome("")
	for _, s := range ordered {
		switch s.Outcome {
		case memory.OutcomeUseful:
			hasUseful = true
			corroborators[corroboratorKey(s)] = true
		case memory.OutcomeDeadEnd, memory.OutcomeCorrected:
			hasNegative = true
			lastNegative = s.Outcome
		}
	}

	switch {
	case hasUseful && hasNegative:
		return memory.StatusContested
	case hasNegative:
		if lastNegative == memory.OutcomeCorrected {
			return memory.StatusCorrected
		}
		return memory.StatusDeadEnd
	case len(corroborators) >= MinCorroboration:
		return memory.StatusPreferred
	}
	return memory.StatusTentative
}

func corroboratorKey(s memory.Signal) string {
	if s.Source != "" {
		return s.Source
	}
	return fmt.Sprintf("anon:%d", s.ID)
}

// Recency returns a value in (0,1] that decays with a 60 day half-life.
func Recency(t, now time.Time) float64 {
	age := now.Sub(t).Hours() / 24
	if age < 0 {
		age = 0
	}
	return math.Pow(0.5, age/RecencyHalfLifeDays)
}

func round9(v float64) float64 {
	return math.Round(v*1e9) / 1e9
}
