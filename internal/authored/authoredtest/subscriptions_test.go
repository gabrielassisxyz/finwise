package authoredtest_test

import (
	"context"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

func TestTheSeededSubscriptionIsMadeFromTheSeededPurchaseAndHasNoLine(t *testing.T) {
	database := authoredtest.Fresh(t, func() time.Time { return seedClock })
	var id authored.SubscriptionID
	recorded(t, database, func(ctx context.Context, recorder *authored.Recorder) error {
		for _, seed := range []func(*authored.Recorder) error{authoredtest.SeedBudget, authoredtest.SeedTransactions} {
			if err := seed(recorder); err != nil {
				return err
			}
		}
		var err error
		id, err = authoredtest.SeedSubscription(recorder)
		return err
	})
	view := database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(t.Context(), []string{authoredtest.SeedPurchase})
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := view.Subscription(t.Context(), id)
	want := authored.Subscription{
		ID: id, EstablishmentKey: authoredtest.SeedEstablishment, CreatedFrom: keys[authoredtest.SeedPurchase],
		CreatedDay: "2026-02-10", CreatedCents: 12_000, Name: authoredtest.SeedSubscriptionName, Status: authored.StatusKeep,
		StatusDay: "2026-02-10", ReviewOn: "2026-05-10", Note: authoredtest.SeedSubscriptionNote,
	}
	if err != nil || !found || got != want || want.CreatedFrom == 0 {
		t.Errorf("seeded %+v (%t, %v), want %+v", got, found, err, want)
	}
}

func TestTheSubscriptionIsNotSeededWithoutThePurchase(t *testing.T) {
	database := authoredtest.Fresh(t, func() time.Time { return seedClock })
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.seed", At: seedClock}
	err := database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		_, err := authoredtest.SeedSubscription(recorder)
		return err
	})
	if err == nil {
		t.Error("a subscription was seeded from a purchase nobody indexed")
	}
}
