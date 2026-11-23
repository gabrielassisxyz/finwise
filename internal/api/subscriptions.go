package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/subrules"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// A subscription is something the person makes from a card purchase inside
// the app; what it costs and when it charges are worked out from the card's
// charges on every read. Spotting repeating purchases in the ledger needs
// thresholds measured on real data, so the suggestion and the recurring
// purchases are empty on purpose, and so are a subscription's alternatives,
// which are the model's sentences.

const (
	subscriptionToneWarning = "warning"
	subscriptionToneBad     = "bad"
)

type subscriptionMark struct {
	Text string `json:"text"`
	Tone string `json:"tone"`
}

type subscriptionRow struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Amount            Reais              `json:"amount"`
	Cycle             string             `json:"cycle"`
	NextCharge        string             `json:"nextCharge"`
	MonthlyEquivalent Reais              `json:"monthlyEquivalent"`
	Line              *string            `json:"line"`
	Status            string             `json:"status"`
	ReviewOn          string             `json:"reviewOn"`
	ReviewOverdue     bool               `json:"reviewOverdue"`
	Marks             []subscriptionMark `json:"marks"`
}

type subscriptionGroup struct {
	Status        *string           `json:"status"`
	MonthlyTotal  Reais             `json:"monthlyTotal"`
	Subscriptions []subscriptionRow `json:"subscriptions"`
}

type subscriptionList struct {
	MonthlyTotal Reais               `json:"monthlyTotal"`
	AnnualTotal  Reais               `json:"annualTotal"`
	Groups       []subscriptionGroup `json:"groups"`
	Suggestion   any                 `json:"suggestion"`
}

type subscriptionForeign struct {
	OriginalAmount string `json:"originalAmount"`
	Iof            Reais  `json:"iof"`
}

type subscriptionCharge struct {
	Day     string               `json:"day"`
	Amount  Reais                `json:"amount"`
	Foreign *subscriptionForeign `json:"foreign"`
}

type subscriptionDetail struct {
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	Cycle             string               `json:"cycle"`
	Amount            Reais                `json:"amount"`
	Marks             []subscriptionMark   `json:"marks"`
	NextCharge        string               `json:"nextCharge"`
	MonthlyEquivalent Reais                `json:"monthlyEquivalent"`
	AnnualEquivalent  Reais                `json:"annualEquivalent"`
	Line              *string              `json:"line"`
	Status            string               `json:"status"`
	ReviewOn          string               `json:"reviewOn"`
	Note              string               `json:"note"`
	Charges           []subscriptionCharge `json:"charges"`
	Alternatives      []string             `json:"alternatives"`
}

type subscriptionListOptions struct {
	Grouped *bool `json:"grouped"`
}

func registerSubscriptions(registry *Registry) {
	registry.Register("subscriptions.list", readSubscriptionsList)
	registry.Register("subscriptions.detail", readSubscriptionDetail)
	registry.Register("subscriptions.candidates", readSubscriptionCandidates)
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
	stored, err := views.Authored.Subscriptions(ctx)
	if err != nil {
		return nil, err
	}
	derived, err := deriveSubscriptions(ctx, views, stored)
	if err != nil {
		return nil, err
	}
	return subscriptionListOf(derived, *options.Grouped, brday.DayOf(nowOf(ctx))), nil
}

// subscriptionListOf is every subscription in the order they were created:
// one group of them all, or one group per status that has any, in the
// statuses' order. The totals add the equivalents as the rows show them.
func subscriptionListOf(derived []derivedSubscription, grouped bool, today brday.Day) subscriptionList {
	list := subscriptionList{Groups: []subscriptionGroup{}}
	list.MonthlyTotal, list.AnnualTotal = subscriptionTotalsOf(derived)
	if len(derived) == 0 {
		return list
	}
	if !grouped {
		list.Groups = append(list.Groups, subscriptionGroupOf(nil, derived, today))
		return list
	}
	for _, status := range authored.SubscriptionStatuses {
		var members []derivedSubscription
		for _, subscription := range derived {
			if subscription.stored.Status == status {
				members = append(members, subscription)
			}
		}
		if len(members) > 0 {
			name := string(status)
			list.Groups = append(list.Groups, subscriptionGroupOf(&name, members, today))
		}
	}
	return list
}

func subscriptionTotalsOf(derived []derivedSubscription) (month, year Reais) {
	monthly := make([]money.Cents, len(derived))
	for index, subscription := range derived {
		monthly[index] = subscription.derived.Monthly
	}
	monthTotal, yearTotal := subrules.Totals(monthly)
	return Reais(monthTotal), Reais(yearTotal)
}

func subscriptionGroupOf(status *string, members []derivedSubscription, today brday.Day) subscriptionGroup {
	group := subscriptionGroup{Status: status, Subscriptions: make([]subscriptionRow, 0, len(members))}
	group.MonthlyTotal, _ = subscriptionTotalsOf(members)
	for _, member := range members {
		group.Subscriptions = append(group.Subscriptions, subscriptionRowOf(member, today))
	}
	return group
}

func subscriptionRowOf(subscription derivedSubscription, today brday.Day) subscriptionRow {
	stored, derived := subscription.stored, subscription.derived
	return subscriptionRow{
		ID: stored.ID.String(), Name: stored.Name, Amount: Reais(derived.Amount), Cycle: subscriptionCycleName(derived.Cycle),
		NextCharge: string(derived.NextCharge), MonthlyEquivalent: Reais(derived.Monthly), Line: subscription.line,
		Status: string(stored.Status), ReviewOn: string(stored.ReviewOn), ReviewOverdue: stored.ReviewOn <= today,
		Marks: subscriptionMarksOf(derived.Marks),
	}
}

// readSubscriptionDetail answers null for an id the database does not have,
// which is any id that is not the plain decimal text of a subscription's.
func readSubscriptionDetail(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	text, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	id, ok := authored.ParseSubscriptionID(text)
	if !ok {
		return nil, nil
	}
	stored, found, err := views.Authored.Subscription(ctx, id)
	if err != nil || !found {
		return nil, err
	}
	derived, err := deriveSubscriptions(ctx, views, []authored.Subscription{stored})
	if err != nil {
		return nil, err
	}
	return subscriptionDetailOf(derived[0]), nil
}

func subscriptionDetailOf(subscription derivedSubscription) subscriptionDetail {
	stored, derived := subscription.stored, subscription.derived
	detail := subscriptionDetail{
		ID: stored.ID.String(), Name: stored.Name, Cycle: subscriptionCycleName(derived.Cycle), Amount: Reais(derived.Amount),
		Marks: subscriptionMarksOf(derived.Marks), NextCharge: string(derived.NextCharge), MonthlyEquivalent: Reais(derived.Monthly),
		AnnualEquivalent: Reais(derived.Yearly), Line: subscription.line, Status: string(stored.Status),
		ReviewOn: string(stored.ReviewOn), Note: stored.Note, Charges: make([]subscriptionCharge, 0, len(derived.History)),
		Alternatives: []string{},
	}
	for _, charge := range derived.History {
		detail.Charges = append(detail.Charges, subscriptionChargeOf(charge))
	}
	return detail
}

func subscriptionChargeOf(charge subrules.Charge) subscriptionCharge {
	wire := subscriptionCharge{Day: string(charge.Day), Amount: Reais(charge.Cents)}
	if charge.Foreign != nil {
		original := originalAmountOf(&txrules.Original{Amount: charge.Foreign.Amount, Currency: charge.Foreign.Currency})
		wire.Foreign = &subscriptionForeign{OriginalAmount: *original, Iof: Reais(charge.Tax)}
	}
	return wire
}

func readSubscriptionCandidates(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
	return subscriptionCandidatesOf(ctx, views)
}

var subscriptionCycleNames = map[subrules.Cycle]string{
	subrules.Monthly: cycleMonthly, subrules.Quarterly: cycleQuarterly, subrules.Semiannual: cycleSemiannual,
	subrules.Yearly: cycleYearly, subrules.Biennial: cycleBiennial,
}

func subscriptionCycleName(cycle subrules.Cycle) string { return subscriptionCycleNames[cycle] }

// subscriptionMarksOf writes the marks that hold in the stand-in's order.
func subscriptionMarksOf(marks subrules.Marks) []subscriptionMark {
	written := []subscriptionMark{}
	if marks.PriceRose {
		written = append(written, subscriptionMark{Text: markPriceRose, Tone: subscriptionToneWarning})
	}
	if marks.Renews {
		written = append(written, subscriptionMark{Text: renewalMark(marks.RenewsIn), Tone: subscriptionToneWarning})
	}
	if marks.ChargedAfterCancel {
		written = append(written, subscriptionMark{Text: markChargedAfterCancel, Tone: subscriptionToneBad})
	}
	return written
}

func readNothingListed(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
	if err := requireReadableLedger(ctx, views.Ledger); err != nil {
		return nil, err
	}
	return []any{}, nil
}
