package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

func TestTheHomeSummaryHasNoLimitNoWeeksAndNothingPendingOverAMonthWithPurchases(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	status, body := env.answer("transactions.list", `[{"month":"2026-03","period":"month"}]`)
	if status != http.StatusOK || len(body) < 100 {
		t.Fatalf("the month has no purchases to be set against: status %d", status)
	}

	home := answerOf(t, env, "home.summary", `["2026-03"]`)

	limit := home["limit"].(map[string]any)
	if limit["cap"].(float64) != 0 || limit["spent"].(float64) != 0 {
		t.Errorf("the limit is %v, want cap 0 and spent 0 until a budget exists", limit)
	}
	if len(home["weeks"].([]any)) != 0 || home["pendingCount"].(float64) != 0 {
		t.Errorf("weeks %v pending %v", home["weeks"], home["pendingCount"])
	}
}

func TestTheHomeNetWorthIsTheOneTheNetWorthScreenShows(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	home := answerOf(t, env, "home.summary", `["2026-03"]`)["netWorth"].(map[string]any)
	totals := bucketTotals(t, answerOf(t, env, "netWorth.overview", `[]`))
	if cents(home["available"].(float64)) != totals["Disponível"] || cents(home["reserved"].(float64)) != totals["Reservado"] ||
		cents(home["invested"].(float64)) != totals["Investido"] {
		t.Errorf("the home shows %v and the net worth screen %v", home, totals)
	}
}

func TestTheHomeOffersThePayAheadFigureOfTheCardsNextBill(t *testing.T) {
	f := c6Ledger(t)
	addCardPurchase(f, "tx-on-closing", utc(2026, time.March, 4, 15, 0), 50, "")
	addCardPurchase(f, "tx-later", utc(2026, time.March, 12, 15, 0), 20, "")

	home := answerOf(t, envAt(t, f, 2026, time.March, 20), "home.summary", `["2026-03"]`)

	payAhead, ok := home["payAhead"].(map[string]any)
	if !ok || payAhead["amount"].(float64) != 70 || payAhead["cardLabel"] != "C6 Carbon" {
		t.Errorf("pay ahead %v, want 70 on the C6 card", home["payAhead"])
	}
	card := answerOf(t, envAt(t, f, 2026, time.March, 20), "card.overview", `[]`)
	if card["payAhead"].(map[string]any)["amount"] != payAhead["amount"] {
		t.Error("the home and the card page disagree on what falls on the next bill")
	}
}

func TestTheHomeOffersNoPayAheadBeforeThereIsAnythingToPay(t *testing.T) {
	home := answerOf(t, envAt(t, c6Ledger(t), 2026, time.March, 20), "home.summary", `["2026-03"]`)
	if home["payAhead"] != nil {
		t.Errorf("pay ahead %v with nothing bought since the closing", home["payAhead"])
	}
}

func TestTheHomeRefusesAMonthThatIsNotOne(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, arguments := range []string{`["2026-13"]`, `["march"]`, `[1]`, `[]`} {
		if status, _ := env.answer("home.summary", arguments); status == http.StatusOK {
			t.Errorf("%s was answered", arguments)
		}
	}
}

func TestOverTheEmptyLedgerTheHomeIsAllZeroAndOffersNoPayAhead(t *testing.T) {
	home := answerOf(t, newReadEnv(t, newEmptyLedger(t)), "home.summary", `["2026-03"]`)
	worth := home["netWorth"].(map[string]any)
	if worth["available"].(float64) != 0 || worth["reserved"].(float64) != 0 || worth["invested"].(float64) != 0 || home["payAhead"] != nil {
		t.Errorf("the empty ledger's home is %v", home)
	}
}

func TestTheHomeSummaryDoesNotWorkOutAYieldItDoesNotShow(t *testing.T) {
	view, err := loadPots(withNow(t.Context(), func() time.Time { return fixtureNow }), ledgerOf(t, nightlyLedger(t, 6)), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range view.entries {
		if entry.yield.Known || len(entry.yield.Days) != 0 {
			t.Errorf("%s was given a yield the home does not show", entry.id)
		}
	}
}

// The summary shows no yield, so its handler must not ask for one. Nothing it
// answers shows the difference, so the call itself is what is checked.
func TestTheHomeHandlerLoadsThePotsWithoutAYield(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "home.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name, isName := call.Fun.(*ast.Ident); !isName || name.Name != "loadPotsWithChoices" {
			return true
		}
		calls++
		if flag, isName := call.Args[2].(*ast.Ident); !isName || flag.Name != "false" {
			t.Errorf("home.go loads the pots with %v as its yield flag, want false", call.Args[2])
		}
		return true
	})
	if calls != 1 {
		t.Errorf("home.go calls loadPotsWithChoices %d times, want once", calls)
	}
}

func TestTheHomeOffersNoPayAheadWhenTheNextBillsAmountIsNotKnown(t *testing.T) {
	home := answerOf(t, envAt(t, noTableLedger(t), 2026, time.March, 20), "home.summary", `["2026-03"]`)
	if home["payAhead"] != nil {
		t.Errorf("home offers a pay-ahead figure for an amount that is not known: %v", home["payAhead"])
	}
}

func TestTheHomeNetWorthFollowsTheBucketsThePersonChose(t *testing.T) {
	env := potsPlacedEnv(t, newFullLedger(t), map[string]authored.PotBucket{
		potCaixinha: authored.BucketAvailable, potLCI: authored.BucketReserved,
	})
	home := answerOf(t, env, "home.summary", `["2026-03"]`)["netWorth"].(map[string]any)
	got := [3]int64{cents(home["available"].(float64)), cents(home["reserved"].(float64)), cents(home["invested"].(float64))}
	if want := [3]int64{285966, 827580, 300000}; got != want {
		t.Errorf("the home shows %v cents, want %v", got, want)
	}
	totals := bucketTotals(t, answerOf(t, env, "netWorth.overview", `[]`))
	if got != [3]int64{totals["Disponível"], totals["Reservado"], totals["Investido"]} {
		t.Errorf("the home shows %v and the net worth screen %v", got, totals)
	}
}

// The limit's example: a bank account and a card of one bank, a budget from
// 2026-03 whose only variable line is Mercado at 1000.00, and three card
// purchases put on it, the first at 02:00 UTC on the 8th, which is still the
// 7th in Brazil. Everything is invented.
const (
	limitBank = "acc-limit"
	limitCard = "card-limit"

	txMercadoSeventh   = "tx-mercado-seventh"
	txMercadoTenth     = "tx-mercado-tenth"
	txMercadoSixteenth = "tx-mercado-sixteenth"
)

// limitPurchase is a card purchase in reais.
func limitPurchase(id string, at time.Time, amount float64) ledgertest.Transaction {
	return ledgertest.Transaction{ID: id, AccountID: limitCard, Date: at, Description: "COMPRA " + id, Amount: amount,
		Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "4444555566667777"}}}
}

var marchPurchases = []ledgertest.Transaction{
	limitPurchase(txMercadoSeventh, utc(2026, time.March, 8, 2, 0), 400),
	limitPurchase(txMercadoTenth, utc(2026, time.March, 10, 15, 0), 700),
	limitPurchase(txMercadoSixteenth, utc(2026, time.March, 16, 15, 0), 30),
}

// aprilPurchases are example A's: 800.00 in 01–07, 650.00 in 08–14 and 210.00
// on 15–17.
var aprilPurchases = []ledgertest.Transaction{
	limitPurchase("tx-april-third", utc(2026, time.April, 3, 15, 0), 800),
	limitPurchase("tx-april-ninth", utc(2026, time.April, 9, 15, 0), 650),
	limitPurchase("tx-april-sixteenth", utc(2026, time.April, 16, 15, 0), 210),
}

// newLimitLedger is the example's two accounts, its owner's identity and rows.
func newLimitLedger(t testing.TB, rows ...ledgertest.Transaction) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-limit", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddIdentity(ledgertest.Identity{ID: "identity-limit", ItemID: "item-limit", Document: "987.654.321-00", FullName: "Pessoa Exemplo"})
	f.AddAccount(ledgertest.Account{ID: limitBank, ItemID: "item-limit", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Corrente", Currency: "BRL", Balance: 2000, TransferNumber: "341/0001/40000-4"})
	f.AddAccount(ledgertest.Account{ID: limitCard, ItemID: "item-limit", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Gold Visa", Currency: "BRL", Balance: 1130, Credit: map[string]any{"creditLimit": 6000, "availableCreditLimit": 4870}})
	for _, row := range rows {
		f.AddTransaction(row)
	}
	return f
}

// limitBudget is the budget from 2026-03: a fixed line and Mercado.
func limitBudget() authored.Document {
	return authored.Document{
		authoredtest.Group("fix", authoredtest.Line("Contas da casa", 200_000)),
		authoredtest.Group("var", authoredtest.Line("Mercado", 100_000)),
	}
}

type limitExample struct {
	t       *testing.T
	ledger  *ledgertest.Fixture
	scratch *authoredtest.Scratch
	lines   map[string]authored.LineID
}

// newLimitExample saves the budget from 2026-03 and, when april holds, a
// version from 2026-04 whose Mercado is 3000.00; then puts every purchase of
// the ledger on Mercado.
func newLimitExample(t *testing.T, april bool, rows ...ledgertest.Transaction) *limitExample {
	t.Helper()
	f := newLimitLedger(t, rows...)
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-03", limitBudget())
	example := &limitExample{t: t, ledger: f, scratch: scratch, lines: linesOfMonthByName(t, scratch.Database, "2026-03")}
	if april {
		example.saveMercadoFrom("2026-04", 300_000)
	}
	example.author("scratch.limitExample", func(a authoring) error {
		for _, row := range rows {
			if row.AccountID != limitCard {
				continue
			}
			if err := a.state(row.ID, authored.TransactionState{Line: a.line("Mercado"), ClassifiedBy: authored.AuthorPerson}); err != nil {
				return err
			}
		}
		return nil
	})
	return example
}

// saveMercadoFrom saves the March budget again from month, Mercado at cents
// under its own id.
func (example *limitExample) saveMercadoFrom(month brday.Month, cents money.Cents) {
	view := example.scratch.Database.View()
	defer view.Close()
	march, err := view.BudgetOf(example.t.Context(), "2026-03")
	if err != nil {
		example.t.Fatal(err)
	}
	document := authoredtest.DocumentOf(march)
	for _, group := range document {
		for i := range group.Lines {
			if group.Lines[i].Name == "Mercado" {
				group.Lines[i].Cents = cents
			}
		}
	}
	authoredtest.SaveBudget(example.t, example.scratch.Database, fixtureNow, month, document)
}

func (example *limitExample) author(method string, write func(authoring) error) {
	example.t.Helper()
	authorOver(example.t, example.scratch, viewOf(example.t, example.ledger.Path), example.lines, fixtureNow, method, write)
}

// at is a router over both stores with the clock at noon of the day in Brazil.
func (example *limitExample) at(year int, month time.Month, day int) *readEnv {
	example.t.Helper()
	env := newTestEnv(example.t, withLedger(example.ledger.Path, utc(year, month, day, 15, 0)), withDatabase(example.scratch.Database))
	return &readEnv{testEnv: env, cookie: env.unlock()}
}

type homeFigures struct {
	Limit json.RawMessage `json:"limit"`
	Weeks json.RawMessage `json:"weeks"`
}

type weeklyFigures struct {
	Limit json.RawMessage `json:"limit"`
	Week  json.RawMessage `json:"week"`
}

// figuresOf is one answer's figures as the server wrote them, after checking
// it against the method's result schema.
func figuresOf(t *testing.T, env *readEnv, method, month string, into any) {
	t.Helper()
	status, body := env.answer(method, `["`+month+`"]`)
	if status != http.StatusOK {
		t.Fatalf("%s %s answered %d %s", method, month, status, body)
	}
	if problem := conformanceProblem(method, body); problem != "" {
		t.Errorf("%s %s does not conform: %s", method, month, problem)
	}
	if err := json.Unmarshal(body, into); err != nil {
		t.Fatal(err)
	}
}

func homeOf(t *testing.T, env *readEnv, month string) homeFigures {
	t.Helper()
	var figures homeFigures
	figuresOf(t, env, "home.summary", month, &figures)
	return figures
}

func weeklyOf(t *testing.T, env *readEnv, month string) weeklyFigures {
	t.Helper()
	var figures weeklyFigures
	figuresOf(t, env, "review.weekly", month, &figures)
	return figures
}

// week is one WeekGuide as the server writes it; a spent of "null" is a week
// not begun.
func week(label, guide, spent string, current bool) string {
	return `{"label":"` + label + `","guide":` + guide + `,"spent":` + spent + `,"current":` + strconv.FormatBool(current) + `}`
}

func weeksJSON(weeks ...string) string { return "[" + strings.Join(weeks, ",") + "]" }

// Example C's guides, and the same month once every week has begun.
var (
	exampleCWeeks = weeksJSON(week("01–07", "225.81", "400.00", false), week("08–14", "175.00", "700.00", false),
		week("15–21", "0.00", "30.00", true), week("22–28", "0.00", "null", false), week("29–31", "0.00", "null", false))
	exampleCWeeksPast = weeksJSON(week("01–07", "225.81", "400.00", false), week("08–14", "175.00", "700.00", false),
		week("15–21", "0.00", "30.00", false), week("22–28", "0.00", "0.00", false), week("29–31", "0.00", "0.00", false))
)

const (
	exampleCLimit       = `{"cap":1000.00,"spent":1130.00}`
	exampleCWeeklyLimit = `{"cap":1000.00,"spent":1130.00,"remaining":-130.00,"over":true,"fill":100}`
	exampleCWeek        = `{"guide":0.00,"spent":30.00,"fill":0}`
)

func expectFigures(t *testing.T, label string, got [2]json.RawMessage, want [2]string) {
	t.Helper()
	if string(got[0]) != want[0] || string(got[1]) != want[1] {
		t.Errorf("%s:\n  %s\n  %s\nwant\n  %s\n  %s", label, got[0], got[1], want[0], want[1])
	}
}

func (figures homeFigures) pair() [2]json.RawMessage {
	return [2]json.RawMessage{figures.Limit, figures.Weeks}
}
func (figures weeklyFigures) pair() [2]json.RawMessage {
	return [2]json.RawMessage{figures.Limit, figures.Week}
}

func TestTheHomeAndTheWeeklyReviewWeighTheMonthAgainstItsVariableCap(t *testing.T) {
	env := newLimitExample(t, false, marchPurchases...).at(2026, time.March, 20)
	expectFigures(t, "home.summary 2026-03", homeOf(t, env, "2026-03").pair(), [2]string{exampleCLimit, exampleCWeeks})
	expectFigures(t, "review.weekly 2026-03", weeklyOf(t, env, "2026-03").pair(), [2]string{exampleCWeeklyLimit, exampleCWeek})
}

func TestTheCapIsTheBudgetOfTheMonthAskedAndTheGuidesFollowToday(t *testing.T) {
	example := newLimitExample(t, true, append(slices.Clone(marchPurchases), aprilPurchases...)...)
	march := example.at(2026, time.March, 20)
	if got := homeOf(t, march, "2026-04").Limit; string(got) != `{"cap":3000.00,"spent":1660.00}` {
		t.Errorf("April's limit is %s, want its own version's cap 3000.00", got)
	}
	if got := homeOf(t, march, "2026-03").Limit; string(got) != exampleCLimit {
		t.Errorf("March's limit is %s, want %s", got, exampleCLimit)
	}
	april := example.at(2026, time.April, 17)
	exampleA := weeksJSON(week("01–07", "700.00", "800.00", false), week("08–14", "669.57", "650.00", false),
		week("15–21", "678.13", "210.00", true), week("22–28", "1042.22", "null", false), week("29–30", "1340.00", "null", false))
	expectFigures(t, "home.summary 2026-04 on 2026-04-17", homeOf(t, april, "2026-04").pair(),
		[2]string{`{"cap":3000.00,"spent":1660.00}`, exampleA})
	expectFigures(t, "review.weekly 2026-04 on 2026-04-17", weeklyOf(t, april, "2026-04").pair(),
		[2]string{`{"cap":3000.00,"spent":1660.00,"remaining":1340.00,"over":false,"fill":55}`, `{"guide":678.13,"spent":210.00,"fill":31}`})
	expectFigures(t, "home.summary 2026-03 on 2026-04-17", homeOf(t, april, "2026-03").pair(),
		[2]string{exampleCLimit, exampleCWeeksPast})
	if got := weeklyOf(t, april, "2026-03").Week; string(got) != "null" {
		t.Errorf("a past month has the current week %s", got)
	}
}

func TestAMonthNotBegunHasItsGuidesAndNoSpendingAndNoCurrentWeek(t *testing.T) {
	env := newLimitExample(t, true, marchPurchases...).at(2026, time.March, 20)
	exampleF := weeksJSON(week("01–07", "700.00", "null", false), week("08–14", "913.04", "null", false),
		week("15–21", "1312.50", "null", false), week("22–28", "2333.33", "null", false), week("29–30", "3000.00", "null", false))
	expectFigures(t, "home.summary 2026-04", homeOf(t, env, "2026-04").pair(), [2]string{`{"cap":3000.00,"spent":0.00}`, exampleF})
	if got := weeklyOf(t, env, "2026-04").Week; string(got) != "null" {
		t.Errorf("a month not begun has the current week %s", got)
	}
}

func TestAMonthWithNoVariableLineHasNoWeeks(t *testing.T) {
	example := newLimitExample(t, false, marchPurchases...)
	authoredtest.SaveBudget(t, example.scratch.Database, fixtureNow, "2026-03", authored.Document{
		authoredtest.Group("fix", authoredtest.Line("Contas da casa", 200_000)),
	})
	env := example.at(2026, time.March, 20)
	expectFigures(t, "home.summary 2026-03", homeOf(t, env, "2026-03").pair(), [2]string{`{"cap":0.00,"spent":0.00}`, `[]`})
	expectFigures(t, "review.weekly 2026-03", weeklyOf(t, env, "2026-03").pair(),
		[2]string{`{"cap":0.00,"spent":0.00,"remaining":0.00,"over":false,"fill":0}`, "null"})
}

// Each of these would add to Mercado if what it counts for were not decided by
// Dispose: all four are put on it, or on a fixed line.
func TestWhatTheVariableLinesDoNotCountMovesNoFigure(t *testing.T) {
	transfer := ledgertest.Transaction{ID: "tx-own-transfer-limit", AccountID: limitBank, Date: utc(2026, time.March, 13, 12, 0),
		Description: "PIX ENVIADO PESSOA EXEMPLO", Amount: -300, Currency: "BRL", Type: "DEBIT", Raw: map[string]any{"operationType": "PIX",
			"paymentData": payment("PIX", nil, party("Pessoa Exemplo", "98765432100"))}}
	rows := append(slices.Clone(marchPurchases), limitPurchase("tx-fixed", utc(2026, time.March, 9, 15, 0), 150),
		limitPurchase("tx-excluded", utc(2026, time.March, 11, 15, 0), 99), limitPurchase("tx-waiting", utc(2026, time.March, 12, 15, 0), 55), transfer)
	example := newLimitExample(t, false, rows...)
	example.author("scratch.notCounted", func(a authoring) error {
		return each(
			func() error {
				return a.state("tx-fixed", authored.TransactionState{Line: a.line("Contas da casa"), ClassifiedBy: authored.AuthorPerson})
			},
			func() error {
				return a.state("tx-excluded", authored.TransactionState{Line: a.line("Mercado"), ClassifiedBy: authored.AuthorPerson,
					Exclusion: authoredtest.Some("Compra de outra pessoa")})
			},
			func() error {
				return a.state("tx-waiting", authored.TransactionState{Line: a.line("Mercado"), PendingReason: "Confiança abaixo do limite",
					PendingLine: a.line("Mercado")})
			},
			func() error {
				return a.state(transfer.ID, authored.TransactionState{Line: a.line("Mercado"), ClassifiedBy: authored.AuthorPerson})
			},
		)
	})
	env := example.at(2026, time.March, 20)
	expectFigures(t, "home.summary 2026-03", homeOf(t, env, "2026-03").pair(), [2]string{exampleCLimit, exampleCWeeks})
	expectFigures(t, "review.weekly 2026-03", weeklyOf(t, env, "2026-03").pair(), [2]string{exampleCWeeklyLimit, exampleCWeek})
}

// The home's limit is the variable group of budget.view, over every month of
// two examples: this one, and the budget's, whose Mercado has a split part and
// a refund.
func TestTheLimitIsTheVariableGroupTheBudgetViewShows(t *testing.T) {
	limit := newLimitExample(t, true, append(slices.Clone(marchPurchases), aprilPurchases...)...)
	budget := newBudgetExample(t)
	for label, env := range map[string]*readEnv{"the limit's example": limit.at(2026, time.March, 20), "the budget's example": budget.readEnv} {
		for _, month := range []string{"2026-01", "2026-02", "2026-03", "2026-04", "2026-05"} {
			var view struct {
				Groups []struct {
					Key           string          `json:"key"`
					Subtotal      json.RawMessage `json:"subtotal"`
					SubtotalSpent json.RawMessage `json:"subtotalSpent"`
				} `json:"groups"`
			}
			figuresOf(t, env, "budget.view", month, &view)
			index := slices.IndexFunc(view.Groups, func(group struct {
				Key           string          `json:"key"`
				Subtotal      json.RawMessage `json:"subtotal"`
				SubtotalSpent json.RawMessage `json:"subtotalSpent"`
			}) bool {
				return group.Key == groupVariable
			})
			want := `{"cap":` + string(view.Groups[index].Subtotal) + `,"spent":` + string(view.Groups[index].SubtotalSpent) + `}`
			if got := homeOf(t, env, month).Limit; string(got) != want {
				t.Errorf("%s, %s: the home's limit is %s and the budget's variable group %s", label, month, got, want)
			}
		}
	}
}

func TestOverASnapshotThatCannotBeReadTheHomeTheFrameAndTheReviewAreUnavailable(t *testing.T) {
	example := newLimitExample(t, false, marchPurchases...)
	unreadable := filepath.Join(t.TempDir(), "pluggy-snapshot.db")
	if err := os.WriteFile(unreadable, []byte("not a SQLite database, only text long enough to fill a header"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{unreadable, filepath.Join(t.TempDir(), "missing.db")} {
		env := newTestEnv(t, withLedger(path, fixtureNow), withDatabase(example.scratch.Database))
		cookie := env.unlock()
		for _, call := range [][2]string{{"/api/home/summary", `["2026-03"]`}, {"/api/shell/overview", `[]`}, {"/api/review/weekly", `["2026-03"]`}} {
			expectError(t, call[0]+" over "+filepath.Base(path), env.call(call[0], call[1], cookie), http.StatusServiceUnavailable, "ledger_unavailable")
		}
	}
}
