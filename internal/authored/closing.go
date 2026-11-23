package authored

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// What the closing of a month reads and keeps besides the budget: the steps the
// person ticked for it, and the months already closed.

const (
	statementClosingStepsOfMonth = `SELECT step FROM closing_steps WHERE month = ? ORDER BY step`
	statementClosedMonthsNewest  = `SELECT month, closed_on FROM closed_months ORDER BY month DESC`

	// The key of an effect on closed_months is the row's key as a JSON array,
	// ["<month>"]. While the row stands at most one such change is in force;
	// the newest is taken all the same.
	statementCloseChangeOfMonth = `
SELECT changes.id
FROM changes JOIN change_effects AS effects ON effects.change_id = changes.id
WHERE changes.method = ? AND changes.undone_at IS NULL
  AND effects.tbl = 'closed_months' AND json_extract(effects.key, '$[0]') = ?
ORDER BY changes.id DESC
LIMIT 1`
)

// ClosedMonth is a month that was closed and the day it was.
type ClosedMonth struct {
	Month    brday.Month
	ClosedOn brday.Day
}

// ClosingStepsDone is every step ticked for month; none on the empty view.
func (view *View) ClosingStepsDone(ctx context.Context, month brday.Month) (map[string]bool, error) {
	done := map[string]bool{}
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementClosingStepsOfMonth, string(month))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var step string
			if err := rows.Scan(&step); err != nil {
				return err
			}
			done[step] = true
		}
		return rows.Err()
	})
	return done, err
}

// MarkClosingStepDone ticks step for month inside the open change; a step
// already ticked records nothing. The store checks no step: the table refuses
// one the person does not tick by hand, and the write then fails.
func (recorder *Recorder) MarkClosingStepDone(ctx context.Context, month brday.Month, step string) error {
	return recorder.Put(ctx, closingStepsTable, Row{"month": Text(string(month)), "step": Text(step)})
}

// ClosedMonths is every closed month with its day, newest first; none on the
// empty view.
func (view *View) ClosedMonths(ctx context.Context) ([]ClosedMonth, error) {
	var closed []ClosedMonth
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementClosedMonthsNewest)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var month, day string
			if err := rows.Scan(&month, &day); err != nil {
				return err
			}
			closed = append(closed, ClosedMonth{Month: brday.Month(month), ClosedOn: brday.Day(day)})
		}
		return rows.Err()
	})
	return closed, err
}

// CloseChangeOf is the change of method, not undone, whose effects wrote the
// closed_months row of month; found is false when there is none, and on the
// empty view. It does not look at whether the row stands now.
func (view *View) CloseChangeOf(ctx context.Context, method string, month brday.Month) (id ChangeID, found bool, err error) {
	_, err = view.read(ctx, func(tx *sql.Tx) error {
		var stored int64
		err := tx.QueryRowContext(ctx, statementCloseChangeOfMonth, method, string(month)).Scan(&stored)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		id, found = ChangeID(stored), err == nil
		return err
	})
	if err != nil {
		return 0, false, err
	}
	return id, found, nil
}
