package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		fail("open", err)
	}
	defer db.Close()

	var version string
	if err := db.QueryRow("select sqlite_version()").Scan(&version); err != nil {
		fail("version", err)
	}
	fmt.Println("sqlite_version:", version)

	const tokenizer = "porter unicode61 remove_diacritics 2"

	if _, err := db.Exec(`create virtual table fts using fts5(title, body, tags, tokenize="` + tokenizer + `")`); err != nil {
		fail("create fts5", err)
	}
	fmt.Println("ok: fts5 virtual table")

	rows := [][]any{
		{"Go build cache", "module cache lives in GOMODCACHE", "go docker"},
		{"Cafe moka", "kopi dengan susu การจัดการหน่วยความจำ", "minuman kopi"},
		{"Docker build", "cache invalidation strategy", "docker build"},
	}
	for _, r := range rows {
		if _, err := db.Exec(`insert into fts(title, body, tags) values(?, ?, ?)`, r[0], r[1], r[2]); err != nil {
			fail("insert", err)
		}
	}
	fmt.Println("ok: insert")

	count := func(label, query string, want int) {
		var n int
		if err := db.QueryRow(`select count(*) from fts where fts match ?`, query).Scan(&n); err != nil {
			fail(label, err)
		}
		if n != want {
			fail(label, fmt.Errorf("query %q got %d rows, want %d", query, n, want))
		}
		fmt.Printf("ok: %-24s rows=%d\n", label, n)
	}

	count("bm25 match", "cache", 2)
	count("diacritic folding", "cafe", 1)
	count("accented query", "café", 1)

	// unicode61 does not segment scripts without spaces: a run of Thai characters
	// becomes one token, so a query must match the whole run, and a prefix of it
	// does not match.
	var cjk int
	if err := db.QueryRow(`select count(*) from fts where fts match ?`, "การจัดการหน่วยความจำ").Scan(&cjk); err != nil {
		fail("thai exact", err)
	}
	fmt.Printf("ok: %-24s rows=%d\n", "thai exact run", cjk)
	if err := db.QueryRow(`select count(*) from fts where fts match ?`, "การจัดการ").Scan(&cjk); err != nil {
		fail("thai prefix", err)
	}
	fmt.Printf("ok: %-24s rows=%d\n", "thai prefix (no match)", cjk)

	// BM25 ranking on a realistic corpus: a strong hit must outrank a weak one.
	for i := 0; i < 200; i++ {
		if _, err := db.Exec(`insert into fts(title, body, tags) values(?, ?, ?)`, fmt.Sprintf("Row %d unrelated", i), fmt.Sprintf("lorem ipsum dolor sit amet %d", i), "filler"); err != nil {
			fail("filler insert", err)
		}
	}
	if _, err := db.Exec(`insert into fts(title, body, tags) values('Docker cache strategy','cache cache cache invalidation','docker')`); err != nil {
		fail("strong hit insert", err)
	}
	if _, err := db.Exec(`insert into fts(title, body, tags) values('Unrelated title','mentions cache once','misc')`); err != nil {
		fail("weak hit insert", err)
	}

	type hit struct {
		title string
		rank  float64
	}
	rowsOut, err := db.Query(`select title, bm25(fts) from fts where fts match 'cache' order by bm25(fts)`)
	if err != nil {
		fail("bm25 rank", err)
	}
	defer rowsOut.Close()
	var hits []hit
	for rowsOut.Next() {
		var h hit
		if err := rowsOut.Scan(&h.title, &h.rank); err != nil {
			fail("bm25 scan", err)
		}
		hits = append(hits, h)
	}
	if len(hits) != 4 {
		fail("bm25 rank", fmt.Errorf("got %d hits, want 4", len(hits)))
	}
	if hits[0].title != "Docker cache strategy" {
		fail("bm25 order", fmt.Errorf("top hit is %q, want the strong match", hits[0].title))
	}
	if hits[0].rank >= hits[1].rank {
		fail("bm25 order", fmt.Errorf("strong hit rank %.6f is not below weak hit %.6f", hits[0].rank, hits[1].rank))
	}
	fmt.Printf("ok: %-24s first=%q rank=%.4f second=%q rank=%.4f\n", "bm25 ordering", hits[0].title, hits[0].rank, hits[1].title, hits[1].rank)

	// External-content FTS table kept in sync by triggers.
	_, err = db.Exec(`create table base(id integer primary key, title text, body text, tags text)`)
	if err != nil {
		fail("create base", err)
	}
	_, err = db.Exec(`create virtual table fts_ext using fts5(title, body, tags, content='base', content_rowid='id')`)
	if err != nil {
		fail("create fts_ext", err)
	}
	for _, t := range []string{
		`create trigger base_ai after insert on base begin insert into fts_ext(rowid, title, body, tags) values (new.id, new.title, new.body, new.tags); end`,
		`create trigger base_ad after delete on base begin insert into fts_ext(fts_ext, rowid, title, body, tags) values('delete', old.id, old.title, old.body, old.tags); end`,
		`create trigger base_au after update on base begin insert into fts_ext(fts_ext, rowid, title, body, tags) values('delete', old.id, old.title, old.body, old.tags); insert into fts_ext(rowid, title, body, tags) values (new.id, new.title, new.body, new.tags); end`,
	} {
		if _, err := db.Exec(t); err != nil {
			fail("trigger", err)
		}
	}
	fmt.Println("ok: sync triggers")

	if _, err := db.Exec(`insert into base(title, body, tags) values('Keputusan','pakai Go bukan Rust','keputusan')`); err != nil {
		fail("trigger insert", err)
	}
	var n int
	if err := db.QueryRow(`select count(*) from fts_ext where fts_ext match 'Rust'`).Scan(&n); err != nil {
		fail("trigger query", err)
	}
	if n != 1 {
		fail("trigger query", fmt.Errorf("got %d rows, want 1", n))
	}
	fmt.Printf("ok: %-24s rows=%d\n", "trigger insert visible", n)

	if _, err := db.Exec(`update base set body = 'pakai Go dan Rust' where id = 1`); err != nil {
		fail("trigger update", err)
	}
	if err := db.QueryRow(`select count(*) from fts_ext where fts_ext match 'Rust'`).Scan(&n); err != nil {
		fail("trigger update query", err)
	}
	if n != 1 {
		fail("trigger update query", fmt.Errorf("got %d rows, want 1", n))
	}
	fmt.Printf("ok: %-24s rows=%d\n", "trigger update visible", n)

	if _, err := db.Exec(`delete from base`); err != nil {
		fail("trigger delete", err)
	}
	if err := db.QueryRow(`select count(*) from fts_ext where fts_ext match 'Rust'`).Scan(&n); err != nil {
		fail("trigger delete query", err)
	}
	if n != 0 {
		fail("trigger delete query", fmt.Errorf("got %d rows, want 0", n))
	}
	fmt.Printf("ok: %-24s rows=%d\n", "trigger delete visible", n)

	// Raw user text must not reach the FTS5 parser unescaped.
	for _, raw := range []string{"go-build", "a:b", "(cache", "col:cache", "\"unterminated"} {
		var rawN int
		if err := db.QueryRow(`select count(*) from fts where fts match ?`, raw).Scan(&rawN); err == nil {
			fail("raw query safety", fmt.Errorf("raw query %q parsed without error, expected rejection", raw))
		}
	}
	fmt.Println("ok: raw user queries rejected")

	fmt.Println("SPIKE PASS")
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", step, err)
	os.Exit(1)
}
