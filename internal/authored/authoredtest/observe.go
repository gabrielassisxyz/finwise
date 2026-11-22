package authoredtest

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const (
	selectTableNames = `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`
	selectEffects    = `SELECT seq, tbl, key, col, before, after FROM change_effects WHERE change_id = ? ORDER BY seq`

	forceFact  = `INSERT INTO scratch_facts (id, amount) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET amount = excluded.amount`
	forceChild = `INSERT OR REPLACE INTO scratch_children (id, parent_id, label) VALUES (?, ?, ?)`
	touchMeta  = `INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`
)

// Fresh creates an app database the way the server does, with the registry the
// app ships and nothing authored in it, in the test's temporary directory.
func Fresh(t testing.TB, now func() time.Time) *authored.Database {
	t.Helper()
	database, err := authored.Open(t.Context(), filepath.Join(t.TempDir(), "finwise.db"), authored.Options{Now: now})
	if err != nil {
		t.Fatalf("authoredtest: create a database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// Canonical is the canonical dump of every journaled table: the one JSON
// document two databases in the same state share byte for byte.
func (scratch *Scratch) Canonical(t testing.TB) string {
	t.Helper()
	dump, err := scratch.Database.CanonicalDump(t.Context())
	if err != nil {
		t.Fatalf("authoredtest: canonical dump: %v", err)
	}
	return dump
}

// Journal is every column of both journal tables, one line per row.
func (scratch *Scratch) Journal(t testing.TB) string {
	t.Helper()
	rows, err := scratch.Inspect(t).QueryContext(t.Context(), selectJournal)
	if err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	defer rows.Close()
	var journal []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("authoredtest: %v", err)
		}
		journal = append(journal, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	return strings.Join(journal, "\n")
}

// Everything is the content of every table of the file, journaled or not and
// whatever a later migration adds, every column of every row. A table is found
// in the file's own catalogue, not in a list that a later table would have to
// be added to, so a write to any of them shows.
func (scratch *Scratch) Everything(t testing.TB) string {
	t.Helper()
	database := scratch.Inspect(t)
	names, err := columnOfText(t, database, selectTableNames)
	if err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	var out strings.Builder
	for _, name := range names {
		lines, err := rowsOfTable(t, database, name)
		if err != nil {
			t.Fatalf("authoredtest: %s: %v", name, err)
		}
		out.WriteString("== " + name + "\n")
		for _, line := range lines {
			out.WriteString(line + "\n")
		}
	}
	return out.String()
}

func columnOfText(t testing.TB, database *sql.DB, query string) ([]string, error) {
	rows, err := database.QueryContext(t.Context(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// rowsOfTable reads a table by a name the catalogue gave, which is why this one
// statement is assembled instead of being a constant: no table's name is known
// to the harness, by design.
func rowsOfTable(t testing.TB, database *sql.DB, name string) ([]string, error) {
	rows, err := database.QueryContext(t.Context(), fmt.Sprintf(`SELECT * FROM "%s"`, strings.ReplaceAll(name, `"`, `""`)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var lines []string
	for rows.Next() {
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		parts := make([]string, len(columns))
		for i, value := range values {
			parts[i] = columns[i] + "=" + describe(value)
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	slices.Sort(lines)
	return lines, rows.Err()
}

func describe(value any) string {
	switch typed := value.(type) {
	case nil:
		return "NULL"
	case int64:
		return fmt.Sprint(typed)
	case string:
		return fmt.Sprintf("%q", typed)
	case []byte:
		return fmt.Sprintf("x'%x'", typed)
	}
	return fmt.Sprintf("%T(%v)", value, value)
}

// Effect is a row of the journal's change_effects table as the recorder wrote
// it. Column is empty and Before or After are empty for what a row does not
// have: a set table's effect has no column, and one side of a creation or a
// removal has no row.
type Effect struct {
	Seq    int64
	Table  string
	Key    string
	Column string
	Before string
	After  string
}

// EffectsOf is the effects of one change, in the order of their seq.
func (scratch *Scratch) EffectsOf(t testing.TB, change int64) []Effect {
	t.Helper()
	rows, err := scratch.Inspect(t).QueryContext(t.Context(), selectEffects, change)
	if err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	defer rows.Close()
	var effects []Effect
	for rows.Next() {
		var effect Effect
		var column, before, after sql.NullString
		if err := rows.Scan(&effect.Seq, &effect.Table, &effect.Key, &column, &before, &after); err != nil {
			t.Fatalf("authoredtest: %v", err)
		}
		effect.Column, effect.Before, effect.After = column.String, before.String, after.String
		effects = append(effects, effect)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("authoredtest: %v", err)
	}
	return effects
}

// ForceFact writes a fact's amount straight into the file, creating its row if
// need be, as an undo that put a unit back wrongly would have left it.
func (scratch *Scratch) ForceFact(t testing.TB, fact string, amount int64) {
	t.Helper()
	around(t, scratch.Path, forceFact, fact, amount)
}

// ForceChild writes a child row straight into the file, as an undo that left it
// behind would have.
func (scratch *Scratch) ForceChild(t testing.TB, id, parent int64, label string) {
	t.Helper()
	around(t, scratch.Path, forceChild, id, parent, label)
}

// TouchMeta writes a row of the meta table, which is not journaled, straight
// into the file.
func (scratch *Scratch) TouchMeta(t testing.TB, key, value string) {
	t.Helper()
	around(t, scratch.Path, touchMeta, key, value)
}
