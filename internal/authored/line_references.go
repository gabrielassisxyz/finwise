package authored

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// A transaction names a budget line in four places that belong to its month:
// its own line, a part of its split, the line its refund takes off and the
// suggestion of a pending review. Each must be a line the budget in force for
// the transaction's month has, or its money counts in no group of that month
// and every total is silently wrong. A budget save takes off what its versions
// leave stray. An establishment's default line, a pot's link and a
// subscription's line belong to no month and are never read here: a save for a
// later month must not touch the current one. Of the tables that name a line,
// only tx_state and tx_split_parts are read or written here.

// feesLine is the built-in line, which the budget's migration seeds under this
// id.
const feesLine LineID = 1

// The month of a transaction is the month of its index row's Brazil day, so
// nothing here reads the ledger. ?1 is null for every indexed transaction, or a
// JSON array of the keys to judge.
const (
	statementStateLineReferences = `
SELECT s.tx_key, substr(i.day, 1, 7), s.line_id, s.pending_line_id, s.refund_line_id
FROM tx_state AS s JOIN tx_index AS i ON i.tx_key = s.tx_key
WHERE (s.line_id IS NOT NULL OR s.pending_line_id IS NOT NULL OR s.refund_line_id IS NOT NULL)
  AND (?1 IS NULL OR s.tx_key IN (SELECT value FROM json_each(?1)))
ORDER BY s.tx_key`
	statementSplitLineReferences = `
SELECT p.tx_key, substr(i.day, 1, 7), p.position, p.line_id, p.cents
FROM tx_split_parts AS p JOIN tx_index AS i ON i.tx_key = p.tx_key
WHERE p.line_id IS NOT NULL
  AND (?1 IS NULL OR p.tx_key IN (SELECT value FROM json_each(?1)))
ORDER BY p.tx_key, p.position`
)

// ClearingScope is the transactions a clearing judges.
type ClearingScope struct {
	keys  []TxKey
	every bool
}

// EveryIndexedTransaction is every transaction the index holds, whether the
// ledger still lists it or not. A budget save judges all of them: its versions
// can take a line from any month, an earlier one included.
func EveryIndexedTransaction() ClearingScope { return ClearingScope{every: true} }

// OnlyTransactions is the transactions at keys and no other, as when the index
// moves one transaction to another month.
func OnlyTransactions(keys ...TxKey) ClearingScope {
	return ClearingScope{keys: append([]TxKey{}, keys...)}
}

func (scope ClearingScope) argument() (any, error) {
	if scope.every {
		return nil, nil
	}
	set, err := json.Marshal(scope.keys)
	return string(set), err
}

// ClearLinesTheirMonthLacks takes off, inside the recorder's change, every
// reference that a transaction of scope holds to a line the budget in force for
// its month does not have, as that budget reads in the change's own
// transaction. Each reference is judged on its own:
//
//   - its own line goes with who put it there and the confidence, and the
//     question of why it was corrected is closed;
//   - a pending review keeps its reason and loses its suggestion and the
//     suggestion's confidence;
//   - a refund stays a refund and loses its line;
//   - a split part stays at its place with its amount and loses its line.
//
// Nothing else of the transaction changes and no history step is written. Each
// column that changes is an effect of the recorder's change, so the change's
// undo gives every reference back.
func (recorder *Recorder) ClearLinesTheirMonthLacks(ctx context.Context, scope ClearingScope) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	return recorder.guard(recorder.clearStrayLines(ctx, scope))
}

func (recorder *Recorder) clearStrayLines(ctx context.Context, scope ClearingScope) error {
	stray, err := findStrayReferences(ctx, recorder.tx, scope)
	if err != nil || stray.none() {
		return err
	}
	states, err := recorder.registry.table(txStateTable)
	if err != nil {
		return err
	}
	parts, err := recorder.registry.table(txSplitPartsTable)
	if err != nil {
		return err
	}
	for _, found := range stray.states {
		if err := recorder.set(ctx, states, []Value{Integer(int64(found.key))}, found.cleared()); err != nil {
			return err
		}
	}
	for _, part := range stray.parts {
		row := Row{"tx_key": Integer(int64(part.key)), "position": Integer(part.position), "line_id": Null, "cents": Integer(part.cents)}
		if err := recorder.put(ctx, parts, row); err != nil {
			return err
		}
	}
	return nil
}

// strayState is which of one transaction's own references name a line its
// month lacks.
type strayState struct {
	key                          TxKey
	line, suggestion, refundLine bool
}

func (found strayState) cleared() Row {
	row := Row{}
	if found.line {
		row["line_id"], row["classified_by"], row["confidence"], row["ask_reason"] = Null, Null, Null, Integer(0)
	}
	if found.suggestion {
		row["pending_line_id"], row["pending_confidence"] = Null, Null
	}
	if found.refundLine {
		row["refund_line_id"] = Null
	}
	return row
}

// strayPart is a split part on a line its month lacks.
type strayPart struct {
	key      TxKey
	position int64
	cents    int64
}

type strayReferences struct {
	states []strayState
	parts  []strayPart
}

func (stray strayReferences) none() bool { return len(stray.states) == 0 && len(stray.parts) == 0 }

type stateReference struct {
	key                          TxKey
	month                        brday.Month
	line, suggestion, refundLine *LineID
}

type partReference struct {
	key      TxKey
	month    brday.Month
	position int64
	line     LineID
	cents    int64
}

// findStrayReferences is every reference of scope to a line its month's budget
// lacks, in the order of the keys and, within a split, of the parts' places.
func findStrayReferences(ctx context.Context, tx *sql.Tx, scope ClearingScope) (strayReferences, error) {
	scoped, err := scope.argument()
	if err != nil {
		return strayReferences{}, err
	}
	states, err := stateReferences(ctx, tx, scoped)
	if err != nil {
		return strayReferences{}, err
	}
	parts, err := partReferences(ctx, tx, scoped)
	if err != nil {
		return strayReferences{}, err
	}
	months := monthLines{}
	var stray strayReferences
	for _, found := range states {
		has, err := months.of(ctx, tx, found.month)
		if err != nil {
			return strayReferences{}, err
		}
		judged := strayState{key: found.key, line: lacks(has, found.line), suggestion: lacks(has, found.suggestion), refundLine: lacks(has, found.refundLine)}
		if judged.line || judged.suggestion || judged.refundLine {
			stray.states = append(stray.states, judged)
		}
	}
	for _, part := range parts {
		has, err := months.of(ctx, tx, part.month)
		if err != nil {
			return strayReferences{}, err
		}
		if !has[part.line] {
			stray.parts = append(stray.parts, strayPart{key: part.key, position: part.position, cents: part.cents})
		}
	}
	return stray, nil
}

// lacks says whether a reference names a line the month does not have; an
// absent reference lacks nothing.
func lacks(has map[LineID]bool, line *LineID) bool { return line != nil && !has[*line] }

// monthLines holds, per month, the lines the budget in force for it has: its
// version's, and the fees line exactly when there is one of those, as the line
// menu and the reads have them.
type monthLines map[brday.Month]map[LineID]bool

func (cache monthLines) of(ctx context.Context, tx *sql.Tx, month brday.Month) (map[LineID]bool, error) {
	if has, found := cache[month]; found {
		return has, nil
	}
	budget, err := budgetInForce(ctx, tx, month)
	if err != nil {
		return nil, err
	}
	has := map[LineID]bool{}
	for _, line := range budget.Lines {
		has[line.ID] = true
	}
	if budget.HasLines() {
		has[feesLine] = true
	}
	cache[month] = has
	return has, nil
}

// stateReferences and partReferences read every row of scope first, so the
// budgets are read with no statement still open on the transaction.
func stateReferences(ctx context.Context, tx *sql.Tx, scoped any) ([]stateReference, error) {
	rows, err := tx.QueryContext(ctx, statementStateLineReferences, scoped)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []stateReference
	for rows.Next() {
		var key int64
		var month string
		var line, suggestion, refundLine sql.NullInt64
		if err := rows.Scan(&key, &month, &line, &suggestion, &refundLine); err != nil {
			return nil, err
		}
		found = append(found, stateReference{
			key: TxKey(key), month: brday.Month(month),
			line: nullable[LineID](line), suggestion: nullable[LineID](suggestion), refundLine: nullable[LineID](refundLine),
		})
	}
	return found, rows.Err()
}

func partReferences(ctx context.Context, tx *sql.Tx, scoped any) ([]partReference, error) {
	rows, err := tx.QueryContext(ctx, statementSplitLineReferences, scoped)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []partReference
	for rows.Next() {
		var key, line int64
		var month string
		var part partReference
		if err := rows.Scan(&key, &month, &part.position, &line, &part.cents); err != nil {
			return nil, err
		}
		part.key, part.month, part.line = TxKey(key), brday.Month(month), LineID(line)
		found = append(found, part)
	}
	return found, rows.Err()
}
