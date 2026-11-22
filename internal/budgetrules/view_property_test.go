package budgetrules

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

const generatedPlans = 2000

// generatedPlan is a draft as a client could send one: up to seven groups of
// any of the five keys, a key possibly twice, lines with an id or without, and
// amounts up to one billion reais.
func generatedPlan(rng *rand.Rand) Plan {
	plan := Plan{}
	for range rng.IntN(8) {
		group := PlanGroup{Key: EditableGroups[rng.IntN(len(EditableGroups))]}
		for range rng.IntN(7) {
			line := PlannedLine{Name: "linha", Amount: money.Cents(rng.Int64N(100_000_000_001))}
			if rng.IntN(4) > 0 {
				line.ID = OnLine(LineID(rng.IntN(12) + 1))
			}
			group.Lines = append(group.Lines, line)
		}
		plan = append(plan, group)
	}
	return plan
}

func generatedFacts(rng *rand.Rand) MonthFacts {
	facts := MonthFacts{Figures: map[LineID]money.Cents{}, PotBalances: map[LineID]money.Cents{}, FeesLine: feesLine}
	for id := LineID(1); id <= 12; id++ {
		if rng.IntN(3) > 0 {
			facts.Figures[id] = money.Cents(rng.Int64N(400_000_000) - 100_000_000)
		}
		if rng.IntN(3) == 0 {
			facts.PotBalances[id] = money.Cents(rng.Int64N(100_000_000))
		}
	}
	facts.Figures[feesLine] = money.Cents(rng.Int64N(100_000))
	return facts
}

// planProblems checks a view against sums made here from the plan itself.
func planProblems(plan Plan, view BudgetView) []string {
	var income, outgoing money.Cents
	sent := map[Group]int{}
	for _, group := range plan {
		sent[group.Key] += len(group.Lines)
		for _, line := range group.Lines {
			if group.Key == GroupIncome {
				income += line.Amount
			} else {
				outgoing += line.Amount
			}
		}
	}
	var problems []string
	if view.Income != income || view.Allocated != income || view.Investment != income-outgoing {
		problems = append(problems, fmt.Sprintf("income %d, allocated %d, investment %d; the plan's income %d and outgoing %d",
			view.Income, view.Allocated, view.Investment, income, outgoing))
	}
	for _, group := range view.Groups {
		var first money.Cents
		for _, line := range group.Lines {
			first += line.Cells[0].Amount
			if line.Bar != nil && (*line.Bar < 0 || *line.Bar > 100) {
				problems = append(problems, fmt.Sprintf("%s %s: bar %d", group.Key, line.Name, *line.Bar))
			}
		}
		if group.Subtotal != first {
			problems = append(problems, fmt.Sprintf("%s: subtotal %d, its lines' first cells add up to %d", group.Key, group.Subtotal, first))
		}
	}
	for _, group := range view.Groups[:len(EditableGroups)] {
		if len(group.Lines) != sent[group.Key] {
			problems = append(problems, fmt.Sprintf("%s: %d lines shown, %d sent", group.Key, len(group.Lines), sent[group.Key]))
		}
	}
	return problems
}

func generatedOutcomes(rng *rand.Rand) []KeyedOutcome {
	keys := []string{"doc:1", "name:Ana", "text:IFOOD", "text:RAPPI"}
	effects := []Effect{Adds, TakesOff, Received}
	var listed []KeyedOutcome
	for range rng.IntN(30) {
		var outcome Outcome
		for range rng.IntN(4) {
			outcome.Lines = append(outcome.Lines, LineAmount{
				Line: LineID(rng.IntN(3) + 1), Effect: effects[rng.IntN(len(effects))], Amount: money.Cents(rng.Int64N(50_000)),
			})
		}
		listed = append(listed, KeyedOutcome{EstablishmentKey: keys[rng.IntN(len(keys))], Outcome: outcome})
	}
	return listed
}

func establishmentProblems(listed []KeyedOutcome) []string {
	outcomes := make([]Outcome, 0, len(listed))
	for _, entry := range listed {
		outcomes = append(outcomes, entry.Outcome)
	}
	figures := LineFigures(outcomes)
	var problems []string
	for line := LineID(1); line <= 3; line++ {
		bearing := 0
		for _, entry := range listed {
			if _, bears := onLine(line, entry.Outcome); bears {
				bearing++
			}
		}
		var sum money.Cents
		count := 0
		for _, total := range EstablishmentsOfLine(line, listed) {
			sum, count = sum+total.Amount, count+total.Count
		}
		if sum != figures[line] || count != bearing {
			problems = append(problems, fmt.Sprintf("line %d: the establishments add up to %d over %d transactions, the figure is %d over %d",
				line, sum, count, figures[line], bearing))
		}
	}
	return problems
}

func TestEveryGeneratedPlanAddsUpAndEveryLinesEstablishmentsMakeItsFigure(t *testing.T) {
	seed := *seedFlag
	if seed == 0 {
		seed = rand.Uint64()
	}
	reproduce := fmt.Sprintf("reproduce with: go test ./internal/budgetrules -run %s -seed=%d", t.Name(), seed)
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	failures := 0
	for index := range generatedPlans {
		plan := generatedPlan(rng)
		problems := append(planProblems(plan, View(plan, generatedFacts(rng))), establishmentProblems(generatedOutcomes(rng))...)
		for _, problem := range problems {
			failures++
			if failures <= 10 {
				t.Errorf("case %d: %s\n  %s", index, problem, reproduce)
			}
		}
	}
}

func TestThePlanCheckSeesASubtotalOffItsLinesAndALineDropped(t *testing.T) {
	plan := november()
	view := View(plan, novemberFacts())
	if problems := planProblems(plan, view); len(problems) != 0 {
		t.Fatalf("the November budget was accused: %v", problems)
	}
	view.Groups[1].Subtotal++
	view.Groups[2].Lines = view.Groups[2].Lines[1:]
	bar := int64(101)
	view.Groups[0].Lines[0].Bar = &bar
	if problems := planProblems(plan, view); len(problems) != 4 {
		t.Errorf("a subtotal off, a line dropped and a bar past 100 gave %d problems: %v", len(problems), problems)
	}
}
