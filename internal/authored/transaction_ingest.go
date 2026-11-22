package authored

import (
	"context"
	"database/sql"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// What taking a snapshot in reads and writes besides IndexTransaction: the
// whole index with the runs that listed each row, whether the rules have met a
// transaction, the examples an establishment's transactions give, a pending
// review, and the lines a month's budget has. The index columns are not
// journaled, so the writes to them record no effect.

const (
	statementIndexedTransactions = `
SELECT tx_key, account_id, day, cents, establishment_key, bank_pending, first_run, last_run, evaluated
FROM tx_index ORDER BY tx_key`
	statementIndexedProviderIDs = `SELECT provider_id, tx_key FROM tx_ids ORDER BY tx_key, provider_id`
	statementMarkListed         = `
UPDATE tx_index SET first_run = coalesce(first_run, ?1), last_run = max(coalesce(last_run, ?1), ?1)
WHERE tx_key = ?2`
	statementMarkMetUnlisted = `UPDATE tx_index SET first_run = coalesce(first_run, ?1) WHERE tx_key = ?2`
	statementMarkEvaluated   = `UPDATE tx_index SET evaluated = 1 WHERE tx_key = ?`
	statementLineExamples    = `
SELECT s.tx_key, i.day, s.line_id, s.classified_by, s.exclusion,
       EXISTS (SELECT 1 FROM tx_split_parts AS p WHERE p.tx_key = s.tx_key)
FROM tx_state AS s JOIN tx_index AS i ON i.tx_key = s.tx_key
WHERE i.establishment_key = ? AND s.line_id IS NOT NULL
ORDER BY s.tx_key`
)

// IndexedTransaction is a row of the index with every provider id mapped to it.
type IndexedTransaction struct {
	Key              TxKey
	ProviderIDs      []string
	AccountID        string
	Day              brday.Day
	Cents            money.Cents
	EstablishmentKey string
	BankPending      bool
	// FirstRun is the first collector run in which taking a snapshot in met the
	// row, listed or not; LastRun the last run whose snapshot listed it. Both
	// are nil for a row no snapshot taken in has met.
	FirstRun  *int64
	LastRun   *int64
	Evaluated bool
}

// IndexedTransactions is every row of the index, by key.
func (view *View) IndexedTransactions(ctx context.Context) ([]IndexedTransaction, error) {
	var found []IndexedTransaction
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		var err error
		if found, err = indexRows(ctx, tx); err != nil {
			return err
		}
		return attachProviderIDs(ctx, tx, found)
	})
	return found, err
}

func indexRows(ctx context.Context, tx *sql.Tx) ([]IndexedTransaction, error) {
	rows, err := tx.QueryContext(ctx, statementIndexedTransactions)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []IndexedTransaction
	for rows.Next() {
		var row IndexedTransaction
		var key, cents, pending, evaluated int64
		var day string
		var first, last sql.NullInt64
		if err := rows.Scan(&key, &row.AccountID, &day, &cents, &row.EstablishmentKey, &pending, &first, &last, &evaluated); err != nil {
			return nil, err
		}
		row.Key, row.Day, row.Cents = TxKey(key), brday.Day(day), money.Cents(cents)
		row.BankPending, row.Evaluated = pending == 1, evaluated == 1
		row.FirstRun, row.LastRun = nullable[int64](first), nullable[int64](last)
		found = append(found, row)
	}
	return found, rows.Err()
}

func attachProviderIDs(ctx context.Context, tx *sql.Tx, found []IndexedTransaction) error {
	at := make(map[TxKey]int, len(found))
	for i, row := range found {
		at[row.Key] = i
	}
	rows, err := tx.QueryContext(ctx, statementIndexedProviderIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var providerID string
		var key int64
		if err := rows.Scan(&providerID, &key); err != nil {
			return err
		}
		if i, indexed := at[TxKey(key)]; indexed {
			found[i].ProviderIDs = append(found[i].ProviderIDs, providerID)
		}
	}
	return rows.Err()
}

// MarkListed records that the snapshot of run listed the transaction at key:
// it is the row's last run, and its first unless one was met before.
func (recorder *Recorder) MarkListed(ctx context.Context, key TxKey, run int64) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	return recorder.updatedIndex(recorder.tx.ExecContext(ctx, statementMarkListed, run, int64(key)))
}

// MarkMetUnlisted records that taking in the snapshot of run met the
// transaction at key without the ledger listing it, so a row that vanished
// before any snapshot taken in listed it is told apart from one met before.
func (recorder *Recorder) MarkMetUnlisted(ctx context.Context, key TxKey, run int64) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	return recorder.updatedIndex(recorder.tx.ExecContext(ctx, statementMarkMetUnlisted, run, int64(key)))
}

// MarkEvaluated records that the rules that classify new transactions have met
// the transaction at key, so they never meet it again, whatever an undo does.
func (recorder *Recorder) MarkEvaluated(ctx context.Context, key TxKey) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	return recorder.updatedIndex(recorder.tx.ExecContext(ctx, statementMarkEvaluated, int64(key)))
}

// updatedIndex turns an update of one index row into the recorder's failure
// when it failed or found no row at the key.
func (recorder *Recorder) updatedIndex(result sql.Result, err error) error {
	if err == nil {
		var updated int64
		if updated, err = result.RowsAffected(); err == nil && updated == 0 {
			err = ErrNoSuchTxKey
		}
	}
	if err != nil {
		return recorder.fail(err)
	}
	return nil
}

// LineExample is an indexed transaction on a line, as the rules weigh it as an
// example of its establishment.
type LineExample struct {
	Key          TxKey
	Day          brday.Day
	Line         LineID
	ClassifiedBy Author
	// Exclusion is nil while the transaction counts, and empty while it is
	// switched off without a reason.
	Exclusion *string
	Split     bool
}

// LineExamples is every indexed transaction under establishmentKey that is on
// a line, by key.
func (view *View) LineExamples(ctx context.Context, establishmentKey string) ([]LineExample, error) {
	var examples []LineExample
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementLineExamples, establishmentKey)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var example LineExample
			var key, line, split int64
			var day string
			var by, exclusion sql.NullString
			if err := rows.Scan(&key, &day, &line, &by, &exclusion, &split); err != nil {
				return err
			}
			example.Key, example.Day, example.Line = TxKey(key), brday.Day(day), LineID(line)
			example.ClassifiedBy, example.Split = Author(by.String), split == 1
			if exclusion.Valid {
				example.Exclusion = &exclusion.String
			}
			examples = append(examples, example)
		}
		return rows.Err()
	})
	return examples, err
}

// SetPendingReview puts the transaction at key in the queue for reason, with
// the line it is offered and how sure the rules are of it, or with neither.
func (recorder *Recorder) SetPendingReview(ctx context.Context, key TxKey, reason string, line *LineID, confidence *int64) error {
	percent := Null
	if confidence != nil {
		percent = Integer(*confidence)
	}
	return recorder.Set(ctx, txStateTable, []Value{Integer(int64(key))}, Row{
		"pending_reason": Text(reason), "pending_line_id": lineValue(line), "pending_confidence": percent,
	})
}

// MonthLine is a line a month's budget has, with the name it has there.
type MonthLine struct {
	ID   LineID
	Name string
}

// LinesOfMonth is the lines the budget in force for month has, as its menu
// offers them: the version's, in its order, then the fees line exactly when
// there is one of those. A month with no budget, or with five empty groups,
// has none.
func (view *View) LinesOfMonth(ctx context.Context, month brday.Month) ([]MonthLine, error) {
	budget, err := view.BudgetOf(ctx, month)
	if err != nil || !budget.HasLines() {
		return nil, err
	}
	lines := make([]MonthLine, 0, len(budget.Lines)+1)
	for _, line := range budget.Lines {
		lines = append(lines, MonthLine{ID: line.ID, Name: line.Name})
	}
	return append(lines, MonthLine{ID: feesLine, Name: FeesLineName}), nil
}
