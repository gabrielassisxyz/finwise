package budgetrules

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

// The cases of the stand-in's contributions (web/src/api/fake/rules.test.ts),
// moved to cents. Where this package departs from it, the departure has a test
// of its own below.
func TestWhatTheStandInCountsIsCountedTheSameInCents(t *testing.T) {
	waiting := on(marketLine, debit(18000))
	waiting.ReviewPending = true

	movement := on(marketLine, debit(18000))
	movement.Movement = true

	excluded := on(marketLine, debit(18000))
	excluded.Exclusion = ExcludedWithReason

	reimbursed := on(travelLine, debit(30000))
	reimbursed.LinkedCredit = 15000

	split := debit(18000)
	split.Split = []SplitPart{{OnLine(marketLine), 10000}, {OnLine(rentLine), 6000}, {LineRef{}, 2000}}

	refundNoLine := credit(12000)
	refundNoLine.RefundOverride = RefundYes

	refundOnFees := refundNoLine
	refundOnFees.RefundLine = OnLine(feesLine)

	checkCases(t, []dispositionCase{
		{"a purchase on its line for what was paid", on(marketLine, debit(18000)), Outcome{Disposition: OnALine, Lines: []LineAmount{adds(marketLine, 18000)}, Magnitude: 18000}},
		{"nothing while a review is pending, even with a line set", waiting, nothing(Waiting, 18000)},
		{"nothing for a movement between own accounts", movement, nothing(Movement, 18000)},
		{"nothing once excluded with a reason", excluded, nothing(Excluded, 18000)},
		{"a linked reimbursement taken off the purchase", reimbursed, Outcome{Disposition: OnALine, Lines: []LineAmount{adds(travelLine, 15000)}, AppliedReimbursement: 15000, Magnitude: 30000}},
		{"a split spread over its parts, a part without a line on none", split, Outcome{Disposition: Split, Lines: []LineAmount{adds(marketLine, 10000), adds(rentLine, 6000)}, Unassigned: 2000, Magnitude: 18000}},
		{"a credit on a line that is not income is ignored", on(rentLine, credit(20000)), nothing(OtherCredit, 20000)},
		{"a refund pointed at no line counts nothing", refundNoLine, nothing(Refund, 12000)},
		{"a refund pointed at a line reduces it", refundOnFees, Outcome{Disposition: Refund, Lines: []LineAmount{takesOff(feesLine, 12000)}, Magnitude: 12000}},
	})
}

// The stand-in keeps a seeded baseline of spending per month and moves only
// the current one; it ignores a credit on an income line. Here a month's
// spending is what its own transactions add, for any month, and income
// received is counted.
func TestUnlikeTheStandInAnyMonthIsSummedFromItsTransactionsAndIncomeIsReceived(t *testing.T) {
	months := map[string][]Transaction{
		"2026-10": {on(marketLine, debit(26500)), on(marketLine, debit(4200)), on(salaryLine, credit(820000))},
		"2026-11": {on(marketLine, debit(30800)), on(salaryLine, credit(20000))},
	}
	want := map[string]struct{ spent, received money.Cents }{
		"2026-10": {30700, 820000},
		"2026-11": {30800, 20000},
	}
	for name, listed := range months {
		var spent, received money.Cents
		for _, outcome := range Dispose(month, listed) {
			for _, line := range outcome.Lines {
				switch {
				case line.Line == marketLine && line.Effect == Adds:
					spent += line.Amount
				case line.Line == salaryLine && line.Effect == Received:
					received += line.Amount
				}
			}
		}
		if spent != want[name].spent || received != want[name].received {
			t.Errorf("%s: spent %d and received %d, want %d and %d", name, spent, received, want[name].spent, want[name].received)
		}
	}
}
