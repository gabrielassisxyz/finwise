package subrules

import (
	"reflect"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Every figure here is invented.

func rowsOf(cents money.Cents, days ...brday.Day) []Row {
	rows := make([]Row, len(days))
	for index, day := range days {
		rows[index] = Row{Day: day, Cents: cents}
	}
	return rows
}

func charge(day brday.Day, cents money.Cents) Charge { return Charge{Day: day, Cents: cents} }

func subscriptionOf(rows []Row) Subscription {
	return Subscription{Rows: rows, CreatedDay: "2023-01-01", CreatedCents: 1, StatusDay: "2023-01-01", Today: "2026-11-22"}
}

func derive(t *testing.T, subscription Subscription) Derivation {
	t.Helper()
	derivation, err := Derive(subscription)
	if err != nil {
		t.Fatal(err)
	}
	return derivation
}

// The worked examples, each answered whole: the cycle, the figures, the next
// charge, the marks and the history.
func TestTheWorkedExamplesAreAnsweredExactly(t *testing.T) {
	cases := []struct {
		name         string
		subscription Subscription
		want         Derivation
	}{
		{
			name:         "irregular gaps",
			subscription: subscriptionOf(rowsOf(3_990, "2026-01-05", "2026-02-03", "2026-03-07", "2026-04-04", "2026-05-06")),
			want: Derivation{Cycle: Monthly, Amount: 3_990, Previous: 3_990, HasPrevious: true, NextCharge: "2026-06-06",
				Monthly: 3_990, Yearly: 47_880,
				History: []Charge{charge("2026-05-06", 3_990), charge("2026-04-04", 3_990), charge("2026-03-07", 3_990)}},
		},
		{
			name:         "a tie between two cycles",
			subscription: subscriptionOf(rowsOf(6_000, "2026-01-10", "2026-02-10", "2026-03-12", "2026-06-12", "2026-09-11")),
			want: Derivation{Cycle: Monthly, Amount: 6_000, Previous: 6_000, HasPrevious: true, NextCharge: "2026-10-11",
				Monthly: 6_000, Yearly: 72_000,
				History: []Charge{charge("2026-09-11", 6_000), charge("2026-06-12", 6_000), charge("2026-03-12", 6_000)}},
		},
		{
			name:         "the tie with one more charge",
			subscription: subscriptionOf(rowsOf(6_000, "2026-01-10", "2026-02-10", "2026-03-12", "2026-06-12", "2026-09-11", "2026-12-11")),
			want: Derivation{Cycle: Quarterly, Amount: 6_000, Previous: 6_000, HasPrevious: true, NextCharge: "2027-03-11",
				Monthly: 2_000, Yearly: 24_000,
				History: []Charge{charge("2026-12-11", 6_000), charge("2026-09-11", 6_000), charge("2026-06-12", 6_000)}},
		},
		{
			name: "more than six gaps",
			subscription: subscriptionOf(rowsOf(2_490, "2023-03-10", "2024-03-10", "2025-03-10", "2026-03-10", "2026-04-09",
				"2026-05-10", "2026-06-09", "2026-07-10", "2026-08-09", "2026-09-09")),
			want: Derivation{Cycle: Monthly, Amount: 2_490, Previous: 2_490, HasPrevious: true, NextCharge: "2026-10-09",
				Monthly: 2_490, Yearly: 29_880,
				History: []Charge{charge("2026-09-09", 2_490), charge("2026-08-09", 2_490), charge("2026-07-10", 2_490)}},
		},
		{
			name: "charges on the same day",
			subscription: subscriptionOf([]Row{
				{Day: "2026-03-10", Cents: 2_000, Foreign: &Foreign{Amount: 400, Currency: "USD"}, Tax: 70},
				{Day: "2026-03-10", Cents: 500},
				{Day: "2026-04-10", Cents: 2_500},
			}),
			want: Derivation{Cycle: Monthly, Amount: 2_500, Previous: 2_500, HasPrevious: true, NextCharge: "2026-05-10",
				Monthly: 2_500, Yearly: 30_000,
				History: []Charge{charge("2026-04-10", 2_500), charge("2026-03-10", 2_500)}},
		},
		{
			name:         "a history that ends on the 31st",
			subscription: subscriptionOf(rowsOf(1_500, "2026-12-31", "2027-01-31")),
			want: Derivation{Cycle: Monthly, Amount: 1_500, Previous: 1_500, HasPrevious: true, NextCharge: "2027-02-28",
				Monthly: 1_500, Yearly: 18_000,
				History: []Charge{charge("2027-01-31", 1_500), charge("2026-12-31", 1_500)}},
		},
		{
			name: "a yearly plan",
			subscription: Subscription{Rows: rowsOf(16_680, "2024-03-25", "2025-03-25"), CreatedDay: "2024-03-25", CreatedCents: 16_680,
				StatusDay: "2024-03-25", Today: "2026-03-20"},
			want: Derivation{Cycle: Yearly, Amount: 16_680, Previous: 16_680, HasPrevious: true, NextCharge: "2026-03-25",
				Monthly: 1_390, Yearly: 16_680, Marks: Marks{Renews: true, RenewsIn: 5},
				History: []Charge{charge("2025-03-25", 16_680), charge("2024-03-25", 16_680)}},
		},
		{
			name:         "fewer than two charge days",
			subscription: subscriptionOf(rowsOf(6_400, "2026-11-27")),
			want: Derivation{Cycle: Monthly, Amount: 6_400, NextCharge: "2026-12-27", Monthly: 6_400, Yearly: 76_800,
				History: []Charge{charge("2026-11-27", 6_400)}},
		},
		{
			name: "no charge left",
			subscription: Subscription{CreatedDay: "2026-11-27", CreatedCents: 6_400, Cancelling: true, StatusDay: "2026-11-01",
				Today: "2026-11-28"},
			want: Derivation{Cycle: Monthly, Amount: 6_400, NextCharge: "2026-12-27", Monthly: 6_400, Yearly: 76_800,
				History: []Charge{{Day: "2026-11-27", Cents: 6_400, stored: true}}},
		},
		{
			name:         "a price that rose",
			subscription: subscriptionOf(append(rowsOf(1_990, "2026-09-22", "2026-10-22"), Row{Day: "2026-11-22", Cents: 2_190})),
			want: Derivation{Cycle: Monthly, Amount: 2_190, Previous: 1_990, HasPrevious: true, NextCharge: "2026-12-22",
				Monthly: 2_190, Yearly: 26_280, Marks: Marks{PriceRose: true},
				History: []Charge{charge("2026-11-22", 2_190), charge("2026-10-22", 1_990), charge("2026-09-22", 1_990)}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := derive(t, c.subscription); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestTheMedianOfTheWorkedExamplesIsTheLowerMiddleOfTheLatestSixGaps(t *testing.T) {
	cases := []struct {
		name string
		gaps []int
		want int
	}{
		{"irregular gaps", []int{29, 32, 28, 32}, 29},
		{"a tie between two cycles", []int{31, 30, 92, 91}, 31},
		{"the tie with one more charge", []int{31, 30, 92, 91, 91}, 91},
		{"more than six gaps", []int{366, 365, 365, 30, 31, 30, 31, 30, 31}, 30},
		{"one gap", []int{365}, 365},
	}
	for _, c := range cases {
		if got := medianGap(c.gaps); got != c.want {
			t.Errorf("%s: median of %v = %d, want %d", c.name, c.gaps, got, c.want)
		}
	}
}
