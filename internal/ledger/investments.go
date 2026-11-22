package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Investment is a current investment. Balance is the provider's figure; the
// gross, tax and net split of a day is in InvestmentHistory.
type Investment struct {
	ID       string
	ItemID   string
	Type     string
	Subtype  string
	Name     string
	Currency string
	Balance  money.Cents
}

// InvestmentDay is one investment on one day of observation.
type InvestmentDay struct {
	InvestmentID      string
	Day               brday.Day
	ProviderUpdatedAt *time.Time
	Currency          *string
	AmountGross       *money.Cents
	// Taxes reads as provisioned income tax and Taxes2 as provisioned IOF; that
	// is inferred by the collector's authors, not published by the provider.
	Taxes     *money.Cents
	Taxes2    *money.Cents
	AmountNet *money.Cents
	Rate      *float64
	RateType  *string
	DueDate   *brday.Day
	Status    *string
}

// Investments returns every current investment, by id.
func (view *View) Investments(ctx context.Context) ([]Investment, error) {
	var investments []Investment
	err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, queryInvestments)
		if err != nil {
			return fmt.Errorf("ledger: investments: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			investment, err := scanInvestment(rows)
			if err != nil {
				return err
			}
			investments = append(investments, investment)
		}
		return rows.Err()
	})
	return investments, err
}

func scanInvestment(rows *sql.Rows) (Investment, error) {
	var (
		investment                            Investment
		itemID, kind, subtype, name, currency sql.NullString
		balance                               sql.NullFloat64
	)
	if err := rows.Scan(&investment.ID, &itemID, &kind, &subtype, &name, &balance, &currency); err != nil {
		return Investment{}, fmt.Errorf("ledger: investments: %w", err)
	}
	investment.ItemID, investment.Type, investment.Subtype = itemID.String, kind.String, subtype.String
	investment.Name, investment.Currency = name.String, currency.String

	var err error
	investment.Balance, err = requiredCents(balance, "investments.balance of "+investment.ID)
	return investment, err
}

// InvestmentHistory returns one investment's observations, one per day, oldest
// first. amountOriginal is not carried: it is never summed.
func (view *View) InvestmentHistory(ctx context.Context, investmentID string) ([]InvestmentDay, error) {
	var days []InvestmentDay
	err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, queryInvestmentHistory, sql.Named("investment_id", investmentID))
		if err != nil {
			return fmt.Errorf("ledger: investment history of %s: %w", investmentID, err)
		}
		defer rows.Close()
		for rows.Next() {
			day, err := scanInvestmentDay(rows, investmentID)
			if err != nil {
				return err
			}
			days = append(days, day)
		}
		return rows.Err()
	})
	return days, err
}

func scanInvestmentDay(rows *sql.Rows, investmentID string) (InvestmentDay, error) {
	var (
		day                                   InvestmentDay
		observed, updated, currency, rateType sql.NullString
		due, status                           sql.NullString
		gross, taxes, taxes2, net, rate       sql.NullFloat64
	)
	if err := rows.Scan(&observed, &updated, &currency, &gross, &taxes, &taxes2, &net,
		&rate, &rateType, &due, &status); err != nil {
		return InvestmentDay{}, fmt.Errorf("ledger: investment history of %s: %w", investmentID, err)
	}
	field := func(name string) string { return name + " of " + investmentID }

	day.InvestmentID, day.Day = investmentID, brday.Day(observed.String)
	day.Currency, day.RateType, day.Status = stringPointer(currency), stringPointer(rateType), stringPointer(status)
	if rate.Valid {
		day.Rate = &rate.Float64
	}
	return completeInvestmentDay(day, updated, due, [4]sql.NullFloat64{gross, taxes, taxes2, net}, field)
}

func completeInvestmentDay(day InvestmentDay, updated, due sql.NullString,
	amounts [4]sql.NullFloat64, field func(string) string) (InvestmentDay, error) {
	var err error
	if day.ProviderUpdatedAt, err = optionalInstant(updated, field("provider_updated_at")); err != nil {
		return InvestmentDay{}, err
	}
	if day.DueDate, err = optionalDayOfDatePart(due, field("due_date")); err != nil {
		return InvestmentDay{}, err
	}
	targets := []**money.Cents{&day.AmountGross, &day.Taxes, &day.Taxes2, &day.AmountNet}
	names := []string{"amount_gross", "taxes", "taxes2", "amount_net"}
	for i, amount := range amounts {
		if *targets[i], err = optionalCents(amount, field(names[i])); err != nil {
			return InvestmentDay{}, err
		}
	}
	return day, nil
}
