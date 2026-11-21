package cardrules_test

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/cardrules"
)

// Two closed bills of an invented card whose bank follows the C6 table: due on
// Thursday 2026-02-12 (closed 2026-02-06) and Thursday 2026-03-12 (closed 2026-03-06).
func twoBillCard() cardrules.Card {
	return cardrules.Card{
		Bills: []cardrules.Bill{
			{ID: "bill-feb", Due: "2026-02-12", Closing: dayPointer("2026-02-06")},
			{ID: "bill-mar", Due: "2026-03-12", Closing: dayPointer("2026-03-06")},
		},
		Table: cardrules.C6ClosingTable(),
	}
}

func placeOn(card cardrules.Card, day brday.Day, billID *string) cardrules.Placement {
	return card.PlacePurchase(cardrules.Purchase{ID: "p", Day: day, BillID: billID})
}

func TestAPurchaseOnTheClosingDateBelongsToTheNextBill(t *testing.T) {
	card := twoBillCard()

	if got := placeOn(card, "2026-03-05", nil); got.BillID != "bill-mar" || got.Source != cardrules.FromDate {
		t.Errorf("the day before closing: %+v, want bill-mar from the date", got)
	}
	got := placeOn(card, "2026-03-06", nil)
	if !got.Predicted || got.BillID != "" || got.Due != "2026-04-12" {
		t.Errorf("on the closing date: %+v, want the predicted bill due 2026-04-12", got)
	}
}

func TestTheFirstBillStillOpenOnADayIsTheOneClosingAfterIt(t *testing.T) {
	card := twoBillCard()
	if got := placeOn(card, "2026-02-06", nil); got.BillID != "bill-mar" {
		t.Errorf("on the earlier closing date: %+v, want bill-mar", got)
	}
	if got := placeOn(card, "2026-02-05", nil); got.BillID != "bill-feb" {
		t.Errorf("the day before the earlier closing: %+v, want bill-feb", got)
	}
}

func TestAPurchaseWithABillIDBelongsToThatBillWhateverItsDate(t *testing.T) {
	card := twoBillCard()
	id := "bill-feb"
	got := placeOn(card, "2026-03-20", &id)
	if got.BillID != "bill-feb" || got.Source != cardrules.FromBillID || got.Due != "2026-02-12" {
		t.Fatalf("got %+v, want bill-feb by its id with its due date", got)
	}
}

func TestABillIDOfABillTheLedgerLacksStillIsTheAnswer(t *testing.T) {
	id := "bill-gone"
	got := placeOn(twoBillCard(), "2026-03-01", &id)
	if got.BillID != "bill-gone" || got.Source != cardrules.FromBillID || got.Due != "" {
		t.Fatalf("got %+v, want the unknown bill's id kept and no due date invented", got)
	}
}

func TestAPurchaseIsPlacedByTheBrazilDayNotTheUTCDay(t *testing.T) {
	// 2026-03-06 01:00 UTC is 22:00 on the 5th in Brazil: the purchase is on the
	// bill that closes on the 6th. The conversion is the reader's, and this pins
	// that the day it hands over is the one the rules compare.
	if got := placeOn(twoBillCard(), brday.DayOf(mustInstant(t, "2026-03-06T01:00:00Z")), nil); got.BillID != "bill-mar" {
		t.Fatalf("got %+v, want bill-mar", got)
	}
}

func TestAPurchaseWithNothingToCompareItWithIsUnknown(t *testing.T) {
	cases := map[string]cardrules.Card{
		"no bills":             {Table: cardrules.C6ClosingTable()},
		"a bank with no table": {Bills: []cardrules.Bill{{ID: "b", Due: "2026-03-12"}}},
	}
	for name, card := range cases {
		if got := placeOn(card, "2026-03-01", nil); got.Source != cardrules.SourceUnknown {
			t.Errorf("%s: got %+v, want unknown", name, got)
		}
	}
}

func TestAMissingBillBetweenPurchaseAndMatchIsUnknownNotGuessed(t *testing.T) {
	card := twoBillCard()
	if got := placeOn(card, "2025-11-01", nil); got.Source != cardrules.SourceUnknown {
		t.Fatalf("a purchase months before the first bill: %+v, want unknown", got)
	}
}

func TestAClosingDateTheLedgerLacksComesFromTheTable(t *testing.T) {
	card := twoBillCard()
	card.Bills[1].Closing = nil
	if got := placeOn(card, "2026-03-05", nil); got.BillID != "bill-mar" {
		t.Errorf("the day before the table's closing: %+v, want bill-mar", got)
	}
	if got := placeOn(card, "2026-03-06", nil); !got.Predicted {
		t.Errorf("on the table's closing: %+v, want the predicted bill", got)
	}
}

func TestPredictedDueDatesKeepTheDayNumberAcrossShortMonths(t *testing.T) {
	card := cardrules.Card{
		Bills: []cardrules.Bill{{ID: "b", Due: "2026-01-31", Closing: dayPointer("2026-01-24")}},
		Table: cardrules.C6ClosingTable(),
	}
	if got := placeOn(card, "2026-02-01", nil); got.Due != "2026-02-28" {
		t.Errorf("February: %+v, want due 2026-02-28", got)
	}
	if got := placeOn(card, "2026-03-01", nil); got.Due != "2026-03-31" {
		t.Errorf("March: %+v, want due 2026-03-31, not drifted to the 28th", got)
	}
}

func TestTheOrderOfTheBillsHandedInDoesNotMatter(t *testing.T) {
	card := twoBillCard()
	card.Bills[0], card.Bills[1] = card.Bills[1], card.Bills[0]
	if got := placeOn(card, "2026-02-05", nil); got.BillID != "bill-feb" {
		t.Errorf("got %+v, want bill-feb", got)
	}
	if got := placeOn(card, "2026-03-06", nil); !got.Predicted || got.Due != "2026-04-12" {
		t.Errorf("got %+v, want the predicted bill due 2026-04-12", got)
	}
}
