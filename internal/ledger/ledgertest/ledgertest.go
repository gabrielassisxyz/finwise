// Package ledgertest builds invented pluggy-sync snapshots for tests. It writes
// the collector's base tables (documents, versions, observations, document
// state, scopes) as well as the current-state tables, so the collector's own
// views compute coverage, daily sightings and pending pairs the way they do on a
// real ledger. Nothing here is derived from a real account.
package ledgertest

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	_ "modernc.org/sqlite"
)

const (
	// DefaultDay is the day of observation of everything a fixture adds unless
	// a row says otherwise.
	DefaultDay = "2026-03-15"

	instantLayout = "2006-01-02T15:04:05.000Z"
	// noLowerBound is a scope whose walk reached back before any invented date.
	noLowerBound = "0001-01-01"
)

// Fixture is a snapshot file under construction. The file starts as an empty
// ledger at user_version 1 with one complete run that the projection reached.
type Fixture struct {
	t    testing.TB
	db   *sql.DB
	Path string

	coverageStart map[string]string
	bodies        map[string]map[string]any
	run           int64
}

// New creates a fixture in the test's temporary directory.
func New(t testing.TB) *Fixture {
	t.Helper()
	return NewAt(t, filepath.Join(t.TempDir(), "pluggy-snapshot.db"))
}

// NewAt creates a fixture at path.
func NewAt(t testing.TB, path string) *Fixture {
	t.Helper()
	return newFixture(t, path, "complete")
}

// NewWithFirstRunOutcome creates a fixture whose only run ended with the given
// outcome, for a ledger that never completed a run.
func NewWithFirstRunOutcome(t testing.TB, outcome string) *Fixture {
	t.Helper()
	return newFixture(t, filepath.Join(t.TempDir(), "pluggy-snapshot.db"), outcome)
}

func newFixture(t testing.TB, path, firstRunOutcome string) *Fixture {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("ledgertest: open %s: %v", path, err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	f := &Fixture{
		t: t, db: db, Path: path,
		coverageStart: map[string]string{}, bodies: map[string]map[string]any{},
	}
	// Speed only: the fixture is rebuilt for every test and never has to survive a crash.
	f.exec("PRAGMA synchronous = OFF")
	f.exec(ledger.SchemaV1)
	f.exec("PRAGMA user_version = 1")
	f.exec(`INSERT INTO runs (id, started_at, started_day, finished_at, finished_day, outcome,
	        api_calls, code_version, item_ids) VALUES
	        (1, '2026-03-15T02:59:00+00:00', '2026-03-15', '2026-03-15T03:00:00+00:00', '2026-03-15',
	         ?, 10, 'fixture', '[]')`, firstRunOutcome)
	f.exec("INSERT INTO projection_state (id, projected_run, code_version) VALUES (1, 1, 'fixture')")
	return f
}

func (f *Fixture) currentRun() int64 {
	if f.run == 0 {
		return 1
	}
	return f.run
}

// UseRun makes the observations recorded from now on belong to a run added with
// AddRun, so that a later run can see something differently from an earlier one.
func (f *Fixture) UseRun(run int64) {
	f.run = run
}

// MarkAmbiguous records that the run saw the document under more than one
// parent, which makes every sighting of it in that run ambiguous.
func (f *Fixture) MarkAmbiguous(kind, id string, run int64) {
	f.t.Helper()
	f.exec(`INSERT INTO ambiguities (run_id, document_id, candidate_parents, reason)
	        SELECT ?, id, '[]', 'fixture' FROM documents WHERE kind = ? AND provider_id = ?`, run, kind, id)
}

// RecordEvent records that a run saw the document removed (or its parent gone),
// effective on a day.
func (f *Fixture) RecordEvent(kind, id string, run int64, event, day string) {
	f.t.Helper()
	f.exec(`INSERT INTO events (run_id, document_id, seq, event, effective_at, effective_day)
	        SELECT ?, id, 1, ?, ?, ? FROM documents WHERE kind = ? AND provider_id = ?`,
		run, event, day+"T03:00:00.000Z", day, kind, id)
}

// SetUserVersion changes the schema version the file declares.
func (f *Fixture) SetUserVersion(version int) {
	f.t.Helper()
	f.exec("PRAGMA user_version = " + strconv.Itoa(version))
}

// Run is a run of the collector after the fixture's first one.
type Run struct {
	Outcome    string
	FinishedAt time.Time
}

// AddRun appends a run that finished at the given instant and returns its id.
// It leaves the projection where it was: see ProjectTo.
func (f *Fixture) AddRun(run Run) int64 {
	f.t.Helper()
	finished := run.FinishedAt.UTC().Format("2006-01-02T15:04:05+00:00")
	day := run.FinishedAt.UTC().Format("2006-01-02")
	return f.lastID(f.exec(`INSERT INTO runs (started_at, started_day, finished_at, finished_day, outcome,
	        api_calls, code_version, item_ids) VALUES (?, ?, ?, ?, ?, 1, 'fixture', '[]')`,
		finished, day, finished, day, run.Outcome))
}

// ProjectTo records run as the one that produced the current rows. The
// fixture's scopes all belong to run 1, and a document is covered only while
// its scope's run is the projected one, so projecting a later run leaves every
// document the fixture added uncovered.
func (f *Fixture) ProjectTo(run int64) {
	f.t.Helper()
	var exists bool
	if err := f.db.QueryRow("SELECT EXISTS (SELECT 1 FROM runs WHERE id = ?)", run).Scan(&exists); err != nil {
		f.t.Fatalf("ledgertest: run lookup: %v", err)
	}
	if !exists {
		f.t.Fatalf("ledgertest: cannot project run %d: the ledger has no such run", run)
	}
	f.exec("UPDATE projection_state SET projected_run = ? WHERE id = 1", run)
}

func (f *Fixture) exec(query string, args ...any) sql.Result {
	f.t.Helper()
	result, err := f.db.Exec(query, args...)
	if err != nil {
		f.t.Fatalf("ledgertest: %v\n%.200s", err, query)
	}
	return result
}

func (f *Fixture) lastID(result sql.Result) int64 {
	f.t.Helper()
	id, err := result.LastInsertId()
	if err != nil {
		f.t.Fatalf("ledgertest: last insert id: %v", err)
	}
	return id
}

func mustJSON(t testing.TB, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("ledgertest: encode %v: %v", value, err)
	}
	return string(encoded)
}

func formatInstant(instant time.Time) string {
	return instant.UTC().Format(instantLayout)
}

func shaOf(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// merge copies overrides on top of base; neither argument is modified.
func merge(base, overrides map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(overrides))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

// InTransaction runs fn as one transaction, for tests that add many rows.
func (f *Fixture) InTransaction(fn func()) {
	f.t.Helper()
	f.exec("BEGIN")
	fn()
	f.exec("COMMIT")
}
