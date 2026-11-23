package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/classrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

func init() {
	readCases = append(readCases,
		readCase{"review.closing", `["2026-03"]`},
		readCase{"review.closing", `["2025-12"]`},
	)
	// Over the third state, beside its case of 2026-03: 2026-02, open with a
	// step ticked and nothing waiting; 2026-01, closed; and 2026-04 with steps
	// and a transfer of its own marked, the boleto of March still waiting and
	// its version of the budget in force.
	authoredReadCases = append(authoredReadCases,
		authoredRead("review.closing", `["2026-02"]`),
		authoredRead("review.closing", `["2026-01"]`),
		authoredReadOver("review.closing", `["2026-04"]`, closingMarksOverTheThirdState),
	)
}

const emptyClosingSteps = `[{"id":"pending","state":"done","markable":false},` +
	`{"id":"planned-actual","state":"todo","markable":true},` +
	`{"id":"leftover","state":"todo","markable":true},` +
	`{"id":"contributions","state":"todo","markable":true},` +
	`{"id":"card-bill","state":"todo","markable":true},` +
	`{"id":"subscriptions","state":"todo","markable":true},` +
	`{"id":"report","state":"todo","markable":true},` +
	`{"id":"close","state":"todo","markable":false}]`

const freshPlannedActualJSON = `[` +
	`{"name":"Receita","planned":0.00,"actual":0.00,"difference":0.00,"contributions":false,"lines":[]},` +
	`{"name":"Fixos e essenciais","planned":0.00,"actual":0.00,"difference":0.00,"contributions":false,"lines":[]},` +
	`{"name":"Variáveis","planned":0.00,"actual":0.00,"difference":0.00,"contributions":false,"lines":[]},` +
	`{"name":"Provisões","planned":0.00,"actual":0.00,"difference":0.00,"contributions":true,"lines":[]},` +
	`{"name":"Reservas de desejo","planned":0.00,"actual":0.00,"difference":0.00,"contributions":true,"lines":[]}]`

func closingOf(month, cardBill string) string {
	return `{"month":"` + month + `","closed":false,"closedOn":null,"closeChangeId":null,"steps":` + emptyClosingSteps +
		`,"pendingCount":0,"plannedActual":` + freshPlannedActualJSON + `,"leftover":0.00,"contributions":[],` +
		`"cardBill":` + cardBill + `,"subscriptionsToReview":[],"report":[],"closedMonths":[]}`
}

// The full ledger's card is not one the bank's closing table covers, so the
// month's purchases are charged on no bill the ledger can name and the bill is
// worked out as empty; the empty ledger holds no card at all.
func TestTheClosingAnswersAsAFreshInstallOverBothLedgers(t *testing.T) {
	expectAnswer(t, newReadEnv(t, newFullLedger(t)), "review.closing", `["2026-03"]`,
		closingOf("2026-03", `{"amount":0.00,"cardLabel":"Platinum Visa · final 1234","purchases":[]}`))
	expectAnswer(t, newReadEnv(t, newEmptyLedger(t)), "review.closing", `["2026-03"]`, closingOf("2026-03", `null`))
}

func TestTheClosingNamesTheMonthItWasAskedFor(t *testing.T) {
	expectAnswer(t, newReadEnv(t, newEmptyLedger(t)), "review.closing", `["2025-12"]`, closingOf("2025-12", `null`))
}

func manifestClosingSteps(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../contract/contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Definitions struct {
			ClosingStepID struct {
				Enum []string `json:"enum"`
			} `json:"ClosingStepId"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Definitions.ClosingStepID.Enum) == 0 {
		t.Fatal("the manifest lists no closing step")
	}
	return manifest.Definitions.ClosingStepID.Enum
}

// A step the contract gains must be one the closing lists, or the screen has a
// step that can never be reached.
func TestTheClosingListsEveryStepOfTheContractOnceAndInOrder(t *testing.T) {
	env := newReadEnv(t, newEmptyLedger(t))
	status, body := env.answer("review.closing", `["2026-03"]`)
	if status != http.StatusOK {
		t.Fatalf("answered %d", status)
	}
	var view struct {
		Steps []struct {
			ID string `json:"id"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatal(err)
	}
	listed := make([]string, 0, len(view.Steps))
	for _, step := range view.Steps {
		listed = append(listed, step.ID)
	}
	if want := manifestClosingSteps(t); !reflect.DeepEqual(listed, want) {
		t.Errorf("the closing lists %v, the contract has %v", listed, want)
	}
}

func TestTheClosingStepsOfAMonthNobodyHasWorkedOnAreOnlyDoneWhereNothingWaits(t *testing.T) {
	for _, step := range closingSteps(false, 0, nil) {
		wantDone := step.ID == stepPending
		if (step.State == "done") != wantDone {
			t.Errorf("step %s is %s", step.ID, step.State)
		}
		if wantMarkable := step.ID != stepPending && step.ID != stepClose; step.Markable != wantMarkable {
			t.Errorf("step %s markable = %t", step.ID, step.Markable)
		}
	}
}

// c6BillLedger has purchases that land on the bill after the one closing in
// March, so the card's bill is a figure and not a zero.
func c6BillLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := c6Ledger(t)
	addCardPurchase(f, "tx-on-closing", utc(2026, time.March, 4, 15, 0), 50.25, "")
	addCardPurchase(f, "tx-later", utc(2026, time.March, 12, 15, 0), 20.5, "")
	addCardPurchase(f, "tx-latest", utc(2026, time.March, 19, 15, 0), 9.99, "")
	return f
}

func cardBillFigures(t *testing.T, env *readEnv) (card, closing map[string]any) {
	t.Helper()
	overview := answerOf(t, env, "card.overview", `[]`)
	view := answerOf(t, env, "review.closing", `["2026-03"]`)
	bill, ok := view["cardBill"].(map[string]any)
	if !ok {
		t.Fatalf("the closing has no card bill: %v", view["cardBill"])
	}
	return overview["payAhead"].(map[string]any), bill
}

func TestTheClosingsCardBillIsTheCardPagesPayAhead(t *testing.T) {
	env := envAt(t, c6BillLedger(t), 2026, time.March, 20)
	payAhead, bill := cardBillFigures(t, env)

	if payAhead["amount"].(float64) != 80.74 {
		t.Fatalf("the card page's pay-ahead is %v, the control expects 80.74", payAhead["amount"])
	}
	if bill["amount"] != payAhead["amount"] {
		t.Errorf("the closing's bill is %v, the card page's is %v", bill["amount"], payAhead["amount"])
	}
	if bill["cardLabel"] != "C6 Carbon · final 8888" {
		t.Errorf("card label = %v", bill["cardLabel"])
	}
	cardPurchases, billPurchases := payAhead["purchases"].([]any), bill["purchases"].([]any)
	if len(billPurchases) != 3 || len(cardPurchases) != 3 {
		t.Fatalf("purchases: closing %d, card page %d, want 3", len(billPurchases), len(cardPurchases))
	}
	for index, purchase := range cardPurchases {
		onCard, onClosing := purchase.(map[string]any), billPurchases[index].(map[string]any)
		if onClosing["name"] != onCard["name"] || onClosing["day"] != onCard["day"] || onClosing["amount"].(float64) != -onCard["amount"].(float64) {
			t.Errorf("purchase %d: closing %v, card page %v", index, onClosing, onCard)
		}
	}
}

func TestTheClosingHasNoCardBillWhenTheLedgerHoldsNoCard(t *testing.T) {
	f := newEmptyLedger(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-only-bank", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddAccount(ledgertest.Account{ID: "acc-only-bank", ItemID: "item-only-bank", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Corrente", Currency: "BRL", Balance: 10, TransferNumber: "341/0001/10000-1"})
	for _, ledgerOf := range []*ledgertest.Fixture{f, newEmptyLedger(t)} {
		view := answerOf(t, newReadEnv(t, ledgerOf), "review.closing", `["2026-03"]`)
		if card, present := view["cardBill"]; !present || card != nil {
			t.Errorf("cardBill = %v, want null", card)
		}
	}
}

func TestTheClosingRefusesAMonthThatIsNotOne(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, body := range []string{`["2026-13"]`, `["March"]`, `[3]`, `[null]`, `[]`, `["2026-03","2026-04"]`} {
		response := env.call("/api/review/closing", body, env.cookie)
		expectError(t, "review.closing "+body, response, http.StatusBadRequest, "bad_request")
	}
}

func TestTheClosingHasNoCardBillWhenTheNextBillAmountIsUnknown(t *testing.T) {
	env := envAt(t, noTableLedger(t), 2026, time.March, 20)
	view := answerOf(t, env, "review.closing", `["2026-03"]`)
	if bill, present := view["cardBill"]; !present || bill != nil {
		t.Errorf("cardBill = %v for a card whose next-bill amount is unknown, want null and never a bill of zero", bill)
	}
}

func answerBody(t *testing.T, env *readEnv, method, arguments string) string {
	t.Helper()
	status, body := env.answer(method, arguments)
	if status != http.StatusOK {
		t.Fatalf("%s %s answered %d %s", method, arguments, status, body)
	}
	return string(body)
}

// c6FebruaryLedger is c6BillLedger with four purchases of February around the
// closing of 2026-02-04: one before the two days that can slip, one of the
// closed bill, one that slipped and one after.
func c6FebruaryLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := c6BillLedger(t)
	addCardPurchase(f, "tx-feb-early", utc(2026, time.February, 1, 15, 0), 30, "")
	addCardPurchase(f, "tx-feb-closed", utc(2026, time.February, 3, 15, 0), 12, "bill-jan")
	addCardPurchase(f, "tx-feb-slipped", utc(2026, time.February, 3, 16, 0), 7, "")
	addCardPurchase(f, "tx-feb-later", utc(2026, time.February, 15, 15, 0), 40, "")
	return f
}

type billSeen struct {
	Amount    float64 `json:"amount"`
	CardLabel string  `json:"cardLabel"`
	Purchases []struct {
		Name   string  `json:"name"`
		Day    string  `json:"day"`
		Amount float64 `json:"amount"`
	} `json:"purchases"`
}

func closingBillOf(t *testing.T, env *readEnv, month string) *billSeen {
	t.Helper()
	var view struct {
		CardBill *billSeen `json:"cardBill"`
	}
	if err := json.Unmarshal([]byte(answerBody(t, env, "review.closing", `["`+month+`"]`)), &view); err != nil {
		t.Fatal(err)
	}
	return view.CardBill
}

func TestTheClosingsCardBillIsTheBillOfTheMonthAsked(t *testing.T) {
	env := envAt(t, c6FebruaryLedger(t), 2026, time.March, 20)
	february := closingBillOf(t, env, "2026-02")
	if february == nil || february.Amount != 47 || len(february.Purchases) != 2 {
		t.Fatalf("February's bill is %+v, want 47.00 over two purchases", february)
	}
	if first, second := february.Purchases[0], february.Purchases[1]; first.Day != "2026-02-15" || first.Amount != 40 ||
		second.Day != "2026-02-03" || second.Amount != 7 {
		t.Errorf("February's purchases are %+v", february.Purchases)
	}
	if march := closingBillOf(t, env, "2026-03"); march == nil || march.Amount != 80.74 || len(march.Purchases) != 3 {
		t.Errorf("March's bill is %+v, want 80.74 over three purchases", march)
	}
	if april := closingBillOf(t, env, "2026-04"); april == nil || april.Amount != 0 || len(april.Purchases) != 0 {
		t.Errorf("April's bill is %+v, want 0.00 with no purchase before its closing has come", april)
	}
	if january := closingBillOf(t, env, "2026-01"); january != nil {
		t.Errorf("January, in which no closing of the card falls, has the bill %+v", january)
	}
}

func closingMarksOverTheThirdState(t *testing.T, scratch *authoredtest.Scratch, _ *ledgertest.Fixture) {
	t.Helper()
	tickSteps(t, scratch, "2026-04", "planned-actual", "card-bill")
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: markTransferDoneMethod, At: fixtureNow}
	err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		_, err := recorder.MarkTransferDone(t.Context(), "2026-04", budgetrules.InvestmentTransferKey, "2026-03-20")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func stepStates(steps []closingStepWire) []string {
	states := make([]string, 0, len(steps))
	for _, step := range steps {
		marked := ""
		if step.Markable {
			marked = "*"
		}
		states = append(states, step.State+marked)
	}
	return states
}

// Steps two to seven follow the ticks and can be ticked; the first follows
// the count and the last the close; a closed month is done throughout.
func TestTheClosingStepsFollowTheCountTheTicksAndTheClose(t *testing.T) {
	cases := []struct {
		label   string
		closed  bool
		pending int
		ticked  map[string]bool
		want    []string
	}{
		{"open, two waiting, two ticked", false, 2, map[string]bool{"leftover": true, "report": true},
			[]string{"pending", "todo*", "done*", "todo*", "todo*", "todo*", "done*", "todo"}},
		{"open, nothing waiting, every step ticked", false, 0,
			map[string]bool{"planned-actual": true, "leftover": true, "contributions": true, "card-bill": true, "subscriptions": true, "report": true},
			[]string{"done", "done*", "done*", "done*", "done*", "done*", "done*", "todo"}},
		{"closed, with a review counted and nothing ticked", true, 1, nil,
			[]string{"done", "done", "done", "done", "done", "done", "done", "done"}},
	}
	for _, c := range cases {
		if got := stepStates(closingSteps(c.closed, c.pending, c.ticked)); !slices.Equal(got, c.want) {
			t.Errorf("%s: steps %v, want %v", c.label, got, c.want)
		}
	}
}

// The plan of the arithmetic's table: IPTU linked to an open pot, Revisão on
// no pot, a provision of zero, and Viagem linked to a pot that closed, whose
// transfer is then its own line's.
const (
	tableOpenPot   = "reserve:acc-table/pot-open"
	tableClosedPot = "reserve:acc-table/pot-closed"
)

func tablePlan() budgetrules.Plan {
	line := func(id budgetrules.LineID, name string, cents money.Cents) budgetrules.PlannedLine {
		return budgetrules.PlannedLine{ID: budgetrules.OnLine(id), Name: name, Amount: cents}
	}
	return budgetrules.Plan{
		{Key: budgetrules.GroupIncome, Lines: []budgetrules.PlannedLine{line(2, "Salário", 800_000)}},
		{Key: budgetrules.GroupFixed, Lines: []budgetrules.PlannedLine{line(3, "Aluguel", 250_000)}},
		{Key: budgetrules.GroupVariable, Lines: []budgetrules.PlannedLine{line(4, "Mercado", 90_000), line(5, "Lazer", 30_000)}},
		{Key: budgetrules.GroupProvisions, Lines: []budgetrules.PlannedLine{line(6, "IPTU", 15_000), line(7, "Revisão do carro", 20_000), line(8, "Material", 0)}},
		{Key: budgetrules.GroupReserves, Lines: []budgetrules.PlannedLine{line(9, "Viagem", 40_000)}},
	}
}

func tableTransfers(marked ...string) []budgetrules.MarkedTransfer {
	closedName, openName := "Antiga", "Caixinha"
	transfers := budgetrules.Transfers(budgetrules.PaydayInputs{
		Plan: tablePlan(), Links: map[budgetrules.LineID]string{6: tableOpenPot, 9: tableClosedPot},
		Pots:       []budgetrules.TransferPot{{ID: tableOpenPot, Name: &openName}, {ID: tableClosedPot, Name: &closedName, Closed: true}},
		UnnamedPot: unnamedPot, InvestmentName: budgetGroupInvestment,
	})
	marks := budgetrules.TransferMarks{}
	for _, key := range marked {
		marks[key] = "2026-03-18"
	}
	return budgetrules.Marked(transfers, marks)
}

var tableFigures = map[budgetrules.LineID]money.Cents{2: 750_000, 3: 250_000, 4: 95_000, 5: 12_000, 7: 9_999}

// actualsOf is each line's actual and difference, and each group's, as
// "name actual difference" in cents.
func actualsOf(groups []plannedActualGroupWire) map[string][2]Reais {
	actuals := map[string][2]Reais{}
	for _, group := range groups {
		actuals[group.Name] = [2]Reais{group.Actual, group.Difference}
		for _, line := range group.Lines {
			actuals[line.Name] = [2]Reais{line.Actual, line.Difference}
		}
	}
	return actuals
}

func TestPlannedAgainstActualSignsTheIncomeTheOtherWayAndSetsAsideWhatAMarkedTransferCarries(t *testing.T) {
	cases := []struct {
		label  string
		marked []string
		closed bool
		want   map[string][2]Reais
	}{
		{"the open pot's transfer marked", []string{"pot:" + tableOpenPot}, false, map[string][2]Reais{
			"IPTU": {15_000, 0}, "Revisão do carro": {0, 20_000}, "Material": {0, 0}, "Viagem": {0, 40_000},
			"Provisões": {15_000, 20_000}, "Reservas de desejo": {0, 40_000},
		}},
		{"nothing marked", nil, false, map[string][2]Reais{
			"IPTU": {0, 15_000}, "Revisão do carro": {0, 20_000}, "Material": {0, 0}, "Provisões": {0, 35_000},
		}},
		{"a mark of the closed pot counts for nothing, the line's own carries it", []string{"pot:" + tableClosedPot, "line:9"}, false, map[string][2]Reais{
			"IPTU": {0, 15_000}, "Viagem": {40_000, 0}, "Reservas de desejo": {40_000, 0},
		}},
		{"closed, nothing marked", nil, true, map[string][2]Reais{
			"IPTU": {15_000, 0}, "Revisão do carro": {20_000, 0}, "Material": {0, 0}, "Viagem": {40_000, 0},
			"Provisões": {35_000, 0}, "Reservas de desejo": {40_000, 0},
		}},
	}
	spent := map[string][2]Reais{
		"Salário": {750_000, -50_000}, "Receita": {750_000, -50_000}, "Aluguel": {250_000, 0}, "Fixos e essenciais": {250_000, 0},
		"Mercado": {95_000, -5_000}, "Lazer": {12_000, 18_000}, "Variáveis": {107_000, 13_000},
	}
	for _, c := range cases {
		got := actualsOf(plannedActualOf(tablePlan(), tableFigures, tableTransfers(c.marked...), c.closed))
		for name, want := range c.want {
			if got[name] != want {
				t.Errorf("%s: %s actual and difference %v, want %v", c.label, name, got[name], want)
			}
		}
		for name, want := range spent {
			if got[name] != want {
				t.Errorf("%s: %s actual and difference %v, want %v", c.label, name, got[name], want)
			}
		}
	}
}

func TestPlannedAgainstActualListsTheFiveGroupsInOrderWithTheirLinesAndContributionsOnTheLastTwo(t *testing.T) {
	groups := plannedActualOf(tablePlan(), tableFigures, tableTransfers(), false)
	var shape []string
	for _, group := range groups {
		entry := group.Name
		if group.Contributions {
			entry += " (aportes)"
		}
		for _, line := range group.Lines {
			entry += " | " + line.Name
		}
		shape = append(shape, entry)
	}
	want := []string{"Receita | Salário", "Fixos e essenciais | Aluguel", "Variáveis | Mercado | Lazer",
		"Provisões (aportes) | IPTU | Revisão do carro | Material", "Reservas de desejo (aportes) | Viagem"}
	if !slices.Equal(shape, want) {
		t.Errorf("groups %v, want %v", shape, want)
	}
	if groups[3].Planned != 35_000 || groups[2].Planned != 120_000 {
		t.Errorf("planned of provisions %v and of variables %v", groups[3].Planned, groups[2].Planned)
	}
}

func TestTheLeftoverIsTheVariableCapLessWhatTheVariableLinesSpent(t *testing.T) {
	for _, c := range []struct {
		mercado, lazer money.Cents
		want           money.Cents
	}{{95_000, 12_000, 13_000}, {90_000, 30_000, 0}, {150_000, 12_000, -42_000}} {
		figures := map[budgetrules.LineID]money.Cents{2: 750_000, 3: 999_999, 4: c.mercado, 5: c.lazer}
		if got := leftoverOf(tablePlan(), figures); got != c.want {
			t.Errorf("Mercado %d and Lazer %d leave %d, want %d", c.mercado, c.lazer, got, c.want)
		}
	}
}

func TestEachContributionIsDoneWhenMarkedOrTheMonthIsClosedAndNeverOnTarget(t *testing.T) {
	states := func(rows []closingContributionWire) []string {
		var listed []string
		for _, row := range rows {
			listed = append(listed, row.Name+"="+row.State)
		}
		return listed
	}
	open := contributionsOf(tableTransfers("pot:"+tableOpenPot, "investment"), false)
	want := []string{"Caixinha=done", "Revisão do carro=not-found", "Viagem=not-found", "Investimento=done"}
	if got := states(open); !slices.Equal(got, want) {
		t.Errorf("open month: %v, want %v", got, want)
	}
	closed := contributionsOf(tableTransfers(), true)
	want = []string{"Caixinha=done", "Revisão do carro=done", "Viagem=done", "Investimento=done"}
	if got := states(closed); !slices.Equal(got, want) {
		t.Errorf("closed month: %v, want %v", got, want)
	}
}

// The closing's example: a bank account with a pot named "Caixinha da casa",
// a month of salary, rent and purchases, and three purchases left to review
// across the turn of February. Everything is invented.
const (
	closingBank          = "acc-closing"
	closingPot           = "reserve:acc-closing/pot-casa"
	txClosingSalary      = "tx-closing-salary"
	txClosingRent        = "tx-closing-rent"
	txClosingMarket      = "tx-closing-market"
	txClosingMarketAgain = "tx-closing-market-again"
	txClosingCinema      = "tx-closing-cinema"
	txClosingFeb27       = "tx-closing-feb-27"
	txClosingFeb28       = "tx-closing-feb-28"
	txClosingMar05       = "tx-closing-mar-05"
)

func closingLedger(t testing.TB, marketAgain float64, leftOut ...string) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-closing", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddAccount(ledgertest.Account{ID: closingBank, ItemID: "item-closing", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Corrente", Currency: "BRL", Balance: 2000, TransferNumber: "323/0001/40000-4",
		Reserves: []ledgertest.Reserve{{ID: "pot-casa", Name: "Caixinha da casa", Amount: 450, CDIRatio: 1}}})
	row := func(id string, at time.Time, description string, amount float64, kind string, raw map[string]any) {
		if !slices.Contains(leftOut, id) {
			f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: closingBank, Date: at, Description: description,
				Amount: amount, Currency: "BRL", Type: kind, Raw: raw})
		}
	}
	debitCard := map[string]any{"operationType": "CARTAO"}
	row(txClosingSalary, utc(2026, time.March, 5, 12, 0), "PIX RECEBIDO EMPRESA EXEMPLO", 7500, "CREDIT",
		map[string]any{"operationType": "PIX", "paymentData": payment("PIX", party("Empresa Exemplo Ltda", "12345678000199"), nil)})
	row(txClosingRent, utc(2026, time.March, 6, 12, 0), "IMOBILIARIA EXEMPLO", -2500, "DEBIT",
		map[string]any{"operationType": "BOLETO", "paymentData": payment("BOLETO", nil, party("Imobiliária Exemplo", "55566677000188"))})
	row(txClosingMarket, utc(2026, time.March, 8, 15, 0), "SUPERMERCADO EXEMPLO", -600, "DEBIT", debitCard)
	row(txClosingMarketAgain, utc(2026, time.March, 15, 15, 0), "SUPERMERCADO EXEMPLO", -marketAgain, "DEBIT", debitCard)
	row(txClosingCinema, utc(2026, time.March, 12, 15, 0), "CINEMA EXEMPLO", -120, "DEBIT", debitCard)
	row(txClosingFeb27, utc(2026, time.February, 27, 15, 0), "PADARIA EXEMPLO", -30, "DEBIT", debitCard)
	row(txClosingFeb28, utc(2026, time.February, 28, 15, 0), "PADARIA EXEMPLO", -20, "DEBIT", debitCard)
	row(txClosingMar05, utc(2026, time.March, 5, 15, 0), "PADARIA EXEMPLO", -15, "DEBIT", debitCard)
	return f
}

func closingBudget() authored.Document {
	line := authoredtest.Line
	return authored.Document{
		authoredtest.Group("rec", line("Salário", 800_000)),
		authoredtest.Group("fix", line("Aluguel", 250_000)),
		authoredtest.Group("var", line("Mercado", 90_000), line("Lazer", 30_000)),
		authoredtest.Group("prov", line("IPTU", 15_000), line("Revisão do carro", 20_000)),
		authoredtest.Group("res", line("Viagem", 40_000)),
	}
}

var closingRowsOnLines = []struct{ id, line string }{
	{txClosingSalary, "Salário"}, {txClosingRent, "Aluguel"}, {txClosingMarket, "Mercado"},
	{txClosingMarketAgain, "Mercado"}, {txClosingCinema, "Lazer"},
}

// closingExample is the example over its own ledger with the clock at
// 2026-03-20: the budget from 2026-03, IPTU linked to the pot, the month's rows
// on their lines and the pot's transfer of March marked.
type closingExample struct {
	*settingsEnv
	ledger *ledgertest.Fixture
}

func newClosingExample(t *testing.T, marketAgain float64) *closingExample {
	t.Helper()
	fixture := closingLedger(t, marketAgain)
	example := &closingExample{settingsEnv: newSettingsEnv(t, fixture, fixtureNow), ledger: fixture}
	database := example.scratch.Database
	authoredtest.SaveBudget(t, database, fixtureNow, "2026-03", closingBudget())
	authoredtest.ToggleLink(t, database, fixtureNow, budgetLineID(t, database, "2026-03", "IPTU"), closingPot)
	for _, row := range closingRowsOnLines {
		settledOn(t, example.scratch, fixture.Path, "2026-03-16", "transactions.setLine", row.id, row.line)
	}
	example.change(markTransferDoneMethod, `["2026-03","pot:`+closingPot+`"]`)
	return example
}

// closingSeen is the part of a closing answer the tests read, decoded.
type closingSeen struct {
	Closed        bool    `json:"closed"`
	ClosedOn      *string `json:"closedOn"`
	CloseChangeID *string `json:"closeChangeId"`
	PendingCount  int     `json:"pendingCount"`
	PlannedActual []struct {
		Name   string  `json:"name"`
		Actual float64 `json:"actual"`
		Lines  []struct {
			Name    string  `json:"name"`
			Planned float64 `json:"planned"`
			Actual  float64 `json:"actual"`
		} `json:"lines"`
	} `json:"plannedActual"`
	Steps []struct {
		ID       string `json:"id"`
		State    string `json:"state"`
		Markable bool   `json:"markable"`
	} `json:"steps"`
	Contributions []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"contributions"`
	SubscriptionsToReview []struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Amount   float64 `json:"amount"`
		Cycle    string  `json:"cycle"`
		ReviewOn string  `json:"reviewOn"`
	} `json:"subscriptionsToReview"`
}

func closingIn(t *testing.T, env *readEnv, month string) closingSeen {
	t.Helper()
	status, body := env.answer("review.closing", `["`+month+`"]`)
	if status != http.StatusOK {
		t.Fatalf("review.closing %s answered %d %s", month, status, body)
	}
	if problem := conformanceProblem("review.closing", body); problem != "" {
		t.Fatalf("review.closing %s does not conform: %s", month, problem)
	}
	var seen closingSeen
	if err := json.Unmarshal(body, &seen); err != nil {
		t.Fatal(err)
	}
	return seen
}

func (seen closingSeen) stepState(id string) string {
	for _, step := range seen.Steps {
		if step.ID == id {
			return step.State
		}
	}
	return "absent"
}

const examplePlannedActual = `[` +
	`{"name":"Receita","planned":8000.00,"actual":7500.00,"difference":-500.00,"contributions":false,"lines":[` +
	`{"name":"Salário","planned":8000.00,"actual":7500.00,"difference":-500.00}]},` +
	`{"name":"Fixos e essenciais","planned":2500.00,"actual":2500.00,"difference":0.00,"contributions":false,"lines":[` +
	`{"name":"Aluguel","planned":2500.00,"actual":2500.00,"difference":0.00}]},` +
	`{"name":"Variáveis","planned":1200.00,"actual":1070.00,"difference":130.00,"contributions":false,"lines":[` +
	`{"name":"Mercado","planned":900.00,"actual":950.00,"difference":-50.00},` +
	`{"name":"Lazer","planned":300.00,"actual":120.00,"difference":180.00}]},` +
	`{"name":"Provisões","planned":350.00,"actual":150.00,"difference":200.00,"contributions":true,"lines":[` +
	`{"name":"IPTU","planned":150.00,"actual":150.00,"difference":0.00},` +
	`{"name":"Revisão do carro","planned":200.00,"actual":0.00,"difference":200.00}]},` +
	`{"name":"Reservas de desejo","planned":400.00,"actual":0.00,"difference":400.00,"contributions":true,"lines":[` +
	`{"name":"Viagem","planned":400.00,"actual":0.00,"difference":400.00}]}]`

func TestTheClosingOfTheExampleWeighsTheMonthsBudgetAgainstTheMonth(t *testing.T) {
	example := newClosingExample(t, 350)
	if got := example.fieldOf("review.closing", `["2026-03"]`, "plannedActual"); got != examplePlannedActual {
		t.Errorf("plannedActual =\n%s\nwant\n%s", got, examplePlannedActual)
	}
	if got := example.fieldOf("review.closing", `["2026-03"]`, "leftover"); got != "130.00" {
		t.Errorf("leftover %s, want 130.00", got)
	}
	want := `[{"name":"Caixinha da casa","amount":150.00,"state":"done"},{"name":"Revisão do carro","amount":200.00,"state":"not-found"},` +
		`{"name":"Viagem","amount":400.00,"state":"not-found"},{"name":"Investimento","amount":3550.00,"state":"not-found"}]`
	if got := example.fieldOf("review.closing", `["2026-03"]`, "contributions"); got != want {
		t.Errorf("contributions = %s\nwant %s", got, want)
	}
	if got := closingIn(t, example.readEnv, "2026-03"); got.Closed || got.ClosedOn != nil || got.PendingCount != 0 {
		t.Errorf("the open month reads closed %t on %v with %d pending", got.Closed, got.ClosedOn, got.PendingCount)
	}

	overspent := newClosingExample(t, 900)
	if got := overspent.fieldOf("review.closing", `["2026-03"]`, "leftover"); got != "-420.00" {
		t.Errorf("with Mercado at 1500.00 the leftover is %s, want -420.00", got)
	}
}

// budgetViewSpending is, by line name, the figure budget.view shows as done or
// received for each income, fixed and variable line of the month.
func budgetViewSpending(t *testing.T, env *readEnv, month string) map[string]float64 {
	t.Helper()
	var view struct {
		Groups []struct {
			Key   string `json:"key"`
			Lines []struct {
				Name  string `json:"name"`
				Cells []struct {
					Amount float64 `json:"amount"`
				} `json:"cells"`
			} `json:"lines"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(answerBody(t, env, "budget.view", `["`+month+`"]`)), &view); err != nil {
		t.Fatal(err)
	}
	spending := map[string]float64{}
	for _, group := range view.Groups {
		if group.Key != "rec" && group.Key != groupFixed && group.Key != groupVariable {
			continue
		}
		for _, line := range group.Lines {
			spending[line.Name] = line.Cells[1].Amount
		}
	}
	return spending
}

func TestEachSpentLineOfTheClosingIsTheFigureTheBudgetViewShows(t *testing.T) {
	example := newClosingExample(t, 350)
	spending := budgetViewSpending(t, example.readEnv, "2026-03")
	if len(spending) != 4 {
		t.Fatalf("budget.view shows %v", spending)
	}
	compared := 0
	for _, group := range closingIn(t, example.readEnv, "2026-03").PlannedActual[:3] {
		for _, line := range group.Lines {
			compared++
			if figure, shown := spending[line.Name]; !shown || figure != line.Actual {
				t.Errorf("%s: the closing's actual is %v, budget.view shows %v", line.Name, line.Actual, figure)
			}
		}
	}
	if compared != 4 {
		t.Errorf("compared %d lines, want 4", compared)
	}
}

func TestAClosedMonthIsDoneThroughoutAndTakesEveryTransferAsMade(t *testing.T) {
	example := newClosingExample(t, 350)
	authoredtest.CloseMonth(t, example.scratch.Database, fixtureNow, "2026-02", "2026-03-02")
	seen := closingIn(t, example.readEnv, "2026-02")
	if !seen.Closed || seen.ClosedOn == nil || *seen.ClosedOn != "2026-03-02" {
		t.Errorf("closed %t on %v, want closed on 2026-03-02", seen.Closed, seen.ClosedOn)
	}
	for _, step := range seen.Steps {
		if step.State != "done" || step.Markable {
			t.Errorf("step %s of the closed month is %s, markable %t", step.ID, step.State, step.Markable)
		}
	}
	setAside := 0
	for _, group := range seen.PlannedActual[3:] {
		for _, line := range group.Lines {
			setAside++
			if line.Actual != line.Planned {
				t.Errorf("%s of the closed month set aside %v of %v", line.Name, line.Actual, line.Planned)
			}
		}
	}
	if setAside != 3 || len(seen.Contributions) != 4 {
		t.Fatalf("%d provision and reserve lines and %d contributions", setAside, len(seen.Contributions))
	}
	for _, contribution := range seen.Contributions {
		if contribution.State != "done" {
			t.Errorf("%s of the closed month is %s", contribution.Name, contribution.State)
		}
	}
	if open := closingIn(t, example.readEnv, "2026-03"); open.Closed || open.stepState("leftover") != "todo" {
		t.Errorf("the open month after the close reads closed %t, leftover %s", open.Closed, open.stepState("leftover"))
	}
}

// closingPendingExample is the example's ledger and budget with reviews waiting on the
// purchases named, over a ledger that leaves out the rows named.
func closingPendingExample(t *testing.T, waiting []string, leftOut ...string) (*authoredtest.Scratch, *ledgertest.Fixture) {
	t.Helper()
	fixture := closingLedger(t, 350, leftOut...)
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-03", closingBudget())
	reviews := make([]waitingReview, 0, len(waiting))
	for _, id := range waiting {
		reviews = append(reviews, waitingReview{id, string(classrules.ReasonNewEstablishment), "", nil})
	}
	rulesLeave(t, scratch, fixture.Path, reviews...)
	return scratch, fixture
}

func expectPending(t *testing.T, env *readEnv, month string, count int) {
	t.Helper()
	seen := closingIn(t, env, month)
	state := "done"
	if count > 0 {
		state = "pending"
	}
	if seen.PendingCount != count || seen.stepState(stepPending) != state {
		t.Errorf("%s counts %d pending with the first step %s, want %d and %s", month, seen.PendingCount, seen.stepState(stepPending), count, state)
	}
}

func TestTheFirstStepCountsTheReviewsWaitingUpToTheMonthsEnd(t *testing.T) {
	three := []string{txClosingFeb27, txClosingFeb28, txClosingMar05}
	scratch, fixture := closingPendingExample(t, three)
	env := newReadEnvOver(t, fixture, scratch.Database)
	expectPending(t, env, "2026-02", 2)
	expectPending(t, env, "2026-03", 3)
	expectPending(t, env, "2026-01", 0)

	later, laterLedger := closingPendingExample(t, []string{txClosingMar05})
	onlyMarch := newReadEnvOver(t, laterLedger, later.Database)
	expectPending(t, onlyMarch, "2026-02", 0)
	expectPending(t, onlyMarch, "2026-03", 1)

	// The same database over a snapshot that no longer lists one of them.
	_, without := closingPendingExample(t, nil, txClosingFeb28)
	vanished := newReadEnvOver(t, without, scratch.Database)
	expectPending(t, vanished, "2026-02", 1)
	expectPending(t, vanished, "2026-03", 2)
}

func tickSteps(t *testing.T, scratch *authoredtest.Scratch, month brday.Month, steps ...string) {
	t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.tickSteps", At: fixtureNow}
	err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		for _, step := range steps {
			if err := recorder.MarkClosingStepDone(t.Context(), month, step); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTheStepsTickedForAMonthAreDoneInThatMonthAlone(t *testing.T) {
	example := newClosingExample(t, 350)
	markable := closingStepOrder[1:7]
	tickSteps(t, example.scratch, "2026-03", markable...)
	march, april := closingIn(t, example.readEnv, "2026-03"), closingIn(t, example.readEnv, "2026-04")
	for _, step := range markable {
		if march.stepState(step) != "done" || april.stepState(step) != "todo" {
			t.Errorf("%s reads %s in 2026-03 and %s in 2026-04", step, march.stepState(step), april.stepState(step))
		}
	}
	if march.stepState(stepClose) != "todo" {
		t.Errorf("the close of the ticked month reads %s", march.stepState(stepClose))
	}
}

func TestTheClosedMonthsAreListedNewestFirstAndTheReportIsEmpty(t *testing.T) {
	example := newClosingExample(t, 350)
	authoredtest.CloseMonth(t, example.scratch.Database, fixtureNow, "2025-12", "2026-01-05")
	authoredtest.CloseMonth(t, example.scratch.Database, fixtureNow, "2026-02", "2026-03-02")
	want := `[{"month":"2026-02","closedOn":"2026-03-02"},{"month":"2025-12","closedOn":"2026-01-05"}]`
	for _, month := range []string{"2026-03", "2026-02"} {
		if got := example.fieldOf("review.closing", `["`+month+`"]`, "closedMonths"); got != want {
			t.Errorf("closedMonths of %s = %s, want %s", month, got, want)
		}
		if got := example.fieldOf("review.closing", `["`+month+`"]`, "report"); got != "[]" {
			t.Errorf("report of %s = %s", month, got)
		}
	}
}

// subscriptionsLedger is the C6 card with the charges of four services, one
// of them every three months.
func subscriptionsLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := c6Ledger(t)
	charge := func(id string, at time.Time, description string, amount float64) {
		f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: "card-c6", Date: at, Description: description,
			Amount: amount, Currency: "BRL", Type: "DEBIT",
			Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "5555666677778888"}}})
	}
	charge("tx-sub-one-feb", utc(2026, time.February, 10, 15, 0), "STREAMING EXEMPLO", 39.9)
	charge("tx-sub-one-mar", utc(2026, time.March, 10, 15, 0), "STREAMING EXEMPLO", 39.9)
	charge("tx-sub-two-mar", utc(2026, time.March, 1, 15, 0), "MUSICA EXEMPLO", 19.9)
	charge("tx-sub-three-dec", utc(2025, time.December, 5, 15, 0), "NUVEM EXEMPLO", 120)
	charge("tx-sub-three-mar", utc(2026, time.March, 5, 15, 0), "NUVEM EXEMPLO", 120)
	charge("tx-sub-four-mar", utc(2026, time.March, 15, 15, 0), "JORNAL EXEMPLO", 25)
	return f
}

// closingSubscription is a subscription the person keeps, made from one of the
// charges of its service.
type closingSubscription struct {
	purchase, name string
	status         authored.SubscriptionStatus
	reviewOn       brday.Day
}

// The four subscriptions of the criteria, created in this order, ids 1 to 4,
// each with its status set on 2026-03-02.
var closingSubscriptions = []closingSubscription{
	{"tx-sub-one-mar", "Streaming", authored.StatusReview, "2026-03-15"},
	{"tx-sub-two-mar", "Música", authored.StatusCancel, "2026-03-01"},
	{"tx-sub-three-mar", "Nuvem", authored.StatusKeep, "2026-04-10"},
	{"tx-sub-four-mar", "Jornal", authored.StatusKeep, "2026-03-31"},
}

func subscribeOver(t *testing.T, scratch *authoredtest.Scratch, path string, subscriptions ...closingSubscription) {
	t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.subscribe", At: utc(2026, time.March, 2, 15, 0)}
	err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		writes := writesOver(recorder, viewOf(t, path))
		defer writes.Authored.Close()
		for _, subscription := range subscriptions {
			target, found, err := lookupTarget(t.Context(), writes, subscription.purchase)
			if err != nil || !found {
				return errors.Join(err, errors.New(subscription.purchase+" is not listed"))
			}
			_, err = recorder.CreateSubscription(t.Context(), authored.Subscription{
				EstablishmentKey: target.listed.EstablishmentKey, CreatedFrom: target.key, CreatedDay: target.listed.Day,
				CreatedCents: -target.listed.Amount, Name: subscription.name, Status: subscription.status,
				StatusDay: "2026-03-02", ReviewOn: subscription.reviewOn,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("the subscriptions: %v", err)
	}
}

func newSubscriptionsToReviewEnv(t *testing.T) *settingsEnv {
	t.Helper()
	fixture := subscriptionsLedger(t)
	env := newSettingsEnv(t, fixture, fixtureNow)
	subscribeOver(t, env.scratch, fixture.Path, closingSubscriptions...)
	return env
}

func TestTheSubscriptionsToReviewAreTheOnesDueByTheMonthsEndAndNotBeingCancelled(t *testing.T) {
	env := newSubscriptionsToReviewEnv(t)
	for _, c := range []struct {
		month string
		want  []string
	}{{"2026-03", []string{"1", "4"}}, {"2026-02", nil}, {"2026-04", []string{"1", "3", "4"}}} {
		seen := closingIn(t, env.readEnv, c.month)
		var ids []string
		for _, due := range seen.SubscriptionsToReview {
			ids = append(ids, due.ID)
			detail := answerOf(t, env.readEnv, "subscriptions.detail", `["`+due.ID+`"]`)
			if due.Name != detail["name"] || due.Amount != detail["amount"] || due.Cycle != detail["cycle"] || due.ReviewOn != detail["reviewOn"] {
				t.Errorf("%s: subscription %s reads %+v, its detail %v %v %v %v", c.month, due.ID, due, detail["name"], detail["amount"], detail["cycle"], detail["reviewOn"])
			}
		}
		if !slices.Equal(ids, c.want) {
			t.Errorf("%s lists %v, want %v", c.month, ids, c.want)
		}
	}
	if got := closingIn(t, env.readEnv, "2026-04").SubscriptionsToReview; got[1].Cycle != cycleQuarterly || got[1].Amount != 120 {
		t.Errorf("the quarterly service reads %+v", got[1])
	}
}

func TestAMissingSnapshotLeavesTheClosingUnavailable(t *testing.T) {
	example := newClosingExample(t, 350)
	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), fixtureNow), withDatabase(example.scratch.Database))
	expectError(t, "a missing snapshot", missing.call("/api/review/closing", `["2026-03"]`, missing.unlock()),
		http.StatusServiceUnavailable, "ledger_unavailable")
}
