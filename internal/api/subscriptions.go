package api

import (
	"context"
	"encoding/json"
)

// A subscription is something the person confirms inside the app. Spotting
// repeating purchases in the ledger is not decided and belongs with that side,
// so the candidates and the recurring purchases are empty on purpose.

type subscriptionList struct {
	MonthlyTotal Reais `json:"monthlyTotal"`
	AnnualTotal  Reais `json:"annualTotal"`
	Groups       []any `json:"groups"`
	Suggestion   any   `json:"suggestion"`
}

type subscriptionListOptions struct {
	Grouped *bool `json:"grouped"`
}

func registerSubscriptions(registry *Registry) {
	registry.Register("subscriptions.list", readSubscriptionsList)
	registry.Register("subscriptions.detail", readSubscriptionDetail)
	registry.Register("subscriptions.candidates", readNothingListed)
	registry.Register("subscriptions.recurring", readNothingListed)
}

func readSubscriptionsList(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	if len(args) != 1 {
		return nil, &badArgumentsError{}
	}
	var options subscriptionListOptions
	if err := decodeArgument(args[0], &options); err != nil || options.Grouped == nil {
		return nil, &badArgumentsError{}
	}
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	return subscriptionList{Groups: []any{}}, nil
}

// readSubscriptionDetail answers null for any id: there is none to find.
func readSubscriptionDetail(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	if _, err := stringArgument(args); err != nil {
		return nil, err
	}
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	return nil, nil
}

func readNothingListed(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	return []any{}, nil
}
