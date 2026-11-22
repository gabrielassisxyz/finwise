package subrules

import (
	"reflect"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

func TestEachBoundaryBetweenTwoCyclesFallsWhereTheMidpointIs(t *testing.T) {
	cases := []struct {
		median int
		want   Cycle
	}{
		{1, Monthly}, {60, Monthly}, {61, Quarterly}, {136, Quarterly}, {137, Semiannual},
		{273, Semiannual}, {274, Yearly}, {547, Yearly}, {548, Biennial}, {2_000, Biennial},
	}
	for _, c := range cases {
		if got := cycleNearest(c.median); got != c.want {
			t.Errorf("a median of %d is cycle %d, want %d", c.median, got, c.want)
		}
		if got := derive(t, subscriptionOf(twoChargesApart(t, c.median))).Cycle; got != c.want {
			t.Errorf("two charges %d days apart are cycle %d, want %d", c.median, got, c.want)
		}
	}
}

func twoChargesApart(t *testing.T, days int) []Row {
	t.Helper()
	first := time.Date(2024, time.January, 15, 0, 0, 0, 0, time.UTC)
	second := first.AddDate(0, 0, days)
	return rowsOf(1_000, brday.Day(first.Format(dayLayout)), brday.Day(second.Format(dayLayout)))
}

func TestTheMonthArithmeticKeepsTheDayNumberAndHoldsItToTheMonthsEnd(t *testing.T) {
	cases := []struct {
		from   brday.Day
		months int
		want   brday.Day
	}{
		{"2027-01-31", 1, "2027-02-28"},
		{"2027-01-31", 3, "2027-04-30"},
		{"2026-08-31", 6, "2027-02-28"},
		{"2028-02-29", 12, "2029-02-28"},
		{"2028-02-29", 24, "2030-02-28"},
		{"2027-12-31", 2, "2028-02-29"},
		{"2026-11-28", 3, "2027-02-28"},
		{"2026-11-30", 3, "2027-02-28"},
		{"2026-03-20", 3, "2026-06-20"},
		{"2026-12-27", 1, "2027-01-27"},
	}
	for _, c := range cases {
		got, err := AddMonths(c.from, c.months)
		if err != nil || got != c.want {
			t.Errorf("%s plus %d months = %s (%v), want %s", c.from, c.months, got, err, c.want)
		}
	}
	if _, err := AddMonths("2026-13-01", 1); err == nil {
		t.Error("a day that does not exist was moved")
	}
}

func TestTheFirstReviewIsThreeMonthsAfterToday(t *testing.T) {
	for today, want := range map[brday.Day]brday.Day{"2026-11-28": "2027-02-28", "2026-11-30": "2027-02-28", "2026-03-20": "2026-06-20"} {
		if got, err := FirstReview(today); err != nil || got != want {
			t.Errorf("made on %s, reviewed on %s (%v), want %s", today, got, err, want)
		}
	}
}

func TestDaysBetweenCountsWholeDaysAcrossLeapYearsAndBackwards(t *testing.T) {
	cases := []struct {
		from, to brday.Day
		want     int
	}{
		{"2026-03-20", "2026-03-25", 5}, {"2026-03-25", "2026-03-20", -5}, {"2023-03-10", "2024-03-10", 366},
		{"2024-03-10", "2025-03-10", 365}, {"2026-10-25", "2026-10-26", 1}, {"2026-11-22", "2026-11-22", 0},
	}
	for _, c := range cases {
		if got, err := DaysBetween(c.from, c.to); err != nil || got != c.want {
			t.Errorf("%s to %s = %d (%v), want %d", c.from, c.to, got, err, c.want)
		}
	}
}

// The stand-in's own vectors, restated in cents, and the two halves.
func TestTheEquivalentsSpreadTheAmountOverTheMonthsOfItsCycle(t *testing.T) {
	cases := []struct {
		amount          money.Cents
		cycle           Cycle
		monthly, yearly money.Cents
	}{
		{12_000, Monthly, 12_000, 144_000},
		{12_000, Quarterly, 4_000, 48_000},
		{12_000, Semiannual, 2_000, 24_000},
		{12_000, Yearly, 1_000, 12_000},
		{12_000, Biennial, 500, 6_000},
		{8_000, Yearly, 667, 8_000},
		{3, Semiannual, 1, 6},
		{1_001, Biennial, 42, 501},
		{16_680, Yearly, 1_390, 16_680},
	}
	for _, c := range cases {
		monthly, yearly := MonthlyEquivalent(c.amount, c.cycle), YearlyEquivalent(c.amount, c.cycle)
		if monthly != c.monthly || yearly != c.yearly {
			t.Errorf("%d on cycle %d: %d a month and %d a year, want %d and %d", c.amount, c.cycle, monthly, yearly, c.monthly, c.yearly)
		}
	}
}

func TestTheTotalsAddTheEquivalentsAsShownAndTheYearIsTwelveTimesTheMonth(t *testing.T) {
	month, year := Totals([]money.Cents{11_000, 2_190, 1_390, 2_000, 2_990, 667})
	if month != 20_237 || year != 242_844 {
		t.Errorf("totals %d and %d, want 20237 and 242844", month, year)
	}
	if month, year := Totals(nil); month != 0 || year != 0 {
		t.Errorf("totals of nothing %d and %d", month, year)
	}
}

func TestAPriceMarkNeedsAPreviousChargeThatWasLower(t *testing.T) {
	cases := []struct {
		name     string
		previous money.Cents
		want     bool
	}{{"up", 1_990, true}, {"down", 2_390, false}, {"equal", 2_190, false}}
	for _, c := range cases {
		rows := []Row{{Day: "2026-10-22", Cents: c.previous}, {Day: "2026-11-22", Cents: 2_190}}
		if got := derive(t, subscriptionOf(rows)).Marks.PriceRose; got != c.want {
			t.Errorf("a price %s: marked %t", c.name, got)
		}
	}
	if derive(t, subscriptionOf(rowsOf(2_190, "2026-11-22"))).Marks.PriceRose {
		t.Error("one charge has no previous price to have risen from")
	}
}

// A price that rose and then fell back a little has fallen: the previous
// amount is the charge just before the latest, not the oldest.
func TestThePreviousAmountIsTheChargeJustBeforeTheLatest(t *testing.T) {
	rows := []Row{{Day: "2026-08-22", Cents: 1_000}, {Day: "2026-09-22", Cents: 2_500}, {Day: "2026-10-22", Cents: 2_190}}
	got := derive(t, subscriptionOf(rows))
	if !got.HasPrevious || got.Previous != 2_500 || got.Amount != 2_190 || got.Marks.PriceRose {
		t.Errorf("amount %d, previous %d (%t), marks %+v; want 2190 after 2500 and no mark", got.Amount, got.Previous, got.HasPrevious, got.Marks)
	}
}

func TestARenewalIsMarkedForAYearlyCycleUpToThirtyDaysAhead(t *testing.T) {
	yearly := rowsOf(16_680, "2025-03-25", "2026-03-25")
	cases := []struct {
		today brday.Day
		want  Marks
	}{
		{"2027-03-25", Marks{Renews: true, RenewsIn: 0}},
		{"2027-03-24", Marks{Renews: true, RenewsIn: 1}},
		{"2027-02-23", Marks{Renews: true, RenewsIn: 30}},
		{"2027-02-22", Marks{}},
		{"2027-03-26", Marks{}},
	}
	for _, c := range cases {
		subscription := subscriptionOf(yearly)
		subscription.Today = c.today
		if got := derive(t, subscription).Marks; got != c.want {
			t.Errorf("today %s: marks %+v, want %+v", c.today, got, c.want)
		}
	}
	biennial := subscriptionOf(rowsOf(30_000, "2024-04-10", "2026-04-10"))
	biennial.Today = "2028-04-01"
	if got := derive(t, biennial).Marks; got != (Marks{Renews: true, RenewsIn: 9}) {
		t.Errorf("a biennial cycle nine days away: %+v", got)
	}
	semiannual := subscriptionOf(rowsOf(6_000, "2025-10-10", "2026-04-10"))
	semiannual.Today = "2026-10-07"
	if got := derive(t, semiannual); got.Cycle != Semiannual || got.Marks != (Marks{}) {
		t.Errorf("a semiannual cycle three days away: cycle %d, marks %+v", got.Cycle, got.Marks)
	}
}

func TestChargedAfterCancellingNeedsTheStatusAndALaterRealCharge(t *testing.T) {
	cases := []struct {
		name       string
		latest     brday.Day
		cancelling bool
		want       bool
	}{
		{"the day before the status day", "2026-11-09", true, false},
		{"the status day", "2026-11-10", true, false},
		{"the day after", "2026-11-11", true, true},
		{"after, for another status", "2026-11-11", false, false},
	}
	for _, c := range cases {
		subscription := subscriptionOf(rowsOf(2_190, "2026-10-08", c.latest))
		subscription.Cancelling, subscription.StatusDay = c.cancelling, "2026-11-10"
		if got := derive(t, subscription).Marks.ChargedAfterCancel; got != c.want {
			t.Errorf("latest charge %s: marked %t, want %t", c.name, got, c.want)
		}
	}
	stored := Subscription{CreatedDay: "2026-11-11", CreatedCents: 2_190, Cancelling: true, StatusDay: "2026-11-10", Today: "2026-11-22"}
	if derive(t, stored).Marks.ChargedAfterCancel {
		t.Error("the stored creation charge was taken for a charge after cancelling")
	}
}

func TestTheHistoryIsTheThreeLatestChargesNewestFirst(t *testing.T) {
	rows := []Row{
		{Day: "2026-07-05", Cents: 100}, {Day: "2026-11-05", Cents: 500}, {Day: "2026-08-05", Cents: 200},
		{Day: "2026-10-05", Cents: 400}, {Day: "2026-09-05", Cents: 300},
	}
	want := []Charge{charge("2026-11-05", 500), charge("2026-10-05", 400), charge("2026-09-05", 300)}
	if got := derive(t, subscriptionOf(rows)).History; !reflect.DeepEqual(got, want) {
		t.Errorf("history %+v, want %+v", got, want)
	}
}

func TestOnlyAChargeOfOneRowWithAForeignAmountAndATaxCarriesThem(t *testing.T) {
	dollars := &Foreign{Amount: 2_000, Currency: "USD"}
	cases := []struct {
		name string
		rows []Row
		want Charge
	}{
		{"one foreign row with its tax", []Row{{Day: "2026-03-10", Cents: 11_385, Foreign: dollars, Tax: 385}},
			Charge{Day: "2026-03-10", Cents: 11_385, Foreign: dollars, Tax: 385}},
		{"one foreign row with no tax", []Row{{Day: "2026-03-10", Cents: 11_000, Foreign: dollars}}, charge("2026-03-10", 11_000)},
		{"one row with a tax in reais", []Row{{Day: "2026-03-10", Cents: 20_700, Tax: 700}}, charge("2026-03-10", 20_700)},
		{"two foreign rows of one day", []Row{
			{Day: "2026-03-10", Cents: 11_385, Foreign: dollars, Tax: 385}, {Day: "2026-03-10", Cents: 11_385, Foreign: dollars, Tax: 385},
		}, charge("2026-03-10", 22_770)},
	}
	for _, c := range cases {
		got := ChargesOf(c.rows)
		if len(got) != 1 || !reflect.DeepEqual(got[0], c.want) {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}
