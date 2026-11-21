package cardrules_test

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/cardrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// placementsWithForecast builds an invented card ledger in which every purchase
// carries the given billForecastDate, and returns the placement of each
// purchase through the real reader.
func placementsWithForecast(t *testing.T, forecast any) map[string]cardrules.Placement {
	t.Helper()
	f := ledgertest.New(t)
	f.AddItem(ledgertest.Item{
		ID: "item-1", Connector: "Test Bank", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: mustInstant(t, "2026-03-15T02:00:00Z"), NextAutoSync: mustInstant(t, "2026-03-16T02:00:00Z"),
	})
	f.AddAccount(ledgertest.Account{
		ID: "card-1", ItemID: "item-1", Type: "CREDIT", Subtype: "CREDIT_CARD", Name: "Test Card", Currency: "BRL",
	})
	f.AddBill(ledgertest.Bill{ID: "bill-feb", AccountID: "card-1", DueDate: "2026-02-12", ClosingDate: "2026-02-06", Total: 10})
	f.AddBill(ledgertest.Bill{ID: "bill-mar", AccountID: "card-1", DueDate: "2026-03-12", ClosingDate: "2026-03-06", Total: 20})
	for id, moment := range map[string]string{
		// 22:00 on the 5th in Brazil, already the 6th in UTC.
		"before-closing": "2026-03-06T01:00:00Z",
		"on-closing":     "2026-03-06T13:00:00Z",
		"early-in-cycle": "2026-02-03T15:00:00Z",
	} {
		metadata := map[string]any{"cardNumber": "0000"}
		if forecast != nil {
			metadata["billForecastDate"] = forecast
		}
		f.AddTransaction(ledgertest.Transaction{
			ID: id, AccountID: "card-1", Date: mustInstant(t, moment), Description: "Test purchase",
			Amount: 12.5, Currency: "BRL", Type: "DEBIT",
			Raw: map[string]any{"creditCardMetadata": metadata},
		})
	}

	reader := ledger.New(f.Path)
	bills, err := reader.Bills(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	transactions, err := reader.CleanTransactions(t.Context(), brday.Period{First: "2026-01-01", Last: "2026-12-31"})
	if err != nil {
		t.Fatal(err)
	}
	card := cardrules.Card{Bills: cardBillsOf(bills), Table: cardrules.C6ClosingTable()}
	placements := map[string]cardrules.Placement{}
	for _, transaction := range transactions {
		placements[transaction.ID] = card.PlacePurchase(purchaseOf(transaction))
	}
	return placements
}

func TestABillForecastDateThatContradictsTheTableChangesNoResult(t *testing.T) {
	without := placementsWithForecast(t, nil)
	if len(without) != 3 {
		t.Fatalf("read %d purchases, want 3", len(without))
	}
	want := map[string]string{"early-in-cycle": "bill-feb", "before-closing": "bill-mar", "on-closing": ""}
	for id, bill := range want {
		if without[id].BillID != bill {
			t.Fatalf("%s: placed on %q, want %q (a baseline the forecast must not move)", id, without[id].BillID, bill)
		}
	}

	for _, forecast := range []string{"2026-07", "2025-01", "2026-03-30"} {
		with := placementsWithForecast(t, forecast)
		for id, placement := range without {
			if with[id] != placement {
				t.Errorf("forecast %s moved %s from %+v to %+v", forecast, id, placement, with[id])
			}
		}
	}
}
