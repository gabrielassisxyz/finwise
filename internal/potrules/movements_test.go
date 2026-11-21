package potrules_test

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

func deposit(account string, day brday.Day, change money.Cents) potrules.Movement {
	return potrules.Movement{AccountID: account, Day: day, Change: change}
}

func TestAMovementOnAnAccountWithOnePotIsAttributedToIt(t *testing.T) {
	result := potrules.AttributeMovements([]potrules.Sighting{
		stable("acc", "2026-03-14", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-15", "pot-a", "Trip", 1500),
	}, []potrules.Movement{deposit("acc", "2026-03-14", 500)})

	if len(result.Attributed) != 1 || result.Attributed[0].Pot.ReserveID != "pot-a" || len(result.Unattributed) != 0 {
		t.Fatalf("got %+v, want the movement on pot-a", result)
	}
}

func TestOnAnAccountWithOnePotTheAmountNeedNotMatchTheChange(t *testing.T) {
	// The pot earned between the sightings, so its balance moved by more than the deposit.
	result := potrules.AttributeMovements([]potrules.Sighting{
		stable("acc", "2026-03-14", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-15", "pot-a", "Trip", 1507),
	}, []potrules.Movement{deposit("acc", "2026-03-14", 500)})

	if len(result.Attributed) != 1 || len(result.Unattributed) != 0 {
		t.Fatalf("got %+v, want the movement on the account's only pot", result)
	}
}

func TestOnAnAccountWithTwoPotsAMovementNeedsExactlyOneBalanceToHaveChangedByItsAmount(t *testing.T) {
	sightings := []potrules.Sighting{
		stable("acc", "2026-03-14", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-14", "pot-b", "Home", 2000),
		stable("acc", "2026-03-15", "pot-a", "Trip", 1500),
		stable("acc", "2026-03-15", "pot-b", "Home", 2000),
	}
	result := potrules.AttributeMovements(sightings, []potrules.Movement{deposit("acc", "2026-03-14", 500)})
	if len(result.Attributed) != 1 || result.Attributed[0].Pot.ReserveID != "pot-a" {
		t.Fatalf("got %+v, want the movement on pot-a, the only balance that moved by 500", result)
	}
}

func TestAMovementIsUnattributedWhenTwoBalancesMovedByItsAmount(t *testing.T) {
	sightings := []potrules.Sighting{
		stable("acc", "2026-03-14", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-14", "pot-b", "Home", 2000),
		stable("acc", "2026-03-15", "pot-a", "Trip", 1500),
		stable("acc", "2026-03-15", "pot-b", "Home", 2500),
	}
	result := potrules.AttributeMovements(sightings, []potrules.Movement{deposit("acc", "2026-03-14", 500)})
	if len(result.Attributed) != 0 || len(result.Unattributed) != 1 {
		t.Fatalf("got %+v, want the movement left on the account", result)
	}
}

func TestAMovementIsUnattributedWhenNoBalanceMovedByItsAmount(t *testing.T) {
	sightings := []potrules.Sighting{
		stable("acc", "2026-03-14", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-14", "pot-b", "Home", 2000),
		stable("acc", "2026-03-15", "pot-a", "Trip", 1501),
		stable("acc", "2026-03-15", "pot-b", "Home", 2000),
	}
	result := potrules.AttributeMovements(sightings, []potrules.Movement{deposit("acc", "2026-03-14", 500)})
	if len(result.Attributed) != 0 || len(result.Unattributed) != 1 {
		t.Fatalf("got %+v, want the movement left on the account", result)
	}
}

func TestAPotOpenedByTheMovementMovedFromZero(t *testing.T) {
	sightings := []potrules.Sighting{
		stable("acc", "2026-03-14", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-15", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-15", "pot-b", "New", 300),
	}
	result := potrules.AttributeMovements(sightings, []potrules.Movement{deposit("acc", "2026-03-14", 300)})
	if len(result.Attributed) != 1 || result.Attributed[0].Pot.ReserveID != "pot-b" {
		t.Fatalf("got %+v, want the movement on the new pot-b", result)
	}
}

func TestAWithdrawalMatchesANegativeChange(t *testing.T) {
	sightings := []potrules.Sighting{
		stable("acc", "2026-03-14", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-14", "pot-b", "Home", 2000),
		stable("acc", "2026-03-15", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-15", "pot-b", "Home", 1200),
	}
	result := potrules.AttributeMovements(sightings, []potrules.Movement{deposit("acc", "2026-03-14", -800)})
	if len(result.Attributed) != 1 || result.Attributed[0].Pot.ReserveID != "pot-b" {
		t.Fatalf("got %+v, want the withdrawal on pot-b", result)
	}
}

func TestAMovementOnADayWithNoNearbySightingStaysOnTheAccount(t *testing.T) {
	sightings := []potrules.Sighting{
		stable("acc", "2026-03-14", "pot-a", "Trip", 1000),
		stable("acc", "2026-06-01", "pot-a", "Trip", 1500),
	}
	movements := []potrules.Movement{
		deposit("acc", "2026-04-01", 500),
		deposit("acc", "2026-06-10", 500),
		deposit("other", "2026-03-14", 500),
	}
	result := potrules.AttributeMovements(sightings, movements)
	if len(result.Attributed) != 0 || len(result.Unattributed) != 3 {
		t.Fatalf("got %+v, want all three left on their accounts", result)
	}
}

func TestAnUnreadableBalanceNeverMatches(t *testing.T) {
	unreadable := stable("acc", "2026-03-15", "pot-a", "Trip", 0)
	unreadable.Balance = nil
	sightings := []potrules.Sighting{
		stable("acc", "2026-03-14", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-14", "pot-b", "Home", 2000),
		unreadable,
		stable("acc", "2026-03-15", "pot-b", "Home", 2000),
	}
	result := potrules.AttributeMovements(sightings, []potrules.Movement{deposit("acc", "2026-03-14", 0)})
	if len(result.Attributed) != 1 || result.Attributed[0].Pot.ReserveID != "pot-b" {
		t.Fatalf("got %+v, want only pot-b, whose balance is known not to have moved", result)
	}
}

func TestTheFirstSightingOfManyPotsCannotShowAChange(t *testing.T) {
	sightings := []potrules.Sighting{
		stable("acc", "2026-03-15", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-15", "pot-b", "Home", 2000),
	}
	result := potrules.AttributeMovements(sightings, []potrules.Movement{deposit("acc", "2026-03-14", 1000)})
	if len(result.Attributed) != 0 {
		t.Fatalf("got %+v, want nothing attributed: there is no earlier sighting to compare with", result)
	}
}

func TestAMovementIsShownByTheSightingAfterItsDayNotTheOneOfItsDay(t *testing.T) {
	// The sighting dated the 14th was taken before the movement of the 14th, so
	// its balances cannot show it: the 15th's is compared with the 14th's.
	sightings := []potrules.Sighting{
		stable("acc", "2026-03-13", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-13", "pot-b", "Home", 2000),
		stable("acc", "2026-03-14", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-14", "pot-b", "Home", 2000),
		stable("acc", "2026-03-15", "pot-a", "Trip", 1000),
		stable("acc", "2026-03-15", "pot-b", "Home", 2400),
	}
	result := potrules.AttributeMovements(sightings, []potrules.Movement{deposit("acc", "2026-03-14", 400)})
	if len(result.Attributed) != 1 || result.Attributed[0].Pot.ReserveID != "pot-b" {
		t.Fatalf("got %+v, want the movement of the 14th on pot-b, which moved on the 15th", result)
	}
}
