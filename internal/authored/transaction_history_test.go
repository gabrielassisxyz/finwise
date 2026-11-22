package authored_test

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// lineChange records what a correction of the line does: the line, and a step
// that says so. It answers the change and the step.
func lineChange(t *testing.T, database *authored.Database, key authored.TxKey) (authored.ChangeID, authored.StepID) {
	t.Helper()
	var step authored.StepID
	id := changeBy(t, database, "transactions.setLine", func(recorder *authored.Recorder) error {
		ctx := context.Background()
		if err := recorder.Set(ctx, "tx_state", key1(integer(int64(key))), authored.Row{"line_id": integer(1), "classified_by": text("person")}); err != nil {
			return err
		}
		var err error
		step, err = recorder.AddHistoryStep(ctx, key, authored.StepEffect, "Você colocou em Tarifas e juros")
		return err
	})
	return id, step
}

func giveReason(t *testing.T, database *authored.Database, step authored.StepID, reason string) authored.ChangeID {
	t.Helper()
	return changeBy(t, database, "transactions.setReason", put("history_step_reasons", authored.Row{
		"step_id": integer(int64(step)), "reason": text(reason),
	}))
}

func canonical(t *testing.T, database *authored.Database) string {
	t.Helper()
	dump, err := database.CanonicalDump(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return dump
}

func TestAReasonGivenLaterLeavesTheStepsChangeToBeUndoneWithIt(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	before := canonical(t, database)
	id, step := lineChange(t, database, key)
	giveReason(t, database, step, "Era presente")

	if can, err := database.CanRestorePart(t.Context(), id, func(authored.Unit) bool { return true }); err != nil || !can {
		t.Errorf("after the reason, the line change can be taken back: %v (%v)", can, err)
	}
	if err := database.Undo(t.Context(), id, undoInstant); err != nil {
		t.Fatalf("undoing the line change after its reason: %v", err)
	}
	if got := canonical(t, database); got != before {
		t.Errorf("the dump after the undo:\n%s\nwant the one before the change:\n%s", got, before)
	}
}

func TestAReasonGivenLaterLeavesTheStepsChangeToBeExtended(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	before := canonical(t, database)
	id, step := lineChange(t, database, key)
	giveReason(t, database, step, "Era presente")

	err := database.Extend(t.Context(), id, "transactions.setLine", func(recorder *authored.Recorder) error {
		return recorder.Set(context.Background(), "tx_state", key1(integer(int64(key))), authored.Row{"note": text("aplicada a todas")})
	})
	if err != nil {
		t.Fatalf("extending the line change after its reason: %v", err)
	}
	undo(t, database, id)
	if got := canonical(t, database); got != before {
		t.Errorf("the dump after the undo of the extended change:\n%s\nwant:\n%s", got, before)
	}
}

func TestAStepCarriesItsChangesIDAndDayEvenInAnExtension(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	id, step := lineChange(t, database, key)
	var note authored.StepID
	err := database.Extend(t.Context(), id, "transactions.setLine", func(recorder *authored.Recorder) error {
		var err error
		note, err = recorder.AddHistoryStep(context.Background(), key, authored.StepNote, "Aplicado a todas as compras daqui")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// changeInstant, 01:30 UTC on 1 December, is 30 November in Brazil.
	for _, s := range []authored.StepID{step, note} {
		view := database.View()
		row, found, err := view.JournaledRow(t.Context(), "history_steps", key1(integer(int64(s))))
		view.Close()
		if err != nil || !found {
			t.Fatalf("step %s: found %v err %v", s, found, err)
		}
		if row["change_id"] != integer(int64(id)) || row["day"] != text("2026-11-30") || row["tx_key"] != integer(int64(key)) {
			t.Errorf("step %s is %v, want change %s on 2026-11-30", s, row, id)
		}
	}
	if row := effectsOf(t, database, id); len(row) != 4 {
		t.Errorf("the extended change has %d effects, want the line, its classifier and two steps", len(row))
	}
}

func TestAStepsIDIsNeverHandedOutAgainAfterAnUndo(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	key := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	id, first := lineChange(t, database, key)
	undo(t, database, id)
	_, second := lineChange(t, database, key)
	if second <= first {
		t.Errorf("the step after undoing step %s got id %s", first, second)
	}
}
