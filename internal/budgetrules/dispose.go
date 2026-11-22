package budgetrules

import "github.com/gabrielassisxyz/finwise/internal/money"

// Dispose puts every listed transaction of a month in exactly one disposition
// and says what it counts for. The outcomes are in the order of listed.
//
// A line id the month's budget does not have, the fees line aside, is treated
// as no line wherever a transaction names one: what was paid then shows as
// unassigned instead of going to a line no screen of that month shows.
func Dispose(budget Budget, listed []Transaction) []Outcome {
	outcomes := make([]Outcome, 0, len(listed))
	for _, transaction := range listed {
		outcomes = append(outcomes, budget.dispose(transaction))
	}
	return outcomes
}

func (budget Budget) dispose(transaction Transaction) Outcome {
	disposition := budget.dispositionOf(transaction)
	outcome := Outcome{Disposition: disposition, Magnitude: magnitude(transaction)}
	switch disposition {
	case Split:
		budget.countSplit(transaction, &outcome)
	case Fee:
		outcome.Lines = feeLines(budget.FeesLine, transaction)
	case Refund:
		if line, ok := budget.known(transaction.RefundLine); ok {
			outcome.Lines = []LineAmount{{Line: line, Effect: TakesOff, Amount: transaction.Amount}}
		}
	case Income:
		outcome.Lines = []LineAmount{{Line: transaction.Line.ID, Effect: Received, Amount: transaction.Amount}}
	case OnALine:
		outcome.AppliedReimbursement = appliedReimbursement(transaction)
		outcome.Lines = []LineAmount{{Line: transaction.Line.ID, Effect: Adds, Amount: outcome.Magnitude - outcome.AppliedReimbursement}}
	case Uncategorised:
		outcome.AppliedReimbursement = appliedReimbursement(transaction)
		outcome.Unassigned = outcome.Magnitude - outcome.AppliedReimbursement
	}
	return outcome
}

// dispositionOf is the table of dispositions: the first condition that holds wins.
func (budget Budget) dispositionOf(transaction Transaction) Disposition {
	credit := transaction.Amount > 0
	_, onKnownLine := budget.known(transaction.Line)
	switch {
	case transaction.Unconverted:
		return Unconverted
	case transaction.Movement:
		return Movement
	case transaction.Exclusion == ExcludedWithReason:
		return Excluded
	case transaction.ReviewPending:
		return Waiting
	case !credit && len(transaction.Split) >= 2:
		return Split
	case budget.isFee(transaction):
		return Fee
	case credit && transaction.LinkedToPurchase:
		return Reimbursement
	case credit && isRefund(transaction):
		return Refund
	case credit && onKnownLine && budget.Lines[transaction.Line.ID] == GroupIncome:
		return Income
	case credit:
		return OtherCredit
	case onKnownLine:
		return OnALine
	default:
		return Uncategorised
	}
}

// isFee follows the fees rule the budget already shows: a fee row on no line
// counts once the bank has posted it. One on the fees line counts whoever put
// it there, a person or a rule, and while still pending.
func (budget Budget) isFee(transaction Transaction) bool {
	if !transaction.Fee {
		return false
	}
	line, onKnownLine := budget.known(transaction.Line)
	if onKnownLine {
		return line == budget.FeesLine
	}
	return transaction.Posted
}

func feeLines(feesLine LineID, transaction Transaction) []LineAmount {
	if transaction.Amount > 0 {
		return []LineAmount{{Line: feesLine, Effect: TakesOff, Amount: transaction.Amount}}
	}
	return []LineAmount{{Line: feesLine, Effect: Adds, Amount: -transaction.Amount}}
}

func isRefund(transaction Transaction) bool {
	switch transaction.RefundOverride {
	case RefundYes:
		return true
	case RefundNo:
		return false
	default:
		return transaction.LedgerRefund
	}
}

// countSplit adds each part with a line to it; everything else of the amount,
// parts on no line included, is unassigned.
func (budget Budget) countSplit(transaction Transaction, outcome *Outcome) {
	var assigned money.Cents
	for _, part := range transaction.Split {
		line, ok := budget.known(part.Line)
		if !ok {
			continue
		}
		outcome.Lines = append(outcome.Lines, LineAmount{Line: line, Effect: Adds, Amount: part.Amount})
		assigned += part.Amount
	}
	outcome.Unassigned = outcome.Magnitude - assigned
}

// appliedReimbursement is capped at the purchase so that what it counts for
// never changes sign; a credit larger than the purchase leaves its excess
// counting nowhere.
func appliedReimbursement(transaction Transaction) money.Cents {
	if transaction.LinkedCredit <= 0 {
		return 0
	}
	return min(transaction.LinkedCredit, -transaction.Amount)
}

func (budget Budget) known(ref LineRef) (LineID, bool) {
	if !ref.Set {
		return 0, false
	}
	if ref.ID == budget.FeesLine {
		return ref.ID, true
	}
	_, ok := budget.Lines[ref.ID]
	return ref.ID, ok
}

func magnitude(transaction Transaction) money.Cents {
	if transaction.Unconverted {
		return 0
	}
	if transaction.Amount < 0 {
		return -transaction.Amount
	}
	return transaction.Amount
}
