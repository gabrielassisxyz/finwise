package authored_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

const acceptMethod = "review.acceptPending"

// changeOn records write as one change of method at noon of a Brazil day.
func changeOn(t *testing.T, database *authored.Database, method string, day brday.Day, write func(*authored.Recorder) error) authored.ChangeID {
	t.Helper()
	at, err := time.Parse(time.DateOnly, string(day))
	if err != nil {
		t.Fatal(err)
	}
	var id authored.ChangeID
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: method, At: at.Add(15 * time.Hour)}
	err = database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		if err := write(recorder); err != nil {
			return err
		}
		id, err = recorder.ID(t.Context())
		return err
	})
	if err != nil {
		t.Fatalf("%s on %s: %v", method, day, err)
	}
	return id
}

func settleOn(t *testing.T, database *authored.Database, method string, day brday.Day, line authored.LineID, keys ...authored.TxKey) authored.ChangeID {
	t.Helper()
	return changeOn(t, database, method, day, func(recorder *authored.Recorder) error {
		for _, key := range keys {
			if err := recorder.SettleLine(context.Background(), key, line, authored.AuthorPerson); err != nil {
				return err
			}
		}
		return nil
	})
}

func waitForReview(t *testing.T, database *authored.Database, keys ...authored.TxKey) {
	t.Helper()
	changeOn(t, database, "ingest.run", "2026-03-01", func(recorder *authored.Recorder) error {
		for _, key := range keys {
			if err := recorder.SetPendingReview(context.Background(), key, "Estabelecimento novo", nil, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

func indexedRows(t *testing.T, database *authored.Database, count int) []authored.TxKey {
	t.Helper()
	keys := make([]authored.TxKey, count)
	for i := range keys {
		keys[i] = indexed(t, database, seenRow("p-"+string(rune('a'+i)), "2026-03-01", -100, "name:a"))
	}
	return keys
}

func TestTheTransactionsSettledByAMethodAreThoseOfItsChangesInForceOnTheDaysOfAPeriod(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first, _ := twoLines(t, database)
	keys := indexedRows(t, database, 7)
	week := brday.Period{First: "2026-03-15", Last: "2026-03-21"}

	settleOn(t, database, acceptMethod, "2026-03-15", first, keys[0])
	settleOn(t, database, acceptMethod, "2026-03-21", first, keys[1], keys[2])
	settleOn(t, database, acceptMethod, "2026-03-14", first, keys[3])
	settleOn(t, database, acceptMethod, "2026-03-22", first, keys[4])
	undone := settleOn(t, database, acceptMethod, "2026-03-17", first, keys[5])
	if err := database.Undo(t.Context(), undone, time.Date(2026, 3, 18, 15, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	settleOn(t, database, "transactions.setLine", "2026-03-19", first, keys[6])
	// A second change of the method over a transaction it already settled.
	waitForReview(t, database, keys[0])
	settleOn(t, database, acceptMethod, "2026-03-18", first, keys[0])
	// A change of the method that wrote no fact of a transaction.
	changeOn(t, database, acceptMethod, "2026-03-16", func(recorder *authored.Recorder) error {
		return recorder.SetNickname(context.Background(), "name:a", "Apelido")
	})

	view := database.View()
	defer view.Close()
	got, err := view.TransactionsSettledBy(t.Context(), acceptMethod, week)
	if err != nil {
		t.Fatal(err)
	}
	if want := []authored.TxKey{keys[0], keys[1], keys[2]}; !slices.Equal(got, want) {
		t.Errorf("settled = %v, want %v", got, want)
	}
	if got, err := authored.EmptyView().TransactionsSettledBy(t.Context(), acceptMethod, week); err != nil || len(got) != 0 {
		t.Errorf("the empty view answered %v, %v", got, err)
	}
}

func TestThePendingTransactionsAreTheKeysAReviewWaitsFor(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first, _ := twoLines(t, database)
	keys := indexedRows(t, database, 3)
	waitForReview(t, database, keys[2], keys[0], keys[1])
	settleOn(t, database, acceptMethod, "2026-03-15", first, keys[1])

	view := database.View()
	defer view.Close()
	got, err := view.PendingTransactionKeys(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if want := []authored.TxKey{keys[0], keys[2]}; !slices.Equal(got, want) {
		t.Errorf("pending = %v, want %v", got, want)
	}
	if got, err := authored.EmptyView().PendingTransactionKeys(t.Context()); err != nil || len(got) != 0 {
		t.Errorf("the empty view answered %v, %v", got, err)
	}
}

// dismissOn runs the dismissal of ids as one change of the method on a day,
// and answers whether it left a change in the journal: one that recorded no
// effect leaves none.
func dismissOn(t *testing.T, scratch *authoredtest.Scratch, day brday.Day, ids ...string) (authored.ChangeID, bool) {
	t.Helper()
	at, err := time.Parse(time.DateOnly, string(day))
	if err != nil {
		t.Fatal(err)
	}
	before := len(scratch.Changes(t))
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "review.dismissWarning", At: at.Add(15 * time.Hour)}
	err = scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		for _, id := range ids {
			if err := recorder.DismissWarning(t.Context(), id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("dismiss %v: %v", ids, err)
	}
	changes := scratch.Changes(t)
	if len(changes) == before {
		return 0, false
	}
	return authored.ChangeID(changes[len(changes)-1].ID), true
}

func dismissedIn(t *testing.T, database *authored.Database) []string {
	t.Helper()
	view := database.View()
	defer view.Close()
	dismissed, err := view.DismissedWarnings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(dismissed))
	for id := range dismissed {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func TestADismissedWarningIsOneRowAndItsUndoTakesItBack(t *testing.T) {
	scratch := authoredtest.Open(t, clockAt(t, creationInstant))
	before := scratch.Canonical(t)
	change, recorded := dismissOn(t, scratch, "2026-03-15", "price:1:2026-03-10")
	if !recorded {
		t.Fatal("a dismissal recorded no change")
	}
	if got := dismissedIn(t, scratch.Database); !slices.Equal(got, []string{"price:1:2026-03-10"}) {
		t.Errorf("dismissed = %v, want the one warning", got)
	}
	if effects := scratch.EffectsOf(t, int64(change)); len(effects) != 1 || effects[0].Table != "dismissed_warnings" || effects[0].Before != "" {
		t.Errorf("effects = %+v, want one row appearing in dismissed_warnings", effects)
	}
	if err := scratch.Database.Undo(t.Context(), change, time.Date(2026, 3, 16, 15, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if got := dismissedIn(t, scratch.Database); len(got) != 0 {
		t.Errorf("after the undo %v is still dismissed", got)
	}
	if after := scratch.Canonical(t); after != before {
		t.Errorf("the undo left the journaled tables changed:\n%s", after)
	}
}

func TestDismissingAWarningAlreadyDismissedRecordsNothing(t *testing.T) {
	scratch := authoredtest.Open(t, clockAt(t, creationInstant))
	dismissOn(t, scratch, "2026-03-15", "charged-after-cancel:2:2026-03-11")
	effects := scratch.EffectCount(t)
	if _, recorded := dismissOn(t, scratch, "2026-03-16", "charged-after-cancel:2:2026-03-11"); recorded {
		t.Error("dismissing a warning already dismissed left a change")
	}
	if scratch.EffectCount(t) != effects {
		t.Error("dismissing a warning already dismissed recorded an effect")
	}
}

func TestEveryDismissedWarningIsReadBackWhateverItsKindAndTheEmptyViewHoldsNone(t *testing.T) {
	scratch := authoredtest.Open(t, clockAt(t, creationInstant))
	ids := []string{"charged-after-cancel:2:2026-03-11", "price:1:2026-03-10", "renewal:3:2026-04-20"}
	dismissOn(t, scratch, "2026-03-15", ids...)
	if got := dismissedIn(t, scratch.Database); !slices.Equal(got, ids) {
		t.Errorf("dismissed = %v, want %v", got, ids)
	}
	if got, err := authored.EmptyView().DismissedWarnings(t.Context()); err != nil || len(got) != 0 {
		t.Errorf("the empty view answered %v, %v", got, err)
	}
}
