package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// monthDispositions is a month's listed transactions, as transactions.list
// answers them for that month, each with what it counts for in that month's
// budget: outcomes[i] is listed[i]'s.
type monthDispositions struct {
	listed   []txrules.Transaction
	outcomes []budgetrules.Outcome
}

// disposeMonth is the one place a figure of the budget starts from. It reads
// the month exactly as the list does, widened by the pairing window and cut
// back, joins what was authored about each transaction by its key, and lets
// Dispose decide what each counts for; nothing else decides it again.
func disposeMonth(ctx context.Context, views Views, month brday.Month) (monthDispositions, error) {
	period, err := brday.MonthPeriod(month)
	if err != nil {
		return monthDispositions{}, err
	}
	around, err := loadContext(ctx, views.Ledger)
	if err != nil {
		return monthDispositions{}, err
	}
	view, err := interpretPeriod(ctx, views.Ledger, around, period)
	if err != nil {
		return monthDispositions{}, err
	}
	rows, err := readAuthoredRows(ctx, views, view.listed)
	if err != nil {
		return monthDispositions{}, err
	}
	budget, err := views.Authored.BudgetOf(ctx, month)
	if err != nil {
		return monthDispositions{}, err
	}
	inputs := make([]budgetrules.Transaction, 0, len(view.listed))
	for _, listed := range view.listed {
		inputs = append(inputs, rows.disposable(listed))
	}
	return monthDispositions{listed: view.listed, outcomes: budgetrules.Dispose(transactionLinesOf(budget).budget, inputs)}, nil
}
