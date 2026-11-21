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
