package authored

import (
	"context"
	"database/sql"
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
