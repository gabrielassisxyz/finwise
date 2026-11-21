package api

import (
	"net/http"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
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

const emptyBudgetDocumentJSON = `[{"key":"rec","name":"Receita","lines":[]},{"key":"fix","name":"Fixos e essenciais","lines":[]},` +
	`{"key":"var","name":"Variáveis","lines":[]},{"key":"prov","name":"Provisões","lines":[]},` +
	`{"key":"res","name":"Reservas de desejo","lines":[]}]`

func groupJSON(key, name, spent string) string {
	return `{"key":"` + key + `","name":"` + name + `","subtotal":0.00,"subtotalSpent":` + spent + `,"over":false,"lines":[]}`
}

func moneyCellJSON(amount string) string { return `{"type":"money","amount":` + amount + `}` }

// emptyBudgetViewOf is the view of a month with no budget, written out in full
// so that a group missing or a figure moved is seen. The only figure that is not
// zero is the fees the bank charged, as the contract's decimal.
func emptyBudgetViewOf(month, fees string) string {
	feesLine := `{"name":"Tarifas e juros","cells":[` + moneyCellJSON(fees) +
		`,{"type":"none"},{"type":"none"}],"marks":[],"over":false,"bar":null,"expandable":true}`
	investmentLine := `{"name":"Investimento","cells":[` + moneyCellJSON("0.00") + `,` + moneyCellJSON("0.00") +
		`,{"type":"percent","value":null}],"marks":[],"over":false,"bar":null,"expandable":false}`
	return `{"month":"` + month + `","closed":false,"income":0.00,"allocated":0.00,"investmentRate":null,"groups":[` +
		groupJSON("rec", "Receita", "null") + `,` + groupJSON("fix", "Fixos e essenciais", "0.00") + `,` +
		groupJSON("var", "Variáveis", "0.00") + `,` + groupJSON("prov", "Provisões", "null") + `,` +
		groupJSON("res", "Reservas de desejo", "null") + `,` + groupJSON("rf", "Rainy Funds", "null") + `,` +
		`{"key":"tar","name":"Tarifas e juros","subtotal":` + fees + `,"subtotalSpent":null,"over":false,"lines":[` + feesLine + `]},` +
		`{"key":"inv","name":"Investimento","subtotal":0.00,"subtotalSpent":null,"over":false,"lines":[` + investmentLine + `]}]}`
}

// expectAnswer calls a method over a fixture ledger and compares the exact body.
func expectAnswer(t *testing.T, env *readEnv, method, arguments, want string) {
	t.Helper()
	status, body := env.answer(method, arguments)
	if status != http.StatusOK || string(body) != want {
		t.Errorf("%s %s answered %d %s, want 200 %s", method, arguments, status, body, want)
	}
}

// The full ledger holds one 25 reais fee in March and the empty one nothing.
func TestTheBudgetAnswersAsAFreshInstallOverBothLedgers(t *testing.T) {
	for name, fees := range map[string]string{"full": "25.00", "empty": "0.00"} {
		env := newReadEnv(t, ledgerNamed(t, name))
		expectAnswer(t, env, "budget.document", `["2026-03"]`, emptyBudgetDocumentJSON)
		expectAnswer(t, env, "budget.view", `["2026-03"]`, emptyBudgetViewOf("2026-03", fees))
		expectAnswer(t, env, "budget.view", `["2026-03",null]`, emptyBudgetViewOf("2026-03", fees))
		expectAnswer(t, env, "budget.establishments", `["2026-03","Mercado"]`, `[]`)
		expectAnswer(t, env, "budget.scenarios", `[]`, `[]`)
	}
}

func TestTheBudgetViewNamesTheMonthItWasAskedForAndOnlyCountsItsFees(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	expectAnswer(t, env, "budget.view", `["2025-12"]`, emptyBudgetViewOf("2025-12", "0.00"))
	expectAnswer(t, env, "budget.view", `["2026-02"]`, emptyBudgetViewOf("2026-02", "0.00"))
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
	calls := []struct{ path, body string }{
		{"/api/budget/document", `["2026-13"]`},
		{"/api/budget/document", `[3]`},
		{"/api/budget/view", `["March"]`},
		{"/api/budget/establishments", `["2026-03",5]`},
	}
	for _, call := range calls {
		expectError(t, call.path+" "+call.body, env.call(call.path, call.body, env.cookie), http.StatusBadRequest, "bad_request")
	}
}

func feeRow(amount money.Cents, change func(*txrules.Transaction)) txrules.Transaction {
	row := txrules.Transaction{Amount: amount, Fee: true, Posted: true}
	if change != nil {
		change(&row)
	}
	return row
}

func TestSpentOnFeesCountsWhatWasChargedNetOfDiscounts(t *testing.T) {
	listed := []txrules.Transaction{
		feeRow(-2500, nil),
		feeRow(-1000, nil),
		feeRow(300, nil),
		feeRow(-9900, func(row *txrules.Transaction) { row.Posted = false }),
		feeRow(-9900, func(row *txrules.Transaction) { row.Movement = txrules.OwnTransfer }),
		feeRow(-9900, func(row *txrules.Transaction) { row.Unconverted = true }),
		feeRow(-9900, func(row *txrules.Transaction) { row.Fee = false }),
	}
	if got := spentOnFees(listed); got != 3200 {
		t.Errorf("spent %d cents on fees, want 3200", got)
	}
}
