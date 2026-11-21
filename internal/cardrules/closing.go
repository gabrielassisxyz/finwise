// Package cardrules works out a card's bill cycle: when a bill closes, which
// bill a purchase falls on, and how much of the month's spending the next bill
// will charge. Everything is a pure function over values the caller read from
// the ledger; which bank a card belongs to, and so which closing table applies,
// is the caller's to say.
package cardrules

import (
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// ClosingTable maps the weekday of a bill's nominal due date to the number of
// days from the closing date to that due date. A weekday with no entry, and a
// nil table, mean the bank's closing rule is not known.
type ClosingTable map[time.Weekday]int

// C6ClosingTable is the rule read off the C6 bills in the ledger: the closing
// date depends only on the weekday of the nominal due date, with no exception
// across the bills examined. The ledger keeps the nominal due date; the bank
// moves a due date on a non-business day to the next business day for payment,
// and that does not change the closing date. The two Itaú cards show no single
// offset per weekday, so there is no table for them.
func C6ClosingTable() ClosingTable {
	return ClosingTable{
		time.Monday: 6, time.Tuesday: 6, time.Wednesday: 6, time.Thursday: 6,
		time.Friday: 7, time.Saturday: 7, time.Sunday: 8,
	}
}

// ClosingDate is the day the bill due on the given nominal date closes, which is
// the first day of the next cycle: a purchase on that day belongs to the next
// bill. ok is false when the table has no rule for the due date's weekday.
func (table ClosingTable) ClosingDate(due brday.Day) (closing brday.Day, ok bool) {
	parsed, valid := parseDay(due)
	if !valid {
		return "", false
	}
	offset, known := table[parsed.Weekday()]
	if !known {
		return "", false
	}
	return addDays(due, -offset)
}

// ManualClosingDate is the closing date when the person chooses the closing day
// of the month: the latest such day strictly before the due date, held to the
// month's last day in a shorter month. The due date does not move.
func ManualClosingDate(due brday.Day, closingDay int) (closing brday.Day, ok bool) {
	parsed, valid := parseDay(due)
	if !valid || closingDay < 1 || closingDay > 31 {
		return "", false
	}
	sameMonth := dayOfMonth(parsed.Year(), parsed.Month(), closingDay)
	if sameMonth.Before(parsed) {
		return formatDay(sameMonth), true
	}
	return formatDay(dayOfMonth(parsed.Year(), parsed.Month()-1, closingDay)), true
}
