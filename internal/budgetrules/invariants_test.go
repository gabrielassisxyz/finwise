package budgetrules

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

var seedFlag = flag.Uint64("seed", 0, "seed of the generated amounts; zero draws a new one")

// listedMagnitude is what a listed transaction weighs in reais in invariant 2.
// An unconverted row weighs nothing: its amount is in a foreign currency, and
// adding it to reais would make the sum mean nothing.
func listedMagnitude(transaction Transaction) money.Cents {
	switch {
	case transaction.Unconverted:
		return 0
	case transaction.Amount < 0:
		return -transaction.Amount
	default:
		return transaction.Amount
	}
}

// dimensions are the choices a generated transaction is made of. Every
// combination of them is generated once per run; the amounts are random.
var dimensions = []int{
	2, // debit or credit
	2, // unconverted
	2, // movement
	2, // fee row
	2, // posted
	2, // the ledger's refund rule
	5, // line: none, income, variable, fees, unknown
	2, // review pending
	3, // exclusion
	8, // split: none, one part, then all parts on lines or not, summing below, at or above
	3, // refund override
	3, // refund line: none, known, unknown
	2, // linked to a purchase
	4, // linked credit: none, smaller than, equal to or larger than the purchase
}

const (
	splitNone = iota
	splitOnePart
)

const (
	below = iota
	at
	above
)

var relationNames = [...]string{below: "below", at: "at", above: "above"}

func lineChoice(choice int) LineRef {
	return [...]LineRef{{}, OnLine(salaryLine), OnLine(marketLine), OnLine(feesLine), OnLine(unknownLine)}[choice]
}

func generated(rng *rand.Rand, choice []int) Transaction {
	isCredit := choice[0] == 1
	size := money.Cents(rng.IntN(500000) + 1)
	if !isCredit && rng.IntN(64) == 0 {
		size = 0
	}
	transaction := Transaction{
		Amount:           -size,
		Unconverted:      choice[1] == 1,
		Movement:         choice[2] == 1,
		Fee:              choice[3] == 1,
		Posted:           choice[4] == 1,
		LedgerRefund:     choice[5] == 1,
		Line:             lineChoice(choice[6]),
		LineSetBy:        LineSource(rng.IntN(2)),
		ReviewPending:    choice[7] == 1,
		Exclusion:        Exclusion(choice[8]),
		Split:            generatedSplit(rng, choice[9], size),
		RefundOverride:   RefundOverride(choice[10]),
		RefundLine:       [...]LineRef{{}, OnLine(marketLine), OnLine(unknownLine)}[choice[11]],
		LinkedToPurchase: choice[12] == 1,
		LinkedCredit:     linkedCredit(rng, choice[13], size),
	}
	if isCredit {
		transaction.Amount = size
	}
	return transaction
}

func generatedSplit(rng *rand.Rand, shape int, size money.Cents) []SplitPart {
	if shape == splitNone {
		return nil
	}
	if shape == splitOnePart {
		return []SplitPart{{OnLine(marketLine), size}}
	}
	shape -= 2
	allOnLines, relation := shape < 3, shape%3
	total := size
	switch relation {
	case below:
		total -= min(size, money.Cents(rng.IntN(int(size)+1))+1)
	case above:
		total += money.Cents(rng.IntN(int(size)+1)) + 1
	}
	count := 2 + rng.IntN(3)
	parts := make([]SplitPart, count)
	left := total
	for i := range parts {
		share := left
		if i < count-1 {
			share = money.Cents(rng.IntN(int(left) + 1))
		}
		left -= share
		parts[i] = SplitPart{Line: lineChoice(1 + rng.IntN(4)), Amount: share}
		if allOnLines {
			parts[i].Line = lineChoice(1 + rng.IntN(3))
		}
	}
	if !allOnLines {
		parts[rng.IntN(count)].Line = [...]LineRef{{}, OnLine(unknownLine)}[rng.IntN(2)]
	}
	return parts
}

func linkedCredit(rng *rand.Rand, choice int, size money.Cents) money.Cents {
	switch choice {
	case 1:
		if size < 2 {
			return 0
		}
		return 1 + money.Cents(rng.IntN(int(size)-1))
	case 2:
		return size
	case 3:
		return size + 1 + money.Cents(rng.IntN(int(size)+1))
	}
	return 0
}

// everyCombination calls visit with each combination of dimensions in turn.
func everyCombination(visit func(choice []int)) {
	choice := make([]int, len(dimensions))
	for {
		visit(choice)
		position := 0
		for position < len(choice) {
			choice[position]++
			if choice[position] < dimensions[position] {
				break
			}
			choice[position] = 0
			position++
		}
		if position == len(choice) {
			return
		}
	}
}

func generatedMonths(seed uint64) [][]Transaction {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	var all []Transaction
	everyCombination(func(choice []int) { all = append(all, generated(rng, choice)) })
	rng.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	months := [][]Transaction{nil}
	for len(all) > 0 {
		size := min(len(all), 1+rng.IntN(120))
		months = append(months, all[:size])
		all = all[size:]
	}
	return months
}

type coverage struct {
	dispositions  map[Disposition]int
	splits        map[string]int
	reimbursement map[string]int
	markedDebits  map[string]int
}

func relation(got, want money.Cents) int {
	switch {
	case got < want:
		return below
	case got == want:
		return at
	default:
		return above
	}
}

func (seen coverage) note(transaction Transaction, outcome Outcome) {
	seen.dispositions[outcome.Disposition]++
	switch outcome.Disposition {
	case Split:
		var total money.Cents
		onLines := true
		for _, part := range transaction.Split {
			total += part.Amount
			onLines = onLines && part.Line.Set && part.Line.ID != unknownLine
		}
		seen.splits[fmt.Sprintf("parts on lines %t, summing %s", onLines, relationNames[relation(total, outcome.Magnitude)])]++
	case OnALine, Uncategorised:
		if transaction.LinkedCredit > 0 {
			seen.reimbursement[fmt.Sprintf("%s, credit %s the purchase", outcome.Disposition, relationNames[relation(transaction.LinkedCredit, outcome.Magnitude)])]++
		}
		if mark := refundMark(transaction); mark != "" {
			seen.markedDebits[fmt.Sprintf("%s, %s, refund line set %t", outcome.Disposition, mark, transaction.RefundLine.Set)]++
		}
	}
}

func refundMark(transaction Transaction) string {
	switch {
	case transaction.RefundOverride == RefundYes:
		return "override yes"
	case transaction.RefundOverride == RefundAsLedgerSays && transaction.LedgerRefund:
		return "the ledger's rule"
	}
	return ""
}

// wrongWith says what is wrong with one outcome, or nothing.
func wrongWith(outcome Outcome, transaction Transaction) string {
	if outcome.Disposition < Unconverted || outcome.Disposition > Uncategorised {
		return "is in no disposition"
	}
	if outcome.Magnitude != listedMagnitude(transaction) {
		return fmt.Sprintf("weighs %d, want %d", outcome.Magnitude, listedMagnitude(transaction))
	}
	if problem := wrongSide(outcome.Disposition, transaction.Amount > 0); problem != "" {
		return problem
	}
	switch outcome.Disposition {
	case Split, OnALine, Uncategorised:
		var added money.Cents
		for _, line := range outcome.Lines {
			if line.Effect != Adds {
				return "is a debit that does not add"
			}
			added += line.Amount
		}
		if added+outcome.Unassigned+outcome.AppliedReimbursement != outcome.Magnitude {
			return "breaks invariant 1: added, unassigned and reimbursed do not make its amount"
		}
		if outcome.AppliedReimbursement < 0 || outcome.AppliedReimbursement > outcome.Magnitude {
			return "has a reimbursement that turns the purchase's sign"
		}
	case Unconverted, Movement, Excluded, Waiting, Reimbursement, OtherCredit:
		if outcome.Lines != nil || outcome.Unassigned != 0 || outcome.AppliedReimbursement != 0 {
			return "counts for something in a disposition that counts nothing"
		}
	}
	return ""
}

// wrongSide refuses a debit in a disposition only a credit can have, and the
// reverse; invariant 1 sees a debit only in the dispositions of debits.
func wrongSide(disposition Disposition, isCredit bool) string {
	switch disposition {
	case Reimbursement, Refund, Income, OtherCredit:
		if !isCredit {
			return "is a debit in a disposition of credits"
		}
	case Split, OnALine, Uncategorised:
		if isCredit {
			return "is a credit in a disposition of debits"
		}
	}
	return ""
}

func TestNoTransactionIsDroppedOrCountedTwice(t *testing.T) {
	seed := *seedFlag
	if seed == 0 {
		seed = rand.Uint64()
	}
	reproduce := fmt.Sprintf("reproduce with: go test ./internal/budgetrules -run %s -seed=%d", t.Name(), seed)
	seen := coverage{map[Disposition]int{}, map[string]int{}, map[string]int{}, map[string]int{}}
	failures := 0
	fail := func(format string, args ...any) {
		failures++
		if failures <= 10 {
			t.Errorf(format+"\n  "+reproduce, args...)
		}
	}
	for index, listed := range generatedMonths(seed) {
		outcomes := Dispose(month, listed)
		if len(outcomes) != len(listed) {
			fail("month %d: %d transactions listed, %d disposed", index, len(listed), len(outcomes))
			continue
		}
		var listedTotal, disposedTotal money.Cents
		counted := 0
		for i, outcome := range outcomes {
			if problem := wrongWith(outcome, listed[i]); problem != "" {
				fail("month %d, transaction %d %s\n  transaction %+v\n  outcome     %+v", index, i, problem, listed[i], outcome)
			}
			seen.note(listed[i], outcome)
			listedTotal += listedMagnitude(listed[i])
			if outcome.Disposition >= Unconverted && outcome.Disposition <= Uncategorised {
				counted++
				disposedTotal += outcome.Magnitude
			}
		}
		if counted != len(listed) || disposedTotal != listedTotal {
			fail("month %d breaks invariant 2: %d of %d transactions disposed, weighing %d of %d", index, counted, len(listed), disposedTotal, listedTotal)
		}
	}
	for disposition := Unconverted; disposition <= Uncategorised; disposition++ {
		if seen.dispositions[disposition] == 0 {
			fail("no generated transaction reached disposition %s", disposition)
		}
	}
	requireReached(t, "split", seen.splits, 6, fail)
	requireReached(t, "reimbursement", seen.reimbursement, 6, fail)
	requireReached(t, "debit with the refund mark", seen.markedDebits, 8, fail)
}

func requireReached(t *testing.T, what string, seen map[string]int, want int, fail func(string, ...any)) {
	t.Helper()
	if len(seen) != want {
		fail("the generated %s cases reached %d of %d shapes: %v", what, len(seen), want, seen)
	}
}
