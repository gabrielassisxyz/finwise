package api

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func init() {
	readCases = append(readCases,
		readCase{"review.closing", `["2026-03"]`},
		readCase{"review.closing", `["2025-12"]`},
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
	return `{"month":"` + month + `","closed":false,"closedOn":null,"steps":` + emptyClosingSteps +
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
	for _, step := range freshClosingSteps() {
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
