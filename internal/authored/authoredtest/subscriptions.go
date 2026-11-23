package authoredtest

import (
	"context"
	"errors"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// The invented subscription SeedSubscription writes. Nothing of it is real.
const (
	SeedSubscriptionName = "Mercado da esquina"
	SeedSubscriptionNote = "Cesta do mês, revisar o plano"
)

// SeedSubscription writes into recorder's change a subscription made from the
// posted purchase SeedTransactions writes, on that purchase's establishment,
// which no row of a fixture ledger carries: it shows the purchase it was made
// from, as stored, in place of charges. It is kept, with no line and a note,
// and is reviewed on 2026-05-10. SeedTransactions must have run before it in
// the change.
func SeedSubscription(recorder *authored.Recorder) (authored.SubscriptionID, error) {
	ctx := context.Background()
	view := recorder.View()
	defer view.Close()
	keys, err := view.TransactionKeys(ctx, []string{SeedPurchase})
	if err != nil {
		return 0, err
	}
	key, found := keys[SeedPurchase]
	if !found {
		return 0, errors.New("authoredtest: the subscription is seeded after SeedTransactions")
	}
	return recorder.CreateSubscription(ctx, authored.Subscription{
		EstablishmentKey: SeedEstablishment, CreatedFrom: key, CreatedDay: "2026-02-10", CreatedCents: 12_000,
		Name: SeedSubscriptionName, Status: authored.StatusKeep, StatusDay: "2026-02-10", ReviewOn: "2026-05-10",
		Note: SeedSubscriptionNote,
	})
}
