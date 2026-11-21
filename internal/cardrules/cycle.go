package cardrules

import (
	"sort"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Bill is a bill of the ledger as the rules need it. Closing is the first day of
// the next cycle, and nil when the ledger does not carry one.
type Bill struct {
	ID      string
	Due     brday.Day
	Closing *brday.Day
}

// Purchase is a card purchase as the rules need it: the Brazil day it was made
// on, the bill the provider placed it on when it says so, and its amount, which
// the rules only add up, in whatever sign convention the caller chose.
type Purchase struct {
	ID     string
	Day    brday.Day
	BillID *string
	Amount money.Cents
}

// Card is one card account: its bills as the ledger holds them, the closing rule
// of its bank when there is one, and the closing day the person set, if any.
type Card struct {
	Bills []Bill
	// Table is nil for a bank with no known rule; closing dates the ledger does
	// not carry are then unknown rather than guessed.
	Table ClosingTable
	// ManualClosingDay, when not zero, replaces the table for every closing date
	// the ledger does not carry. A closing date the ledger does carry is what the
	// bank did, and a setting made now does not rewrite it.
	ManualClosingDay int
}

// maxPredictedCycles bounds how far past the latest bill a purchase can be
// placed: the ledger lags the bank by about one cycle, not more.
const maxPredictedCycles = 3

// cycle is one bill cycle: a bill of the ledger, or one predicted after the
// latest of them.
type cycle struct {
	billID    string
	due       brday.Day
	closing   brday.Day
	known     bool
	predicted bool
}

func (card Card) predictedClosing(due brday.Day) (brday.Day, bool) {
	if card.ManualClosingDay != 0 {
		return ManualClosingDate(due, card.ManualClosingDay)
	}
	return card.Table.ClosingDate(due)
}

func (card Card) ledgerCycles() []cycle {
	bills := append([]Bill(nil), card.Bills...)
	sort.SliceStable(bills, func(i, j int) bool { return bills[i].Due < bills[j].Due })
	cycles := make([]cycle, 0, len(bills))
	for _, bill := range bills {
		entry := cycle{billID: bill.ID, due: bill.Due}
		if bill.Closing != nil {
			entry.closing, entry.known = *bill.Closing, true
		} else {
			entry.closing, entry.known = card.predictedClosing(bill.Due)
		}
		cycles = append(cycles, entry)
	}
	return cycles
}

// cycles is the ledger's bills followed by the cycles predicted after the latest
// one, from its due day.
func (card Card) cycles() []cycle {
	cycles := card.ledgerCycles()
	if len(cycles) == 0 {
		return nil
	}
	latest := cycles[len(cycles)-1]
	for step := 1; step <= maxPredictedCycles; step++ {
		due, ok := dueInMonthsAfter(latest.due, step)
		if !ok {
			break
		}
		entry := cycle{due: due, predicted: true}
		entry.closing, entry.known = card.predictedClosing(due)
		cycles = append(cycles, entry)
	}
	return cycles
}
