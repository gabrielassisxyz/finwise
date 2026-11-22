package api

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// The ids of the invented budget are the ones a fresh database mints for it,
// in the order of its groups: 2 Salário, 3 Renda extra, 4 Contas da casa,
// 5 Saúde, 6 Mercado, 7 Delivery, 8 Remédios, 9 Viagens. The fees line is 1.

// budgetState is the invented budget saved from 2026-02, with the months
// given closed.
func budgetState(closed ...brday.Month) func(t *testing.T, scratch *authoredtest.Scratch) {
	return func(t *testing.T, scratch *authoredtest.Scratch) {
		t.Helper()
		authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
		for _, month := range closed {
			authoredtest.CloseMonth(t, scratch.Database, fixtureNow, month, "2026-03-05")
		}
	}
}

func budgetAndScenarioState(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	budgetState()(t, scratch)
	authoredtest.SaveScenario(t, scratch.Database, fixtureNow, "Antes", authoredtest.InventedBudget())
}

const (
	// A line renamed and moved to another group under its id, and one new line.
	renamedAndNew = `[{"key":"fix","name":"Fixos e essenciais","lines":[{"id":"6","name":"Feira","amount":450.5},{"name":"Bônus-extra","amount":10}]}]`
	// A document of one income line, flagged.
	oneIncome = `[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":1000,"flag":"Não garantida"}]}]`
	// Two lines of one name.
	twiceTheName = `[{"key":"var","name":"Variáveis","lines":[{"name":"Mercado","amount":1},{"name":"Mercado","amount":2}]}]`
)

// budgetWriteCases are the three budget writes, each recorded and undone, and
// the two refusals the budget is the first to produce.
var budgetWriteCases = []writeCase{
	{method: "budget.save", arguments: `["2026-03",` + renamedAndNew + `]`, expects: expectsRecorded, state: budgetState()},
	{method: "budget.save", arguments: `["2026-04",` + oneIncome + `]`, expects: expectsRecorded, state: budgetState("2026-05")},
	{method: "budget.applyScenario", arguments: `["2026-04",` + oneIncome + `]`, expects: expectsRecorded, state: budgetState()},
	{method: "budget.saveScenario", arguments: `["  Sem extra ",` + renamedAndNew + `]`, expects: expectsRecorded, state: budgetAndScenarioState},
	{method: "budget.saveScenario", arguments: `["Antes",` + oneIncome + `]`, expects: expectsRecorded, state: budgetAndScenarioState},
	{method: "budget.save", arguments: `["2026-03",` + oneIncome + `]`, expects: expectsRefusal, reason: reasonMonthClosed, state: budgetState("2026-03")},
	{method: "budget.applyScenario", arguments: `["2026-03",` + twiceTheName + `]`, expects: expectsRefusal, reason: reasonMonthClosed, state: budgetState("2026-03")},
	{method: "budget.save", arguments: `["2026-03",` + twiceTheName + `]`, expects: expectsRefusal, reason: reasonInvalidBudget, state: budgetState()},
	{method: "budget.saveScenario", arguments: `["Antes",` + twiceTheName + `]`, expects: expectsRefusal, reason: reasonInvalidBudget, state: budgetState()},
}
