package authored_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

var subscriptionsStart = time.Date(2026, time.November, 22, 12, 0, 0, 0, time.UTC)

// streamingPurchase is an invented card purchase a subscription is made from.
var streamingPurchase = seenRow("tx-stream", "2026-11-20", -6_400, "text:STREAMSVC")

func newSubscription(key authored.TxKey) authored.Subscription {
	return authored.Subscription{
		EstablishmentKey: "text:STREAMSVC", CreatedFrom: key, CreatedDay: "2026-11-20", CreatedCents: 6_400,
		Name: "STREAMSVC", Status: authored.StatusReview, StatusDay: "2026-11-22", ReviewOn: "2027-02-22",
	}
}

// createSubscription indexes the purchase and makes the subscription in one
// change, and answers the subscription's id and the change's.
func createSubscription(t *testing.T, database *authored.Database, at time.Time) (authored.SubscriptionID, authored.ChangeID) {
	t.Helper()
	var subscription authored.SubscriptionID
	change := recordPots(t, database, at, func(ctx context.Context, recorder *authored.Recorder) error {
		key, err := recorder.IndexTransaction(ctx, streamingPurchase)
		if err != nil {
			return err
		}
		subscription, err = recorder.CreateSubscription(ctx, newSubscription(key))
		return err
	})
	return subscription, change
}

func readSubscription(t *testing.T, database *authored.Database, id authored.SubscriptionID) authored.Subscription {
	t.Helper()
	subscription, found, err := nowView(t, database).Subscription(t.Context(), id)
	if err != nil || !found {
		t.Fatalf("subscription %s: found %t, %v", id, found, err)
	}
	return subscription
}

func setStatus(id authored.SubscriptionID, status authored.SubscriptionStatus, day brday.Day) func(context.Context, *authored.Recorder) error {
	return func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.SetSubscriptionStatus(ctx, id, status, day)
	}
}

func TestASubscriptionIsReadBackAndUndoingItsCreationRestoresTheDumpAndFreesItsEstablishment(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	empty := canonicalOf(t, database)
	id, change := createSubscription(t, database, subscriptionsStart)

	want := newSubscription(1)
	want.ID = 1
	if got := readSubscription(t, database, id); got != want {
		t.Errorf("read back %+v, want %+v", got, want)
	}
	all, err := nowView(t, database).Subscriptions(t.Context())
	if err != nil || len(all) != 1 || all[0] != want {
		t.Errorf("every subscription: %+v (%v)", all, err)
	}
	if found, taken, err := nowView(t, database).SubscriptionOfEstablishment(t.Context(), "text:STREAMSVC"); err != nil || !taken || found != id {
		t.Errorf("the establishment's subscription: %s, %t, %v", found, taken, err)
	}
	effects := effectsOf(t, database, change)
	if len(effects) != 1 || effects[0].table != "subscriptions" || !effects[0].columnNull || effects[0].before.Valid {
		t.Errorf("the creation recorded %+v, want one effect creating the row", effects)
	}

	if err := undoAt(t, database, change, 10); err != nil {
		t.Fatal(err)
	}
	if got := canonicalOf(t, database); got != empty {
		t.Errorf("the dump after the undo is not the empty one:\n%s", got)
	}
	if _, taken, err := nowView(t, database).SubscriptionOfEstablishment(t.Context(), "text:STREAMSVC"); err != nil || taken {
		t.Errorf("after the undo the establishment still has a subscription (%v)", err)
	}
	again, _ := createSubscription(t, database, subscriptionsStart.Add(time.Hour))
	if again == id {
		t.Errorf("the subscription made again got the undone one's id %s", id)
	}
}

func TestSubscriptionsAreReadInTheOrderTheyWereCreated(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	keys := []string{"text:STREAMSVC", "doc:12345678000199", "name:academia"}
	recordPots(t, database, subscriptionsStart, func(ctx context.Context, recorder *authored.Recorder) error {
		key, err := recorder.IndexTransaction(ctx, streamingPurchase)
		if err != nil {
			return err
		}
		for _, establishment := range keys {
			subscription := newSubscription(key)
			subscription.EstablishmentKey = establishment
			if _, err := recorder.CreateSubscription(ctx, subscription); err != nil {
				return err
			}
		}
		return nil
	})
	all, err := nowView(t, database).Subscriptions(t.Context())
	if err != nil || len(all) != 3 {
		t.Fatalf("every subscription: %+v (%v)", all, err)
	}
	for index, subscription := range all {
		if subscription.ID != authored.SubscriptionID(index+1) || subscription.EstablishmentKey != keys[index] {
			t.Errorf("subscription %d is %s of %s", index, subscription.ID, subscription.EstablishmentKey)
		}
	}
}

func TestTheSameStatusRecordsNothingAndKeepsItsDayWhileAnotherMovesTheDay(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	id, _ := createSubscription(t, database, subscriptionsStart)

	same := recordPots(t, database, subscriptionsStart.Add(24*time.Hour), setStatus(id, authored.StatusReview, "2026-11-23"))
	if effects := effectsOf(t, database, same); len(effects) != 0 {
		t.Errorf("the status it had recorded %+v", effects)
	}
	if got := readSubscription(t, database, id); got.StatusDay != "2026-11-22" {
		t.Errorf("the status day moved to %s for the status it had", got.StatusDay)
	}

	cancelled := recordPots(t, database, subscriptionsStart.Add(48*time.Hour), setStatus(id, authored.StatusCancel, "2026-11-24"))
	effects := effectsOf(t, database, cancelled)
	if len(effects) != 2 || effects[0].column != "status" || effects[1].column != "status_day" {
		t.Errorf("another status recorded %+v, want the status and its day", effects)
	}
	if got := readSubscription(t, database, id); got.Status != authored.StatusCancel || got.StatusDay != "2026-11-24" {
		t.Errorf("after the change: %s since %s", got.Status, got.StatusDay)
	}

	recordPots(t, database, subscriptionsStart.Add(72*time.Hour), setStatus(id, authored.StatusCancel, "2026-11-25"))
	if got := readSubscription(t, database, id); got.StatusDay != "2026-11-24" {
		t.Errorf("setting Cancelar again moved its day to %s", got.StatusDay)
	}
}

func TestEachSetterRecordsOneColumnAndNothingForTheValueItHas(t *testing.T) {
	database := potsDatabase(t)
	id, _ := createSubscription(t, database, subscriptionsStart)
	setters := []struct {
		column string
		write  func(context.Context, *authored.Recorder) error
	}{
		{"line_id", func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.SetSubscriptionLine(ctx, id, 3)
		}},
		{"review_on", func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.SetSubscriptionReviewDay(ctx, id, "2027-01-05")
		}},
		{"note", func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.SetSubscriptionNote(ctx, id, "  dividida com a casa ")
		}},
	}
	for index, setter := range setters {
		at := subscriptionsStart.Add(time.Duration(index+1) * time.Hour)
		effects := effectsOf(t, database, recordPots(t, database, at, setter.write))
		if len(effects) != 1 || effects[0].column != setter.column {
			t.Errorf("%s: recorded %+v", setter.column, effects)
		}
		if again := effectsOf(t, database, recordPots(t, database, at.Add(time.Minute), setter.write)); len(again) != 0 {
			t.Errorf("%s set to the value it has recorded %+v", setter.column, again)
		}
	}
	got := readSubscription(t, database, id)
	if got.Line == nil || *got.Line != 3 || got.ReviewOn != "2027-01-05" || got.Note != "  dividida com a casa " {
		t.Errorf("after the setters: line %v, review %s, note %q", got.Line, got.ReviewOn, got.Note)
	}
}

func TestAWriteAboutASubscriptionTheDatabaseDoesNotHaveWritesNothing(t *testing.T) {
	database := potsDatabase(t)
	createSubscription(t, database, subscriptionsStart)
	before := canonicalOf(t, database)
	effects := effectCount(t, database)
	writes := map[string]func(context.Context, *authored.Recorder) error{
		"status": setStatus(99, authored.StatusKeep, "2026-11-23"),
		"line": func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.SetSubscriptionLine(ctx, 99, 2)
		},
		"review": func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.SetSubscriptionReviewDay(ctx, 99, "2027-01-05")
		},
		"note": func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.SetSubscriptionNote(ctx, 99, "x")
		},
	}
	for name, write := range writes {
		err := changePots(t.Context(), database, subscriptionsStart, func(recorder *authored.Recorder) error {
			return write(t.Context(), recorder)
		})
		if !errors.Is(err, authored.ErrNoSuchSubscription) {
			t.Errorf("%s of subscription 99: %v, want ErrNoSuchSubscription", name, err)
		}
	}
	if canonicalOf(t, database) != before || effectCount(t, database) != effects {
		t.Error("a write about a missing subscription changed the database")
	}
}

func TestUndoingACreationIsRefusedWhileALaterStatusStandsAndSucceedsOnceItIsUndone(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	empty := canonicalOf(t, database)
	id, created := createSubscription(t, database, subscriptionsStart)
	kept := recordPots(t, database, subscriptionsStart.Add(time.Hour), setStatus(id, authored.StatusKeep, "2026-11-22"))
	beforeUndo := canonicalOf(t, database)

	requireRefusal(t, undoAt(t, database, created, 10), authored.CauseLaterChange)
	if canonicalOf(t, database) != beforeUndo {
		t.Error("the refused undo changed the database")
	}
	for step, id := range []authored.ChangeID{kept, created} {
		if err := undoAt(t, database, id, 20+step); err != nil {
			t.Fatalf("undo %d: %v", step, err)
		}
	}
	if got := canonicalOf(t, database); got != empty {
		t.Error("undoing both changes did not restore the empty dump")
	}
}

func TestTheEmptyViewHasNoSubscription(t *testing.T) {
	view := authored.EmptyView()
	all, err := view.Subscriptions(t.Context())
	if err != nil || len(all) != 0 {
		t.Errorf("every subscription: %+v (%v)", all, err)
	}
	if _, found, err := view.Subscription(t.Context(), 1); err != nil || found {
		t.Errorf("subscription 1: %t (%v)", found, err)
	}
	if _, found, err := view.SubscriptionOfEstablishment(t.Context(), "text:STREAMSVC"); err != nil || found {
		t.Errorf("the establishment's: %t (%v)", found, err)
	}
}

func TestASubscriptionIDIsTheDecimalTextOfAPositiveNumberOnly(t *testing.T) {
	for _, text := range []string{"1", "42"} {
		if id, ok := authored.ParseSubscriptionID(text); !ok || id.String() != text {
			t.Errorf("%q read as %s (%t)", text, id, ok)
		}
	}
	for _, text := range []string{"", "0", "-1", "07", "+7", "sub-1", "1.0", " 1", "999999999999999999999"} {
		if id, ok := authored.ParseSubscriptionID(text); ok {
			t.Errorf("%q read as the id %s", text, id)
		}
	}
}
