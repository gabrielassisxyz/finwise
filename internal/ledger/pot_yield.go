package ledger

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// PotYield is what the collector's yield query says about one reserve of an
// account between two days. Yield is nil exactly when Reason is set: the ledger
// could not tie every transfer of the window to the reserve, and the honest
// answer is that the yield is unknown, never zero.
type PotYield struct {
	AccountID string
	From, To  brday.Day
	// ReserveID and ReserveName are nil on the row of a transfer that names no
	// reserve (reason "unmatched transfer"), and ReserveID is nil on an entry whose
	// identity is not stable.
	ReserveID   *string
	ReserveName *string
	// Previous is nil for a reserve opened inside the window, Current for one
	// closed inside it.
	Previous  *money.Cents
	Current   *money.Cents
	Transfers *money.Cents
	Yield     *money.Cents
	Reason    *string
}

// PotYields returns one row per reserve of the account for the window between
// two days, from < to. It returns no row at all when either day has no
// qualifying sighting of the account, or when both fall on the same sighting.
func (view *View) PotYields(ctx context.Context, accountID string, from, to brday.Day) ([]PotYield, error) {
	if err := requireOrderedDays(from, to); err != nil {
		return nil, err
	}
	var yields []PotYield
	err := view.read(ctx, func(tx *sql.Tx) error {
		window := []any{
			sql.Named("account_id", accountID), sql.Named("d1", string(from)), sql.Named("d2", string(to)),
		}
		// The three SELECTs of the README's one statement; why they are separate
		// is beside the constants in queries.go.
		for _, run := range []func() (*sql.Rows, error){
			func() (*sql.Rows, error) { return tx.QueryContext(ctx, queryPotYieldReserves, window...) },
			func() (*sql.Rows, error) { return tx.QueryContext(ctx, queryPotYieldUnstable, window...) },
			func() (*sql.Rows, error) { return tx.QueryContext(ctx, queryPotYieldTransfers, window...) },
		} {
			rows, err := collectPotYields(run, accountID, from, to)
			if err != nil {
				return err
			}
			yields = append(yields, rows...)
		}
		return nil
	})
	return yields, err
}

// PotYields is View.PotYields on a view of its own. It is here and not beside
// the reader's other queries because no other file may name it.
func (reader *Reader) PotYields(ctx context.Context, accountID string, from, to brday.Day) ([]PotYield, error) {
	view := reader.View()
	defer view.Close()
	return view.PotYields(ctx, accountID, from, to)
}

func collectPotYields(run func() (*sql.Rows, error), accountID string, from, to brday.Day) ([]PotYield, error) {
	rows, err := run()
	if err != nil {
		return nil, fmt.Errorf("ledger: pot yield of %s: %w", accountID, err)
	}
	defer rows.Close()

	var yields []PotYield
	for rows.Next() {
		yield, err := scanPotYield(rows, accountID)
		if err != nil {
			return nil, err
		}
		yield.From, yield.To = from, to
		yields = append(yields, yield)
	}
	return yields, rows.Err()
}

func requireOrderedDays(from, to brday.Day) error {
	first, err := brday.ParseDay(string(from))
	if err != nil {
		return err
	}
	last, err := brday.ParseDay(string(to))
	if err != nil {
		return err
	}
	if first >= last {
		return fmt.Errorf("ledger: pot yield window %s..%s is not increasing", first, last)
	}
	return nil
}

func scanPotYield(rows *sql.Rows, accountID string) (PotYield, error) {
	var (
		reserveID, reserveName, reason              sql.NullString
		previous, current, transfers, yieldedAmount sql.NullInt64
	)
	if err := rows.Scan(&reserveID, &reserveName, &previous, &current, &transfers,
		&yieldedAmount, &reason); err != nil {
		return PotYield{}, fmt.Errorf("ledger: pot yield of %s: %w", accountID, err)
	}
	return PotYield{
		AccountID: accountID, ReserveID: stringPointer(reserveID), ReserveName: stringPointer(reserveName),
		Previous: centsPointer(previous), Current: centsPointer(current),
		Transfers: centsPointer(transfers), Yield: centsPointer(yieldedAmount),
		Reason: stringPointer(reason),
	}, nil
}

// The query already rounds to integer cents, so no float is involved here.
func centsPointer(amount sql.NullInt64) *money.Cents {
	if !amount.Valid {
		return nil
	}
	cents := money.Cents(amount.Int64)
	return &cents
}
