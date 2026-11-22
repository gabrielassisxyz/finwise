// Package authoredtest builds app databases for the tests of the layers above
// internal/authored: a real finwise.db, opened the way the server opens it,
// whose journal also covers a few scratch tables, one of each shape the
// recorder knows. None of them is a table of the app, so a test can record,
// undo and refuse changes before the app has journaled tables of its own.
package authoredtest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	_ "modernc.org/sqlite"
)

// The scratch tables. Facts is an attribute table of optional facts, Things an
// attribute table of things created whole, Tags a set table, and Parents and
// Children a set table pair joined by a foreign key, which the database checks
// when a change commits.
const (
	Facts    = "scratch_facts"
	Things   = "scratch_things"
	Tags     = "scratch_tags"
	Parents  = "scratch_parents"
	Children = "scratch_children"
)

const schema = `
CREATE TABLE scratch_facts (
  id     TEXT PRIMARY KEY NOT NULL,
  amount INTEGER NOT NULL DEFAULT 0,
  note   TEXT DEFAULT NULL
) STRICT;
CREATE TABLE scratch_things (
  id     INTEGER PRIMARY KEY NOT NULL,
  name   TEXT NOT NULL UNIQUE,
  amount INTEGER NOT NULL
) STRICT;
CREATE TABLE scratch_tags (
  tx  TEXT NOT NULL,
  tag TEXT NOT NULL,
  PRIMARY KEY (tx, tag)
) STRICT;
CREATE TABLE scratch_parents (
  id   INTEGER PRIMARY KEY NOT NULL,
  name TEXT NOT NULL
) STRICT;
CREATE TABLE scratch_children (
  id        INTEGER PRIMARY KEY NOT NULL,
  parent_id INTEGER NOT NULL REFERENCES scratch_parents (id),
  label     TEXT NOT NULL
) STRICT;
`

// Scratch is an app database with the scratch tables, and the file it lives in.
type Scratch struct {
	Database *authored.Database
	Path     string
}

// Open creates a database in the test's temporary directory and closes it when
// the test ends. The file is made by the app's own open first, so it carries
// the journal tables exactly as a real one does, and then opened again with a
// registry that names the scratch tables.
func Open(t testing.TB, now func() time.Time) *Scratch {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finwise.db")
	created, err := authored.Open(t.Context(), path, authored.Options{Now: now})
	if err != nil {
		t.Fatalf("authoredtest: create %s: %v", path, err)
	}
	if err := created.Close(); err != nil {
		t.Fatalf("authoredtest: close %s: %v", path, err)
	}
	around(t, path, schema)
	registry, err := authored.NewRegistry(tables())
	if err != nil {
		t.Fatalf("authoredtest: registry: %v", err)
	}
	database, err := authored.Open(t.Context(), path, authored.Options{Now: now, Journal: registry})
	if err != nil {
		t.Fatalf("authoredtest: open %s: %v", path, err)
	}
	t.Cleanup(func() { database.Close() })
	return &Scratch{Database: database, Path: path}
}

// Inspect opens the file read-only on a connection of its own, for what a test
// reads that the app does not offer, such as the rows of the journal.
func (scratch *Scratch) Inspect(t testing.TB) *sql.DB {
	t.Helper()
	query := url.Values{}
	query.Set("mode", "ro")
	query.Add("_pragma", "busy_timeout(5000)")
	return open(t, scratch.Path, query)
}

// Change is a row of the journal's changes table.
type Change struct {
	ID       int64
	At       string
	Day      string
	Author   string
	Method   string
	Extended bool
	// UndoneAt is empty for a change that has not been undone.
	UndoneAt string
}

const (
	selectChanges = `SELECT id, at, day, author, method, extended, coalesce(undone_at, '') FROM changes ORDER BY id`
	countEffects  = `SELECT count(*) FROM change_effects`
	selectJournal = `
SELECT 'change ' || id || ' ' || at || ' ' || day || ' ' || author || ' ' || method || ' ' || extended || ' ' || coalesce(undone_at, '-')
FROM changes
UNION ALL
SELECT 'effect ' || change_id || ' ' || seq || ' ' || tbl || ' ' || key || ' ' || coalesce(col, '-') || ' ' || coalesce(before, '-') || ' ' || coalesce(after, '-')
FROM change_effects
ORDER BY 1`
)

// Changes is every row of the journal's changes table, in order.
func (scratch *Scratch) Changes(t testing.TB) []Change {
	t.Helper()
	rows, err := scratch.Inspect(t).QueryContext(t.Context(), selectChanges)
	if err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	defer rows.Close()
	var changes []Change
	for rows.Next() {
		var change Change
		if err := rows.Scan(&change.ID, &change.At, &change.Day, &change.Author, &change.Method, &change.Extended, &change.UndoneAt); err != nil {
			t.Fatalf("authoredtest: %v", err)
		}
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	return changes
}

// EffectCount is how many rows the journal's change_effects table holds.
func (scratch *Scratch) EffectCount(t testing.TB) int {
	t.Helper()
	var count int
	if err := scratch.Inspect(t).QueryRowContext(t.Context(), countEffects).Scan(&count); err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	return count
}

// State is everything a refused or failed write must leave as it was: the
// canonical content of the scratch tables and every column of both journal
// tables.
func (scratch *Scratch) State(t testing.TB) string {
	t.Helper()
	dump, err := scratch.Database.Dump(t.Context())
	if err != nil {
		t.Fatalf("authoredtest: dump: %v", err)
	}
	journal := scratch.Journal(t)
	return dump + "--\n" + journal
}

const overwriteFactAmount = `UPDATE scratch_facts SET amount = ? WHERE id = ?`

// OverwriteFactAmount sets a fact's amount straight in the file, as a write
// that escaped the journal would.
func (scratch *Scratch) OverwriteFactAmount(t testing.TB, fact string, amount int64) {
	t.Helper()
	around(t, scratch.Path, overwriteFactAmount, amount, fact)
}

// HoldWriteLock takes the file's write lock on a connection of its own and
// keeps it until release is called, which the test's end does too if it has not
// been. Meanwhile any other write waits for the lock.
func (scratch *Scratch) HoldWriteLock(t testing.TB) (release func()) {
	t.Helper()
	query := url.Values{}
	query.Add("_pragma", "busy_timeout(5000)")
	holder := open(t, scratch.Path, query)
	connection, err := holder.Conn(t.Context())
	if err != nil {
		t.Fatalf("authoredtest: connection: %v", err)
	}
	if _, err := connection.ExecContext(t.Context(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("authoredtest: take the write lock: %v", err)
	}
	released := false
	release = func() {
		if released {
			return
		}
		released = true
		// The test's own context is already done when its cleanups run.
		if _, err := connection.ExecContext(context.Background(), "ROLLBACK"); err != nil {
			t.Errorf("authoredtest: release the write lock: %v", err)
		}
		connection.Close()
	}
	t.Cleanup(release)
	return release
}

func around(t testing.TB, path, statement string, args ...any) {
	t.Helper()
	query := url.Values{}
	query.Add("_pragma", "busy_timeout(5000)")
	database := open(t, path, query)
	defer database.Close()
	if _, err := database.ExecContext(t.Context(), statement, args...); err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
}

// open is a connection pool of one, closed when the test ends.
func open(t testing.TB, path string, query url.Values) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String())
	if err != nil {
		t.Fatalf("authoredtest: open %s: %v", path, err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { database.Close() })
	return database
}

func tables() []authored.Table {
	set := func(name string, key []string, columns ...string) authored.Table {
		return authored.Table{
			Name: name, Shape: authored.ShapeSet, Columns: columnsOf(columns...), Key: key,
			Statements: statements(name, columns, key),
		}
	}
	return []authored.Table{
		{
			Name: Facts, Shape: authored.ShapeAttribute, Kind: authored.KindOptionalFacts, Key: []string{"id"},
			Columns: []authored.Column{
				{Name: "id"}, {Name: "amount", Default: authored.Integer(0)}, {Name: "note", Default: authored.Null},
			},
			Statements: statements(Facts, []string{"id", "amount", "note"}, []string{"id"}),
		},
		{
			Name: Things, Shape: authored.ShapeAttribute, Kind: authored.KindCreatedWhole, Key: []string{"id"},
			Columns:    columnsOf("id", "name", "amount"),
			Statements: statements(Things, []string{"id", "name", "amount"}, []string{"id"}),
		},
		set(Tags, []string{"tx", "tag"}, "tx", "tag"),
		set(Parents, []string{"id"}, "id", "name"),
		set(Children, []string{"id"}, "id", "parent_id", "label"),
	}
}

func columnsOf(names ...string) []authored.Column {
	columns := make([]authored.Column, len(names))
	for i, name := range names {
		columns[i] = authored.Column{Name: name}
	}
	return columns
}

// statements builds a table's statements from its names. The discipline that
// every statement of the app is a registered constant covers the app's own
// tables, not these.
func statements(name string, columns, key []string) authored.TableStatements {
	var others []string
	for _, column := range columns {
		if !contains(key, column) {
			others = append(others, column)
		}
	}
	list := strings.Join(columns, ", ")
	result := authored.TableStatements{
		SelectRow: fmt.Sprintf("SELECT %s FROM %s WHERE %s", list, name, assignments(key, " AND ")),
		SelectAll: fmt.Sprintf("SELECT %s FROM %s", list, name),
		Insert:    fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", name, list, strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", ")),
		Delete:    fmt.Sprintf("DELETE FROM %s WHERE %s", name, assignments(key, " AND ")),
	}
	if len(others) > 0 {
		result.Update = fmt.Sprintf("UPDATE %s SET %s WHERE %s", name, assignments(others, ", "), assignments(key, " AND "))
	}
	return result
}

func assignments(names []string, separator string) string {
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = name + " = ?"
	}
	return strings.Join(parts, separator)
}

func contains(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}
