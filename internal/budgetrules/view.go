package budgetrules

import "github.com/gabrielassisxyz/finwise/internal/money"

// MonthFacts is what a plan is weighed against in one month.
type MonthFacts struct {
	// Figures is what the month's outcomes add to each line, as LineFigures
	// sums them. A line it lacks has a figure of zero.
	Figures map[LineID]money.Cents
	// PotBalances is the balance of the pot each line's link counts for, as
	// PotBalances answers it; a line it lacks has no balance to show.
	PotBalances map[LineID]money.Cents
	FeesLine    LineID
	Closed      bool
	// Made is what the investment line shows as already invested.
	Made money.Cents
}

// CellKind is what one cell of a line shows.
type CellKind int

const (
	CellNone CellKind = iota
	CellMoney
	// CellDifference is money with a sign: what is left of a line's amount.
	CellDifference
	CellPercent
	// CellNextCharge is a provision's next charge, which has no source yet and
	// is always the contract's "no date".
	CellNextCharge
)

// Cell is one value of a line. Amount is a money or difference cell's; Percent
// is a percent cell's, nil when it is unknown.
type Cell struct {
	Kind    CellKind
	Amount  money.Cents
	Percent *int64
}

// MarkKind is what a mark on a line says. The words are the interface's.
type MarkKind int

const (
	// MarkFlag is the line's own flag, in Flag.
	MarkFlag MarkKind = iota + 1
	// MarkOverspent is a variable line that spent past its amount.
	MarkOverspent
	// MarkOverIncome is a budget that plans more than its income.
	MarkOverIncome
)

type Mark struct {
	Kind MarkKind
	Flag string
}

// LineView is one row of the budget screen. The one line of a derived group
// has no Name of its own: it is named as its group.
type LineView struct {
	Name       string
	Cells      [3]Cell
	Marks      []Mark
	Over       bool
	Bar        *int64
	Expandable bool
}

// GroupView is one group of the screen. SubtotalSpent is set only for a group
// whose subtotal is a cap to compare spending with.
type GroupView struct {
	Key           Group
	Subtotal      money.Cents
	SubtotalSpent *money.Cents
	Over          bool
	Lines         []LineView
}

// BudgetView is every figure of the budget screen for one month. Rate is nil
// when there is no income to take a share of.
type BudgetView struct {
	Closed     bool
	Income     money.Cents
	Allocated  money.Cents
	Investment money.Cents
	Rate       *int64
	Groups     []GroupView
}

// View weighs a plan, the month's budget or a draft, against the month. A line
// is matched to its spending and its pot by its id alone, so a renamed or moved
// line keeps both and a line without an id has neither.
func View(plan Plan, facts MonthFacts) BudgetView {
	investment, rate := Investment(plan), InvestmentRate(plan)
	groups := make([]GroupView, 0, len(EditableGroups)+3)
	for _, key := range EditableGroups {
		groups = append(groups, facts.editableGroup(key, plan.linesOf(key)))
	}
	fees := facts.Figures[facts.FeesLine]
	groups = append(groups,
		GroupView{Key: GroupRainyFunds, Lines: []LineView{}},
		GroupView{Key: GroupFees, Subtotal: fees, Lines: []LineView{{
			Cells: [3]Cell{moneyCell(fees), {}, {}}, Marks: []Mark{}, Expandable: true,
		}}},
		investmentGroup(investment, rate, facts.Made),
	)
	return BudgetView{
		Closed: facts.Closed, Income: plan.total(GroupIncome), Allocated: plan.outgoing() + investment,
		Investment: investment, Rate: rate, Groups: groups,
	}
}

func moneyCell(amount money.Cents) Cell { return Cell{Kind: CellMoney, Amount: amount} }

func (facts MonthFacts) figureOf(line PlannedLine) money.Cents {
	if !line.ID.Set {
		return 0
	}
	return facts.Figures[line.ID.ID]
}

// potCell is the balance of the line's counted pot, and nothing for a line
// with none.
func (facts MonthFacts) potCell(line PlannedLine) Cell {
	if !line.ID.Set {
		return Cell{}
	}
	balance, counted := facts.PotBalances[line.ID.ID]
	if !counted {
		return Cell{}
	}
	return moneyCell(balance)
}

// editableGroup is one of the five groups. Only fixed and variable compare
// their lines with what was spent, and only variable is over as a whole.
func (facts MonthFacts) editableGroup(key Group, lines []PlannedLine) GroupView {
	group := GroupView{Key: key, Subtotal: totalOf(lines), Lines: make([]LineView, 0, len(lines))}
	var spent money.Cents
	for _, line := range lines {
		group.Lines = append(group.Lines, facts.lineOf(key, line))
		spent += facts.figureOf(line)
	}
	if key == GroupFixed || key == GroupVariable {
		group.SubtotalSpent = &spent
		group.Over = key == GroupVariable && spent > group.Subtotal
	}
	return group
}

func (facts MonthFacts) lineOf(key Group, line PlannedLine) LineView {
	view := LineView{Name: line.Name, Marks: []Mark{}}
	amount := moneyCell(line.Amount)
	switch key {
	case GroupIncome:
		view.Cells = [3]Cell{amount, moneyCell(facts.figureOf(line)), {}}
		view.Expandable = true
		if line.Flag != "" {
			view.Marks = append(view.Marks, Mark{Kind: MarkFlag, Flag: line.Flag})
		}
	case GroupFixed, GroupVariable:
		facts.outgoingLine(key, line, &view)
	case GroupProvisions:
		view.Cells = [3]Cell{amount, facts.potCell(line), {Kind: CellNextCharge}}
	case GroupReserves:
		view.Cells = [3]Cell{amount, facts.potCell(line), moneyCell(facts.figureOf(line))}
		view.Expandable = true
	}
	return view
}

// outgoingLine is a fixed or a variable line: what is left of its amount, over
// once strictly past it. A variable line's amount is a cap, drawn as a bar.
func (facts MonthFacts) outgoingLine(key Group, line PlannedLine, view *LineView) {
	done := facts.figureOf(line)
	left := line.Amount - done
	view.Cells = [3]Cell{moneyCell(line.Amount), moneyCell(done), {Kind: CellDifference, Amount: left}}
	view.Over = left < 0
	view.Expandable = true
	if key != GroupVariable {
		return
	}
	bar := Share(done, line.Amount)
	view.Bar = &bar
	if view.Over {
		view.Marks = append(view.Marks, Mark{Kind: MarkOverspent})
	}
}

func investmentGroup(investment money.Cents, rate *int64, made money.Cents) GroupView {
	over := investment < 0
	line := LineView{
		Cells: [3]Cell{moneyCell(investment), moneyCell(made), {Kind: CellPercent, Percent: rate}},
		Marks: []Mark{}, Over: over,
	}
	if over {
		line.Marks = append(line.Marks, Mark{Kind: MarkOverIncome})
	}
	return GroupView{Key: GroupInvestment, Subtotal: investment, Over: over, Lines: []LineView{line}}
}
