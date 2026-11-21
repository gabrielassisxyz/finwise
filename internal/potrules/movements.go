package potrules

import (
	"sort"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Movement is a deposit or withdrawal as it sits on the account: it does not
// name the pot. Change is the amount by which it changes a pot's balance, positive
// for a deposit into a pot. Working that out from a transaction's kind and sign is
// the caller's job, not this package's.
type Movement struct {
	AccountID string
	Day       brday.Day
	Change    money.Cents
}

// PotRef names the pot a movement was attributed to; ReserveID is empty when the
// pot's identity is not stable and Name is all there is.
type PotRef struct {
	AccountID string
	ReserveID string
	Name      *string
}

// AttributedMovement is a movement and the pot it belongs to.
type AttributedMovement struct {
	Movement Movement
	Pot      PotRef
}

// Attribution splits movements into those placed on a pot and those that stay on
// the account, which the caller counts.
type Attribution struct {
	Attributed   []AttributedMovement
	Unattributed []Movement
}

// maxLagDays is how far after a movement the sighting that shows its effect may
// be. Sightings are nightly in the ledger today; past this a gap in the history
// has swallowed the day, and what the nearest sighting holds says nothing about
// the movement.
const maxLagDays = 3

// AttributeMovements places each movement on a pot. The smallest safe rule: the
// movement belongs to a pot when the account's sighting that follows it holds that
// one pot only, or when exactly one pot's balance moved by the movement's amount
// between that sighting and the one before it (a pot new in the sighting moved
// from zero). Anything else stays on the account.
//
// The sighting that shows a movement is the first one dated after its day. The
// collector sights an account in the small hours of the local day it labels it
// with (the ledger's own observation times: most are about 02:00 in Brazil), so a
// movement of the day is in the balance of the next day's sighting; the one dated
// the same day was taken before it. A balance moves by its yield as well as by a
// deposit, so a pot that earned in the same interval does not match by amount and
// its movement stays unattributed; that is the cost of not guessing.
func AttributeMovements(sightings []Sighting, movements []Movement) Attribution {
	history := sightingsByAccountAndDay(sightings)
	var result Attribution
	for _, movement := range movements {
		if pot, ok := history.potOf(movement); ok {
			result.Attributed = append(result.Attributed, AttributedMovement{Movement: movement, Pot: pot})
		} else {
			result.Unattributed = append(result.Unattributed, movement)
		}
	}
	return result
}

type accountHistory struct {
	days  []brday.Day
	byDay map[brday.Day][]Sighting
}

type sightingIndex map[string]*accountHistory

func sightingsByAccountAndDay(sightings []Sighting) sightingIndex {
	index := sightingIndex{}
	for _, sighting := range sightings {
		history := index[sighting.AccountID]
		if history == nil {
			history = &accountHistory{byDay: map[brday.Day][]Sighting{}}
			index[sighting.AccountID] = history
		}
		history.byDay[sighting.Day] = append(history.byDay[sighting.Day], sighting)
	}
	for _, history := range index {
		for day := range history.byDay {
			history.days = append(history.days, day)
		}
		sort.Slice(history.days, func(i, j int) bool { return history.days[i] < history.days[j] })
	}
	return index
}

func (index sightingIndex) potOf(movement Movement) (PotRef, bool) {
	history := index[movement.AccountID]
	if history == nil {
		return PotRef{}, false
	}
	position := sort.Search(len(history.days), func(i int) bool { return history.days[i] > movement.Day })
	if position == len(history.days) || !withinLag(movement.Day, history.days[position]) {
		return PotRef{}, false
	}
	shown := history.byDay[history.days[position]]
	if len(shown) == 1 {
		return refOf(shown[0]), true
	}
	if position == 0 {
		return PotRef{}, false
	}
	return matchByChange(shown, history.byDay[history.days[position-1]], movement.Change)
}

func withinLag(movement, sighted brday.Day) bool {
	from, fromErr := time.Parse("2006-01-02", string(movement))
	to, toErr := time.Parse("2006-01-02", string(sighted))
	return fromErr == nil && toErr == nil && to.Sub(from) <= maxLagDays*24*time.Hour
}

// matchByChange returns the pot whose balance moved by change, when exactly one did.
func matchByChange(after, before []Sighting, change money.Cents) (PotRef, bool) {
	var matches []Sighting
	for _, now := range after {
		if now.Balance == nil || !now.StableIdentity || now.ReserveID == nil {
			continue
		}
		was, known := balanceBefore(before, *now.ReserveID)
		if known && *now.Balance-was == change {
			matches = append(matches, now)
		}
	}
	if len(matches) != 1 {
		return PotRef{}, false
	}
	return refOf(matches[0]), true
}

// balanceBefore is the balance of a stable pot in the earlier sighting; a pot
// that was not there starts from zero, and one whose amount was unreadable is unknown.
func balanceBefore(before []Sighting, reserveID string) (money.Cents, bool) {
	for _, sighting := range before {
		if sighting.StableIdentity && sighting.ReserveID != nil && *sighting.ReserveID == reserveID {
			if sighting.Balance == nil {
				return 0, false
			}
			return *sighting.Balance, true
		}
	}
	return 0, true
}

func refOf(sighting Sighting) PotRef {
	ref := PotRef{AccountID: sighting.AccountID, Name: sighting.Name}
	if sighting.StableIdentity && sighting.ReserveID != nil {
		ref.ReserveID = *sighting.ReserveID
	}
	return ref
}
