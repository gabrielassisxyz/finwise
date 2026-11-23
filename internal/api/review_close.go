package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

const closeMonthMethod = "review.closeMonth"

func registerMonthClose(registry *Registry) {
	registry.RegisterWrite(closeMonthMethod, closeMonth)
}

// closeMonth closes a month that has come and of which, as of any earlier
// month, nothing waits for review, by the same definition the closing's first
// step counts with: a purchase of a later month never stops it. The close keeps
// the month's budget as a version of its own and touches none of its
// transactions, which stay editable.
//
// The first two refusals read no ledger, so a closed month answers
// month_closed with the snapshot gone. The third asks the snapshot even when
// the database keeps no review, so a missing one is never taken for an empty
// queue.
func closeMonth(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	month, err := onlyMonthArgument(args)
	if err != nil {
		return nil, err
	}
	period, err := brday.MonthPeriod(month)
	if err != nil {
		return nil, err
	}
	closedMonths, err := writes.Authored.ClosedMonths(ctx)
	if err != nil {
		return nil, err
	}
	if _, closed := closedOnOf(closedMonths, month); closed {
		return nil, refuse(reasonMonthClosed)
	}
	now := nowOf(ctx)
	if month > brday.MonthOf(now) {
		return nil, refuse(reasonFutureMonth)
	}
	if err := requireReadableLedger(ctx, writes.Ledger); err != nil {
		return nil, err
	}
	pending, err := pendingTransactions(ctx, Views{Ledger: writes.Ledger, Authored: writes.Authored}, period.Last)
	if err != nil {
		return nil, err
	}
	if len(pending) > 0 {
		return nil, refuse(reasonPendingReviews)
	}
	if err := writes.Recorder.CloseMonth(ctx, month, brday.DayOf(now)); err != nil {
		return nil, budgetRefusal(err)
	}
	return changeResultOf(ctx, writes.Recorder)
}
