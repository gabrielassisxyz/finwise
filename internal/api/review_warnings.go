package api

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/subrules"
)

// A warning is never stored: every read of the weekly review works it out from
// the subscriptions the person keeps and the card's charges, through the marks
// the subscription rules already derive, so a warning holds exactly when its
// mark is on the subscription's row. What the person stores is the id of one
// they dismissed.
//
// Nothing detects a new recurring purchase or two plans of one service yet, so
// no warning offers an action.

// reviewWarning is the contract's ReviewWarning.
type reviewWarning struct {
	ID             string `json:"id"`
	Text           string `json:"text"`
	SubscriptionID string `json:"subscriptionId"`
	Actions        []any  `json:"actions"`
}

func registerReviewWarnings(registry *Registry) {
	registry.RegisterWrite("review.dismissWarning", dismissWarning)
	registry.RegisterWrite("review.runWarningAction", answerNothingToRun)
}

// warningKinds are the kinds in the order the review lists them.
var warningKinds = []func(derivedSubscription) (reviewWarning, bool, error){
	priceWarning, renewalWarning, chargedAfterCancelWarning,
}

// warningsHolding are the warnings that hold now, by kind and within a kind by
// ascending subscription id, which is the order the store reads them in.
func warningsHolding(ctx context.Context, views Views) ([]reviewWarning, error) {
	stored, err := views.Authored.Subscriptions(ctx)
	if err != nil {
		return nil, err
	}
	derived, err := deriveSubscriptions(ctx, views, stored)
	if err != nil {
		return nil, err
	}
	return warningsOf(derived)
}

func warningsOf(derived []derivedSubscription) ([]reviewWarning, error) {
	warnings := []reviewWarning{}
	for _, kind := range warningKinds {
		for _, subscription := range derived {
			warning, holds, err := kind(subscription)
			if err != nil {
				return nil, err
			}
			if holds {
				warnings = append(warnings, warning)
			}
		}
	}
	return warnings, nil
}

// shownWarnings are the warnings that hold now and were not dismissed.
func shownWarnings(ctx context.Context, views Views) ([]reviewWarning, error) {
	holding, err := warningsHolding(ctx, views)
	if err != nil || len(holding) == 0 {
		return holding, err
	}
	dismissed, err := views.Authored.DismissedWarnings(ctx)
	if err != nil {
		return nil, err
	}
	shown := make([]reviewWarning, 0, len(holding))
	for _, warning := range holding {
		if !dismissed[warning.ID] {
			shown = append(shown, warning)
		}
	}
	return shown, nil
}

// warningIDOf joins a warning's kind, its subscription and what triggered it.
// The trigger is a day, never the days left of a countdown, so a renewal keeps
// its id until it passes and a price that rises again is a new warning.
func warningIDOf(kind string, subscription derivedSubscription, trigger string) string {
	return kind + ":" + subscription.stored.ID.String() + ":" + trigger
}

func warningOf(kind string, subscription derivedSubscription, trigger, text string) reviewWarning {
	return reviewWarning{
		ID: warningIDOf(kind, subscription, trigger), Text: text,
		SubscriptionID: subscription.stored.ID.String(), Actions: []any{},
	}
}

// latestChargeDay is the day of the charge the amount is read from. Whenever a
// mark that triggers on a charge holds, the rules found that charge in the
// ledger, so it is never the stored creation purchase.
func latestChargeDay(subscription derivedSubscription) string {
	return string(subscription.derived.History[0].Day)
}

func priceWarning(subscription derivedSubscription) (reviewWarning, bool, error) {
	derived := subscription.derived
	if !derived.Marks.PriceRose {
		return reviewWarning{}, false, nil
	}
	text := priceRoseText(subscription.stored.Name, derived.Previous, derived.Amount)
	return warningOf(warningKindPrice, subscription, latestChargeDay(subscription), text), true, nil
}

var renewalWords = map[subrules.Cycle]string{subrules.Yearly: renewalWordYearly, subrules.Biennial: renewalWordBiennial}

func renewalWarning(subscription derivedSubscription) (reviewWarning, bool, error) {
	derived := subscription.derived
	if !derived.Marks.Renews {
		return reviewWarning{}, false, nil
	}
	word, known := renewalWords[derived.Cycle]
	if !known {
		return reviewWarning{}, false, fmt.Errorf("api: a renewal of cycle %d has no word", derived.Cycle)
	}
	text := renewalText(word, derived.Marks.RenewsIn, subscription.stored.Name, derived.Amount)
	return warningOf(warningKindRenewal, subscription, string(derived.NextCharge), text), true, nil
}

func chargedAfterCancelWarning(subscription derivedSubscription) (reviewWarning, bool, error) {
	if !subscription.derived.Marks.ChargedAfterCancel {
		return reviewWarning{}, false, nil
	}
	text := chargedAfterCancelText(subscription.stored.Name)
	return warningOf(warningKindChargedAfterCancel, subscription, latestChargeDay(subscription), text), true, nil
}

// dismissWarning stores the id of a warning that holds and has not been
// dismissed, as one change. Anything else, an id no warning has included, is
// answered null and records nothing. It reads the ledger to know what holds,
// so with the snapshot missing it is unavailable like the review itself.
func dismissWarning(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	if err := requireReadableLedger(ctx, writes.Ledger); err != nil {
		return nil, err
	}
	shown, err := shownWarnings(ctx, Views{Ledger: writes.Ledger, Authored: writes.Authored})
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(shown, func(warning reviewWarning) bool { return warning.ID == id }) {
		return nil, nil
	}
	if err := writes.Recorder.DismissWarning(ctx, id); err != nil {
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// answerNothingToRun answers null and records nothing: no warning offers an
// action until something detects what one would act on.
func answerNothingToRun(context.Context, []json.RawMessage, Writes) (any, error) {
	return nil, nil
}
