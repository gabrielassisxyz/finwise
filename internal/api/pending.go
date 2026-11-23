package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// pendingTransaction is a transaction that waits for the person's review, as
// the ledger lists it now, with what the app's database keeps about it.
type pendingTransaction struct {
	listedRow
	key   authored.TxKey
	state authored.TransactionState
}

// pendingTransactions is the one definition of what waits for review, which
// every queue and every count of it reads: a transaction with a review kept in
// the app's database, that the ledger lists now under one of the ids its key
// maps to, dated on or before last in a month whose budget has a line. They
// come newest first, in the order the transaction list gives them.
//
// The review is kept whatever happens to its row: one the bank stopped
// listing, or of a month whose budget lost every line, waits out of every
// count until it is listed, or the month has a line, again. Counting the kept
// reviews alone would let a row nobody can see block a month's close forever.
func pendingTransactions(ctx context.Context, views Views, last brday.Day) ([]pendingTransaction, error) {
	waiting, err := views.Authored.PendingTransactionKeys(ctx)
	if err != nil || len(waiting) == 0 {
		return nil, err
	}
	whole, around, err := listedLedger(ctx, views.Ledger)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(whole.listed))
	for _, listed := range whole.listed {
		ids = append(ids, listed.ID)
	}
	keys, err := views.Authored.TransactionKeys(ctx, ids)
	if err != nil {
		return nil, err
	}
	kept, err := views.Authored.Transactions(ctx, waiting)
	if err != nil {
		return nil, err
	}
	lined := monthsWithLines{view: views.Authored, known: map[brday.Month]bool{}}
	taken := map[authored.TxKey]bool{}
	var pending []pendingTransaction
	for _, listed := range whole.listed {
		key, indexed := keys[listed.ID]
		state := kept[key].State
		if !indexed || taken[key] || state.PendingReason == "" || listed.Day > last {
			continue
		}
		hasLine, err := lined.has(ctx, monthOfDay(listed.Day))
		if err != nil {
			return nil, err
		}
		if !hasLine {
			continue
		}
		taken[key] = true
		pending = append(pending, pendingTransaction{
			listedRow: listedRow{listed: listed, source: whole.rows[listed.ID], around: around}, key: key, state: state,
		})
	}
	return pending, nil
}

// monthsWithLines says whether a month's budget has a line, reading each
// month's once.
type monthsWithLines struct {
	view  *authored.View
	known map[brday.Month]bool
}

func (months monthsWithLines) has(ctx context.Context, month brday.Month) (bool, error) {
	if has, read := months.known[month]; read {
		return has, nil
	}
	lines, err := linesOfMonth(ctx, months.view, month)
	if err != nil {
		return false, err
	}
	months.known[month] = len(lines) > 0
	return len(lines) > 0, nil
}

// pendingCount is how many transactions wait for review in every month: the
// length of the weekly queue, so a badge never counts what the queue does not
// show.
func pendingCount(ctx context.Context, views Views) (int, error) {
	pending, err := pendingTransactions(ctx, views, wholeLedger.Last)
	return len(pending), err
}
