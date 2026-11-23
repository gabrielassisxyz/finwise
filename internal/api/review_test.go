package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/classrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func init() {
	readCases = append(readCases,
		readCase{"review.weekly", `["2026-03"]`},
		readCase{"review.payday", `["2026-03"]`},
	)
}

const (
	emptyWeeklyReview = `{"limit":{"cap":0.00,"spent":0.00,"remaining":0.00,"over":false,"fill":0},` +
		`"week":null,"queue":{"items":[],"resolved":0},"approvals":[],"warnings":[]}`
	paydayWithNoSalaryAccount = `{"cushion":0.00,"balance":null,"balanceLabel":null,"difference":null,"covered":null,"transfers":[]}`
)

// The full ledger has purchases in March, and still nothing is spent against a
// cap that does not exist.
func TestTheWeeklyReviewHasACapOfZeroOverAMonthWithPurchases(t *testing.T) {
	for _, name := range []string{"full", "empty"} {
		env := newReadEnv(t, ledgerNamed(t, name))
		expectAnswer(t, env, "review.weekly", `["2026-03"]`, emptyWeeklyReview)
	}
}

func TestThePaydayHasTheContractsEmptyValueWhenNoSalaryEverLanded(t *testing.T) {
	for _, name := range []string{"full", "empty"} {
		env := newReadEnv(t, ledgerNamed(t, name))
		expectAnswer(t, env, "review.payday", `["2026-03"]`, paydayWithNoSalaryAccount)
	}
}

func TestTheReviewMethodsRefuseAMonthThatIsNotOne(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, path := range []string{"/api/review/weekly", "/api/review/payday"} {
		for _, body := range []string{`["2026-13"]`, `["March"]`, `[3]`, `[null]`} {
			expectError(t, path+" "+body, env.call(path, body, env.cookie), http.StatusBadRequest, "bad_request")
		}
	}
}

// The review's invented ledger, as the examples of the queue and of the accept
// are written: a bank account and a card of one bank, a card purchase P1 of
// 2026-03-18, a PIX P2 of 2026-03-12 to a person and a card purchase P3 of
// 2026-03-05, an earlier transaction of P3's place and of P2's person, a
// purchase nothing waits on, two purchases of one day, one of February and
// eight more that the journal's examples answer.
const (
	reviewP1        = "tx-p1"
	reviewP2        = "tx-p2"
	reviewP3        = "tx-p3"
	reviewShopAgain = "tx-shop-earlier"
	reviewPixAgain  = "tx-friend-earlier"
	reviewSettled   = "tx-settled"
	reviewDayEarly  = "tx-day-early"
	reviewDayLate   = "tx-day-late"
	reviewFebruary  = "tx-february"

	reviewBank = "acc-review"
	reviewCard = "card-review"

	keyOfTheReviewFriend = "doc:33344455566"
	keyOfTheReviewShop   = "text:LOJA NOVA EXEMPLO"
)

// reviewAnswered are the purchases the journal's examples answer, one each.
var reviewAnswered = []string{"tx-w1", "tx-w2", "tx-w3", "tx-w4", "tx-w5", "tx-w6", "tx-w7", "tx-w8"}

// newReviewLedger builds the review's ledger without the rows left out.
func newReviewLedger(t testing.TB, leftOut ...string) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-review", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddIdentity(ledgertest.Identity{ID: "identity-review", ItemID: "item-review", Document: "987.654.321-00", FullName: "Pessoa Exemplo"})
	f.AddAccount(ledgertest.Account{ID: reviewBank, ItemID: "item-review", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Corrente", Currency: "BRL", Balance: 900, TransferNumber: "341/0001/30000-3"})
	f.AddAccount(ledgertest.Account{ID: reviewCard, ItemID: "item-review", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Gold Visa", Currency: "BRL", Balance: 400, Credit: map[string]any{"creditLimit": 3000, "availableCreditLimit": 2600}})
	add := func(row ledgertest.Transaction) {
		if !slices.Contains(leftOut, row.ID) {
			f.AddTransaction(row)
		}
	}
	card := func(id string, at time.Time, description string, amount float64) {
		add(ledgertest.Transaction{ID: id, AccountID: reviewCard, Date: at, Description: description, Amount: amount,
			Currency: "BRL", Type: "DEBIT",
			Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "5555666677774321"}}})
	}
	pixToTheFriend := func(id string, at time.Time, amount float64) {
		add(ledgertest.Transaction{ID: id, AccountID: reviewBank, Date: at, Description: "PIX ENVIADO JOANA EXEMPLO",
			Amount: amount, Currency: "BRL", Type: "DEBIT", Raw: map[string]any{"operationType": "PIX",
				"paymentData": payment("PIX", nil, party("Joana Exemplo", "33344455566"))}})
	}
	card(reviewP1, utc(2026, time.March, 18, 15, 0), "RESTAURANTE EXEMPLO", 64.9)
	pixToTheFriend(reviewP2, utc(2026, time.March, 12, 12, 0), -150)
	card(reviewP3, utc(2026, time.March, 5, 15, 0), "LOJA NOVA EXEMPLO", 89)
	card(reviewShopAgain, utc(2026, time.March, 2, 15, 0), "LOJA NOVA EXEMPLO", 45)
	pixToTheFriend(reviewPixAgain, utc(2026, time.March, 3, 12, 0), -40)
	card(reviewSettled, utc(2026, time.March, 10, 15, 0), "PADARIA EXEMPLO", 12)
	card(reviewDayEarly, utc(2026, time.March, 8, 11, 0), "BANCA EXEMPLO", 9)
	card(reviewDayLate, utc(2026, time.March, 8, 20, 0), "QUIOSQUE EXEMPLO", 15)
	card(reviewFebruary, utc(2026, time.February, 20, 15, 0), "FEIRA EXEMPLO", 70)
	for i, id := range reviewAnswered {
		card(id, utc(2026, time.March, 4, 10+i, 0), "MERCADINHO EXEMPLO "+string(rune('A'+i)), float64(10+i))
	}
	return f
}

// reviewBudget is the budget of the examples, saved from 2026-03.
func reviewBudget() authored.Document {
	return authored.Document{
		authoredtest.Group("rec", authoredtest.Line("Salário", 800_000)),
		authoredtest.Group("fix", authoredtest.Line("Contas da casa", 250_000)),
		authoredtest.Group("var", authoredtest.Line("Mercado", 90_000), authoredtest.Line("Restaurantes", 40_000)),
	}
}

func savedReviewBudget(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	authoredtest.SaveBudget(t, scratch.Database, utc(2026, time.March, 1, 15, 0), "2026-03", reviewBudget())
}

// waitingReview is a review the rules left on a transaction: its reason, and
// the line they suggest with their confidence, or neither.
type waitingReview struct {
	id, reason, suggestion string
	confidence             *int64
}

var reviewsOfTheExamples = []waitingReview{
	{reviewP1, string(classrules.ReasonLowConfidence), "Restaurantes", authoredtest.Some[int64](58)},
	{reviewP2, string(classrules.ReasonUnnamedPerson), "", nil},
	{reviewP3, string(classrules.ReasonNewEstablishment), "", nil},
}

// rulesLeave records, as one change of the ingest, each review waiting on its
// transaction of the ledger at path, the suggestion named in the
// transaction's own month.
func rulesLeave(t *testing.T, scratch *authoredtest.Scratch, path string, reviews ...waitingReview) {
	t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorRule, Method: "ingest.run", At: utc(2026, time.March, 1, 9, 0)}
	err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		writes := writesOver(recorder, viewOf(t, path))
		defer writes.Authored.Close()
		for _, review := range reviews {
			target, found, err := lookupTarget(t.Context(), writes, review.id)
			if err != nil || !found {
				return errors.Join(err, errors.New(review.id+" is not listed"))
			}
			var line *authored.LineID
			if review.suggestion != "" {
				id, err := lineNamedInMonth(t.Context(), writes.Authored, target.listed.Day, review.suggestion)
				if err != nil {
					return err
				}
				line = &id
			}
			if err := recorder.SetPendingReview(t.Context(), target.key, review.reason, line, review.confidence); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("the rules' reviews: %v", err)
	}
}

func writesOver(recorder *authored.Recorder, snapshot *ledger.View) Writes {
	return Writes{Ledger: snapshot, Authored: recorder.View(), Recorder: recorder}
}

// answeredOn records what an accept of id on line records, as a change of the
// accept on day at noon, and answers the step it wrote.
func answeredOn(t *testing.T, scratch *authoredtest.Scratch, path string, day brday.Day, id, line string) string {
	t.Helper()
	return settledOn(t, scratch, path, day, acceptPendingMethod, id, line)
}

// settledOn puts id on line with a step, as a change of method on day at noon.
func settledOn(t *testing.T, scratch *authoredtest.Scratch, path string, day brday.Day, method, id, line string) string {
	t.Helper()
	at, err := time.Parse(time.DateOnly, string(day))
	if err != nil {
		t.Fatal(err)
	}
	var step authored.StepID
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: method, At: at.Add(15 * time.Hour)}
	err = scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		writes := writesOver(recorder, viewOf(t, path))
		defer writes.Authored.Close()
		target, found, err := lookupTarget(t.Context(), writes, id)
		if err != nil || !found {
			return errors.Join(err, errors.New(id+" is not listed"))
		}
		settled, err := lineNamedInMonth(t.Context(), writes.Authored, target.listed.Day, line)
		if err != nil {
			return err
		}
		if err := recorder.SettleLine(t.Context(), target.key, settled, authored.AuthorPerson); err != nil {
			return err
		}
		step, err = recorder.AddHistoryStep(t.Context(), target.key, authored.StepEffect, "Você aceitou "+line)
		return err
	})
	if err != nil {
		t.Fatalf("the answer of %s on %s: %v", id, day, err)
	}
	return step.String()
}

// newReviewEnv is a router over the ledger at path and the app's database of
// scratch, with the clock at now and a session open.
func newReviewEnv(t *testing.T, path string, scratch *authoredtest.Scratch, now time.Time) *writeEnv {
	t.Helper()
	env := newTestEnv(t, withLedger(path, now), withDatabase(scratch.Database))
	return &writeEnv{testEnv: env, scratch: scratch, cookie: env.unlock()}
}

// reviewExamples is the examples' state over a ledger of its own, with the
// clock at 2026-03-20: the budget and the three reviews.
func reviewExamples(t *testing.T) (*writeEnv, *ledgertest.Fixture) {
	t.Helper()
	fixture := newReviewLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	savedReviewBudget(t, scratch)
	rulesLeave(t, scratch, fixture.Path, reviewsOfTheExamples...)
	return newReviewEnv(t, fixture.Path, scratch, fixtureNow), fixture
}

// queueItemSeen is an item of the queue as the wire carries it, its review
// kept as written.
type queueItemSeen struct {
	TransactionID  string          `json:"transactionId"`
	Day            string          `json:"day"`
	Name           string          `json:"name"`
	RawDescription string          `json:"rawDescription"`
	Amount         float64         `json:"amount"`
	AccountLabel   string          `json:"accountLabel"`
	Person         bool            `json:"person"`
	Pending        json.RawMessage `json:"pending"`
}

type queueSeen struct {
	Items    []queueItemSeen `json:"items"`
	Resolved int             `json:"resolved"`
}

func (seen queueSeen) ids() []string {
	ids := []string{}
	for _, item := range seen.Items {
		ids = append(ids, item.TransactionID)
	}
	return ids
}

func (env *writeEnv) queueOf(month string) queueSeen {
	env.t.Helper()
	response := env.post("review.weekly", `["`+month+`"]`)
	if response.Code != http.StatusOK {
		env.t.Fatalf("review.weekly %s answered %d %s", month, response.Code, response.Body.String())
	}
	if problem := conformanceProblem("review.weekly", response.Body.Bytes()); problem != "" {
		env.t.Errorf("review.weekly %s does not conform: %s", month, problem)
	}
	var answer struct {
		Queue queueSeen `json:"queue"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		env.t.Fatal(err)
	}
	return answer.Queue
}

// listedIn is every transaction the list answers for a month, as written.
func (env *writeEnv) listedIn(month string) map[string]map[string]json.RawMessage {
	env.t.Helper()
	response := env.post("transactions.list", listRaw(month, "month"))
	var rows []map[string]json.RawMessage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &rows) != nil {
		env.t.Fatalf("transactions.list %s answered %d %s", month, response.Code, response.Body.String())
	}
	listed := map[string]map[string]json.RawMessage{}
	for _, row := range rows {
		var id string
		if err := json.Unmarshal(row["id"], &id); err != nil {
			env.t.Fatal(err)
		}
		listed[id] = row
	}
	return listed
}

func TestTheQueueIsEveryPendingTransactionNewestFirstAsTheListAndThePanelShowIt(t *testing.T) {
	env, _ := reviewExamples(t)
	queue := env.queueOf("2026-03")
	if !slices.Equal(queue.ids(), []string{reviewP1, reviewP2, reviewP3}) {
		t.Fatalf("the queue holds %v, want P1, P2 and P3 in that order", queue.ids())
	}
	wantPending := map[string]string{
		reviewP1: `{"reason":"Baixa confiança","suggestion":"Restaurantes","confidence":58,"justification":null}`,
		reviewP2: `{"reason":"PIX para pessoa sem apelido","suggestion":null,"confidence":null,"justification":null}`,
		reviewP3: `{"reason":"Estabelecimento novo","suggestion":null,"confidence":null,"justification":null}`,
	}
	want := map[string]queueItemSeen{
		reviewP1: {Day: "2026-03-18", Name: "RESTAURANTE EXEMPLO", RawDescription: "RESTAURANTE EXEMPLO", Amount: -64.9, AccountLabel: "Gold Visa · final 4321"},
		reviewP2: {Day: "2026-03-12", Name: "Joana Exemplo", RawDescription: "PIX ENVIADO JOANA EXEMPLO", Amount: -150, AccountLabel: "Itaú · Conta Corrente", Person: true},
		reviewP3: {Day: "2026-03-05", Name: "LOJA NOVA EXEMPLO", RawDescription: "LOJA NOVA EXEMPLO", Amount: -89, AccountLabel: "Gold Visa · final 4321"},
	}
	listed := env.listedIn("2026-03")
	for _, item := range queue.Items {
		id := item.TransactionID
		if string(item.Pending) != wantPending[id] || string(item.Pending) != string(listed[id]["pending"]) {
			t.Errorf("%s: pending %s, want %s, and the list answers %s", id, item.Pending, wantPending[id], listed[id]["pending"])
		}
		item.TransactionID, item.Pending = "", nil
		if !reflect.DeepEqual(item, want[id]) {
			t.Errorf("%s: %+v, want %+v", id, item, want[id])
		}
		var listedName string
		if err := json.Unmarshal(listed[id]["name"], &listedName); err != nil || listedName != item.Name {
			t.Errorf("%s: the queue names it %q and the list %s", id, item.Name, listed[id]["name"])
		}
		detail := env.post("transactions.detail", `["`+id+`"]`)
		if described := decodeAnswer(t, detail.Body.Bytes())["accountDescription"]; detail.Code != http.StatusOK || described != item.AccountLabel {
			t.Errorf("%s: the panel describes the account as %v, the queue as %q", id, described, item.AccountLabel)
		}
	}
}

func TestTheQueueIsTheAccountsNowWhateverMonthIsAsked(t *testing.T) {
	env, _ := reviewExamples(t)
	march := env.queueOf("2026-03")
	for _, month := range []string{"2026-01", "2026-02", "2027-12"} {
		if got := env.queueOf(month); !reflect.DeepEqual(got, march) {
			t.Errorf("the queue of %s is %+v, want the one of 2026-03 %+v", month, got, march)
		}
	}
	expectError(t, "review.weekly 2026-13", env.post("review.weekly", `["2026-13"]`), http.StatusBadRequest, "bad_request")
}

// The review stays in the database while the bank does not list its row, and
// the row comes back with it when a later snapshot lists it again.
func TestAReviewOfARowTheSnapshotStoppedListingIsOutOfTheQueueUntilItIsListedAgain(t *testing.T) {
	env, _ := reviewExamples(t)
	before := env.queueOf("2026-03")
	without := newReviewEnv(t, newReviewLedger(t, reviewP2).Path, env.scratch, fixtureNow)
	if got := without.queueOf("2026-03").ids(); !slices.Equal(got, []string{reviewP1, reviewP3}) {
		t.Errorf("over a snapshot without P2 the queue holds %v, want P1 and P3", got)
	}
	again := newReviewEnv(t, newReviewLedger(t).Path, env.scratch, fixtureNow)
	if got := again.queueOf("2026-03"); !reflect.DeepEqual(got, before) {
		t.Errorf("over a snapshot that lists P2 again the queue is %+v, want %+v", got, before)
	}
}

func TestAReviewOfAMonthWhoseBudgetHasNoLineWaitsOutOfTheQueue(t *testing.T) {
	fixture := newReviewLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, utc(2026, time.February, 1, 15, 0), "2026-02", authoredtest.Emptied(reviewBudget()))
	savedReviewBudget(t, scratch)
	rulesLeave(t, scratch, fixture.Path, reviewsOfTheExamples[0],
		waitingReview{reviewFebruary, string(classrules.ReasonNewEstablishment), "", nil})
	env := newReviewEnv(t, fixture.Path, scratch, fixtureNow)
	if got := env.queueOf("2026-03").ids(); !slices.Equal(got, []string{reviewP1}) {
		t.Errorf("with February's budget empty the queue holds %v, want P1 alone", got)
	}
	for label, february := range map[string]authored.Document{
		"one line":             {authoredtest.Group("var", authoredtest.Line("Feira", 30_000))},
		"its only line income": {authoredtest.Group("rec", authoredtest.Line("Salário", 800_000))},
	} {
		authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", february)
		if got := env.queueOf("2026-03").ids(); !slices.Equal(got, []string{reviewP1, reviewFebruary}) {
			t.Errorf("with February's budget of %s the queue holds %v, want P1 and the February purchase", label, got)
		}
	}
}

func TestTwoPendingTransactionsOfOneDayAreInTheOrderOfTheList(t *testing.T) {
	fixture := newReviewLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	savedReviewBudget(t, scratch)
	rulesLeave(t, scratch, fixture.Path,
		waitingReview{reviewDayEarly, string(classrules.ReasonNewEstablishment), "", nil},
		waitingReview{reviewDayLate, string(classrules.ReasonNewEstablishment), "", nil})
	env := newReviewEnv(t, fixture.Path, scratch, fixtureNow)
	var inList []string
	response := env.post("transactions.list", listRaw("2026-03", "month"))
	var rows []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if id := row["id"].(string); id == reviewDayEarly || id == reviewDayLate {
			inList = append(inList, id)
		}
	}
	if got := env.queueOf("2026-03").ids(); !slices.Equal(got, inList) || !slices.Equal(got, []string{reviewDayLate, reviewDayEarly}) {
		t.Errorf("the queue holds %v and the list %v, want both the later first", got, inList)
	}
}

// The closing view and the close read the same definition up to a month's last
// day.
func TestThePendingTransactionsUpToALastDayAreThoseDatedOnOrBeforeIt(t *testing.T) {
	env, fixture := reviewExamples(t)
	view := env.scratch.Database.View()
	defer view.Close()
	views := Views{Ledger: viewOf(t, fixture.Path), Authored: view}
	for last, want := range map[brday.Day][]string{
		"2026-03-12": {reviewP2, reviewP3},
		"2026-03-11": {reviewP3},
		"2026-03-04": {},
		"2026-03-31": {reviewP1, reviewP2, reviewP3},
	} {
		pending, err := pendingTransactions(t.Context(), views, last)
		if err != nil {
			t.Fatal(err)
		}
		got := []string{}
		for _, transaction := range pending {
			got = append(got, transaction.listed.ID)
			if transaction.state.PendingReason == "" || transaction.key == 0 || transaction.listed.Day > last {
				t.Errorf("%s: key %d, review %q, day %s", transaction.listed.ID, transaction.key, transaction.state.PendingReason, transaction.listed.Day)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("pending up to %s = %v, want %v", last, got, want)
		}
	}
}

// answeredSoFar is the journal of the worked example of "answered so far": one
// transaction per row, the clock at 2026-03-21 and the week 15 to 21 March.
func answeredSoFar(t *testing.T) *writeEnv {
	t.Helper()
	fixture := newReviewLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	savedReviewBudget(t, scratch)
	var reviews []waitingReview
	for _, id := range reviewAnswered {
		reviews = append(reviews, waitingReview{id, string(classrules.ReasonNewEstablishment), "", nil})
	}
	rulesLeave(t, scratch, fixture.Path, reviews...)
	env := newReviewEnv(t, fixture.Path, scratch, utc(2026, time.March, 21, 15, 0))
	w := reviewAnswered
	answeredOn(t, scratch, fixture.Path, "2026-03-14", w[0], "Mercado")
	answeredOn(t, scratch, fixture.Path, "2026-03-15", w[1], "Mercado")
	answeredOn(t, scratch, fixture.Path, "2026-03-21", w[2], "Mercado")
	answeredOn(t, scratch, fixture.Path, "2026-03-17", w[3], "Mercado")
	expectUndone(t, "the undo of the answer of the 17th", env.undo(lastChangeOf(t, scratch)))
	env.undoStep(w[4], answeredOn(t, scratch, fixture.Path, "2026-03-18", w[4], "Mercado"))
	env.undoStep(w[5], answeredOn(t, scratch, fixture.Path, "2026-03-16", w[5], "Mercado"))
	answeredOn(t, scratch, fixture.Path, "2026-03-19", w[5], "Restaurantes")
	env.undoStep(w[6], answeredOn(t, scratch, fixture.Path, "2026-03-14", w[6], "Mercado"))
	answeredOn(t, scratch, fixture.Path, "2026-03-20", w[6], "Mercado")
	settledOn(t, scratch, fixture.Path, "2026-03-19", "transactions.setLine", w[7], "Mercado")
	return env
}

func lastChangeOf(t *testing.T, scratch *authoredtest.Scratch) string {
	t.Helper()
	changes := scratch.Changes(t)
	return strconv.FormatInt(changes[len(changes)-1].ID, 10)
}

func TestAnsweredSoFarCountsTheTransactionsAnsweredThisWeekFromTheJournal(t *testing.T) {
	env := answeredSoFar(t)
	for _, month := range []string{"2026-03", "2026-02"} {
		if got := env.queueOf(month).Resolved; got != 4 {
			t.Errorf("resolved for %s = %d, want 4", month, got)
		}
	}
}

// Each week counted is pinned by a transaction answered on its first day and
// one on its last, both counted, and one on the day before, not.
func TestTheWeekCountedIsTheOneOfItsMonthThatHoldsToday(t *testing.T) {
	cases := []struct {
		today    time.Time
		answered []brday.Day
		want     int
	}{
		{utc(2026, time.March, 31, 15, 0), []brday.Day{"2026-03-29", "2026-03-31", "2026-03-28"}, 2},
		{utc(2026, time.February, 28, 15, 0), []brday.Day{"2026-02-22", "2026-02-28", "2026-02-21", "2026-03-01"}, 2},
		{utc(2028, time.February, 29, 15, 0), []brday.Day{"2028-02-29", "2028-02-28"}, 1},
	}
	for _, c := range cases {
		fixture := newReviewLedger(t)
		scratch := authoredtest.Open(t, fixtureClock)
		savedReviewBudget(t, scratch)
		for i, day := range c.answered {
			answeredOn(t, scratch, fixture.Path, day, reviewAnswered[i], "Mercado")
		}
		env := newReviewEnv(t, fixture.Path, scratch, c.today)
		if got := env.queueOf("2026-03").Resolved; got != c.want {
			t.Errorf("on %s, answered on %v: resolved = %d, want %d", c.today.Format(time.DateOnly), c.answered, got, c.want)
		}
	}
}

// queueOverTheThirdState adds to the third state a review of each reason the
// rules give, beside the boleto's of an earlier wording: one on a row no
// snapshot lists, one in January, whose budget is left with no line, and two
// answers, one of them in this week.
func queueOverTheThirdState(t *testing.T, scratch *authoredtest.Scratch, full *ledgertest.Fixture) {
	t.Helper()
	authoredtest.PutVersion(t, scratch.Database, utc(2026, time.March, 1, 9, 0), "2026-01")
	rulesLeave(t, scratch, full.Path,
		waitingReview{txPixOut, string(classrules.ReasonUnnamedPerson), "", nil},
		waitingReview{txInstalment, string(classrules.ReasonNewEstablishment), "", nil},
		waitingReview{txForeign, string(classrules.ReasonLowConfidence), "Viagens", authoredtest.Some[int64](58)},
		waitingReview{txUnconverted, string(classrules.ReasonLowConfidence), "Delivery", authoredtest.Some[int64](61)},
		waitingReview{txRefund, string(classrules.ReasonLowConfidence), "Delivery", authoredtest.Some[int64](52)},
		waitingReview{txStale, string(classrules.ReasonUnnamedPerson), "", nil},
	)
	facts := authored.ChangeFacts{Author: authored.AuthorRule, Method: "ingest.run", At: utc(2026, time.March, 11, 9, 0)}
	err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		key, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen("tx-vanished", "2026-03-11", -4_500, "text:BANCA EXEMPLO"))
		if err != nil {
			return err
		}
		return recorder.SetPendingReview(t.Context(), key, string(classrules.ReasonNewEstablishment), nil, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	answeredOn(t, scratch, full.Path, "2026-03-14", txUnconverted, "Delivery")
	answeredOn(t, scratch, full.Path, "2026-03-16", txRefund, "Delivery")
}

// An undo puts the review back, so the transaction leaves the count for being
// pending again; once another write settles it, only the undo keeps the
// answer out of the count.
func TestAnAnswerUndoneCountsNothingOnceAnotherWriteSettlesTheTransaction(t *testing.T) {
	fixture := newReviewLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	savedReviewBudget(t, scratch)
	rulesLeave(t, scratch, fixture.Path, waitingReview{reviewAnswered[0], string(classrules.ReasonNewEstablishment), "", nil})
	env := newReviewEnv(t, fixture.Path, scratch, utc(2026, time.March, 21, 15, 0))
	answeredOn(t, scratch, fixture.Path, "2026-03-16", reviewAnswered[0], "Mercado")
	expectUndone(t, "the undo of the answer", env.undo(lastChangeOf(t, scratch)))
	settledOn(t, scratch, fixture.Path, "2026-03-18", "transactions.setLine", reviewAnswered[0], "Restaurantes")
	if got := env.queueOf("2026-03").Resolved; got != 0 {
		t.Errorf("resolved = %d, want 0", got)
	}
}
