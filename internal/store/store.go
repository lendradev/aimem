package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/lendradev/aimem/internal/memory"
)

const schema = `
CREATE TABLE IF NOT EXISTS items (
  id          TEXT PRIMARY KEY,
  kind        TEXT NOT NULL,
  title       TEXT NOT NULL,
  body        TEXT NOT NULL DEFAULT '',
  tags        TEXT NOT NULL DEFAULT '',
  scope       TEXT NOT NULL,
  source      TEXT NOT NULL DEFAULT 'cli',
  status      TEXT NOT NULL DEFAULT 'tentative',
  score       REAL NOT NULL DEFAULT 0,
  use_count   INTEGER NOT NULL DEFAULT 0,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL,
  last_used   INTEGER NOT NULL DEFAULT 0,
  fundamental INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS signals (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id     TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  outcome     TEXT NOT NULL,
  correction  TEXT NOT NULL DEFAULT '',
  source      TEXT NOT NULL DEFAULT '',
  at          INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS relations (
  kind TEXT NOT NULL, src TEXT NOT NULL, dst TEXT NOT NULL,
  PRIMARY KEY (kind, src, dst)
);

CREATE TABLE IF NOT EXISTS embeddings (
  item_id  TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  model    TEXT NOT NULL,
  dim      INTEGER NOT NULL,
  vector   BLOB NOT NULL,
  PRIMARY KEY (item_id, model)
);

CREATE INDEX IF NOT EXISTS items_scope_status ON items(scope, status);
CREATE INDEX IF NOT EXISTS items_kind_scope ON items(kind, scope);
CREATE INDEX IF NOT EXISTS signals_item_at ON signals(item_id, at);

CREATE VIRTUAL TABLE IF NOT EXISTS items_fts USING fts5(
  title, body, tags,
  content='items', content_rowid='rowid',
  tokenize="porter unicode61 remove_diacritics 2"
);

CREATE VIRTUAL TABLE IF NOT EXISTS items_tri USING fts5(
  title, body,
  content='items', content_rowid='rowid',
  tokenize="trigram"
);

CREATE TRIGGER IF NOT EXISTS items_ai AFTER INSERT ON items BEGIN
  INSERT INTO items_fts(rowid, title, body, tags) VALUES (new.rowid, new.title, new.body, new.tags);
  INSERT INTO items_tri(rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;

CREATE TRIGGER IF NOT EXISTS items_ad AFTER DELETE ON items BEGIN
  INSERT INTO items_fts(items_fts, rowid, title, body, tags) VALUES('delete', old.rowid, old.title, old.body, old.tags);
  INSERT INTO items_tri(items_tri, rowid, title, body) VALUES('delete', old.rowid, old.title, old.body);
END;

CREATE TRIGGER IF NOT EXISTS items_au AFTER UPDATE ON items BEGIN
  INSERT INTO items_fts(items_fts, rowid, title, body, tags) VALUES('delete', old.rowid, old.title, old.body, old.tags);
  INSERT INTO items_fts(rowid, title, body, tags) VALUES (new.rowid, new.title, new.body, new.tags);
  INSERT INTO items_tri(items_tri, rowid, title, body) VALUES('delete', old.rowid, old.title, old.body);
  INSERT INTO items_tri(rowid, title, body) VALUES (new.rowid, new.title, new.body);
END;
`

// Store is one SQLite database holding one memory scope.
type Store struct {
	db   *sql.DB
	path string
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=auto_vacuum(INCREMENTAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	// auto_vacuum only takes effect on an empty database, so it must be set
	// before the first table is created.
	if _, err := db.Exec(`PRAGMA auto_vacuum=INCREMENTAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set auto_vacuum: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	if _, err := db.Exec(`PRAGMA synchronous=NORMAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set synchronous: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

// migrate adds columns that older database files do not have yet. SQLite has
// no "ADD COLUMN IF NOT EXISTS", so the column list is checked first.
func migrate(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(items)`)
	if err != nil {
		return err
	}
	hasFundamental := false
	for rows.Next() {
		var (
			cid        int
			name, typ  string
			notNull    int
			dflt       sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "fundamental" {
			hasFundamental = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if hasFundamental {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE items ADD COLUMN fundamental INTEGER NOT NULL DEFAULT 0`)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Compact reclaims free pages in the database file.
func (s *Store) Compact() error {
	_, err := s.db.Exec(`PRAGMA incremental_vacuum`)
	return err
}

// Prune drops items that have not been used within the given window and
// signals older than the signal window. It keeps the database small without
// touching items that are still in play.
//
// Fundamental items are never dropped: they hold knowledge that stays valid
// across projects and years, so aging them out would lose permanent context.
//
// A stale window handles the rest. Items that are no longer fundamental and
// have not been recalled for that long are dropped even when they were used
// once, because a fact nobody looked up for years is no longer worth its
// storage.
func (s *Store) Prune(now time.Time, itemIdleDays, signalDays, staleDays int) (int, error) {
	itemCutoff := now.AddDate(0, 0, -itemIdleDays).Unix()
	signalCutoff := now.AddDate(0, 0, -signalDays).Unix()

	res, err := s.db.Exec(`DELETE FROM items WHERE fundamental = 0 AND use_count = 0 AND last_used <= ?`, itemCutoff)
	if err != nil {
		return 0, err
	}
	items, _ := res.RowsAffected()

	if staleDays >= 0 {
		staleCutoff := now.AddDate(0, 0, -staleDays).Unix()
		res, err := s.db.Exec(`DELETE FROM items WHERE fundamental = 0 AND last_used <= ?`, staleCutoff)
		if err != nil {
			return int(items), err
		}
		n, _ := res.RowsAffected()
		items += n
	}

	if _, err := s.db.Exec(`DELETE FROM signals WHERE at < ?`, signalCutoff); err != nil {
		return int(items), err
	}
	return int(items), nil
}

// Stats reports row counts and the file size for one store.
func (s *Store) Stats() (map[string]int, int64, error) {
	out := map[string]int{}
	for _, t := range []string{"items", "signals", "relations", "embeddings"} {
		var n int
		if err := s.db.QueryRow(`SELECT count(*) FROM ` + t).Scan(&n); err != nil {
			return nil, 0, err
		}
		out[t] = n
	}
	var size int64
	if fi, err := os.Stat(s.path); err == nil {
		size = fi.Size()
	}
	return out, size, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Path() string { return s.path }

var ErrNotFound = errors.New("item not found")

func (s *Store) Insert(it memory.Item) error {
	_, err := s.db.Exec(
		`INSERT INTO items (id, kind, title, body, tags, scope, source, status, score, use_count, created_at, updated_at, last_used, fundamental)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.ID, string(it.Kind), it.Title, it.Body, strings.Join(it.Tags, " "), it.Scope, it.Source,
		string(it.Status), it.Score, it.UseCount, it.CreatedAt.Unix(), it.UpdatedAt.Unix(), it.LastUsed.Unix(),
		boolToInt(it.Fundamental),
	)
	return err
}

func (s *Store) Update(it memory.Item) error {
	res, err := s.db.Exec(
		`UPDATE items SET kind=?, title=?, body=?, tags=?, status=?, score=?, updated_at=?, last_used=?, fundamental=? WHERE id=?`,
		string(it.Kind), it.Title, it.Body, strings.Join(it.Tags, " "), string(it.Status), it.Score,
		it.UpdatedAt.Unix(), it.LastUsed.Unix(), boolToInt(it.Fundamental), it.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Get(id string) (memory.Item, error) {
	row := s.db.QueryRow(`SELECT id, kind, title, body, tags, scope, source, status, score, use_count, created_at, updated_at, last_used, fundamental FROM items WHERE id=?`, id)
	it, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Item{}, ErrNotFound
	}
	return it, err
}

func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM items WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) List(scope string) ([]memory.Item, error) {
	rows, err := s.db.Query(`SELECT id, kind, title, body, tags, scope, source, status, score, use_count, created_at, updated_at, last_used, fundamental FROM items WHERE scope=? ORDER BY score DESC, updated_at DESC`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) AddSignal(sig memory.Signal) error {
	res, err := s.db.Exec(`INSERT INTO signals (item_id, outcome, correction, source, at) VALUES (?, ?, ?, ?, ?)`,
		sig.ItemID, string(sig.Outcome), sig.Correction, sig.Source, sig.At.Unix())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	sig.ID = id
	return nil
}

func (s *Store) Signals(itemID string) ([]memory.Signal, error) {
	rows, err := s.db.Query(`SELECT id, item_id, outcome, correction, source, at FROM signals WHERE item_id=? ORDER BY at ASC, id ASC`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.Signal
	for rows.Next() {
		var (
			s       memory.Signal
			outcome string
			at      int64
		)
		if err := rows.Scan(&s.ID, &s.ItemID, &outcome, &s.Correction, &s.Source, &at); err != nil {
			return nil, err
		}
		s.Outcome = memory.Outcome(outcome)
		s.At = time.Unix(at, 0).UTC()
		out = append(out, s)
	}
	return out, rows.Err()
}

func (s *Store) AllSignals() ([]memory.Signal, error) {
	rows, err := s.db.Query(`SELECT id, item_id, outcome, correction, source, at FROM signals ORDER BY at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.Signal
	for rows.Next() {
		var (
			s       memory.Signal
			outcome string
			at      int64
		)
		if err := rows.Scan(&s.ID, &s.ItemID, &outcome, &s.Correction, &s.Source, &at); err != nil {
			return nil, err
		}
		s.Outcome = memory.Outcome(outcome)
		s.At = time.Unix(at, 0).UTC()
		out = append(out, s)
	}
	return out, rows.Err()
}

// Search runs a lexical FTS5 query and returns candidate ids with bm25 ranks.
func (s *Store) Search(match string, limit int) ([]SearchHit, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT i.id, bm25(items_fts) FROM items_fts
		 JOIN items i ON i.rowid = items_fts.rowid
		 WHERE items_fts MATCH ? ORDER BY bm25(items_fts) LIMIT ?`, match, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ID, &h.Rank); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// SearchLike does a trigram FTS5 match, which finds substrings of three or
// more characters. It is the fallback when the token FTS table finds nothing,
// because natural language queries often share no whole token with the text.
func (s *Store) SearchLike(terms []string, limit int) ([]SearchHit, error) {
	if limit <= 0 {
		limit = 50
	}
	kept := make([]string, 0, len(terms))
	for _, t := range terms {
		if len([]rune(t)) >= 3 {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		return nil, nil
	}
	quoted := make([]string, 0, len(kept))
	for _, t := range kept {
		quoted = append(quoted, `"`+strings.ReplaceAll(t, `"`, `""`)+`"`)
	}
	match := strings.Join(quoted, " OR ")

	rows, err := s.db.Query(
		`SELECT i.id, bm25(items_tri) FROM items_tri
		 JOIN items i ON i.rowid = items_tri.rowid
		 WHERE items_tri MATCH ? ORDER BY bm25(items_tri) LIMIT ?`, match, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ID, &h.Rank); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Touch records that an item was served to a caller. The stored score is
// refreshed with the usage reinforcement so the next recall ranks it higher.
func (s *Store) Touch(id string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE items SET last_used=?, use_count=use_count+1 WHERE id=?`, at.Unix(), id)
	return err
}

type SearchHit struct {
	ID   string
	Rank float64
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanItem(row rowScanner) (memory.Item, error) {
	var (
		it       memory.Item
		kind     string
		tags     string
		scope    string
		status   string
		created  int64
		updated  int64
		lastUsed int64
		fund     int
	)
	if err := row.Scan(&it.ID, &kind, &it.Title, &it.Body, &tags, &scope, &it.Source, &status,
		&it.Score, &it.UseCount, &created, &updated, &lastUsed, &fund); err != nil {
		return memory.Item{}, err
	}
	it.Kind = memory.Kind(kind)
	it.Scope = scope
	it.Status = memory.Status(status)
	it.Fundamental = fund != 0
	it.CreatedAt = time.Unix(created, 0).UTC()
	it.UpdatedAt = time.Unix(updated, 0).UTC()
	it.LastUsed = time.Unix(lastUsed, 0).UTC()
	if tags != "" {
		it.Tags = strings.Fields(tags)
	}
	return it, nil
}
