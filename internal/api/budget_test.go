package api

import (
	"net/http"
	"testing"
)

func init() {
	readCases = append(readCases,
		readCase{"budget.document", `["2026-03"]`},
		readCase{"budget.view", `["2026-03"]`},
		readCase{"budget.view", `["2026-03",null]`},
		readCase{"budget.establishments", `["2026-03","Mercado"]`},
		readCase{"budget.scenarios", `[]`},
	)
}

const emptyBudgetView = `{"month":"2026-03","closed":false,"income":0.00,"allocated":0.00,"investmentRate":null,"groups":[]}`

// expectAnswer calls a method over a fixture ledger and compares the exact body.
func expectAnswer(t *testing.T, env *readEnv, method, arguments, want string) {
	t.Helper()
	status, body := env.answer(method, arguments)
	if status != http.StatusOK || string(body) != want {
		t.Errorf("%s %s answered %d %s, want 200 %s", method, arguments, status, body, want)
	}
}

func TestTheBudgetAnswersAsAFreshInstallOverBothLedgers(t *testing.T) {
	for _, name := range []string{"full", "empty"} {
		env := newReadEnv(t, ledgerNamed(t, name))
		expectAnswer(t, env, "budget.document", `["2026-03"]`, `[]`)
		expectAnswer(t, env, "budget.view", `["2026-03"]`, emptyBudgetView)
		expectAnswer(t, env, "budget.view", `["2026-03",null]`, emptyBudgetView)
		expectAnswer(t, env, "budget.establishments", `["2026-03","Mercado"]`, `[]`)
		expectAnswer(t, env, "budget.scenarios", `[]`, `[]`)
	}
}

func TestTheBudgetViewNamesTheMonthItWasAskedFor(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	expectAnswer(t, env, "budget.view", `["2025-12"]`,
		`{"month":"2025-12","closed":false,"income":0.00,"allocated":0.00,"investmentRate":null,"groups":[]}`)
}

// A draft is refused with the very answer a write gets, so the two cannot drift.
func TestTheBudgetViewRefusesAnyDraftAsAWriteIsRefused(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	write := env.call("/api/budget/save", `["2026-03",[]]`, env.cookie)
	expectError(t, "a refused write", write, http.StatusForbidden, "read_only")
	drafts := []string{
		`[]`,
		`[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":8000}]}]`,
		`0`,
		`"x"`,
	}
	for _, draft := range drafts {
		response := env.call("/api/budget/view", `["2026-03",`+draft+`]`, env.cookie)
		expectError(t, "budget.view with the draft "+draft, response, http.StatusForbidden, "read_only")
		if response.Body.String() != write.Body.String() {
			t.Errorf("the refusal of a draft reads %s, a write's reads %s", response.Body, write.Body)
		}
	}
}

func TestTheBudgetMethodsRefuseArgumentsThatAreNotWhatTheyTake(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	calls := map[string]string{
		"/api/budget/document":       `["2026-13"]`,
		"/api/budget/view":           `["March"]`,
		"/api/budget/establishments": `["2026-03",5]`,
	}
	for path, body := range calls {
		expectError(t, path+" "+body, env.call(path, body, env.cookie), http.StatusBadRequest, "bad_request")
	}
	expectError(t, "a month that is not a string", env.call("/api/budget/document", `[3]`, env.cookie), http.StatusBadRequest, "bad_request")
}
