package api

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/subrules"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// The person's subscriptions: one is made from a card purchase of the current
// month, then given a status, a line, a day to review it and a note. Each
// write is one change through the subscriptions' store. Only create reads the
// ledger; the setters answer the same while the snapshot is missing.

func registerSubscriptionWrites(registry *Registry) {
	registry.RegisterWrite("subscriptions.create", createSubscription)
	registry.RegisterWrite("subscriptions.acceptSuggestion", answerNoSuggestion)
	registry.RegisterWrite("subscriptions.rejectSuggestion", answerNoSuggestion)
	registry.RegisterWrite("subscriptions.setStatus", setSubscriptionStatus)
	registry.RegisterWrite("subscriptions.setLine", setSubscriptionLine)
	registry.RegisterWrite("subscriptions.setReviewDate", setSubscriptionReviewDate)
	registry.RegisterWrite("subscriptions.setNote", setSubscriptionNote)
}

type createdSubscription struct {
	ChangeID string `json:"changeId"`
	Name     string `json:"name"`
}

// createSubscription makes a subscription from any purchase a candidate could
// be, shown or not, under review from today, with the purchase's own line and
// the name the candidates show for it, which it keeps. Anything else answers
// null and records nothing; so does a second create for one establishment,
// which finds it taken because writes queue on the database's lock.
func createSubscription(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	id, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	purchase, found, err := purchaseToSubscribe(ctx, writes, id)
	if err != nil || !found {
		return nil, err
	}
	key, err := indexListed(ctx, writes.Recorder, purchase.listed)
	if err != nil {
		return nil, err
	}
	today := brday.DayOf(nowOf(ctx))
	reviewOn, err := subrules.FirstReview(today)
	if err != nil {
		return nil, err
	}
	_, err = writes.Recorder.CreateSubscription(ctx, authored.Subscription{
		EstablishmentKey: purchase.listed.EstablishmentKey, CreatedFrom: key, CreatedDay: purchase.listed.Day,
		CreatedCents: -purchase.listed.Amount, Name: purchase.name, Line: purchase.line,
		Status: authored.StatusReview, StatusDay: today, ReviewOn: reviewOn,
	})
	if err != nil {
		return nil, err
	}
	change, err := writes.Recorder.ID(ctx)
	if err != nil {
		return nil, err
	}
	return createdSubscription{ChangeID: change.String(), Name: purchase.name}, nil
}

// purchaseToKeep is a purchase a subscription can be made from: its line is
// the one stored for it, never a suggestion waiting for review.
type purchaseToKeep struct {
	listed txrules.Transaction
	name   string
	line   *authored.LineID
}

// purchaseToSubscribe finds the purchase id names as the detail would, and
// says whether a subscription can be made from it: a candidate's conditions,
// whatever the twelve shown are. It writes nothing.
func purchaseToSubscribe(ctx context.Context, writes Writes, id string) (purchaseToKeep, bool, error) {
	row, found, err := findListed(ctx, writes.Ledger, id)
	if err != nil || !found || monthOfDay(row.listed.Day) != brday.MonthOf(nowOf(ctx)) {
		return purchaseToKeep{}, false, err
	}
	rows, err := readAuthoredRows(ctx, writes.Authored, []txrules.Transaction{row.listed})
	if err != nil {
		return purchaseToKeep{}, false, err
	}
	kept, _, _ := rows.keptOf(row.listed)
	if !canBecomeSubscription(row.listed, kept.State) {
		return purchaseToKeep{}, false, nil
	}
	_, taken, err := writes.Authored.SubscriptionOfEstablishment(ctx, row.listed.EstablishmentKey)
	if err != nil || taken {
		return purchaseToKeep{}, false, err
	}
	name := shownName(rows.establishments, row.listed.EstablishmentKey, row.listed.Name)
	return purchaseToKeep{listed: row.listed, name: name, line: kept.State.Line}, true, nil
}

// answerNoSuggestion answers null and records nothing: nothing suggests a
// subscription until something detects one.
func answerNoSuggestion(context.Context, []json.RawMessage, Writes) (any, error) {
	return nil, nil
}

// setSubscriptionStatus moves the status day to today only when the status
// really changes; the store decides that, for every caller.
func setSubscriptionStatus(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	status, err := subscriptionStatusArgument(args[1])
	if err != nil {
		return nil, err
	}
	return changeSubscription(ctx, writes, args[0], func(id authored.SubscriptionID) error {
		return writes.Recorder.SetSubscriptionStatus(ctx, id, status, brday.DayOf(nowOf(ctx)))
	})
}

// setSubscriptionLine names the line in the current month's budget, the fees
// line under its fixed name included: a subscription has no month of its own.
func setSubscriptionLine(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	name, err := textArgument(args[1])
	if err != nil {
		return nil, err
	}
	return changeSubscription(ctx, writes, args[0], func(id authored.SubscriptionID) error {
		lines, err := linesOfMonth(ctx, writes.Authored, brday.MonthOf(nowOf(ctx)))
		if err != nil {
			return err
		}
		line, found := lines.idOf(name)
		if !found {
			return refuse(reasonUnknownLine)
		}
		return writes.Recorder.SetSubscriptionLine(ctx, id, line)
	})
}

func setSubscriptionReviewDate(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	text, err := textArgument(args[1])
	if err != nil {
		return nil, err
	}
	day, err := brday.ParseDay(text)
	if err != nil {
		return nil, &badArgumentsError{}
	}
	return changeSubscription(ctx, writes, args[0], func(id authored.SubscriptionID) error {
		return writes.Recorder.SetSubscriptionReviewDay(ctx, id, day)
	})
}

func setSubscriptionNote(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	note, err := textArgument(args[1])
	if err != nil {
		return nil, err
	}
	return changeSubscription(ctx, writes, args[0], func(id authored.SubscriptionID) error {
		return writes.Recorder.SetSubscriptionNote(ctx, id, note)
	})
}

// changeSubscription runs a setter on the subscription raw names and answers
// the change. An id the database does not have is refused before anything is
// written, whatever else the setter would have refused.
func changeSubscription(ctx context.Context, writes Writes, raw json.RawMessage, write func(authored.SubscriptionID) error) (any, error) {
	text, err := textArgument(raw)
	if err != nil {
		return nil, err
	}
	id, ok := authored.ParseSubscriptionID(text)
	if !ok {
		return nil, refuse(reasonNotFound)
	}
	if _, found, err := writes.Authored.Subscription(ctx, id); err != nil || !found {
		if err == nil {
			err = refuse(reasonNotFound)
		}
		return nil, err
	}
	if err := write(id); err != nil {
		if errors.Is(err, authored.ErrNoSuchSubscription) {
			return nil, refuse(reasonNotFound)
		}
		return nil, err
	}
	return changeResultOf(ctx, writes.Recorder)
}

// subscriptionStatusArgument holds the status to the contract's four, as the
// router's schema does, so the store is never handed another.
func subscriptionStatusArgument(raw json.RawMessage) (authored.SubscriptionStatus, error) {
	text, err := textArgument(raw)
	if err != nil {
		return "", err
	}
	status := authored.SubscriptionStatus(text)
	if !slices.Contains(authored.SubscriptionStatuses, status) {
		return "", &badArgumentsError{}
	}
	return status, nil
}
