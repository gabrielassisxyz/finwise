package authored

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// TransactionState is what was decided about a transaction. Its zero value is
// a transaction nobody decided anything about.
type TransactionState struct {
	Line *LineID
	// ClassifiedBy is who put the transaction on its line, empty for nobody.
	ClassifiedBy Author
	Confidence   *int64
	// PendingReason is empty unless the transaction waits for a review; the
	// suggestion that goes with it may be absent.
	PendingReason     string
	PendingLine       *LineID
	PendingConfidence *int64
	AskReason         bool
	Note              string
	// Exclusion is nil while the transaction counts, and empty while it is
	// switched off without a reason yet.
	Exclusion *string
	// Refund is nil while the ledger's rule decides.
	Refund        *bool
	RefundLine    *LineID
	Reimbursable  bool
	ExpectedCents *money.Cents
	Credit        *TxKey
	CreditCents   *money.Cents
}

// Tag is a tag with the place it was added at, which is never renumbered.
type Tag struct {
	Name     string
	Position int64
}

// SplitPart is one part of a split purchase. Line is nil for a part without one.
type SplitPart struct {
	Position int64
	Line     *LineID
	Cents    money.Cents
}

// HistoryStep is one step of a transaction's history with the change behind it.
// Reason is empty for a step the person gave no reason for.
type HistoryStep struct {
	ID     StepID
	Change ChangeID
	Kind   StepKind
	Text   string
	Day    brday.Day
	Reason string
}

// AuthoredTransaction is everything kept about one transaction: tags and parts
// in position order, the history oldest first.
type AuthoredTransaction struct {
	State   TransactionState
	Tags    []Tag
	Split   []SplitPart
	History []HistoryStep
}

// Establishment is what is kept about an establishment or a person. Nickname is
// empty for none; fixed tags are in position order.
type Establishment struct {
	Nickname    string
	DefaultLine *LineID
	FixedTags   []Tag
}

// Each read takes its whole set as one JSON array, so a list of transactions
// costs one statement per table and never one per transaction.
const (
	statementTxKeysOf = `
SELECT provider_id, tx_key FROM tx_ids WHERE provider_id IN (SELECT value FROM json_each(?))`
	statementTxStatesOf = `SELECT ` + txStateColumns + `
FROM tx_state WHERE tx_key IN (SELECT value FROM json_each(?))`
	statementTxTagsOf = `
SELECT tx_key, tag, position FROM tx_tags WHERE tx_key IN (SELECT value FROM json_each(?))
ORDER BY tx_key, position, tag`
	statementSplitPartsOf = `
SELECT tx_key, position, line_id, cents FROM tx_split_parts WHERE tx_key IN (SELECT value FROM json_each(?))
ORDER BY tx_key, position`
	statementHistoryStepsOf = `
SELECT s.tx_key, s.id, s.change_id, s.kind, s.text, s.day, r.reason
FROM history_steps AS s LEFT JOIN history_step_reasons AS r ON r.step_id = s.id
WHERE s.tx_key IN (SELECT value FROM json_each(?))
ORDER BY s.tx_key, s.id`
	statementEstablishmentsOf = `
SELECT key, nickname, default_line_id FROM establishments WHERE key IN (SELECT value FROM json_each(?))`
	statementFixedTagsOf = `
SELECT key, tag, position FROM establishment_tags WHERE key IN (SELECT value FROM json_each(?))
ORDER BY key, position, tag`
	statementKnownTags        = `SELECT tag FROM tx_tags UNION SELECT tag FROM establishment_tags ORDER BY tag`
	statementPurchaseOfCredit = `SELECT tx_key FROM tx_state WHERE credit_tx_key = ?`
	statementDayOfTxKey       = `SELECT day FROM tx_index WHERE tx_key = ?`
)

// TransactionKeys is the key of each provider id the index has met; an id never
// seen is absent.
func (view *View) TransactionKeys(ctx context.Context, providerIDs []string) (map[string]TxKey, error) {
	keys := map[string]TxKey{}
	if len(providerIDs) == 0 {
		return keys, nil
	}
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		set, err := json.Marshal(providerIDs)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, statementTxKeysOf, string(set))
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
			keys[providerID] = TxKey(key)
		}
		return rows.Err()
	})
	return keys, err
}

// Transactions is what is kept about each of keys. A key nothing is kept about
// is absent.
func (view *View) Transactions(ctx context.Context, keys []TxKey) (map[TxKey]AuthoredTransaction, error) {
	found := map[TxKey]AuthoredTransaction{}
	if len(keys) == 0 {
		return found, nil
	}
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		set, err := json.Marshal(keys)
		if err != nil {
			return err
		}
		for _, read := range []func(context.Context, *sql.Tx, string, map[TxKey]AuthoredTransaction) error{
			readTxStates, readTxTags, readSplitParts, readHistorySteps,
		} {
			if err := read(ctx, tx, string(set), found); err != nil {
				return err
			}
		}
		return nil
	})
	return found, err
}

func readTxStates(ctx context.Context, tx *sql.Tx, set string, found map[TxKey]AuthoredTransaction) error {
	rows, err := tx.QueryContext(ctx, statementTxStatesOf, set)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key int64
		var state TransactionState
		var line, confidence, pendingLine, pendingConfidence, refund, refundLine, expected, credit, creditCents sql.NullInt64
		var classifiedBy, pendingReason, exclusion sql.NullString
		var askReason, reimbursable int64
		err := rows.Scan(&key, &line, &classifiedBy, &confidence, &pendingReason, &pendingLine, &pendingConfidence,
			&askReason, &state.Note, &exclusion, &refund, &refundLine, &reimbursable, &expected, &credit, &creditCents)
		if err != nil {
			return err
		}
		state.Line, state.Confidence = nullable[LineID](line), nullable[int64](confidence)
		state.ClassifiedBy, state.PendingReason = Author(classifiedBy.String), pendingReason.String
		state.PendingLine, state.PendingConfidence = nullable[LineID](pendingLine), nullable[int64](pendingConfidence)
		state.AskReason, state.Reimbursable = askReason == 1, reimbursable == 1
		if exclusion.Valid {
			state.Exclusion = &exclusion.String
		}
		if refund.Valid {
			on := refund.Int64 == 1
			state.Refund = &on
		}
		state.RefundLine, state.ExpectedCents = nullable[LineID](refundLine), nullable[money.Cents](expected)
		state.Credit, state.CreditCents = nullable[TxKey](credit), nullable[money.Cents](creditCents)
		entry := found[TxKey(key)]
		entry.State = state
		found[TxKey(key)] = entry
	}
	return rows.Err()
}

func readTxTags(ctx context.Context, tx *sql.Tx, set string, found map[TxKey]AuthoredTransaction) error {
	rows, err := tx.QueryContext(ctx, statementTxTagsOf, set)
	if err != nil {
		return err
	}
	return scanTags(rows, func(key TxKey, tag Tag) {
		entry := found[key]
		entry.Tags = append(entry.Tags, tag)
		found[key] = entry
	})
}

func readSplitParts(ctx context.Context, tx *sql.Tx, set string, found map[TxKey]AuthoredTransaction) error {
	rows, err := tx.QueryContext(ctx, statementSplitPartsOf, set)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key, cents int64
		var part SplitPart
		var line sql.NullInt64
		if err := rows.Scan(&key, &part.Position, &line, &cents); err != nil {
			return err
		}
		part.Line, part.Cents = nullable[LineID](line), money.Cents(cents)
		entry := found[TxKey(key)]
		entry.Split = append(entry.Split, part)
		found[TxKey(key)] = entry
	}
	return rows.Err()
}

func readHistorySteps(ctx context.Context, tx *sql.Tx, set string, found map[TxKey]AuthoredTransaction) error {
	rows, err := tx.QueryContext(ctx, statementHistoryStepsOf, set)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key, id, change int64
		var kind, day string
		var step HistoryStep
		var reason sql.NullString
		if err := rows.Scan(&key, &id, &change, &kind, &step.Text, &day, &reason); err != nil {
			return err
		}
		step.ID, step.Change, step.Kind, step.Day, step.Reason = StepID(id), ChangeID(change), StepKind(kind), brday.Day(day), reason.String
		entry := found[TxKey(key)]
		entry.History = append(entry.History, step)
		found[TxKey(key)] = entry
	}
	return rows.Err()
}

// Establishments is what is kept about each of keys. A key nothing is kept
// about is absent.
func (view *View) Establishments(ctx context.Context, keys []string) (map[string]Establishment, error) {
	found := map[string]Establishment{}
	if len(keys) == 0 {
		return found, nil
	}
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		set, err := json.Marshal(keys)
		if err != nil {
			return err
		}
		if err := readEstablishments(ctx, tx, string(set), found); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, statementFixedTagsOf, string(set))
		if err != nil {
			return err
		}
		return scanTags(rows, func(key string, tag Tag) {
			entry := found[key]
			entry.FixedTags = append(entry.FixedTags, tag)
			found[key] = entry
		})
	})
	return found, err
}

func readEstablishments(ctx context.Context, tx *sql.Tx, set string, found map[string]Establishment) error {
	rows, err := tx.QueryContext(ctx, statementEstablishmentsOf, set)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var nickname sql.NullString
		var line sql.NullInt64
		if err := rows.Scan(&key, &nickname, &line); err != nil {
			return err
		}
		entry := found[key]
		entry.Nickname, entry.DefaultLine = nickname.String, nullable[LineID](line)
		found[key] = entry
	}
	return rows.Err()
}

// scanTags hands each row of a tag statement, in the statement's order, to add.
func scanTags[K TxKey | string](rows *sql.Rows, add func(K, Tag)) error {
	defer rows.Close()
	for rows.Next() {
		var owner K
		var tag Tag
		if err := rows.Scan(&owner, &tag.Name, &tag.Position); err != nil {
			return err
		}
		add(owner, tag)
	}
	return rows.Err()
}

// KnownTags is every distinct tag, a transaction's own or an establishment's
// fixed one, in the order of their text.
func (view *View) KnownTags(ctx context.Context) ([]string, error) {
	var tags []string
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementKnownTags)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tag string
			if err := rows.Scan(&tag); err != nil {
				return err
			}
			tags = append(tags, tag)
		}
		return rows.Err()
	})
	return tags, err
}

// PurchaseLinkedTo is the purchase whose reimbursement credit is linked; found
// is false when none links it.
func (view *View) PurchaseLinkedTo(ctx context.Context, credit TxKey) (purchase TxKey, found bool, err error) {
	read, err := view.read(ctx, func(tx *sql.Tx) error {
		var key int64
		err := tx.QueryRowContext(ctx, statementPurchaseOfCredit, int64(credit)).Scan(&key)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		purchase, found = TxKey(key), err == nil
		return err
	})
	if err != nil || !read {
		return 0, false, err
	}
	return purchase, found, nil
}

// TransactionDay is the Brazil day the index last saw the transaction on;
// found is false for a key it never gave.
func (view *View) TransactionDay(ctx context.Context, key TxKey) (day brday.Day, found bool, err error) {
	read, err := view.read(ctx, func(tx *sql.Tx) error {
		var stored string
		err := tx.QueryRowContext(ctx, statementDayOfTxKey, int64(key)).Scan(&stored)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		day, found = brday.Day(stored), err == nil
		return err
	})
	if err != nil || !read {
		return "", false, err
	}
	return day, found, nil
}

func nullable[T ~int64](value sql.NullInt64) *T {
	if !value.Valid {
		return nil
	}
	typed := T(value.Int64)
	return &typed
}
