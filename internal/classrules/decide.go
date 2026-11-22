// Package classrules decides by rule what happens to a transaction the app has
// never evaluated, and which bank-pending transaction a newly posted one
// replaces. Both are pure functions over plain values the caller read from the
// ledger and the app's database; nothing here opens a file or a connection.
package classrules

import (
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// LineID is a budget line's id, minted by the app's database.
type LineID int64

// ClassifiedBy says who put a transaction on its line.
type ClassifiedBy int

const (
	// The zero value names nobody, so an example whose author was never filled
	// in is not mistaken for the person's own decision.
	ClassifiedByUnknown ClassifiedBy = iota
	ClassifiedByPerson
	ClassifiedByRule
	ClassifiedByModel
)

// The ranges the classifier settings can take.
const (
	ThresholdLowest    = 50
	ThresholdHighest   = 99
	MinExamplesLowest  = 1
	MinExamplesHighest = 50
)

// Settings are the classifier settings in force when the transaction is
// evaluated.
type Settings struct {
	// Threshold is the share of examples, in percent, the leading line needs.
	Threshold int
	// MinExamples is how many examples a line is applied on at the least.
	MinExamples int
}

// Transaction is what Decide needs to know about the transaction evaluated.
type Transaction struct {
	OwnTransfer bool
	BillPayment bool
	// Unconverted is a foreign amount the ledger gives no value in reais for.
	Unconverted bool
	Credit      bool
	// Refund is the ledger's rule marking the row as a refund.
	Refund bool
	// Fee is a fee or interest charge, or the credit that discounts one.
	Fee bool
	// Person is set when the other side is a person.
	Person bool
	// Nicknamed is set when the establishment or person has a nickname.
	Nicknamed bool
	// DefaultLine is the establishment's default line; nil when it has none.
	DefaultLine *LineID
}

// Example is another transaction of the same establishment key, as the app's
// database has it.
type Example struct {
	Line         LineID
	Day          brday.Day
	ClassifiedBy ClassifiedBy
	// Excluded is the exclusion switch. An exclusion only takes effect with a
	// reason, so a switch with an empty reason still counts as an example.
	Excluded        bool
	ExclusionReason string
	// Split is set when the transaction has split parts.
	Split bool
}

// Outcome is which of the four answers Decide gave.
type Outcome int

const (
	// Leave: the transaction is not the queue's nor a line's by rule.
	Leave Outcome = iota + 1
	// NothingToStore: a fee row already counts on the fees line by rule.
	NothingToStore
	// Apply: the line is applied by rule.
	Apply
	// Pending: the transaction goes to the queue.
	Pending
)

// LeaveCause says why a transaction was left, so the causes can be counted.
type LeaveCause string

const (
	LeftOwnTransfer LeaveCause = "own-transfer"
	LeftBillPayment LeaveCause = "bill-payment"
	LeftUnconverted LeaveCause = "unconverted"
	LeftRefund      LeaveCause = "refund"
)

// Reason is why a transaction is pending, in the words the queue shows.
type Reason string

const (
	ReasonLowConfidence    Reason = "Baixa confiança"
	ReasonUnnamedPerson    Reason = "PIX para pessoa sem apelido"
	ReasonNewEstablishment Reason = "Estabelecimento novo"
)

// Suggestion is the line a pending transaction is offered and how sure the
// rules are of it, in whole percent.
type Suggestion struct {
	Line       LineID
	Confidence int
}

// Decision is Decide's answer. Left is set for Leave, Line for Apply, Reason
// for Pending; Suggestion only for a pending "Baixa confiança".
type Decision struct {
	Outcome    Outcome
	Left       LeaveCause
	Line       LineID
	Reason     Reason
	Suggestion *Suggestion
}

// Decide is called once for a transaction that was never evaluated and has
// neither a line nor a pending review. monthLines are the lines of the budget
// in force in the transaction's own month, the fees line included when that
// budget has any line. The first step that matches wins.
func Decide(tx Transaction, examples []Example, monthLines []LineID, settings Settings) (Decision, error) {
	if err := settings.validate(); err != nil {
		return Decision{}, err
	}
	if cause, left := leaveCause(tx); left {
		return Decision{Outcome: Leave, Left: cause}, nil
	}
	if tx.Fee {
		return Decision{Outcome: NothingToStore}, nil
	}
	inMonth := lineSet(monthLines)
	if tx.DefaultLine != nil && inMonth[*tx.DefaultLine] {
		return Decision{Outcome: Apply, Line: *tx.DefaultLine}, nil
	}
	if tally := tallyExamples(examples, inMonth); tally.total > 0 {
		return tally.decide(settings), nil
	}
	if tx.Person && !tx.Nicknamed {
		return Decision{Outcome: Pending, Reason: ReasonUnnamedPerson}, nil
	}
	return Decision{Outcome: Pending, Reason: ReasonNewEstablishment}, nil
}

// A zero Settings would apply any line with a strict lead; refusing it keeps a
// caller that forgot to fill them from classifying without being asked.
func (settings Settings) validate() error {
	if settings.Threshold < ThresholdLowest || settings.Threshold > ThresholdHighest {
		return fmt.Errorf("classrules: threshold %d is outside %d..%d", settings.Threshold, ThresholdLowest, ThresholdHighest)
	}
	if settings.MinExamples < MinExamplesLowest || settings.MinExamples > MinExamplesHighest {
		return fmt.Errorf("classrules: minimum of examples %d is outside %d..%d", settings.MinExamples, MinExamplesLowest, MinExamplesHighest)
	}
	return nil
}

func leaveCause(tx Transaction) (LeaveCause, bool) {
	switch {
	case tx.OwnTransfer:
		return LeftOwnTransfer, true
	case tx.BillPayment:
		return LeftBillPayment, true
	case tx.Unconverted:
		return LeftUnconverted, true
	case tx.Credit && tx.Refund:
		return LeftRefund, true
	}
	return "", false
}

func lineSet(lines []LineID) map[LineID]bool {
	set := make(map[LineID]bool, len(lines))
	for _, line := range lines {
		set[line] = true
	}
	return set
}
