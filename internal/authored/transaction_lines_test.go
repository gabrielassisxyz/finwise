package authored_test

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// twoLines saves a budget of two lines and answers their ids.
func twoLines(t *testing.T, database *authored.Database) (authored.LineID, authored.LineID) {
	t.Helper()
	authoredtest.SaveBudget(t, database, changeInstant, "2026-11", authored.Document{
		authoredtest.Group("var", authoredtest.Line("Mercado", 40_000), authoredtest.Line("Delivery", 20_000)),
	})
	view := database.View()
	defer view.Close()
	budget, err := view.BudgetOf(t.Context(), "2026-11")
	if err != nil || len(budget.Lines) != 2 {
		t.Fatalf("the budget of two lines: %v, %v", budget, err)
	}
	return budget.Lines[0].ID, budget.Lines[1].ID
}

func keptAbout(t *testing.T, database *authored.Database, key authored.TxKey) authored.AuthoredTransaction {
	t.Helper()
	view := database.View()
	defer view.Close()
	kept, err := view.Transactions(t.Context(), []authored.TxKey{key})
	if err != nil {
		t.Fatal(err)
	}
	return kept[key]
}

// reviewedByTheModel indexes a purchase the model put on first and that waits
// for a review suggesting second, with the question of why open.
func reviewedByTheModel(t *testing.T, database *authored.Database, first, second authored.LineID) authored.TxKey {
	t.Helper()
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	changeBy(t, database, "scratch.seed", func(recorder *authored.Recorder) error {
		return authoredtest.WriteState(context.Background(), recorder, key, authored.TransactionState{
			Line: &first, ClassifiedBy: authored.AuthorModel, Confidence: authoredtest.Some[int64](55),
			PendingReason: "Confiança abaixo do limite", PendingLine: &second, PendingConfidence: authoredtest.Some[int64](70),
			AskReason: true, Note: "nota",
		})
	})
	return key
}

func TestSettlingALineEndsTheReviewAndLeavesEverythingElseAsItWas(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first, second := twoLines(t, database)
	key := reviewedByTheModel(t, database, first, second)
	changeBy(t, database, "transactions.setLine", func(recorder *authored.Recorder) error {
		return recorder.SettleLine(context.Background(), key, second, authored.AuthorPerson)
	})
	state := keptAbout(t, database, key).State
	if state.Line == nil || *state.Line != second || state.ClassifiedBy != authored.AuthorPerson {
		t.Errorf("the line is %v by %q, want %d by the person", state.Line, state.ClassifiedBy, second)
	}
	if state.Confidence != nil || state.PendingReason != "" || state.PendingLine != nil || state.PendingConfidence != nil {
		t.Errorf("the review is still there: %+v", state)
	}
	if !state.AskReason || state.Note != "nota" {
		t.Errorf("settling touched the question of why or the note: %+v", state)
	}
	err := database.Change(t.Context(), facts("transactions.setLine"), func(recorder *authored.Recorder) error {
		return recorder.SettleLine(context.Background(), key, first, "nobody")
	})
	if err != authored.ErrInvalidAuthor {
		t.Errorf("a line settled by nobody answered %v", err)
	}
}

func TestAReasonIsOwnedByItsStepAndAnEmptyOneRemovesIt(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	var step authored.StepID
	origin := changeBy(t, database, "transactions.setLine", func(recorder *authored.Recorder) error {
		if err := recorder.SettleLine(context.Background(), key, 1, authored.AuthorPerson); err != nil {
			return err
		}
		var err error
		step, err = recorder.AddHistoryStep(context.Background(), key, authored.StepEffect, "Você alterou para Tarifas e juros")
		return err
	})
	reason := func(text string) authored.ChangeID {
		return changeBy(t, database, "transactions.setReason", func(recorder *authored.Recorder) error {
			return recorder.SetStepReason(context.Background(), step, text)
		})
	}
	effects := func(id authored.ChangeID) int {
		return len(columnValues(t, database.Pool(), `SELECT seq FROM change_effects WHERE change_id = `+id.String()))
	}
	given := reason("é do apartamento")
	if history := keptAbout(t, database, key).History; len(history) != 1 || history[0].Reason != "é do apartamento" {
		t.Fatalf("the history after a reason is %+v", history)
	}
	if same := reason("é do apartamento"); effects(same) != 0 {
		t.Errorf("the same reason again recorded %d effects", effects(same))
	}
	if removed := reason(""); effects(removed) != 1 || keptAbout(t, database, key).History[0].Reason != "" {
		t.Errorf("an empty reason recorded %d effects and left %+v", effects(removed), keptAbout(t, database, key).History)
	}
	if none := reason(""); effects(none) != 0 {
		t.Errorf("an empty reason on a step without one recorded %d effects", effects(none))
	}
	reason("outra razão")
	undo(t, database, origin)
	if left := columnValues(t, database.Pool(), `SELECT step_id FROM history_step_reasons`); len(left) != 0 {
		t.Errorf("undoing the step's change left reasons %v", left)
	}
	if given == 0 {
		t.Error("a reason was given in no change")
	}
}

func TestTheStepsOfAChangeAreTheOnesItWroteThatAreStillThere(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	second := indexed(t, database, seenRow("p-2", "2026-11-28", -200, "name:b"))
	id, firstStep, secondStep := twoTransactionsInOneChange(t, database, first, second)
	view := database.View()
	steps, err := view.StepsOfChange(t.Context(), id)
	view.Close()
	want := []authored.ChangeStep{{ID: firstStep, Key: first}, {ID: secondStep, Key: second}}
	if err != nil || len(steps) != 2 || steps[0] != want[0] || steps[1] != want[1] {
		t.Fatalf("the steps of the change are %v, %v; want %v", steps, err, want)
	}
	undo(t, database, id)
	view = database.View()
	defer view.Close()
	if steps, err := view.StepsOfChange(t.Context(), id); err != nil || len(steps) != 0 {
		t.Errorf("an undone change still has steps %v, %v", steps, err)
	}
}

// takeBack takes back step of the transaction at key in a change of its own.
func takeBack(t *testing.T, database *authored.Database, key authored.TxKey, step authored.StepID) (authored.ChangeID, error) {
	t.Helper()
	kept := keptAbout(t, database, key)
	var found authored.HistoryStep
	var ids []authored.StepID
	for _, held := range kept.History {
		ids = append(ids, held.ID)
		if held.ID == step {
			found = held
		}
	}
	var id authored.ChangeID
	err := database.Change(t.Context(), facts("transactions.undoHistoryStep"), func(recorder *authored.Recorder) error {
		if err := recorder.TakeBackStep(context.Background(), key, found, ids); err != nil {
			return err
		}
		var err error
		id, err = recorder.ID(context.Background())
		return err
	})
	return id, err
}

func TestTakingAStepBackRestoresThatTransactionAloneAndClosesTheQuestion(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first, second := twoLines(t, database)
	key := reviewedByTheModel(t, database, first, second)
	other := indexed(t, database, seenRow("p-2", "2026-11-28", -200, "name:a"))
	var step, otherStep authored.StepID
	changeBy(t, database, "transactions.setLine", func(recorder *authored.Recorder) error {
		ctx := context.Background()
		for _, settled := range []authored.TxKey{key, other} {
			if err := recorder.SettleLine(ctx, settled, second, authored.AuthorPerson); err != nil {
				return err
			}
		}
		var err error
		if step, err = recorder.AddHistoryStep(ctx, key, authored.StepEffect, "Você alterou para Delivery"); err != nil {
			return err
		}
		otherStep, err = recorder.AddHistoryStep(ctx, other, authored.StepEffect, "Regra aplicou Delivery")
		return err
	})
	changeBy(t, database, "transactions.setReason", func(recorder *authored.Recorder) error {
		return recorder.SetStepReason(context.Background(), step, "é do apartamento")
	})
	before := dump(t, database)
	id, err := takeBack(t, database, key, step)
	if err != nil {
		t.Fatalf("taking the step back: %v", err)
	}
	state := keptAbout(t, database, key)
	if state.State.Line == nil || *state.State.Line != first || state.State.ClassifiedBy != authored.AuthorModel ||
		state.State.PendingLine == nil || *state.State.PendingLine != second || state.State.AskReason || len(state.History) != 0 {
		t.Errorf("the transaction after its step was taken back is %+v", state)
	}
	if kept := keptAbout(t, database, other); kept.State.Line == nil || *kept.State.Line != second || len(kept.History) != 1 || kept.History[0].ID != otherStep {
		t.Errorf("the other transaction of the change moved: %+v", kept)
	}
	undo(t, database, id)
	if after := dump(t, database); after != before {
		t.Errorf("undoing the step's undo does not put the step and its reason back:\n%s\nwas\n%s", after, before)
	}
}

func TestAStepChangedSinceIsNotTakenBack(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first, second := twoLines(t, database)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	var step authored.StepID
	changeBy(t, database, "transactions.setLine", func(recorder *authored.Recorder) error {
		if err := recorder.SettleLine(context.Background(), key, first, authored.AuthorPerson); err != nil {
			return err
		}
		var err error
		step, err = recorder.AddHistoryStep(context.Background(), key, authored.StepEffect, "Você alterou para Mercado")
		return err
	})
	changeBy(t, database, "transactions.setLineForMany", func(recorder *authored.Recorder) error {
		return recorder.SettleLine(context.Background(), key, second, authored.AuthorPerson)
	})
	before := dump(t, database)
	_, err := takeBack(t, database, key, step)
	assertStale(t, err, authored.CauseLaterChange)
	if after := dump(t, database); after != before {
		t.Errorf("a refused step's undo changed the tables:\n%s\nwas\n%s", after, before)
	}
}
