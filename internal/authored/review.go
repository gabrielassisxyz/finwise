package authored

import (
	"context"
	"database/sql"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// What the review reads of the app's database besides a transaction's own
// rows: which transactions wait for a review, and which a kind of change has
// settled.

const (
	statementPendingTxKeys = `SELECT tx_key FROM tx_state WHERE pending_reason IS NOT NULL ORDER BY tx_key`

	// A transaction a change settled is the key of a tx_state row it wrote;
	// the key of an effect is the row's key as a JSON array.
	statementTransactionsSettledBy = `
SELECT DISTINCT CAST(json_extract(effects.key, '$[0]') AS INTEGER) AS tx_key
FROM changes JOIN change_effects AS effects ON effects.change_id = changes.id
WHERE changes.method = ? AND changes.undone_at IS NULL AND changes.day BETWEEN ? AND ?
  AND effects.tbl = 'tx_state'
ORDER BY tx_key`
)

// PendingTransactionKeys is every transaction a review waits for, by ascending
// key, whether or not the ledger lists it now; none on the empty view.
func (view *View) PendingTransactionKeys(ctx context.Context) ([]TxKey, error) {
	var keys []TxKey
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementPendingTxKeys)
		if err != nil {
			return err
		}
		keys, err = scanTxKeys(rows)
		return err
	})
	return keys, err
}

// TransactionsSettledBy is every transaction whose facts a change of method,
// not undone, wrote on a day of period, each once by ascending key; none on the
// empty view. A change taken back step by step is not undone, so what it wrote
// may have been restored since.
func (view *View) TransactionsSettledBy(ctx context.Context, method string, period brday.Period) ([]TxKey, error) {
	var keys []TxKey
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementTransactionsSettledBy, method, string(period.First), string(period.Last))
		if err != nil {
			return err
		}
		keys, err = scanTxKeys(rows)
		return err
	})
	return keys, err
}

func scanTxKeys(rows *sql.Rows) ([]TxKey, error) {
	defer rows.Close()
	var keys []TxKey
	for rows.Next() {
		var key int64
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, TxKey(key))
	}
	return keys, rows.Err()
}

// DismissedWarnings is the id of every warning the person dismissed, whether
// or not it holds now; none on the empty view. A row stays after its warning
// stops holding, so the same warning holding again later is still dismissed.
func (view *View) DismissedWarnings(ctx context.Context) (map[string]bool, error) {
	dismissed := map[string]bool{}
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementDismissedWarningAll)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			dismissed[id] = true
		}
		return rows.Err()
	})
	return dismissed, err
}

// DismissWarning makes id a dismissed warning inside the open change. A
// warning already dismissed records nothing. The store checks no warning: the
// table refuses an empty id, and a caller dismisses only what holds.
func (recorder *Recorder) DismissWarning(ctx context.Context, id string) error {
	return recorder.Put(ctx, dismissedWarningsTable, Row{"id": Text(id)})
}
