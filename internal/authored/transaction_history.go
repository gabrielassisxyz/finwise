package authored

import (
	"context"
	"strconv"
)

// StepID is the identity of a history step, written on the wire as a decimal
// string.
type StepID int64

func (id StepID) String() string { return strconv.FormatInt(int64(id), 10) }

// StepKind says what a history step records.
type StepKind string

const (
	// StepEffect records what its change did to the transaction.
	StepEffect StepKind = "step"
	// StepNote records that something happened beside it.
	StepNote StepKind = "note"
)

const (
	// The id a new step gets: one past the highest history_steps ever held,
	// which AUTOINCREMENT keeps in sqlite_sequence, so an undone step's id is
	// never handed out again.
	statementNextStepID = `SELECT coalesce((SELECT seq FROM sqlite_sequence WHERE name = 'history_steps'), 0) + 1`
	statementChangeDay  = `SELECT day FROM changes WHERE id = ?`
)

// AddHistoryStep writes a step of the transaction at key into the recorder's
// change, with that change's id and Brazil day, and answers the id it minted.
// The step is one effect of the change, so undoing the change removes it.
func (recorder *Recorder) AddHistoryStep(ctx context.Context, key TxKey, kind StepKind, text string) (StepID, error) {
	change, err := recorder.ID(ctx)
	if err != nil {
		return 0, err
	}
	var day string
	var id int64
	if err := recorder.tx.QueryRowContext(ctx, statementChangeDay, int64(change)).Scan(&day); err != nil {
		return 0, recorder.fail(err)
	}
	if err := recorder.tx.QueryRowContext(ctx, statementNextStepID).Scan(&id); err != nil {
		return 0, recorder.fail(err)
	}
	row := Row{
		"id": Integer(id), "tx_key": Integer(int64(key)), "change_id": Integer(int64(change)),
		"kind": Text(string(kind)), "text": Text(text), "day": Text(day),
	}
	if err := recorder.Put(ctx, historyStepsTable, row); err != nil {
		return 0, err
	}
	return StepID(id), nil
}
