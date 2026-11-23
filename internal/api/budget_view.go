package api

import (
	"context"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// The budget screen's figures are worked out by budgetrules from the plan, the
// month's dispositions and the pots; this file gathers those facts and writes
// the answer in the contract's shape. Every group is always there, so an empty
// budget is still its places: the five a person fills in and the three the
// screen derives.

const (
	groupFixed    = "fix"
	groupVariable = "var"
)

var editableGroupNames = []struct{ key, name string }{
	{"rec", budgetGroupIncome},
	{groupFixed, budgetGroupFixed},
	{groupVariable, budgetGroupVariable},
	{"prov", budgetGroupProvisions},
	{"res", budgetGroupWishes},
}

// derivedGroupNames name the three derived groups, and the one line each of the
// fees and the investment has.
var derivedGroupNames = map[budgetrules.Group]string{
	budgetrules.GroupRainyFunds: budgetGroupRainyFunds,
	budgetrules.GroupFees:       budgetGroupFees,
	budgetrules.GroupInvestment: budgetGroupInvestment,
}

type budgetGroupDoc struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Lines []any  `json:"lines"`
}

type budgetGroupView struct {
	Key           string           `json:"key"`
	Name          string           `json:"name"`
	Subtotal      Reais            `json:"subtotal"`
	SubtotalSpent *Reais           `json:"subtotalSpent"`
	Over          bool             `json:"over"`
	Lines         []budgetLineView `json:"lines"`
}

type budgetLineView struct {
	Name       string           `json:"name"`
	Cells      [3]any           `json:"cells"`
	Marks      []budgetMarkWire `json:"marks"`
	Over       bool             `json:"over"`
	Bar        *int64           `json:"bar"`
	Expandable bool             `json:"expandable"`
}

type budgetMarkWire struct {
	Text string `json:"text"`
	Tone string `json:"tone"`
}

type noneCell struct {
	Type string `json:"type"`
}

type moneyCell struct {
	Type   string `json:"type"`
	Amount Reais  `json:"amount"`
}

type percentCell struct {
	Type  string `json:"type"`
	Value *int64 `json:"value"`
}

// nextChargeCell is a provision's next charge; nothing knows one yet, so its
// charge is always the contract's "no date".
type nextChargeCell struct {
	Type   string `json:"type"`
	Charge *struct {
		Description string `json:"description"`
		Day         string `json:"day"`
	} `json:"charge"`
}

func none() noneCell                 { return noneCell{Type: "none"} }
func moneyOf(amount Reais) moneyCell { return moneyCell{Type: "money", Amount: amount} }

func emptyBudgetDocument() []budgetGroupDoc {
	groups := make([]budgetGroupDoc, 0, len(editableGroupNames))
	for _, group := range editableGroupNames {
		groups = append(groups, budgetGroupDoc{Key: group.key, Name: group.name, Lines: []any{}})
	}
	return groups
}

// planOfBudget is the budget in force for a month as budgetrules weighs it:
// the five groups in their order, every line with its id.
func planOfBudget(budget authored.Budget) budgetrules.Plan {
	plan := make(budgetrules.Plan, 0, len(authored.GroupKeys))
	for _, key := range authored.GroupKeys {
		plan = append(plan, budgetrules.PlanGroup{Key: budgetrules.Group(key), Lines: []budgetrules.PlannedLine{}})
	}
	for _, line := range budget.Lines {
		at := slices.Index(authored.GroupKeys, line.Group)
		plan[at].Lines = append(plan[at].Lines, budgetrules.PlannedLine{
			ID: budgetrules.OnLine(budgetrules.LineID(line.ID)), Name: line.Name, Amount: line.Cents, Flag: line.Flag,
		})
	}
	return plan
}

// planOfDocument is a draft as it was sent: its groups in their order, one key
// possibly twice. A draft is weighed as typed, so an id that is no line's
// spelling is a line without one, which nothing has been spent on.
func planOfDocument(document authored.Document) budgetrules.Plan {
	plan := make(budgetrules.Plan, 0, len(document))
	for _, group := range document {
		lines := make([]budgetrules.PlannedLine, 0, len(group.Lines))
		for _, line := range group.Lines {
			planned := budgetrules.PlannedLine{Name: line.Name, Amount: line.Cents, Flag: line.Flag}
			if line.ID != nil {
				if id, ok := authored.ParseLineID(*line.ID); ok {
					planned.ID = budgetrules.OnLine(budgetrules.LineID(id))
				}
			}
			lines = append(lines, planned)
		}
		plan = append(plan, budgetrules.PlanGroup{Key: budgetrules.Group(group.Key), Lines: lines})
	}
	return plan
}

// monthFactsOf is what the plan is weighed against in the month: what the
// month's transactions add to each line, the balance of each counted pot, and
// whether the month is closed. The investment made follows the payday's
// transfer, which nothing marks yet, so it is nothing made.
func monthFactsOf(ctx context.Context, views Views, month brday.Month, plan budgetrules.Plan, disposed monthDispositions) (budgetrules.MonthFacts, error) {
	balances, err := countedPotBalances(ctx, views, plan)
	if err != nil {
		return budgetrules.MonthFacts{}, err
	}
	_, closed, err := views.Authored.JournaledRow(ctx, "closed_months", []authored.Value{authored.Text(string(month))})
	if err != nil {
		return budgetrules.MonthFacts{}, err
	}
	return budgetrules.MonthFacts{
		Figures: budgetrules.LineFigures(disposed.outcomes), PotBalances: balances,
		FeesLine: budgetrules.LineID(feesLineID), Closed: closed,
	}, nil
}

// countedPotBalances is the balance of the pot each provision and reserve line
// of the plan is linked to, where the link counts. The pots are read without
// the yield, which a balance does not need, and only when a line has a link.
func countedPotBalances(ctx context.Context, views Views, plan budgetrules.Plan) (map[budgetrules.LineID]money.Cents, error) {
	links, err := linksOfPlan(ctx, views.Authored, plan)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return map[budgetrules.LineID]money.Cents{}, nil
	}
	pots, err := loadPots(ctx, views.Ledger, false)
	if err != nil {
		return nil, err
	}
	return budgetrules.PotBalances(links, pots.listedPots()), nil
}

// linksOfPlan is the pot each provision and reserve line of the plan is linked
// to now, by line id.
func linksOfPlan(ctx context.Context, chosen *authored.View, plan budgetrules.Plan) (map[budgetrules.LineID]string, error) {
	links := map[budgetrules.LineID]string{}
	for _, group := range plan {
		if group.Key != budgetrules.GroupProvisions && group.Key != budgetrules.GroupReserves {
			continue
		}
		for _, line := range group.Lines {
			if !line.ID.Set {
				continue
			}
			potID, linked, err := chosen.PotOfLine(ctx, authored.LineID(line.ID.ID))
			if err != nil {
				return nil, err
			}
			if linked {
				links[line.ID.ID] = potID
			}
		}
	}
	return links, nil
}

// listedPots are the pots the ledger lists now, as the rule of a counted link
// reads them.
func (view *potsView) listedPots() []budgetrules.ListedPot {
	listed := make([]budgetrules.ListedPot, 0, len(view.entries))
	for _, entry := range view.entries {
		listed = append(listed, budgetrules.ListedPot{ID: entry.id, Balance: entry.balance, Closed: entry.closed})
	}
	return listed
}

func budgetViewWire(month brday.Month, view budgetrules.BudgetView) budgetView {
	groups := make([]budgetGroupView, 0, len(view.Groups))
	for _, group := range view.Groups {
		groups = append(groups, budgetGroupWire(group))
	}
	return budgetView{
		Month: month, Closed: view.Closed, Income: Reais(view.Income), Allocated: Reais(view.Allocated),
		InvestmentRate: view.Rate, Groups: groups,
	}
}

func budgetGroupWire(group budgetrules.GroupView) budgetGroupView {
	derivedName, derived := derivedGroupNames[group.Key]
	wire := budgetGroupView{
		Key: string(group.Key), Name: derivedName, Subtotal: Reais(group.Subtotal), Over: group.Over,
		Lines: make([]budgetLineView, 0, len(group.Lines)),
	}
	if !derived {
		wire.Name = editableGroupName(string(group.Key))
	}
	if group.SubtotalSpent != nil {
		spent := Reais(*group.SubtotalSpent)
		wire.SubtotalSpent = &spent
	}
	for _, line := range group.Lines {
		name := line.Name
		if derived {
			name = derivedName
		}
		wire.Lines = append(wire.Lines, budgetLineView{
			Name: name, Cells: [3]any{cellWire(line.Cells[0]), cellWire(line.Cells[1]), cellWire(line.Cells[2])},
			Marks: marksWire(line.Marks), Over: line.Over, Bar: line.Bar, Expandable: line.Expandable,
		})
	}
	return wire
}

func editableGroupName(key string) string {
	for _, group := range editableGroupNames {
		if group.key == key {
			return group.name
		}
	}
	return ""
}

func cellWire(cell budgetrules.Cell) any {
	switch cell.Kind {
	case budgetrules.CellMoney:
		return moneyOf(Reais(cell.Amount))
	case budgetrules.CellDifference:
		return moneyCell{Type: "difference", Amount: Reais(cell.Amount)}
	case budgetrules.CellPercent:
		return percentCell{Type: "percent", Value: cell.Percent}
	case budgetrules.CellNextCharge:
		return nextChargeCell{Type: "next-charge"}
	}
	return none()
}

func marksWire(marks []budgetrules.Mark) []budgetMarkWire {
	wire := make([]budgetMarkWire, 0, len(marks))
	for _, mark := range marks {
		switch mark.Kind {
		case budgetrules.MarkFlag:
			wire = append(wire, budgetMarkWire{Text: mark.Flag, Tone: markNeutral})
		case budgetrules.MarkOverspent:
			wire = append(wire, budgetMarkWire{Text: budgetMarkOverspent, Tone: markBad})
		case budgetrules.MarkOverIncome:
			wire = append(wire, budgetMarkWire{Text: budgetMarkOverIncome, Tone: markBad})
		}
	}
	return wire
}

// The contract's BudgetMark tones.
const (
	markNeutral = "neutral"
	markBad     = "bad"
)
