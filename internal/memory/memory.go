package memory

import "time"

type Kind string

const (
	KindFact       Kind = "fact"
	KindDecision   Kind = "decision"
	KindProcedure  Kind = "procedure"
	KindPreference Kind = "preference"
	KindLesson     Kind = "lesson"
	KindDeadEnd    Kind = "dead_end"
	KindCorrection Kind = "correction"
)

func (k Kind) Valid() bool {
	switch k {
	case KindFact, KindDecision, KindProcedure, KindPreference, KindLesson, KindDeadEnd, KindCorrection:
		return true
	}
	return false
}

type Status string

const (
	StatusTentative Status = "tentative"
	StatusPreferred Status = "preferred"
	StatusContested Status = "contested"
	StatusDeadEnd   Status = "dead_end"
	StatusCorrected Status = "corrected"
	StatusArchived  Status = "archived"
)

type Outcome string

const (
	OutcomeUseful    Outcome = "useful"
	OutcomeDeadEnd   Outcome = "dead_end"
	OutcomeCorrected Outcome = "corrected"
)

func (o Outcome) Valid() bool {
	switch o {
	case OutcomeUseful, OutcomeDeadEnd, OutcomeCorrected:
		return true
	}
	return false
}

const (
	ScopeGlobal  = "global"
	ScopeProject = "project"
)

type Item struct {
	ID        string
	Kind      Kind
	Title     string
	Body      string
	Tags      []string
	Scope     string
	Source    string
	Status    Status
	Score     float64
	UseCount  int
	CreatedAt time.Time
	UpdatedAt time.Time
	LastUsed  time.Time
	// Fundamental marks knowledge that stays valid across projects and
	// years. Prune never drops it, and decay never erases it.
	Fundamental bool
}

type Signal struct {
	ID         int64
	ItemID     string
	Outcome    Outcome
	Correction string
	Source     string
	At         time.Time
}
