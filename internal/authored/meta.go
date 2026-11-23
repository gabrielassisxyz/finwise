package authored

import (
	"context"
	"database/sql"
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
