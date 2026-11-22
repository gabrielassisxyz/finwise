package budgetrules

import (
	"reflect"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

// The prototype's November budget (BUDGET in web/src/api/fake/prototype-data.ts)
// in cents, each line with an invented id and, as its figure, what the
// stand-in's seeded baseline says it spent or received in 2026-11.
type novemberLine struct {
	id     LineID
	group  Group
	name   string
	amount money.Cents
	flag   string
	figure money.Cents
}

const (
	gasLine     LineID = 117
	remedyLine  LineID = 119
	tripLine    LineID = 121
	openPot            = "reserve:acc/gas"
	tripPot            = "reserve:acc/trip"
	closedPot          = "reserve:acc/old"
	unlistedPot        = "reserve:gone/pot"
	novemberFee        = 2500
)

var novemberLines = []novemberLine{
	{101, GroupIncome, "Salário", 1000000, "", 1000000},
	{102, GroupIncome, "Renda extra", 150000, "Não garantida", 20000},
	{103, GroupIncome, "Reembolsos", 50000, "", 15000},
	{104, GroupFixed, "Contas da casa", 200000, "", 205990},
	{105, GroupFixed, "Saúde", 50000, "", 48000},
	{106, GroupFixed, "Pets", 20000, "", 9600},
	{107, GroupFixed, "Transporte para o trabalho", 120000, "", 120000},
	{108, GroupVariable, "Mercado", 40000, "", 30800},
	{109, GroupVariable, "Restaurantes e bares", 45000, "", 32415},
	{110, GroupVariable, "Delivery", 20000, "", 20600},
	{111, GroupVariable, "Transporte", 28000, "", 18000},
	{112, GroupVariable, "Casa", 22000, "", 12900},
	{113, GroupVariable, "Assinaturas", 18000, "", 13390},
	{114, GroupVariable, "Ferramentas de trabalho", 60000, "", 11385},
	{115, GroupVariable, "Lazer", 40000, "", 6510},
	{116, GroupVariable, "Educação", 15000, "", 0},
	{gasLine, GroupProvisions, "Gás e IPTU", 12000, "", 0},
	{118, GroupProvisions, "Assinaturas anuais", 5000, "", 0},
	{remedyLine, GroupProvisions, "Remédios", 15000, "", 0},
	{120, GroupReserves, "Compras", 50000, "", 0},
	{tripLine, GroupReserves, "Viagens", 60000, "", 42000},
}

func november() Plan {
	plan := Plan{}
	for _, key := range EditableGroups {
		group := PlanGroup{Key: key, Lines: []PlannedLine{}}
		for _, line := range novemberLines {
			if line.group == key {
				group.Lines = append(group.Lines, PlannedLine{ID: OnLine(line.id), Name: line.name, Amount: line.amount, Flag: line.flag})
			}
		}
		plan = append(plan, group)
	}
	return plan
}

func novemberLinks() map[LineID]string {
	return map[LineID]string{gasLine: openPot, tripLine: tripPot}
}

func novemberPots() []ListedPot {
	return []ListedPot{{ID: openPot, Balance: 60000}, {ID: tripPot, Balance: 235000}, {ID: closedPot, Balance: 900, Closed: true}}
}

func novemberFacts() MonthFacts {
	figures := map[LineID]money.Cents{feesLine: novemberFee}
	for _, line := range novemberLines {
		if line.figure != 0 {
			figures[line.id] = line.figure
		}
	}
	return MonthFacts{Figures: figures, PotBalances: PotBalances(novemberLinks(), novemberPots()), FeesLine: feesLine}
}

// edited is a copy of plan with change applied to the line called name.
func edited(plan Plan, name string, change func(*PlannedLine)) Plan {
	copied := make(Plan, len(plan))
	for i, group := range plan {
		copied[i] = PlanGroup{Key: group.Key, Lines: slices.Clone(group.Lines)}
		for j := range copied[i].Lines {
			if copied[i].Lines[j].Name == name {
				change(&copied[i].Lines[j])
			}
		}
	}
	return copied
}

func atAmount(plan Plan, name string, amount money.Cents) Plan {
	return edited(plan, name, func(line *PlannedLine) { line.Amount = amount })
}

func groupIn(t *testing.T, view BudgetView, key Group) GroupView {
	t.Helper()
	for _, group := range view.Groups {
		if group.Key == key {
			return group
		}
	}
	t.Fatalf("the view has no group %s", key)
	return GroupView{}
}

func lineIn(t *testing.T, view BudgetView, key Group, name string) LineView {
	t.Helper()
	for _, line := range groupIn(t, view, key).Lines {
		if line.Name == name {
			return line
		}
	}
	t.Fatalf("the group %s has no line %s", key, name)
	return LineView{}
}

func cash(amount money.Cents) Cell       { return Cell{Kind: CellMoney, Amount: amount} }
func difference(amount money.Cents) Cell { return Cell{Kind: CellDifference, Amount: amount} }
func percentOf(value int64) Cell         { return Cell{Kind: CellPercent, Percent: &value} }

var (
	noCell     = Cell{Kind: CellNone}
	nextCharge = Cell{Kind: CellNextCharge}
)

func whole(value int64) *int64 { return &value }

func cents(amount money.Cents) *money.Cents { return &amount }

func expectCells(t *testing.T, label string, line LineView, want [3]Cell) {
	t.Helper()
	if !reflect.DeepEqual(line.Cells, want) {
		t.Errorf("%s: cells %+v, want %+v", label, line.Cells, want)
	}
}

func expectGroup(t *testing.T, group GroupView, subtotal money.Cents, spent *money.Cents, over bool) {
	t.Helper()
	if group.Subtotal != subtotal || !reflect.DeepEqual(group.SubtotalSpent, spent) || group.Over != over {
		t.Errorf("group %s: subtotal %d, spent %v, over %v; want %d, %v, %v", group.Key, group.Subtotal,
			group.SubtotalSpent, group.Over, subtotal, spent, over)
	}
}

func TestTheViewWeighsTheIncomeAgainstEverythingPlannedAndRatesTheInvestment(t *testing.T) {
	view := View(november(), novemberFacts())
	if view.Income != 1200000 || view.Allocated != 1200000 || view.Investment != 380000 || !reflect.DeepEqual(view.Rate, whole(32)) {
		t.Errorf("income %d, allocated %d, investment %d, rate %v; want 1200000, 1200000, 380000, 32",
			view.Income, view.Allocated, view.Investment, view.Rate)
	}
	expectGroup(t, groupIn(t, view, GroupInvestment), 380000, nil, false)
	investment := groupIn(t, view, GroupInvestment).Lines[0]
	expectCells(t, "the investment line", investment, [3]Cell{cash(380000), cash(0), percentOf(32)})
	var keys []Group
	for _, group := range view.Groups {
		keys = append(keys, group.Key)
	}
	if want := []Group{"rec", "fix", "var", "prov", "res", "rf", "tar", "inv"}; !slices.Equal(keys, want) {
		t.Errorf("groups %v, want %v", keys, want)
	}
}

func TestAFixedLineSaysWhatIsLeftAndIsOverOnlyPastItsAmount(t *testing.T) {
	view := View(november(), novemberFacts())
	house := lineIn(t, view, GroupFixed, "Contas da casa")
	expectCells(t, "Contas da casa", house, [3]Cell{cash(200000), cash(205990), difference(-5990)})
	if !house.Over || house.Bar != nil || len(house.Marks) != 0 || !house.Expandable {
		t.Errorf("Contas da casa: over %v, bar %v, marks %v, expandable %v; want over, no bar, no mark, expandable",
			house.Over, house.Bar, house.Marks, house.Expandable)
	}
	exact := lineIn(t, view, GroupFixed, "Transporte para o trabalho")
	if exact.Over || exact.Cells[2] != difference(0) {
		t.Errorf("a fixed line that spent its amount exactly: over %v, left %+v", exact.Over, exact.Cells[2])
	}
	expectGroup(t, groupIn(t, view, GroupFixed), 390000, cents(383590), false)
}

func TestAVariableLineIsACapDrawnAsARoundedBar(t *testing.T) {
	view := View(november(), novemberFacts())
	delivery := lineIn(t, view, GroupVariable, "Delivery")
	if !delivery.Over || !reflect.DeepEqual(delivery.Bar, whole(100)) || !reflect.DeepEqual(delivery.Marks, []Mark{{Kind: MarkOverspent}}) ||
		delivery.Cells[2] != difference(-600) {
		t.Errorf("Delivery: %+v", delivery)
	}
	market := lineIn(t, view, GroupVariable, "Mercado")
	if market.Over || !reflect.DeepEqual(market.Bar, whole(77)) || len(market.Marks) != 0 {
		t.Errorf("Mercado: %+v", market)
	}
	if education := lineIn(t, view, GroupVariable, "Educação"); !reflect.DeepEqual(education.Bar, whole(0)) {
		t.Errorf("Educação's bar is %v, want 0", education.Bar)
	}
	expectGroup(t, groupIn(t, view, GroupVariable), 288000, cents(146000), false)
}

func TestTheVariableGroupIsOverOnlyWhenItsLinesTogetherSpentPastTheirCaps(t *testing.T) {
	low, exact := november(), november()
	figures := novemberFacts().Figures
	for i := range low[2].Lines {
		low[2].Lines[i].Amount = 10000
		exact[2].Lines[i].Amount = figures[exact[2].Lines[i].ID.ID]
	}
	expectGroup(t, groupIn(t, View(low, novemberFacts()), GroupVariable), 90000, cents(146000), true)
	expectGroup(t, groupIn(t, View(exact, novemberFacts()), GroupVariable), 146000, cents(146000), false)
	// A fixed group past its plan is never over as a whole: only its lines are.
	fixed := november()
	for i := range fixed[1].Lines {
		fixed[1].Lines[i].Amount = 100
	}
	expectGroup(t, groupIn(t, View(fixed, novemberFacts()), GroupFixed), 400, cents(383590), false)
}

func TestAnIncomeLineCarriesItsFlagAndWhatWasReceived(t *testing.T) {
	extra := lineIn(t, View(november(), novemberFacts()), GroupIncome, "Renda extra")
	if !reflect.DeepEqual(extra.Marks, []Mark{{Kind: MarkFlag, Flag: "Não garantida"}}) || !extra.Expandable || extra.Bar != nil {
		t.Errorf("Renda extra: %+v", extra)
	}
	expectCells(t, "Renda extra", extra, [3]Cell{cash(150000), cash(20000), noCell})
}

func TestAProvisionAndAReserveShowTheBalanceOfTheirCountedPotOnly(t *testing.T) {
	view := View(november(), novemberFacts())
	expectCells(t, "Gás e IPTU", lineIn(t, view, GroupProvisions, "Gás e IPTU"), [3]Cell{cash(12000), cash(60000), nextCharge})
	expectCells(t, "Remédios, linked to nothing", lineIn(t, view, GroupProvisions, "Remédios"), [3]Cell{cash(15000), noCell, nextCharge})
	provisions := groupIn(t, view, GroupProvisions)
	expectGroup(t, provisions, 32000, nil, false)
	for _, line := range provisions.Lines {
		if line.Expandable || line.Over || line.Bar != nil {
			t.Errorf("the provision %s is expandable, over or drawn: %+v", line.Name, line)
		}
	}
	trip := lineIn(t, view, GroupReserves, "Viagens")
	expectCells(t, "Viagens", trip, [3]Cell{cash(60000), cash(235000), cash(42000)})
	if !trip.Expandable {
		t.Error("a reserve line opens into its establishments")
	}

	links := novemberLinks()
	links[gasLine], links[remedyLine] = closedPot, unlistedPot
	facts := novemberFacts()
	facts.PotBalances = PotBalances(links, novemberPots())
	view = View(november(), facts)
	expectCells(t, "a line linked to a closed pot", lineIn(t, view, GroupProvisions, "Gás e IPTU"), [3]Cell{cash(12000), noCell, nextCharge})
	expectCells(t, "a line linked to a pot not listed", lineIn(t, view, GroupProvisions, "Remédios"), [3]Cell{cash(15000), noCell, nextCharge})
}

func TestALinkCountsOnlyWhileItsPotIsListedAndOpen(t *testing.T) {
	links := map[LineID]string{1: openPot, 2: closedPot, 3: unlistedPot, 4: tripPot}
	got := PotBalances(links, novemberPots())
	want := map[LineID]money.Cents{1: 60000, 4: 235000}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("balances %v, want %v", got, want)
	}
}

func TestTheDerivedGroupsAreTheRainyFundsTheFeesAndTheInvestment(t *testing.T) {
	view := View(november(), novemberFacts())
	rainy := groupIn(t, view, GroupRainyFunds)
	if rainy.Subtotal != 0 || rainy.SubtotalSpent != nil || rainy.Over || len(rainy.Lines) != 0 {
		t.Errorf("the rainy funds: %+v", rainy)
	}
	fees := groupIn(t, view, GroupFees)
	expectGroup(t, fees, novemberFee, nil, false)
	if len(fees.Lines) != 1 || !fees.Lines[0].Expandable || fees.Lines[0].Name != "" {
		t.Fatalf("the fees group's lines: %+v", fees.Lines)
	}
	expectCells(t, "the fees line", fees.Lines[0], [3]Cell{cash(novemberFee), noCell, noCell})
	if investment := groupIn(t, view, GroupInvestment).Lines[0]; investment.Expandable || investment.Name != "" {
		t.Errorf("the investment line: %+v", investment)
	}
}

func TestABudgetPastItsIncomeHasANegativeInvestmentAndRate(t *testing.T) {
	view := View(atAmount(november(), "Salário", 100000), novemberFacts())
	investment := groupIn(t, view, GroupInvestment)
	expectGroup(t, investment, -520000, nil, true)
	line := investment.Lines[0]
	if !line.Over || !reflect.DeepEqual(line.Marks, []Mark{{Kind: MarkOverIncome}}) {
		t.Errorf("the investment line of a budget past its income: %+v", line)
	}
	if view.Income != 300000 || !reflect.DeepEqual(view.Rate, whole(-173)) || !reflect.DeepEqual(line.Cells[2], percentOf(-173)) {
		t.Errorf("income %d, rate %v, cell %+v; want 300000 and -173", view.Income, view.Rate, line.Cells[2])
	}
	health := View(atAmount(november(), "Saúde", 90000), novemberFacts())
	expectGroup(t, groupIn(t, health, GroupFixed), 430000, cents(383590), false)
}

func TestTheInvestmentMadeAndTheClosedMonthAreTheInputs(t *testing.T) {
	for _, made := range []money.Cents{0, 380000} {
		facts := novemberFacts()
		facts.Made, facts.Closed = made, made > 0
		view := View(november(), facts)
		if cell := groupIn(t, view, GroupInvestment).Lines[0].Cells[1]; cell != cash(made) || view.Closed != facts.Closed {
			t.Errorf("made %d: the cell is %+v and closed %v", made, cell, view.Closed)
		}
	}
}

// The rows the stand-in has no case for.
func TestALineIsMatchedToItsFiguresByIdAlone(t *testing.T) {
	renamed := edited(november(), "Mercado", func(line *PlannedLine) { line.Name = "Feira" })
	renamed[2].Lines = append(renamed[2].Lines, PlannedLine{Name: "Delivery", Amount: 5000})
	view := View(renamed, novemberFacts())
	expectCells(t, "Mercado renamed under its id", lineIn(t, view, GroupVariable, "Feira"), [3]Cell{cash(40000), cash(30800), difference(9200)})
	added := groupIn(t, view, GroupVariable).Lines[9]
	expectCells(t, "a new line named as an old one", added, [3]Cell{cash(5000), cash(0), difference(5000)})
	if !reflect.DeepEqual(added.Bar, whole(0)) {
		t.Errorf("a new line's bar is %v, want 0", added.Bar)
	}
	moved := november()
	moved[4].Lines = append(moved[4].Lines, moved[2].Lines[0])
	moved[2].Lines = moved[2].Lines[1:]
	expectCells(t, "Mercado moved to the reserves", lineIn(t, View(moved, novemberFacts()), GroupReserves, "Mercado"),
		[3]Cell{cash(40000), noCell, cash(30800)})
}

func TestRefundsLargerThanTheSpendingLeaveTheBarAtZero(t *testing.T) {
	facts := novemberFacts()
	facts.Figures[108] = -2000
	market := lineIn(t, View(november(), facts), GroupVariable, "Mercado")
	expectCells(t, "Mercado with more refunded than spent", market, [3]Cell{cash(40000), cash(-2000), difference(42000)})
	if market.Over || !reflect.DeepEqual(market.Bar, whole(0)) {
		t.Errorf("Mercado: over %v, bar %v; want not over and 0", market.Over, market.Bar)
	}
}

func TestAnEmptyBudgetHasItsFiveGroupsAndCountsTheFees(t *testing.T) {
	view := View(Plan{}, MonthFacts{Figures: map[LineID]money.Cents{feesLine: 2500}, FeesLine: feesLine})
	if view.Income != 0 || view.Allocated != 0 || view.Investment != 0 || view.Rate != nil || len(view.Groups) != 8 {
		t.Fatalf("an empty budget: %+v", view)
	}
	for _, key := range EditableGroups {
		group := groupIn(t, view, key)
		if len(group.Lines) != 0 || group.Lines == nil {
			t.Errorf("%s has lines %v", key, group.Lines)
		}
	}
	expectGroup(t, groupIn(t, view, GroupFixed), 0, cents(0), false)
	expectGroup(t, groupIn(t, view, GroupVariable), 0, cents(0), false)
	expectGroup(t, groupIn(t, view, GroupIncome), 0, nil, false)
	expectGroup(t, groupIn(t, view, GroupFees), 2500, nil, false)
	expectCells(t, "the investment line", groupIn(t, view, GroupInvestment).Lines[0], [3]Cell{cash(0), cash(0), {Kind: CellPercent}})
}

// A draft is a document as typed: its groups in any order, one key twice. The
// view lists the five groups in their order, and a key sent twice keeps every
// line of both, in the order sent.
func TestADraftsGroupsAreListedInTheirOrderAndAKeySentTwiceKeepsEveryLine(t *testing.T) {
	plan := november()
	draft := Plan{plan[4], plan[2], {Key: GroupVariable, Lines: []PlannedLine{{Name: "Pets", Amount: 10000}}}, plan[0]}
	view := View(draft, novemberFacts())
	var keys []Group
	for _, group := range view.Groups[:5] {
		keys = append(keys, group.Key)
	}
	if !slices.Equal(keys, EditableGroups) {
		t.Errorf("the editable groups are %v, want %v", keys, EditableGroups)
	}
	variable := groupIn(t, view, GroupVariable)
	if len(variable.Lines) != 10 || variable.Lines[9].Name != "Pets" || variable.Subtotal != 298000 {
		t.Errorf("the variable group of a key sent twice: %d lines, the last %q, subtotal %d", len(variable.Lines),
			variable.Lines[len(variable.Lines)-1].Name, variable.Subtotal)
	}
	if fixed := groupIn(t, view, GroupFixed); len(fixed.Lines) != 0 || view.Investment != 1200000-298000-110000 {
		t.Errorf("a group left out has %d lines; the investment is %d", len(fixed.Lines), view.Investment)
	}
}
