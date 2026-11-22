package classrules

import (
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Invented rows of one account.
const (
	accountChecking = "acc-checking"
	accountCard     = "acc-card"
	keyBakery       = "name:padaria do bairro"
	keyPharmacy     = "name:farmacia central"
)

func vanished(key TxKey, day brday.Day) VanishedPending {
	return VanishedPending{Key: key, AccountID: accountCard, Day: day, Cents: money.Cents(-4590), EstablishmentKey: keyBakery}
}

func fresh(id string, day brday.Day) NewPosted {
	return NewPosted{ProviderID: id, AccountID: accountCard, Day: day, Cents: money.Cents(-4590), EstablishmentKey: keyBakery}
}

func reversed[T any](rows []T) []T {
	out := slices.Clone(rows)
	slices.Reverse(out)
	return out
}

type pairCase struct {
	name    string
	pending []VanishedPending
	posted  []NewPosted
	want    []Pairing
}

func TestPairMatchesOneToOneWithinFiveDays(t *testing.T) {
	otherAccount := fresh("p-other", "2026-03-12")
	otherAccount.AccountID = accountChecking
	otherCents := fresh("p-cents", "2026-03-12")
	otherCents.Cents = money.Cents(-4591)
	otherKey := fresh("p-key", "2026-03-12")
	otherKey.EstablishmentKey = keyPharmacy
	pharmacy := vanished(2, "2026-03-20")
	pharmacy.EstablishmentKey = keyPharmacy
	pharmacyPosted := fresh("p-pharmacy", "2026-03-21")
	pharmacyPosted.EstablishmentKey = keyPharmacy

	runPairCases(t, []pairCase{
		{"posted the same day", []VanishedPending{vanished(1, "2026-03-10")}, []NewPosted{fresh("p-1", "2026-03-10")}, []Pairing{{1, "p-1"}}},
		{"posted 5 days after", []VanishedPending{vanished(1, "2026-03-10")}, []NewPosted{fresh("p-1", "2026-03-15")}, []Pairing{{1, "p-1"}}},
		{"posted 6 days after", []VanishedPending{vanished(1, "2026-03-10")}, []NewPosted{fresh("p-1", "2026-03-16")}, nil},
		{"posted 1 day before", []VanishedPending{vanished(1, "2026-03-10")}, []NewPosted{fresh("p-1", "2026-03-09")}, nil},
		{"another account", []VanishedPending{vanished(1, "2026-03-10")}, []NewPosted{otherAccount}, nil},
		{"another cent", []VanishedPending{vanished(1, "2026-03-10")}, []NewPosted{otherCents}, nil},
		{"another establishment key", []VanishedPending{vanished(1, "2026-03-10")}, []NewPosted{otherKey}, nil},
		{"one pending, two posted", []VanishedPending{vanished(1, "2026-03-10")}, []NewPosted{fresh("p-1", "2026-03-11"), fresh("p-2", "2026-03-12")}, nil},
		{"two pending, one posted", []VanishedPending{vanished(1, "2026-03-10"), vanished(2, "2026-03-11")}, []NewPosted{fresh("p-1", "2026-03-12")}, nil},
		{"two independent matches", []VanishedPending{vanished(1, "2026-03-10"), pharmacy}, []NewPosted{pharmacyPosted, fresh("p-1", "2026-03-12")}, []Pairing{{1, "p-1"}, {2, "p-pharmacy"}}},
		{"an ambiguity beside a clean match", []VanishedPending{vanished(1, "2026-03-10"), vanished(3, "2026-03-11"), pharmacy}, []NewPosted{fresh("p-1", "2026-03-12"), pharmacyPosted}, []Pairing{{2, "p-pharmacy"}}},
		{"two pending in a chain over two posted", []VanishedPending{vanished(1, "2026-03-01"), vanished(2, "2026-03-05")}, []NewPosted{fresh("p-1", "2026-03-05"), fresh("p-2", "2026-03-10")}, nil},
		{"no candidate", nil, nil, nil},
		{"pending but nothing posted", []VanishedPending{vanished(1, "2026-03-10")}, nil, nil},
		{"posted but nothing pending", nil, []NewPosted{fresh("p-1", "2026-03-10")}, nil},
	})
}

func TestPairCountsCalendarDaysAcrossMonthAndLeapYearEnds(t *testing.T) {
	runPairCases(t, []pairCase{
		{"last of January to first of February", []VanishedPending{vanished(1, "2026-01-31")}, []NewPosted{fresh("p-1", "2026-02-01")}, []Pairing{{1, "p-1"}}},
		{"last of the year to the fifth of January", []VanishedPending{vanished(1, "2026-12-31")}, []NewPosted{fresh("p-1", "2027-01-05")}, []Pairing{{1, "p-1"}}},
		{"last of the year to the sixth of January", []VanishedPending{vanished(1, "2026-12-31")}, []NewPosted{fresh("p-1", "2027-01-06")}, nil},
		{"leap year: 25 February to 1 March is 5 days", []VanishedPending{vanished(1, "2028-02-25")}, []NewPosted{fresh("p-1", "2028-03-01")}, []Pairing{{1, "p-1"}}},
		{"leap year: 24 February to 1 March is 6 days", []VanishedPending{vanished(1, "2028-02-24")}, []NewPosted{fresh("p-1", "2028-03-01")}, nil},
		{"leap year: 29 February to 5 March is 5 days", []VanishedPending{vanished(1, "2028-02-29")}, []NewPosted{fresh("p-1", "2028-03-05")}, []Pairing{{1, "p-1"}}},
		{"common year: 24 February to 1 March is 5 days", []VanishedPending{vanished(1, "2027-02-24")}, []NewPosted{fresh("p-1", "2027-03-01")}, []Pairing{{1, "p-1"}}},
	})
}

func runPairCases(t *testing.T, cases []pairCase) {
	t.Helper()
	for _, c := range cases {
		for _, order := range []struct {
			name    string
			pending []VanishedPending
			posted  []NewPosted
		}{
			{"as given", c.pending, c.posted},
			{"pending reversed", reversed(c.pending), c.posted},
			{"posted reversed", c.pending, reversed(c.posted)},
			{"both reversed", reversed(c.pending), reversed(c.posted)},
		} {
			got, err := Pair(order.pending, order.posted)
			if err != nil {
				t.Errorf("%s, %s: %v", c.name, order.name, err)
				continue
			}
			if !slices.Equal(got, c.want) && !(len(got) == 0 && len(c.want) == 0) {
				t.Errorf("%s, %s: got %v, want %v", c.name, order.name, got, c.want)
			}
		}
	}
}

func TestPairRefusesAMalformedDayEvenOnARowThatMatchesNothing(t *testing.T) {
	stray := fresh("p-stray", "2026-02-30")
	stray.AccountID = accountChecking
	if _, err := Pair([]VanishedPending{vanished(1, "2026-03-10")}, []NewPosted{fresh("p-1", "2026-03-10"), stray}); err == nil {
		t.Error("a posted row dated 2026-02-30 was accepted")
	}
	if _, err := Pair([]VanishedPending{vanished(1, "10/03/2026")}, nil); err == nil {
		t.Error("a pending row dated 10/03/2026 was accepted")
	}
}

func TestDayNumbersDifferByCalendarDays(t *testing.T) {
	cases := []struct {
		from, to brday.Day
		want     int
	}{
		{"2026-03-10", "2026-03-10", 0},
		{"2026-03-31", "2026-04-01", 1},
		{"2026-02-28", "2026-03-01", 1},
		{"2028-02-28", "2028-03-01", 2},
		{"2028-02-29", "2028-03-01", 1},
		{"2026-12-31", "2027-01-01", 1},
		{"2026-10-31", "2026-11-02", 2},
		{"2026-03-15", "2026-03-10", -5},
		{"1969-12-31", "1970-01-01", 1},
	}
	for _, c := range cases {
		from, err := dayNumber(c.from)
		if err != nil {
			t.Fatal(err)
		}
		to, err := dayNumber(c.to)
		if err != nil {
			t.Fatal(err)
		}
		if to-from != c.want {
			t.Errorf("%s to %s: %d days, want %d", c.from, c.to, to-from, c.want)
		}
	}
}
