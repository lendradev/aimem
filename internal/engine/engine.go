package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lendradev/aimem/internal/idgen"
	"github.com/lendradev/aimem/internal/memory"
	"github.com/lendradev/aimem/internal/score"
	"github.com/lendradev/aimem/internal/store"
)

const (
	// DefaultTokenBudget caps the approximate token cost of one recall result.
	DefaultTokenBudget = 600
	// DefaultLimit caps how many items one recall returns.
	DefaultLimit = 5
	// MaxBodyBytes is the hard cap for an item body.
	MaxBodyBytes = 32 * 1024
	// MaxTitleBytes is the hard cap for an item title.
	MaxTitleBytes = 120
	// MaxTags is the hard cap for tags on one item.
	MaxTags = 8
	// StaleDays is how long a non fundamental item survives without being
	// recalled before prune treats it as stale memory.
	StaleDays = 5 * 365
)

// Engine holds both scopes and exposes the operations the CLI and MCP server need.
type Engine struct {
	Global  *store.Store
	Project *store.Store
	Now     func() time.Time
}

func NewEngine(global, project *store.Store) *Engine {
	return &Engine{Global: global, Project: project, Now: time.Now}
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

// Store returns the store for a scope.
func (e *Engine) Store(scope string) (*store.Store, error) {
	switch scope {
	case memory.ScopeGlobal:
		if e.Global == nil {
			return nil, errors.New("global scope is not available")
		}
		return e.Global, nil
	case memory.ScopeProject:
		if e.Project == nil {
			return nil, ErrNoProjectScope
		}
		return e.Project, nil
	}
	return nil, fmt.Errorf("unknown scope %q", scope)
}

// ErrNoProjectRoot means no project marker was found in the starting
// directory or any parent, so there is no project scope to use.
var ErrNoProjectRoot = errors.New("no project root found")

// ErrNoProjectScope is returned when a command needs the project scope but
// no project root could be located.
var ErrNoProjectScope = errors.New("project scope is not available: no .git or go.mod found in this directory or any parent, and AI_MEMORY_PROJECT is not set")

// ResolveScope finds the project root by walking up for .git, then go.mod,
// then a memory data directory. When the current directory has a
// workspace/ subdirectory that is itself a repository, that subdirectory
// wins, because this project's layout keeps the git repository in workspace/
// while the project root stays outside it.
//
// It reports ErrNoProjectRoot when no marker exists anywhere. Callers must
// treat that as "there is no project scope", not as a failure to fall back
// to the current directory: falling back made every unrelated directory
// grow its own database.
func ResolveScope(startDir string) (string, error) {
	return resolveScope(startDir, DataDirName)
}

func resolveScope(startDir, dataDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	if inner := filepath.Join(dir, "workspace"); isRepoRoot(inner, dataDir) {
		return inner, nil
	}
	for {
		if isRepoRoot(dir, dataDir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoProjectRoot
		}
		dir = parent
	}
}

func isRepoRoot(dir, dataDir string) bool {
	// The legacy directory name counts as a marker too. A store left over
	// from before the rename is still a project anchor, and dropping it
	// would strand that data where it cannot be reached.
	for _, marker := range []string{".git", "go.mod", dataDir, legacyDataDirName} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

// DataDirName is the directory that holds a scope's database.
const DataDirName = ".aimem"

// legacyDataDirName is the directory name used before the rename. Its
// presence marks an existing store that has not been migrated yet.
const legacyDataDirName = ".ai-memory"

// Aliases are the alternative names this tool is installed under. The alias
// decides the data directory name and the label shown in output, so a user
// who installs it as "ai-memory" gets ".ai-memory" instead of ".aimem".
var Aliases = []string{
	"aimem",
	"ai-memory",
	"aimemory",
	"ai-memory-cli",
	"aimem-cli",
}

// Alias returns the canonical alias for a binary name. It reports false when
// the name is not one of the known aliases.
func Alias(binaryName string) (string, bool) {
	base := strings.ToLower(filepath.Base(binaryName))
	// Strip a .exe suffix so Windows resolves the same alias.
	base = strings.TrimSuffix(base, ".exe")
	for _, a := range Aliases {
		if base == a {
			return a, true
		}
	}
	return "", false
}

// DataDirFor returns the data directory name for an alias.
//
// Every alias uses the same ".aimem" directory. The alias only changes the
// name the tool is invoked as, never where data lives. A different directory
// per alias would split the user's memory across installs, which is exactly
// what this tool is meant to avoid.
func DataDirFor(alias string) string {
	return DataDirName
}

// DefaultPaths returns the global and project database paths.
//
// The project path is empty when no project root could be located. An empty
// project path means "no project scope", and the caller must not create a
// database for it.
func DefaultPaths(startDir string) (globalPath, projectPath string, err error) {
	return defaultPaths(startDir, DataDirName)
}

// DefaultPathsFor returns the database paths for a given data directory name.
func DefaultPathsFor(startDir, dataDir string) (globalPath, projectPath string, err error) {
	return defaultPaths(startDir, dataDir)
}

func defaultPaths(startDir, dataDir string) (globalPath, projectPath string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("locate home dir: %w", err)
	}
	globalHome := os.Getenv("AI_MEMORY_HOME")
	globalParent := home
	if globalHome == "" {
		globalHome = filepath.Join(home, dataDir)
	} else {
		globalParent = filepath.Dir(globalHome)
	}
	root, rootErr := resolveScope(startDir, dataDir)
	if rootErr != nil && !errors.Is(rootErr, ErrNoProjectRoot) {
		return "", "", rootErr
	}
	projectDir := os.Getenv("AI_MEMORY_PROJECT")
	projectParent := root
	if projectDir == "" {
		if rootErr != nil {
			// No project root. The project scope does not exist, so there is
			// no project path to report.
			projectPath = ""
		} else {
			projectDir = filepath.Join(root, dataDir)
		}
	} else {
		projectParent = filepath.Dir(projectDir)
	}
	if err := migrateLegacyScope(globalHome, globalParent, dataDir); err != nil {
		return "", "", err
	}
	if projectDir != "" {
		if err := migrateLegacyScope(projectDir, projectParent, dataDir); err != nil {
			return "", "", err
		}
	}
	globalPath = filepath.Join(globalHome, "data.db")
	if projectDir == "" {
		return globalPath, "", nil
	}
	projectPath = filepath.Join(projectDir, "data.db")
	if projectPath == globalPath {
		// The project root resolved to the home directory, so both scopes
		// would point at the same file. Two stores on one database corrupt
		// the scope separation, so the project scope is dropped instead.
		return globalPath, "", nil
	}
	return globalPath, projectPath, nil
}

// migrateLegacyScope renames <parent>/.ai-memory to <parent>/<dataDir> when
// the new name is not taken. The parent is the home directory for the global
// scope and the project root for the project scope. The migration is skipped
// when an environment override points somewhere else, so a custom path is
// never touched.
func migrateLegacyScope(dir, parent, dataDir string) error {
	if dir != filepath.Join(parent, dataDir) {
		return nil
	}
	legacy := filepath.Join(parent, legacyDataDirName)
	current := filepath.Join(parent, dataDir)
	if _, err := os.Stat(current); err == nil {
		return nil
	}
	if _, err := os.Stat(legacy); err != nil {
		return nil
	}
	if err := os.Rename(legacy, current); err != nil {
		return fmt.Errorf("migrate %s to %s: %w", legacy, current, err)
	}
	return nil
}

// DuplicateThreshold is the similarity above which two items are considered
// to say the same thing.
const DuplicateThreshold = 0.75

// RememberOptions controls one remember call.
type RememberOptions struct {
	Scope  string
	Kind   string
	Title  string
	Body   string
	Tags   []string
	Source string
	// AllowDuplicate stores the item even when a near-identical one exists.
	AllowDuplicate bool
	// Fundamental marks knowledge that stays valid across projects and years.
	// Prune never drops it.
	Fundamental bool
}

// RememberResult reports what happened to a remember call.
type RememberResult struct {
	Item memory.Item
	// DuplicateOf is set when an existing item says the same thing.
	DuplicateOf *memory.Item
	// DuplicateScore is how similar the duplicate is, from 0 to 1.
	DuplicateScore float64
}

// Remember stores a new item.
//
// When a similar item already exists in the scope, the call reports the
// duplicate instead of storing a second copy, unless duplicates are allowed.
// This keeps the store small, which is the whole point of the tool.
func (e *Engine) Remember(opts RememberOptions) (RememberResult, error) {
	var res RememberResult
	scope := opts.Scope
	kind := opts.Kind
	title := strings.TrimSpace(opts.Title)
	body := opts.Body
	tags := opts.Tags
	source := opts.Source

	if !memory.Kind(kind).Valid() {
		return res, fmt.Errorf("invalid kind %q", kind)
	}
	if title == "" {
		return res, errors.New("title is required")
	}
	if len(title) > MaxTitleBytes {
		return res, fmt.Errorf("title exceeds %d bytes", MaxTitleBytes)
	}
	if len(body) > MaxBodyBytes {
		return res, fmt.Errorf("body exceeds %d bytes", MaxBodyBytes)
	}
	if len(tags) > MaxTags {
		return res, fmt.Errorf("at most %d tags allowed", MaxTags)
	}
	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		clean = append(clean, t)
	}
	now := e.now()
	it := memory.Item{
		ID:          idgen.New(),
		Kind:        memory.Kind(kind),
		Title:       title,
		Body:        body,
		Tags:        clean,
		Scope:       scope,
		Source:      source,
		Status:      memory.StatusTentative,
		CreatedAt:   now,
		UpdatedAt:   now,
		LastUsed:    now,
		Fundamental: opts.Fundamental,
	}
	st, err := e.Store(scope)
	if err != nil {
		if scope == memory.ScopeProject {
			return res, ErrNoProjectScope
		}
		return res, err
	}

	if !opts.AllowDuplicate {
		if dup, sim := e.findDuplicate(st, scope, title, body, it.ID); dup != nil {
			res.DuplicateOf = dup
			res.DuplicateScore = sim
			return res, nil
		}
	}

	if err := st.Insert(it); err != nil {
		return res, err
	}
	res.Item = it
	return res, nil
}

// findDuplicate looks for an existing item that says the same thing as the
// new one. The project scope is checked first, then the global scope, so a
// project item wins over a global one.
//
// Token sets are built once per item, so a large body is tokenized once
// instead of once per comparison.
func (e *Engine) findDuplicate(st *store.Store, scope, title, body, excludeID string) (*memory.Item, float64) {
	existing, err := st.List(scope)
	if err != nil {
		return nil, 0
	}
	newTokens := score.TokenSet(title + " " + body)
	var best *memory.Item
	bestScore := 0.0
	for i := range existing {
		if existing[i].ID == excludeID {
			continue
		}
		sim := score.SetDuplicateScore(newTokens, score.TokenSet(existing[i].Title+" "+existing[i].Body))
		if sim > bestScore {
			bestScore = sim
			best = &existing[i]
		}
	}
	if bestScore < DuplicateThreshold {
		return nil, 0
	}
	return best, bestScore
}

// Show returns one item by id, looking in the project scope first.
func (e *Engine) Show(id string) (memory.Item, error) {
	if e.Project != nil {
		if it, err := e.Project.Get(id); err == nil {
			return it, nil
		} else if !errors.Is(err, store.ErrNotFound) {
			return memory.Item{}, err
		}
	}
	if e.Global != nil {
		it, err := e.Global.Get(id)
		if errors.Is(err, store.ErrNotFound) {
			return memory.Item{}, store.ErrNotFound
		}
		return it, err
	}
	return memory.Item{}, store.ErrNotFound
}

// Forget removes items by id. It never touches the global store implicitly.
func (e *Engine) Forget(ids []string) (int, error) {
	removed := 0
	for _, id := range ids {
		found := false
		for _, st := range []*store.Store{e.Project, e.Global} {
			if st == nil {
				continue
			}
			err := st.Delete(id)
			if err == nil {
				found = true
				removed++
				break
			}
			if !errors.Is(err, store.ErrNotFound) {
				return removed, err
			}
		}
		if !found {
			return removed, fmt.Errorf("item %s not found", id)
		}
	}
	return removed, nil
}

// Feedback records an outcome signal for an item and refreshes its score and status.
func (e *Engine) Feedback(id string, outcome memory.Outcome, correction, source string) (memory.Item, error) {
	if !outcome.Valid() {
		return memory.Item{}, fmt.Errorf("invalid outcome %q", outcome)
	}
	it, err := e.Show(id)
	if err != nil {
		return memory.Item{}, err
	}
	st, err := e.Store(it.Scope)
	if err != nil {
		return memory.Item{}, err
	}
	now := e.now()
	if err := st.AddSignal(memory.Signal{
		ItemID:     id,
		Outcome:    outcome,
		Correction: correction,
		Source:     source,
		At:         now,
	}); err != nil {
		return memory.Item{}, err
	}
	signals, err := st.Signals(id)
	if err != nil {
		return memory.Item{}, err
	}
	it.Score = score.TotalScore(signals, it.UseCount, it.LastUsed, now)
	it.Status = score.DeriveStatus(signals)
	it.UpdatedAt = now
	if err := st.Update(it); err != nil {
		return memory.Item{}, err
	}
	return it, nil
}

// RecallResult is one item in a recall response.
type RecallResult struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Status    string   `json:"status"`
	Score     float64  `json:"score"`
	Scope     string   `json:"scope"`
	Truncated bool     `json:"truncated,omitempty"`
	// Fundamental marks knowledge that survives prune.
	Fundamental bool `json:"fundamental,omitempty"`
}

// RecallOptions controls one recall call.
type RecallOptions struct {
	Query           string
	Scope           string
	Kinds           []string
	Limit           int
	TokenBudget     int
	IncludeDeadEnds bool
	Full            bool
}

// Recall searches both scopes and returns the best items within the token budget.
func (e *Engine) Recall(opts RecallOptions) ([]RecallResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = DefaultLimit
	}
	if opts.TokenBudget <= 0 {
		opts.TokenBudget = DefaultTokenBudget
	}
	match := store.FTS5Match(opts.Query)
	if match == "" {
		return nil, nil
	}

	type candidate struct {
		item memory.Item
		lex  float64
	}
	var cands []candidate

	scopes := []string{}
	switch opts.Scope {
	case memory.ScopeGlobal:
		scopes = []string{memory.ScopeGlobal}
	case memory.ScopeProject:
		scopes = []string{memory.ScopeProject}
	default:
		scopes = []string{memory.ScopeProject, memory.ScopeGlobal}
	}

	for _, scope := range scopes {
		st, err := e.Store(scope)
		if err != nil {
			continue
		}
		hits, err := st.Search(match, 50)
		if err != nil {
			return nil, fmt.Errorf("search %s: %w", scope, err)
		}
		for _, h := range hits {
			it, err := st.Get(h.ID)
			if err != nil {
				continue
			}
			if !opts.IncludeDeadEnds && (it.Status == memory.StatusDeadEnd || it.Status == memory.StatusCorrected) {
				continue
			}
			cands = append(cands, candidate{item: it, lex: h.Rank})
		}
	}

	// Fallback: FTS5 needs whole token matches, so a natural language query
	// often shares no token with the stored text. Substring match recovers
	// those cases without giving up the ranking FTS5 provides.
	if len(cands) == 0 {
		terms := store.NormalizeQuery(opts.Query)
		for _, scope := range scopes {
			st, err := e.Store(scope)
			if err != nil {
				continue
			}
			hits, err := st.SearchLike(terms, 50)
			if err != nil {
				continue
			}
			for _, h := range hits {
				it, err := st.Get(h.ID)
				if err != nil {
					continue
				}
				if !opts.IncludeDeadEnds && (it.Status == memory.StatusDeadEnd || it.Status == memory.StatusCorrected) {
					continue
				}
				cands = append(cands, candidate{item: it, lex: h.Rank})
			}
		}
	}

	if len(cands) == 0 {
		return nil, nil
	}

	// bm25 is negative, more negative means more relevant. Normalize to [0,1].
	minRank, maxRank := cands[0].lex, cands[0].lex
	for _, c := range cands {
		if c.lex < minRank {
			minRank = c.lex
		}
		if c.lex > maxRank {
			maxRank = c.lex
		}
	}
	rankRange := maxRank - minRank

	now := e.now()

	// Rank on the live score: outcome evidence plus usage reinforcement.
	// Reading the stored score alone would ignore how often an item was
	// actually useful.
	for i := range cands {
		st, err := e.Store(cands[i].item.Scope)
		if err != nil {
			cands[i].item.Score = 0
			continue
		}
		sigs, err := st.Signals(cands[i].item.ID)
		if err != nil {
			sigs = nil
		}
		cands[i].item.Score = score.TotalScore(sigs, cands[i].item.UseCount, cands[i].item.LastUsed, now)
	}

	maxUse, minUse := cands[0].item.Score, cands[0].item.Score
	for _, c := range cands {
		if c.item.Score > maxUse {
			maxUse = c.item.Score
		}
		if c.item.Score < minUse {
			minUse = c.item.Score
		}
	}
	useRange := maxUse - minUse

	// A project item outranks a global item with the same relevance: the
	// project scope is more specific, so it wins ties and near-ties.
	const scopeBoost = 0.05

	ranked := make([]candidate, 0, len(cands))
	for _, c := range cands {
		lex := 0.0
		if rankRange > 0 {
			lex = (maxRank - c.lex) / rankRange
		} else {
			lex = 1
		}
		use := 0.0
		if useRange > 0 {
			use = (c.item.Score - minUse) / useRange
		} else if c.item.Score > 0 {
			use = 1
		}
		rec := score.Recency(c.item.UpdatedAt, now)
		final := 1.0*lex + 0.6*use + 0.2*rec
		if c.item.Scope == memory.ScopeProject {
			final += scopeBoost
		}
		c.item.Score = final
		ranked = append(ranked, c)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].item.Score != ranked[j].item.Score {
			return ranked[i].item.Score > ranked[j].item.Score
		}
		return ranked[i].item.ID < ranked[j].item.ID
	})

	kindSet := map[string]bool{}
	for _, k := range opts.Kinds {
		kindSet[k] = true
	}

	results := []RecallResult{}
	used := 0
	for _, c := range ranked {
		if len(results) >= opts.Limit {
			break
		}
		if len(kindSet) > 0 && !kindSet[string(c.item.Kind)] {
			continue
		}
		body := c.item.Body
		truncated := false
		if !opts.Full {
			titleCost := EstimateTokens(c.item.Title)
			cost := titleCost + EstimateTokens(body)
			if used+cost > opts.TokenBudget && len(results) > 0 {
				break
			}
			if used+cost > opts.TokenBudget {
				// Keep as much of the body as the budget allows instead of
				// dropping it, so a large item still carries usable detail.
				remaining := opts.TokenBudget - used - titleCost
				if remaining <= 0 {
					body = ""
				} else {
					body = truncateToTokens(body, remaining)
				}
				truncated = true
			}
		}
		used += EstimateTokens(c.item.Title) + EstimateTokens(body)
		results = append(results, RecallResult{
			ID:          c.item.ID,
			Kind:        string(c.item.Kind),
			Title:       c.item.Title,
			Body:        body,
			Tags:        c.item.Tags,
			Status:      string(c.item.Status),
			Score:       round3(c.item.Score),
			Scope:       c.item.Scope,
			Truncated:   truncated,
			Fundamental: c.item.Fundamental,
		})
	}

	// Record usage so unused items decay over time, and refresh the stored
	// score so the next recall ranks this item slightly higher.
	for _, r := range results {
		st, err := e.Store(r.Scope)
		if err != nil {
			continue
		}
		if err := st.Touch(r.ID, now); err != nil {
			continue
		}
		it, err := st.Get(r.ID)
		if err != nil {
			continue
		}
		sigs, err := st.Signals(r.ID)
		if err != nil {
			sigs = nil
		}
		it.Score = score.TotalScore(sigs, it.UseCount, it.LastUsed, now)
		_ = st.Update(it)
	}
	return results, nil
}

// EstimateTokens approximates token count without a tokenizer.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return len(s)/4 + 4
}

// truncateToTokens cuts text down to roughly the given token count, keeping
// whole words so the result stays readable.
func truncateToTokens(s string, tokens int) string {
	if tokens <= 0 || s == "" {
		return ""
	}
	limit := (tokens - 4) * 4
	if limit <= 0 {
		return ""
	}
	if len(s) <= limit {
		return s
	}
	cut := s[:limit]
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return cut
}

// Promotion is a project item that proved useful often enough to be worth
// keeping in the global scope.
type Promotion struct {
	Item      memory.Item
	Reason    string
	UseCount  int
	SignalCnt int
}

// PromotionThreshold is the number of distinct confirmations a project item
// needs before it is proposed for the global scope.
const PromotionThreshold = 2

// FindPromotions returns project items that are worth copying to the global
// scope. It reports candidates only; the caller decides whether to promote.
//
// A project item qualifies when it was confirmed as useful from at least two
// distinct sources and it has been recalled more than once. Both conditions
// together mean the item generalises beyond a single conversation.
func (e *Engine) FindPromotions() ([]Promotion, error) {
	if e.Project == nil || e.Global == nil {
		return nil, nil
	}
	items, err := e.Project.List(memory.ScopeProject)
	if err != nil {
		return nil, err
	}
	globalItems, err := e.Global.List(memory.ScopeGlobal)
	if err != nil {
		return nil, err
	}
	globalTitles := make(map[string]bool, len(globalItems))
	for _, it := range globalItems {
		globalTitles[it.Title] = true
	}

	signals, err := e.Project.AllSignals()
	if err != nil {
		return nil, err
	}
	byItem := map[string][]memory.Signal{}
	for _, s := range signals {
		byItem[s.ItemID] = append(byItem[s.ItemID], s)
	}

	var out []Promotion
	for _, it := range items {
		if globalTitles[it.Title] {
			continue
		}
		sigs := byItem[it.ID]
		sources := map[string]bool{}
		useful := 0
		for _, s := range sigs {
			if s.Outcome == memory.OutcomeUseful {
				useful++
				sources[s.Source] = true
			}
		}
		if len(sources) < PromotionThreshold || it.UseCount < 2 {
			continue
		}
		out = append(out, Promotion{
			Item:      it,
			Reason:    fmt.Sprintf("confirmed by %d sources and recalled %d times", len(sources), it.UseCount),
			UseCount:  it.UseCount,
			SignalCnt: useful,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UseCount != out[j].UseCount {
			return out[i].UseCount > out[j].UseCount
		}
		return out[i].Item.ID < out[j].Item.ID
	})
	return out, nil
}

// Promote copies a project item into the global scope and removes it from the
// project scope. The global copy keeps the signal history, so the score and
// status carry over.
func (e *Engine) Promote(id string) (memory.Item, error) {
	if e.Project == nil || e.Global == nil {
		return memory.Item{}, errors.New("promotion needs both scopes")
	}
	it, err := e.Project.Get(id)
	if err != nil {
		return memory.Item{}, err
	}
	if it.Scope != memory.ScopeProject {
		return memory.Item{}, fmt.Errorf("item %s is not in the project scope", id)
	}

	sigs, err := e.Project.Signals(id)
	if err != nil {
		return memory.Item{}, err
	}
	now := e.now()
	global := it
	global.Scope = memory.ScopeGlobal
	global.ID = idgen.New()
	global.CreatedAt = now
	global.UpdatedAt = now
	global.Score = score.TotalScore(sigs, it.UseCount, it.LastUsed, now)
	if err := e.Global.Insert(global); err != nil {
		return memory.Item{}, err
	}
	for _, s := range sigs {
		s.ItemID = global.ID
		s.ID = 0
		if err := e.Global.AddSignal(s); err != nil {
			return memory.Item{}, err
		}
	}
	if err := e.Project.Delete(id); err != nil {
		return memory.Item{}, err
	}
	return global, nil
}

func round3(v float64) float64 {
	return float64(int64(v*1000+0.5)) / 1000
}
