package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// candidatesShown is how many purchases "Nova assinatura" offers, as the
// stand-in's CANDIDATES_SHOWN.
const candidatesShown = 12

type subscriptionCandidate struct {
	TransactionID string `json:"transactionId"`
	Name          string `json:"name"`
	Amount        Reais  `json:"amount"`
}

// eligiblePurchase is a purchase of the current month a subscription can be
// made from, with the name the transaction list shows for it.
type eligiblePurchase struct {
	listed txrules.Transaction
	name   string
}

// canBecomeSubscription is a card charge that is not a fee and was not
// excluded. Whether it is of the current month and whether its establishment
// already has a subscription are the caller's to check.
func canBecomeSubscription(listed txrules.Transaction, state authored.TransactionState) bool {
	return isSubscriptionCharge(listed) && !listed.Fee && !exclusionInEffect(state)
}

// subscriptionCandidatesOf offers one purchase per establishment that has
// no subscription: its newest eligible one, the lower id on the same day. The
// filter comes before the pick, so an excluded newest purchase leaves the next
// eligible one as its establishment's candidate. They come newest first, two
// of one day in the order the transaction list gives them.
func subscriptionCandidatesOf(ctx context.Context, views Views) ([]subscriptionCandidate, error) {
	eligible, err := eligiblePurchasesOfMonth(ctx, views, brday.MonthOf(nowOf(ctx)))
	if err != nil {
		return nil, err
	}
	chosen := map[string]txrules.Transaction{}
	for _, purchase := range eligible {
		held, found := chosen[purchase.listed.EstablishmentKey]
		if !found || newerCandidate(purchase.listed, held) {
			chosen[purchase.listed.EstablishmentKey] = purchase.listed
		}
	}
	candidates := []subscriptionCandidate{}
	for _, purchase := range eligible {
		if len(candidates) == candidatesShown {
			break
		}
		if chosen[purchase.listed.EstablishmentKey].ID != purchase.listed.ID {
			continue
		}
		candidates = append(candidates, subscriptionCandidate{
			TransactionID: purchase.listed.ID, Name: purchase.name, Amount: Reais(-purchase.listed.Amount),
		})
	}
	return candidates, nil
}

func newerCandidate(a, b txrules.Transaction) bool {
	if a.Day != b.Day {
		return a.Day > b.Day
	}
	return a.ID < b.ID
}

// eligiblePurchasesOfMonth is the month's purchases a subscription can be made
// from, in the transaction list's order, read as that list reads the month.
func eligiblePurchasesOfMonth(ctx context.Context, views Views, month brday.Month) ([]eligiblePurchase, error) {
	period, err := brday.MonthPeriod(month)
	if err != nil {
		return nil, err
	}
	around, err := loadContext(ctx, views.Ledger)
	if err != nil {
		return nil, err
	}
	view, err := interpretPeriod(ctx, views.Ledger, around, period)
	if err != nil {
		return nil, err
	}
	rows, err := readAuthoredRows(ctx, views, view.listed)
	if err != nil {
		return nil, err
	}
	subscribed := map[string]bool{}
	var eligible []eligiblePurchase
	for _, listed := range view.listed {
		kept, _, _ := rows.keptOf(listed)
		if !canBecomeSubscription(listed, kept.State) {
			continue
		}
		taken, err := establishmentSubscribed(ctx, views.Authored, subscribed, listed.EstablishmentKey)
		if err != nil {
			return nil, err
		}
		if !taken {
			eligible = append(eligible, eligiblePurchase{listed: listed, name: shownName(rows.establishments, listed.EstablishmentKey, listed.Name)})
		}
	}
	return eligible, nil
}

// establishmentSubscribed asks once per establishment whether it has a
// subscription, keeping the answers in seen.
func establishmentSubscribed(ctx context.Context, view *authored.View, seen map[string]bool, key string) (bool, error) {
	if taken, asked := seen[key]; asked {
		return taken, nil
	}
	_, taken, err := view.SubscriptionOfEstablishment(ctx, key)
	if err != nil {
		return false, err
	}
	seen[key] = taken
	return taken, nil
}
