package authored_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// A step of a transaction's history is taken back by a change of its own that
// restores, of the step's change, the units of that one transaction, the step
// included. These are the steps the exclusion and the refund leave, written as
// their writes write them.

type steppedWrite func(context.Context, *authored.Recorder, authored.TxKey) (bool, error)

// stepped records write in a change of its own and, when it changed the
// transaction, the step with text; it answers the step, or the zero one.
func stepped(t *testing.T, database *authored.Database, key authored.TxKey, text string, write steppedWrite) authored.HistoryStep {
	t.Helper()
	var id authored.StepID
	changeBy(t, database, "transactions.test", func(recorder *authored.Recorder) error {
		ctx := context.Background()
		changed, err := write(ctx, recorder, key)
		if err != nil || !changed {
			return err
		}
		id, err = recorder.AddHistoryStep(ctx, key, authored.StepEffect, text)
		return err
	})
	for _, step := range heldOf(t, database, key).History {
		if step.ID == id {
			return step
		}
	}
	return authored.HistoryStep{}
}

func markRefund(on bool) steppedWrite {
	return func(ctx context.Context, recorder *authored.Recorder, key authored.TxKey) (bool, error) {
		return recorder.SetRefund(ctx, key, on)
	}
}

func refundLineOf(line authored.LineID) steppedWrite {
	return func(ctx context.Context, recorder *authored.Recorder, key authored.TxKey) (bool, error) {
		return recorder.SetRefundLine(ctx, key, line)
	}
}

func exclusionOf(exclusion *string) steppedWrite {
	return func(ctx context.Context, recorder *authored.Recorder, key authored.TxKey) (bool, error) {
		return recorder.SetExclusion(ctx, key, exclusion)
	}
}

func stepUnits(key authored.TxKey, step authored.HistoryStep) func(authored.Unit) bool {
	return authored.TransactionUnits(key, []authored.StepID{step.ID})
}

// takeBack takes step back in a change of its own, after checking that the
// read-only check says what the attempt then does. It answers the change, or
// the refusal.
func takeBack(t *testing.T, database *authored.Database, key authored.TxKey, step authored.HistoryStep) (authored.ChangeID, error) {
	t.Helper()
	can, err := database.CanRestorePart(t.Context(), step.Change, stepUnits(key, step))
	if err != nil {
		t.Fatal(err)
	}
	var id authored.ChangeID
	err = database.Change(t.Context(), facts("transactions.undoHistoryStep"), func(recorder *authored.Recorder) error {
		if err := recorder.RestorePart(context.Background(), step.Change, stepUnits(key, step)); err != nil {
			return err
		}
		var err error
		id, err = recorder.ID(context.Background())
		return err
	})
	var refused *authored.StaleChangeError
	if err != nil && !errors.As(err, &refused) {
		t.Fatalf("taking back %q failed: %v", step.Text, err)
	}
	if can != (err == nil) {
		t.Errorf("the check said %q could be taken back: %v, and taking it back answered %v", step.Text, can, err)
	}
	return id, err
}

func stepTexts(steps []authored.HistoryStep) []string {
	texts := []string{}
	for _, step := range steps {
		texts = append(texts, step.Text)
	}
	return texts
}

// creditOverABudget is a database with the invented budget and one credit
// indexed; it answers the database, the credit's key and the id of Mercado.
func creditOverABudget(t *testing.T) (*authored.Database, authored.TxKey, authored.LineID) {
	t.Helper()
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	authoredtest.SaveBudget(t, database, time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), "2026-11", authoredtest.InventedBudget())
	view := database.View()
	defer view.Close()
	budget, err := view.BudgetOf(t.Context(), "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(budget.Lines, func(line authored.BudgetLine) bool { return line.Name == "Mercado" })
	if at < 0 {
		t.Fatal("the invented budget has no Mercado")
	}
	return database, indexed(t, database, seenRow("p-pix", "2026-11-28", 3_000, "name:a")), budget.Lines[at].ID
}

func TestTakingBackTheRefundMarkLeavesTheLineChosenAfterIt(t *testing.T) {
	database, key, market := creditOverABudget(t)
	mark := stepped(t, database, key, "Você marcou como estorno", markRefund(true))
	stepped(t, database, key, "Você definiu que o estorno abate Mercado", refundLineOf(market))
	if _, err := takeBack(t, database, key, mark); err != nil {
		t.Fatalf("the mark's step was refused: %v", err)
	}
	held := heldOf(t, database, key)
	if held.State.Refund != nil || held.State.RefundLine == nil || *held.State.RefundLine != market {
		t.Errorf("after taking the mark back the state is %+v, want no mark and the line Mercado", held.State)
	}
	if got := stepTexts(held.History); !slices.Equal(got, []string{"Você definiu que o estorno abate Mercado"}) {
		t.Errorf("the history is %v, want the line's step alone", got)
	}
}

func TestTakingBackCountingAgainBringsTheReasonBack(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -18_000, "name:a"))
	reason := "Compra da empresa"
	fact(t, database, "transactions.setExclusionReason", func(ctx context.Context, recorder *authored.Recorder) (bool, error) {
		return exclusionOf(&reason)(ctx, recorder, key)
	})
	again := stepped(t, database, key, "Você voltou a contar esta transação", exclusionOf(nil))
	if _, err := takeBack(t, database, key, again); err != nil {
		t.Fatalf("the step was refused: %v", err)
	}
	held := heldOf(t, database, key)
	if held.State.Exclusion == nil || *held.State.Exclusion != reason || len(held.History) != 0 {
		t.Errorf("after taking the step back the exclusion is %v and the history %v", held.State.Exclusion, stepTexts(held.History))
	}
}

func TestUndoingTheTakingBackOfAMarkMarksItAgain(t *testing.T) {
	database, key, _ := creditOverABudget(t)
	mark := stepped(t, database, key, "Você marcou como estorno", markRefund(true))
	takenBack, err := takeBack(t, database, key, mark)
	if err != nil {
		t.Fatal(err)
	}
	if state := heldOf(t, database, key).State; state.Refund != nil {
		t.Errorf("after taking the mark back it is %v, want the ledger's rule", *state.Refund)
	}
	undo(t, database, takenBack)
	held := heldOf(t, database, key)
	if held.State.Refund == nil || !*held.State.Refund || !slices.Equal(stepTexts(held.History), []string{"Você marcou como estorno"}) {
		t.Errorf("after the undo the refund is %v and the history %v", held.State.Refund, stepTexts(held.History))
	}
}

func TestTakingBackTheFeesLineAsARefundLineLeavesNoLine(t *testing.T) {
	database, key, _ := creditOverABudget(t)
	line := stepped(t, database, key, "Você definiu que o estorno abate Tarifas e juros", refundLineOf(1))
	if _, err := takeBack(t, database, key, line); err != nil {
		t.Fatal(err)
	}
	if state := heldOf(t, database, key).State; state.RefundLine != nil {
		t.Errorf("after taking the line back it is %d, want none", *state.RefundLine)
	}
}

func TestAStepWhoseColumnALaterChangeRewroteIsNotTakenBackAndNothingChanges(t *testing.T) {
	database, key, _ := creditOverABudget(t)
	mark := stepped(t, database, key, "Você marcou como estorno", markRefund(true))
	stepped(t, database, key, "Você desmarcou o estorno", markRefund(false))
	before := canonical(t, database)
	if _, err := takeBack(t, database, key, mark); err == nil {
		t.Fatal("the mark's step was taken back over the later unmarking")
	}
	if after := canonical(t, database); after != before {
		t.Errorf("the refused attempt changed the database:\n%s\nwas\n%s", after, before)
	}
}
