package authored_test

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

const markStepMethod = "review.markStepDone"

func tickOn(t *testing.T, database *authored.Database, day brday.Day, month brday.Month, step string) authored.ChangeID {
	t.Helper()
	return changeOn(t, database, markStepMethod, day, func(recorder *authored.Recorder) error {
		return recorder.MarkClosingStepDone(context.Background(), month, step)
	})
}

func stepsOf(t *testing.T, database *authored.Database, month brday.Month) map[string]bool {
	t.Helper()
	view := database.View()
	defer view.Close()
	steps, err := view.ClosingStepsDone(t.Context(), month)
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

func TestAMonthsTickedStepsReadBackAndNoOtherMonths(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	tickOn(t, database, "2026-03-20", "2026-03", "leftover")
	tickOn(t, database, "2026-03-21", "2026-03", "card-bill")
	tickOn(t, database, "2026-04-02", "2026-04", "report")

	if got := stepsOf(t, database, "2026-03"); !maps.Equal(got, map[string]bool{"leftover": true, "card-bill": true}) {
		t.Errorf("steps of 2026-03 = %v", got)
	}
	if got := stepsOf(t, database, "2026-04"); !maps.Equal(got, map[string]bool{"report": true}) {
		t.Errorf("steps of 2026-04 = %v", got)
	}
	if got := stepsOf(t, database, "2026-05"); len(got) != 0 {
		t.Errorf("steps of a month nothing ticked = %v", got)
	}
	if got, err := authored.EmptyView().ClosingStepsDone(t.Context(), "2026-03"); err != nil || len(got) != 0 {
		t.Errorf("the empty view answered %v, %v", got, err)
	}
}

// A step ticked twice leaves the second change empty, and its undo takes
// nothing away; the undo of the first takes the row back.
func TestASecondTickOfTheSameStepRecordsNothing(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first := tickOn(t, database, "2026-03-20", "2026-03", "leftover")
	effects := columnValues(t, database.Pool(), "SELECT count(*) FROM change_effects")
	second := tickOn(t, database, "2026-03-21", "2026-03", "leftover")
	if got := columnValues(t, database.Pool(), "SELECT count(*) FROM change_effects"); !equalStrings(got, effects) {
		t.Errorf("effects went from %v to %v", effects, got)
	}
	if err := database.Undo(t.Context(), second, time.Date(2026, 3, 21, 16, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("the undo of the change that recorded nothing: %v", err)
	}
	if got := stepsOf(t, database, "2026-03"); !got["leftover"] {
		t.Error("undoing the change that recorded nothing took the tick away")
	}
	if err := database.Undo(t.Context(), first, time.Date(2026, 3, 21, 17, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if got := stepsOf(t, database, "2026-03"); len(got) != 0 {
		t.Errorf("after the undo of the first tick the steps are %v", got)
	}
}

func TestTheTableRefusesAStepNobodyTicksByHand(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	for _, step := range []string{"pending", "close", "xyz"} {
		facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: markStepMethod, At: time.Date(2026, 3, 20, 15, 0, 0, 0, time.UTC)}
		err := database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
			return recorder.MarkClosingStepDone(t.Context(), "2026-03", step)
		})
		if err == nil {
			t.Errorf("a tick of %q was kept", step)
		}
	}
	if got := columnValues(t, database.Pool(), "SELECT count(*) FROM closing_steps"); !equalStrings(got, []string{"0"}) {
		t.Errorf("closing_steps holds %v rows", got)
	}
}

func TestTheClosedMonthsReadBackNewestFirstWithTheirDays(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	at := time.Date(2026, 3, 20, 15, 0, 0, 0, time.UTC)
	authoredtest.CloseMonth(t, database, at, "2025-12", "2026-01-04")
	authoredtest.CloseMonth(t, database, at, "2026-02", "2026-03-02")
	authoredtest.CloseMonth(t, database, at, "2026-01", "2026-02-03")

	view := database.View()
	defer view.Close()
	closed, err := view.ClosedMonths(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []authored.ClosedMonth{{Month: "2026-02", ClosedOn: "2026-03-02"}, {Month: "2026-01", ClosedOn: "2026-02-03"}, {Month: "2025-12", ClosedOn: "2026-01-04"}}
	if !slices.Equal(closed, want) {
		t.Errorf("closed months = %v, want %v", closed, want)
	}
	if got, err := authored.EmptyView().ClosedMonths(t.Context()); err != nil || len(got) != 0 {
		t.Errorf("the empty view answered %v, %v", got, err)
	}
}
