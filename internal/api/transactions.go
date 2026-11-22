package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

type filterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type budgetLineOption struct {
	Name  string `json:"name"`
	Group string `json:"group"`
}

type transactionFilterOptions struct {
	Accounts []filterOption     `json:"accounts"`
	Tags     []string           `json:"tags"`
	Lines    []budgetLineOption `json:"lines"`
}

func registerTransactions(registry *Registry) {
	registry.Register("transactions.list", readTransactionsList)
	registry.Register("transactions.filterOptions", readTransactionFilterOptions)
	registry.Register("transactions.detail", readTransactionDetail)
}

func readTransactionsList(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	period, err := queryPeriod(args)
	if err != nil {
		return nil, err
	}
	around, err := loadContext(ctx, views.Ledger)
	if err != nil {
		return nil, err
	}
	view, err := interpretPeriod(ctx, views.Ledger, around, period)
	if err != nil {
		return nil, err
	}
	rows, err := readAuthoredRows(ctx, views.Authored, view.listed)
	if err != nil {
		return nil, err
	}
	list := make([]transactionWire, 0, len(view.listed))
	for _, listed := range view.listed {
		list = append(list, rows.transactionOf(listed, view.rows[listed.ID], around))
	}
	return list, nil
}

func queryPeriod(args []json.RawMessage) (brday.Period, error) {
	if len(args) != 1 {
		return brday.Period{}, &badArgumentsError{}
	}
	var query transactionQuery
	if err := decodeArgument(args[0], &query); err != nil {
		return brday.Period{}, err
	}
	return periodOf(query)
}

// readTransactionFilterOptions offers every account, every tag in use and the
// lines of the budget in force for the month asked.
func readTransactionFilterOptions(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	month, err := onlyMonthArgument(args)
	if err != nil {
		return nil, err
	}
	accounts, err := views.Ledger.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	banks := bankLabels(accounts)
	labels := accountLabelsOf(accounts, banks)
	options := make([]filterOption, 0, len(accounts))
	for _, account := range orderedAccounts(accounts, banks) {
		options = append(options, filterOption{Value: account.ID, Label: labels[account.ID]})
	}
	budget, err := views.Authored.BudgetOf(ctx, month)
	if err != nil {
		return nil, err
	}
	tags, err := knownTags(ctx, views.Authored)
	if err != nil {
		return nil, err
	}
	return transactionFilterOptions{Accounts: options, Tags: tags, Lines: lineMenuOf(budget)}, nil
}

// lineMenuGroups is the order of a line menu, group by group; the fees line
// sits among them as a group of its own.
var lineMenuGroups = []string{"var", "fix", "res", "prov", feesMenuGroup, "rec"}

const feesMenuGroup = "fees"

// lineMenuOf offers the lines of a month's budget, and the fees line once that
// budget has any: a budget with no line offers nothing.
func lineMenuOf(budget authored.Budget) []budgetLineOption {
	menu := []budgetLineOption{}
	if !budget.HasLines() {
		return menu
	}
	for _, key := range lineMenuGroups {
		if key == feesMenuGroup {
			menu = append(menu, budgetLineOption{Name: authored.FeesLineName, Group: budgetGroupFees})
			continue
		}
		for _, line := range budget.Lines {
			if line.Group == key {
				menu = append(menu, budgetLineOption{Name: line.Name, Group: budgetGroupNamed(key)})
			}
		}
	}
	return menu
}

func budgetGroupNamed(key string) string {
	for _, group := range editableGroupNames {
		if group.key == key {
			return group.name
		}
	}
	return key
}
