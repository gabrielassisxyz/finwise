package budgetrules

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

const generatedPaydays = 2000

var generatedPotIDs = []string{"reserve:a/1", "reserve:a/2", "reserve:b/1", "investment:x", "reserve:gone/1"}

// generatedPayday is a budget of up to twenty lines with distinct ids in any
// group, many of them zero; links from any line, the fixed and variable ones
// included, to pots that are open, closed or not listed; and pots that share a
// name or have none.
func generatedPayday(rng *rand.Rand) PaydayInputs {
	inputs := PaydayInputs{Links: map[LineID]string{}, UnnamedPot: paydayUnnamed, InvestmentName: paydayInvestment}
	var lines []paydayLine
	for id := range LineID(rng.IntN(21)) {
		amount := money.Cents(0)
		if rng.IntN(3) > 0 {
			amount = money.Cents(rng.Int64N(10_000_000))
		}
		lines = append(lines, paydayLine{EditableGroups[rng.IntN(len(EditableGroups))], id + 2, "linha", amount})
		if rng.IntN(3) > 0 {
			inputs.Links[id+2] = generatedPotIDs[rng.IntN(len(generatedPotIDs))]
		}
	}
	inputs.Plan = paydayPlan(lines...)
	names := []*string{nil, named("Reserva"), named("Reserva"), named("Casa")}
	for _, id := range generatedPotIDs[:len(generatedPotIDs)-1] {
		if rng.IntN(5) > 0 {
			inputs.Pots = append(inputs.Pots, TransferPot{ID: id, Name: names[rng.IntN(len(names))], Closed: rng.IntN(4) == 0})
		}
	}
	return inputs
}

// transferProblems checks a month's transfers against sums made here from the
// plan itself.
func transferProblems(inputs PaydayInputs, transfers []Transfer) []string {
	var problems []string
	keys := map[string]bool{}
	carried := map[LineID]int{}
	var carriedTotal money.Cents
	for _, transfer := range transfers {
		if keys[transfer.Key] {
			problems = append(problems, "the key "+transfer.Key+" is listed twice")
		}
		keys[transfer.Key] = true
		if transfer.Amount == 0 {
			problems = append(problems, "the transfer "+transfer.Key+" is zero")
		}
		for _, line := range transfer.Lines {
			carried[line]++
		}
		if transfer.Key != InvestmentTransferKey {
			carriedTotal += transfer.Amount
		}
	}
	groups := inputs.Plan.total(GroupProvisions) + inputs.Plan.total(GroupReserves)
	if carriedTotal != groups {
		problems = append(problems, fmt.Sprintf("the pot and line transfers add up to %d, the two groups to %d", carriedTotal, groups))
	}
	for _, line := range append(inputs.Plan.linesOf(GroupProvisions), inputs.Plan.linesOf(GroupReserves)...) {
		if count := carried[line.ID.ID]; count > 1 || (count == 0 && line.Amount != 0) {
			problems = append(problems, fmt.Sprintf("the line %d of %d is carried %d times", line.ID.ID, line.Amount, count))
		}
	}
	return problems
}

func TestGeneratedMonthsGiveDistinctKeysNoZeroAndTheTwoGroupsTotal(t *testing.T) {
	rng := rand.New(rand.NewPCG(66, 2026))
	for run := range generatedPaydays {
		inputs := generatedPayday(rng)
		for _, problem := range transferProblems(inputs, Transfers(inputs)) {
			t.Errorf("month %d: %s", run, problem)
		}
	}
}

// The checks above must see each way a derivation can go wrong.
func TestTheTransferChecksSeeEveryPlantedFault(t *testing.T) {
	inputs := exampleInputs(800000)
	good := Transfers(inputs)
	if problems := transferProblems(inputs, good); len(problems) != 0 {
		t.Fatalf("the worked example has problems: %v", problems)
	}
	twice := append(append([]Transfer{}, good...), good[0])
	zero := append(append([]Transfer{}, good...), Transfer{Key: "line:15", Amount: 0, Lines: []LineID{15}})
	short := append([]Transfer{}, good[1:]...)
	for label, planted := range map[string][]Transfer{"a key twice": twice, "a zero transfer": zero, "a line dropped": short} {
		if problems := transferProblems(inputs, planted); len(problems) == 0 {
			t.Errorf("%s was not seen", label)
		}
	}
}
