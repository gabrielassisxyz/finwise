package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/subrules"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// A subscription keeps only what the person decided. Everything shown about
// its money comes from the card's charges of its establishment, read from the
// whole ledger in one pass of the request's snapshot, so a tax row is already
// folded into its purchase. The index is never the source of a figure.

// derivedSubscription is a subscription as a read shows it.
type derivedSubscription struct {
	stored  authored.Subscription
	derived subrules.Derivation
	// line is the subscription's line as the current month's budget names it.
	line *string
}

// isSubscriptionCharge is a listed transaction that charged a card: on a card, money
// out, not a movement, and in reais. A debit of the same establishment on a
// bank account is not a charge.
func isSubscriptionCharge(listed txrules.Transaction) bool {
	return listed.Kind == txrules.KindCard && listed.Amount < 0 && listed.Movement == txrules.NotAMovement && !listed.Unconverted
}

// exclusionInEffect is an exclusion that took effect: one switched on with no
// reason yet still counts.
func exclusionInEffect(state authored.TransactionState) bool {
	return exclusionOf(state.Exclusion) == budgetrules.ExcludedWithReason
}

// deriveSubscriptions works out each subscription from its charges, in the
// order given.
func deriveSubscriptions(ctx context.Context, views Views, stored []authored.Subscription) ([]derivedSubscription, error) {
	if len(stored) == 0 {
		return nil, nil
	}
	charges, err := chargeRowsOf(ctx, views, stored)
	if err != nil {
		return nil, err
	}
	lines, err := linesOfMonth(ctx, views.Authored, brday.MonthOf(nowOf(ctx)))
	if err != nil {
		return nil, err
	}
	today := brday.DayOf(nowOf(ctx))
	derived := make([]derivedSubscription, 0, len(stored))
	for _, subscription := range stored {
		derivation, err := subrules.Derive(subrules.Subscription{
			Rows: charges[subscription.EstablishmentKey], CreatedDay: subscription.CreatedDay, CreatedCents: subscription.CreatedCents,
			Cancelling: subscription.Status == authored.StatusCancel, StatusDay: subscription.StatusDay, Today: today,
		})
		if err != nil {
			return nil, err
		}
		derived = append(derived, derivedSubscription{stored: subscription, derived: derivation, line: subscriptionLineName(lines, subscription.Line)})
	}
	return derived, nil
}

// subscriptionLineName is a line's name in the month, or nil when the month's budget
// does not have it: a subscription's line belongs to no month, and is shown
// only while the current one has it.
func subscriptionLineName(lines monthLines, line *authored.LineID) *string {
	if line == nil {
		return nil
	}
	names := lines.namesOf([]authored.LineID{*line})
	if len(names) == 0 {
		return nil
	}
	return &names[0]
}

// chargeRowsOf is the card charges of each subscribed establishment, by key,
// as the rules take them.
func chargeRowsOf(ctx context.Context, views Views, stored []authored.Subscription) (map[string][]subrules.Row, error) {
	subscribed := map[string]bool{}
	for _, subscription := range stored {
		subscribed[subscription.EstablishmentKey] = true
	}
	whole, _, err := listedLedger(ctx, views.Ledger)
	if err != nil {
		return nil, err
	}
	var charged []txrules.Transaction
	for _, listed := range whole.listed {
		if subscribed[listed.EstablishmentKey] && isSubscriptionCharge(listed) {
			charged = append(charged, listed)
		}
	}
	authoredRows, err := readAuthoredRows(ctx, views, charged)
	if err != nil {
		return nil, err
	}
	rows := map[string][]subrules.Row{}
	for _, listed := range charged {
		if kept, _, _ := authoredRows.keptOf(listed); exclusionInEffect(kept.State) {
			continue
		}
		rows[listed.EstablishmentKey] = append(rows[listed.EstablishmentKey], subscriptionChargeRow(listed))
	}
	return rows, nil
}

func subscriptionChargeRow(listed txrules.Transaction) subrules.Row {
	row := subrules.Row{Day: listed.Day, Cents: -listed.Amount, Tax: listed.Tax}
	if row.Tax < 0 {
		row.Tax = -row.Tax
	}
	if listed.Original != nil {
		row.Foreign = &subrules.Foreign{Amount: listed.Original.Amount, Currency: listed.Original.Currency}
	}
	return row
}
