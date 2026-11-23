package authored

import (
	"context"
	"database/sql"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// A payday transfer is derived from the month's budget at every read, so a mark
// keeps only the month, the transfer's key and the day it was marked done.

const statementTransferMarksOfMonth = `SELECT transfer_key, done_on FROM transfer_marks WHERE month = ? ORDER BY transfer_key`

// TransferMarks is the day each transfer of month was marked done, by key,
// whether or not the month still lists that key; none on the empty view.
func (view *View) TransferMarks(ctx context.Context, month brday.Month) (map[string]brday.Day, error) {
	marks := map[string]brday.Day{}
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementTransferMarksOfMonth, string(month))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key, day string
			if err := rows.Scan(&key, &day); err != nil {
				return err
			}
			marks[key] = brday.Day(day)
		}
		return rows.Err()
	})
	return marks, err
}

// MarkTransferDone records that the transfer under key was made for month on
// day, and answers whether it recorded anything: a transfer already marked for
// the month keeps the day it was first marked. The store checks no key against
// the month's transfers, which it cannot derive; the table refuses a month or a
// day that is not one and an empty key, and the write then fails.
func (recorder *Recorder) MarkTransferDone(ctx context.Context, month brday.Month, key string, day brday.Day) (bool, error) {
	_, marked, err := recorder.View().JournaledRow(ctx, transferMarksTable, []Value{Text(string(month)), Text(key)})
	if err != nil || marked {
		return false, err
	}
	row := Row{"month": Text(string(month)), "transfer_key": Text(key), "done_on": Text(string(day))}
	if err := recorder.Put(ctx, transferMarksTable, row); err != nil {
		return false, err
	}
	return true, nil
}
