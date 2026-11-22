package ledger_test

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// historyLedger is one account sighted on three days: with two pots, with one,
// and then with none at all.
func historyLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	f.AddItem(ledgertest.Item{
		ID: "item-1", Connector: "Test Bank", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: instant(t, "2026-03-15T02:00:00Z"), NextAutoSync: instant(t, "2026-03-16T02:00:00Z"),
	})
	f.AddAccount(ledgertest.Account{
		ID: "acc-1", ItemID: "item-1", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Test Checking", Currency: "BRL", Balance: 30, Reserves: []ledgertest.Reserve{},
	})
	f.ObserveAccount("acc-1", "2026-03-10", merged(
		ledgertest.ReservesOverride(
			ledgertest.Reserve{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1},
			ledgertest.Reserve{ID: "pot-b", Name: "Car", Amount: 50.5, CDIRatio: 1.1}),
		map[string]any{"balance": 10.0}))
	f.ObserveAccount("acc-1", "2026-03-12", merged(
		ledgertest.ReservesOverride(ledgertest.Reserve{ID: "pot-a", Name: "Trip", Amount: 101, CDIRatio: 1}),
		map[string]any{"balance": 20.0}))
	return f
}

func merged(base, extra map[string]any) map[string]any {
	for key, value := range extra {
		base[key] = value
	}
	return base
}

func TestBalanceHistoryHasOneRowPerSightingIncludingOneWithNoPot(t *testing.T) {
	days, err := viewOf(t, historyLedger(t).Path).BalanceHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"2026-03-10": 1000, "2026-03-12": 2000, "2026-03-15": 3000}
	if len(days) != len(want) {
		t.Fatalf("%d sightings, want %d", len(days), len(want))
	}
	for _, day := range days {
		if day.AccountID != "acc-1" || day.Balance == nil || int64(*day.Balance) != want[string(day.Day)] {
			t.Errorf("sighting %+v is not one of %v", day, want)
		}
	}
	for index := 1; index < len(days); index++ {
		if days[index-1].Day >= days[index].Day {
			t.Errorf("sightings are not in day order: %v then %v", days[index-1].Day, days[index].Day)
		}
	}
}

func TestReserveHistoryHoldsEveryEntryOfEveryDayAndNothingForAnEmptySighting(t *testing.T) {
	pots, err := viewOf(t, historyLedger(t).Path).ReserveHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	perDay := map[string]int{}
	for _, pot := range pots {
		perDay[string(pot.Day)]++
		if !pot.StableIdentity || pot.ReserveID == nil || pot.Amount == nil || pot.AmountStatus != "ok" {
			t.Errorf("entry %+v is not a readable stable pot", pot)
		}
	}
	want := map[string]int{"2026-03-10": 2, "2026-03-12": 1}
	if len(perDay) != len(want) || perDay["2026-03-10"] != 2 || perDay["2026-03-12"] != 1 {
		t.Errorf("entries per day %v, want %v and none for the day with no pot", perDay, want)
	}
}

func TestInvestmentPrincipalsAreTheOriginalAmountsOfCurrentInvestments(t *testing.T) {
	f := ledgertest.New(t)
	f.AddInvestment(ledgertest.Investment{ID: "inv-1", ItemID: "item-1", Type: "FIXED_INCOME", Name: "One",
		Currency: "BRL", Balance: 120, Raw: map[string]any{"amountOriginal": 100.25}})
	f.AddInvestment(ledgertest.Investment{ID: "inv-2", ItemID: "item-1", Type: "FIXED_INCOME", Name: "Two",
		Currency: "BRL", Balance: 5, Raw: map[string]any{"amountOriginal": nil}})
	f.AddInvestment(ledgertest.Investment{ID: "inv-gone", ItemID: "item-1", Type: "FIXED_INCOME", Name: "Gone",
		Currency: "BRL", Balance: 1})
	f.MarkRemoved("investments", "inv-gone")

	principals, err := viewOf(t, f.Path).InvestmentPrincipals(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(principals) != 1 || principals["inv-1"] != 10025 {
		t.Errorf("principals %v, want only inv-1 at 10025", principals)
	}
}

func TestABillWithNoClosingDateReadsWithNoClosingDate(t *testing.T) {
	f := ledgertest.New(t)
	f.AddBill(ledgertest.Bill{ID: "bill-open", AccountID: "acc-card", DueDate: "2026-04-10", Total: 10})
	f.AddBill(ledgertest.Bill{ID: "bill-closed", AccountID: "acc-card", DueDate: "2026-03-10", ClosingDate: "2026-03-04", Total: 20})

	bills, err := viewOf(t, f.Path).Bills(t.Context())
	if err != nil || len(bills) != 2 {
		t.Fatalf("%v, %v", bills, err)
	}
	if bills[0].ID != "bill-closed" || bills[0].ClosingDate == nil || *bills[0].ClosingDate != "2026-03-04" {
		t.Errorf("the dated bill read as %+v", bills[0])
	}
	if bills[1].ID != "bill-open" || bills[1].ClosingDate != nil {
		t.Errorf("the undated bill read as %+v, want no closing date", bills[1])
	}
}
