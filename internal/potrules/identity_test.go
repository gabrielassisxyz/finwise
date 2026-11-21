package potrules_test

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

func text(value string) *string { return &value }

func cents(value money.Cents) *money.Cents { return &value }

func stable(account string, day brday.Day, id, name string, balance money.Cents) potrules.Sighting {
	return potrules.Sighting{
		AccountID: account, Day: day, ReserveID: text(id), Name: text(name),
		StableIdentity: true, Balance: cents(balance),
	}
}

func loose(account string, day brday.Day, name string, balance money.Cents) potrules.Sighting {
	return potrules.Sighting{AccountID: account, Day: day, Name: text(name), Balance: cents(balance)}
}

func potWithID(t *testing.T, pots []potrules.Pot, id string) potrules.Pot {
	t.Helper()
	for _, pot := range pots {
		if pot.ReserveID == id {
			return pot
		}
	}
	t.Fatalf("no pot with id %q among %d pots", id, len(pots))
	return potrules.Pot{}
}

func TestTwoPotsWithOneNameAreTwoPotsWithSeparateBalances(t *testing.T) {
	pots := potrules.Identify([]potrules.Sighting{
		stable("acc", "2026-03-14", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-14", "pot-b", "Trip", 2500),
		stable("acc", "2026-03-15", "pot-a", "Trip", 1100),
		stable("acc", "2026-03-15", "pot-b", "Trip", 2600),
	})
	if len(pots) != 2 {
		t.Fatalf("got %d pots, want 2", len(pots))
	}
	first, second := potWithID(t, pots, "pot-a"), potWithID(t, pots, "pot-b")
	if *first.Balance != 1100 || *second.Balance != 2600 {
		t.Errorf("balances %d and %d, want 1100 and 2600", *first.Balance, *second.Balance)
	}
	if len(first.History) != 2 || *first.History[0].Balance != 1000 || *second.History[0].Balance != 2500 {
		t.Errorf("histories got mixed up: %+v and %+v", first.History, second.History)
	}
}

func TestAPotWithoutAStableIdentityIsReportedForItsLatestDayOnly(t *testing.T) {
	pots := potrules.Identify([]potrules.Sighting{
		loose("acc", "2026-03-14", "Loose", 500),
		loose("acc", "2026-03-15", "Loose", 600),
		stable("acc", "2026-03-15", "pot-a", "Kept", 100),
	})
	if len(pots) != 2 {
		t.Fatalf("got %d pots, want the stable one and the loose one of the latest day", len(pots))
	}
	unfollowed := pots[0]
	if unfollowed.Stable || unfollowed.ReserveID != "" || len(unfollowed.History) != 0 {
		t.Errorf("loose pot = %+v, want unstable with no id and no history", unfollowed)
	}
	if unfollowed.LatestDay != "2026-03-15" || unfollowed.Balance == nil || *unfollowed.Balance != 600 {
		t.Errorf("loose pot is not the latest day's: %+v", unfollowed)
	}
}

func TestALooseEntryOfAnEarlierDayIsNotReported(t *testing.T) {
	pots := potrules.Identify([]potrules.Sighting{
		loose("acc", "2026-03-14", "Gone", 500),
		stable("acc", "2026-03-15", "pot-a", "Kept", 100),
	})
	if len(pots) != 1 || pots[0].ReserveID != "pot-a" {
		t.Fatalf("got %+v, want only the stable pot", pots)
	}
}

func TestAPotAbsentFromTheLatestSightingIsClosed(t *testing.T) {
	pots := potrules.Identify([]potrules.Sighting{
		stable("acc", "2026-03-14", "pot-a", "Old", 700),
		stable("acc", "2026-03-14", "pot-b", "Kept", 100),
		stable("acc", "2026-03-15", "pot-b", "Kept", 110),
	})
	closed, open := potWithID(t, pots, "pot-a"), potWithID(t, pots, "pot-b")
	if !closed.Closed || closed.Balance != nil || closed.LatestDay != "2026-03-14" {
		t.Errorf("closed pot = %+v, want closed, no balance, last seen 2026-03-14", closed)
	}
	if len(closed.History) != 1 || *closed.History[0].Balance != 700 {
		t.Errorf("a closed pot keeps its history: %+v", closed.History)
	}
	if open.Closed || *open.Balance != 110 {
		t.Errorf("open pot = %+v, want open with 110", open)
	}
}

func TestAnotherAccountsLatestDayDoesNotCloseAPot(t *testing.T) {
	pots := potrules.Identify([]potrules.Sighting{
		stable("acc-1", "2026-03-10", "pot-a", "Kept", 700),
		stable("acc-2", "2026-03-15", "pot-b", "Other", 100),
	})
	if potWithID(t, pots, "pot-a").Closed {
		t.Fatal("pot-a was closed by a sighting of another account")
	}
}

func TestAPotWhoseAmountCannotBeReadHasNoBalanceNotZero(t *testing.T) {
	unreadable := stable("acc", "2026-03-15", "pot-a", "Odd", 0)
	unreadable.Balance = nil
	pots := potrules.Identify([]potrules.Sighting{unreadable})
	if pots[0].Balance != nil {
		t.Fatalf("balance = %d, want none", *pots[0].Balance)
	}
}

func TestThePotsNameAndRateAreThoseOfItsLatestSighting(t *testing.T) {
	older := stable("acc", "2026-03-14", "pot-a", "Before", 1)
	newer := stable("acc", "2026-03-15", "pot-a", "After", 2)
	rate := 110.0
	newer.CDIPercent = &rate
	pot := potrules.Identify([]potrules.Sighting{newer, older})[0]
	if *pot.Name != "After" || pot.CDIPercent == nil || *pot.CDIPercent != 110 {
		t.Fatalf("pot = %+v, want the newer name and rate whatever the input order", pot)
	}
}

func TestAnIdThatTwoEntriesShareDoesNotIdentifyEitherOfThem(t *testing.T) {
	duplicate := func(day brday.Day, balance money.Cents) potrules.Sighting {
		sighting := stable("acc", day, "pot-a", "Twin", balance)
		sighting.StableIdentity = false
		return sighting
	}
	pots := potrules.Identify([]potrules.Sighting{
		duplicate("2026-03-14", 100), duplicate("2026-03-14", 200),
		duplicate("2026-03-15", 110), duplicate("2026-03-15", 210),
	})
	if len(pots) != 2 {
		t.Fatalf("got %d pots, want the two entries of the latest day", len(pots))
	}
	for _, pot := range pots {
		if pot.Stable || pot.ReserveID != "" || len(pot.History) != 0 {
			t.Errorf("pot = %+v, want no identity and no history", pot)
		}
	}
}
