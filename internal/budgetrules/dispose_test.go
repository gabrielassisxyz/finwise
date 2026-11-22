package budgetrules

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

const (
	salaryLine  LineID = 1
	rentLine    LineID = 2
	marketLine  LineID = 3
	travelLine  LineID = 4
	reserveLine LineID = 5
	feesLine    LineID = 90
	unknownLine LineID = 77
)

var month = Budget{
	Lines: map[LineID]Group{
		salaryLine:  GroupIncome,
		rentLine:    GroupFixed,
		marketLine:  GroupVariable,
		travelLine:  GroupProvisions,
		reserveLine: GroupReserves,
	},
	FeesLine: feesLine,
}

func debit(cents money.Cents) Transaction  { return Transaction{Amount: -cents, Posted: true} }
func credit(cents money.Cents) Transaction { return Transaction{Amount: cents, Posted: true} }

func on(line LineID, transaction Transaction) Transaction {
	transaction.Line = OnLine(line)
	return transaction
}

func adds(line LineID, cents money.Cents) LineAmount {
	return LineAmount{Line: line, Effect: Adds, Amount: cents}
}

func takesOff(line LineID, cents money.Cents) LineAmount {
	return LineAmount{Line: line, Effect: TakesOff, Amount: cents}
}

func nothing(disposition Disposition, magnitude money.Cents) Outcome {
	return Outcome{Disposition: disposition, Magnitude: magnitude}
}

type dispositionCase struct {
	name        string
	transaction Transaction
	want        Outcome
}

func checkCases(t *testing.T, cases []dispositionCase) {
	t.Helper()
	for _, c := range cases {
		got := Dispose(month, []Transaction{c.transaction})
		if len(got) != 1 || !reflect.DeepEqual(got[0], c.want) {
			t.Errorf("%s:\n  transaction %+v\n  got  %+v\n  want %+v", c.name, c.transaction, got, c.want)
		}
	}
}

func TestEachDispositionCountsWhatItsRowOfTheTableSays(t *testing.T) {
	unconverted := on(marketLine, debit(4200))
	unconverted.Unconverted = true

	movement := on(marketLine, debit(150000))
	movement.Movement = true

	excluded := on(marketLine, debit(5000))
	excluded.Exclusion = ExcludedWithReason

	waiting := on(marketLine, debit(5000))
	waiting.ReviewPending = true

	split := debit(18000)
	split.Split = []SplitPart{{OnLine(marketLine), 10000}, {OnLine(rentLine), 6000}, {LineRef{}, 2000}}

	fee := debit(1990)
	fee.Fee = true

	reimbursement := credit(15000)
	reimbursement.LinkedToPurchase = true

	refund := credit(12000)
	refund.LedgerRefund = true
	refund.RefundLine = OnLine(feesLine)

	linked := on(travelLine, debit(30000))
	linked.LinkedCredit = 15000

	uncategorised := debit(2590)
	uncategorised.LinkedCredit = 590

	checkCases(t, []dispositionCase{
		{"1 unconverted", unconverted, nothing(Unconverted, 0)},
		{"2 movement", movement, nothing(Movement, 150000)},
		{"3 excluded", excluded, nothing(Excluded, 5000)},
		{"4 waiting", waiting, nothing(Waiting, 5000)},
		{"5 split", split, Outcome{Disposition: Split, Lines: []LineAmount{adds(marketLine, 10000), adds(rentLine, 6000)}, Unassigned: 2000, Magnitude: 18000}},
		{"6 fee", fee, Outcome{Disposition: Fee, Lines: []LineAmount{adds(feesLine, 1990)}, Magnitude: 1990}},
		{"7 reimbursement", reimbursement, nothing(Reimbursement, 15000)},
		{"8 refund", refund, Outcome{Disposition: Refund, Lines: []LineAmount{takesOff(feesLine, 12000)}, Magnitude: 12000}},
		{"9 income", on(salaryLine, credit(820000)), Outcome{Disposition: Income, Lines: []LineAmount{{salaryLine, Received, 820000}}, Magnitude: 820000}},
		{"10 other credit", credit(20000), nothing(OtherCredit, 20000)},
		{"11 on a line", linked, Outcome{Disposition: OnALine, Lines: []LineAmount{adds(travelLine, 15000)}, AppliedReimbursement: 15000, Magnitude: 30000}},
		{"12 uncategorised", uncategorised, Outcome{Disposition: Uncategorised, Unassigned: 2000, AppliedReimbursement: 590, Magnitude: 2590}},
	})
}

func TestTheFirstConditionThatHoldsWins(t *testing.T) {
	everything := on(marketLine, debit(9000))
	everything.Unconverted = true
	everything.Movement = true
	everything.Exclusion = ExcludedWithReason
	everything.ReviewPending = true
	everything.Split = []SplitPart{{OnLine(marketLine), 4000}, {OnLine(rentLine), 5000}}

	unconvertedExcludedPendingSplit := everything
	unconvertedExcludedPendingSplit.Movement = false

	movementExcluded := everything
	movementExcluded.Unconverted = false

	excludedPending := on(marketLine, debit(9000))
	excludedPending.Exclusion = ExcludedWithReason
	excludedPending.ReviewPending = true

	pendingSplit := debit(9000)
	pendingSplit.ReviewPending = true
	pendingSplit.Split = everything.Split

	splitFee := debit(9000)
	splitFee.Fee = true
	splitFee.Split = everything.Split

	feeLinked := credit(500)
	feeLinked.Fee = true
	feeLinked.LinkedToPurchase = true

	linkedRefund := credit(500)
	linkedRefund.LinkedToPurchase = true
	linkedRefund.LedgerRefund = true
	linkedRefund.RefundLine = OnLine(marketLine)

	refundOnIncome := on(salaryLine, credit(500))
	refundOnIncome.LedgerRefund = true

	checkCases(t, []dispositionCase{
		{"unconverted, excluded, pending and split is unconverted", unconvertedExcludedPendingSplit, nothing(Unconverted, 0)},
		{"unconverted before movement", everything, nothing(Unconverted, 0)},
		{"movement before excluded", movementExcluded, nothing(Movement, 9000)},
		{"excluded and pending is excluded", excludedPending, nothing(Excluded, 9000)},
		{"pending before split", pendingSplit, nothing(Waiting, 9000)},
		{"split before fee", splitFee, Outcome{Disposition: Split, Lines: []LineAmount{adds(marketLine, 4000), adds(rentLine, 5000)}, Magnitude: 9000}},
		{"fee before reimbursement", feeLinked, Outcome{Disposition: Fee, Lines: []LineAmount{takesOff(feesLine, 500)}, Magnitude: 500}},
		{"reimbursement before refund", linkedRefund, nothing(Reimbursement, 500)},
		{"refund before income, with no refund line", refundOnIncome, nothing(Refund, 500)},
	})
}

func TestAFeeRowIsAFeeOnNoLineOncePostedAndOnTheFeesLineAlways(t *testing.T) {
	pendingOnNoLine := debit(1990)
	pendingOnNoLine.Fee = true
	pendingOnNoLine.Posted = false

	pendingOnFeesLine := on(feesLine, pendingOnNoLine)

	byRule := on(feesLine, debit(1990))
	byRule.Fee = true
	byRule.Posted = false
	byRule.LineSetBy = SetByRule

	discount := credit(500)
	discount.Fee = true

	onAnotherLine := on(marketLine, debit(1990))
	onAnotherLine.Fee = true

	notAFeeOnFeesLine := on(feesLine, debit(3000))

	checkCases(t, []dispositionCase{
		{"a bank-pending fee row on no line is not a fee", pendingOnNoLine, Outcome{Disposition: Uncategorised, Unassigned: 1990, Magnitude: 1990}},
		{"a fee charge on the fees line while bank-pending is a fee", pendingOnFeesLine, Outcome{Disposition: Fee, Lines: []LineAmount{adds(feesLine, 1990)}, Magnitude: 1990}},
		{"a fee charge a rule put on the fees line is a fee", byRule, Outcome{Disposition: Fee, Lines: []LineAmount{adds(feesLine, 1990)}, Magnitude: 1990}},
		{"a discount credit takes off the fees line", discount, Outcome{Disposition: Fee, Lines: []LineAmount{takesOff(feesLine, 500)}, Magnitude: 500}},
		{"a fee row on another line counts on that line", onAnotherLine, Outcome{Disposition: OnALine, Lines: []LineAmount{adds(marketLine, 1990)}, Magnitude: 1990}},
		{"a purchase on the fees line that is no fee row is on a line", notAFeeOnFeesLine, Outcome{Disposition: OnALine, Lines: []LineAmount{adds(feesLine, 3000)}, Magnitude: 3000}},
	})
}

func TestARefundTakesOffItsLineAndTheOverrideDecidesWhatIsOne(t *testing.T) {
	noLine := credit(12000)
	noLine.LedgerRefund = true

	overriddenNo := on(salaryLine, credit(12000))
	overriddenNo.LedgerRefund = true
	overriddenNo.RefundOverride = RefundNo
	overriddenNo.RefundLine = OnLine(marketLine)

	overriddenNoOtherCredit := credit(12000)
	overriddenNoOtherCredit.LedgerRefund = true
	overriddenNoOtherCredit.RefundOverride = RefundNo
	overriddenNoOtherCredit.RefundLine = OnLine(marketLine)

	overriddenYes := credit(4000)
	overriddenYes.RefundOverride = RefundYes
	overriddenYes.RefundLine = OnLine(marketLine)

	checkCases(t, []dispositionCase{
		{"a refund with no refund line counts nothing", noLine, nothing(Refund, 12000)},
		{"an override of no on a ledger refund on an income line is income", overriddenNo, Outcome{Disposition: Income, Lines: []LineAmount{{salaryLine, Received, 12000}}, Magnitude: 12000}},
		{"an override of no on a ledger refund on no line is an other credit", overriddenNoOtherCredit, nothing(OtherCredit, 12000)},
		{"an override of yes makes a refund of a credit the ledger does not mark", overriddenYes, Outcome{Disposition: Refund, Lines: []LineAmount{takesOff(marketLine, 4000)}, Magnitude: 4000}},
	})
}

// Only a credit can be a refund: a purchase carrying the mark that became one
// would leave its line and, with no refund line, every total.
func TestADebitWithTheRefundMarkIsNoRefund(t *testing.T) {
	marks := map[string]func(Transaction) Transaction{
		"the override set to yes": func(transaction Transaction) Transaction {
			transaction.RefundOverride = RefundYes
			return transaction
		},
		"the ledger's refund rule": func(transaction Transaction) Transaction {
			transaction.LedgerRefund = true
			return transaction
		},
	}
	var cases []dispositionCase
	for mark, marked := range marks {
		for _, refundLine := range []LineRef{{}, OnLine(rentLine)} {
			suffix := " with " + mark + fmt.Sprintf(", refund line %+v", refundLine)
			purchase := marked(on(marketLine, debit(5000)))
			purchase.RefundLine = refundLine
			uncategorised := marked(debit(5000))
			uncategorised.RefundLine = refundLine
			cases = append(cases,
				dispositionCase{"a debit on a line" + suffix, purchase, Outcome{Disposition: OnALine, Lines: []LineAmount{adds(marketLine, 5000)}, Magnitude: 5000}},
				dispositionCase{"a debit on no line" + suffix, uncategorised, Outcome{Disposition: Uncategorised, Unassigned: 5000, Magnitude: 5000}},
			)
		}
	}
	checkCases(t, cases)
}

func TestTheOtherAuthoredFieldsCountAsTheTableSays(t *testing.T) {
	excludedWithoutReason := on(marketLine, debit(5000))
	excludedWithoutReason.Exclusion = ExcludedWithoutReason

	exceeding := debit(10000)
	exceeding.Split = []SplitPart{{OnLine(marketLine), 6000}, {OnLine(rentLine), 5000}}

	partWithoutLine := debit(10000)
	partWithoutLine.Split = []SplitPart{{OnLine(marketLine), 7000}, {LineRef{}, 3000}}

	onePart := on(marketLine, debit(10000))
	onePart.Split = []SplitPart{{OnLine(rentLine), 10000}}

	splitCredit := credit(10000)
	splitCredit.Split = exceeding.Split

	bankPending := on(marketLine, debit(5000))
	bankPending.Posted = false

	checkCases(t, []dispositionCase{
		{"an exclusion with no reason keeps counting", excludedWithoutReason, Outcome{Disposition: OnALine, Lines: []LineAmount{adds(marketLine, 5000)}, Magnitude: 5000}},
		{"a credit on a fix line is an other credit", on(rentLine, credit(20000)), nothing(OtherCredit, 20000)},
		{"a split whose parts exceed it by 10.00 leaves -10.00 unassigned", exceeding, Outcome{Disposition: Split, Lines: []LineAmount{adds(marketLine, 6000), adds(rentLine, 5000)}, Unassigned: -1000, Magnitude: 10000}},
		{"a split part without a line adds to no line", partWithoutLine, Outcome{Disposition: Split, Lines: []LineAmount{adds(marketLine, 7000)}, Unassigned: 3000, Magnitude: 10000}},
		{"one part is no split", onePart, Outcome{Disposition: OnALine, Lines: []LineAmount{adds(marketLine, 10000)}, Magnitude: 10000}},
		{"a credit with parts is no split", splitCredit, nothing(OtherCredit, 10000)},
		{"a row the bank still reports pending counts like any other", bankPending, Outcome{Disposition: OnALine, Lines: []LineAmount{adds(marketLine, 5000)}, Magnitude: 5000}},
	})
}

func TestAReimbursementIsCappedAtItsPurchaseAndItsExcessCountsNowhere(t *testing.T) {
	for _, c := range []struct {
		credit, applied, onLine money.Cents
	}{
		{15000, 15000, 15000},
		{30000, 30000, 0},
		{40000, 30000, 0},
	} {
		purchase := on(travelLine, debit(30000))
		purchase.LinkedCredit = c.credit
		reimbursement := credit(c.credit)
		reimbursement.LinkedToPurchase = true

		got := Dispose(month, []Transaction{purchase, reimbursement})
		want := []Outcome{
			{Disposition: OnALine, Lines: []LineAmount{adds(travelLine, c.onLine)}, AppliedReimbursement: c.applied, Magnitude: 30000},
			nothing(Reimbursement, c.credit),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("a purchase of 300.00 linked to a credit of %d cents:\n  got  %+v\n  want %+v", c.credit, got, want)
		}
	}
}

// A line id the month's budget lacks is no line: the amount shows as
// unassigned rather than landing where no screen of the month looks.
func TestALineTheMonthsBudgetLacksIsNoLine(t *testing.T) {
	split := debit(10000)
	split.Split = []SplitPart{{OnLine(marketLine), 4000}, {OnLine(unknownLine), 6000}}

	refund := credit(3000)
	refund.LedgerRefund = true
	refund.RefundLine = OnLine(unknownLine)

	fee := on(unknownLine, debit(1990))
	fee.Fee = true

	pendingFee := fee
	pendingFee.Posted = false

	checkCases(t, []dispositionCase{
		{"a debit is uncategorised", on(unknownLine, debit(5000)), Outcome{Disposition: Uncategorised, Unassigned: 5000, Magnitude: 5000}},
		{"a split part is unassigned", split, Outcome{Disposition: Split, Lines: []LineAmount{adds(marketLine, 4000)}, Unassigned: 6000, Magnitude: 10000}},
		{"a refund line is none", refund, nothing(Refund, 3000)},
		{"a credit is no income", on(unknownLine, credit(3000)), nothing(OtherCredit, 3000)},
		{"a posted fee row is on no line, so a fee", fee, Outcome{Disposition: Fee, Lines: []LineAmount{adds(feesLine, 1990)}, Magnitude: 1990}},
		{"a bank-pending fee row is on no line, so no fee", pendingFee, Outcome{Disposition: Uncategorised, Unassigned: 1990, Magnitude: 1990}},
	})
}
