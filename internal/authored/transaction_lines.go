package authored

import (
	"context"
	"database/sql"
)

// What the writes that put transactions on lines keep: a transaction's line and
// who put it there, the question of why, the person's reason beside a step, an
// establishment's default line, and the undo of one step of a history.

const statementStepsOfChange = `SELECT id, tx_key FROM history_steps WHERE change_id = ? ORDER BY id`

// SettleLine puts the transaction at key on line, by who, and ends the review
// it waited for: once someone decided, the model's confidence and its
// suggestion no longer apply. Each column that changes is one effect.
func (recorder *Recorder) SettleLine(ctx context.Context, key TxKey, line LineID, by Author) error {
	if !by.valid() {
		return ErrInvalidAuthor
	}
	return recorder.Set(ctx, txStateTable, []Value{Integer(int64(key))}, Row{
		"line_id": Integer(int64(line)), "classified_by": Text(string(by)), "confidence": Null,
		"pending_reason": Null, "pending_line_id": Null, "pending_confidence": Null,
	})
}

// SetAskReason opens or closes the panel's question of why the person moved the
// transaction at key to another line.
func (recorder *Recorder) SetAskReason(ctx context.Context, key TxKey, ask bool) error {
	return recorder.Set(ctx, txStateTable, []Value{Integer(int64(key))}, Row{"ask_reason": Integer(zeroOrOne(ask))})
}

// SetStepReason keeps reason beside step, replacing the one it had. The table
// holds no empty reason, so an empty one removes the step's: a step without a
// row reads as one without a reason.
func (recorder *Recorder) SetStepReason(ctx context.Context, step StepID, reason string) error {
	if reason == "" {
		return recorder.Delete(ctx, historyReasonsTable, []Value{Integer(int64(step))})
	}
	return recorder.Put(ctx, historyReasonsTable, Row{"step_id": Integer(int64(step)), "reason": Text(reason)})
}

// SetDefaultLine makes line, by its id, the default line of an establishment or
// a person. It belongs to no month: a read names it in the month it reads.
func (recorder *Recorder) SetDefaultLine(ctx context.Context, establishment string, line LineID) error {
	return recorder.Set(ctx, establishmentsTable, []Value{Text(establishment)}, Row{"default_line_id": Integer(int64(line))})
}

// ChangeStep is a step of a history that a change wrote, with the transaction
// it belongs to.
type ChangeStep struct {
	ID  StepID
	Key TxKey
}

// StepsOfChange is every step that change wrote and that is still in the
// history, oldest first.
func (view *View) StepsOfChange(ctx context.Context, change ChangeID) ([]ChangeStep, error) {
	var steps []ChangeStep
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementStepsOfChange, int64(change))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, key int64
			if err := rows.Scan(&id, &key); err != nil {
				return err
			}
			steps = append(steps, ChangeStep{ID: StepID(id), Key: TxKey(key)})
		}
		return rows.Err()
	})
	return steps, err
}

// TakeBackStep is the undo of one step of the history of the transaction at
// key, inside the recorder's change: what the step's change did to that
// transaction alone is restored, refused by the rules of an undo when any of it
// changed since, and the step goes with its reason. steps are the
// transaction's own, which tell its rows of the history from another
// transaction's in the same change, as the history's undoable flag tells them.
// When the line is among what comes back, the question of why is closed: the
// correction it asked about is gone.
func (recorder *Recorder) TakeBackStep(ctx context.Context, key TxKey, step HistoryStep, steps []StepID) error {
	line := Unit{Table: txStateTable, Key: EncodeKey(Integer(int64(key))), Column: "line_id"}
	picks := TransactionUnits(key, steps)
	restoresLine := false
	err := recorder.RestorePart(ctx, step.Change, func(unit Unit) bool {
		picked := picks(unit)
		restoresLine = restoresLine || (picked && unit == line)
		return picked
	})
	if err != nil {
		return err
	}
	if restoresLine {
		if err := recorder.SetAskReason(ctx, key, false); err != nil {
			return err
		}
	}
	return recorder.Delete(ctx, historyStepsTable, []Value{Integer(int64(step.ID))})
}
