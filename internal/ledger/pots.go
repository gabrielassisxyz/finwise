package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Pot is one reserve entry (a caixinha) in an account's latest sighting on or
// before the day asked. A pot's key is ReserveID, and only when StableIdentity
// is true; names repeat across pots.
type Pot struct {
	AccountID         string
	Day               brday.Day
	ProviderUpdatedAt *time.Time
	Position          int
	ReserveID         *string
	Name              *string
	StableIdentity    bool
	Currency          *string
	// Amount is nil when AmountStatus is not "ok".
	Amount        *money.Cents
	CDIPercentage *float64
	AmountStatus  string
}

// Pots returns the latest sighting of every pot of every current account as of
// a day. A pot absent from an account's latest sighting has no balance and is
// not carried forward from an earlier day.
func (reader *Reader) Pots(ctx context.Context, asOf brday.Day) ([]Pot, error) {
	if _, err := brday.ParseDay(string(asOf)); err != nil {
		return nil, err
	}
	var pots []Pot
	err := reader.read(ctx, func(tx *sql.Tx) error {
		accounts, err := queryAccountRows(ctx, tx)
		if err != nil {
			return err
		}
		for _, account := range accounts {
			accountPots, err := queryAccountPots(ctx, tx, account.ID, asOf)
			if err != nil {
				return err
			}
			pots = append(pots, accountPots...)
		}
		return nil
	})
	return pots, err
}

func queryAccountPots(ctx context.Context, tx *sql.Tx, accountID string, asOf brday.Day) ([]Pot, error) {
	rows, err := tx.QueryContext(ctx, queryPotsAsOf,
		sql.Named("account_id", accountID), sql.Named("day", string(asOf)))
	if err != nil {
		return nil, fmt.Errorf("ledger: pots of %s: %w", accountID, err)
	}
	defer rows.Close()

	var pots []Pot
	for rows.Next() {
		pot, present, err := scanPot(rows, accountID)
		if err != nil {
			return nil, err
		}
		if present {
			pots = append(pots, pot)
		}
	}
	return pots, rows.Err()
}

// scanPot reports present=false for the empty row the LEFT JOIN yields when the
// account's latest sighting holds no reserve at all.
func scanPot(rows *sql.Rows, accountID string) (pot Pot, present bool, err error) {
	var (
		day, updated, reserveID, name, currency, status sql.NullString
		position, stable                                sql.NullInt64
		amount, cdi                                     sql.NullFloat64
	)
	if err = rows.Scan(&day, &updated, &position, &reserveID, &name, &stable,
		&currency, &amount, &cdi, &status); err != nil {
		return Pot{}, false, fmt.Errorf("ledger: pots of %s: %w", accountID, err)
	}
	if !position.Valid {
		return Pot{}, false, nil
	}
	pot = Pot{
		AccountID: accountID, Day: brday.Day(day.String), Position: int(position.Int64),
		ReserveID: stringPointer(reserveID), Name: stringPointer(name),
		StableIdentity: stable.Int64 == 1, Currency: stringPointer(currency),
		AmountStatus: status.String,
	}
	if cdi.Valid {
		pot.CDIPercentage = &cdi.Float64
	}
	if pot.ProviderUpdatedAt, err = optionalInstant(updated, "provider_updated_at of "+accountID); err != nil {
		return Pot{}, false, err
	}
	pot.Amount, err = optionalCents(amount, "pot amount of "+accountID)
	return pot, true, err
}
