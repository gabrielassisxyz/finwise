package authored

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// TxKey is a transaction's own key in the app's database. Everything said about
// a transaction is kept under it, whatever ids the bank gives the row.
type TxKey int64

// SeenTransaction is a ledger row as the index takes it in: its provider id and
// the facts the ledger gives it now.
type SeenTransaction struct {
	ProviderID       string
	AccountID        string
	Day              brday.Day
	Cents            money.Cents
	EstablishmentKey string
	BankPending      bool
	// Replaces is zero, or the key of the transaction this row continues, as a
	// posted row continues the pending row it replaces. A provider id never seen
	// is then mapped to that key instead of being given a new one.
	Replaces TxKey
}

// ErrProviderIDElsewhere is a provider id already mapped to another key than
// the one it was said to continue. One id names one transaction.
var ErrProviderIDElsewhere = errors.New("authored: the provider id already names another transaction")

// ErrNoSuchTxKey is a key the index has never given.
var ErrNoSuchTxKey = errors.New("authored: the index has no transaction under that key")

const (
	statementTxKeyOfProviderID = `SELECT tx_key FROM tx_ids WHERE provider_id = ?`
	statementMintTxKey         = `
INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending, evaluated) VALUES (?, ?, ?, ?, ?, 0)`
	statementMapProviderID = `INSERT INTO tx_ids (provider_id, tx_key) VALUES (?, ?)`
	statementRefreshTxKey  = `
UPDATE tx_index SET account_id = ?, day = ?, cents = ?, establishment_key = ?, bank_pending = ? WHERE tx_key = ?`
)

// IndexTransaction answers the key of the transaction a ledger row is, inside
// the recorder's change. A provider id never seen is given a new key, or the
// one seen.Replaces names; a known one keeps its key. Either way the key's
// index row takes the facts given, and nothing authored moves. The index is
// not journaled: this records no effect, does not write the change's row, and
// an undo of the change leaves what it wrote; a change that fails takes it back.
func (recorder *Recorder) IndexTransaction(ctx context.Context, seen SeenTransaction) (TxKey, error) {
	if err := recorder.usable(); err != nil {
		return 0, err
	}
	key, err := indexTransaction(ctx, recorder.tx, seen)
	if err != nil {
		return 0, recorder.fail(err)
	}
	return key, nil
}

func indexTransaction(ctx context.Context, tx *sql.Tx, seen SeenTransaction) (TxKey, error) {
	known, found, err := keyOfProviderID(ctx, tx, seen.ProviderID)
	if err != nil {
		return 0, err
	}
	switch {
	case found && seen.Replaces != 0 && known != seen.Replaces:
		return 0, ErrProviderIDElsewhere
	case found:
		return known, refreshTxKey(ctx, tx, known, seen)
	case seen.Replaces != 0:
		if err := refreshTxKey(ctx, tx, seen.Replaces, seen); err != nil {
			return 0, err
		}
		return seen.Replaces, mapProviderID(ctx, tx, seen.ProviderID, seen.Replaces)
	}
	result, err := tx.ExecContext(ctx, statementMintTxKey,
		seen.AccountID, string(seen.Day), int64(seen.Cents), seen.EstablishmentKey, zeroOrOne(seen.BankPending))
	if err != nil {
		return 0, err
	}
	minted, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return TxKey(minted), mapProviderID(ctx, tx, seen.ProviderID, TxKey(minted))
}

func keyOfProviderID(ctx context.Context, tx *sql.Tx, providerID string) (TxKey, bool, error) {
	var key int64
	err := tx.QueryRowContext(ctx, statementTxKeyOfProviderID, providerID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return TxKey(key), err == nil, err
}

func mapProviderID(ctx context.Context, tx *sql.Tx, providerID string, key TxKey) error {
	_, err := tx.ExecContext(ctx, statementMapProviderID, providerID, int64(key))
	return err
}

func refreshTxKey(ctx context.Context, tx *sql.Tx, key TxKey, seen SeenTransaction) error {
	result, err := tx.ExecContext(ctx, statementRefreshTxKey,
		seen.AccountID, string(seen.Day), int64(seen.Cents), seen.EstablishmentKey, zeroOrOne(seen.BankPending), int64(key))
	if err != nil {
		return err
	}
	refreshed, err := result.RowsAffected()
	if err == nil && refreshed == 0 {
		err = ErrNoSuchTxKey
	}
	return err
}

func zeroOrOne(on bool) int64 {
	if on {
		return 1
	}
	return 0
}
