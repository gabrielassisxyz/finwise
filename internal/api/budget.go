package api

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// The budget is something the person builds inside the app, and the backend has
// nowhere to keep it yet, so every method answers what a fresh install would:
// the groups with no lines, no scenarios, nothing planned.

// budgetView is the screen's figures. The rate is not worked out until a budget
// exists: it is unknown, not zero.
type budgetView struct {
	Month          brday.Month       `json:"month"`
	Closed         bool              `json:"closed"`
	Income         Reais             `json:"income"`
	Allocated      Reais             `json:"allocated"`
	InvestmentRate any               `json:"investmentRate"`
	Groups         []budgetGroupView `json:"groups"`
}

func registerBudget(registry *Registry) {
	registry.Register("budget.document", readBudgetDocument)
	registry.Register("budget.view", readBudgetView)
	registry.Register("budget.establishments", readBudgetEstablishments)
	registry.Register("budget.scenarios", readBudgetScenarios)
}

func readBudgetDocument(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	if _, err := onlyMonthArgument(args); err != nil {
		return nil, err
	}
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	return emptyBudgetDocument(), nil
}

// readBudgetView refuses a draft: evaluating one is the budget's arithmetic,
// which belongs to the part of the app that is not built.
func readBudgetView(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	if len(args) == 0 {
		return nil, &badArgumentsError{}
	}
	month, err := monthArgument(args[0])
	if err != nil {
		return nil, err
	}
	if len(args) > 1 && !isNull(args[1]) {
		return nil, &readOnlyError{}
	}
	fees, err := feesOfMonth(ctx, views.Ledger, month)
	if err != nil {
		return nil, err
	}
	return budgetView{Month: month, Groups: emptyBudgetGroups(fees)}, nil
}

func readBudgetEstablishments(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	if _, err := monthArgument(args[0]); err != nil {
		return nil, err
	}
	var line *string
	if err := decodeArgument(args[1], &line); err != nil || line == nil {
		return nil, &badArgumentsError{}
	}
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	return []any{}, nil
}

func readBudgetScenarios(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	return []any{}, nil
}

func isNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
