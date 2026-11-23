package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
)

// A month's payday transfers are derived here for every read that shows them:
// the payday, a pot's page, the net worth and the budget's investment made, so
// none of them can answer from a second derivation.

// monthTransfers is a month's transfers with their marks, and the budget in
// force they were derived from.
type monthTransfers struct {
	budget    authored.Budget
	plan      budgetrules.Plan
	transfers []budgetrules.MarkedTransfer
}

// transfersOfMonth derives a month's transfers from the app's database and the
// pots the ledger lists now, read without the yield and only when a line is
// linked to a pot.
func transfersOfMonth(ctx context.Context, views Views, month brday.Month) (monthTransfers, error) {
	return deriveTransfers(ctx, views.Authored, month, func() ([]potEntry, error) {
		pots, err := loadPots(ctx, views.Ledger, false)
		if err != nil {
			return nil, err
		}
		return pots.entries, nil
	})
}

// currentTransfers are the transfers of the current month over pots already
// loaded.
func (view *potsView) currentTransfers(ctx context.Context, chosen *authored.View) (monthTransfers, error) {
	return deriveTransfers(ctx, chosen, view.month, func() ([]potEntry, error) { return view.entries, nil })
}

func deriveTransfers(ctx context.Context, chosen *authored.View, month brday.Month, listed func() ([]potEntry, error)) (monthTransfers, error) {
	budget, err := chosen.BudgetOf(ctx, month)
	if err != nil {
		return monthTransfers{}, err
	}
	plan := planOfBudget(budget)
	links, err := linksOfPlan(ctx, chosen, plan)
	if err != nil {
		return monthTransfers{}, err
	}
	var pots []budgetrules.TransferPot
	if len(links) > 0 {
		entries, err := listed()
		if err != nil {
			return monthTransfers{}, err
		}
		pots = transferPotsOf(entries)
	}
	marks, err := chosen.TransferMarks(ctx, month)
	if err != nil {
		return monthTransfers{}, err
	}
	transfers := budgetrules.Transfers(budgetrules.PaydayInputs{
		Plan: plan, Links: links, Pots: pots, UnnamedPot: unnamedPot, InvestmentName: budgetGroupInvestment,
	})
	return monthTransfers{budget: budget, plan: plan, transfers: budgetrules.Marked(transfers, marks)}, nil
}

func transferPotsOf(entries []potEntry) []budgetrules.TransferPot {
	pots := make([]budgetrules.TransferPot, 0, len(entries))
	for _, entry := range entries {
		pots = append(pots, budgetrules.TransferPot{ID: entry.id, Name: entry.name, Closed: entry.closed})
	}
	return pots
}
