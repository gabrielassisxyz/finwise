package ledger

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Bill is a card bill as the provider reports it. A bill's items are the
// transactions whose card metadata carries its id.
type Bill struct {
	ID             string
	AccountID      string
	DueDate        brday.Day
	ClosingDate    *brday.Day
	TotalAmount    money.Cents
	MinimumPayment *money.Cents
}

// Bills returns every current bill, by account and due date.
func (view *View) Bills(ctx context.Context) ([]Bill, error) {
	var bills []Bill
	err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, queryBills)
		if err != nil {
			return fmt.Errorf("ledger: bills: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			bill, err := scanBill(rows)
			if err != nil {
				return err
			}
			bills = append(bills, bill)
		}
		return rows.Err()
	})
	return bills, err
}

func scanBill(rows *sql.Rows) (Bill, error) {
	var (
		bill                    Bill
		accountID, due, closing sql.NullString
		total, minimum          sql.NullFloat64
	)
	if err := rows.Scan(&bill.ID, &accountID, &due, &closing, &total, &minimum); err != nil {
		return Bill{}, fmt.Errorf("ledger: bills: %w", err)
	}
	bill.AccountID = accountID.String
	field := func(name string) string { return "bills." + name + " of " + bill.ID }

	var err error
	if bill.DueDate, err = dayOfDatePart(due.String, field("due_date")); err != nil {
		return Bill{}, err
	}
	if bill.ClosingDate, err = optionalDayOfDatePart(closing, field("closing_date")); err != nil {
		return Bill{}, err
	}
	if bill.TotalAmount, err = requiredCents(total, field("total_amount")); err != nil {
		return Bill{}, err
	}
	bill.MinimumPayment, err = optionalCents(minimum, field("minimum_payment"))
	return bill, err
}
