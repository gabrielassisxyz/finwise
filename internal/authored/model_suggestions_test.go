package authored_test

import (
	"database/sql"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

func TestTheModelSuggestionsMigrationIsRecorded(t *testing.T) {
	path := databasePath(t)
	openAndClose(t, path, creationInstant, nil)
	want := embeddedMigrationNames(t)
	if !slices.Contains(want, "0009_model_suggestions") {
		t.Fatalf("the embedded set lacks 0009_model_suggestions: %v", want)
	}
	if got := namesOf(t, path); !equalStrings(got, want) {
		t.Errorf("recorded = %v, want %v", got, want)
	}
}

func TestModelSuggestionsChecksRefuseInvalidShapes(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	var key authored.TxKey
	var changeID authored.ChangeID
	at := time.Date(2026, time.November, 10, 12, 0, 0, 0, time.UTC)
	if err := database.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorPerson, Method: "test.seed", At: at}, func(recorder *authored.Recorder) error {
		var err error
		if key, err = recorder.IndexTransaction(t.Context(), authoredtest.Seen("tx-1", "2026-11-10", -4500, "text:PADARIA")); err != nil {
			return err
		}
		changeID, err = recorder.ID(t.Context())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	pool := database.Pool()
	validAt := "2026-11-10T12:00:00.000Z"

	refused := map[string]struct {
		statement string
		args      []any
	}{
		"empty model": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at) VALUES (?, '', ?)",
			[]any{int64(key), validAt},
		},
		"bad asked_at": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at) VALUES (?, 'm', '2026-11-10')",
			[]any{int64(key)},
		},
		"line without confidence and justification": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at, line_id) VALUES (?, 'm', ?, 1)",
			[]any{int64(key), validAt},
		},
		"confidence without line": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at, confidence) VALUES (?, 'm', ?, 80)",
			[]any{int64(key), validAt},
		},
		"justification without line": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at, justification) VALUES (?, 'm', ?, 'Padaria')",
			[]any{int64(key), validAt},
		},
		"empty justification": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at, line_id, confidence, justification) VALUES (?, 'm', ?, 1, 80, '')",
			[]any{int64(key), validAt},
		},
		"confidence above 100": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at, line_id, confidence, justification) VALUES (?, 'm', ?, 1, 101, 'Padaria')",
			[]any{int64(key), validAt},
		},
		"confidence below 0": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at, line_id, confidence, justification) VALUES (?, 'm', ?, 1, -1, 'Padaria')",
			[]any{int64(key), validAt},
		},
		"answer without change id": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at, line_id, confidence, justification, answer) VALUES (?, 'm', ?, 1, 80, 'Padaria', 'accepted')",
			[]any{int64(key), validAt},
		},
		"change id without answer": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at, line_id, confidence, justification, answer_change_id) VALUES (?, 'm', ?, 1, 80, 'Padaria', ?)",
			[]any{int64(key), validAt, int64(changeID)},
		},
		"answer on row without line": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at, answer, answer_change_id) VALUES (?, 'm', ?, 'accepted', ?)",
			[]any{int64(key), validAt, int64(changeID)},
		},
		"unknown answer": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at, line_id, confidence, justification, answer, answer_change_id) VALUES (?, 'm', ?, 1, 80, 'Padaria', 'skipped', ?)",
			[]any{int64(key), validAt, int64(changeID)},
		},
		"unknown tx_key": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at) VALUES (9999, 'm', ?)",
			[]any{validAt},
		},
		"unknown line_id": {
			"INSERT INTO model_suggestions (tx_key, model, asked_at, line_id, confidence, justification) VALUES (?, 'm', ?, 9999, 80, 'Padaria')",
			[]any{int64(key), validAt},
		},
	}
	for name, c := range refused {
		if _, err := pool.ExecContext(t.Context(), c.statement, c.args...); err == nil {
			t.Errorf("the database accepted %s", name)
		}
	}
}

func TestRecordModelSuggestionsWritesEachOnceWithoutJournaling(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	at := time.Date(2026, time.November, 10, 12, 0, 0, 0, time.UTC)
	authoredtest.SaveBudget(t, database, at, "2026-11", authoredtest.InventedBudget())
	view := database.View()
	budget, err := view.BudgetOf(t.Context(), "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	view.Close()
	lineOne, lineTwo := budget.Lines[0].ID, budget.Lines[1].ID

	var keyUsable, keyUnusable authored.TxKey
	if err := database.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorRule, Method: "ingest.run", At: at}, func(recorder *authored.Recorder) error {
		var err error
		if keyUsable, err = recorder.IndexTransaction(t.Context(), authoredtest.Seen("tx-1", "2026-11-10", -4500, "text:PADARIA")); err != nil {
			return err
		}
		keyUnusable, err = recorder.IndexTransaction(t.Context(), authoredtest.Seen("tx-2", "2026-11-11", -2500, "text:CAFE"))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	changesBefore := countRows(t, database.Pool(), "changes")
	askedAt := time.Date(2026, time.November, 10, 13, 0, 0, 250_000_000, time.UTC)
	conf := int64(82)
	reason := "Padaria de bairro"
	if err := database.RecordModelSuggestions(t.Context(), "model-a", askedAt, []authored.ModelSuggestionInput{
		{Key: keyUsable, Line: &lineOne, Confidence: &conf, Justification: &reason},
		{Key: keyUnusable},
	}); err != nil {
		t.Fatal(err)
	}

	// Recording again for the same keys leaves both original rows untouched.
	laterConf := int64(99)
	laterReason := "Outro motivo"
	if err := database.RecordModelSuggestions(t.Context(), "model-b", askedAt.Add(time.Hour), []authored.ModelSuggestionInput{
		{Key: keyUsable, Line: &lineTwo, Confidence: &laterConf, Justification: &laterReason},
		{Key: keyUnusable, Line: &lineTwo, Confidence: &laterConf, Justification: &laterReason},
	}); err != nil {
		t.Fatal(err)
	}

	if got := countRows(t, database.Pool(), "changes"); got != changesBefore {
		t.Errorf("changes grew from %d to %d", changesBefore, got)
	}
	readView := database.View()
	defer readView.Close()
	got, err := readView.ModelSuggestions(t.Context(), []authored.TxKey{keyUsable, keyUnusable})
	if err != nil {
		t.Fatal(err)
	}
	usable, ok := got[keyUsable]
	if !ok || usable.Model != "model-a" || !usable.AskedAt.Equal(askedAt) ||
		usable.Line == nil || *usable.Line != lineOne ||
		usable.Confidence == nil || *usable.Confidence != 82 ||
		usable.Justification == nil || *usable.Justification != reason ||
		usable.Answer != "" || usable.AnswerChange != nil {
		t.Errorf("usable suggestion = %+v", usable)
	}
	unusable, ok := got[keyUnusable]
	if !ok || unusable.Model != "model-a" || unusable.Line != nil || unusable.Confidence != nil || unusable.Justification != nil {
		t.Errorf("unusable suggestion = %+v", unusable)
	}
}

func TestRecordModelAnswerWritesInThePersonsChangeAndSurvivesUndo(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	at := time.Date(2026, time.November, 10, 12, 0, 0, 0, time.UTC)
	authoredtest.SaveBudget(t, database, at, "2026-11", authoredtest.InventedBudget())
	view := database.View()
	budget, err := view.BudgetOf(t.Context(), "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	view.Close()
	lineOne, lineTwo := budget.Lines[0].ID, budget.Lines[1].ID

	var key authored.TxKey
	if err := database.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorRule, Method: "ingest.run", At: at}, func(recorder *authored.Recorder) error {
		var err error
		key, err = recorder.IndexTransaction(t.Context(), authoredtest.Seen("tx-1", "2026-11-10", -4500, "text:PADARIA"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	conf := int64(82)
	reason := "Padaria de bairro"
	if err := database.RecordModelSuggestions(t.Context(), "model-a", at, []authored.ModelSuggestionInput{
		{Key: key, Line: &lineOne, Confidence: &conf, Justification: &reason},
	}); err != nil {
		t.Fatal(err)
	}

	// A rolled-back change keeps no answer.
	errRollback := errors.New("abort")
	if err := database.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorPerson, Method: "review.acceptPending", At: at.Add(time.Minute)}, func(recorder *authored.Recorder) error {
		if err := recorder.SettleLine(t.Context(), key, lineOne, authored.AuthorPerson); err != nil {
			return err
		}
		if err := recorder.RecordModelAnswer(t.Context(), key, authored.ModelAnswerAccepted); err != nil {
			return err
		}
		return errRollback
	}); !errors.Is(err, errRollback) {
		t.Fatalf("expected rollback error, got %v", err)
	}
	checkView := database.View()
	got, err := checkView.ModelSuggestions(t.Context(), []authored.TxKey{key})
	checkView.Close()
	if err != nil || got[key].Answer != "" || got[key].AnswerChange != nil {
		t.Fatalf("rolled-back answer leaked: %+v (%v)", got[key], err)
	}

	// Committed accept records accepted with the change id.
	var firstChange authored.ChangeID
	if err := database.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorPerson, Method: "review.acceptPending", At: at.Add(2 * time.Minute)}, func(recorder *authored.Recorder) error {
		if err := recorder.SettleLine(t.Context(), key, lineOne, authored.AuthorPerson); err != nil {
			return err
		}
		if err := recorder.RecordModelAnswer(t.Context(), key, authored.ModelAnswerAccepted); err != nil {
			return err
		}
		firstChange, err = recorder.ID(t.Context())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	checkView = database.View()
	got, _ = checkView.ModelSuggestions(t.Context(), []authored.TxKey{key})
	checkView.Close()
	if got[key].Answer != authored.ModelAnswerAccepted || got[key].AnswerChange == nil || *got[key].AnswerChange != firstChange {
		t.Fatalf("after first accept = %+v, want accepted/%d", got[key], firstChange)
	}

	// Undoing leaves the non-journaled row; a second answer overwrites it.
	if err := database.Undo(t.Context(), firstChange, at.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var secondChange authored.ChangeID
	if err := database.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorPerson, Method: "review.acceptPending", At: at.Add(4 * time.Minute)}, func(recorder *authored.Recorder) error {
		if err := recorder.SettleLine(t.Context(), key, lineTwo, authored.AuthorPerson); err != nil {
			return err
		}
		if err := recorder.RecordModelAnswer(t.Context(), key, authored.ModelAnswerCorrected); err != nil {
			return err
		}
		secondChange, err = recorder.ID(t.Context())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	checkView = database.View()
	got, _ = checkView.ModelSuggestions(t.Context(), []authored.TxKey{key})
	checkView.Close()
	if got[key].Answer != authored.ModelAnswerCorrected || got[key].AnswerChange == nil || *got[key].AnswerChange != secondChange {
		t.Fatalf("after second accept = %+v, want corrected/%d", got[key], secondChange)
	}
}

func TestPersonExamplesReturnsNewestPerEstablishmentByPersonWithoutPersons(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	at := time.Date(2026, time.November, 15, 12, 0, 0, 0, time.UTC)
	authoredtest.SaveBudget(t, database, at, "2026-11", authoredtest.InventedBudget())
	view := database.View()
	budget, err := view.BudgetOf(t.Context(), "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	view.Close()
	lineOne, lineTwo := budget.Lines[0].ID, budget.Lines[1].ID

	if err := database.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorPerson, Method: "test.seed", At: at}, func(recorder *authored.Recorder) error {
		seed := []struct {
			id, day, est string
			line         authored.LineID
			by           authored.Author
		}{
			{"tx-old-padaria", "2026-11-01", "text:PADARIA", lineOne, authored.AuthorPerson},
			{"tx-new-padaria", "2026-11-10", "text:PADARIA", lineTwo, authored.AuthorPerson},
			{"tx-rule-mercado", "2026-11-12", "text:MERCADO", lineOne, authored.AuthorRule},
			{"tx-person-cpf", "2026-11-13", "doc:11122233344", lineOne, authored.AuthorPerson},
			{"tx-company-cnpj", "2026-11-14", "doc:12345678000195", lineOne, authored.AuthorPerson},
			{"tx-cafe", "2026-11-05", "text:CAFE", lineOne, authored.AuthorPerson},
		}
		for _, item := range seed {
			key, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen(item.id, brday.Day(item.day), -1000, item.est))
			if err != nil {
				return err
			}
			if err := recorder.SettleLine(t.Context(), key, item.line, item.by); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	readView := database.View()
	defer readView.Close()
	examples, err := readView.PersonExamples(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) != 3 {
		t.Fatalf("got %d examples (%+v), want 3", len(examples), examples)
	}
	if examples[0].EstablishmentKey != "doc:12345678000195" || examples[0].Line != lineOne ||
		examples[1].EstablishmentKey != "text:PADARIA" || examples[1].Line != lineTwo ||
		examples[2].EstablishmentKey != "text:CAFE" || examples[2].Line != lineOne {
		t.Errorf("unexpected examples order/content: %+v", examples)
	}
}

func TestLastModelRunIsRecordedAndReplacedInMeta(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	empty := authored.EmptyView()
	if _, found, err := empty.LastModelRun(t.Context()); err != nil || found {
		t.Fatalf("empty view answered found=%v err=%v", found, err)
	}
	view := database.View()
	if _, found, err := view.LastModelRun(t.Context()); err != nil || found {
		t.Fatalf("fresh database answered found=%v err=%v", found, err)
	}
	view.Close()

	first := time.Date(2026, time.November, 10, 12, 0, 0, 123_000_000, time.UTC)
	if err := database.RecordModelRun(t.Context(), first, false); err != nil {
		t.Fatal(err)
	}
	view = database.View()
	got, found, err := view.LastModelRun(t.Context())
	view.Close()
	if err != nil || !found || !got.At.Equal(first) || got.OK {
		t.Fatalf("after failed run: %+v found=%v err=%v", got, found, err)
	}

	second := first.Add(time.Hour)
	if err := database.RecordModelRun(t.Context(), second, true); err != nil {
		t.Fatal(err)
	}
	view = database.View()
	got, found, err = view.LastModelRun(t.Context())
	view.Close()
	if err != nil || !found || !got.At.Equal(second) || !got.OK {
		t.Fatalf("after ok run: %+v found=%v err=%v", got, found, err)
	}
}

func TestOpenMustExistRefusesMissingOrEmptyFileWithoutCreatingIt(t *testing.T) {
	missing := databasePath(t)
	if _, err := authored.Open(t.Context(), missing, authored.Options{Now: clockAt(t, creationInstant), MustExist: true}); err == nil {
		t.Fatal("missing file was accepted")
	} else if asOpenError(t, err).Failure != authored.FailureMissing {
		t.Errorf("failure = %v, want FailureMissing", err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file was created: %v", err)
	}

	empty := databasePath(t)
	writeFile(t, empty, "")
	if _, err := authored.Open(t.Context(), empty, authored.Options{Now: clockAt(t, creationInstant), MustExist: true}); err == nil {
		t.Fatal("empty file was accepted")
	} else if asOpenError(t, err).Failure != authored.FailureMissing {
		t.Errorf("failure = %v, want FailureMissing", err)
	}
}

func countRows(t *testing.T, pool *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
