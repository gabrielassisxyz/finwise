package authored_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

func lastIngestedRun(t *testing.T, database *authored.Database) (int64, bool) {
	t.Helper()
	view := database.View()
	defer view.Close()
	run, found, err := view.LastIngestedRun(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return run, found
}

func recordIngestedRun(t *testing.T, database *authored.Database, run int64) authored.ChangeID {
	t.Helper()
	return changeBy(t, database, "ingest.run", func(recorder *authored.Recorder) error {
		return recorder.RecordIngestedRun(context.Background(), run)
	})
}

func TestTheLastIngestedRunMovesOnlyForwardAndOutlivesAnUndo(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	if run, found := lastIngestedRun(t, database); found {
		t.Fatalf("a new database has taken in run %d", run)
	}
	steps := []struct {
		record, want int64
	}{{3, 3}, {3, 3}, {2, 3}, {12, 12}, {9, 12}}
	var last authored.ChangeID
	for _, step := range steps {
		last = recordIngestedRun(t, database, step.record)
		if run, found := lastIngestedRun(t, database); !found || run != step.want {
			t.Errorf("after recording run %d the last is %d (%t), want %d", step.record, run, found, step.want)
		}
	}
	undo(t, database, last)
	if run, _ := lastIngestedRun(t, database); run != 12 {
		t.Errorf("an undo moved the last ingested run to %d", run)
	}
	if got := dump(t, database); got != "" {
		t.Errorf("recording a run authored something:\n%s", got)
	}
	if _, found, err := authored.EmptyView().LastIngestedRun(t.Context()); found || err != nil {
		t.Errorf("the empty view answered a run: %t, %v", found, err)
	}
}

func TestTheRunsAndTheEvaluatedFlagOfAnIndexRowAreObservationsNoUndoTakesBack(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	listed := indexed(t, database, seenRow("p-1", "2026-11-28", -100, "name:a"))
	unlisted := indexed(t, database, seenRow("p-2", "2026-11-28", -200, "name:a"))
	mark := func(write func(context.Context, *authored.Recorder) error) authored.ChangeID {
		return changeBy(t, database, "ingest.run", func(recorder *authored.Recorder) error {
			return write(context.Background(), recorder)
		})
	}
	mark(func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.MarkListed(ctx, listed, 3)
	})
	mark(func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.MarkListed(ctx, listed, 5)
	})
	mark(func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.MarkListed(ctx, listed, 4)
	})
	mark(func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.MarkMetUnlisted(ctx, unlisted, 6)
	})
	mark(func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.MarkMetUnlisted(ctx, unlisted, 7)
	})
	last := mark(func(ctx context.Context, recorder *authored.Recorder) error {
		return recorder.MarkEvaluated(ctx, listed)
	})

	want := "id p-1 1\nid p-2 2\nindex 1 acc-1 2026-11-28 -100 name:a 0 3 5 1\nindex 2 acc-1 2026-11-28 -200 name:a 0 6 - 0"
	if got := theIndex(t, database); got != want {
		t.Errorf("index:\n%s\nwant:\n%s", got, want)
	}
	if rows := journalRows(t, database); rows != 6 {
		t.Errorf("the marks wrote %d rows of the journal, want only the six empty changes the test asked for", rows)
	}
	undo(t, database, last)
	if got := theIndex(t, database); got != want {
		t.Errorf("an undo changed the index:\n%s", got)
	}

	for name, write := range map[string]func(context.Context, *authored.Recorder) error{
		"MarkListed": func(ctx context.Context, recorder *authored.Recorder) error { return recorder.MarkListed(ctx, 99, 1) },
		"MarkMetUnlisted": func(ctx context.Context, recorder *authored.Recorder) error {
			return recorder.MarkMetUnlisted(ctx, 99, 1)
		},
		"MarkEvaluated": func(ctx context.Context, recorder *authored.Recorder) error { return recorder.MarkEvaluated(ctx, 99) },
	} {
		err := database.Change(t.Context(), facts("ingest.run"), func(recorder *authored.Recorder) error {
			return write(context.Background(), recorder)
		})
		if !errors.Is(err, authored.ErrNoSuchTxKey) {
			t.Errorf("%s of a key never given answered %v", name, err)
		}
	}
}

func TestTheIndexReadsBackEveryRowWithEveryIDMappedToIt(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	pending := seenRow("p-pending", "2026-11-28", -4590, "name:padaria")
	pending.BankPending = true
	key := indexed(t, database, pending)
	posted := seenRow("p-posted", "2026-11-30", -4590, "name:padaria")
	posted.Replaces = key
	indexed(t, database, posted)
	other := indexed(t, database, seenRow("p-other", "2026-11-29", 700, "doc:12345678901"))
	changeBy(t, database, "ingest.run", func(recorder *authored.Recorder) error {
		if err := recorder.MarkListed(context.Background(), other, 4); err != nil {
			return err
		}
		return recorder.MarkEvaluated(context.Background(), other)
	})

	view := database.View()
	defer view.Close()
	rows, err := view.IndexedTransactions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("the index read %d rows, want 2: %+v", len(rows), rows)
	}
	first, second := rows[0], rows[1]
	if first.Key != key || !slices.Equal(first.ProviderIDs, []string{"p-pending", "p-posted"}) || first.Day != "2026-11-30" ||
		first.Cents != -4590 || first.BankPending || first.Evaluated || first.FirstRun != nil || first.LastRun != nil {
		t.Errorf("the paired row reads %+v", first)
	}
	if second.Key != other || !slices.Equal(second.ProviderIDs, []string{"p-other"}) || second.EstablishmentKey != "doc:12345678901" ||
		second.Cents != 700 || !second.Evaluated || second.FirstRun == nil || *second.FirstRun != 4 || second.LastRun == nil || *second.LastRun != 4 {
		t.Errorf("the evaluated row reads %+v", second)
	}
}

func TestTheExamplesOfAnEstablishmentAreItsIndexedTransactionsOnALine(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first, second := twoLines(t, database)
	byPerson := indexed(t, database, seenRow("p-1", "2026-11-10", -100, "name:a"))
	excluded := indexed(t, database, seenRow("p-2", "2026-11-11", -100, "name:a"))
	split := indexed(t, database, seenRow("p-3", "2026-11-12", -100, "name:a"))
	noLine := indexed(t, database, seenRow("p-4", "2026-11-13", -100, "name:a"))
	elsewhere := indexed(t, database, seenRow("p-5", "2026-11-14", -100, "name:b"))
	changeBy(t, database, "scratch.seed", func(recorder *authored.Recorder) error {
		ctx := context.Background()
		for _, write := range []struct {
			key   authored.TxKey
			state authored.TransactionState
		}{
			{byPerson, authored.TransactionState{Line: &first, ClassifiedBy: authored.AuthorPerson}},
			{excluded, authored.TransactionState{Line: &second, ClassifiedBy: authored.AuthorRule, Exclusion: authoredtest.Some("Reembolsada")}},
			{split, authored.TransactionState{Line: &first, ClassifiedBy: authored.AuthorPerson}},
			{noLine, authored.TransactionState{Note: "sem linha"}},
			{elsewhere, authored.TransactionState{Line: &first, ClassifiedBy: authored.AuthorPerson}},
		} {
			if err := authoredtest.WriteState(ctx, recorder, write.key, write.state); err != nil {
				return err
			}
		}
		return authoredtest.WriteSplit(ctx, recorder, split, []authored.SplitPart{{Position: 0, Line: &first, Cents: 60}, {Position: 1, Line: &second, Cents: 40}})
	})

	view := database.View()
	defer view.Close()
	examples, err := view.LineExamples(t.Context(), "name:a")
	if err != nil {
		t.Fatal(err)
	}
	want := []authored.LineExample{
		{Key: byPerson, Day: "2026-11-10", Line: first, ClassifiedBy: authored.AuthorPerson},
		{Key: excluded, Day: "2026-11-11", Line: second, ClassifiedBy: authored.AuthorRule, Exclusion: authoredtest.Some("Reembolsada")},
		{Key: split, Day: "2026-11-12", Line: first, ClassifiedBy: authored.AuthorPerson, Split: true},
	}
	if len(examples) != len(want) {
		t.Fatalf("the examples are %+v, want %+v", examples, want)
	}
	for i, example := range examples {
		wanted := want[i]
		sameExclusion := (example.Exclusion == nil) == (wanted.Exclusion == nil) && (example.Exclusion == nil || *example.Exclusion == *wanted.Exclusion)
		example.Exclusion, wanted.Exclusion = nil, nil
		if example != wanted || !sameExclusion {
			t.Errorf("example %d is %+v, want %+v", i, examples[i], want[i])
		}
	}
}

func TestAPendingReviewKeepsItsSuggestionOrNone(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	first, _ := twoLines(t, database)
	suggested := indexed(t, database, seenRow("p-1", "2026-11-10", -100, "name:a"))
	unsuggested := indexed(t, database, seenRow("p-2", "2026-11-11", -100, "name:b"))
	changeBy(t, database, "ingest.run", func(recorder *authored.Recorder) error {
		ctx := context.Background()
		if err := recorder.SetPendingReview(ctx, suggested, "Baixa confiança", &first, authoredtest.Some[int64](83)); err != nil {
			return err
		}
		return recorder.SetPendingReview(ctx, unsuggested, "Estabelecimento novo", nil, nil)
	})
	state := keptAbout(t, database, suggested).State
	if state.PendingReason != "Baixa confiança" || state.PendingLine == nil || *state.PendingLine != first ||
		state.PendingConfidence == nil || *state.PendingConfidence != 83 || state.Line != nil {
		t.Errorf("the suggested review is %+v", state)
	}
	state = keptAbout(t, database, unsuggested).State
	if state.PendingReason != "Estabelecimento novo" || state.PendingLine != nil || state.PendingConfidence != nil {
		t.Errorf("the review without a suggestion is %+v", state)
	}
}

func TestTheLinesOfAMonthAreItsVersionsThenTheFeesLineOnlyWhenItHasOne(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	linesOf := func(month string) []authored.MonthLine {
		t.Helper()
		view := database.View()
		defer view.Close()
		lines, err := view.LinesOfMonth(t.Context(), brday.Month(month))
		if err != nil {
			t.Fatal(err)
		}
		return lines
	}
	if lines := linesOf("2026-11"); len(lines) != 0 {
		t.Errorf("a month with no budget has %v", lines)
	}
	empty := authored.Document{}
	for _, key := range authored.GroupKeys {
		empty = append(empty, authoredtest.Group(key))
	}
	authoredtest.SaveBudget(t, database, changeInstant, "2026-10", empty)
	if lines := linesOf("2026-11"); len(lines) != 0 {
		t.Errorf("a month whose five groups are empty has %v", lines)
	}
	first, second := twoLines(t, database)
	want := []authored.MonthLine{{ID: first, Name: "Mercado"}, {ID: second, Name: "Delivery"}, {ID: 1, Name: authored.FeesLineName}}
	if lines := linesOf("2026-11"); !slices.Equal(lines, want) {
		t.Errorf("the lines of the month are %v, want %v", lines, want)
	}
	if lines := linesOf("2026-10"); len(lines) != 0 {
		t.Errorf("the month before the save has %v", lines)
	}
}
