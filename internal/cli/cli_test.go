package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withEnv points both scopes at a temp dir and returns the dir.
func withEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AI_MEMORY_HOME", filepath.Join(dir, "home"))
	t.Setenv("AI_MEMORY_PROJECT", filepath.Join(dir, "proj"))
	return dir
}

func run(t *testing.T, args ...string) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := Run(args)
	w.Close()
	os.Stdout = old
	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	if code != 0 {
		t.Fatalf("command %v exited with %d, output %q", args, code, string(buf[:n]))
	}
	return string(buf[:n])
}

func TestCLIRememberShowRecall(t *testing.T) {
	withEnv(t)
	out := run(t, "remember", "pakai Go bukan Rust", "--kind", "decision", "--body", "keputusan user", "--tag", "keputusan")
	if !strings.Contains(out, "saved ") {
		t.Fatalf("unexpected output %q", out)
	}
	id := strings.Fields(out)[1]

	out = run(t, "show", id)
	if !strings.Contains(out, "pakai Go bukan Rust") {
		t.Fatalf("show output %q", out)
	}

	out = run(t, "recall", "Rust")
	if !strings.Contains(out, "pakai Go bukan Rust") {
		t.Fatalf("recall output %q", out)
	}
}

func TestCLIRecallJSON(t *testing.T) {
	withEnv(t)
	run(t, "remember", "json item", "--kind", "fact")
	out := run(t, "recall", "json", "--json")
	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
	if items[0]["kind"] != "fact" {
		t.Fatalf("got %+v", items[0])
	}
}

func TestCLIFeedbackAndReflect(t *testing.T) {
	dir := withEnv(t)
	out := run(t, "remember", "reflect item", "--kind", "fact")
	id := strings.Fields(out)[1]

	out = run(t, "feedback", id, "useful")
	if !strings.Contains(out, "tentative") {
		t.Fatalf("feedback output %q", out)
	}
	out = run(t, "feedback", id, "useful", "--source", "second-session")
	if !strings.Contains(out, "preferred") {
		t.Fatalf("feedback output %q", out)
	}

	out = run(t, "reflect")
	if !strings.Contains(out, "LESSONS.md") {
		t.Fatalf("reflect output %q", out)
	}
	path := filepath.Join(dir, "proj", "LESSONS.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "reflect item") {
		t.Fatalf("lessons file missing item:\n%s", data)
	}
}

func TestCLIStatsAndForget(t *testing.T) {
	withEnv(t)
	out := run(t, "remember", "forgettable")
	id := strings.Fields(out)[1]
	out = run(t, "stats")
	if !strings.Contains(out, "items: 1") {
		t.Fatalf("stats output %q", out)
	}
	out = run(t, "forget", id)
	if !strings.Contains(out, "removed 1") {
		t.Fatalf("forget output %q", out)
	}
	out = run(t, "stats")
	if !strings.Contains(out, "items: 0") {
		t.Fatalf("stats after forget %q", out)
	}
}

func TestCLIStatsShowsAbbreviatedSize(t *testing.T) {
	withEnv(t)
	out := run(t, "stats")
	if !strings.Contains(out, "size: ") {
		t.Fatalf("stats output %q", out)
	}
	if strings.Contains(out, "bytes") {
		t.Fatalf("stats still prints raw bytes %q", out)
	}
	if !strings.Contains(out, " B") && !strings.Contains(out, " KiB") && !strings.Contains(out, " MiB") {
		t.Fatalf("stats size not abbreviated %q", out)
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{1073741824, "1.0 GiB"},
	}
	for _, c := range cases {
		if got := humanSize(c.in); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCLIPrune(t *testing.T) {
	withEnv(t)
	run(t, "remember", "stale item")
	out := run(t, "prune", "--idle-days", "0")
	if !strings.Contains(out, "pruned") {
		t.Fatalf("prune output %q", out)
	}
}

func TestCLIRememberFundamental(t *testing.T) {
	withEnv(t)
	out := run(t, "remember", "gravity is universal", "--fundamental")
	id := strings.Fields(out)[1]
	out = run(t, "show", id)
	if !strings.Contains(out, "fundamental") {
		t.Fatalf("show output %q", out)
	}
	run(t, "prune", "--idle-days", "0", "--stale-days", "0")
	out = run(t, "show", id)
	if !strings.Contains(out, "fundamental") {
		t.Fatalf("fundamental item was pruned: %q", out)
	}
}

// Without the fundamental flag an item is ordinary memory and goes stale.
func TestCLIPruneDropsOrdinaryItem(t *testing.T) {
	withEnv(t)
	run(t, "remember", "ordinary note")
	out := run(t, "prune", "--idle-days", "0", "--stale-days", "0")
	if !strings.Contains(out, "pruned 1") {
		t.Fatalf("prune output %q", out)
	}
	out = run(t, "stats")
	if !strings.Contains(out, "items: 0") {
		t.Fatalf("ordinary item survived prune: %q", out)
	}
}

func TestCLIHelpAndVersion(t *testing.T) {
	if code := Run([]string{"version"}); code != 0 {
		t.Fatalf("version exited %d", code)
	}
	if code := Run([]string{"help"}); code != 0 {
		t.Fatalf("help exited %d", code)
	}
	if code := Run([]string{"bogus"}); code != 2 {
		t.Fatalf("unknown command exited %d, want 2", code)
	}
}

func TestCLIRecallNoMatch(t *testing.T) {
	withEnv(t)
	out := run(t, "recall", "nothingmatcheshere")
	if !strings.Contains(out, "no match") {
		t.Fatalf("recall output %q", out)
	}
}

func TestCLIGlobalScopeSeparate(t *testing.T) {
	withEnv(t)
	run(t, "remember", "global preference", "--scope", "global", "--kind", "preference")
	out := run(t, "recall", "global preference", "--scope", "project")
	if !strings.Contains(out, "no match") {
		t.Fatalf("project scope saw the global item: %q", out)
	}
	out = run(t, "recall", "global preference", "--scope", "global")
	if !strings.Contains(out, "global preference") {
		t.Fatalf("global scope missed the item: %q", out)
	}
}

// A directory that is not a project must not grow a database. The old
// fallback pointed the project scope at the current directory, so every
// unrelated folder picked up a 92 KB .aimem directory.
func TestCLIDoesNotCreateDatabaseOutsideAProject(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_MEMORY_HOME", filepath.Join(dir, "home"))
	t.Setenv("AI_MEMORY_PROJECT", "")

	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(plain); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })

	run(t, "stats")

	entries, err := os.ReadDir(plain)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("files created outside a project: %v", names)
	}
}

// Remembering into the project scope needs a project. Without one the command
// must fail loudly instead of silently writing to the global scope.
func TestCLIRememberWithoutProjectFails(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_MEMORY_HOME", filepath.Join(dir, "home"))
	t.Setenv("AI_MEMORY_PROJECT", "")

	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(plain); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })

	if code := Run([]string{"remember", "no project here"}); code == 0 {
		t.Fatal("remember succeeded without a project scope")
	}
}
