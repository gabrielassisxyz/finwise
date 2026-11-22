package budgetrules

import "github.com/gabrielassisxyz/finwise/internal/money"

// The groups the budget screen derives from the five a person edits.
const (
	GroupRainyFunds Group = "rf"
	GroupFees       Group = "tar"
	GroupInvestment Group = "inv"
)

// EditableGroups are the five groups of a budget in the order every view
// lists them.
var EditableGroups = []Group{GroupIncome, GroupFixed, GroupVariable, GroupProvisions, GroupReserves}

// PlannedLine is a line of a budget, or of a draft being weighed. ID is unset
// for a line a draft has just added. Amount is at or above zero.
type PlannedLine struct {
	ID     LineRef
	Name   string
	Amount money.Cents
	Flag   string
}

// PlanGroup is one group of a plan with its lines in their order.
type PlanGroup struct {
	Key   Group
	Lines []PlannedLine
}

// Plan is a budget's groups as they were given. A draft may leave a group out,
// send the groups in any order or send one key twice; a group's lines are then
// every line sent under its key, in the order sent, so none is dropped.
type Plan []PlanGroup

// linesOf is every line the plan has under key, in the order sent.
func (plan Plan) linesOf(key Group) []PlannedLine {
	var lines []PlannedLine
	for _, group := range plan {
		if group.Key == key {
			lines = append(lines, group.Lines...)
		}
	}
	return lines
}

func totalOf(lines []PlannedLine) money.Cents {
	var total money.Cents
	for _, line := range lines {
		total += line.Amount
	}
	return total
}

func (plan Plan) total(key Group) money.Cents { return totalOf(plan.linesOf(key)) }

func (plan Plan) outgoing() money.Cents {
	return plan.total(GroupFixed) + plan.total(GroupVariable) + plan.total(GroupProvisions) + plan.total(GroupReserves)
}

// Investment is what a plan leaves to invest: its income less everything else
// it plans. It is negative when the plan outgrows the income. The payday's
// transfer and the net worth's rate take it from here, so that they answer
// what the budget shows.
func Investment(plan Plan) money.Cents {
	return plan.total(GroupIncome) - plan.outgoing()
}

// InvestmentRate is the investment as a whole percent of the income, and nil
// when the plan has no income to take a share of.
func InvestmentRate(plan Plan) *int64 {
	income := plan.total(GroupIncome)
	if income <= 0 {
		return nil
	}
	rate := RoundedPercent(Investment(plan), income)
	return &rate
}

// Signed is what an amount does to its line's figure: spending and income
// received add to it, a discount or a refund takes off.
func (amount LineAmount) Signed() money.Cents {
	if amount.Effect == TakesOff {
		return -amount.Amount
	}
	return amount.Amount
}

// LineFigures is, by line id, what a month's outcomes add to each line: what
// was spent on it net of what took spending back, or what it received.
func LineFigures(outcomes []Outcome) map[LineID]money.Cents {
	figures := map[LineID]money.Cents{}
	for _, outcome := range outcomes {
		for _, line := range outcome.Lines {
			figures[line.Line] += line.Signed()
		}
	}
	return figures
}

// ListedPot is a pot as the ledger lists it now.
type ListedPot struct {
	ID      string
	Balance money.Cents
	Closed  bool
}

// PotBalances is, for every line whose link to a pot counts, that pot's
// balance. A link counts only while the ledger lists its pot and the pot is
// open: a pot gone or closed has no balance to show under a line.
func PotBalances(links map[LineID]string, listed []ListedPot) map[LineID]money.Cents {
	open := map[string]money.Cents{}
	for _, pot := range listed {
		if !pot.Closed {
			open[pot.ID] = pot.Balance
		}
	}
	balances := map[LineID]money.Cents{}
	for line, potID := range links {
		if balance, counted := open[potID]; counted {
			balances[line] = balance
		}
	}
	return balances
}
