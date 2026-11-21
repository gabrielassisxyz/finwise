package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

const includeClosed = `[{"includeClosed":true}]`

func potListOf(t *testing.T, env *readEnv, arguments string) []map[string]any {
	t.Helper()
	status, body := env.answer("savingsPots.list", arguments)
	if status != http.StatusOK {
		t.Fatalf("savingsPots.list answered %d", status)
	}
	return decodeList(t, body)
}

// potsOf flattens the groups of a list into its pots, by id.
func potsOf(groups []map[string]any) map[string]map[string]any {
	pots := map[string]map[string]any{}
	for _, group := range groups {
		for _, pot := range group["pots"].([]any) {
			pots[pot.(map[string]any)["id"].(string)] = pot.(map[string]any)
		}
	}
	return pots
}

func TestTheListOmitsAClosedPotByDefaultAndIncludesItWhenAsked(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	closedID := "reserve:" + accountNubank + "/" + potNubankOld

	if _, shown := potsOf(potListOf(t, env, `[{"includeClosed":false}]`))[closedID]; shown {
		t.Error("a closed pot is listed by default")
	}
	closed, shown := potsOf(potListOf(t, env, includeClosed))[closedID]
	if !shown || closed["closed"] != true || closed["balance"].(float64) != 0 || closed["monthlyYield"].(float64) != 0 {
		t.Errorf("the closed pot is not listed as closed, holding nothing and earning nothing: %v", closed)
	}
}

func TestEveryPotNamesItsBucketAsTheBankSaysIt(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	pots := potsOf(potListOf(t, env, includeClosed))
	want := map[string]string{
		"reserve:" + accountPots + "/pot-1":              "Reservado",
		"reserve:" + accountNubank + "/" + potNubankMain: "Reservado",
		"slot:" + accountNubank + "/1":                   "Reservado",
		"investment:" + investmentNubank:                 "Investido",
		"investment:" + investmentItau:                   "Investido",
	}
	for id, bucket := range want {
		if pots[id] == nil || pots[id]["bucket"] != bucket {
			t.Errorf("%s has the bucket %v, want %s", id, pots[id]["bucket"], bucket)
		}
		detail := answerOf(t, env, "savingsPots.detail", `["`+id+`"]`)
		if detail["bucket"] != bucket {
			t.Errorf("the detail of %s names the bucket %v, want %s", id, detail["bucket"], bucket)
		}
	}
	for _, group := range potListOf(t, env, includeClosed) {
		if group["bucket"] == nil {
			t.Error("a group of pots has no bucket, so its pots would count nowhere")
		}
	}
}

func TestAPotWithoutAnIdentityHasNoYieldRatherThanAZero(t *testing.T) {
	pots := potsOf(potListOf(t, newReadEnv(t, newFullLedger(t)), includeClosed))
	if slot := pots["slot:"+accountNubank+"/1"]; slot["monthlyYield"] != nil {
		t.Errorf("a pot that cannot be followed has the yield %v", slot["monthlyYield"])
	}
	if investment := pots["investment:"+investmentNubank]; investment["monthlyYield"] != nil {
		t.Errorf("an investment has the yield %v with no statement behind it", investment["monthlyYield"])
	}
}

// pendingTransferLedger is a Mercado Pago account sighted on 10 and 15 March,
// with a transfer into the pot between them that the bank has not settled: the
// statement cannot say what the pot yielded.
func pendingTransferLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	addBank(f, "item-mp", accountPots, "Mercado Pago", "323/0001/2-2", 10)
	f.ObserveAccount(accountPots, "2026-03-10", sightedAt("2026-03-10", 10,
		ledgertest.Reserve{ID: "pot-t", Name: "Trip", Amount: 100, CDIRatio: 1}))
	walkReachesBack(t, f, accountPots)
	f.AddTransaction(ledgertest.Transaction{ID: "tx-pending-transfer", AccountID: accountPots, Status: "PENDING",
		Date: utc(2026, time.March, 12, 12, 0), Description: "Dinheiro reservado Trip", Amount: -50, Currency: "BRL",
		Type: "DEBIT", Raw: map[string]any{"operationType": "TRANSFERENCIA_SALDO_RESERVADO"}})
	return f
}

func addBank(f *ledgertest.Fixture, itemID, accountID, name, transfer string, balance float64, reserves ...ledgertest.Reserve) {
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: itemID, Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	if reserves == nil {
		reserves = []ledgertest.Reserve{{ID: "pot-t", Name: "Trip", Amount: 150, CDIRatio: 1}}
	}
	f.AddAccount(ledgertest.Account{ID: accountID, ItemID: itemID, Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: name, Currency: "BRL", Balance: balance, TransferNumber: transfer, Reserves: reserves})
}

func TestAMonthWithAYieldTheQueryCannotComputeIsUnknownNotZero(t *testing.T) {
	env := envAt(t, pendingTransferLedger(t), 2026, time.March, 20)
	pot := potsOf(potListOf(t, env, includeClosed))["reserve:"+accountPots+"/pot-t"]
	if pot == nil || pot["monthlyYield"] != nil {
		t.Fatalf("the pot's yield is %v, want null", pot["monthlyYield"])
	}
	detail := answerOf(t, env, "savingsPots.detail", `["reserve:`+accountPots+`/pot-t"]`)
	if detail["yield"].(map[string]any)["total"] != nil {
		t.Errorf("the detail's yield total is %v, want null", detail["yield"].(map[string]any)["total"])
	}
}

func TestAMovementNoPotCanClaimIsOnItsAccountAndTheOneThatMatchesIsOnThePot(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))

	pot := answerOf(t, env, "savingsPots.detail", `["reserve:`+accountNubank+`/`+potNubankMain+`"]`)
	onPot := pot["movements"].([]any)
	if len(onPot) != 1 || onPot[0].(map[string]any)["amount"].(float64) != 50 || onPot[0].(map[string]any)["description"] != "Depósito" {
		t.Errorf("the deposit that moved the pot by its amount: %v", onPot)
	}
	account := answerOf(t, env, "savingsPots.detail", `["`+accountNubank+`"]`)
	unclaimed := account["movements"].([]any)
	if len(unclaimed) != 1 || unclaimed[0].(map[string]any)["amount"].(float64) != 75 {
		t.Errorf("the deposit no pot can claim: %v", unclaimed)
	}
	if account["bucket"] != "Disponível" || account["balance"].(float64) != 500 {
		t.Errorf("the account's side is %v holding %v", account["bucket"], account["balance"])
	}
}

func TestAMovementThatOnlyAPotWithNoIdentityCouldHaveReceivedStaysOnItsAccount(t *testing.T) {
	f := ledgertest.New(t)
	addBank(f, "item-nu", accountNubank, "Conta Nubank", "260/0001/3-3", 10, ledgertest.Reserve{Name: "Sem identidade", Amount: 150, CDIRatio: 1})
	f.ObserveAccount(accountNubank, "2026-03-10", sightedAt("2026-03-10", 10, ledgertest.Reserve{Name: "Sem identidade", Amount: 100, CDIRatio: 1}))
	walkReachesBack(t, f, accountNubank)
	addDeposit(f, "tx-deposit", 50)
	env := envAt(t, f, 2026, time.March, 20)

	account := answerOf(t, env, "savingsPots.detail", `["`+accountNubank+`"]`)
	if movements := account["movements"].([]any); len(movements) != 1 || movements[0].(map[string]any)["amount"].(float64) != 50 {
		t.Errorf("the deposit read as %v on the account", movements)
	}
	if slot := answerOf(t, env, "savingsPots.detail", `["slot:`+accountNubank+`/1"]`); len(slot["movements"].([]any)) != 0 {
		t.Errorf("a pot that cannot be followed was given the deposit: %v", slot["movements"])
	}
}

func TestAnAccountWithNothingUnclaimedHasNoPageOfItsOwn(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	status, body := env.answer("savingsPots.detail", `["`+accountItau+`"]`)
	if status != http.StatusOK || string(body) != "null\n" && string(body) != "null" {
		t.Errorf("status %d body %q, want null", status, body)
	}
}

// emptiedLedger is an account that held a pot on 10 March and none since.
func emptiedLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	addBank(f, "item-mp", accountPots, "Mercado Pago", "323/0001/2-2", 10, []ledgertest.Reserve{}...)
	f.AddAccount(ledgertest.Account{ID: "acc-emptied", ItemID: "item-mp", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Mercado Pago", Currency: "BRL", Balance: 5, TransferNumber: "323/0001/9-9", Reserves: []ledgertest.Reserve{}})
	f.ObserveAccount("acc-emptied", "2026-03-10", sightedAt("2026-03-10", 5,
		ledgertest.Reserve{ID: "pot-gone", Name: "Old", Amount: 80, CDIRatio: 1}))
	return f
}

func TestAnAccountThatEmptiedEveryPotShowsThemClosed(t *testing.T) {
	env := envAt(t, emptiedLedger(t), 2026, time.March, 20)

	if len(potsOf(potListOf(t, env, `[{"includeClosed":false}]`))) != 0 {
		t.Error("a pot of an account that emptied every pot is listed as open")
	}
	pot := potsOf(potListOf(t, env, includeClosed))["reserve:acc-emptied/pot-gone"]
	if pot == nil || pot["closed"] != true || pot["balance"].(float64) != 0 {
		t.Errorf("the pot reads as %v, want closed with nothing in it", pot)
	}
}

func TestAnInvestmentIsShownAtItsLatestNetAmountWithItsOwnFigures(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	detail := answerOf(t, env, "savingsPots.detail", `["investment:`+investmentNubank+`"]`)
	investment := detail["investment"].(map[string]any)
	want := map[string]float64{"applied": 2800, "gross": 3100, "tax": 100, "net": 3000}
	for field, amount := range want {
		if investment[field].(float64) != amount {
			t.Errorf("%s is %v, want %v", field, investment[field], amount)
		}
	}
	if investment["redeemableOn"] != "2028-03-15" || detail["balance"].(float64) != 3000 || detail["cdiPercent"].(float64) != 115 {
		t.Errorf("redeemable %v balance %v rate %v", investment["redeemableOn"], detail["balance"], detail["cdiPercent"])
	}
	if reserve := answerOf(t, env, "savingsPots.detail", `["reserve:`+accountPots+`/pot-1"]`); reserve["investment"] != nil {
		t.Error("a reserve entry carries an investment's figures")
	}
}

func TestADetailOfAnIdNoPotHasIsNull(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, id := range []string{"no-such-pot", "reserve:acc-pots/nothing", "investment:nothing"} {
		status, body := env.answer("savingsPots.detail", `["`+id+`"]`)
		if status != http.StatusOK || string(body) != "null\n" && string(body) != "null" {
			t.Errorf("%s: status %d body %q, want null", id, status, body)
		}
	}
}

func TestAListWithoutItsOptionIsRefused(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, arguments := range []string{`[{}]`, `[{"includeClosed":"yes"}]`, `[{"includeClosed":true,"more":1}]`} {
		if status, _ := env.answer("savingsPots.list", arguments); status != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400", arguments, status)
		}
	}
}

// nightlyLedger is a Nubank account sighted on each night from the 1st of
// March, so that the number of sightings in the month can be varied.
func nightlyLedger(t *testing.T, nights int) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	addBank(f, "item-nu", accountNubank, "Conta Nubank", "260/0001/3-3", 10)
	for night := 1; night <= nights; night++ {
		day := time.Date(2026, time.March, night, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		f.ObserveAccount(accountNubank, day, sightedAt(day, 10,
			ledgertest.Reserve{ID: "pot-t", Name: "Trip", Amount: 100 + float64(night)/10, CDIRatio: 1}))
	}
	walkReachesBack(t, f, accountNubank)
	return f
}

func decodeList(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var list []map[string]any
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("the answer is not a list of objects: %v", err)
	}
	return list
}
