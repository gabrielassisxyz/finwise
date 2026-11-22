package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// What the budget screen draws on the first day comes from the groups the API
// returns, so an empty budget is still its places: the five groups a person
// fills in and the three the screen derives. Every figure that comes from the
// budget is zero, and the one the bank's records support, the fees, is read.

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
	Name       string `json:"name"`
	Cells      [3]any `json:"cells"`
	Marks      []any  `json:"marks"`
	Over       bool   `json:"over"`
	Bar        any    `json:"bar"`
	Expandable bool   `json:"expandable"`
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
	Value any    `json:"value"`
}

func none() noneCell                 { return noneCell{Type: "none"} }
func moneyOf(amount Reais) moneyCell { return moneyCell{Type: "money", Amount: amount} }
func unknownPercent() percentCell    { return percentCell{Type: "percent"} }

func emptyBudgetDocument() []budgetGroupDoc {
	groups := make([]budgetGroupDoc, 0, len(editableGroupNames))
	for _, group := range editableGroupNames {
		groups = append(groups, budgetGroupDoc{Key: group.key, Name: group.name, Lines: []any{}})
	}
	return groups
}

// emptyBudgetGroups is the editable groups with nothing in them, then the three
// derived ones. Only fixed and variable compare their lines with what was spent.
func emptyBudgetGroups(fees Reais) []budgetGroupView {
	groups := make([]budgetGroupView, 0, len(editableGroupNames)+3)
	for _, group := range editableGroupNames {
		view := budgetGroupView{Key: group.key, Name: group.name, Lines: []budgetLineView{}}
		if group.key == groupFixed || group.key == groupVariable {
			spent := Reais(0)
			view.SubtotalSpent = &spent
		}
		groups = append(groups, view)
	}
	return append(groups,
		budgetGroupView{Key: "rf", Name: budgetGroupRainyFunds, Lines: []budgetLineView{}},
		feesGroup(fees),
		investmentGroup(),
	)
}

func feesGroup(fees Reais) budgetGroupView {
	line := budgetLineView{
		Name: budgetGroupFees, Cells: [3]any{moneyOf(fees), none(), none()},
		Marks: []any{}, Expandable: true,
	}
	return budgetGroupView{Key: "tar", Name: budgetGroupFees, Subtotal: fees, Lines: []budgetLineView{line}}
}

func investmentGroup() budgetGroupView {
	line := budgetLineView{
		Name: budgetGroupInvestment, Cells: [3]any{moneyOf(0), moneyOf(0), unknownPercent()}, Marks: []any{},
	}
	return budgetGroupView{Key: "inv", Name: budgetGroupInvestment, Lines: []budgetLineView{line}}
}

// feesOfMonth reads the month the way the transaction list does, widened and
// then cut, so a fee is paired and placed exactly as the list shows it.
func feesOfMonth(ctx context.Context, snapshot *ledger.View, month brday.Month) (Reais, error) {
	period, err := brday.MonthPeriod(month)
	if err != nil {
		return 0, &badArgumentsError{}
	}
	around, err := loadContext(ctx, snapshot)
	if err != nil {
		return 0, err
	}
	view, err := interpretPeriod(ctx, snapshot, around, period)
	if err != nil {
		return 0, err
	}
	return Reais(spentOnFees(view.listed)), nil
}

// spentOnFees is what went out as fees and interest, net of the credits that
// discount them. A pending row has not been charged, a movement is not
// spending, and a foreign row with no value in reais has no amount to add.
func spentOnFees(listed []txrules.Transaction) money.Cents {
	var spent money.Cents
	for _, row := range listed {
		if row.Fee && row.Posted && row.Movement == txrules.NotAMovement && !row.Unconverted {
			spent -= row.Amount
		}
	}
	return spent
}
