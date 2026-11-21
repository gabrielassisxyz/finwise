package potrules_test

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

func stablePot(id string) potrules.Pot {
	return potrules.Pot{AccountID: "acc", ReserveID: id, Stable: true}
}

func brdayOf(day string) brday.Day { return brday.Day(day) }

func row(id string, from, to string, yield *money.Cents) potrules.DayYield {
	return potrules.DayYield{AccountID: "acc", ReserveID: id, From: brdayOf(from), To: brdayOf(to), Yield: yield}
}

func TestAMonthsYieldIsTheSumOfItsRowsWithTheRowsBehindIt(t *testing.T) {
	got := potrules.YieldOfMonth(stablePot("pot-a"), potrules.BaseGross, "2026-03", []potrules.DayYield{
		row("pot-a", "2026-03-02", "2026-03-03", cents(12)),
		row("pot-a", "2026-03-01", "2026-03-02", cents(10)),
		row("pot-a", "2026-02-28", "2026-03-01", cents(8)),
		row("pot-a", "2026-03-31", "2026-04-01", cents(99)),
		row("pot-b", "2026-03-02", "2026-03-03", cents(1000)),
	})
	if !got.Known || got.Total != 30 || got.Base != potrules.BaseGross {
		t.Fatalf("got known=%t total=%d base=%s, want known 30 gross", got.Known, got.Total, got.Base)
	}
	if len(got.Days) != 3 || got.Days[0].To != "2026-03-01" || got.Days[2].To != "2026-03-03" {
		t.Fatalf("daily rows = %+v, want the month's three rows oldest first", got.Days)
	}
	if got.Since != "2026-02-28" {
		t.Errorf("since = %s, want 2026-02-28: the first row starts before the month", got.Since)
	}
}

func TestADayWithADepositAndNoYieldReportsZeroForThatDay(t *testing.T) {
	got := potrules.YieldOfMonth(stablePot("pot-a"), potrules.BaseNet, "2026-03", []potrules.DayYield{
		row("pot-a", "2026-03-01", "2026-03-02", cents(0)),
	})
	if !got.Known || got.Total != 0 || *got.Days[0].Yield != 0 {
		t.Fatalf("got %+v, want a known zero: the ledger computed it", got)
	}
}

func TestADayTheLedgerCouldNotComputeMakesTheMonthUnknown(t *testing.T) {
	got := potrules.YieldOfMonth(stablePot("pot-a"), potrules.BaseNet, "2026-03", []potrules.DayYield{
		row("pot-a", "2026-03-01", "2026-03-02", cents(10)),
		row("pot-a", "2026-03-02", "2026-03-03", nil),
		row("pot-a", "2026-03-03", "2026-03-04", cents(10)),
	})
	if got.Known || got.Total != 0 {
		t.Fatalf("got known=%t total=%d, want unknown", got.Known, got.Total)
	}
	if len(got.Days) != 3 {
		t.Errorf("the rows behind an unknown month are kept: %d", len(got.Days))
	}
}

func TestAMonthWithNoRowAtAllIsUnknownNotZero(t *testing.T) {
	got := potrules.YieldOfMonth(stablePot("pot-a"), potrules.BaseNet, "2026-03", []potrules.DayYield{
		row("pot-a", "2026-04-01", "2026-04-02", cents(10)),
	})
	if got.Known {
		t.Fatal("a month nothing was sighted in has a known yield")
	}
}

func TestAPotWithoutAStableIdentityHasAnUnknownYield(t *testing.T) {
	got := potrules.YieldOfMonth(potrules.Pot{AccountID: "acc"}, potrules.BaseNet, "2026-03", []potrules.DayYield{
		row("", "2026-03-01", "2026-03-02", cents(10)),
	})
	if got.Known || len(got.Days) != 0 {
		t.Fatalf("got %+v, want unknown with no rows", got)
	}
}

func TestYieldsOfANetPotAndAGrossPotAreNeverSummedIntoOneFigure(t *testing.T) {
	totals := potrules.TotalYieldsByBase([]potrules.MonthYield{
		{ReserveID: "n1", Base: potrules.BaseNet, Known: true, Total: 100},
		{ReserveID: "g1", Base: potrules.BaseGross, Known: true, Total: 7},
		{ReserveID: "n2", Base: potrules.BaseNet, Known: true, Total: 50},
		{ReserveID: "u1", Base: potrules.BaseUnknown, Known: true, Total: 3},
	})
	want := []potrules.BaseTotal{
		{Base: potrules.BaseGross, Total: 7, Pots: 1},
		{Base: potrules.BaseNet, Total: 150, Pots: 2},
		{Base: potrules.BaseUnknown, Total: 3, Pots: 1},
	}
	if len(totals) != len(want) {
		t.Fatalf("got %d totals, want one per base: %+v", len(totals), totals)
	}
	for index := range want {
		if totals[index] != want[index] {
			t.Errorf("total %d = %+v, want %+v", index, totals[index], want[index])
		}
	}
}

func TestAnUnknownPotIsCountedNotAddedAsZero(t *testing.T) {
	totals := potrules.TotalYieldsByBase([]potrules.MonthYield{
		{Base: potrules.BaseNet, Known: true, Total: 100},
		{Base: potrules.BaseNet, Known: false, Total: 0},
	})
	if len(totals) != 1 || totals[0].Total != 100 || totals[0].Unknown != 1 || totals[0].Pots != 2 {
		t.Fatalf("got %+v, want 100 with one unknown of two pots", totals)
	}
}
