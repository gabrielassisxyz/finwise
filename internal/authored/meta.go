package authored

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// The app reviews uncategorised transactions from the month finwise.db was
// created in. It is written once, with the first migration, and never again: not
// by a later migration, not by a restore from a copy.
const statementRecordTrackingStart = `
INSERT INTO meta (key, value) VALUES ('tracking_start_month', ?)`

const statementTrackingStart = `SELECT value FROM meta WHERE key = 'tracking_start_month'`

func recordTrackingStart(ctx context.Context, tx *sql.Tx, now time.Time) error {
	_, err := tx.ExecContext(ctx, statementRecordTrackingStart, string(brday.MonthOf(now)))
	return err
}

// TrackingStartMonth is the Brazil month in which the database was created.
func (database *Database) TrackingStartMonth(ctx context.Context) (brday.Month, error) {
	var stored string
	err := database.read(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, statementTrackingStart).Scan(&stored)
	})
	if err != nil {
		return "", err
	}
	month, err := brday.ParseMonth(stored)
	if err != nil {
		return "", &TransactionError{cause: err}
	}
	return month, nil
}

// The collector run whose snapshot was last taken in, in decimal. A run is
// never replaced by an earlier one: a caller that read an older snapshot must
// not move it backwards.
const (
	statementLastIngestedRun   = `SELECT value FROM meta WHERE key = 'last_ingested_run'`
	statementRecordIngestedRun = `
INSERT INTO meta (key, value) VALUES ('last_ingested_run', ?1)
ON CONFLICT (key) DO UPDATE SET value = excluded.value
WHERE CAST(meta.value AS INTEGER) < CAST(excluded.value AS INTEGER)`
)

// LastIngestedRun is the collector run whose snapshot was last taken in. found
// is false until one has been, and on the empty view.
func (view *View) LastIngestedRun(ctx context.Context) (run int64, found bool, err error) {
	var stored string
	read, err := view.read(ctx, func(tx *sql.Tx) error {
		scanned := tx.QueryRowContext(ctx, statementLastIngestedRun).Scan(&stored)
		if errors.Is(scanned, sql.ErrNoRows) {
			return nil
		}
		found = scanned == nil
		return scanned
	})
	if err != nil || !read || !found {
		return 0, false, err
	}
	run, err = strconv.ParseInt(stored, 10, 64)
	if err != nil {
		return 0, false, &TransactionError{cause: err}
	}
	return run, true, nil
}

// RecordIngestedRun records, in the recorder's transaction, that the snapshot
// of run was taken in, unless a later run already is. meta is not journaled:
// this records no effect, and an undo of the change leaves it.
func (recorder *Recorder) RecordIngestedRun(ctx context.Context, run int64) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	if _, err := recorder.tx.ExecContext(ctx, statementRecordIngestedRun, strconv.FormatInt(run, 10)); err != nil {
		return recorder.fail(err)
	}
	return nil
}

// The last run of the model suggestion step that made at least one request:
// its instant (UTC with milliseconds, as the journal writes them) and whether
// every request was answered, stored as one JSON object under last_model_run.
const (
	statementLastModelRun   = `SELECT value FROM meta WHERE key = 'last_model_run'`
	statementRecordModelRun = `
INSERT INTO meta (key, value) VALUES ('last_model_run', ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value`
)

// ModelRun is the recorded outcome of the last model suggestion run that made
// at least one request.
type ModelRun struct {
	At time.Time
	OK bool
}

type storedModelRun struct {
	At string `json:"at"`
	OK bool   `json:"ok"`
}

// LastModelRun is the outcome of the last model suggestion run that made at
// least one request. found is false until one has run, and on the empty view.
func (view *View) LastModelRun(ctx context.Context) (run ModelRun, found bool, err error) {
	var raw string
	read, err := view.read(ctx, func(tx *sql.Tx) error {
		scanned := tx.QueryRowContext(ctx, statementLastModelRun).Scan(&raw)
		if errors.Is(scanned, sql.ErrNoRows) {
			return nil
		}
		found = scanned == nil
		return scanned
	})
	if err != nil || !read || !found {
		return ModelRun{}, false, err
	}
	var stored storedModelRun
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return ModelRun{}, false, &TransactionError{cause: err}
	}
	at, err := time.Parse(instantLayout, stored.At)
	if err != nil {
		return ModelRun{}, false, &TransactionError{cause: err}
	}
	return ModelRun{At: at, OK: stored.OK}, true, nil
}

// RecordModelRun records the instant and outcome of a model suggestion run in
// meta. Nothing is written to the journal.
func (database *Database) RecordModelRun(ctx context.Context, at time.Time, ok bool) error {
	encoded, err := json.Marshal(storedModelRun{At: formatInstant(at), OK: ok})
	if err != nil {
		return &TransactionError{cause: err}
	}
	return database.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, statementRecordModelRun, string(encoded))
		return err
	})
}
