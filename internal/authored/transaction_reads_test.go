package authored_test

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// readsOf asks a view every read the transactions' tables offer, about keys and
// ids that a database may or may not hold, and counts what came back.
func readsOf(t *testing.T, view *authored.View) map[string]int {
	t.Helper()
	ctx := t.Context()
	keys, err := view.TransactionKeys(ctx, []string{"p-1", "p-2"})
	if err != nil {
		t.Fatal(err)
	}
	transactions, err := view.Transactions(ctx, []authored.TxKey{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	establishments, err := view.Establishments(ctx, []string{"name:a", "name:b"})
	if err != nil {
		t.Fatal(err)
	}
	tags, err := view.KnownTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, linked, err := view.PurchaseLinkedTo(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, dated, err := view.TransactionDay(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{
		"keys": len(keys), "transactions": len(transactions), "establishments": len(establishments), "tags": len(tags),
	}
	if linked {
		counts["linked"] = 1
	}
	if dated {
		counts["dated"] = 1
	}
	return counts
}

// heldState writes one of everything the reads answer about, in one change.
func heldState(t *testing.T, database *authored.Database) {
	t.Helper()
	change(t, database, func(recorder *authored.Recorder) error {
		ctx := context.Background()
		purchase, err := recorder.IndexTransaction(ctx, seenRow("p-1", "2026-11-28", -100, "name:a"))
		if err != nil {
			return err
		}
		credit, err := recorder.IndexTransaction(ctx, seenRow("p-2", "2026-11-29", 100, "name:b"))
		if err != nil {
			return err
		}
		return all(
			func(r *authored.Recorder) error {
				return r.Set(ctx, "tx_state", key1(integer(int64(purchase))), authored.Row{"credit_tx_key": integer(int64(credit))})
			},
			put("tx_tags", authored.Row{"tx_key": integer(int64(purchase)), "tag": text("#a"), "position": integer(0)}),
			put("establishment_tags", authored.Row{"key": text("name:b"), "tag": text("#b"), "position": integer(0)}),
		)(recorder)
	})
}

func TestEveryTransactionReadAnswersNothingWhereNothingIsKept(t *testing.T) {
	empty := authored.EmptyView()
	defer empty.Close()
	fresh := openDatabase(t, databasePath(t), creationInstant, nil).View()
	defer fresh.Close()
	for name, view := range map[string]*authored.View{"the empty view": empty, "a fresh database": fresh} {
		if got := readsOf(t, view); got["keys"]+got["transactions"]+got["establishments"]+got["tags"]+got["linked"]+got["dated"] != 0 {
			t.Errorf("%s answered %v, want nothing", name, got)
		}
	}

	database := openDatabase(t, databasePath(t), creationInstant, nil)
	heldState(t, database)
	held := database.View()
	defer held.Close()
	want := map[string]int{"keys": 2, "transactions": 1, "establishments": 1, "tags": 2, "linked": 1, "dated": 1}
	if got := readsOf(t, held); len(got) != len(want) {
		t.Errorf("a database that holds one of everything answered %v, want %v", got, want)
	} else {
		for name, count := range want {
			if got[name] != count {
				t.Errorf("%s: %d, want %d", name, got[name], count)
			}
		}
	}
}

func TestTheReadsOfAChangeSeeWhatItWroteBeforeItCommits(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	change(t, database, func(recorder *authored.Recorder) error {
		ctx := context.Background()
		key, err := recorder.IndexTransaction(ctx, seenRow("p-1", "2026-11-28", -100, "name:a"))
		if err != nil {
			return err
		}
		if _, err := recorder.AddHistoryStep(ctx, key, authored.StepNote, "Nota"); err != nil {
			return err
		}
		view := recorder.View()
		defer view.Close()
		keys, err := view.TransactionKeys(ctx, []string{"p-1"})
		if err != nil || keys["p-1"] != key {
			t.Errorf("the change's view answered %v (%v), want p-1 at %d", keys, err, key)
		}
		day, found, err := view.TransactionDay(ctx, key)
		if err != nil || !found || day != "2026-11-28" {
			t.Errorf("the change's view dates the transaction %q %v (%v)", day, found, err)
		}
		kept, err := view.Transactions(ctx, []authored.TxKey{key})
		if err != nil || len(kept[key].History) != 1 {
			t.Errorf("the change's view holds %v (%v), want its one step", kept, err)
		}
		return nil
	})
}
