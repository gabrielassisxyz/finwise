package api

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/subrules"
)

// The closing's three writes: ticking a step of a month, and keeping or
// cancelling a subscription whose review has come. None of them reads the
// ledger, so each answers the same while the snapshot is missing. A
// subscription is decided through the store functions its own screen uses.

const (
	markStepDoneMethod       = "review.markStepDone"
	keepSubscriptionMethod   = "review.keepSubscription"
	cancelSubscriptionMethod = "review.cancelSubscription"
)

// keepReviewMonths is how long after the end of the month it was reviewed in
// a kept subscription is looked at again.
const keepReviewMonths = 6

func registerClosingWrites(registry *Registry) {
	registry.RegisterWrite(markStepDoneMethod, markClosingStepDone)
	registry.RegisterWrite(keepSubscriptionMethod, keepSubscriptionUnderReview)
	registry.RegisterWrite(cancelSubscriptionMethod, cancelSubscriptionUnderReview)
}

type keptSubscriptionWire struct {
	ChangeID   string `json:"changeId"`
	Name       string `json:"name"`
	NextReview string `json:"nextReview"`
}

type cancelledSubscriptionWire struct {
	ChangeID string `json:"changeId"`
	Name     string `json:"name"`
}

// markClosingStepDone ticks one of steps two to seven of an open month. The
// first and last steps follow from the data and a closed month is done
// throughout, so those answer null and record nothing. A step already ticked
// answers a change that holds nothing, as two tabs ticking it would: the
// second finds the row there. No rule refuses a future month or one before any
// budget.
func markClosingStepDone(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	month, err := monthArgument(args[0])
	if err != nil {
		return nil, err
	}
	step, err := textArgument(args[1])
	if err != nil {
		return nil, err
	}
	if !slices.Contains(closingStepOrder, step) {
		return nil, &badArgumentsError{}
	}
	closedMonths, err := writes.Authored.ClosedMonths(ctx)
	if err != nil {
		return nil, err
	}
	if _, closed := closedOnOf(closedMonths, month); closed || step == stepPending || step == stepClose {
		return nil, nil
	}
	if err := writes.Recorder.MarkClosingStepDone(ctx, month, step); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// keepSubscriptionUnderReview keeps a subscription and puts its next review
// six months after the end of the month it was reviewed in. One that is
// already kept with that review day has nothing to change and answers null.
// A closed month does not refuse it: closing freezes a budget, not the
// subscriptions.
func keepSubscriptionUnderReview(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	month, err := monthArgument(args[0])
	if err != nil {
		return nil, err
	}
	stored, found, err := subscriptionUnderReview(ctx, writes, args[1])
	if err != nil || !found {
		return nil, err
	}
	next, err := nextReviewAfter(month)
	if err != nil {
		return nil, err
	}
	if stored.Status == authored.StatusKeep && stored.ReviewOn == next {
		return nil, nil
	}
	if err := writes.Recorder.SetSubscriptionStatus(ctx, stored.ID, authored.StatusKeep, brday.DayOf(nowOf(ctx))); err != nil {
		return nil, err
	}
	if err := writes.Recorder.SetSubscriptionReviewDay(ctx, stored.ID, next); err != nil {
		return nil, err
	}
	change, err := changeResultOf(ctx, writes.Recorder)
	if err != nil {
		return nil, err
	}
	return keptSubscriptionWire{ChangeID: change.ChangeID, Name: stored.Name, NextReview: string(next)}, nil
}

// cancelSubscriptionUnderReview marks a subscription to cancel; one already
// marked answers null.
func cancelSubscriptionUnderReview(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 1 {
		return nil, &badArgumentsError{}
	}
	stored, found, err := subscriptionUnderReview(ctx, writes, args[0])
	if err != nil || !found || stored.Status == authored.StatusCancel {
		return nil, err
	}
	if err := writes.Recorder.SetSubscriptionStatus(ctx, stored.ID, authored.StatusCancel, brday.DayOf(nowOf(ctx))); err != nil {
		return nil, err
	}
	change, err := changeResultOf(ctx, writes.Recorder)
	if err != nil {
		return nil, err
	}
	return cancelledSubscriptionWire{ChangeID: change.ChangeID, Name: stored.Name}, nil
}

// subscriptionUnderReview is the subscription raw names. Unlike the
// subscription screen's setters, whose results cannot be null, an id the
// database does not have is not refused: these two results allow null, and
// nothing is there to keep or cancel.
func subscriptionUnderReview(ctx context.Context, writes Writes, raw json.RawMessage) (authored.Subscription, bool, error) {
	text, err := textArgument(raw)
	if err != nil {
		return authored.Subscription{}, false, err
	}
	id, ok := authored.ParseSubscriptionID(text)
	if !ok {
		return authored.Subscription{}, false, nil
	}
	return writes.Authored.Subscription(ctx, id)
}

// nextReviewAfter is the last day of month moved six months on, its day
// number held to the end of a shorter month.
func nextReviewAfter(month brday.Month) (brday.Day, error) {
	period, err := brday.MonthPeriod(month)
	if err != nil {
		return "", err
	}
	return subrules.AddMonths(period.Last, keepReviewMonths)
}
