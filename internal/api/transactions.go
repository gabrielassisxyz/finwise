package api

import (
	"context"
	"encoding/json"

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
	list := make([]transactionWire, 0, len(view.listed))
	for _, listed := range view.listed {
		list = append(list, transactionFrom(listed, view.rows[listed.ID], around))
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

// readTransactionFilterOptions offers every account. Tags and budget lines are
// the person's to create, so there are none yet.
func readTransactionFilterOptions(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
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
	return transactionFilterOptions{Accounts: options, Tags: []string{}, Lines: []budgetLineOption{}}, nil
}
