package authored

import (
	"context"
	"database/sql"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

// What the person says about being paid back for a purchase: that they expect
// it, how much, and which credit paid it. Each write sets columns of the
// purchase's facts through the recorder; the rules of when a write applies are
// the caller's.

const statementCreditLinks = `SELECT credit_tx_key, tx_key FROM tx_state WHERE credit_tx_key IS NOT NULL`

func stateKey(key TxKey) []Value { return []Value{Integer(int64(key))} }

// MarkReimbursable marks the transaction at key as one the person expects
// expected back for, with no credit linked: marking again starts over.
func (recorder *Recorder) MarkReimbursable(ctx context.Context, key TxKey, expected money.Cents) error {
	return recorder.Set(ctx, txStateTable, stateKey(key), Row{
		"reimbursable": Integer(1), "expected_cents": Integer(int64(expected)), "credit_tx_key": Null, "credit_cents": Null,
	})
}

// UnmarkReimbursable takes away the mark, the amount expected and the credit
// linked, which is then free to pay another purchase.
func (recorder *Recorder) UnmarkReimbursable(ctx context.Context, key TxKey) error {
	return recorder.Set(ctx, txStateTable, stateKey(key), Row{
		"reimbursable": Integer(0), "expected_cents": Null, "credit_tx_key": Null, "credit_cents": Null,
	})
}

// SetExpectedReimbursement stores what the person expects back, nil when they
// cleared it.
func (recorder *Recorder) SetExpectedReimbursement(ctx context.Context, key TxKey, expected *money.Cents) error {
	value := Null
	if expected != nil {
		value = Integer(int64(*expected))
	}
	return recorder.Set(ctx, txStateTable, stateKey(key), Row{"expected_cents": value})
}

// LinkReimbursement records that the credit at credit, of cents, paid the
// purchase at purchase back, replacing the credit it had, and settles the
// credit's pending review: a credit that pays a purchase back is nobody's
// income, so no line is left to choose for it, and a line an agent proposed for
// it is closed as stale. It answers whether the purchase's link changed.
func (recorder *Recorder) LinkReimbursement(ctx context.Context, purchase, credit TxKey, cents money.Cents) (bool, error) {
	before := recorder.seq
	link := Row{"credit_tx_key": Integer(int64(credit)), "credit_cents": Integer(int64(cents))}
	if err := recorder.Set(ctx, txStateTable, stateKey(purchase), link); err != nil {
		return false, err
	}
	linked := recorder.seq > before
	settled := Row{"pending_reason": Null, "pending_line_id": Null, "pending_confidence": Null}
	if err := recorder.Set(ctx, txStateTable, stateKey(credit), settled); err != nil {
		return false, err
	}
	if err := recorder.staleQueueProposals(ctx, credit); err != nil {
		return false, err
	}
	return linked, nil
}

// UnlinkReimbursement drops the credit linked to the purchase at key and keeps
// the mark and the amount expected. It answers whether there was a link.
func (recorder *Recorder) UnlinkReimbursement(ctx context.Context, key TxKey) (bool, error) {
	before := recorder.seq
	if err := recorder.Set(ctx, txStateTable, stateKey(key), Row{"credit_tx_key": Null, "credit_cents": Null}); err != nil {
		return false, err
	}
	return recorder.seq > before, nil
}

// CreditLinks is every credit a purchase names as its reimbursement, each with
// that purchase, whether or not the ledger still lists either of them.
func (view *View) CreditLinks(ctx context.Context) (map[TxKey]TxKey, error) {
	links := map[TxKey]TxKey{}
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementCreditLinks)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var credit, purchase int64
			if err := rows.Scan(&credit, &purchase); err != nil {
				return err
			}
			links[TxKey(credit)] = TxKey(purchase)
		}
		return rows.Err()
	})
	return links, err
}
