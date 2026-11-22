package ledger

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

// InvestmentPrincipals returns what was originally applied in each current
// investment, by investment id. It is shown beside an investment's own figures
// and is never added to anything: the provider's other amounts already hold it.
// An investment whose principal the provider does not state has no entry.
func (view *View) InvestmentPrincipals(ctx context.Context) (map[string]money.Cents, error) {
	principals := map[string]money.Cents{}
	err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, queryInvestmentPrincipals)
		if err != nil {
			return fmt.Errorf("ledger: investment principals: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var (
				id        string
				principal sql.NullFloat64
			)
			if err := rows.Scan(&id, &principal); err != nil {
				return fmt.Errorf("ledger: investment principals: %w", err)
			}
			cents, err := optionalCents(principal, "amountOriginal of "+id)
			if err != nil {
				return err
			}
			if cents != nil {
				principals[id] = *cents
			}
		}
		return rows.Err()
	})
	return principals, err
}
