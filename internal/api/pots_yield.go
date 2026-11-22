package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

// attachYields gives each pot to follow its yield for the month, summed from a
// row for every pair of consecutive sightings. The rows come from one read of
// the ledger and the rules in potrules, never from the collector's yield
// statement, whose cost is the same for a day as for a year.
//
// A false withYield is for a reader that needs no yield, such as the home
// summary; every pot is then left with an unknown one.
func (view *potsView) attachYields(ctx context.Context, snapshot *ledger.View, withYield bool, reserves []potEntry) error {
	accounts := accountsToAsk(reserves)
	if !withYield || len(accounts) == 0 {
		return nil
	}
	history, err := snapshot.YieldHistory(ctx)
	if err != nil {
		return err
	}
	rules := yieldRulesOf(history, accounts)
	rows := map[string][]potrules.DayYield{}
	for _, accountID := range accounts {
		from, to, ok := yieldSpanOf(view.sightings[accountID], view.month, view.today)
		if !ok {
			continue
		}
		rows[accountID] = rules[accountID].DailyYields(sightingsFromTo(view.sightings[accountID], from, to))
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

// sightingsFromTo are the sightings from one day to another, both included.
func sightingsFromTo(days []brday.Day, from, to brday.Day) []brday.Day {
	var between []brday.Day
	for _, day := range days {
		if day >= from && day <= to {
			between = append(between, day)
		}
	}
	return between
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
