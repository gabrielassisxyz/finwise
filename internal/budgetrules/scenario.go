package budgetrules

import "github.com/gabrielassisxyz/finwise/internal/money"

// InvestmentFigures are what a plan invests and its rate, as View answers them.
type InvestmentFigures struct {
	Investment money.Cents
	Rate       *int64
}

// Impact is a scenario weighed against the month's budget.
type Impact struct {
	Current    InvestmentFigures
	Scenario   InvestmentFigures
	Difference money.Cents
	// Changed has one entry per group of the scenario and one per line of it,
	// in the order they were sent, since the screen draws it by those places.
	Changed [][]bool
}

// ScenarioImpact weighs scenario against current, the budget in force for the
// month. A scenario line is unchanged only when current has a line of its id
// at the same amount, wherever that line sits and whatever it is called: a
// line without an id, or with one the month's budget lacks, is changed.
func ScenarioImpact(current, scenario Plan) Impact {
	amounts := map[LineID]money.Cents{}
	for _, group := range current {
		for _, line := range group.Lines {
			if line.ID.Set {
				amounts[line.ID.ID] = line.Amount
			}
		}
	}
	changed := make([][]bool, 0, len(scenario))
	for _, group := range scenario {
		lines := make([]bool, 0, len(group.Lines))
		for _, line := range group.Lines {
			amount, known := amounts[line.ID.ID]
			lines = append(lines, !line.ID.Set || !known || amount != line.Amount)
		}
		changed = append(changed, lines)
	}
	before, after := figuresOf(current), figuresOf(scenario)
	return Impact{Current: before, Scenario: after, Difference: after.Investment - before.Investment, Changed: changed}
}

func figuresOf(plan Plan) InvestmentFigures {
	return InvestmentFigures{Investment: Investment(plan), Rate: InvestmentRate(plan)}
}
