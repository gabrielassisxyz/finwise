package authored

import (
	"context"
	"database/sql"
)

// TransactionUnits picks, among the units of a change, those of the
// transaction at key: its facts, its tags, the parts of its split, and the
// steps among steps with their reasons. What the same change did to another
// transaction or to an establishment is left out, so one transaction's step is
// judged, and taken back, on its own.
func TransactionUnits(key TxKey, steps []StepID) func(Unit) bool {
	owner := Integer(int64(key))
	own := make(map[Value]bool, len(steps))
	for _, step := range steps {
		own[Integer(int64(step))] = true
	}
	return func(unit Unit) bool {
		values, err := decodeKey(unit.Key)
		if err != nil || len(values) == 0 {
			return false
		}
		switch unit.Table {
		case txStateTable, txTagsTable, txSplitPartsTable:
			return values[0] == owner
		case historyStepsTable, historyReasonsTable:
			return own[values[0]]
		}
		return false
	}
}

// CanRestorePart is the database's CanRestorePart on the view's own
// transaction, so a read answers it from the state the rest of its answer
// comes from. The empty view holds no change and answers false.
func (view *View) CanRestorePart(ctx context.Context, source ChangeID, selected func(Unit) bool) (bool, error) {
	var can bool
	_, err := view.read(ctx, func(tx *sql.Tx) (err error) {
		can, err = canRestorePart(ctx, tx, view.registry(), source, selected)
		return err
	})
	return can, err
}
