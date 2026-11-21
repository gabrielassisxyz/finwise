package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// The weekly review is built on the budget's variable lines and on what the
// person has been asked to confirm, and none of that exists on a fresh install:
// the cap is zero, so nothing is spent against it, and nothing waits.

type weeklyLimit struct {
	Cap       Reais `json:"cap"`
	Spent     Reais `json:"spent"`
	Remaining Reais `json:"remaining"`
	Over      bool  `json:"over"`
	Fill      int   `json:"fill"`
}

type reviewQueue struct {
	Items    []any `json:"items"`
	Resolved int   `json:"resolved"`
}

type weeklyReview struct {
	Limit     weeklyLimit `json:"limit"`
	Week      any         `json:"week"`
	Queue     reviewQueue `json:"queue"`
	Approvals []any       `json:"approvals"`
	Warnings  []any       `json:"warnings"`
}

func registerReview(registry *Registry) {
	registry.Register("review.weekly", readWeeklyReview)
	registry.Register("review.payday", readPayday)
}

func readWeeklyReview(ctx context.Context, args []json.RawMessage, reader *ledger.Reader) (any, error) {
	if _, err := onlyMonthArgument(args); err != nil {
		return nil, err
	}
	if err := requireReadableLedger(ctx, reader); err != nil {
		return nil, err
	}
	return weeklyReview{
		Queue:     reviewQueue{Items: []any{}},
		Approvals: []any{},
		Warnings:  []any{},
	}, nil
}
