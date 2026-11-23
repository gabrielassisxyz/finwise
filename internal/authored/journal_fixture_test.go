package authored_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// The scratch tables every journal test runs on, one of each shape the
// registry knows. None of them is a table of the app.
const scratchSchema = `
CREATE TABLE facts (
  id     TEXT PRIMARY KEY NOT NULL,
  refund INTEGER NOT NULL DEFAULT 0,
  amount INTEGER NOT NULL DEFAULT 0,
  line   TEXT DEFAULT NULL,
  asking INTEGER NOT NULL DEFAULT 0,
  -- Spans two columns so that restoring either alone, in either order, from a
  -- write that set both is refused.
  CHECK ((line IS NULL) = (asking = 0))
) STRICT;
CREATE TABLE things (
  id     INTEGER PRIMARY KEY NOT NULL,
  name   TEXT NOT NULL UNIQUE,
  amount INTEGER NOT NULL,
  note   TEXT
) STRICT;
CREATE TABLE tags (
  tx  TEXT NOT NULL,
  tag TEXT NOT NULL,
  PRIMARY KEY (tx, tag)
) STRICT;
CREATE TABLE parts (
  tx       TEXT NOT NULL,
  position INTEGER NOT NULL,
  line     TEXT,
  amount   INTEGER NOT NULL,
  PRIMARY KEY (tx, position)
) STRICT;
CREATE TABLE steps (
  id    INTEGER PRIMARY KEY NOT NULL,
  tx    TEXT NOT NULL,
  label TEXT NOT NULL
) STRICT;
CREATE TABLE step_reasons (
  step_id INTEGER PRIMARY KEY NOT NULL REFERENCES steps (id),
  reason  TEXT NOT NULL
) STRICT;
CREATE TABLE parents (
  id   INTEGER PRIMARY KEY NOT NULL,
  name TEXT NOT NULL
) STRICT;
CREATE TABLE children (
  id        INTEGER PRIMARY KEY NOT NULL,
  parent_id INTEGER NOT NULL REFERENCES parents (id),
  label     TEXT NOT NULL
) STRICT;
CREATE TABLE plain (
  id   INTEGER PRIMARY KEY NOT NULL,
  note TEXT NOT NULL
) STRICT;
CREATE TABLE lists (
  id   INTEGER PRIMARY KEY NOT NULL,
  name TEXT NOT NULL
) STRICT;
CREATE TABLE entries (
  list_id INTEGER NOT NULL REFERENCES lists (id),
  item    TEXT NOT NULL,
  PRIMARY KEY (list_id, item)
) STRICT;
`

var (
	integer = authored.Integer
	text    = authored.Text
	null    = authored.Null
)

// scratchStatements builds a table's statements from its names; the
// statement discipline covers the package's own files, not a test's.
func scratchStatements(name string, columns, key, owner []string) authored.TableStatements {
	var others []string
	for _, column := range columns {
		if !contains(key, column) {
			others = append(others, column)
		}
	}
	where := func(names []string) string {
		parts := make([]string, len(names))
		for i, n := range names {
			parts[i] = n + " = ?"
		}
		return strings.Join(parts, " AND ")
	}
	list := strings.Join(columns, ", ")
	statements := authored.TableStatements{
		SelectRow: fmt.Sprintf("SELECT %s FROM %s WHERE %s", list, name, where(key)),
		SelectAll: fmt.Sprintf("SELECT %s FROM %s", list, name),
		Insert:    fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", name, list, strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", ")),
		Delete:    fmt.Sprintf("DELETE FROM %s WHERE %s", name, where(key)),
	}
	if len(others) > 0 {
		sets := make([]string, len(others))
		for i, n := range others {
			sets[i] = n + " = ?"
		}
		statements.Update = fmt.Sprintf("UPDATE %s SET %s WHERE %s", name, strings.Join(sets, ", "), where(key))
	}
	if owner != nil {
		statements.SelectOwned = fmt.Sprintf("SELECT %s FROM %s WHERE %s", list, name, where(owner))
	}
	return statements
}

func contains(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

func columnsOf(names ...string) []authored.Column {
	columns := make([]authored.Column, len(names))
	for i, name := range names {
		columns[i] = authored.Column{Name: name}
	}
	return columns
}

func scratchTables() []authored.Table {
	set := func(name string, key []string, columns ...string) authored.Table {
		return authored.Table{Name: name, Shape: authored.ShapeSet, Columns: columnsOf(columns...), Key: key,
			Statements: scratchStatements(name, columns, key, nil)}
	}
	reasons := set("step_reasons", []string{"step_id"}, "step_id", "reason")
	reasons.OwnedBy = &authored.Ownership{Owner: "steps", Columns: []string{"step_id"}, OnUndo: authored.RowsFollowTheirOwner}
	reasons.Statements = scratchStatements("step_reasons", []string{"step_id", "reason"}, []string{"step_id"}, []string{"step_id"})
	// A list's entries are owned by it as a version's lines are: they hold it.
	entries := set("entries", []string{"list_id", "item"}, "list_id", "item")
	entries.OwnedBy = &authored.Ownership{Owner: "lists", Columns: []string{"list_id"}, OnUndo: authored.RowsHoldTheirOwner}
	entries.Statements = scratchStatements("entries", []string{"list_id", "item"}, []string{"list_id", "item"}, []string{"list_id"})
	return []authored.Table{
		{
			Name: "facts", Shape: authored.ShapeAttribute, Kind: authored.KindOptionalFacts, Key: []string{"id"},
			Columns: []authored.Column{
				{Name: "id"}, {Name: "refund", Default: integer(0)}, {Name: "amount", Default: integer(0)},
				{Name: "line", Default: null}, {Name: "asking", Default: integer(0)},
			},
			Statements: scratchStatements("facts", []string{"id", "refund", "amount", "line", "asking"}, []string{"id"}, nil),
		},
		{
			Name: "things", Shape: authored.ShapeAttribute, Kind: authored.KindCreatedWhole, Key: []string{"id"},
			Columns:    columnsOf("id", "name", "amount", "note"),
			Statements: scratchStatements("things", []string{"id", "name", "amount", "note"}, []string{"id"}, nil),
		},
		set("tags", []string{"tx", "tag"}, "tx", "tag"),
		set("parts", []string{"tx", "position"}, "tx", "position", "line", "amount"),
		set("steps", []string{"id"}, "id", "tx", "label"),
		reasons,
		set("parents", []string{"id"}, "id", "name"),
		set("children", []string{"id"}, "id", "parent_id", "label"),
		set("lists", []string{"id"}, "id", "name"),
		entries,
	}
}

func scratchRegistry(t testing.TB, checks ...authored.RestoreCheck) *authored.Registry {
	t.Helper()
	registry, err := authored.NewRegistry(scratchTables(), checks...)
	if err != nil {
		t.Fatalf("scratch registry: %v", err)
	}
	return registry
}

// openScratch is a fresh database with the scratch tables, journaled by a
// registry of them, at path if one is given.
func openScratch(t testing.TB, path string, checks ...authored.RestoreCheck) *authored.Database {
	t.Helper()
	if path == "" {
		path = databasePath(t)
	}
	database, err := authored.Open(t.Context(), path, authored.Options{
		Now:        clockAt(t, creationInstant),
		Migrations: withMigrations(t, map[string]string{"9000_scratch": scratchSchema}),
		Journal:    scratchRegistry(t, checks...),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

var changeInstant = time.Date(2026, 12, 1, 1, 30, 0, 0, time.UTC)

func facts(method string) authored.ChangeFacts {
	return authored.ChangeFacts{Author: authored.AuthorPerson, Method: method, At: changeInstant}
}

// change records write as one change and returns its id, asking for it after
// write so that a change that records nothing is still an empty change.
func change(t testing.TB, database *authored.Database, write func(*authored.Recorder) error) authored.ChangeID {
	t.Helper()
	return changeBy(t, database, "test.write", write)
}

func changeBy(t testing.TB, database *authored.Database, method string, write func(*authored.Recorder) error) authored.ChangeID {
	t.Helper()
	var id authored.ChangeID
	err := database.Change(t.Context(), facts(method), func(recorder *authored.Recorder) error {
		if err := write(recorder); err != nil {
			return err
		}
		var err error
		id, err = recorder.ID(t.Context())
		return err
	})
	if err != nil {
		t.Fatalf("change: %v", err)
	}
	return id
}

func undo(t testing.TB, database *authored.Database, id authored.ChangeID) {
	t.Helper()
	if err := database.Undo(t.Context(), id, undoInstant); err != nil {
		t.Fatalf("undo %s: %v", id, err)
	}
}

var undoInstant = time.Date(2026, 12, 2, 10, 0, 0, 0, time.UTC)

// refusedAs undoes id and wants a refusal of that cause.
func refusedAs(t testing.TB, database *authored.Database, id authored.ChangeID, cause authored.StaleCause) {
	t.Helper()
	assertStale(t, database.Undo(t.Context(), id, undoInstant), cause)
}

func assertStale(t testing.TB, err error, cause authored.StaleCause) {
	t.Helper()
	var refused *authored.StaleChangeError
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v (%T), want a refusal as stale", err, err)
	}
	if refused.Cause != cause {
		t.Fatalf("refused with cause %d, want %d", refused.Cause, cause)
	}
}

func dump(t testing.TB, database *authored.Database) string {
	t.Helper()
	content, err := database.Dump(t.Context())
	if err != nil {
		t.Fatalf("dump: %v", err)
	}
	return content
}

// journal is both journal tables, every column of every row.
func journal(t testing.TB, database *authored.Database) string {
	t.Helper()
	rows := columnValues(t, database.Pool(), `
SELECT 'change ' || id || ' ' || at || ' ' || day || ' ' || author || ' ' || method || ' ' || extended || ' ' || coalesce(undone_at, '-')
FROM changes
UNION ALL
SELECT 'effect ' || change_id || ' ' || seq || ' ' || tbl || ' ' || key || ' ' || coalesce(col, '-') || ' ' || coalesce(before, '-') || ' ' || coalesce(after, '-')
FROM change_effects
ORDER BY 1`)
	return strings.Join(rows, "\n")
}

// everything is what a refusal must leave as it was: the dump, the journal
// and the one table that is not journaled.
func everything(t testing.TB, database *authored.Database) string {
	t.Helper()
	plain := columnValues(t, database.Pool(), "SELECT id || ' ' || note FROM plain ORDER BY id")
	return dump(t, database) + "\n--\n" + journal(t, database) + "\n--\n" + strings.Join(plain, "\n")
}

func assertUnchanged(t testing.TB, database *authored.Database, before string) {
	t.Helper()
	if after := everything(t, database); after != before {
		t.Fatalf("a refusal changed something:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func assertDump(t testing.TB, database *authored.Database, want string) {
	t.Helper()
	if got := dump(t, database); got != want {
		t.Fatalf("dump:\n%s\nwant:\n%s", got, want)
	}
}

// around writes straight to the file, as a write that escaped the journal would.
func around(t testing.TB, database *authored.Database, statement string, args ...any) {
	t.Helper()
	if _, err := database.Pool().ExecContext(context.Background(), statement, args...); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

type effectRow struct {
	change, seq        int64
	table, key, column string
	before, after      sql.NullString
	columnNull         bool
}

func effectsOf(t testing.TB, database *authored.Database, id authored.ChangeID) []effectRow {
	t.Helper()
	rows, err := database.Pool().Query(
		"SELECT change_id, seq, tbl, key, col, before, after FROM change_effects WHERE change_id = ? ORDER BY seq", int64(id))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var effects []effectRow
	for rows.Next() {
		var found effectRow
		var column sql.NullString
		if err := rows.Scan(&found.change, &found.seq, &found.table, &found.key, &column, &found.before, &found.after); err != nil {
			t.Fatal(err)
		}
		found.column, found.columnNull = column.String, !column.Valid
		effects = append(effects, found)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return effects
}

func side(text string) sql.NullString { return sql.NullString{String: text, Valid: true} }

var nothing = sql.NullString{}

// Row builders over the scratch tables.

func thing(id int64, name string, amount int64, note authored.Value) authored.Row {
	return authored.Row{"id": integer(id), "name": text(name), "amount": integer(amount), "note": note}
}

func tag(tx, name string) authored.Row { return authored.Row{"tx": text(tx), "tag": text(name)} }

func part(tx string, position int64, line authored.Value, amount int64) authored.Row {
	return authored.Row{"tx": text(tx), "position": integer(position), "line": line, "amount": integer(amount)}
}

func step(id int64, tx, label string) authored.Row {
	return authored.Row{"id": integer(id), "tx": text(tx), "label": text(label)}
}

func reason(stepID int64, why string) authored.Row {
	return authored.Row{"step_id": integer(stepID), "reason": text(why)}
}

func parent(id int64, name string) authored.Row {
	return authored.Row{"id": integer(id), "name": text(name)}
}

func child(id, parentID int64, label string) authored.Row {
	return authored.Row{"id": integer(id), "parent_id": integer(parentID), "label": text(label)}
}

func list(id int64, name string) authored.Row {
	return authored.Row{"id": integer(id), "name": text(name)}
}

func entry(listID int64, item string) authored.Row {
	return authored.Row{"list_id": integer(listID), "item": text(item)}
}

func key(values ...authored.Value) []authored.Value { return values }

func setFacts(id string, values authored.Row) func(*authored.Recorder) error {
	return func(recorder *authored.Recorder) error {
		return recorder.Set(context.Background(), "facts", key(text(id)), values)
	}
}

func setThing(id int64, values authored.Row) func(*authored.Recorder) error {
	return func(recorder *authored.Recorder) error {
		return recorder.Set(context.Background(), "things", key(integer(id)), values)
	}
}

func createThing(row authored.Row) func(*authored.Recorder) error {
	return func(recorder *authored.Recorder) error { return recorder.Create(context.Background(), "things", row) }
}

func put(table string, row authored.Row) func(*authored.Recorder) error {
	return func(recorder *authored.Recorder) error { return recorder.Put(context.Background(), table, row) }
}

func remove(table string, values ...authored.Value) func(*authored.Recorder) error {
	return func(recorder *authored.Recorder) error { return recorder.Delete(context.Background(), table, values) }
}

// all runs writes in order as one change's body.
func all(writes ...func(*authored.Recorder) error) func(*authored.Recorder) error {
	return func(recorder *authored.Recorder) error {
		for _, write := range writes {
			if err := write(recorder); err != nil {
				return err
			}
		}
		return nil
	}
}
