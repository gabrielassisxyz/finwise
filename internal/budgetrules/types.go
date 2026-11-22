// Package budgetrules decides what each listed transaction of a month counts
// for in that month's budget: which line it adds to or takes off, what is left
// unassigned and what reimbursement was applied inside it. It takes plain
// values and returns plain values; the caller maps the ledger's rows and what
// the person authored into them. It does no SQL and no I/O.
package budgetrules

import "github.com/gabrielassisxyz/finwise/internal/money"

// LineID is a budget line's id as the app's database mints it.
type LineID int64

// Group is the budget group a line belongs to.
type Group string

const (
	GroupIncome     Group = "rec"
	GroupFixed      Group = "fix"
	GroupVariable   Group = "var"
	GroupProvisions Group = "prov"
	GroupReserves   Group = "res"
)

// LineRef is a line a transaction names; the zero value names none.
type LineRef struct {
	ID  LineID
	Set bool
}

// OnLine names the line id.
func OnLine(id LineID) LineRef {
	return LineRef{ID: id, Set: true}
}

// LineSource says who put a transaction on its line.
type LineSource int

const (
	SetByPerson LineSource = iota
	// SetByRule is a line applied by a rule, such as the one that puts every
	// purchase of an establishment on the same line.
	SetByRule
)

// Exclusion is the person's mark that a transaction does not count.
type Exclusion int

const (
	NotExcluded Exclusion = iota
	// ExcludedWithoutReason is the switch turned on with no reason given; the
	// transaction keeps counting until a reason is written.
	ExcludedWithoutReason
	ExcludedWithReason
)

// RefundOverride is the person's answer to whether a credit is a refund.
type RefundOverride int

const (
	// RefundAsLedgerSays leaves the decision to the ledger's refund rule.
	RefundAsLedgerSays RefundOverride = iota
	RefundYes
	RefundNo
)

// SplitPart is one part of a split purchase. Amount is a magnitude in cents.
type SplitPart struct {
	Line   LineRef
	Amount money.Cents
}

// Transaction is one listed transaction: the ledger's facts about it and what
// the person authored. It is a credit when Amount is above zero and a debit
// otherwise.
type Transaction struct {
	// Amount is signed, negative for money out. On an unconverted row it is in
	// a foreign currency.
	Amount money.Cents

	Unconverted bool
	// Movement is a transfer between the person's own accounts or a card bill paid.
	Movement bool
	Fee      bool
	// Posted is false while the bank reports the row as pending.
	Posted       bool
	LedgerRefund bool

	Line           LineRef
	LineSetBy      LineSource
	ReviewPending  bool
	Exclusion      Exclusion
	Split          []SplitPart
	RefundOverride RefundOverride
	RefundLine     LineRef
	// LinkedToPurchase is set on a credit that a purchase names as its reimbursement.
	LinkedToPurchase bool
	// LinkedCredit is, on a purchase, the cents of the credit linked to it as
	// its reimbursement; zero when none is.
	LinkedCredit money.Cents
}

// Budget is what Dispose needs of the month's budget. Lines holds every line of
// the budget in force with its group; the fees line is in no group and need
// not be listed there.
type Budget struct {
	Lines    map[LineID]Group
	FeesLine LineID
}

// Disposition is the one place a transaction lands. The constants are in the
// order Dispose tries them.
type Disposition int

const (
	Unconverted Disposition = iota + 1
	Movement
	Excluded
	Waiting
	Split
	Fee
	Reimbursement
	Refund
	Income
	OtherCredit
	OnALine
	Uncategorised
)

var dispositionNames = [...]string{
	Unconverted:   "unconverted",
	Movement:      "movement",
	Excluded:      "excluded",
	Waiting:       "waiting",
	Split:         "split",
	Fee:           "fee",
	Reimbursement: "reimbursement",
	Refund:        "refund",
	Income:        "income",
	OtherCredit:   "other credit",
	OnALine:       "on a line",
	Uncategorised: "uncategorised",
}

func (disposition Disposition) String() string {
	if disposition < Unconverted || disposition > Uncategorised {
		return "no disposition"
	}
	return dispositionNames[disposition]
}

// Effect is how an amount bears on a line. They stay apart because spending,
// what takes spending back and income received are shown apart.
type Effect int

const (
	// Adds is spending on the line.
	Adds Effect = iota + 1
	// TakesOff is a fee discount or a refund, which reduces the line's spending.
	TakesOff
	// Received is income received on the line.
	Received
)

// LineAmount is what one transaction does to one line. Amount is a magnitude.
type LineAmount struct {
	Line   LineID
	Effect Effect
	Amount money.Cents
}

// Outcome is what Dispose decided for one transaction.
type Outcome struct {
	Disposition Disposition
	// Lines has one entry per line touched, one per part for a split.
	Lines []LineAmount
	// Unassigned is the part of a debit's magnitude on no line; it is negative
	// when a split's parts exceed what was paid.
	Unassigned money.Cents
	// AppliedReimbursement is what a linked credit took off this purchase.
	AppliedReimbursement money.Cents
	// Magnitude is the transaction's amount without its sign, and zero for an
	// unconverted row, whose amount is not in reais.
	Magnitude money.Cents
}
