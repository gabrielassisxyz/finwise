package potrules_test

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"

	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

// The rules are held to the collector's statement over fixture ledgers in the
// api package; these cases state what each rule means in plain figures.

func potEntry(id string, amount money.Cents) potrules.YieldEntry {
	return potrules.YieldEntry{ReserveID: text(id), Name: text(id), Stable: true, BRL: true, AmountOK: true, Cents: cents(amount)}
}

func sighted(day, updated string, entries ...potrules.YieldEntry) potrules.YieldSighting {
	return potrules.YieldSighting{Day: brdayOf(day), RunID: 1, UpdatedAt: updated, Entries: entries}
}

const (
	morning1 = "2026-03-01 02:00:00"
	morning2 = "2026-03-02 02:00:00"
	morning3 = "2026-03-03 02:00:00"
	noon1    = "2026-03-01 12:00:00"
)

// transferOf is a posted transfer of the pot, first seen the day it happened.
func transferOf(id int64, instant, day string, amount money.Cents) potrules.TransferDocument {
	return potrules.TransferDocument{ID: id, Versions: []potrules.TransferVersion{{
		ObservationID: id, RunID: 1, Parent: "acc", Day: brdayOf(day), IsReserve: true,
		Date: instant, Instant: instant, Name: text("pot-a"), Cents: cents(amount),
	}}}
}

func accountOf(sightings []potrules.YieldSighting, transfers ...potrules.TransferDocument) potrules.AccountYield {
	return potrules.AccountYield{
		AccountID: "acc", Sightings: sightings, Transfers: transfers,
		Walks: []potrules.CoverageWalk{{RunID: 1, Since: "2026-01-01 00:00:00"}},
	}
}

func yieldOnly(t *testing.T, rows []potrules.DayYield) []*money.Cents {
	t.Helper()
	figures := make([]*money.Cents, 0, len(rows))
	for _, row := range rows {
		figures = append(figures, row.Yield)
	}
	return figures
}

func TestADepositRaisesTheBalanceAndYieldsNothing(t *testing.T) {
	account := accountOf(
		[]potrules.YieldSighting{
			sighted("2026-03-01", morning1, potEntry("pot-a", 10000)),
			sighted("2026-03-02", morning2, potEntry("pot-a", 15030)),
		},
		transferOf(1, noon1, "2026-03-01", -5000))

	rows := account.DailyYields([]brday.Day{"2026-03-01", "2026-03-02"})
	if got := yieldOnly(t, rows); len(got) != 1 || got[0] == nil || *got[0] != 30 {
		t.Fatalf("yield = %v, want the 30 left after the deposit, not the 5030 the balance rose by", got)
	}
}

func TestADayWithOnlyADepositIsAKnownZero(t *testing.T) {
	account := accountOf(
		[]potrules.YieldSighting{
			sighted("2026-03-01", morning1, potEntry("pot-a", 10000)),
			sighted("2026-03-02", morning2, potEntry("pot-a", 15000)),
		},
		transferOf(1, noon1, "2026-03-01", -5000))

	got := yieldOnly(t, account.DailyYields([]brday.Day{"2026-03-01", "2026-03-02"}))
	if len(got) != 1 || got[0] == nil || *got[0] != 0 {
		t.Fatalf("yield = %v, want a known zero", got)
	}
}

func TestAWithdrawalIsAddedBackWithTheOppositeSign(t *testing.T) {
	account := accountOf(
		[]potrules.YieldSighting{
			sighted("2026-03-01", morning1, potEntry("pot-a", 10000)),
			sighted("2026-03-02", morning2, potEntry("pot-a", 8010)),
		},
		transferOf(1, noon1, "2026-03-01", 2000))

	got := yieldOnly(t, account.DailyYields([]brday.Day{"2026-03-01", "2026-03-02"}))
	if len(got) != 1 || got[0] == nil || *got[0] != 10 {
		t.Fatalf("yield = %v, want 10", got)
	}
}

func TestATransferThatIsNotSettledMakesTheDayUnknownNotZero(t *testing.T) {
	pending := transferOf(1, noon1, "2026-03-01", -5000)
	pending.Versions[0].NotPosted = true
	account := accountOf(
		[]potrules.YieldSighting{
			sighted("2026-03-01", morning1, potEntry("pot-a", 10000)),
			sighted("2026-03-02", morning2, potEntry("pot-a", 15000)),
		}, pending)

	rows := account.Between("2026-03-01", "2026-03-02")
	if len(rows) != 1 || rows[0].Yield != nil || rows[0].Reason == nil || *rows[0].Reason != potrules.ReasonTransferPending {
		t.Fatalf("rows = %+v, want one row unknown for a pending transfer", rows)
	}
}

func TestADayTheLedgerCannotPlaceIsUnknownForEveryPotThatEndsIt(t *testing.T) {
	cases := map[string][]potrules.YieldSighting{
		"no time on the first sighting": {
			sighted("2026-03-01", "", potEntry("pot-a", 100)), sighted("2026-03-02", morning2, potEntry("pot-a", 110))},
		"times that do not increase": {
			sighted("2026-03-01", morning2, potEntry("pot-a", 100)), sighted("2026-03-02", morning2, potEntry("pot-a", 110))},
		"no sighting before the day": {
			sighted("2026-03-02", morning2, potEntry("pot-a", 110))},
	}
	for label, sightings := range cases {
		days := []brday.Day{"2026-03-01", "2026-03-02"}
		rows := accountOf(sightings).DailyYields(days)
		if len(rows) != 1 || rows[0].ReserveID != "pot-a" || rows[0].Yield != nil {
			t.Errorf("%s: rows = %+v, want one unknown row for pot-a", label, rows)
		}
	}
}

func TestADayThatFallsOnTheSameSightingAsThePreviousOneAddsNoRow(t *testing.T) {
	account := accountOf([]potrules.YieldSighting{
		sighted("2026-03-01", morning1, potEntry("pot-a", 100)),
		sighted("2026-03-03", morning3, potEntry("pot-a", 130)),
	})
	// The 2nd was sighted without reserves; the pair reaching over it carries the yield.
	rows := account.DailyYields([]brday.Day{"2026-03-01", "2026-03-02", "2026-03-03"})
	figures := yieldOnly(t, rows)
	if len(rows) != 1 || rows[0].From != "2026-03-01" || figures[0] == nil || *figures[0] != 30 {
		t.Fatalf("rows = %+v, want one row of 30 from the 1st to the 3rd", rows)
	}
}

func TestADayAfterTheLastSightingOfReservesAddsNoRowOfItsOwn(t *testing.T) {
	account := accountOf([]potrules.YieldSighting{
		sighted("2026-03-01", morning1, potEntry("pot-a", 100)),
		sighted("2026-03-02", morning2, potEntry("pot-a", 110)),
	})
	rows := account.DailyYields([]brday.Day{"2026-03-01", "2026-03-02", "2026-03-03"})
	figures := yieldOnly(t, rows)
	if len(rows) != 1 || figures[0] == nil || *figures[0] != 10 {
		t.Fatalf("rows = %+v, want the one known row: the 3rd has nothing new to add", rows)
	}
}

func TestASightingWhoseTimeDoesNotMoveOnIsReachedOverNotLeftOut(t *testing.T) {
	account := accountOf([]potrules.YieldSighting{
		sighted("2026-03-01", morning1, potEntry("pot-a", 100)),
		sighted("2026-03-02", morning1, potEntry("pot-a", 100)),
		sighted("2026-03-03", morning3, potEntry("pot-a", 130)),
	})
	rows := account.DailyYields([]brday.Day{"2026-03-01", "2026-03-02", "2026-03-03"})
	figures := yieldOnly(t, rows)
	if len(rows) != 1 || rows[0].From != "2026-03-01" || rows[0].To != "2026-03-03" || figures[0] == nil || *figures[0] != 30 {
		t.Fatalf("rows = %+v, want the one row of 30 that reaches over the 2nd", rows)
	}
}

func TestADayThatEndsOnATimeThatDoesNotMoveOnIsUnknownNotDropped(t *testing.T) {
	account := accountOf([]potrules.YieldSighting{
		sighted("2026-03-01", morning1, potEntry("pot-a", 100)),
		sighted("2026-03-02", morning2, potEntry("pot-a", 110)),
		sighted("2026-03-03", morning2, potEntry("pot-a", 120)),
	})
	rows := account.DailyYields([]brday.Day{"2026-03-01", "2026-03-02", "2026-03-03"})
	figures := yieldOnly(t, rows)
	if len(rows) != 2 || figures[0] == nil || *figures[0] != 10 || figures[1] != nil || rows[1].To != "2026-03-03" {
		t.Fatalf("rows = %+v, want 10 for the 2nd and an unknown row for the 3rd", rows)
	}
}

func TestTheRowsOfSuccessivePairsAddUpToTheYieldOfTheWholeWindow(t *testing.T) {
	account := accountOf([]potrules.YieldSighting{
		sighted("2026-03-01", morning1, potEntry("pot-a", 100)),
		sighted("2026-03-02", morning2, potEntry("pot-a", 112)),
		sighted("2026-03-03", morning3, potEntry("pot-a", 125)),
	})
	var sum money.Cents
	for _, figure := range yieldOnly(t, account.DailyYields([]brday.Day{"2026-03-01", "2026-03-02", "2026-03-03"})) {
		sum += *figure
	}
	whole := account.Between("2026-03-01", "2026-03-03")
	if len(whole) != 1 || whole[0].Yield == nil || *whole[0].Yield != sum {
		t.Fatalf("the days add up to %d, the whole window is %+v", sum, whole)
	}
}

func TestAPotThatOpensInsideTheWindowStartsFromNothing(t *testing.T) {
	account := accountOf([]potrules.YieldSighting{
		sighted("2026-03-01", morning1),
		sighted("2026-03-02", morning2, potEntry("pot-a", 45)),
	})
	got := yieldOnly(t, account.DailyYields([]brday.Day{"2026-03-01", "2026-03-02"}))
	if len(got) != 1 || got[0] == nil || *got[0] != 45 {
		t.Fatalf("yield = %v, want the 45 the pot opened with", got)
	}
}

func TestAPotWithNoStableIdentityHasNoRowToFollow(t *testing.T) {
	loose := potrules.YieldEntry{Name: text("Loose"), BRL: true, AmountOK: true, Cents: cents(100)}
	account := accountOf([]potrules.YieldSighting{
		sighted("2026-03-01", morning1, loose), sighted("2026-03-02", morning2, loose),
	})
	if rows := account.DailyYields([]brday.Day{"2026-03-01", "2026-03-02"}); len(rows) != 0 {
		t.Fatalf("rows = %+v, want none for an entry no id names", rows)
	}
	if answered := account.Between("2026-03-01", "2026-03-02"); len(answered) != 2 || *answered[0].Reason != potrules.ReasonUnstable {
		t.Fatalf("the statement's own rows = %+v, want the entry once on each side, unknown", answered)
	}
}
