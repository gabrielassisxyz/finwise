package api

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
)

// The budget the person built is read from the app's database, and a server
// without one answers what a fresh install would: the groups with no lines, no
// scenarios, nothing planned. The figures of the screen are worked out by
// budgetrules over the month's dispositions, for the month's budget or for a
// draft, which is weighed and never stored.

// budgetView is the screen's figures. The rate is unknown, not zero, while
// there is no income to take a share of.
type budgetView struct {
	Month          brday.Month       `json:"month"`
	Closed         bool              `json:"closed"`
	Income         Reais             `json:"income"`
	Allocated      Reais             `json:"allocated"`
	InvestmentRate *int64            `json:"investmentRate"`
	Groups         []budgetGroupView `json:"groups"`
}

type budgetEstablishmentWire struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Amount Reais  `json:"amount"`
	Count  int    `json:"count"`
}

type investmentFiguresWire struct {
	Investment Reais  `json:"investment"`
	Rate       *int64 `json:"rate"`
}

type scenarioImpactWire struct {
	Current    investmentFiguresWire `json:"current"`
	Scenario   investmentFiguresWire `json:"scenario"`
	Difference Reais                 `json:"difference"`
	Changed    [][]bool              `json:"changed"`
}

func registerBudget(registry *Registry) {
	registry.Register("budget.document", readBudgetDocument)
	registry.Register("budget.view", readBudgetView)
	registry.Register("budget.establishments", readBudgetEstablishments)
	registry.Register("budget.scenarios", readBudgetScenarios)
	registry.RegisterWrite("budget.save", saveBudget)
	registry.RegisterWrite("budget.applyScenario", applyScenario)
	registry.RegisterWrite("budget.saveScenario", saveScenario)
	registry.Register("budget.evaluateScenario", evaluateScenario)
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

// readBudgetView weighs the month's budget, or the draft when one is sent.
// What a schema cannot say about the arguments is checked before anything is
// read: the month, and each amount, which a draft may not send negative.
func readBudgetView(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	if len(args) == 0 {
		return nil, &badArgumentsError{}
	}
	month, err := monthArgument(args[0])
	if err != nil {
		return nil, err
	}
	var draft authored.Document
	drafted := len(args) > 1 && !isNull(args[1])
	if drafted {
		if draft, err = draftArgument(args[1]); err != nil {
			return nil, err
		}
	}
	disposed, err := disposeMonth(ctx, views, month)
	if err != nil {
		return nil, err
	}
	plan := planOfDocument(draft)
	if !drafted {
		budget, err := views.Authored.BudgetOf(ctx, month)
		if err != nil {
			return nil, err
		}
		plan = planOfBudget(budget)
	}
	facts, err := monthFactsOf(ctx, views, month, plan, disposed)
	if err != nil {
		return nil, err
	}
	return budgetViewWire(month, budgetrules.View(plan, facts)), nil
}

// readBudgetEstablishments lists the places that make a line up in a month,
// the line named as the month's budget names it.
func readBudgetEstablishments(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	month, err := monthArgument(args[0])
	if err != nil {
		return nil, err
	}
	var name *string
	if err := decodeArgument(args[1], &name); err != nil || name == nil {
		return nil, &badArgumentsError{}
	}
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	budget, err := views.Authored.BudgetOf(ctx, month)
	if err != nil {
		return nil, err
	}
	line, found := lineOfMonth(budget, *name)
	if !found {
		return []budgetEstablishmentWire{}, nil
	}
	disposed, err := disposeMonth(ctx, views, month)
	if err != nil {
		return nil, err
	}
	return establishmentsWire(ctx, views, line, disposed)
}

// lineOfMonth is the line a name names in the month. The fees line is named in
// every month, as the view shows it in every month, with a budget or without.
func lineOfMonth(budget authored.Budget, name string) (authored.LineID, bool) {
	if name == authored.FeesLineName {
		return feesLineID, true
	}
	for _, line := range budget.Lines {
		if line.Name == name {
			return line.ID, true
		}
	}
	return 0, false
}

// establishmentsWire names each establishment as the transactions show it:
// the nickname the person gave it, else the name of its latest transaction.
func establishmentsWire(ctx context.Context, views Views, line authored.LineID, disposed monthDispositions) ([]budgetEstablishmentWire, error) {
	keyed := make([]budgetrules.KeyedOutcome, 0, len(disposed.listed))
	ledgerNames := map[string]string{}
	for i, listed := range disposed.listed {
		keyed = append(keyed, budgetrules.KeyedOutcome{EstablishmentKey: listed.EstablishmentKey, Outcome: disposed.outcomes[i]})
		if _, named := ledgerNames[listed.EstablishmentKey]; !named {
			ledgerNames[listed.EstablishmentKey] = listed.Name
		}
	}
	establishments, err := establishmentsOf(ctx, views.Authored, disposed.listed)
	if err != nil {
		return nil, err
	}
	totals := budgetrules.EstablishmentsOfLine(budgetrules.LineID(line), keyed)
	answer := make([]budgetEstablishmentWire, 0, len(totals))
	for _, total := range totals {
		answer = append(answer, budgetEstablishmentWire{
			Key: total.Key, Name: shownName(establishments, total.Key, ledgerNames[total.Key]),
			Amount: Reais(total.Amount), Count: total.Count,
		})
	}
	return answer, nil
}

// evaluateScenario weighs a scenario against the budget in force for the month
// asked. It reads nothing of the ledger, so it answers while the snapshot is
// missing, and it stores nothing.
func evaluateScenario(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	month, err := monthArgument(args[0])
	if err != nil {
		return nil, err
	}
	scenario, err := draftArgument(args[1])
	if err != nil {
		return nil, err
	}
	budget, err := views.Authored.BudgetOf(ctx, month)
	if err != nil {
		return nil, err
	}
	impact := budgetrules.ScenarioImpact(planOfBudget(budget), planOfDocument(scenario))
	return scenarioImpactWire{
		Current:    investmentFiguresWire{Investment: Reais(impact.Current.Investment), Rate: impact.Current.Rate},
		Scenario:   investmentFiguresWire{Investment: Reais(impact.Scenario.Investment), Rate: impact.Scenario.Rate},
		Difference: Reais(impact.Difference), Changed: impact.Changed,
	}, nil
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
