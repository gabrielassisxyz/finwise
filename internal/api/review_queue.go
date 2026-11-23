package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// acceptPendingMethod is the method whose changes the queue's counter counts.
const acceptPendingMethod = "review.acceptPending"

// reviewQueueItem is the contract's ReviewQueueItem: a pending transaction as
// the list and the panel show it.
type reviewQueueItem struct {
	TransactionID  string         `json:"transactionId"`
	Day            string         `json:"day"`
	Name           string         `json:"name"`
	RawDescription string         `json:"rawDescription"`
	Amount         Reais          `json:"amount"`
	AccountLabel   string         `json:"accountLabel"`
	Person         bool           `json:"person"`
	Pending        *pendingReview `json:"pending"`
}

// readQueue is every transaction that waits for review, whatever month the
// screen shows, and how many the person answered through the queue this week.
func readQueue(ctx context.Context, views Views) (reviewQueue, error) {
	pending, err := pendingTransactions(ctx, views, wholeLedger.Last)
	if err != nil {
		return reviewQueue{}, err
	}
	listed := make([]txrules.Transaction, 0, len(pending))
	for _, transaction := range pending {
		listed = append(listed, transaction.listed)
	}
	rows, err := readAuthoredRows(ctx, views, listed)
	if err != nil {
		return reviewQueue{}, err
	}
	items := make([]reviewQueueItem, 0, len(pending))
	for _, transaction := range pending {
		wire := rows.transactionOf(transaction.listed, transaction.source, transaction.around)
		items = append(items, reviewQueueItem{
			TransactionID: wire.ID, Day: wire.Day, Name: wire.Name, RawDescription: wire.RawDescription, Amount: wire.Amount,
			AccountLabel: describeAccount(transaction.listed.AccountID, transaction.source, transaction.around),
			Person:       wire.Person, Pending: wire.Pending,
		})
	}
	resolved, err := answeredThisWeek(ctx, views.Authored)
	if err != nil {
		return reviewQueue{}, err
	}
	return reviewQueue{Items: items, Resolved: resolved}, nil
}

// answeredThisWeek counts, from the journal, the transactions an accept that
// was not undone settled in the week of the current month that holds today.
// It counts transactions and not changes, so one answered twice counts once
// and the counter's total never grows past the reviews there were; and one
// waiting for review again counts nothing, whatever its changes, so taking an
// answer back from the panel lowers it as an undo does.
func answeredThisWeek(ctx context.Context, view *authored.View) (int, error) {
	week, err := brday.WeekOf(brday.DayOf(nowOf(ctx)))
	if err != nil {
		return 0, err
	}
	keys, err := view.TransactionsSettledBy(ctx, acceptPendingMethod, week)
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	kept, err := view.Transactions(ctx, keys)
	if err != nil {
		return 0, err
	}
	answered := 0
	for _, key := range keys {
		if kept[key].State.PendingReason == "" {
			answered++
		}
	}
	return answered, nil
}
