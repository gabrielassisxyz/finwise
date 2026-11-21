package cardrules_test

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/cardrules"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

func mustInstant(t *testing.T, text string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// A card whose November closing, 2026-11-27, is the table's answer for the bill
// due on Thursday 2026-12-03, and is not in the ledger yet.
func novemberCard() cardrules.Card {
	return cardrules.Card{
		Bills: []cardrules.Bill{{ID: "bill-oct", Due: "2026-11-03", Closing: dayPointer("2026-10-28")}},
		Table: cardrules.C6ClosingTable(),
	}
}

func purchase(id string, day brday.Day, cents money.Cents, billID *string) cardrules.Purchase {
	return cardrules.Purchase{ID: id, Day: day, Amount: cents, BillID: billID}
}

func includedIDs(amount cardrules.NextBillAmount) map[string]bool {
	ids := map[string]bool{}
	for _, included := range amount.Included {
		ids[included.ID] = true
	}
	return ids
}

func TestNextBillAmountIncludesTheClosingDayAndTheSlipWithoutTheClosedBillID(t *testing.T) {
	closed := "bill-nov"
	card := novemberCard()
	card.Bills = append(card.Bills, cardrules.Bill{ID: closed, Due: "2026-12-03", Closing: dayPointer("2026-11-27")})
	purchases := []cardrules.Purchase{
		purchase("on-closing", "2026-11-27", 1000, nil),
		purchase("after", "2026-11-28", 200, nil),
		purchase("day-before-unbilled", "2026-11-26", 30, nil),
		purchase("day-before-on-closed-bill", "2026-11-26", 4000, &closed),
		purchase("two-days-before-unbilled", "2026-11-25", 5, nil),
		purchase("three-days-before", "2026-11-24", 90000, nil),
	}

	got := card.AmountOnNextBill("2026-11", "2026-11-30", purchases)

	ids := includedIDs(got)
	for _, want := range []string{"on-closing", "after", "day-before-unbilled", "two-days-before-unbilled"} {
		if !ids[want] {
			t.Errorf("%s is missing from the amount", want)
		}
	}
	for _, unwanted := range []string{"day-before-on-closed-bill", "three-days-before"} {
		if ids[unwanted] {
			t.Errorf("%s is in the amount", unwanted)
		}
	}
	if !got.Known || got.Closing != "2026-11-27" || got.Total != 1235 {
		t.Errorf("got known=%t closing=%s total=%d, want known on 2026-11-27 for 1235", got.Known, got.Closing, got.Total)
	}
}

func TestNextBillAmountUsesThePredictedClosingWhenTheLedgerHasNoBillYet(t *testing.T) {
	purchases := []cardrules.Purchase{purchase("a", "2026-11-27", 700, nil), purchase("b", "2026-11-20", 900, nil)}
	got := novemberCard().AmountOnNextBill("2026-11", "2026-11-28", purchases)
	if !got.Known || got.Closing != "2026-11-27" || got.Total != 700 {
		t.Fatalf("got known=%t closing=%s total=%d, want 700 from the predicted 2026-11-27", got.Known, got.Closing, got.Total)
	}
}

func TestNextBillAmountIsEmptyBeforeTheClosingHasCome(t *testing.T) {
	purchases := []cardrules.Purchase{purchase("a", "2026-11-26", 700, nil), purchase("b", "2026-11-25", 900, nil)}
	got := novemberCard().AmountOnNextBill("2026-11", "2026-11-26", purchases)
	if !got.Known || got.Total != 0 || len(got.Included) != 0 {
		t.Fatalf("got %+v, want known and empty: the slip days are not slipped until the bill has closed", got)
	}
}

func TestNextBillAmountCountsFromTheClosingDayItself(t *testing.T) {
	purchases := []cardrules.Purchase{purchase("on-closing", "2026-11-27", 700, nil)}
	got := novemberCard().AmountOnNextBill("2026-11", "2026-11-27", purchases)
	if !got.Known || got.Total != 700 {
		t.Fatalf("got known=%t total=%d, want 700: the closing day is the first day of the next bill", got.Known, got.Total)
	}
}

func TestNextBillAmountLeavesOutOtherMonths(t *testing.T) {
	// Closing on 2026-12-01: the slip days of 29 and 30 November belong to another month's figure.
	card := cardrules.Card{
		Bills: []cardrules.Bill{{ID: "b", Due: "2026-12-08", Closing: dayPointer("2026-12-01")}},
		Table: cardrules.C6ClosingTable(),
	}
	purchases := []cardrules.Purchase{purchase("nov", "2026-11-30", 100, nil), purchase("dec", "2026-12-01", 20, nil)}
	got := card.AmountOnNextBill("2026-12", "2026-12-02", purchases)
	if got.Total != 20 {
		t.Fatalf("total = %d, want 20: only December's purchases count", got.Total)
	}
}

func TestNextBillAmountIsUnknownWhenTheClosingIsUnknown(t *testing.T) {
	card := cardrules.Card{Bills: []cardrules.Bill{{ID: "b", Due: "2026-11-03"}}}
	got := card.AmountOnNextBill("2026-11", "2026-11-29", []cardrules.Purchase{purchase("a", "2026-11-28", 700, nil)})
	if got.Known {
		t.Fatalf("got %+v, want unknown: a bank with no table has no predicted closing", got)
	}
}

func TestNextBillAmountFollowsAManualClosingDay(t *testing.T) {
	card := novemberCard()
	card.ManualClosingDay = 20
	purchases := []cardrules.Purchase{purchase("a", "2026-11-20", 700, nil), purchase("b", "2026-11-18", 900, nil)}
	got := card.AmountOnNextBill("2026-11", "2026-11-25", purchases)
	if got.Closing != "2026-11-20" || got.Total != 1600 {
		t.Fatalf("got closing=%s total=%d, want 2026-11-20 and 1600 (the 18th is a slip day)", got.Closing, got.Total)
	}
}
