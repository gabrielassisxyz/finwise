package authored_test

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const setLine = "facts.setLine"

func TestAnExtensionAppendsToItsOriginAndOneUndoTakesBackBoth(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, setFacts("t2", authored.Row{"refund": integer(1)}))
	before := dump(t, database)
	origin := changeBy(t, database, setLine, setFacts("t1", authored.Row{"line": text("Lazer"), "asking": integer(1)}))
	var handed authored.ChangeID
	err := database.Extend(t.Context(), origin, setLine, func(recorder *authored.Recorder) error {
		if err := recorder.Set(t.Context(), "facts", key(text("t2")), authored.Row{"line": text("Lazer"), "asking": integer(1)}); err != nil {
			return err
		}
		var err error
		handed, err = recorder.ID(t.Context())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if handed != origin {
		t.Errorf("the extension handed back %s, want the origin's %s", handed, origin)
	}
	effects := effectsOf(t, database, origin)
	if len(effects) != 4 || effects[2].seq != 3 || effects[2].key != `["t2"]` || effects[3].seq != 4 {
		t.Errorf("the origin holds %+v", effects)
	}
	if got := columnValues(t, database.Pool(), "SELECT extended FROM changes WHERE id = "+origin.String()); got[0] != "1" {
		t.Errorf("extended = %v", got)
	}
	if countChanges(t, database.Pool()) != 2 {
		t.Error("the extension wrote a change of its own")
	}
	undo(t, database, origin)
	assertDump(t, database, before)
}

func TestAnExtensionIsRefusedAndChangesNothing(t *testing.T) {
	cases := []struct {
		name    string
		cause   authored.StaleCause
		method  string
		arrange func(t *testing.T, database *authored.Database) authored.ChangeID
	}{
		{"an unknown id", authored.CauseUnknown, setLine, func(t *testing.T, database *authored.Database) authored.ChangeID {
			return 42
		}},
		{"an undone change", authored.CauseUndone, setLine, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := changeBy(t, database, setLine, setFacts("t1", authored.Row{"refund": integer(1)}))
			undo(t, database, id)
			return id
		}},
		{"a change made by another method", authored.CauseOtherMethod, setLine, func(t *testing.T, database *authored.Database) authored.ChangeID {
			return changeBy(t, database, "facts.setRefund", setFacts("t1", authored.Row{"refund": integer(1)}))
		}},
		{"a change already extended", authored.CauseExtended, setLine, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := changeBy(t, database, setLine, setFacts("t1", authored.Row{"refund": integer(1)}))
			if err := database.Extend(t.Context(), id, setLine, setFacts("t2", authored.Row{"refund": integer(1)})); err != nil {
				t.Fatal(err)
			}
			return id
		}},
		{"a later change in force on a unit", authored.CauseLaterChange, setLine, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := changeBy(t, database, setLine, setFacts("t1", authored.Row{"amount": integer(5)}))
			change(t, database, setFacts("t1", authored.Row{"amount": integer(6)}))
			return id
		}},
		{"a later change that put the value back", authored.CauseLaterChange, setLine, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := changeBy(t, database, setLine, setFacts("t1", authored.Row{"amount": integer(5)}))
			change(t, database, setFacts("t1", authored.Row{"amount": integer(6)}))
			change(t, database, setFacts("t1", authored.Row{"amount": integer(5)}))
			return id
		}},
		{"a write that went around the journal", authored.CauseOverwritten, setLine, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := changeBy(t, database, setLine, setFacts("t1", authored.Row{"amount": integer(5)}))
			around(t, database, "UPDATE facts SET amount = 7")
			return id
		}},
		// The extension would write t2 after a later change did, under an id
		// that sorts before it.
		{"an extension onto a unit a later change holds", authored.CauseLaterChange, setLine, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := changeBy(t, database, setLine, setFacts("t1", authored.Row{"refund": integer(1)}))
			change(t, database, setFacts("t2", authored.Row{"refund": integer(1)}))
			return id
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := openScratch(t, "")
			id := c.arrange(t, database)
			before := everything(t, database)
			err := database.Extend(t.Context(), id, c.method, setFacts("t2", authored.Row{"refund": integer(0), "amount": integer(1)}))
			assertStale(t, err, c.cause)
			assertUnchanged(t, database, before)
		})
	}
}

func TestALaterChangeToAnOwnedRowDoesNotStopAnExtension(t *testing.T) {
	database := openScratch(t, "")
	origin := changeBy(t, database, setLine, put("steps", step(1, "t1", "line")))
	change(t, database, put("step_reasons", reason(1, "wrong shop")))
	if err := database.Extend(t.Context(), origin, setLine, setFacts("t1", authored.Row{"refund": integer(1)})); err != nil {
		t.Fatalf("refused: %v", err)
	}
}

// selectKey picks the units of one row of one table.
func selectKey(table, keyText string) func(authored.Unit) bool {
	return func(unit authored.Unit) bool { return unit.Table == table && unit.Key == keyText }
}

func selectColumns(table, keyText string, columns ...string) func(authored.Unit) bool {
	return func(unit authored.Unit) bool {
		return unit.Table == table && unit.Key == keyText && contains(columns, unit.Column)
	}
}

func restorePart(t testing.TB, database *authored.Database, source authored.ChangeID, selected func(authored.Unit) bool) (authored.ChangeID, error) {
	t.Helper()
	var id authored.ChangeID
	err := database.Change(t.Context(), facts("facts.undoHistoryStep"), func(recorder *authored.Recorder) error {
		if err := recorder.RestorePart(t.Context(), source, selected); err != nil {
			return err
		}
		var err error
		id, err = recorder.ID(t.Context())
		return err
	})
	return id, err
}

func canRestore(t testing.TB, database *authored.Database, source authored.ChangeID, selected func(authored.Unit) bool) bool {
	t.Helper()
	can, err := database.CanRestorePart(t.Context(), source, selected)
	if err != nil {
		t.Fatal(err)
	}
	return can
}

// Taking back "marked as refund" leaves a refund line chosen later untouched;
// the step's undo is a change that can itself be undone.
func TestTheInverseOfAPartRestoresOnlyTheSelectedUnits(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, setFacts("t2", authored.Row{"amount": integer(3)}))
	source := change(t, database, all(
		setFacts("t1", authored.Row{"refund": integer(1), "amount": integer(40)}),
		setFacts("t2", authored.Row{"amount": integer(4)}),
	))
	change(t, database, setFacts("t1", authored.Row{"line": text("Mercado"), "asking": integer(1)}))
	beforePart := dump(t, database)
	if !canRestore(t, database, source, selectColumns("facts", `["t1"]`, "refund")) {
		t.Fatal("the read-only check says the refund cannot be taken back")
	}
	part, err := restorePart(t, database, source, selectColumns("facts", `["t1"]`, "refund"))
	if err != nil {
		t.Fatal(err)
	}
	assertDump(t, database, `facts {"id":"t1","refund":0,"amount":40,"line":"Mercado","asking":1}`+"\n"+
		`facts {"id":"t2","refund":0,"amount":4,"line":null,"asking":0}`+"\n")
	assertEffects(t, effectsOf(t, database, part), []effectRow{
		{change: int64(part), seq: 1, table: "facts", key: `["t1"]`, column: "refund", before: side("1"), after: side("0")},
	})
	if got := columnValues(t, database.Pool(), "SELECT coalesce(undone_at, '-') FROM changes WHERE id = "+source.String()); got[0] != "-" {
		t.Error("the source change was marked undone")
	}
	undo(t, database, part)
	assertDump(t, database, beforePart)
}

func TestTheInverseOfAPartIsRefusedOnlyForALaterChangeOnASelectedUnit(t *testing.T) {
	database := openScratch(t, "")
	source := change(t, database, all(
		setFacts("t1", authored.Row{"refund": integer(1)}),
		setFacts("t2", authored.Row{"refund": integer(1)}),
	))
	change(t, database, setFacts("t2", authored.Row{"refund": integer(0)}))
	change(t, database, setFacts("t2", authored.Row{"refund": integer(1)}))
	before := everything(t, database)
	if canRestore(t, database, source, selectKey("facts", `["t2"]`)) {
		t.Error("the read-only check says a unit a later change holds can be taken back")
	}
	_, err := restorePart(t, database, source, selectKey("facts", `["t2"]`))
	assertStale(t, err, authored.CauseLaterChange)
	assertUnchanged(t, database, before)
	if !canRestore(t, database, source, selectKey("facts", `["t1"]`)) {
		t.Error("the read-only check refuses a unit no later change touched")
	}
	if _, err := restorePart(t, database, source, selectKey("facts", `["t1"]`)); err != nil {
		t.Errorf("a later change on an unselected unit refused the part: %v", err)
	}
}

// A refused part leaves the open change usable: the caller's other writes in
// the same change are kept.
func TestARefusedPartLeavesTheRecorderOpen(t *testing.T) {
	database := openScratch(t, "")
	source := change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	change(t, database, setFacts("t1", authored.Row{"refund": integer(0)}))
	err := database.Change(t.Context(), facts("facts.undoHistoryStep"), func(recorder *authored.Recorder) error {
		if err := recorder.Set(t.Context(), "facts", key(text("t9")), authored.Row{"amount": integer(9)}); err != nil {
			return err
		}
		assertStale(t, recorder.RestorePart(t.Context(), source, selectKey("facts", `["t1"]`)), authored.CauseLaterChange)
		return recorder.Set(t.Context(), "facts", key(text("t9")), authored.Row{"refund": integer(1)})
	})
	if err != nil {
		t.Fatal(err)
	}
	assertDump(t, database, `facts {"id":"t9","refund":1,"amount":9,"line":null,"asking":0}`+"\n")
	if got := len(effectsOf(t, database, 3)); got != 2 {
		t.Errorf("the open change holds %d effects, want its two writes", got)
	}
}

func TestTheInverseOfAPartRunsTheRegisteredChecks(t *testing.T) {
	enabled := true
	database := openScratch(t, "", refuseRefundOn(&enabled))
	change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	cleared := change(t, database, setFacts("t1", authored.Row{"refund": integer(0)}))
	before := everything(t, database)
	_, err := restorePart(t, database, cleared, selectKey("facts", `["t1"]`))
	assertStale(t, err, authored.CauseCheck)
	assertUnchanged(t, database, before)
	enabled = false
	if _, err := restorePart(t, database, cleared, selectKey("facts", `["t1"]`)); err != nil {
		t.Fatal(err)
	}
}

// The check runs in the handle's read transaction and takes no write lock: it
// answers while another handle holds one.
func TestTheReadOnlyCheckRunsInsideAReadOnlyTransaction(t *testing.T) {
	path := databasePath(t)
	database, writer := openScratch(t, path), openScratch(t, path)
	source := change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	writing, err := writer.BeginWrite(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer writing.Rollback()
	tx, err := database.BeginRead(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	started := time.Now()
	can, err := database.CanRestorePartIn(t.Context(), tx, source, selectKey("facts", `["t1"]`))
	if err != nil || !can {
		t.Fatalf("can = %v, err = %v", can, err)
	}
	if can, err := database.CanRestorePartIn(t.Context(), tx, 99, selectKey("facts", `["t1"]`)); err != nil || can {
		t.Errorf("an unknown change: can = %v, err = %v", can, err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("the check waited %v for the writer", elapsed)
	}
}

func TestAnUndoThatLostTheRaceToAnotherHandleIsRefused(t *testing.T) {
	path := databasePath(t)
	first, second := openScratch(t, path), openScratch(t, path)
	earlier := change(t, first, setFacts("t1", authored.Row{"amount": integer(5)}))
	change(t, first, setFacts("t1", authored.Row{"amount": integer(6)}))
	before := everything(t, second)
	refusedAs(t, second, earlier, authored.CauseLaterChange)
	assertUnchanged(t, second, before)
}

func TestAnUndoWaitsForAnOpenWriteAndDecidesOnWhatItLeft(t *testing.T) {
	path := databasePath(t)
	first, second := openScratch(t, path), openScratch(t, path)
	earlier := change(t, first, setFacts("t1", authored.Row{"amount": integer(5)}))

	// The write's transaction begins immediate, so it holds the lock from the
	// moment its body runs.
	holding, release, written := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		written <- first.Change(t.Context(), facts("test.write"), func(recorder *authored.Recorder) error {
			if err := recorder.Set(t.Context(), "facts", key(text("t1")), authored.Row{"amount": integer(6)}); err != nil {
				return err
			}
			close(holding)
			<-release
			return nil
		})
	}()
	<-holding

	undone := make(chan error, 1)
	go func() { undone <- second.Undo(t.Context(), earlier, undoInstant) }()
	select {
	case err := <-undone:
		t.Fatalf("the undo decided while the write was open: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-undone:
		assertStale(t, err, authored.CauseLaterChange)
	case <-time.After(4 * time.Second):
		t.Fatal("the undo did not decide after the write committed")
	}
	if got := columnValues(t, second.Pool(), "SELECT amount FROM facts"); got[0] != "6" {
		t.Errorf("amount = %v", got)
	}
}

// The check refuses after the restore has written; the caller carries on and
// commits, and nothing of the refused restore is kept.
func TestAPartRefusedAfterItsRestoreKeepsNothingOfIt(t *testing.T) {
	enabled := true
	database := openScratch(t, "", refuseRefundOn(&enabled))
	change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	cleared := change(t, database, setFacts("t1", authored.Row{"refund": integer(0), "amount": integer(4)}))
	err := database.Change(t.Context(), facts("facts.undoHistoryStep"), func(recorder *authored.Recorder) error {
		assertStale(t, recorder.RestorePart(t.Context(), cleared, selectKey("facts", `["t1"]`)), authored.CauseCheck)
		return recorder.Set(t.Context(), "facts", key(text("t9")), authored.Row{"amount": integer(9)})
	})
	if err != nil {
		t.Fatal(err)
	}
	assertDump(t, database, `facts {"id":"t1","refund":0,"amount":4,"line":null,"asking":0}`+"\n"+
		`facts {"id":"t9","refund":0,"amount":9,"line":null,"asking":0}`+"\n")
	assertEffects(t, effectsOf(t, database, cleared+1), []effectRow{
		{change: int64(cleared + 1), seq: 1, table: "facts", key: `["t9"]`, column: "amount", before: side("0"), after: side("9")},
	})
}

// The database refuses the restored state of a part: a unique value another
// row took since. It is a refusal, not a failure, and the change goes on.
func TestAPartTheDatabaseRefusesIsStaleAndLeavesTheRecorderOpen(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, createThing(thing(1, "rent", 500, null)))
	renamed := change(t, database, setThing(1, authored.Row{"name": text("old rent")}))
	change(t, database, createThing(thing(2, "rent", 700, null)))
	before := dump(t, database)
	err := database.Change(t.Context(), facts("things.undoHistoryStep"), func(recorder *authored.Recorder) error {
		assertStale(t, recorder.RestorePart(t.Context(), renamed, selectKey("things", "[1]")), authored.CauseConstraint)
		return recorder.Set(t.Context(), "facts", key(text("t9")), authored.Row{"amount": integer(9)})
	})
	if err != nil {
		t.Fatalf("the change failed after a refused part: %v", err)
	}
	assertDump(t, database, `facts {"id":"t9","refund":0,"amount":9,"line":null,"asking":0}`+"\n"+before)
}
