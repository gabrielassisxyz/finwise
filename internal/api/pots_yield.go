package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

// attachYields asks the yield statement once for each account that has a pot
// to follow, over the span of the month's sightings. The statement answers for
// the whole span between two sightings, so the cost of a request is one call per
// account and does not grow with the number of sightings; asking for each pair of
// consecutive sightings took about seventy seconds over a real ledger.
//
// What comes back is one figure per pot for the month, not one per day. A nil
// asker is for a reader that needs no yield, such as the home summary; every pot
// is then left with an unknown one.
func (view *potsView) attachYields(ctx context.Context, ask yieldAsker, reserves []potEntry) error {
	if ask == nil {
		return nil
	}
	rows := map[string][]potrules.DayYield{}
	for _, accountID := range accountsToAsk(reserves) {
		from, to, ok := yieldSpanOf(view.sightings[accountID], view.month, view.today)
		if !ok {
			continue
		}
		answered, err := ask(ctx, accountID, from, to)
		if err != nil {
			return err
		}
		rows[accountID] = dayYieldsOf(answered)
	}
	for index := range reserves {
		entry := &reserves[index]
		if !entry.yieldAsked {
			continue
		}
		pot := potrules.Pot{AccountID: entry.accountID, ReserveID: entry.reserveID, Stable: true}
		entry.yield = potrules.YieldOfMonth(pot, entry.base, view.month, rows[entry.accountID])
	}
	return nil
}

func accountsToAsk(reserves []potEntry) []string {
	seen := map[string]bool{}
	var accounts []string
	for _, entry := range reserves {
		if entry.yieldAsked && !seen[entry.accountID] {
			seen[entry.accountID] = true
			accounts = append(accounts, entry.accountID)
		}
	}
	return accounts
}

// yieldSpanOf is the span of an account's sightings that holds the month: from
// the sighting of the last day before the month, or the first sighting of the
// month when that night was missed, to the latest sighting up to today. A
// sighting further back would put yield earned before the month into it, and
// the contract has no field to say so. A span that does not reach into the
// month, or has no length, is not asked for, and the month's yield is unknown.
func yieldSpanOf(days []brday.Day, month brday.Month, today brday.Day) (from, to brday.Day, ok bool) {
	first := brday.Day(string(month) + "-01")
	before, _ := shiftedDay(first, -1)
	for _, day := range days {
		switch {
		case day == before:
			from = day
		case day >= first && day <= today:
			if from == "" {
				from = day
			}
			to = day
		}
	}
	return from, to, to >= first && from < to
}

// dayYieldsOf keeps the rows that name a pot to follow; a row with no reserve is
// an unmatched transfer or an entry with no identity, and belongs to no pot.
func dayYieldsOf(answered []ledger.PotYield) []potrules.DayYield {
	var rows []potrules.DayYield
	for _, yield := range answered {
		if yield.ReserveID == nil {
			continue
		}
		rows = append(rows, potrules.DayYield{
			AccountID: yield.AccountID, ReserveID: *yield.ReserveID, From: yield.From, To: yield.To, Yield: yield.Yield,
		})
	}
	return rows
}

// monthlyYieldOf is what a pot earned in the month, nil when that cannot be
// said. A closed pot holds nothing and earns nothing, which is a zero the
// ledger supports; an investment and a pot with no identity have no statement
// to ask, so their yield is unknown and never a zero.
func monthlyYieldOf(entry potEntry) *money.Cents {
	if entry.closed {
		zero := money.Cents(0)
		return &zero
	}
	if !entry.yieldAsked || !entry.yield.Known {
		return nil
	}
	total := entry.yield.Total
	return &total
}
