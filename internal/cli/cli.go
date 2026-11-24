package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lendradev/aimem/internal/engine"
	"github.com/lendradev/aimem/internal/memory"
	"github.com/lendradev/aimem/internal/reflect"
	"github.com/lendradev/aimem/internal/store"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type options struct {
	dbPath  string
	jsonOut bool
}

// reorderArgs moves flags ahead of positional arguments so that
// "remember some title --kind fact" parses the same way as
// "remember --kind fact some title". The flag package stops parsing at the
// first non-flag argument, so this has to happen before Parse.
func reorderArgs(args []string) []string {
	var flags, positional []string
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			// A flag that takes a value consumes the next argument.
			if !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
			i++
			continue
		}
		positional = append(positional, a)
		i++
	}
	return append(flags, positional...)
}

// Run executes the CLI and returns the process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	cmd := args[0]
	rest := reorderArgs(args[1:])

	switch cmd {
	case "help", "-h", "--help":
		usage()
		return 0
	case "version":
		fmt.Printf("%s %s\n", currentAlias(), versionNumber)
		return 0
	}
	opts := options{}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&opts.dbPath, "db", "", "override the database path")
	fs.BoolVar(&opts.jsonOut, "json", false, "machine readable output")

	// Command specific flags, bound to local variables so they can be read directly.
	var (
		kind             string
		body             string
		scope            string
		limit            int
		tokens           int
		full             bool
		includeDead      bool
		outPath          string
		correction       string
		source           string
		idleDays         int
		signalDays       int
		staleDays        int
		allowDup         bool
		listOnly         bool
		fundamental      bool
		clearFundamental bool
		tags             stringList
		kinds            stringList
	)

	switch cmd {
	case "remember":
		fs.StringVar(&kind, "kind", string(memory.KindFact), "fact|decision|procedure|preference|lesson|dead_end|correction")
		fs.StringVar(&body, "body", "", "longer detail")
		fs.StringVar(&scope, "scope", memory.ScopeProject, "project|global")
		fs.BoolVar(&allowDup, "allow-duplicate", false, "store even when a similar item exists")
		fs.BoolVar(&fundamental, "fundamental", false, "keep permanently, prune never drops it")
		fs.Var(&tags, "tag", "repeatable tag")
	case "promote":
		fs.BoolVar(&listOnly, "list", false, "only list promotion candidates")
	case "recall":
		fs.StringVar(&scope, "scope", "all", "all|project|global")
		fs.IntVar(&limit, "limit", engine.DefaultLimit, "max items")
		fs.IntVar(&tokens, "tokens", engine.DefaultTokenBudget, "token budget")
		fs.BoolVar(&includeDead, "include-dead-ends", false, "also return dead ends and corrections")
		fs.BoolVar(&full, "full", false, "return full bodies")
		fs.Var(&kinds, "kind", "repeatable kind filter")
	case "show":
		fs.BoolVar(&full, "full", false, "show full body")
	case "feedback":
		fs.StringVar(&correction, "correction", "", "what the correct answer was")
		fs.StringVar(&source, "source", "cli", "who reported this")
	case "reflect":
		fs.StringVar(&outPath, "out", "", "output path")
		fs.StringVar(&scope, "scope", "all", "all|project|global")
	case "stats":
		fs.StringVar(&scope, "scope", "all", "all|project|global")
	case "prune":
		fs.IntVar(&idleDays, "idle-days", 180, "drop unused items older than this")
		fs.IntVar(&signalDays, "signal-days", 365, "drop signals older than this")
		fs.IntVar(&staleDays, "stale-days", engine.StaleDays, "drop non fundamental items not recalled for this long")
	case "mark":
		fs.BoolVar(&clearFundamental, "clear", false, "clear the fundamental flag instead of setting it")
	case "forget", "mcp":
		// no extra flags
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		return 2
	}

	if err := fs.Parse(rest); err != nil {
		return 2
	}

	eng, err := openEngine(opts.dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer closeEngine(eng)

	switch cmd {
	case "remember":
		return cmdRemember(eng, fs, opts, kind, body, scope, tags, allowDup, fundamental)
	case "promote":
		return cmdPromote(eng, fs, opts, listOnly)
	case "recall":
		return cmdRecall(eng, fs, opts, scope, limit, tokens, full, includeDead, kinds)
	case "show":
		return cmdShow(eng, fs, opts)
	case "forget":
		return cmdForget(eng, fs, opts)
	case "mark":
		return cmdMark(eng, fs, opts, !clearFundamental)
	case "feedback":
		return cmdFeedback(eng, fs, opts, correction, source)
	case "reflect":
		return cmdReflect(eng, fs, opts, scope, outPath)
	case "stats":
		return cmdStats(eng, fs, opts, scope)
	case "prune":
		return cmdPrune(eng, fs, opts, idleDays, signalDays, staleDays)
	case "mcp":
		return runMCP(eng)
	}
	return 0
}

const versionNumber = "0.3.1"

func usage() {
	name := currentAlias()
	fmt.Fprintf(os.Stderr, `%s - memory for AI agents

Usage:
  %s remember <title> [flags]     store a memory item
  %s recall <query> [flags]       search memory
  %s show <id> [flags]            show one item in full
  %s forget <id>... [flags]       delete items
  %s mark <id>... [flags]         set or clear the fundamental flag
  %s feedback <id> <outcome>      record useful|dead_end|corrected
  %s promote [<id>] [flags]       move a proven item to the global scope
  %s reflect [flags]              write LESSONS.md
  %s stats [flags]                row counts and database size
  %s prune [flags]                drop stale items and signals
  %s mcp                          run the MCP stdio server
  %s version

Global flags:
  --db PATH    override the database path
  --json       machine readable output
`, name, name, name, name, name, name, name, name, name, name, name, name, name)
}

func openEngine(dbPath string) (*engine.Engine, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	alias := currentAlias()
	dataDir := engine.DataDirFor(alias)
	globalPath, projectPath, err := engine.DefaultPathsFor(wd, dataDir)
	if err != nil {
		return nil, err
	}
	if dbPath != "" {
		globalPath = dbPath
		projectPath = dbPath
	}
	g, err := store.Open(globalPath)
	if err != nil {
		return nil, err
	}
	// An empty project path means there is no project scope. Opening a store
	// for it would create a database in whatever directory the user happened
	// to be standing in, so the project scope stays absent instead.
	var p *store.Store
	if projectPath != "" {
		p, err = store.Open(projectPath)
		if err != nil {
			g.Close()
			return nil, err
		}
	}
	return engine.NewEngine(g, p), nil
}

// currentAlias returns the alias this binary was installed under, falling back
// to the canonical name when the binary was renamed to something unknown.
func currentAlias() string {
	if alias, ok := engine.Alias(os.Args[0]); ok {
		return alias
	}
	if exe, err := os.Executable(); err == nil {
		if alias, ok := engine.Alias(exe); ok {
			return alias
		}
	}
	return "aimem"
}

func closeEngine(eng *engine.Engine) {
	if eng.Global != nil {
		eng.Global.Close()
	}
	if eng.Project != nil {
		eng.Project.Close()
	}
}

func cmdRemember(eng *engine.Engine, fs *flag.FlagSet, opts options, kind, body, scope string, tags []string, allowDup, fundamental bool) int {
	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: remember needs a title")
		return 2
	}
	title := strings.Join(args, " ")
	res, err := eng.Remember(engine.RememberOptions{
		Scope: scope, Kind: kind, Title: title, Body: body, Tags: tags,
		Source: "cli", AllowDuplicate: allowDup, Fundamental: fundamental,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if res.DuplicateOf != nil {
		if opts.jsonOut {
			return printJSON(map[string]any{
				"stored": false, "duplicate_of": res.DuplicateOf.ID,
				"similarity": res.DuplicateScore, "title": res.DuplicateOf.Title,
			})
		}
		fmt.Printf("not stored, similar item exists: %s (id %s, similarity %.2f)\n",
			res.DuplicateOf.Title, res.DuplicateOf.ID, res.DuplicateScore)
		fmt.Println("use --allow-duplicate to store it anyway")
		return 0
	}
	it := res.Item
	if opts.jsonOut {
		return printJSON(map[string]any{"id": it.ID, "kind": it.Kind, "scope": it.Scope, "status": it.Status, "title": it.Title})
	}
	fmt.Printf("saved %s (%s, scope %s)\n", it.ID, it.Kind, it.Scope)
	return 0
}

func cmdPromote(eng *engine.Engine, fs *flag.FlagSet, opts options, listOnly bool) int {
	args := fs.Args()
	if len(args) == 1 && !listOnly {
		it, err := eng.Promote(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		if opts.jsonOut {
			return printJSON(map[string]any{"id": it.ID, "scope": it.Scope, "title": it.Title})
		}
		fmt.Printf("promoted to global: %s (id %s)\n", it.Title, it.ID)
		return 0
	}

	promos, err := eng.FindPromotions()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if opts.jsonOut {
		type entry struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Kind   string `json:"kind"`
			Reason string `json:"reason"`
		}
		out := make([]entry, 0, len(promos))
		for _, p := range promos {
			out = append(out, entry{ID: p.Item.ID, Title: p.Item.Title, Kind: string(p.Item.Kind), Reason: p.Reason})
		}
		return printJSON(out)
	}
	if len(promos) == 0 {
		fmt.Println("no promotion candidates")
		return 0
	}
	for _, p := range promos {
		fmt.Printf("- [%s] %s (id %s, %s)\n", p.Item.Kind, p.Item.Title, p.Item.ID, p.Reason)
	}
	fmt.Println("\npromote with: aimem promote <id>")
	return 0
}

func cmdRecall(eng *engine.Engine, fs *flag.FlagSet, opts options, scope string, limit, tokens int, full, includeDead bool, kinds []string) int {
	query := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(query) == "" {
		fmt.Fprintln(os.Stderr, "error: recall needs a query")
		return 2
	}
	res, err := eng.Recall(engine.RecallOptions{
		Query:           query,
		Scope:           scope,
		Kinds:           kinds,
		Limit:           limit,
		TokenBudget:     tokens,
		IncludeDeadEnds: includeDead,
		Full:            full,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if opts.jsonOut {
		return printJSON(res)
	}
	if len(res) == 0 {
		fmt.Println("no match")
		return 0
	}
	for _, r := range res {
		fmt.Printf("- [%s] %s (id %s, score %.2f, scope %s)\n", r.Kind, r.Title, r.ID, r.Score, r.Scope)
		if r.Body != "" {
			for _, line := range strings.Split(r.Body, "\n") {
				fmt.Println("  " + line)
			}
		}
		if r.Truncated {
			fmt.Printf("  (truncated, show %s for the rest)\n", r.ID)
		}
	}
	return 0
}

func cmdShow(eng *engine.Engine, fs *flag.FlagSet, opts options) int {
	args := fs.Args()
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "error: show needs exactly one id")
		return 2
	}
	it, err := eng.Show(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if opts.jsonOut {
		return printJSON(map[string]any{
			"id": it.ID, "kind": it.Kind, "title": it.Title, "body": it.Body,
			"tags": it.Tags, "scope": it.Scope, "status": it.Status, "score": it.Score,
			"fundamental": it.Fundamental,
			"created_at":  it.CreatedAt, "updated_at": it.UpdatedAt,
		})
	}
	fmt.Printf("id:      %s\n", it.ID)
	fmt.Printf("kind:    %s\n", it.Kind)
	fmt.Printf("scope:   %s\n", it.Scope)
	fmt.Printf("status:  %s\n", it.Status)
	fmt.Printf("score:   %.4f\n", it.Score)
	fmt.Printf("tags:    %s\n", strings.Join(it.Tags, " "))
	if it.Fundamental {
		fmt.Println("kept:    fundamental (prune never drops it)")
	}
	fmt.Printf("created: %s\n", it.CreatedAt.Format(time.RFC3339))
	fmt.Printf("title:   %s\n", it.Title)
	if it.Body != "" {
		fmt.Printf("body:\n%s\n", it.Body)
	}
	return 0
}

func cmdForget(eng *engine.Engine, fs *flag.FlagSet, opts options) int {
	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: forget needs at least one id")
		return 2
	}
	n, err := eng.Forget(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if opts.jsonOut {
		return printJSON(map[string]any{"removed": n})
	}
	fmt.Printf("removed %d item(s)\n", n)
	return 0
}

func cmdMark(eng *engine.Engine, fs *flag.FlagSet, opts options, fundamental bool) int {
	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: mark needs at least one id")
		return 2
	}
	n, err := eng.Mark(args, fundamental)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if opts.jsonOut {
		return printJSON(map[string]any{"marked": n, "fundamental": fundamental})
	}
	if fundamental {
		fmt.Printf("marked %d item(s) as fundamental\n", n)
	} else {
		fmt.Printf("cleared fundamental flag on %d item(s)\n", n)
	}
	return 0
}

func cmdFeedback(eng *engine.Engine, fs *flag.FlagSet, opts options, correction, source string) int {
	args := fs.Args()
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "error: feedback needs <id> <useful|dead_end|corrected>")
		return 2
	}
	it, err := eng.Feedback(args[0], memory.Outcome(args[1]), correction, source)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if opts.jsonOut {
		return printJSON(map[string]any{"id": it.ID, "status": it.Status, "score": it.Score})
	}
	fmt.Printf("%s -> status %s, score %.4f\n", it.ID, it.Status, it.Score)
	return 0
}

func cmdReflect(eng *engine.Engine, fs *flag.FlagSet, opts options, scope, outPath string) int {
	var stores []*store.Store
	if scope == "all" || scope == memory.ScopeProject {
		stores = append(stores, eng.Project)
	}
	if scope == "all" || scope == memory.ScopeGlobal {
		stores = append(stores, eng.Global)
	}

	merged := &reflect.Report{}
	for _, st := range stores {
		if st == nil {
			continue
		}
		rep, err := reflect.Build(st, eng.Now())
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		merged.Preferred = append(merged.Preferred, rep.Preferred...)
		merged.Tentative = append(merged.Tentative, rep.Tentative...)
		merged.Contested = append(merged.Contested, rep.Contested...)
		merged.DeadEnds = append(merged.DeadEnds, rep.DeadEnds...)
		merged.Corrections = append(merged.Corrections, rep.Corrections...)
	}
	sort.SliceStable(merged.Preferred, func(i, j int) bool {
		return merged.Preferred[i].Score > merged.Preferred[j].Score
	})
	sort.SliceStable(merged.Tentative, func(i, j int) bool {
		return merged.Tentative[i].Score > merged.Tentative[j].Score
	})

	md := merged.Markdown(eng.Now())
	if outPath == "" {
		// Pick the store that matches the requested scope. Without a project
		// root there is no project store, so the global one is the only place
		// the lessons file can go.
		switch {
		case scope == memory.ScopeGlobal && eng.Global != nil:
			outPath = filepath.Join(filepath.Dir(eng.Global.Path()), "LESSONS.md")
		case eng.Project != nil:
			outPath = filepath.Join(filepath.Dir(eng.Project.Path()), "LESSONS.md")
		case eng.Global != nil:
			outPath = filepath.Join(filepath.Dir(eng.Global.Path()), "LESSONS.md")
		default:
			fmt.Fprintln(os.Stderr, "error: no scope available to write LESSONS.md")
			return 1
		}
	}
	if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if opts.jsonOut {
		return printJSON(map[string]any{"path": outPath, "bytes": len(md)})
	}
	fmt.Printf("wrote %s\n", outPath)
	return 0
}

func cmdStats(eng *engine.Engine, fs *flag.FlagSet, opts options, scope string) int {
	type entry struct {
		Scope string         `json:"scope"`
		Path  string         `json:"path"`
		Rows  map[string]int `json:"rows"`
		Size  int64          `json:"size_bytes"`
	}
	var out []entry
	add := func(scopeName string, st *store.Store) {
		if st == nil {
			return
		}
		rows, size, err := st.Stats()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return
		}
		out = append(out, entry{Scope: scopeName, Path: st.Path(), Rows: rows, Size: size})
	}
	if scope == "all" || scope == memory.ScopeProject {
		add(memory.ScopeProject, eng.Project)
	}
	if scope == "all" || scope == memory.ScopeGlobal {
		add(memory.ScopeGlobal, eng.Global)
	}
	if opts.jsonOut {
		return printJSON(out)
	}
	for _, e := range out {
		fmt.Printf("%s (%s)\n", e.Scope, e.Path)
		fmt.Printf("  size: %s\n", humanSize(e.Size))
		for _, k := range []string{"items", "signals", "relations", "embeddings"} {
			fmt.Printf("  %s: %d\n", k, e.Rows[k])
		}
	}
	return 0
}

// humanSize renders a byte count with binary units so the number stays
// readable. The JSON output keeps the raw byte count for machines.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	for _, u := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, u)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/unit)
}

func cmdPrune(eng *engine.Engine, fs *flag.FlagSet, opts options, idleDays, signalDays, staleDays int) int {
	total := 0
	for _, st := range []*store.Store{eng.Project, eng.Global} {
		if st == nil {
			continue
		}
		n, err := st.Prune(eng.Now(), idleDays, signalDays, staleDays)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		total += n
		if err := st.Compact(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
	}
	if opts.jsonOut {
		return printJSON(map[string]any{"pruned_items": total})
	}
	fmt.Printf("pruned %d item(s)\n", total)
	return 0
}

func printJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}
