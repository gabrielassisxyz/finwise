package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// The budget's example: the full ledger with a month of purchases added, and a
// budget from 2026-03 that classifies them. Everything is invented.
const (
	txMarketFirst  = "tx-market-first"
	txMarketSecond = "tx-market-second"
	txFair         = "tx-fair"
	txDelivery     = "tx-delivery"
	txWholesale    = "tx-wholesale"
	txMarketRefund = "tx-market-refund"
	txOtherPerson  = "tx-other-person"
	txUnreviewed   = "tx-unreviewed"
	txWages        = "tx-wages"
	txAprilFair    = "tx-april-fair"

	potOfTheHouse = "reserve:acc-pots/pot-1"
	closedOldPot  = "reserve:acc-nu/pot-nu-old"
	potGoneAway   = "reserve:acc-gone/pot-9"
)

// marchBudget is the budget in force for 2026-03, its only version.
func marchBudget() authored.Document {
	return authored.Document{
		authoredtest.Group("rec", authoredtest.Line("Salário", 800_000), authoredtest.FlaggedLine("Renda extra", 50_000, "Não garantida")),
		authoredtest.Group("fix", authoredtest.Line("Aluguel", 250_000), authoredtest.Line("Internet", 12_000)),
		authoredtest.Group("var", authoredtest.Line("Mercado", 90_000), authoredtest.Line("Delivery", 20_000)),
		authoredtest.Group("prov", authoredtest.Line("IPTU", 15_000)),
		authoredtest.Group("res", authoredtest.Line("Viagem", 40_000)),
	}
}

func addBudgetExampleRows(f *ledgertest.Fixture) {
	card := func(id string, at time.Time, description string, amount float64, kind, operation string) {
		f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: accountCard, Date: at, Description: description,
			Amount: amount, Currency: "BRL", Type: kind,
			Raw: map[string]any{"operationType": operation, "creditCardMetadata": map[string]any{"cardNumber": "1111222233331234"}}})
	}
	card(txMarketFirst, utc(2026, time.March, 4, 15, 0), "Supermercado A", 250, "DEBIT", "PAGAMENTO")
	card(txMarketSecond, utc(2026, time.March, 11, 15, 0), "Supermercado A", 130, "DEBIT", "PAGAMENTO")
	card(txFair, utc(2026, time.March, 7, 15, 0), "Feira B", 70, "DEBIT", "PAGAMENTO")
	card(txDelivery, utc(2026, time.March, 9, 15, 0), "iFood", 230, "DEBIT", "PAGAMENTO")
	card(txWholesale, utc(2026, time.March, 13, 15, 0), "Atacado C", 300, "DEBIT", "PAGAMENTO")
	card(txMarketRefund, utc(2026, time.March, 15, 15, 0), "Supermercado A", 50, "CREDIT", "ESTORNO")
	card(txOtherPerson, utc(2026, time.March, 16, 15, 0), "Supermercado A", 999, "DEBIT", "PAGAMENTO")
	card(txUnreviewed, utc(2026, time.March, 17, 15, 0), "Feira B", 45, "DEBIT", "PAGAMENTO")
	card(txAprilFair, utc(2026, time.April, 2, 15, 0), "Feira B", 40, "DEBIT", "PAGAMENTO")
	f.AddTransaction(ledgertest.Transaction{ID: txWages, AccountID: accountItau, Date: utc(2026, time.March, 6, 12, 0),
		Description: "PIX RECEBIDO EMPRESA B", Amount: 8000, Currency: "BRL", Type: "CREDIT",
		Raw: map[string]any{"operationType": "PIX", "paymentData": payment("PIX", party("Empresa B Ltda", "99887766000155"), nil)}})
}

// budgetExample is the example's two stores and a router over both.
type budgetExample struct {
	t       *testing.T
	scratch *authoredtest.Scratch
	ledger  *ledgertest.Fixture
	lines   map[string]authored.LineID
	*readEnv
}

func newBudgetExample(t *testing.T) *budgetExample {
	t.Helper()
	scratch := authoredtest.Open(t, fixtureClock)
	fixture := newFullLedger(t)
	addBudgetExampleRows(fixture)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-03", marchBudget())
	example := &budgetExample{t: t, scratch: scratch, ledger: fixture, lines: linesOfMonthByName(t, scratch.Database, "2026-03")}
	authoredtest.ToggleLink(t, scratch.Database, fixtureNow, example.lines["IPTU"], potOfTheHouse)
	authorOver(t, scratch, viewOf(t, fixture.Path), example.lines, firstDecisions, "scratch.budgetExample", example.classify)
	example.readEnv = newReadEnvOver(t, fixture, scratch.Database)
	return example
}

func linesOfMonthByName(t *testing.T, database *authored.Database, month brday.Month) map[string]authored.LineID {
	t.Helper()
	view := database.View()
	defer view.Close()
	budget, err := view.BudgetOf(t.Context(), month)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]authored.LineID{}
	for _, line := range budget.Lines {
		ids[line.Name] = line.ID
	}
	return ids
}

func (example *budgetExample) classify(a authoring) error {
	onLine := func(id, line string) func() error {
		return func() error {
			return a.state(id, authored.TransactionState{Line: a.line(line), ClassifiedBy: authored.AuthorPerson})
		}
	}
	split := func() error {
		key, err := a.key(txWholesale)
		if err != nil {
			return err
		}
		return authoredtest.WriteSplit(a.ctx, a.writes.Recorder, key, []authored.SplitPart{
			{Position: 0, Line: a.line("Mercado"), Cents: 15_000}, {Position: 1, Line: a.line("Mercado"), Cents: 5_000},
			{Position: 2, Line: a.line("Delivery"), Cents: 6_000}, {Position: 3, Cents: 4_000},
		})
	}
	return each(
		onLine(txMarketFirst, "Mercado"), onLine(txMarketSecond, "Mercado"), onLine(txFair, "Mercado"),
		onLine(txDelivery, "Delivery"), onLine(txWages, "Salário"), onLine(txAprilFair, "Mercado"), split,
		func() error {
			return a.state(txMarketRefund, authored.TransactionState{Refund: authoredtest.Some(true), RefundLine: a.line("Mercado")})
		},
		func() error {
			return a.state(txOtherPerson, authored.TransactionState{
				Line: a.line("Mercado"), ClassifiedBy: authored.AuthorPerson, Exclusion: authoredtest.Some("Compra de outra pessoa"),
			})
		},
		func() error {
			return a.state(txUnreviewed, authored.TransactionState{PendingReason: "Confiança abaixo do limite", PendingLine: a.line("Mercado")})
		},
	)
}

// viewAnswer is a budget.view answer with every amount and cell kept as the
// text the server wrote.
type viewAnswer struct {
	Month          string          `json:"month"`
	Closed         bool            `json:"closed"`
	Income         json.Number     `json:"income"`
	Allocated      json.Number     `json:"allocated"`
	InvestmentRate json.RawMessage `json:"investmentRate"`
	Groups         []struct {
		Key           string          `json:"key"`
		Subtotal      json.Number     `json:"subtotal"`
		SubtotalSpent json.RawMessage `json:"subtotalSpent"`
		Over          bool            `json:"over"`
		Lines         []lineAnswer    `json:"lines"`
	} `json:"groups"`
}

type lineAnswer struct {
	Name  string            `json:"name"`
	Cells []json.RawMessage `json:"cells"`
	Marks json.RawMessage   `json:"marks"`
	Over  bool              `json:"over"`
	Bar   json.RawMessage   `json:"bar"`
}

func (line lineAnswer) cells() string {
	texts := make([]string, 0, len(line.Cells))
	for _, cell := range line.Cells {
		texts = append(texts, string(cell))
	}
	return strings.Join(texts, ",")
}

func (example *budgetExample) view(arguments string) viewAnswer {
	example.t.Helper()
	status, body := example.answer("budget.view", arguments)
	if status != http.StatusOK {
		example.t.Fatalf("budget.view %s answered %d %s", arguments, status, body)
	}
	if problem := conformanceProblem("budget.view", body); problem != "" {
		example.t.Errorf("budget.view %s does not conform: %s", arguments, problem)
	}
	var answer viewAnswer
	decodeKeepingNumbers(example.t, body, &answer)
	return answer
}

func (answer viewAnswer) line(t *testing.T, key, name string) lineAnswer {
	t.Helper()
	for _, group := range answer.Groups {
		for _, line := range group.Lines {
			if group.Key == key && line.Name == name {
				return line
			}
		}
	}
	t.Fatalf("no line %s in %s", name, key)
	return lineAnswer{}
}

func (answer viewAnswer) group(key string) string {
	for _, group := range answer.Groups {
		if group.Key == key {
			return string(group.Subtotal) + " " + string(group.SubtotalSpent) + " " + map[bool]string{true: "over", false: "within"}[group.Over]
		}
	}
	return "missing"
}

func differenceJSON(amount string) string { return `{"type":"difference","amount":` + amount + `}` }

func expectLine(t *testing.T, label string, line lineAnswer, cells, marks, bar string, over bool) {
	t.Helper()
	if line.cells() != cells || string(line.Marks) != marks || string(line.Bar) != bar || line.Over != over {
		t.Errorf("%s: cells %s marks %s bar %s over %v\n  want cells %s marks %s bar %s over %v",
			label, line.cells(), line.Marks, line.Bar, line.Over, cells, marks, bar, over)
	}
}

const (
	overspent  = `[{"text":"Estourou","tone":"bad"}]`
	overIncome = `[{"text":"Budget maior que a receita","tone":"bad"}]`
)

func TestTheBudgetViewWeighsTheMonthsBudgetAgainstItsTransactions(t *testing.T) {
	example := newBudgetExample(t)
	view := example.view(`["2026-03"]`)
	if view.Income != "8500.00" || view.Allocated != "8500.00" || string(view.InvestmentRate) != "50" || view.Closed {
		t.Errorf("income %s, allocated %s, rate %s, closed %v", view.Income, view.Allocated, view.InvestmentRate, view.Closed)
	}
	expectLine(t, "the investment", view.line(t, "inv", "Investimento"),
		moneyCellJSON("4230.00")+","+moneyCellJSON("0.00")+`,{"type":"percent","value":50}`, `[]`, "null", false)
	expectLine(t, "Mercado", view.line(t, "var", "Mercado"),
		moneyCellJSON("900.00")+","+moneyCellJSON("600.00")+","+differenceJSON("300.00"), `[]`, "67", false)
	expectLine(t, "Delivery", view.line(t, "var", "Delivery"),
		moneyCellJSON("200.00")+","+moneyCellJSON("290.00")+","+differenceJSON("-90.00"), overspent, "100", true)
	if got := view.group("var"); got != "1100.00 890.00 within" {
		t.Errorf("the variable group is %s, want 1100.00 890.00 within", got)
	}
	expectLine(t, "Salário", view.line(t, "rec", "Salário"),
		moneyCellJSON("8000.00")+","+moneyCellJSON("8000.00")+`,{"type":"none"}`, `[]`, "null", false)
	expectLine(t, "Renda extra", view.line(t, "rec", "Renda extra"),
		moneyCellJSON("500.00")+","+moneyCellJSON("0.00")+`,{"type":"none"}`, `[{"text":"Não garantida","tone":"neutral"}]`, "null", false)
	expectLine(t, "IPTU, linked to the pot", view.line(t, "prov", "IPTU"),
		moneyCellJSON("150.00")+","+moneyCellJSON(potBalanceOf(t, example, potOfTheHouse))+`,{"type":"next-charge","charge":null}`, `[]`, "null", false)
	expectLine(t, "Viagem, linked to nothing", view.line(t, "res", "Viagem"),
		moneyCellJSON("400.00")+`,{"type":"none"},`+moneyCellJSON("0.00"), `[]`, "null", false)
	if got := view.group("tar"); got != "25.00 null within" {
		t.Errorf("the fees group is %s, want 25.00", got)
	}
}

// potBalanceOf is the balance savingsPots.detail answers for a pot.
func potBalanceOf(t *testing.T, example *budgetExample, pot string) string {
	t.Helper()
	var detail struct {
		Balance json.Number `json:"balance"`
	}
	status, body := example.answer("savingsPots.detail", `["`+pot+`"]`)
	if status != http.StatusOK {
		t.Fatalf("savingsPots.detail answered %d", status)
	}
	decodeKeepingNumbers(t, body, &detail)
	if detail.Balance == "" || detail.Balance == "0.00" {
		t.Fatalf("the pot's balance is %q, which cannot tell a balance from none", detail.Balance)
	}
	return string(detail.Balance)
}

// draftOf is the month's document as the interface would send it back, after
// change.
func (example *budgetExample) draftOf(month string, changes ...func([]map[string]any)) string {
	example.t.Helper()
	status, body := example.answer("budget.document", `["`+month+`"]`)
	if status != http.StatusOK {
		example.t.Fatalf("budget.document answered %d", status)
	}
	var groups []map[string]any
	decodeKeepingNumbers(example.t, body, &groups)
	for _, change := range changes {
		change(groups)
	}
	return string(mustMarshal(example.t, groups))
}

func withNewLine(key, name, amount string) func([]map[string]any) {
	return func(groups []map[string]any) {
		for _, group := range groups {
			if group["key"] == key {
				group["lines"] = append(group["lines"].([]any), map[string]any{"name": name, "amount": json.Number(amount)})
			}
		}
	}
}

func TestADraftIsWeighedByLineIdAndNothingOfItIsStored(t *testing.T) {
	example := newBudgetExample(t)
	before := example.scratch.Everything(t)
	draft := example.draftOf("2026-03", withRename("Mercado", "Feira"), withAmount("Feira", "500.00"), withNewLine("var", "Pets", "100.00"))
	view := example.view(`["2026-03",` + draft + `]`)
	expectLine(t, "Mercado renamed Feira under its id", view.line(t, "var", "Feira"),
		moneyCellJSON("500.00")+","+moneyCellJSON("600.00")+","+differenceJSON("-100.00"), overspent, "100", true)
	expectLine(t, "a new line", view.line(t, "var", "Pets"),
		moneyCellJSON("100.00")+","+moneyCellJSON("0.00")+","+differenceJSON("100.00"), `[]`, "0", false)
	if after := example.scratch.Everything(t); after != before {
		t.Errorf("weighing a draft changed the database:\n%s", differences(before, after))
	}

	poorer := example.view(`["2026-03",` + example.draftOf("2026-03", withAmount("Salário", "1000.00")) + `]`)
	if string(poorer.InvestmentRate) != "-185" || poorer.group("inv") != "-2770.00 null over" {
		t.Errorf("a draft past its income: rate %s, investment group %s", poorer.InvestmentRate, poorer.group("inv"))
	}
	expectLine(t, "the investment of a draft past its income", poorer.line(t, "inv", "Investimento"),
		moneyCellJSON("-2770.00")+","+moneyCellJSON("0.00")+`,{"type":"percent","value":-185}`, overIncome, "null", true)
}

func TestAnEmptyDraftHasNoLinesAndANullDraftIsNoDraft(t *testing.T) {
	example := newBudgetExample(t)
	expectAnswer(t, example.readEnv, "budget.view", `["2026-03",[]]`, emptyBudgetViewOf("2026-03", "25.00"))
	_, without := example.answer("budget.view", `["2026-03"]`)
	_, null := example.answer("budget.view", `["2026-03",null]`)
	if !bytes.Equal(without, null) || !strings.Contains(string(without), `"name":"Mercado"`) {
		t.Errorf("a null draft answered\n%s\nand no draft\n%s", null, without)
	}
}

// A closed month is taken as wholly made, with nothing marked.
func TestAClosedMonthIsClosedAndItsInvestmentIsWhollyMade(t *testing.T) {
	example := newBudgetExample(t)
	authoredtest.CloseMonth(t, example.scratch.Database, fixtureNow, "2026-03", "2026-04-02")
	view := example.view(`["2026-03"]`)
	if !view.Closed {
		t.Error("a month in closed_months answers closed false")
	}
	expectLine(t, "the investment of a closed month", view.line(t, "inv", "Investimento"),
		moneyCellJSON("4230.00")+","+moneyCellJSON("4230.00")+`,{"type":"percent","value":50}`, `[]`, "null", false)
	if example.view(`["2026-02"]`).Closed {
		t.Error("a month not in closed_months answers closed true")
	}
}

func TestALinkHoldsThroughARenameAndCountsOnlyWhileItsPotIsListedAndOpen(t *testing.T) {
	example := newBudgetExample(t)
	balance := potBalanceOf(t, example, potOfTheHouse)
	renamed := example.draftOf("2026-03", withRename("IPTU", "IPTU e taxas"))
	saved, err := draftArgument(json.RawMessage(renamed))
	if err != nil {
		t.Fatal(err)
	}
	authoredtest.SaveBudget(t, example.scratch.Database, fixtureNow, "2026-04", saved)
	pot := func(month, name string) string {
		return example.view(`["`+month+`"]`).line(t, "prov", name).cells()
	}
	if got, want := pot("2026-04", "IPTU e taxas"), moneyCellJSON("150.00")+","+moneyCellJSON(balance)+`,{"type":"next-charge","charge":null}`; got != want {
		t.Errorf("the renamed line: %s, want %s", got, want)
	}
	for _, elsewhere := range []string{closedOldPot, potGoneAway} {
		authoredtest.ToggleLink(t, example.scratch.Database, fixtureNow, example.lines["IPTU"], elsewhere)
		if got, want := pot("2026-03", "IPTU"), moneyCellJSON("150.00")+`,{"type":"none"},{"type":"next-charge","charge":null}`; got != want {
			t.Errorf("linked to %s: %s, want %s", elsewhere, got, want)
		}
	}
}
