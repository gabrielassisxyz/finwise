package budgetrules

import (
	"reflect"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

// deliveryLine stands for the prototype's Delivery in these cases.
const deliveryLine = marketLine

type placed struct {
	key         string
	transaction Transaction
}

func keyedOutcomes(listed []placed) []KeyedOutcome {
	transactions := make([]Transaction, 0, len(listed))
	for _, entry := range listed {
		transactions = append(transactions, entry.transaction)
	}
	outcomes := Dispose(month, transactions)
	keyed := make([]KeyedOutcome, 0, len(listed))
	for i, entry := range listed {
		keyed = append(keyed, KeyedOutcome{EstablishmentKey: entry.key, Outcome: outcomes[i]})
	}
	return keyed
}

func expectEstablishments(t *testing.T, label string, listed []placed, want []EstablishmentTotal) {
	t.Helper()
	keyed := keyedOutcomes(listed)
	got := EstablishmentsOfLine(deliveryLine, keyed)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: %+v, want %+v", label, got, want)
	}
	var sum money.Cents
	outcomes := make([]Outcome, 0, len(keyed))
	for _, entry := range keyed {
		outcomes = append(outcomes, entry.Outcome)
	}
	for _, total := range got {
		sum += total.Amount
	}
	if figure := LineFigures(outcomes)[deliveryLine]; sum != figure {
		t.Errorf("%s: the establishments add up to %d, the line's figure is %d", label, sum, figure)
	}
}

func deliveries() []placed {
	return []placed{
		{"iFood", on(deliveryLine, debit(6330))},
		{"Rappi", on(deliveryLine, debit(7270))},
		{"iFood", on(deliveryLine, debit(7000))},
		{"Padaria", on(rentLine, debit(9900))},
	}
}

// Ported from "the establishments of a line" of web/src/api/fake/budget.test.ts.
func TestALinesEstablishmentsAddUpWhatEachPutIntoItBiggestFirst(t *testing.T) {
	expectEstablishments(t, "two iFood purchases and one Rappi", deliveries(),
		[]EstablishmentTotal{{"iFood", 13330, 2}, {"Rappi", 7270, 1}})
	expectEstablishments(t, "a month with nothing on the line", nil, []EstablishmentTotal{})

	excluded := deliveries()
	excluded[0].transaction.Exclusion = ExcludedWithReason
	expectEstablishments(t, "an iFood purchase excluded with a reason", excluded,
		[]EstablishmentTotal{{"Rappi", 7270, 1}, {"iFood", 7000, 1}})
}

func TestASplitCountsOnceWithEveryPartItPutsOnTheLine(t *testing.T) {
	split := debit(30000)
	split.Split = []SplitPart{{OnLine(deliveryLine), 15000}, {OnLine(deliveryLine), 5000}, {OnLine(rentLine), 6000}, {LineRef{}, 4000}}
	listed := append(deliveries(), placed{"Atacado", split})
	expectEstablishments(t, "a split with two parts on the line", listed,
		[]EstablishmentTotal{{"Atacado", 20000, 1}, {"iFood", 13330, 2}, {"Rappi", 7270, 1}})
}

func TestEstablishmentsOfEqualAmountAreOrderedByKey(t *testing.T) {
	listed := []placed{
		{"text:ZETA", on(deliveryLine, debit(5000))},
		{"doc:123", on(deliveryLine, debit(5000))},
		{"name:Alfa", on(deliveryLine, debit(5000))},
	}
	expectEstablishments(t, "a tie", listed,
		[]EstablishmentTotal{{"doc:123", 5000, 1}, {"name:Alfa", 5000, 1}, {"text:ZETA", 5000, 1}})
}

func TestARefundOnTheLineTakesItsAmountOffItsEstablishment(t *testing.T) {
	refund := credit(2000)
	refund.RefundOverride = RefundYes
	refund.RefundLine = OnLine(deliveryLine)
	listed := append(deliveries(), placed{"Rappi", refund})
	expectEstablishments(t, "a refund of Rappi", listed,
		[]EstablishmentTotal{{"iFood", 13330, 2}, {"Rappi", 5270, 2}})

	reimbursed := on(deliveryLine, debit(3000))
	reimbursed.LinkedCredit = 5000
	listed = append(deliveries(), placed{"Lanchonete", reimbursed})
	expectEstablishments(t, "a purchase wholly reimbursed adds zero and still counts", listed,
		[]EstablishmentTotal{{"iFood", 13330, 2}, {"Rappi", 7270, 1}, {"Lanchonete", 0, 1}})
}
