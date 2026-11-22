package cardrules_test

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/cardrules"
)

func weekdayOf(t *testing.T, day brday.Day) time.Weekday {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", string(day))
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Weekday()
}

func TestClosingDateFollowsTheWeekdayOfTheDueDate(t *testing.T) {
	cases := []struct {
		name    string
		due     brday.Day
		weekday time.Weekday
		closing brday.Day
	}{
		{"monday", "2026-01-12", time.Monday, "2026-01-06"},
		{"tuesday", "2026-05-12", time.Tuesday, "2026-05-06"},
		{"wednesday", "2026-08-12", time.Wednesday, "2026-08-06"},
		{"thursday", "2026-03-12", time.Thursday, "2026-03-06"},
		{"friday", "2026-06-12", time.Friday, "2026-06-05"},
		{"saturday", "2026-09-12", time.Saturday, "2026-09-05"},
		{"sunday", "2026-04-12", time.Sunday, "2026-04-04"},
		{"closing in the previous year", "2026-01-03", time.Saturday, "2025-12-27"},
		{"closing in the previous month", "2026-03-03", time.Tuesday, "2026-02-25"},
		{"friday across a leap day", "2028-03-03", time.Friday, "2028-02-25"},
	}
	table := cardrules.C6ClosingTable()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := weekdayOf(t, tc.due); got != tc.weekday {
				t.Fatalf("the case is wrong: %s is a %s, not a %s", tc.due, got, tc.weekday)
			}
			closing, ok := table.ClosingDate(tc.due)
			if !ok || closing != tc.closing {
				t.Fatalf("ClosingDate(%s) = %s, %t; want %s", tc.due, closing, ok, tc.closing)
			}
		})
	}
}

func TestACardWithoutATableHasAnUnknownClosingDate(t *testing.T) {
	for name, table := range map[string]cardrules.ClosingTable{
		"no table": nil, "empty table": {}, "a weekday the table lacks": {time.Monday: 6},
	} {
		if closing, ok := table.ClosingDate("2026-03-12"); ok {
			t.Errorf("%s: got closing %s, want unknown", name, closing)
		}
	}
}

func TestADueDateThatIsNotADayHasAnUnknownClosingDate(t *testing.T) {
	if _, ok := cardrules.C6ClosingTable().ClosingDate("2026-13-40"); ok {
		t.Fatal("a closing date was worked out from an impossible due date")
	}
}

func TestManualClosingDayMovesTheClosingAndLeavesTheDueDate(t *testing.T) {
	bills := []cardrules.Bill{{ID: "b1", Due: "2026-11-03", Closing: dayPointer("2026-10-28")}}
	byTable := cardrules.Card{Bills: bills, Table: cardrules.C6ClosingTable()}
	byHand := cardrules.Card{Bills: bills, Table: cardrules.C6ClosingTable(), ManualClosingDay: 20}

	// The table closes the bill due 2026-12-03 on 2026-11-27; the person's day closes it on the 20th.
	before := cardrules.Purchase{ID: "p-before", Day: "2026-11-19"}
	between := cardrules.Purchase{ID: "p-between", Day: "2026-11-24"}

	for name, card := range map[string]cardrules.Card{"table": byTable, "manual": byHand} {
		if got := card.PlacePurchase(before); got.Due != "2026-12-03" {
			t.Errorf("%s: a purchase before both closings is due %s, want 2026-12-03", name, got.Due)
		}
	}
	if got := byTable.PlacePurchase(between); got.Due != "2026-12-03" {
		t.Errorf("table: purchase of the 24th is due %s, want 2026-12-03", got.Due)
	}
	if got := byHand.PlacePurchase(between); got.Due != "2027-01-03" {
		t.Errorf("manual: purchase of the 24th is due %s, want 2027-01-03 (the closing moved to the 20th)", got.Due)
	}
}

func TestManualClosingDateAtMonthEnds(t *testing.T) {
	cases := []struct {
		due        brday.Day
		closingDay int
		want       brday.Day
	}{
		{"2026-12-03", 28, "2026-11-28"},
		{"2026-03-03", 31, "2026-02-28"},
		{"2028-03-03", 31, "2028-02-29"},
		{"2026-01-03", 28, "2025-12-28"},
		{"2026-12-20", 3, "2026-12-03"},
		{"2026-12-03", 3, "2026-11-03"},
	}
	for _, tc := range cases {
		got, ok := cardrules.ManualClosingDate(tc.due, tc.closingDay)
		if !ok || got != tc.want {
			t.Errorf("ManualClosingDate(%s, %d) = %s, %t; want %s", tc.due, tc.closingDay, got, ok, tc.want)
		}
	}
	for _, bad := range []int{0, 32, -1} {
		if _, ok := cardrules.ManualClosingDate("2026-12-03", bad); ok {
			t.Errorf("closing day %d was accepted", bad)
		}
	}
}

func dayPointer(day brday.Day) *brday.Day { return &day }

func TestThePredictedClosingIsTheManualDayWhenSetTheTablesWhenNotAndUnknownWithNeither(t *testing.T) {
	// 2026-03-10 is a Tuesday, which the C6 table closes six days before.
	const due brday.Day = "2026-03-10"
	dated := []cardrules.Bill{{ID: "b-dated", Due: due, Closing: dayPointer("2026-03-01")}}
	cases := []struct {
		name  string
		card  cardrules.Card
		want  brday.Day
		known bool
	}{
		{"the table", cardrules.Card{Table: cardrules.C6ClosingTable()}, "2026-03-04", true},
		{"a manual day over the table", cardrules.Card{Table: cardrules.C6ClosingTable(), ManualClosingDay: 25}, "2026-02-25", true},
		{"a manual day and no table", cardrules.Card{ManualClosingDay: 22}, "2026-02-22", true},
		{"a manual day and a ledger date for that due", cardrules.Card{Bills: dated, ManualClosingDay: 25}, "2026-02-25", true},
		{"neither a table nor a manual day", cardrules.Card{}, "", false},
		{"neither, and a ledger date for that due", cardrules.Card{Bills: dated}, "", false},
	}
	for _, c := range cases {
		closing, ok := c.card.PredictedClosing(due)
		if closing != c.want || ok != c.known {
			t.Errorf("%s: PredictedClosing(%s) = %q, %t; want %q, %t", c.name, due, closing, ok, c.want, c.known)
		}
	}
}

func shiftedDay(t *testing.T, day brday.Day, days int) brday.Day {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", string(day))
	if err != nil {
		t.Fatal(err)
	}
	return brday.Day(parsed.AddDate(0, 0, days).Format("2006-01-02"))
}

// On every cycle after the dated bill, the day before the closing the exported
// method answers still places a purchase on that cycle, and the closing day
// places it on the next one: the method is the closing the cycles use.
func TestPlacePurchaseAgreesWithThePredictedClosingOnEveryCycle(t *testing.T) {
	bills := []cardrules.Bill{
		{ID: "b-dated", Due: "2026-02-10", Closing: dayPointer("2026-02-04")},
		{ID: "b-undated", Due: "2026-03-10"},
	}
	dues := []brday.Day{"2026-03-10", "2026-04-10", "2026-05-10", "2026-06-10"}
	for _, manualDay := range []int{0, 25, 3} {
		card := cardrules.Card{Bills: bills, Table: cardrules.C6ClosingTable(), ManualClosingDay: manualDay}
		if got := card.PlacePurchase(cardrules.Purchase{ID: "p", Day: "2026-02-03"}); got.BillID != "b-dated" {
			t.Errorf("manual day %d: the day before the ledger's closing falls on %+v, want b-dated", manualDay, got)
		}
		for index, due := range dues {
			closing, ok := card.PredictedClosing(due)
			if !ok {
				t.Fatalf("manual day %d: no closing for %s", manualDay, due)
			}
			before := card.PlacePurchase(cardrules.Purchase{ID: "p", Day: shiftedDay(t, closing, -1)})
			if before.Due != due || (index == 0) != (before.BillID == "b-undated") {
				t.Errorf("manual day %d: the day before %s falls on %+v, want the cycle due %s", manualDay, closing, before, due)
			}
			on := card.PlacePurchase(cardrules.Purchase{ID: "p", Day: closing})
			switch {
			case index+1 < len(dues) && on.Due != dues[index+1]:
				t.Errorf("manual day %d: the closing day %s falls on %+v, want the cycle due %s", manualDay, closing, on, dues[index+1])
			case index+1 == len(dues) && on.Source != cardrules.SourceUnknown:
				t.Errorf("manual day %d: the closing day of the last cycle falls on %+v, want unknown", manualDay, on)
			}
		}
	}
}
