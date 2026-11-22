package authored_test

import (
	"context"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const (
	insertSetting = `INSERT INTO settings (key, value) VALUES (?, ?)`
	countEffects  = `SELECT count(*) FROM change_effects`
)

func TestTheSettingsMigrationIsRecordedOnceAndAppliedNothingTheSecondTime(t *testing.T) {
	path := databasePath(t)
	openAndClose(t, path, creationInstant, nil)
	openAndClose(t, path, laterInstant, nil)
	got := columnValues(t, inspect(t, path), "SELECT name FROM schema_migrations WHERE name = '0004_settings'")
	if !equalStrings(got, []string{"0004_settings"}) {
		t.Errorf("recorded %v, want 0004_settings once", got)
	}
	if copies := copiesBeside(t, path); len(copies) != 0 {
		t.Errorf("the second open took a copy, so it migrated: %v", copies)
	}
}

// Each refusal is preceded by the valid row of the same key it differs from, so
// it can only come from the constraint under test.
func TestTheSettingsTableRefusesWhatIsNotASetting(t *testing.T) {
	cases := []struct {
		name, key      string
		valid, invalid any
	}{
		{"the mode weekly", "closing_mode", "auto", "weekly"},
		{"a threshold of 49", "confidence_threshold", 50, 49},
		{"a threshold of 100", "confidence_threshold", 99, 100},
		{"a minimum of 0", "min_examples", 1, 0},
		{"a minimum of 51", "min_examples", 50, 51},
		{"a manual day of 0", "manual_closing_day", 1, 0},
		{"a manual day of 29", "manual_closing_day", 28, 29},
		{"a threshold of -1", "confidence_threshold", 50, -1},
		{"a minimum of -1", "min_examples", 1, -1},
		{"a manual day of -1", "manual_closing_day", 1, -1},
		{"a threshold written as text", "confidence_threshold", 85, "85"},
		{"a threshold with a fraction", "confidence_threshold", 85, 85.5},
		{"a label written as a number", "card_label", "Físico", 7},
		{"a mode written as a number", "closing_mode", "auto", 1},
		{"no value", "card_label", "Físico", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := migratedPool(t)
			if _, err := database.Exec(insertSetting, c.key, c.valid); err != nil {
				t.Fatalf("the valid row was refused: %v", err)
			}
			mustExec(t, database, `DELETE FROM settings`)
			if _, err := database.Exec(insertSetting, c.key, c.invalid); err == nil {
				t.Errorf("the database accepted %s = %v", c.key, c.invalid)
			}
		})
	}
}

func TestTheSettingsTableRefusesASecondRowForOneKeyAndAnUnknownKey(t *testing.T) {
	database := migratedPool(t)
	if _, err := database.Exec(insertSetting, "min_examples", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(insertSetting, "min_examples", 6); err == nil {
		t.Error("the database accepted a second row for one key")
	}
	if _, err := database.Exec(insertSetting, "other", 5); err == nil {
		t.Error("the database accepted an unknown key")
	}
}

func TestTheSettingsTableAcceptsEachBoundAndBothModes(t *testing.T) {
	accepted := []struct {
		key   string
		value any
	}{
		{"confidence_threshold", 50}, {"confidence_threshold", 99},
		{"min_examples", 1}, {"min_examples", 50},
		{"manual_closing_day", 1}, {"manual_closing_day", 28},
		{"closing_mode", "auto"}, {"closing_mode", "manual"},
		{"card_label", "Físico"},
	}
	for _, c := range accepted {
		database := migratedPool(t)
		if _, err := database.Exec(insertSetting, c.key, c.value); err != nil {
			t.Errorf("the database refused %s = %v: %v", c.key, c.value, err)
		}
	}
}

func settingsAt(t *testing.T, view *authored.View) (threshold, examples, day int64, label string, mode authored.ClosingMode) {
	t.Helper()
	ctx := t.Context()
	var errs [5]error
	threshold, errs[0] = view.ConfidenceThreshold(ctx)
	examples, errs[1] = view.MinExamples(ctx)
	label, errs[2] = view.CardLabel(ctx)
	mode, errs[3] = view.CardClosingMode(ctx)
	day, errs[4] = view.ManualClosingDay(ctx)
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	return
}

func requireDefaults(t *testing.T, view *authored.View) {
	t.Helper()
	threshold, examples, day, label, mode := settingsAt(t, view)
	if threshold != 85 || examples != 5 || label != "Físico" || mode != "auto" || day != 28 {
		t.Errorf("answers %d, %d, %q, %q, %d; want 85, 5, \"Físico\", auto, 28", threshold, examples, label, mode, day)
	}
}

func TestEverySettingAnswersItsDefaultOnAFreshDatabaseAndOnTheEmptyView(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	fresh := database.View()
	defer fresh.Close()
	requireDefaults(t, fresh)
	requireDefaults(t, authored.EmptyView())
}

func recordSetting(t *testing.T, database *authored.Database, at time.Time, write func(ctx context.Context, recorder *authored.Recorder) error) authored.ChangeID {
	t.Helper()
	var id authored.ChangeID
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "settings.test", At: at}
	err := database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		var err error
		if id, err = recorder.ID(t.Context()); err != nil {
			return err
		}
		return write(t.Context(), recorder)
	})
	if err != nil {
		t.Fatalf("change: %v", err)
	}
	return id
}

func effectCount(t *testing.T, database *authored.Database) int {
	t.Helper()
	var count int
	if err := database.Pool().QueryRow(countEffects).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

var settingsStart = time.Date(2026, time.November, 2, 12, 0, 0, 0, time.UTC)

type settingCase struct {
	name          string
	first, second func(ctx context.Context, recorder *authored.Recorder) error
	read          func(t *testing.T, view *authored.View) any
	defaultValue  any
	firstValue    any
	secondValue   any
}

func settingCases() []settingCase {
	return []settingCase{
		{"the confidence threshold",
			func(ctx context.Context, r *authored.Recorder) error { return r.SetConfidenceThreshold(ctx, 70) },
			func(ctx context.Context, r *authored.Recorder) error { return r.SetConfidenceThreshold(ctx, 60) },
			func(t *testing.T, v *authored.View) any { a, _, _, _, _ := settingsAt(t, v); return a }, int64(85), int64(70), int64(60)},
		{"the minimum of examples",
			func(ctx context.Context, r *authored.Recorder) error { return r.SetMinExamples(ctx, 12) },
			func(ctx context.Context, r *authored.Recorder) error { return r.SetMinExamples(ctx, 20) },
			func(t *testing.T, v *authored.View) any { _, a, _, _, _ := settingsAt(t, v); return a }, int64(5), int64(12), int64(20)},
		{"the card label",
			func(ctx context.Context, r *authored.Recorder) error { return r.SetCardLabel(ctx, "Cartão") },
			func(ctx context.Context, r *authored.Recorder) error { return r.SetCardLabel(ctx, "Crédito") },
			func(t *testing.T, v *authored.View) any { _, _, _, a, _ := settingsAt(t, v); return a }, "Físico", "Cartão", "Crédito"},
		{"the closing mode",
			func(ctx context.Context, r *authored.Recorder) error {
				return r.SetClosingMode(ctx, authored.ClosingManual)
			},
			func(ctx context.Context, r *authored.Recorder) error {
				return r.SetClosingMode(ctx, authored.ClosingAuto)
			},
			func(t *testing.T, v *authored.View) any { _, _, _, _, a := settingsAt(t, v); return a },
			authored.ClosingAuto, authored.ClosingManual, authored.ClosingAuto},
		{"the manual closing day",
			func(ctx context.Context, r *authored.Recorder) error { return r.SetManualClosingDay(ctx, 10) },
			func(ctx context.Context, r *authored.Recorder) error { return r.SetManualClosingDay(ctx, 15) },
			func(t *testing.T, v *authored.View) any { _, _, a, _, _ := settingsAt(t, v); return a }, int64(28), int64(10), int64(15)},
	}
}

func readNow(t *testing.T, database *authored.Database, c settingCase) any {
	t.Helper()
	view := database.View()
	defer view.Close()
	return c.read(t, view)
}

func TestEachSettingIsAnsweredByItsReadAndByALaterView(t *testing.T) {
	for _, c := range settingCases() {
		t.Run(c.name, func(t *testing.T) {
			database := openDatabase(t, databasePath(t), creationInstant, nil)
			before := database.View()
			defer before.Close()
			if got := c.read(t, before); got != c.defaultValue {
				t.Fatalf("before the write: %v, want the default %v", got, c.defaultValue)
			}
			recordSetting(t, database, settingsStart, c.first)
			if got := readNow(t, database, c); got != c.firstValue {
				t.Errorf("a later view answers %v, want %v", got, c.firstValue)
			}
			if got := c.read(t, before); got != c.defaultValue {
				t.Errorf("the view opened before the write answers %v, want %v", got, c.defaultValue)
			}
		})
	}
}

func TestTheClosingModeAndTheManualDayWrittenTogetherAreOneChangeWithAnEffectOnEach(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	recordSetting(t, database, settingsStart, func(ctx context.Context, r *authored.Recorder) error {
		return r.SetCardClosing(ctx, authored.ClosingManual, 12)
	})
	if got := effectCount(t, database); got != 2 {
		t.Errorf("%d effects, want 2", got)
	}
	var changes int
	if err := database.Pool().QueryRow(`SELECT count(*) FROM changes`).Scan(&changes); err != nil || changes != 1 {
		t.Errorf("%d changes (%v), want 1", changes, err)
	}
	_, _, day, _, mode := settingsAt(t, database.View())
	if day != 12 || mode != authored.ClosingManual {
		t.Errorf("read %s and %d, want manual and 12", mode, day)
	}
}

func TestWritingTheValueAlreadyStoredAddsNoEffect(t *testing.T) {
	for _, c := range settingCases() {
		t.Run(c.name, func(t *testing.T) {
			database := openDatabase(t, databasePath(t), creationInstant, nil)
			recordSetting(t, database, settingsStart, c.first)
			effects := effectCount(t, database)
			recordSetting(t, database, settingsStart.Add(time.Hour), c.first)
			if got := effectCount(t, database); got != effects {
				t.Errorf("rewriting the stored value took the effects from %d to %d", effects, got)
			}
		})
	}
}

// A value that equals the default still stores a row: what the person chose
// must outlast a later change of the default. The row is what the one effect
// is about, and undoing it takes the row away again.
func TestWritingTheDefaultWithNothingStoredStoresARowAndUndoRemovesIt(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	id := recordSetting(t, database, settingsStart, func(ctx context.Context, r *authored.Recorder) error {
		return r.SetConfidenceThreshold(ctx, authored.DefaultConfidenceThreshold)
	})
	if got := effectCount(t, database); got != 1 {
		t.Errorf("%d effects, want 1", got)
	}
	var rows int
	if err := database.Pool().QueryRow(`SELECT count(*) FROM settings`).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("%d rows (%v), want 1", rows, err)
	}
	if err := database.Undo(t.Context(), id, settingsStart.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := database.Pool().QueryRow(`SELECT count(*) FROM settings`).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("%d rows after the undo (%v), want 0", rows, err)
	}
}

func canonicalOf(t *testing.T, database *authored.Database) string {
	t.Helper()
	dump, err := database.CanonicalDump(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return dump
}

func TestUndoingTheFirstWriteAnswersTheDefaultAndRestoresTheDump(t *testing.T) {
	for _, c := range settingCases() {
		t.Run(c.name, func(t *testing.T) {
			database := openDatabase(t, databasePath(t), creationInstant, nil)
			empty := canonicalOf(t, database)
			id := recordSetting(t, database, settingsStart, c.first)
			if canonicalOf(t, database) == empty {
				t.Fatal("the write left the dump as it was; the instrument is not seeing the table")
			}
			if err := database.Undo(t.Context(), id, settingsStart.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			if got := readNow(t, database, c); got != c.defaultValue {
				t.Errorf("after the undo: %v, want the default %v", got, c.defaultValue)
			}
			if got := canonicalOf(t, database); got != empty {
				t.Errorf("the dump after the undo differs from the one before:\n%s\n%s", got, empty)
			}
		})
	}
}

func TestUndoingASecondWriteAnswersTheFirstValue(t *testing.T) {
	for _, c := range settingCases() {
		t.Run(c.name, func(t *testing.T) {
			database := openDatabase(t, databasePath(t), creationInstant, nil)
			recordSetting(t, database, settingsStart, c.first)
			afterFirst := canonicalOf(t, database)
			id := recordSetting(t, database, settingsStart.Add(time.Hour), c.second)
			if got := readNow(t, database, c); got != c.secondValue {
				t.Fatalf("after the second write: %v, want %v", got, c.secondValue)
			}
			if err := database.Undo(t.Context(), id, settingsStart.Add(2*time.Hour)); err != nil {
				t.Fatal(err)
			}
			if got := readNow(t, database, c); got != c.firstValue {
				t.Errorf("after the undo: %v, want %v", got, c.firstValue)
			}
			if got := canonicalOf(t, database); got != afterFirst {
				t.Errorf("the dump after the undo is not the one after the first write")
			}
		})
	}
}

func TestAValueThatIsOutOfRangeIsRefusedByTheDatabaseAndWritesNothing(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	before := canonicalOf(t, database)
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "settings.test", At: settingsStart}
	err := database.Change(t.Context(), facts, func(r *authored.Recorder) error {
		return r.SetConfidenceThreshold(t.Context(), 100)
	})
	if err == nil {
		t.Fatal("a threshold of 100 was stored")
	}
	if effects := effectCount(t, database); effects != 0 || canonicalOf(t, database) != before {
		t.Errorf("a refused write left %d effects or changed the dump", effects)
	}
}
