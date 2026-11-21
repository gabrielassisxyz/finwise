package ledger_test

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func sightingOn(t *testing.T, history ledger.YieldHistory, day string) ledger.YieldSighting {
	t.Helper()
	for _, sighting := range history.Sightings {
		if sighting.AccountID == "acc-1" && string(sighting.Day) == day {
			return sighting
		}
	}
	t.Fatalf("no sighting of acc-1 on %s among %d", day, len(history.Sightings))
	return ledger.YieldSighting{}
}

func TestYieldHistoryHoldsEverySightingWithItsEntriesAndTheCheckedAmounts(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}, {Name: "Loose", Amount: 7, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 150.30, CDIRatio: 1}})
	history, err := ledger.New(f.Path).YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	first := sightingOn(t, history, string(yieldFrom))
	if first.UpdatedAt != "2026-03-10 02:00:00" || first.RunID != 1 || len(first.Entries) != 2 {
		t.Fatalf("first sighting = %+v, want the provider's time as the statement compares it and two entries", first)
	}
	stable, loose := first.Entries[0], first.Entries[1]
	if !stable.Stable || !stable.BRL || !stable.AmountOK || stable.Cents == nil || *stable.Cents != 10000 {
		t.Errorf("the pot with an id reads as %+v", stable)
	}
	if loose.Stable || loose.ReserveID != nil || loose.Cents == nil || *loose.Cents != 700 {
		t.Errorf("the pot with no id reads as %+v, want unstable with its amount", loose)
	}
}

func TestYieldHistoryLeavesOutASightingThatReportsNoListOfReserves(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 101, CDIRatio: 1}})
	f.ObserveAccount("acc-1", "2026-03-12", map[string]any{"bankData": map[string]any{}})

	history, err := ledger.New(f.Path).YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Sightings) != 2 {
		t.Fatalf("%d sightings, want the two that list their reserves", len(history.Sightings))
	}
}

func TestYieldHistoryFlagsAnAmountTheStatementWouldNotTrust(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 101, CDIRatio: 1}})
	f.ObserveAccount("acc-1", "2026-03-12", map[string]any{
		"updatedAt": "2026-03-12T02:00:00.000Z",
		"bankData": map[string]any{"reservedBalances": []any{
			map[string]any{"identification": "pot-a", "name": "Trip", "availableAmounts": []any{
				map[string]any{"currencyCode": "USD", "amount": 10.005}}},
		}},
	})
	history, err := ledger.New(f.Path).YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	entry := sightingOn(t, history, "2026-03-12").Entries[0]
	if entry.BRL || entry.AmountOK {
		t.Errorf("a dollar amount of three decimals reads as %+v, want neither BRL nor readable", entry)
	}
}

func TestYieldHistoryHoldsEveryVersionOfATransferAndOnlyTransfers(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 150, CDIRatio: 1}})
	walkReachesBack(t, f)
	addReserveTransfer(t, f, "tx-dep", "Dinheiro reservado Trip", -50)
	f.ObserveTransaction("tx-dep", "2026-03-16", map[string]any{"amount": -40.0})
	f.AddTransaction(ledgertest.Transaction{ID: "tx-coffee", AccountID: "acc-1", Date: instant(t, transferMoment),
		Description: "Coffee", Amount: -3, Currency: "BRL", Type: "DEBIT"})
	f.RecordEvent("transactions", "tx-dep", 1, "removed", "2026-03-17")

	history, err := ledger.New(f.Path).YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Transfers) != 2 || len(history.Removals) != 1 || len(history.Walks) != 1 {
		t.Fatalf("%d transfer versions, %d removals, %d walks, want 2, 1 and 1", len(history.Transfers), len(history.Removals), len(history.Walks))
	}
	older, newer := history.Transfers[0], history.Transfers[1]
	if older.ObservationID >= newer.ObservationID || older.Cents == nil || *older.Cents != -5000 || *newer.Cents != -4000 {
		t.Errorf("versions read as %+v then %+v, want the amounts -5000 then -4000 oldest first", older, newer)
	}
	if !older.IsReserve || older.NotPosted || older.Fault != nil || older.Name == nil || *older.Name != "Trip" {
		t.Errorf("the transfer reads as %+v, want a posted one naming Trip with no fault", older)
	}
}
