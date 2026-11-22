package ledger

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Each of these answers one query on a view of its own, opened for the call and
// closed after it: every call sees the file that is there when it is made. A
// caller that makes several queries and needs them to agree uses one View.
// The reader's pot yield query is in pot_yield.go, the only file allowed to name it.

// Accounts is View.Accounts on a view of its own.
func (reader *Reader) Accounts(ctx context.Context) ([]Account, error) {
	view := reader.View()
	defer view.Close()
	return view.Accounts(ctx)
}

// Bills is View.Bills on a view of its own.
func (reader *Reader) Bills(ctx context.Context) ([]Bill, error) {
	view := reader.View()
	defer view.Close()
	return view.Bills(ctx)
}

// BalanceHistory is View.BalanceHistory on a view of its own.
func (reader *Reader) BalanceHistory(ctx context.Context) ([]AccountDay, error) {
	view := reader.View()
	defer view.Close()
	return view.BalanceHistory(ctx)
}

// ReserveHistory is View.ReserveHistory on a view of its own.
func (reader *Reader) ReserveHistory(ctx context.Context) ([]Pot, error) {
	view := reader.View()
	defer view.Close()
	return view.ReserveHistory(ctx)
}

// Identities is View.Identities on a view of its own.
func (reader *Reader) Identities(ctx context.Context) ([]Identity, error) {
	view := reader.View()
	defer view.Close()
	return view.Identities(ctx)
}

// InvestmentPrincipals is View.InvestmentPrincipals on a view of its own.
func (reader *Reader) InvestmentPrincipals(ctx context.Context) (map[string]money.Cents, error) {
	view := reader.View()
	defer view.Close()
	return view.InvestmentPrincipals(ctx)
}

// Investments is View.Investments on a view of its own.
func (reader *Reader) Investments(ctx context.Context) ([]Investment, error) {
	view := reader.View()
	defer view.Close()
	return view.Investments(ctx)
}

// InvestmentHistory is View.InvestmentHistory on a view of its own.
func (reader *Reader) InvestmentHistory(ctx context.Context, investmentID string) ([]InvestmentDay, error) {
	view := reader.View()
	defer view.Close()
	return view.InvestmentHistory(ctx, investmentID)
}

// Pots is View.Pots on a view of its own.
func (reader *Reader) Pots(ctx context.Context, asOf brday.Day) ([]Pot, error) {
	view := reader.View()
	defer view.Close()
	return view.Pots(ctx, asOf)
}

// SyncState is View.SyncState on a view of its own.
func (reader *Reader) SyncState(ctx context.Context) (SyncState, error) {
	view := reader.View()
	defer view.Close()
	return view.SyncState(ctx)
}

// ProjectedRun is View.ProjectedRun on a view of its own.
func (reader *Reader) ProjectedRun(ctx context.Context) (int64, error) {
	view := reader.View()
	defer view.Close()
	return view.ProjectedRun(ctx)
}

// CleanTransactions is View.CleanTransactions on a view of its own.
func (reader *Reader) CleanTransactions(ctx context.Context, period brday.Period) ([]Transaction, error) {
	view := reader.View()
	defer view.Close()
	return view.CleanTransactions(ctx, period)
}

// YieldHistory is View.YieldHistory on a view of its own.
func (reader *Reader) YieldHistory(ctx context.Context) (YieldHistory, error) {
	view := reader.View()
	defer view.Close()
	return view.YieldHistory(ctx)
}
