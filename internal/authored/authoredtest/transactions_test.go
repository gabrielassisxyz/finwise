package authoredtest_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

var seedClock = time.Date(2026, time.March, 20, 15, 0, 0, 0, time.UTC)

// recorded records write as one change made by the person and answers it.
func recorded(t *testing.T, database *authored.Database, write func(ctx context.Context, recorder *authored.Recorder) error) authored.ChangeID {
	t.Helper()
	var id authored.ChangeID
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "transactions.write", At: seedClock}
	err := database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		if err := write(t.Context(), recorder); err != nil {
			return err
		}
		var err error
		id, err = recorder.ID(t.Context())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// withLines is a database whose budget has the invented lines, and the first
// two of them.
func withLines(t *testing.T) (*authored.Database, authored.BudgetLine, authored.BudgetLine) {
	t.Helper()
	database := authoredtest.Fresh(t, func() time.Time { return seedClock })
	authoredtest.SaveBudget(t, database, seedClock, "2026-02", authoredtest.InventedBudget())
	view := database.View()
	defer view.Close()
	budget, err := view.BudgetOf(t.Context(), "2026-02")
	if err != nil || len(budget.Lines) < 2 {
		t.Fatalf("budget %v (%v)", budget, err)
	}
	return database, budget.Lines[0], budget.Lines[1]
}

func index(t *testing.T, database *authored.Database, providerID string) authored.TxKey {
	t.Helper()
	var key authored.TxKey
	recorded(t, database, func(ctx context.Context, recorder *authored.Recorder) error {
		var err error
		key, err = recorder.IndexTransaction(ctx, authoredtest.Seen(providerID, "2026-02-10", 1_000, "name:loja"))
		return err
	})
	return key
}

func transactionOf(t *testing.T, database *authored.Database, key authored.TxKey) authored.AuthoredTransaction {
	t.Helper()
	view := database.View()
	defer view.Close()
	found, err := view.Transactions(t.Context(), []authored.TxKey{key})
	if err != nil {
		t.Fatal(err)
	}
	return found[key]
}

func TestTheReadsAnswerWhatTheSeedWrote(t *testing.T) {
	database := authoredtest.Fresh(t, func() time.Time { return seedClock })
	id := recorded(t, database, func(_ context.Context, recorder *authored.Recorder) error {
		if err := authoredtest.SeedBudget(recorder); err != nil {
			return err
		}
		return authoredtest.SeedTransactions(recorder)
	})
	ctx := t.Context()
	view := database.View()
	defer view.Close()
	budget, err := view.BudgetOf(ctx, "2026-02")
	if err != nil {
		t.Fatal(err)
	}
	first, second := budget.Lines[0].ID, budget.Lines[1].ID

	ids := []string{
		authoredtest.SeedPendingPurchase, authoredtest.SeedPurchase, authoredtest.SeedRuled, authoredtest.SeedSuggested,
		authoredtest.SeedUnsuggested, authoredtest.SeedSplit, authoredtest.SeedExcluded, authoredtest.SeedSwitchedOff,
		authoredtest.SeedRefund, authoredtest.SeedCredit, authoredtest.SeedReimbursable, "never-seen",
	}
	keys, err := view.TransactionKeys(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	wantKeys := map[string]authored.TxKey{
		authoredtest.SeedPendingPurchase: 1, authoredtest.SeedPurchase: 1, authoredtest.SeedRuled: 2, authoredtest.SeedSuggested: 3,
		authoredtest.SeedUnsuggested: 4, authoredtest.SeedSplit: 5, authoredtest.SeedExcluded: 6, authoredtest.SeedSwitchedOff: 7,
		authoredtest.SeedRefund: 8, authoredtest.SeedCredit: 9, authoredtest.SeedReimbursable: 10,
	}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Errorf("keys %v, want %v", keys, wantKeys)
	}

	got, err := view.Transactions(ctx, []authored.TxKey{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11})
	if err != nil {
		t.Fatal(err)
	}
	day := "2026-03-20"
	want := map[authored.TxKey]authored.AuthoredTransaction{
		1: {
			State: authored.TransactionState{Line: &first, ClassifiedBy: authored.AuthorPerson, Note: "Compra do mês"},
			Tags:  []authored.Tag{{Name: "#casa", Position: 0}, {Name: "#mensal", Position: 1}},
			History: []authored.HistoryStep{{ID: 1, Change: id, Kind: authored.StepEffect, Text: "Você colocou em " + budget.Lines[0].Name,
				Day: brday.Day(day), Reason: "É compra de casa"}},
		},
		2: {
			State: authored.TransactionState{Line: &second, ClassifiedBy: authored.AuthorRule},
			History: []authored.HistoryStep{{ID: 2, Change: id, Kind: authored.StepEffect, Text: "Regra colocou em " + budget.Lines[1].Name,
				Day: brday.Day(day)}},
		},
		3: {State: authored.TransactionState{PendingReason: "Confiança abaixo do limite", PendingLine: &first, PendingConfidence: authoredtest.Some[int64](70)}},
		4: {State: authored.TransactionState{PendingReason: "Estabelecimento novo"}},
		5: {Split: []authored.SplitPart{{Position: 0, Line: &first, Cents: 5_000}, {Position: 1, Cents: 3_000}}},
		6: {State: authored.TransactionState{Exclusion: authoredtest.Some("Transferência entre contas")}},
		7: {State: authored.TransactionState{Exclusion: authoredtest.Some("")}},
		8: {State: authored.TransactionState{Refund: authoredtest.Some(true), RefundLine: &second}},
		10: {
			State: authored.TransactionState{Reimbursable: true, ExpectedCents: authoredtest.Some[money.Cents](4_000),
				Credit: authoredtest.Some[authored.TxKey](9), CreditCents: authoredtest.Some[money.Cents](4_000)},
			History: []authored.HistoryStep{{ID: 3, Change: id, Kind: authored.StepNote, Text: "O banco confirmou a compra", Day: brday.Day(day)}},
		},
	}
	for key := range want {
		if !reflect.DeepEqual(got[key], want[key]) {
			t.Errorf("transaction %d is %+v, want %+v", key, got[key], want[key])
		}
	}
	if len(got) != len(want) {
		t.Errorf("%d transactions hold something, want %d", len(got), len(want))
	}

	establishments, err := view.Establishments(ctx, []string{authoredtest.SeedEstablishment, "name:outra"})
	if err != nil {
		t.Fatal(err)
	}
	wantEstablishments := map[string]authored.Establishment{authoredtest.SeedEstablishment: {
		Nickname: "Mercado da esquina", DefaultLine: &first,
		FixedTags: []authored.Tag{{Name: "#mercado", Position: 0}, {Name: "#essencial", Position: 1}},
	}}
	if !reflect.DeepEqual(establishments, wantEstablishments) {
		t.Errorf("establishments %+v, want %+v", establishments, wantEstablishments)
	}
	tags, err := view.KnownTags(ctx)
	if err != nil || !slices.Equal(tags, []string{"#casa", "#essencial", "#mensal", "#mercado"}) {
		t.Errorf("known tags %v (%v)", tags, err)
	}
	purchase, linked, err := view.PurchaseLinkedTo(ctx, 9)
	if err != nil || !linked || purchase != 10 {
		t.Errorf("the credit's purchase is %d %v (%v), want 10", purchase, linked, err)
	}
	if day, found, err := view.TransactionDay(ctx, 1); err != nil || !found || day != "2026-02-10" {
		t.Errorf("the purchase's day is %q %v (%v), want the posted row's", day, found, err)
	}
}

func TestOwnAndFixedTagsKeepTheirPlacesAcrossARemovalAndItsUndo(t *testing.T) {
	database, _, _ := withLines(t)
	key := index(t, database, "tx-tags")
	kinds := map[string]struct {
		add, remove func(context.Context, *authored.Recorder, string) error
		read        func() []authored.Tag
	}{
		"own": {
			add: func(ctx context.Context, r *authored.Recorder, tag string) error {
				return authoredtest.AddTag(ctx, r, key, tag)
			},
			remove: func(ctx context.Context, r *authored.Recorder, tag string) error {
				return authoredtest.RemoveTag(ctx, r, key, tag)
			},
			read: func() []authored.Tag { return transactionOf(t, database, key).Tags },
		},
		"fixed": {
			add: func(ctx context.Context, r *authored.Recorder, tag string) error {
				return authoredtest.AddFixedTag(ctx, r, "name:loja", tag)
			},
			remove: func(ctx context.Context, r *authored.Recorder, tag string) error {
				return authoredtest.RemoveFixedTag(ctx, r, "name:loja", tag)
			},
			read: func() []authored.Tag {
				view := database.View()
				defer view.Close()
				found, err := view.Establishments(t.Context(), []string{"name:loja"})
				if err != nil {
					t.Fatal(err)
				}
				return found["name:loja"].FixedTags
			},
		},
	}
	for name, kind := range kinds {
		for _, tag := range []string{"#a", "#b", "#c"} {
			recorded(t, database, func(ctx context.Context, r *authored.Recorder) error { return kind.add(ctx, r, tag) })
		}
		removal := recorded(t, database, func(ctx context.Context, r *authored.Recorder) error { return kind.remove(ctx, r, "#b") })
		if got, want := kind.read(), []authored.Tag{{Name: "#a", Position: 0}, {Name: "#c", Position: 2}}; !slices.Equal(got, want) {
			t.Errorf("%s tags after removing #b: %v, want %v", name, got, want)
		}
		if err := database.Undo(t.Context(), removal, seedClock); err != nil {
			t.Fatal(err)
		}
		want := []authored.Tag{{Name: "#a", Position: 0}, {Name: "#b", Position: 1}, {Name: "#c", Position: 2}}
		if got := kind.read(); !slices.Equal(got, want) {
			t.Errorf("%s tags after the undo: %v, want %v", name, got, want)
		}
	}
}

func TestAStepIsOverwrittenByALaterChangeToTheSameLine(t *testing.T) {
	database, first, second := withLines(t)
	key := index(t, database, "tx-step")
	put := func(line authored.BudgetLine) authored.ChangeID {
		return recorded(t, database, func(ctx context.Context, r *authored.Recorder) error {
			state := authored.TransactionState{Line: &line.ID, ClassifiedBy: authored.AuthorPerson}
			if err := authoredtest.WriteState(ctx, r, key, state); err != nil {
				return err
			}
			_, err := r.AddHistoryStep(ctx, key, authored.StepEffect, "Você colocou em "+line.Name)
			return err
		})
	}
	earlier, later := put(first), put(second)
	var refused *authored.StaleChangeError
	if err := database.Undo(t.Context(), earlier, seedClock); !errors.As(err, &refused) || refused.Cause != authored.CauseLaterChange {
		t.Fatalf("undoing the overwritten step: %v, want a refusal for a later change", err)
	}
	if history := transactionOf(t, database, key).History; len(history) != 2 || history[0].Change != earlier || history[1].Change != later {
		t.Errorf("history %+v, want a step of each change", history)
	}
}

func TestASplitPartKeepsItsPlaceWhenItLosesItsLine(t *testing.T) {
	database, first, _ := withLines(t)
	key := index(t, database, "tx-split")
	parts := []authored.SplitPart{{Position: 0, Line: &first.ID, Cents: 600}, {Position: 1, Line: &first.ID, Cents: 400}}
	recorded(t, database, func(ctx context.Context, r *authored.Recorder) error {
		return authoredtest.WriteSplit(ctx, r, key, parts)
	})
	lost := recorded(t, database, func(ctx context.Context, r *authored.Recorder) error {
		return authoredtest.WriteSplit(ctx, r, key, []authored.SplitPart{{Position: 0, Cents: 600}, {Position: 1, Line: &first.ID, Cents: 400}})
	})
	if got := transactionOf(t, database, key).Split; len(got) != 2 || got[0].Line != nil || got[0].Cents != 600 || got[1].Line == nil {
		t.Errorf("split after losing a line: %+v", got)
	}
	if err := database.Undo(t.Context(), lost, seedClock); err != nil {
		t.Fatal(err)
	}
	if got := transactionOf(t, database, key).Split; !reflect.DeepEqual(got, parts) {
		t.Errorf("split after the undo: %+v, want %+v", got, parts)
	}
}
