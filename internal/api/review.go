package api

import (
	"context"
	"encoding/json"
)

// The weekly review is built on the budget's variable lines and on what the
// person has been asked to confirm. The limit and the week are those of the
// month asked, as the home shows them; the queue is the account's now.

type weeklyLimit struct {
	Cap       Reais `json:"cap"`
	Spent     Reais `json:"spent"`
	Remaining Reais `json:"remaining"`
	Over      bool  `json:"over"`
	Fill      int   `json:"fill"`
}

type reviewQueue struct {
	Items    []reviewQueueItem `json:"items"`
	Resolved int               `json:"resolved"`
}

type weeklyReview struct {
	Limit     weeklyLimit     `json:"limit"`
	Week      any             `json:"week"`
	Queue     reviewQueue     `json:"queue"`
	Approvals []any           `json:"approvals"`
	Warnings  []reviewWarning `json:"warnings"`
}

func registerReview(registry *Registry) {
	registry.Register("review.weekly", readWeeklyReview)
	registry.Register("review.payday", readPayday)
	registerReviewWarnings(registry)
	registerReviewApprovals(registry)
}

func readWeeklyReview(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	month, err := onlyMonthArgument(args)
	if err != nil {
		return nil, err
	}
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	queue, err := readQueue(ctx, views)
	if err != nil {
		return nil, err
	}
	limit, err := monthLimitOf(ctx, views, month)
	if err != nil {
		return nil, err
	}
	warnings, err := shownWarnings(ctx, views)
	if err != nil {
		return nil, err
	}
	return weeklyReview{
		Limit:     limit.weeklyLimitWire(),
		Week:      limit.currentWeekWire(),
		Queue:     queue,
		Approvals: []any{},
		Warnings:  warnings,
	}, nil
}
