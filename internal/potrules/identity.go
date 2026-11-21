// Package potrules works out what the ledger can say about savings pots: which
// pot is which over time, what each one holds, what it yielded in a month, and
// which pot a deposit or withdrawal belongs to. Everything is a pure function over
// plain values the caller read from the ledger. Where the ledger cannot support an
// answer the result says it is unknown; it is never a zero.
package potrules

import (
	"sort"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Sighting is one reserve entry in an account's latest sighting of a day, which
// is what the ledger's pot reader returns for a day.
type Sighting struct {
	AccountID string
	Day       brday.Day
	ReserveID *string
	Name      *string
	// StableIdentity is the ledger's verdict that ReserveID names this entry and
	// no other in its sighting. Names repeat, so a name never identifies a pot.
	StableIdentity bool
	// Balance is nil when the ledger could not read the amount.
	Balance    *money.Cents
	CDIPercent *float64
}

// Point is a pot's balance on a day it was sighted.
type Point struct {
	Day     brday.Day
	Balance *money.Cents
}

// Pot is one pot as the ledger lets it be followed.
type Pot struct {
	AccountID string
	// ReserveID is empty for a pot whose identity is not stable.
	ReserveID string
	Stable    bool
	// Name and CDIPercent are those of the pot's latest sighting.
	Name       *string
	CDIPercent *float64
	// LatestDay is the day of the pot's latest sighting.
	LatestDay brday.Day
	// Closed means the pot is absent from the latest sighting of its account.
	Closed bool
	// Balance is the latest balance: nil for a closed pot, which holds nothing now,
	// and for one whose amount the ledger could not read.
	Balance *money.Cents
	// History holds one point per sighting, oldest first. A pot whose identity is
	// not stable cannot be followed across days and has none.
	History []Point
}

// Identify turns sightings of many days into pots. A stable pot is the sightings
// that share an account and a reserve id; two pots with one name are two pots. A
// pot without a stable identity is reported for the account's latest day only,
// since nothing says which entry of another day it was.
func Identify(sightings []Sighting) []Pot {
	latestDay := latestDayByAccount(sightings)
	followed := map[[2]string][]Sighting{}
	var pots []Pot
	for _, sighting := range sightings {
		switch {
		case sighting.StableIdentity && sighting.ReserveID != nil:
			key := [2]string{sighting.AccountID, *sighting.ReserveID}
			followed[key] = append(followed[key], sighting)
		case sighting.Day == latestDay[sighting.AccountID]:
			pots = append(pots, unfollowedPot(sighting))
		}
	}
	for _, own := range followed {
		pots = append(pots, followedPot(own, latestDay[own[0].AccountID]))
	}
	sort.SliceStable(pots, func(i, j int) bool { return potLess(pots[i], pots[j]) })
	return pots
}

func latestDayByAccount(sightings []Sighting) map[string]brday.Day {
	latest := map[string]brday.Day{}
	for _, sighting := range sightings {
		latest[sighting.AccountID] = max(latest[sighting.AccountID], sighting.Day)
	}
	return latest
}

func unfollowedPot(sighting Sighting) Pot {
	return Pot{
		AccountID: sighting.AccountID, Name: sighting.Name, CDIPercent: sighting.CDIPercent,
		LatestDay: sighting.Day, Balance: sighting.Balance,
	}
}

func followedPot(own []Sighting, accountLatest brday.Day) Pot {
	sort.SliceStable(own, func(i, j int) bool { return own[i].Day < own[j].Day })
	last := own[len(own)-1]
	pot := Pot{
		AccountID: last.AccountID, ReserveID: *last.ReserveID, Stable: true,
		Name: last.Name, CDIPercent: last.CDIPercent, LatestDay: last.Day,
		Closed: last.Day < accountLatest,
	}
	if !pot.Closed {
		pot.Balance = last.Balance
	}
	for _, sighting := range own {
		pot.History = append(pot.History, Point{Day: sighting.Day, Balance: sighting.Balance})
	}
	return pot
}

func potLess(a, b Pot) bool {
	if a.AccountID != b.AccountID {
		return a.AccountID < b.AccountID
	}
	if a.ReserveID != b.ReserveID {
		return a.ReserveID < b.ReserveID
	}
	return derefOrEmpty(a.Name) < derefOrEmpty(b.Name)
}

func derefOrEmpty(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}
