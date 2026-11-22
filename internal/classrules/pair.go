package classrules

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// TxKey is the key the app's database gives a transaction of its own.
type TxKey int64

// postedWithinDays is how many calendar days after the pending row a posted
// row may fall and still be its replacement.
const postedWithinDays = 5

const secondsPerDay = 24 * 60 * 60

// VanishedPending is a transaction of the index that the bank still reported
// pending when last seen and that the ledger no longer lists.
type VanishedPending struct {
	Key              TxKey
	AccountID        string
	Day              brday.Day
	Cents            money.Cents
	EstablishmentKey string
}

// NewPosted is a posted transaction the ledger lists and the index has never seen.
type NewPosted struct {
	ProviderID       string
	AccountID        string
	Day              brday.Day
	Cents            money.Cents
	EstablishmentKey string
}

// Pairing says that the posted row replaces the pending one, so the person's
// work on the pending row is carried to it.
type Pairing struct {
	PendingKey       TxKey
	PostedProviderID string
}

// Pair answers the pending and posted rows that match one to one: same
// account, cents and establishment key, posted zero to five calendar days after
// the pending day. Any ambiguity pairs nothing, since carrying a category to
// the wrong charge is worse than asking again. The pairs are ordered by key.
func Pair(pending []VanishedPending, posted []NewPosted) ([]Pairing, error) {
	pendingDays, err := daysOf(pending, func(row VanishedPending) brday.Day { return row.Day })
	if err != nil {
		return nil, err
	}
	postedDays, err := daysOf(posted, func(row NewPosted) brday.Day { return row.Day })
	if err != nil {
		return nil, err
	}
	matchesOfPending := make([][]int, len(pending))
	matchesOfPosted := make([]int, len(posted))
	for p, vanished := range pending {
		for q, fresh := range posted {
			if replaces(vanished, fresh, postedDays[q]-pendingDays[p]) {
				matchesOfPending[p] = append(matchesOfPending[p], q)
				matchesOfPosted[q]++
			}
		}
	}
	pairs := []Pairing{}
	for p, matches := range matchesOfPending {
		if len(matches) == 1 && matchesOfPosted[matches[0]] == 1 {
			pairs = append(pairs, Pairing{PendingKey: pending[p].Key, PostedProviderID: posted[matches[0]].ProviderID})
		}
	}
	slices.SortFunc(pairs, func(a, b Pairing) int {
		return cmp.Or(cmp.Compare(a.PendingKey, b.PendingKey), cmp.Compare(a.PostedProviderID, b.PostedProviderID))
	})
	return pairs, nil
}

func replaces(vanished VanishedPending, fresh NewPosted, gap int) bool {
	return vanished.AccountID == fresh.AccountID && vanished.Cents == fresh.Cents &&
		vanished.EstablishmentKey == fresh.EstablishmentKey && gap >= 0 && gap <= postedWithinDays
}

// daysOf numbers every row's day, so that one row with a malformed day fails
// the call whether or not it would have matched anything.
func daysOf[Row any](rows []Row, dayOf func(Row) brday.Day) ([]int, error) {
	numbers := make([]int, len(rows))
	for i, row := range rows {
		number, err := dayNumber(dayOf(row))
		if err != nil {
			return nil, err
		}
		numbers[i] = number
	}
	return numbers, nil
}

// dayNumber counts calendar days from the Unix epoch. A day is read as a UTC
// midnight, which no daylight saving shift moves, so differences are whole days.
func dayNumber(day brday.Day) (int, error) {
	midnight, err := time.Parse(time.DateOnly, string(day))
	if err != nil {
		return 0, fmt.Errorf("classrules: %q is not a YYYY-MM-DD day", day)
	}
	return int(midnight.Unix() / secondsPerDay), nil
}
