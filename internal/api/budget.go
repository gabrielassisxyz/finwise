package api

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// The budget the person built is read from the app's database, and a server
// without one answers what a fresh install would: the groups with no lines, no
// scenarios, nothing planned. The figures of the screen (budget.view,
// budget.establishments) are still the fresh install's.

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
	registry.RegisterWrite("budget.save", saveBudget)
	registry.RegisterWrite("budget.applyScenario", applyScenario)
	registry.RegisterWrite("budget.saveScenario", saveScenario)
}

func readBudgetDocument(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	month, err := onlyMonthArgument(args)
	if err != nil {
		return nil, err
	}
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	budget, err := views.Authored.BudgetOf(ctx, month)
	if err != nil {
		return nil, err
	}
	return documentOf(budget), nil
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

// readBudgetScenarios names each line as the budget in force today names it, so
// applying an old scenario does not bring an old name back.
func readBudgetScenarios(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	scenarios, err := views.Authored.Scenarios(ctx, brday.MonthOf(nowOf(ctx)))
	if err != nil {
		return nil, err
	}
	answer := make([]savedScenarioWire, 0, len(scenarios))
	for _, scenario := range scenarios {
		answer = append(answer, savedScenarioWire{Name: scenario.Name, Budget: documentOfScenario(scenario.Document)})
	}
	return answer, nil
}

func isNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
