package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// The closing of a month is built on a budget, on what the person confirmed and
// on steps they tick, and the backend keeps none of that yet. It answers what a
// fresh install does: every step to do except the one that follows the count of
// what waits for review, and that count is zero.

const (
	stepPending = "pending"
	stepClose   = "close"
)

// closingStepOrder is the contract's ClosingStepId, in the order the screen
// walks them. A test holds it to the manifest.
var closingStepOrder = []string{
	stepPending, "planned-actual", "leftover", "contributions", "card-bill", "subscriptions", "report", stepClose,
}

// contributionGroupKeys are the budget groups compared with what was set aside
// and not with what was spent.
var contributionGroupKeys = map[string]bool{"prov": true, "res": true}

type closingStepWire struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Markable bool   `json:"markable"`
}

type plannedActualGroupWire struct {
	Name          string `json:"name"`
	Planned       Reais  `json:"planned"`
	Actual        Reais  `json:"actual"`
	Difference    Reais  `json:"difference"`
	Contributions bool   `json:"contributions"`
	Lines         []any  `json:"lines"`
}

type closingPurchaseWire struct {
	Name   string `json:"name"`
	Day    string `json:"day"`
	Amount Reais  `json:"amount"`
}

type closingCardBillWire struct {
	Amount    Reais                 `json:"amount"`
	CardLabel string                `json:"cardLabel"`
	Purchases []closingPurchaseWire `json:"purchases"`
}

type closingViewWire struct {
	Month                 brday.Month              `json:"month"`
	Closed                bool                     `json:"closed"`
	ClosedOn              *string                  `json:"closedOn"`
	Steps                 []closingStepWire        `json:"steps"`
	PendingCount          int                      `json:"pendingCount"`
	PlannedActual         []plannedActualGroupWire `json:"plannedActual"`
	Leftover              Reais                    `json:"leftover"`
	Contributions         []any                    `json:"contributions"`
	CardBill              *closingCardBillWire     `json:"cardBill"`
	SubscriptionsToReview []any                    `json:"subscriptionsToReview"`
	Report                []any                    `json:"report"`
	ClosedMonths          []any                    `json:"closedMonths"`
}

func registerClosing(registry *Registry) {
	registry.Register("review.closing", readClosing)
}

// readClosing reads the card even though the rest is fixed: the card's bill is
// the one thing the bank supports, and reading it is also what stops an
// unreadable ledger from being answered as a fresh one.
func readClosing(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	month, err := onlyMonthArgument(args)
	if err != nil {
		return nil, err
	}
	card, err := loadCardWithClosingSetting(ctx, views)
	if err != nil {
		return nil, err
	}
	return closingViewWire{
		Month:                 month,
		Steps:                 freshClosingSteps(),
		PlannedActual:         emptyPlannedActual(),
		Contributions:         []any{},
		CardBill:              closingCardBillOf(card, month),
		SubscriptionsToReview: []any{},
		Report:                []any{},
		ClosedMonths:          []any{},
	}, nil
}

// freshClosingSteps marks the first step done because nothing waits for review,
// and lets the person tick the steps in between by hand.
func freshClosingSteps() []closingStepWire {
	steps := make([]closingStepWire, 0, len(closingStepOrder))
	for _, id := range closingStepOrder {
		state := "todo"
		if id == stepPending {
			state = "done"
		}
		steps = append(steps, closingStepWire{ID: id, State: state, Markable: id != stepPending && id != stepClose})
	}
	return steps
}

// emptyPlannedActual is the budget's own places with nothing planned in them.
func emptyPlannedActual() []plannedActualGroupWire {
	groups := make([]plannedActualGroupWire, 0, len(editableGroupNames))
	for _, group := range editableGroupNames {
		groups = append(groups, plannedActualGroupWire{
			Name: group.name, Contributions: contributionGroupKeys[group.key], Lines: []any{},
		})
	}
	return groups
}

// closingCardBillOf is what the month's purchases charge on the bill after the
// closing that falls in the month, as the card page's pay-ahead works it out
// for the current month. A purchase is money out on the card page and an amount
// owed here. A card whose closing in the month is not known has no bill to
// show: a zero would read as nothing to set aside.
func closingCardBillOf(card *cardView, month brday.Month) *closingCardBillWire {
	if card == nil {
		return nil
	}
	next := card.nextBill(month)
	if !next.Known {
		return nil
	}
	payAhead := card.payAheadOf(next)
	bill := &closingCardBillWire{Amount: *payAhead.Amount, CardLabel: card.title(), Purchases: []closingPurchaseWire{}}
	for _, purchase := range payAhead.Purchases {
		bill.Purchases = append(bill.Purchases, closingPurchaseWire{Name: purchase.Name, Day: purchase.Day, Amount: -purchase.Amount})
	}
	return bill
}
