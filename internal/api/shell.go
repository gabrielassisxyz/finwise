package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// wholeLedger is the period that holds every row the clean list can have.
var wholeLedger = brday.Period{First: "1970-01-01", Last: "2999-12-31"}

type closedMonth struct {
	Month    string `json:"month"`
	ClosedOn string `json:"closedOn"`
}

type shellOverview struct {
	Today        string        `json:"today"`
	PendingCount int           `json:"pendingCount"`
	Sync         syncStatus    `json:"sync"`
	FirstMonth   string        `json:"firstMonth"`
	LastMonth    string        `json:"lastMonth"`
	ClosedMonths []closedMonth `json:"closedMonths"`
}

func registerShell(registry *Registry) {
	registry.Register("shell.overview", readShellOverview)
}

// readShellOverview answers what the ledger knows of the frame and how many
// transactions wait for review. Nobody has closed a month yet, so that list is
// the fresh install's.
func readShellOverview(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
	accounts, err := views.Ledger.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	state, err := views.Ledger.SyncState(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := views.Ledger.CleanTransactions(ctx, wholeLedger)
	if err != nil {
		return nil, err
	}
	pending, err := pendingCount(ctx, views)
	if err != nil {
		return nil, err
	}
	now := nowOf(ctx)
	current := string(brday.MonthOf(now))
	return shellOverview{
		Today:        string(brday.DayOf(now)),
		PendingCount: pending,
		Sync:         syncStatusOf(state, now, itemBankLabels(accounts, bankLabels(accounts))),
		FirstMonth:   firstMonthOf(rows, current),
		LastMonth:    current,
		ClosedMonths: []closedMonth{},
	}, nil
}

// firstMonthOf is the month of the oldest row, or the current month when there
// is none, and never later than the current month.
func firstMonthOf(rows []ledger.Transaction, current string) string {
	first := current
	for _, row := range rows {
		if month := string(brday.MonthOf(row.Date)); month < first {
			first = month
		}
	}
	return first
}
