package api

import (
	"math"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

type yieldDayAnswer struct {
	day    string
	amount int64
}

// yieldOfDetail is the yield block of a pot's detail: its total in cents, nil
// when null, and the days behind it.
func yieldOfDetail(t *testing.T, env *readEnv, id string) (total *int64, days []yieldDayAnswer) {
	t.Helper()
	block := answerOf(t, env, "savingsPots.detail", `["`+id+`"]`)["yield"].(map[string]any)
	if figure, ok := block["total"].(float64); ok {
		cents := int64(math.Round(figure * 100))
		total = &cents
	}
	for _, day := range block["days"].([]any) {
		entry := day.(map[string]any)
		days = append(days, yieldDayAnswer{day: entry["day"].(string), amount: int64(math.Round(entry["amount"].(float64) * 100))})
	}
	return total, days
}

func TestAPotsMonthOpensIntoOneRowPerPairOfSightingsOldestFirst(t *testing.T) {
	env := envAt(t, nightlyLedger(t, 6), 2026, time.March, 20)
	total, days := yieldOfDetail(t, env, "reserve:"+accountNubank+"/pot-t")

	want := []string{"2026-03-02", "2026-03-03", "2026-03-04", "2026-03-05", "2026-03-06", "2026-03-15"}
	if len(days) != len(want) {
		t.Fatalf("%d rows, want %d: %v", len(days), len(want), days)
	}
	for index, day := range want {
		if days[index].day != day {
			t.Errorf("row %d is %s, want %s: the rows are not oldest first, one per pair", index, days[index].day, day)
		}
	}
	if total == nil {
		t.Fatal("the month's yield is unknown")
	}
	var sum int64
	for _, day := range days {
		sum += day.amount
	}
	if sum != *total {
		t.Errorf("the rows add up to %d cents, the month says %d", sum, *total)
	}
}

// transferLedger is a pot sighted on the 10th, the 12th and the 15th, with a
// deposit between the first two and a transfer, in the status given, between the last two.
func transferLedger(t *testing.T, status string) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	addBank(f, "item-nu", accountNubank, "Conta Nubank", "260/0001/3-3", 10,
		ledgertest.Reserve{ID: "pot-t", Name: "Trip", Amount: 160, CDIRatio: 1})
	f.ObserveAccount(accountNubank, "2026-03-10", sightedAt("2026-03-10", 10, ledgertest.Reserve{ID: "pot-t", Name: "Trip", Amount: 100, CDIRatio: 1}))
	f.ObserveAccount(accountNubank, "2026-03-12", sightedAt("2026-03-12", 10, ledgertest.Reserve{ID: "pot-t", Name: "Trip", Amount: 150.25, CDIRatio: 1}))
	walkReachesBack(t, f, accountNubank)
	f.AddTransaction(ledgertest.Transaction{ID: "tx-first", AccountID: accountNubank, Date: utc(2026, time.March, 11, 12, 0),
		Day: "2026-03-11", Description: "Dinheiro reservado Trip", Amount: -50, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "TRANSFERENCIA_SALDO_RESERVADO"}})
	f.AddTransaction(ledgertest.Transaction{ID: "tx-second", AccountID: accountNubank, Date: utc(2026, time.March, 13, 12, 0),
		Day: "2026-03-13", Description: "Dinheiro reservado Trip", Amount: -9.75, Currency: "BRL", Type: "DEBIT", Status: status,
		Raw: map[string]any{"operationType": "TRANSFERENCIA_SALDO_RESERVADO"}})
	return f
}

func TestADayWithADepositAndNoYieldIsARowOfZeroNotTheDeposit(t *testing.T) {
	env := envAt(t, transferLedger(t, "POSTED"), 2026, time.March, 20)
	total, days := yieldOfDetail(t, env, "reserve:"+accountNubank+"/pot-t")

	want := []yieldDayAnswer{{"2026-03-12", 25}, {"2026-03-15", 0}}
	if len(days) != 2 || days[0] != want[0] || days[1] != want[1] {
		t.Fatalf("rows = %v, want %v: the first earned 25 cents beside a deposit of 50 and the second only received one", days, want)
	}
	if total == nil || *total != 25 {
		t.Errorf("the month's yield is %v, want 25 cents", total)
	}
}

func TestADayTheLedgerCannotSupportMakesTheMonthUnknownAndIsNotShownAsZero(t *testing.T) {
	env := envAt(t, transferLedger(t, "PENDING"), 2026, time.March, 20)
	total, days := yieldOfDetail(t, env, "reserve:"+accountNubank+"/pot-t")

	if total != nil {
		t.Errorf("the month's yield is %d cents, want unknown: one of its days is", *total)
	}
	if len(days) != 1 || days[0].day != "2026-03-12" {
		t.Errorf("rows = %v, want only the day the ledger can support", days)
	}
}

func TestAPotWithoutAStableIdentityHasNoRowsAndNoYield(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	total, days := yieldOfDetail(t, env, "slot:"+accountNubank+"/1")
	if total != nil || len(days) != 0 {
		t.Errorf("total %v and rows %v, want unknown and none", total, days)
	}
}

func TestEveryKnownMonthIsTheSumOfItsRowsOverTheFullLedger(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	checked := 0
	for id := range potsOf(potListOf(t, env, includeClosed)) {
		total, days := yieldOfDetail(t, env, id)
		if total == nil || len(days) == 0 {
			continue
		}
		checked++
		var sum int64
		for _, day := range days {
			sum += day.amount
		}
		if sum != *total {
			t.Errorf("%s: rows add up to %d, the month says %d", id, sum, *total)
		}
	}
	if checked == 0 {
		t.Error("no pot of the full ledger has rows to add up")
	}
}
