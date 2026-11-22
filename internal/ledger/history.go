package ledger

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// AccountDay is an account on one day it was sighted. The days an account was
// sighted are what tell an account that emptied every pot from one nobody looked
// at, which the pot rows alone cannot say.
type AccountDay struct {
	AccountID string
	Day       brday.Day
	// Balance is nil when the sighting carries no readable balance.
	Balance *money.Cents
}

// BalanceHistory returns every sighting of every account in the ledger, by
// account and day. It includes accounts that are no longer current; the caller
// keeps the ones it asks about.
func (view *View) BalanceHistory(ctx context.Context) ([]AccountDay, error) {
	var days []AccountDay
	err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, queryBalanceHistory)
		if err != nil {
			return fmt.Errorf("ledger: balance history: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			day, err := scanAccountDay(rows)
			if err != nil {
				return err
			}
			days = append(days, day)
		}
		return rows.Err()
	})
	return days, err
}

func scanAccountDay(rows *sql.Rows) (AccountDay, error) {
	var (
		day      AccountDay
		observed sql.NullString
		balance  sql.NullFloat64
	)
	if err := rows.Scan(&day.AccountID, &observed, &balance); err != nil {
		return AccountDay{}, fmt.Errorf("ledger: balance history: %w", err)
	}
	day.Day = brday.Day(observed.String)
	var err error
	day.Balance, err = optionalCents(balance, "balance of "+day.AccountID)
	return day, err
}

// ReserveHistory returns every reserve entry of every sighting, by account, day
// and position. A sighting that holds no pot has no row here; BalanceHistory is
// where it shows. ProviderUpdatedAt is not read.
func (view *View) ReserveHistory(ctx context.Context) ([]Pot, error) {
	var pots []Pot
	err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, queryReserveHistory)
		if err != nil {
			return fmt.Errorf("ledger: reserve history: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			pot, err := scanReserveEntry(rows)
			if err != nil {
				return err
			}
			pots = append(pots, pot)
		}
		return rows.Err()
	})
	return pots, err
}

func scanReserveEntry(rows *sql.Rows) (Pot, error) {
	var (
		accountID                                    string
		day, reserveID, name, currency, amountStatus sql.NullString
		position, stable                             sql.NullInt64
		amount, cdi                                  sql.NullFloat64
	)
	if err := rows.Scan(&accountID, &day, &position, &reserveID, &name, &stable,
		&currency, &amount, &cdi, &amountStatus); err != nil {
		return Pot{}, fmt.Errorf("ledger: reserve history: %w", err)
	}
	pot := Pot{
		AccountID: accountID, Day: brday.Day(day.String), Position: int(position.Int64),
		ReserveID: stringPointer(reserveID), Name: stringPointer(name),
		StableIdentity: stable.Int64 == 1, Currency: stringPointer(currency),
		AmountStatus: amountStatus.String,
	}
	if cdi.Valid {
		pot.CDIPercentage = &cdi.Float64
	}
	var err error
	pot.Amount, err = optionalCents(amount, "pot amount of "+accountID)
	return pot, err
}
