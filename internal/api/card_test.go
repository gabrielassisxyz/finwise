package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// envAt serves the fixture with the server's clock at the given noon in Brazil.
func envAt(t *testing.T, fixture *ledgertest.Fixture, year int, month time.Month, day int) *readEnv {
	t.Helper()
	env := newTestEnv(t, withLedger(fixture.Path, utc(year, month, day, 15, 0)))
	return &readEnv{testEnv: env, cookie: env.unlock()}
}

func answerOf(t *testing.T, env *readEnv, method, arguments string) map[string]any {
	t.Helper()
	status, body := env.answer(method, arguments)
	if status != http.StatusOK {
		t.Fatalf("%s answered %d", method, status)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("%s did not answer an object: %v", method, err)
	}
	return decoded
}

// c6Ledger holds a C6 card whose latest bill carries no closing date, so the
// closing is predicted from the bank's table: the bill due Tuesday 10 March
// closes on 4 March, and the cycle after it is due on 10 April and closes on 3
// April. Purchases are added by the caller.
func c6Ledger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-c6", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddAccount(ledgertest.Account{ID: "acc-c6", ItemID: "item-c6", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta C6", Currency: "BRL", Balance: 100, TransferNumber: "336/0001/55555-5"})
	f.AddAccount(ledgertest.Account{ID: "card-c6", ItemID: "item-c6", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "C6 Carbon", Currency: "BRL", Balance: 800,
		Credit: map[string]any{"creditLimit": 9000, "availableCreditLimit": 8200}})
	f.AddBill(ledgertest.Bill{ID: "bill-jan", AccountID: "card-c6", DueDate: "2026-02-10", ClosingDate: "2026-02-04", Total: 640.5})
	f.AddBill(ledgertest.Bill{ID: "bill-feb", AccountID: "card-c6", DueDate: "2026-03-10", Total: 910})
	return f
}

func addCardPurchase(f *ledgertest.Fixture, id string, at time.Time, amount float64, billID string) {
	addPurchaseOn(f, "card-c6", id, at, amount, billID)
}

func addPurchaseOn(f *ledgertest.Fixture, accountID, id string, at time.Time, amount float64, billID string) {
	metadata := map[string]any{"cardNumber": "5555666677778888"}
	if billID != "" {
		metadata["billId"] = billID
	}
	f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: accountID, Date: at, Description: "COMPRA " + id,
		Amount: amount, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": metadata}})
}

func purchaseIDs(t *testing.T, purchases any) map[string]bool {
	t.Helper()
	ids := map[string]bool{}
	for _, purchase := range purchases.([]any) {
		ids[purchase.(map[string]any)["transactionId"].(string)] = true
	}
	return ids
}

func TestThePurchaseOnTheClosingDateFallsOnTheNextBillAndOneOfTheClosedBillDoesNot(t *testing.T) {
	f := c6Ledger(t)
	addCardPurchase(f, "tx-on-closing", utc(2026, time.March, 4, 15, 0), 50, "")
	addCardPurchase(f, "tx-day-before-closed-bill", utc(2026, time.March, 3, 15, 0), 40, "bill-feb")
	addCardPurchase(f, "tx-day-before-no-bill", utc(2026, time.March, 3, 16, 0), 30, "")
	addCardPurchase(f, "tx-before-slip-window", utc(2026, time.March, 1, 15, 0), 70, "")
	addCardPurchase(f, "tx-later", utc(2026, time.March, 12, 15, 0), 20, "")

	overview := answerOf(t, envAt(t, f, 2026, time.March, 20), "card.overview", `[]`)

	payAhead := overview["payAhead"].(map[string]any)
	ids := purchaseIDs(t, payAhead["purchases"])
	if !ids["tx-on-closing"] {
		t.Error("the purchase dated on the closing date is not among those that fall on the next bill")
	}
	if ids["tx-day-before-closed-bill"] {
		t.Error("a purchase of the closed bill's own id fell on the next bill")
	}
	if !ids["tx-day-before-no-bill"] || !ids["tx-later"] || ids["tx-before-slip-window"] || len(ids) != 3 {
		t.Errorf("purchases on the next bill: %v", ids)
	}
	if payAhead["amount"].(float64) != 100 {
		t.Errorf("the next bill's amount is %v, want 100", payAhead["amount"])
	}
}

func TestTheOpenBillClosesWhereTheBanksTableSaysAndTheBestDayIsTheClosing(t *testing.T) {
	f := c6Ledger(t)
	overview := answerOf(t, envAt(t, f, 2026, time.March, 20), "card.overview", `[]`)
	for field, want := range map[string]string{
		"closesOn": "2026-04-03", "dueOn": "2026-04-10", "bestPurchaseDay": "2026-04-03", "title": "C6 Carbon",
	} {
		if overview[field] != want {
			t.Errorf("%s is %v, want %s", field, overview[field], want)
		}
	}
	if overview["openBill"].(float64) != 800 || overview["limit"].(float64) != 9000 || overview["availableLimit"].(float64) != 8200 {
		t.Errorf("open bill, limit and what is left of it: %v %v %v", overview["openBill"], overview["limit"], overview["availableLimit"])
	}
	bills := overview["bills"].([]any)
	if len(bills) != 2 || bills[0].(map[string]any)["month"] != "2026-03" || bills[1].(map[string]any)["month"] != "2026-02" {
		t.Errorf("past bills are not the two ledger bills newest first: %v", bills)
	}
	if bills[0].(map[string]any)["closedOn"] != "2026-03-04" {
		t.Errorf("the closing of a bill the ledger gives none for is %v, want the table's 2026-03-04", bills[0].(map[string]any)["closedOn"])
	}
}

func TestPurchasesOfTheTwoDaysBeforeTheClosingMayFallOnEitherBill(t *testing.T) {
	f := c6Ledger(t)
	addCardPurchase(f, "tx-two-days-before", utc(2026, time.April, 1, 15, 0), 10, "")
	addCardPurchase(f, "tx-day-before", utc(2026, time.April, 2, 15, 0), 11, "")
	addCardPurchase(f, "tx-three-days-before", utc(2026, time.March, 31, 15, 0), 12, "")

	overview := answerOf(t, envAt(t, f, 2026, time.April, 2), "card.overview", `[]`)

	ids := purchaseIDs(t, overview["mayFallOnNextBill"])
	if len(ids) != 2 || !ids["tx-two-days-before"] || !ids["tx-day-before"] {
		t.Errorf("purchases that may fall on either bill: %v", ids)
	}
	if overview["payAhead"].(map[string]any)["amount"].(float64) != 0 {
		t.Error("before the closing day nothing is on the next bill yet")
	}
}

func TestBillItemsAreTheCardRowsTheBankPutOnThatBill(t *testing.T) {
	f := c6Ledger(t)
	addCardPurchase(f, "tx-on-feb", utc(2026, time.February, 20, 15, 0), 33.3, "bill-feb")
	addCardPurchase(f, "tx-on-jan", utc(2026, time.January, 20, 15, 0), 44.4, "bill-jan")

	overview := answerOf(t, envAt(t, f, 2026, time.March, 20), "card.overview", `[]`)

	bills := overview["bills"].([]any)
	feb := bills[0].(map[string]any)["items"].([]any)
	if len(feb) != 1 || feb[0].(map[string]any)["amount"].(float64) != 33.3 || feb[0].(map[string]any)["day"] != "2026-02-20" {
		t.Errorf("items of the bill due in March: %v", feb)
	}
	if bills[0].(map[string]any)["total"].(float64) != 910 {
		t.Errorf("a bill's total is the bank's own: %v", bills[0].(map[string]any)["total"])
	}
}

// noTableLedger holds an Itaú card, for which there is no closing table, whose
// only bill carries no closing date.
func noTableLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-it", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddAccount(ledgertest.Account{ID: "acc-it", ItemID: "item-it", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Corrente", Currency: "BRL", Balance: 100, TransferNumber: "341/0001/1-1"})
	f.AddAccount(ledgertest.Account{ID: "card-it", ItemID: "item-it", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Visa Exemplo", Currency: "BRL", Balance: 300,
		Credit: map[string]any{"creditLimit": 4000, "availableCreditLimit": 3700}})
	f.AddBill(ledgertest.Bill{ID: "bill-it", AccountID: "card-it", DueDate: "2026-03-25", Total: 300})
	f.AddTransaction(ledgertest.Transaction{ID: "tx-it", AccountID: "card-it", Date: utc(2026, time.March, 18, 15, 0),
		Description: "COMPRA EXEMPLO", Amount: 60, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "1111222233334444"}}})
	return f
}

func TestACardOfABankWithNoClosingTableHasNoClosingDateAndNoNextBillAmount(t *testing.T) {
	f := noTableLedger(t)

	overview := answerOf(t, envAt(t, f, 2026, time.March, 20), "card.overview", `[]`)
	for _, field := range []string{"closesOn", "dueOn", "bestPurchaseDay"} {
		if overview[field] != "" {
			t.Errorf("%s is %v for a card whose closing is unknown, want the empty string and never a date", field, overview[field])
		}
	}
	card, err := loadCard(withNow(t.Context(), func() time.Time { return utc(2026, time.March, 20, 15, 0) }), ledger.New(f.Path))
	if err != nil || card == nil {
		t.Fatalf("%v, %v", card, err)
	}
	if card.nextBill(brday.Month("2026-03")).Known {
		t.Error("the next bill's amount is known for a card with no closing date")
	}
}

func cardLedgerWith(t *testing.T, firstID, firstDue, secondID, secondDue string) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	for _, card := range []struct{ id, due, name string }{{firstID, firstDue, "Cartão " + firstID}, {secondID, secondDue, "Cartão " + secondID}} {
		f.AddAccount(ledgertest.Account{ID: card.id, ItemID: "item-x", Type: "CREDIT", Subtype: "CREDIT_CARD",
			Name: card.name, Currency: "BRL", Balance: 10})
		f.AddBill(ledgertest.Bill{ID: "bill-" + card.id, AccountID: card.id, DueDate: card.due, ClosingDate: "2026-03-01", Total: 10})
	}
	return f
}

func TestWithTwoCardsTheOneWithTheLaterBillIsDescribed(t *testing.T) {
	for label, arrangement := range map[string][4]string{
		"the later bill on the smaller id": {"card-a", "2026-05-05", "card-b", "2026-04-05"},
		"the later bill on the larger id":  {"card-a", "2026-04-05", "card-b", "2026-05-05"},
	} {
		f := cardLedgerWith(t, arrangement[0], arrangement[1], arrangement[2], arrangement[3])
		overview := answerOf(t, envAt(t, f, 2026, time.March, 20), "card.overview", `[]`)
		want := "Cartão card-a"
		if arrangement[1] < arrangement[3] {
			want = "Cartão card-b"
		}
		if overview["title"] != want {
			t.Errorf("%s: described %v, want %s", label, overview["title"], want)
		}
	}
}

func TestACardWithoutBillsRanksBelowOneWithABill(t *testing.T) {
	accounts := []ledger.Account{
		{ID: "card-1", Type: ledger.AccountCredit}, {ID: "card-2", Type: ledger.AccountCredit}, {ID: "acc", Type: ledger.AccountBank},
	}
	chosen, found := theCard(accounts, []ledger.Bill{{ID: "b", AccountID: "card-2", DueDate: "2026-01-01"}}, nil)
	if !found || chosen.ID != "card-2" {
		t.Errorf("chose %q (%v)", chosen.ID, found)
	}
	if _, found := theCard(accounts[2:], nil, nil); found {
		t.Error("a bank account was chosen as the card")
	}
}

func TestOverTheEmptyLedgerTheCardIsNull(t *testing.T) {
	env := newReadEnv(t, newEmptyLedger(t))
	status, body := env.answer("card.overview", `[]`)
	if status != http.StatusOK || string(body) != "null\n" && string(body) != "null" {
		t.Errorf("status %d body %q, want null", status, body)
	}
}

func ledgerOf(f *ledgertest.Fixture) *ledger.Reader { return ledger.New(f.Path) }

func titleOfTheCard(t *testing.T, f *ledgertest.Fixture) any {
	t.Helper()
	title, _ := answerOf(t, envAt(t, f, 2026, time.March, 20), "card.overview", `[]`)["title"].(string)
	name, _, _ := strings.Cut(title, " · ")
	return name
}

func TestTheCardWithTheNewerPurchaseWinsOverTheOneWithTheLaterBill(t *testing.T) {
	f := cardLedgerWith(t, "card-a", "2026-05-05", "card-b", "2026-04-05")
	addPurchaseOn(f, "card-a", "tx-a", utc(2026, time.March, 1, 15, 0), 10, "")
	addPurchaseOn(f, "card-b", "tx-b", utc(2026, time.March, 10, 15, 0), 10, "")
	if title := titleOfTheCard(t, f); title != "Cartão card-b" {
		t.Errorf("described %v, want the card with the newer purchase", title)
	}
}

func TestWithNoPurchaseAtAllTheLaterBillWins(t *testing.T) {
	if title := titleOfTheCard(t, cardLedgerWith(t, "card-a", "2026-05-05", "card-b", "2026-04-05")); title != "Cartão card-a" {
		t.Errorf("described %v, want the card with the later bill", title)
	}
}

func TestAPurchaseOlderThanTheWindowDoesNotChooseTheCard(t *testing.T) {
	f := cardLedgerWith(t, "card-a", "2026-04-05", "card-b", "2026-05-05")
	addPurchaseOn(f, "card-a", "tx-old", utc(2025, time.October, 1, 15, 0), 10, "")
	if title := titleOfTheCard(t, f); title != "Cartão card-b" {
		t.Errorf("described %v, want the later bill when the only purchase is outside the window", title)
	}
}

func TestTheTieRulesOfTheCardInUse(t *testing.T) {
	sameDay := utc(2026, time.March, 10, 15, 0)
	f := cardLedgerWith(t, "card-a", "2026-04-05", "card-b", "2026-05-05")
	addPurchaseOn(f, "card-a", "tx-a", sameDay, 10, "")
	addPurchaseOn(f, "card-b", "tx-b", sameDay, 10, "")
	if title := titleOfTheCard(t, f); title != "Cartão card-b" {
		t.Errorf("a purchase on the same day: described %v, want the later bill", title)
	}
	f = cardLedgerWith(t, "card-a", "2026-04-05", "card-b", "2026-04-05")
	addPurchaseOn(f, "card-a", "tx-a", sameDay, 10, "")
	addPurchaseOn(f, "card-b", "tx-b", sameDay, 10, "")
	if title := titleOfTheCard(t, f); title != "Cartão card-a" {
		t.Errorf("a purchase and a bill on the same days: described %v, want the lower id", title)
	}
}

func TestOnlyMoneyOutOfTheCardCountsAsAPurchaseForTheChoice(t *testing.T) {
	f := cardLedgerWith(t, "card-a", "2026-05-05", "card-b", "2026-04-05")
	f.AddTransaction(ledgertest.Transaction{ID: "tx-refund", AccountID: "card-b", Date: utc(2026, time.March, 12, 15, 0),
		Description: "ESTORNO", Amount: 10, Currency: "BRL", Type: "CREDIT",
		Raw: map[string]any{"operationType": "ESTORNO", "creditCardMetadata": map[string]any{"cardNumber": "1"}}})
	if title := titleOfTheCard(t, f); title != "Cartão card-a" {
		t.Errorf("described %v, a refund must not make a card the one in use", title)
	}
}
