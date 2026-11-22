package budgetrules

import (
	"cmp"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

// KeyedOutcome is a listed transaction's establishment key and what Dispose
// decided for it.
type KeyedOutcome struct {
	EstablishmentKey string
	Outcome          Outcome
}

// EstablishmentTotal is what one establishment adds to a line in a month and
// how many of its transactions do.
type EstablishmentTotal struct {
	Key    string
	Amount money.Cents
	Count  int
}

// EstablishmentsOfLine groups by establishment the transactions that bear on
// line. A transaction counts once, with the signed sum of everything it does
// to the line, even when that is zero or comes from two parts of one split, so
// the amounts add up to the line's figure. The biggest comes first, and a tie
// is ordered by key so the list reads the same on every call.
func EstablishmentsOfLine(line LineID, listed []KeyedOutcome) []EstablishmentTotal {
	byKey := map[string]*EstablishmentTotal{}
	var order []string
	for _, transaction := range listed {
		sum, bears := onLine(line, transaction.Outcome)
		if !bears {
			continue
		}
		total, seen := byKey[transaction.EstablishmentKey]
		if !seen {
			total = &EstablishmentTotal{Key: transaction.EstablishmentKey}
			byKey[transaction.EstablishmentKey] = total
			order = append(order, transaction.EstablishmentKey)
		}
		total.Amount += sum
		total.Count++
	}
	totals := make([]EstablishmentTotal, 0, len(order))
	for _, key := range order {
		totals = append(totals, *byKey[key])
	}
	slices.SortFunc(totals, func(a, b EstablishmentTotal) int {
		if byAmount := cmp.Compare(b.Amount, a.Amount); byAmount != 0 {
			return byAmount
		}
		return cmp.Compare(a.Key, b.Key)
	})
	return totals
}

func onLine(line LineID, outcome Outcome) (money.Cents, bool) {
	var sum money.Cents
	bears := false
	for _, amount := range outcome.Lines {
		if amount.Line == line {
			sum += amount.Signed()
			bears = true
		}
	}
	return sum, bears
}
