package authored_test

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

const markMethod = "review.markTransferDone"

func markOn(t *testing.T, database *authored.Database, day brday.Day, month brday.Month, key string) (authored.ChangeID, bool) {
	t.Helper()
	var recorded bool
	id := changeOn(t, database, markMethod, day, func(recorder *authored.Recorder) (err error) {
		recorded, err = recorder.MarkTransferDone(context.Background(), month, key, day)
		return err
	})
	return id, recorded
}

func marksOf(t *testing.T, database *authored.Database, month brday.Month) map[string]brday.Day {
	t.Helper()
	view := database.View()
	defer view.Close()
	marks, err := view.TransferMarks(t.Context(), month)
	if err != nil {
		t.Fatal(err)
	}
	return marks
}

func TestAMonthsMarksReadBackByKeyAndNoOtherMonths(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	markOn(t, database, "2026-03-20", "2026-03", "pot:reserve:acc-pots/pot-1")
	markOn(t, database, "2026-03-21", "2026-03", "investment")
	markOn(t, database, "2026-04-05", "2026-04", "pot:reserve:acc-pots/pot-1")

	want := map[string]brday.Day{"pot:reserve:acc-pots/pot-1": "2026-03-20", "investment": "2026-03-21"}
	if got := marksOf(t, database, "2026-03"); !maps.Equal(got, want) {
		t.Errorf("marks of 2026-03 = %v, want %v", got, want)
	}
	if got := marksOf(t, database, "2026-04"); !maps.Equal(got, map[string]brday.Day{"pot:reserve:acc-pots/pot-1": "2026-04-05"}) {
		t.Errorf("marks of 2026-04 = %v", got)
	}
	if got := marksOf(t, database, "2026-05"); len(got) != 0 {
		t.Errorf("marks of a month nothing marked = %v", got)
	}
	if got, err := authored.EmptyView().TransferMarks(t.Context(), "2026-03"); err != nil || len(got) != 0 {
		t.Errorf("the empty view answered %v, %v", got, err)
	}
}

// A second mark of the same month and key never reaches the table, whose
// primary key would refuse it: it records nothing and the first day stays.
func TestASecondMarkOfTheSameMonthAndKeyRecordsNothing(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	if _, recorded := markOn(t, database, "2026-03-20", "2026-03", "line:14"); !recorded {
		t.Fatal("the first mark recorded nothing")
	}
	effects := columnValues(t, database.Pool(), "SELECT count(*) FROM change_effects")
	second, recorded := markOn(t, database, "2026-03-22", "2026-03", "line:14")
	if recorded {
		t.Error("the second mark says it recorded something")
	}
	if got := columnValues(t, database.Pool(), "SELECT count(*) FROM change_effects"); !equalStrings(got, effects) {
		t.Errorf("effects went from %v to %v", effects, got)
	}
	if got := marksOf(t, database, "2026-03"); got["line:14"] != "2026-03-20" {
		t.Errorf("the mark is dated %q, want the first day", got["line:14"])
	}
	if err := database.Undo(t.Context(), second, time.Date(2026, 3, 22, 16, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("the undo of the change that recorded nothing: %v", err)
	}
	if got := marksOf(t, database, "2026-03"); got["line:14"] != "2026-03-20" {
		t.Error("undoing the change that recorded nothing took the mark away")
	}
}

func TestUndoingAMarkRemovesItsRowAndTheKeyCanBeMarkedAgain(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first, _ := markOn(t, database, "2026-03-20", "2026-03", "investment")
	if err := database.Undo(t.Context(), first, time.Date(2026, 3, 20, 16, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if got := marksOf(t, database, "2026-03"); len(got) != 0 {
		t.Errorf("after the undo the marks are %v", got)
	}
	if again, recorded := markOn(t, database, "2026-03-21", "2026-03", "investment"); !recorded || again == first {
		t.Errorf("marking again recorded %v under change %d, the first being %d", recorded, again, first)
	}
}

func TestTheTableRefusesAMarkTheStoreDoesNotCheck(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	for _, c := range []struct {
		month brday.Month
		key   string
	}{{"2026-03", ""}, {"2026-3", "investment"}} {
		facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: markMethod, At: time.Date(2026, 3, 20, 15, 0, 0, 0, time.UTC)}
		err := database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
			_, err := recorder.MarkTransferDone(t.Context(), c.month, c.key, "2026-03-20")
			return err
		})
		if err == nil {
			t.Errorf("a mark of %q under %q was kept", c.month, c.key)
		}
	}
	if got := columnValues(t, database.Pool(), "SELECT count(*) FROM transfer_marks"); !equalStrings(got, []string{"0"}) {
		t.Errorf("transfer_marks holds %v rows", got)
	}
}
