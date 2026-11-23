package authored_test

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

func TestUndoRestoresTheDumpFromBeforeTheChange(t *testing.T) {
	seed := all(
		setFacts("t1", authored.Row{"refund": integer(1), "amount": integer(50)}),
		createThing(thing(1, "rent", 500, null)),
		put("tags", tag("t1", "#a")),
		put("parts", part("t1", 1, text("Mercado"), 300)),
	)
	cases := map[string]func(*authored.Recorder) error{
		"optional facts on a key with no row":  setFacts("t2", authored.Row{"amount": integer(9), "line": text("Lazer"), "asking": integer(1)}),
		"optional facts on an existing row":    setFacts("t1", authored.Row{"refund": integer(0), "amount": integer(70)}),
		"optional facts back to the defaults":  setFacts("t1", authored.Row{"refund": integer(0), "amount": integer(0)}),
		"a thing created whole":                createThing(thing(2, "gym", 100, text("monthly"))),
		"a column of a thing created whole":    setThing(1, authored.Row{"amount": integer(650), "note": text("raised")}),
		"a thing removed":                      remove("things", integer(1)),
		"a set row appearing":                  put("tags", tag("t1", "#b")),
		"a set row disappearing":               remove("tags", text("t1"), text("#a")),
		"a set row whose other columns change": put("parts", part("t1", 1, null, 300)),
		"many rows across tables": all(
			setFacts("t1", authored.Row{"refund": integer(0)}), setFacts("t3", authored.Row{"refund": integer(1)}),
			setThing(1, authored.Row{"name": text("rent 2027")}), remove("tags", text("t1"), text("#a")),
			put("tags", tag("t3", "#a")), put("parts", part("t1", 2, text("Casa"), 1)),
		),
	}
	for name, write := range cases {
		t.Run(name, func(t *testing.T) {
			database := openScratch(t, "")
			change(t, database, seed)
			before := dump(t, database)
			id := change(t, database, write)
			if dump(t, database) == before {
				t.Fatal("the change changed nothing, so its undo proves nothing")
			}
			undo(t, database, id)
			assertDump(t, database, before)
		})
	}
}

func TestARowOfOptionalFactsAtItsDefaultsIsNoRowInTheDump(t *testing.T) {
	database := openScratch(t, "")
	before := dump(t, database)
	id := change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	undo(t, database, id)
	assertDump(t, database, before)
	around(t, database, "INSERT INTO facts (id) VALUES ('t1')")
	assertDump(t, database, before)
}

func TestUndoIsRefusedAndChangesNothing(t *testing.T) {
	cases := []struct {
		name  string
		cause authored.StaleCause
		// arrange returns the change to undo, after whatever made it stale.
		arrange func(t *testing.T, database *authored.Database) authored.ChangeID
	}{
		{"an id that does not exist", authored.CauseUnknown, func(t *testing.T, database *authored.Database) authored.ChangeID {
			change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
			return 99
		}},
		{"a change already undone", authored.CauseUndone, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
			undo(t, database, id)
			return id
		}},
		{"a later change in force on a unit", authored.CauseLaterChange, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
			change(t, database, setFacts("t1", authored.Row{"refund": integer(0)}))
			return id
		}},
		{"a later change that put the value back", authored.CauseLaterChange, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := change(t, database, setFacts("t1", authored.Row{"amount": integer(5)}))
			change(t, database, setFacts("t1", authored.Row{"amount": integer(6)}))
			change(t, database, setFacts("t1", authored.Row{"amount": integer(5)}))
			return id
		}},
		{"a later change to a set row", authored.CauseLaterChange, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := change(t, database, put("tags", tag("t1", "#a")))
			change(t, database, remove("tags", text("t1"), text("#a")))
			change(t, database, put("tags", tag("t1", "#a")))
			return id
		}},
		{"a write that went around the journal", authored.CauseOverwritten, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := change(t, database, setFacts("t1", authored.Row{"amount": integer(5)}))
			around(t, database, "UPDATE facts SET amount = 7 WHERE id = 't1'")
			return id
		}},
		{"a set row removed around the journal", authored.CauseOverwritten, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := change(t, database, put("tags", tag("t1", "#a")))
			around(t, database, "DELETE FROM tags")
			return id
		}},
		{"a thing changed around the journal", authored.CauseOverwritten, func(t *testing.T, database *authored.Database) authored.ChangeID {
			id := change(t, database, createThing(thing(1, "rent", 500, null)))
			around(t, database, "UPDATE things SET note = 'x'")
			return id
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := openScratch(t, "")
			id := c.arrange(t, database)
			before := everything(t, database)
			refusedAs(t, database, id, c.cause)
			assertUnchanged(t, database, before)
		})
	}
}

// The stand-in's undo puts back a snapshot of the whole state taken when the
// change began, over anything written since. Here the undo of a change that a
// later one overwrote is refused, and the later edit stays.
func TestAnUndoThatWouldOverwriteALaterChangeIsRefusedWhereTheStandInRestoresItsSnapshot(t *testing.T) {
	database := openScratch(t, "")
	first := change(t, database, setFacts("t1", authored.Row{"line": text("Mercado"), "asking": integer(1)}))
	change(t, database, setFacts("t1", authored.Row{"line": text("Restaurantes")}))
	later := dump(t, database)
	refusedAs(t, database, first, authored.CauseLaterChange)
	assertDump(t, database, later)
}

func TestALaterChangeThatWasUndoneNoLongerRefuses(t *testing.T) {
	database := openScratch(t, "")
	before := dump(t, database)
	first := change(t, database, setFacts("t1", authored.Row{"amount": integer(5)}))
	second := change(t, database, setFacts("t1", authored.Row{"amount": integer(6)}))
	refusedAs(t, database, first, authored.CauseLaterChange)
	undo(t, database, second)
	undo(t, database, first)
	assertDump(t, database, before)
}

// A to B, then C, then back to B: the unit holds the first change's own after,
// and it is still not the first change's to take back.
func TestTheJournalsOrderDecidesNotTheValue(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, setFacts("t1", authored.Row{"amount": integer(1)}))
	a := dump(t, database)
	toB := change(t, database, setFacts("t1", authored.Row{"amount": integer(2)}))
	toC := change(t, database, setFacts("t1", authored.Row{"amount": integer(3)}))
	backToB := change(t, database, setFacts("t1", authored.Row{"amount": integer(2)}))
	before := everything(t, database)
	refusedAs(t, database, toB, authored.CauseLaterChange)
	assertUnchanged(t, database, before)
	undo(t, database, backToB)
	undo(t, database, toC)
	undo(t, database, toB)
	assertDump(t, database, a)
}

func TestAChangeThatWroteAUnitTwiceIsUndoneToItsFirstBefore(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, setFacts("t1", authored.Row{"amount": integer(1)}))
	a := dump(t, database)
	id := change(t, database, all(
		setFacts("t1", authored.Row{"amount": integer(2)}),
		setFacts("t1", authored.Row{"amount": integer(3)}),
	))
	if len(effectsOf(t, database, id)) != 2 {
		t.Fatal("the change should hold both writes")
	}
	undo(t, database, id)
	assertDump(t, database, a)
}

// The first fact on a key made its row without owning it, so it is taken back
// from under a second fact on another column.
func TestAFirstOptionalFactIsTakenBackFromUnderASecond(t *testing.T) {
	database := openScratch(t, "")
	before := dump(t, database)
	first := change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	second := change(t, database, setFacts("t1", authored.Row{"amount": integer(800)}))
	undo(t, database, first)
	assertDump(t, database, `facts {"id":"t1","refund":0,"amount":800,"line":null,"asking":0}`+"\n")
	undo(t, database, second)
	assertDump(t, database, before)
}

func TestTheCreationOfAThingIsRefusedWhileALaterChangeToItsRowIsInForce(t *testing.T) {
	for _, putBack := range []bool{false, true} {
		database := openScratch(t, "")
		before := dump(t, database)
		creation := change(t, database, createThing(thing(1, "rent", 500, null)))
		later := []authored.ChangeID{change(t, database, setThing(1, authored.Row{"amount": integer(600)}))}
		if putBack {
			later = append(later, change(t, database, setThing(1, authored.Row{"amount": integer(500)})))
		}
		state := everything(t, database)
		refusedAs(t, database, creation, authored.CauseLaterChange)
		assertUnchanged(t, database, state)
		for i := len(later) - 1; i >= 0; i-- {
			undo(t, database, later[i])
		}
		undo(t, database, creation)
		assertDump(t, database, before)
	}
}

func TestTwoChangesToDifferentColumnsOfAnExistingRowAreIndependent(t *testing.T) {
	for _, table := range []string{"facts", "things"} {
		for _, undoFirst := range []bool{true, false} {
			database := openScratch(t, "")
			var write func(authored.Row) func(*authored.Recorder) error
			if table == "facts" {
				change(t, database, setFacts("t1", authored.Row{"refund": integer(1), "amount": integer(10)}))
				write = func(values authored.Row) func(*authored.Recorder) error { return setFacts("t1", values) }
			} else {
				change(t, database, createThing(thing(1, "rent", 10, null)))
				write = func(values authored.Row) func(*authored.Recorder) error { return setThing(1, values) }
			}
			first := change(t, database, write(authored.Row{"amount": integer(20)}))
			otherValue := integer(0)
			if table == "things" {
				otherValue = text("x")
			}
			second := change(t, database, write(authored.Row{otherColumn(table): otherValue}))
			if undoFirst {
				undo(t, database, first)
			} else {
				undo(t, database, second)
			}
			wantAmount, wantOther := "10", otherAfter(table)
			if !undoFirst {
				wantAmount, wantOther = "20", otherBefore(table)
			}
			got := columnValues(t, database.Pool(), "SELECT amount || ' ' || coalesce("+otherColumn(table)+", '') FROM "+table)
			if len(got) != 1 || got[0] != wantAmount+" "+wantOther {
				t.Errorf("%s, undo first %v: row holds %v, want %s %s", table, undoFirst, got, wantAmount, wantOther)
			}
		}
	}
}

func otherColumn(table string) string {
	if table == "facts" {
		return "refund"
	}
	return "note"
}

func otherAfter(table string) string {
	if table == "facts" {
		return "0"
	}
	return "x"
}

func otherBefore(table string) string {
	if table == "facts" {
		return "1"
	}
	return ""
}

// A restore computes the whole row and writes it once: restoring line and
// asking one at a time, in either order, breaks the check that spans them, and
// emptying a required column before removing the row breaks NOT NULL.
func TestARestoreWritesEachAttributeRowOnce(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	before := dump(t, database)
	both := change(t, database, setFacts("t1", authored.Row{"line": text("Lazer"), "asking": integer(1)}))
	undo(t, database, both)
	assertDump(t, database, before)

	creation := change(t, database, createThing(thing(1, "rent", 500, text("x"))))
	undo(t, database, creation)
	assertDump(t, database, before)
}

func TestAnUndoLeavesEveryChangeRowAndEffectInPlace(t *testing.T) {
	database := openScratch(t, "")
	first := change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	id := change(t, database, all(setFacts("t2", authored.Row{"amount": integer(3)}), put("tags", tag("t2", "#x"))))
	effects := effectsOf(t, database, id)
	rows := countChanges(t, database.Pool())
	undo(t, database, id)
	if got := countChanges(t, database.Pool()); got != rows {
		t.Errorf("changes holds %d rows after the undo, %d before", got, rows)
	}
	assertEffects(t, effectsOf(t, database, id), effects)
	got := columnValues(t, database.Pool(), "SELECT id || ' ' || coalesce(undone_at, '-') FROM changes ORDER BY id")
	want := []string{first.String() + " -", id.String() + " 2026-12-02T10:00:00.000Z"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("changes = %v, want %v", got, want)
	}
}

func TestAParentAndItsChildrenAreRestoredInEitherOrder(t *testing.T) {
	createParentFirst := all(put("parents", parent(1, "p")), put("children", child(10, 1, "a")), put("children", child(11, 1, "b")))
	createChildrenFirst := all(put("children", child(10, 1, "a")), put("children", child(11, 1, "b")), put("parents", parent(1, "p")))
	removeParentFirst := all(remove("parents", integer(1)), remove("children", integer(10)), remove("children", integer(11)))
	removeChildrenFirst := all(remove("children", integer(10)), remove("children", integer(11)), remove("parents", integer(1)))
	for name, c := range map[string]struct {
		seed, write func(*authored.Recorder) error
	}{
		"created parent first":   {nil, createParentFirst},
		"created children first": {nil, createChildrenFirst},
		"removed parent first":   {createParentFirst, removeParentFirst},
		"removed children first": {createParentFirst, removeChildrenFirst},
	} {
		t.Run(name, func(t *testing.T) {
			database := openScratch(t, "")
			if c.seed != nil {
				change(t, database, c.seed)
			}
			before := dump(t, database)
			id := change(t, database, c.write)
			undo(t, database, id)
			assertDump(t, database, before)
		})
	}
}

func TestForeignKeysAreEnforcedAtTheCommitOfAnUndo(t *testing.T) {
	database := openScratch(t, "")
	parentChange := change(t, database, put("parents", parent(1, "p")))
	change(t, database, put("children", child(10, 1, "a")))
	before := everything(t, database)
	refusedAs(t, database, parentChange, authored.CauseConstraint)
	assertUnchanged(t, database, before)
}

func TestARowRemovedAndItsUniqueValueTakenInOneChangeIsUndone(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, createThing(thing(1, "rent", 500, null)))
	before := dump(t, database)
	id := change(t, database, all(remove("things", integer(1)), createThing(thing(2, "rent", 700, null))))
	undo(t, database, id)
	assertDump(t, database, before)
}

func TestAUniqueValueHandedFromOneRowToAnotherInOneChangeIsUndone(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, all(createThing(thing(1, "rent", 500, null)), createThing(thing(2, "gym", 100, null))))
	before := dump(t, database)
	id := change(t, database, all(
		setThing(1, authored.Row{"name": text("old rent")}),
		setThing(2, authored.Row{"name": text("rent")}),
		setThing(1, authored.Row{"note": text("moved")}),
	))
	undo(t, database, id)
	assertDump(t, database, before)
}

// A row the change never touched took the unique value since: the state the
// undo would bring back is no longer possible.
func TestAnUndoTheDatabaseRefusesIsStaleAndChangesNothing(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, createThing(thing(1, "rent", 500, null)))
	renamed := change(t, database, setThing(1, authored.Row{"name": text("old rent")}))
	change(t, database, createThing(thing(2, "rent", 700, null)))
	before := everything(t, database)
	refusedAs(t, database, renamed, authored.CauseConstraint)
	assertUnchanged(t, database, before)
}

func TestAnUndoWhoseUniqueValueWentToAnotherRowIsStale(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, all(createThing(thing(1, "rent", 500, null)), createThing(thing(2, "gym", 100, null))))
	movedAway := change(t, database, setThing(1, authored.Row{"name": text("old rent")}))
	change(t, database, setThing(2, authored.Row{"name": text("rent")}))
	before := everything(t, database)
	refusedAs(t, database, movedAway, authored.CauseConstraint)
	assertUnchanged(t, database, before)
}

// refuseRefundOn is a registered check that refuses any state where t1 is
// marked as a refund, reading the database inside the undo's transaction.
func refuseRefundOn(enabled *bool) authored.RestoreCheck {
	return func(ctx context.Context, tx *sql.Tx, restored []authored.Unit) (bool, error) {
		if !*enabled || len(restored) == 0 {
			return true, nil
		}
		var refunds int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM facts WHERE id = 't1' AND refund = 1").Scan(&refunds); err != nil {
			return false, err
		}
		return refunds == 0, nil
	}
}

func TestARegisteredCheckRunsBeforeTheCommitOfAnUndo(t *testing.T) {
	enabled := true
	database := openScratch(t, "", refuseRefundOn(&enabled))
	change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	cleared := change(t, database, setFacts("t1", authored.Row{"refund": integer(0)}))
	before := everything(t, database)
	refusedAs(t, database, cleared, authored.CauseCheck)
	assertUnchanged(t, database, before)

	enabled = false
	undo(t, database, cleared)
	if got := columnValues(t, database.Pool(), "SELECT refund FROM facts"); len(got) != 1 || got[0] != "1" {
		t.Errorf("refund = %v after the accepted undo", got)
	}
}

const beyondFloat = 9007199254740993

func TestAnIntegerNoFloatHoldsSurvivesWriteChangeAndUndo(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, setFacts("t1", authored.Row{"amount": integer(beyondFloat)}))
	changed := change(t, database, setFacts("t1", authored.Row{"amount": integer(5)}))
	effects := effectsOf(t, database, changed)
	if len(effects) != 1 || effects[0].before.String != "9007199254740993" || effects[0].after.String != "5" {
		t.Fatalf("effects = %+v", effects)
	}
	undo(t, database, changed)
	var amount int64
	if err := database.Pool().QueryRow("SELECT amount FROM facts WHERE id = 't1'").Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != beyondFloat {
		t.Errorf("amount = %d, want %d", amount, int64(beyondFloat))
	}
	thingChange := change(t, database, createThing(thing(1, "rent", beyondFloat, null)))
	if got := effectsOf(t, database, thingChange)[0].after.String; got != `{"id":1,"name":"rent","amount":9007199254740993,"note":null}` {
		t.Errorf("creation after = %s", got)
	}
	resized := change(t, database, setThing(1, authored.Row{"amount": integer(1)}))
	undo(t, database, resized)
	undo(t, database, thingChange)
	assertDump(t, database, `facts {"id":"t1","refund":0,"amount":9007199254740993,"line":null,"asking":0}`+"\n")
}

func TestOwnedRowsFollowTheirOwner(t *testing.T) {
	t.Run("writing an owned row does not make the owner's change stale", func(t *testing.T) {
		database := openScratch(t, "")
		before := dump(t, database)
		created := changeBy(t, database, "facts.setLine", put("steps", step(1, "t1", "line")))
		change(t, database, put("step_reasons", reason(1, "wrong shop")))
		err := database.Extend(t.Context(), created, "facts.setLine", put("steps", step(2, "t2", "line")))
		if err != nil {
			t.Fatalf("the extension was refused: %v", err)
		}
		undo(t, database, created)
		assertDump(t, database, before)
	})
	t.Run("a recorded removal of an owner records its owned rows and comes back whole", func(t *testing.T) {
		database := openScratch(t, "")
		change(t, database, all(put("steps", step(1, "t1", "line")), put("step_reasons", reason(1, "wrong shop")),
			put("steps", step(2, "t1", "note"))))
		before := dump(t, database)
		removal := change(t, database, remove("steps", integer(1)))
		assertEffects(t, effectsOf(t, database, removal), []effectRow{
			{change: int64(removal), seq: 1, table: "step_reasons", key: "[1]", columnNull: true,
				before: side(`{"step_id":1,"reason":"wrong shop"}`), after: nothing},
			{change: int64(removal), seq: 2, table: "steps", key: "[1]", columnNull: true,
				before: side(`{"id":1,"tx":"t1","label":"line"}`), after: nothing},
		})
		undo(t, database, removal)
		assertDump(t, database, before)
	})
	t.Run("the restore of a part takes the step back with a reason a later change wrote", func(t *testing.T) {
		database := openScratch(t, "")
		before := dump(t, database)
		created := change(t, database, put("steps", step(1, "t1", "line")))
		change(t, database, put("step_reasons", reason(1, "wrong shop")))
		reasoned := dump(t, database)
		if can, err := database.CanRestorePart(t.Context(), created, everyUnit); err != nil || !can {
			t.Fatalf("the read-only check answered %v, %v for the step's own change", can, err)
		}
		restore := change(t, database, func(recorder *authored.Recorder) error {
			return recorder.RestorePart(t.Context(), created, everyUnit)
		})
		assertDump(t, database, before)
		undo(t, database, restore)
		assertDump(t, database, reasoned)
	})
}

func everyUnit(authored.Unit) bool { return true }

func TestOwnedRowsThatHoldTheirOwnerKeepItsCreationFromBeingUndone(t *testing.T) {
	created := all(put("lists", list(1, "casa")), put("entries", entry(1, "pão")))
	t.Run("a row a later change wrote refuses the undo until that change is undone", func(t *testing.T) {
		database := openScratch(t, "")
		before := dump(t, database)
		creation := change(t, database, created)
		added := change(t, database, put("entries", entry(1, "leite")))
		state := everything(t, database)
		refusedAs(t, database, creation, authored.CauseOwnedRowHeld)
		assertUnchanged(t, database, state)
		undo(t, database, added)
		undo(t, database, creation)
		assertDump(t, database, before)
	})
	t.Run("a row written around the journal refuses it too", func(t *testing.T) {
		database := openScratch(t, "")
		creation := change(t, database, created)
		around(t, database, "INSERT INTO entries (list_id, item) VALUES (1, 'sal')")
		state := everything(t, database)
		refusedAs(t, database, creation, authored.CauseOwnedRowHeld)
		assertUnchanged(t, database, state)
	})
	t.Run("rows the undo restores itself do not refuse it, written before the owner or after", func(t *testing.T) {
		database := openScratch(t, "")
		before := dump(t, database)
		creation := change(t, database, all(put("entries", entry(1, "sal")), created, put("entries", entry(1, "leite"))))
		undo(t, database, creation)
		assertDump(t, database, before)
	})
	t.Run("the restore of a part is refused, changes nothing and the read-only check says so first", func(t *testing.T) {
		database := openScratch(t, "")
		creation := change(t, database, created)
		change(t, database, put("entries", entry(1, "leite")))
		if can, err := database.CanRestorePart(t.Context(), creation, everyUnit); err != nil || can {
			t.Errorf("the read-only check answered %v, %v, want false", can, err)
		}
		state := everything(t, database)
		err := database.Change(t.Context(), facts("test.restorePart"), func(recorder *authored.Recorder) error {
			assertStale(t, recorder.RestorePart(t.Context(), creation, everyUnit), authored.CauseOwnedRowHeld)
			return nil
		})
		if err != nil {
			t.Fatalf("the change around the refused restore: %v", err)
		}
		assertUnchanged(t, database, state)
	})
}

func TestAnOwnedTableThatDeclaresNeitherThatItsRowsFollowNorHoldTheirOwnerIsRefused(t *testing.T) {
	if _, err := authored.NewRegistry(scratchTables()); err != nil {
		t.Fatalf("the scratch tables, every owned one declared, were refused: %v", err)
	}
	for i, table := range scratchTables() {
		if table.OwnedBy == nil {
			continue
		}
		for _, onUndo := range []authored.OwnedRows{0, authored.RowsHoldTheirOwner + 1} {
			tables := scratchTables()
			declared := *tables[i].OwnedBy
			declared.OnUndo = onUndo
			tables[i].OwnedBy = &declared
			if _, err := authored.NewRegistry(tables); err == nil {
				t.Errorf("a registry accepted %s declaring %d", table.Name, onUndo)
			}
		}
	}
}

func TestTheAppsOwnedTablesSayWhetherTheirRowsFollowOrHoldTheirOwner(t *testing.T) {
	want := map[string]authored.OwnedRows{
		"budget_version_lines": authored.RowsHoldTheirOwner,
		"history_step_reasons": authored.RowsFollowTheirOwner,
	}
	got := map[string]authored.OwnedRows{}
	for _, table := range authored.AppTables() {
		if table.OwnedBy != nil {
			got[table.Name] = table.OwnedBy.OnUndo
		}
	}
	if !maps.Equal(got, want) {
		t.Errorf("the owned tables declare %v, want %v", got, want)
	}
}

// addingLine is the document with one new line at the end of a group, so the
// lines it already had keep their places and are put back unchanged.
func addingLine(group, name string) func(authored.Document) authored.Document {
	return func(document authored.Document) authored.Document {
		for i := range document {
			if document[i].Key == group {
				document[i].Lines = append(document[i].Lines, authoredtest.Line(name, 7_000))
			}
		}
		return document
	}
}

// saveAdding saves month again with the budget in force for it and one new
// line, and checks that the save recorded that line's row and nothing else.
func (env *budgetEnv) saveAdding(month brday.Month, group, name string) authored.ChangeID {
	env.t.Helper()
	added := env.save(month, addingLine(group, name)(authoredtest.DocumentOf(env.budget(month))))
	if effects := env.scratch.EffectsOf(env.t, int64(added)); len(effects) != 1 || effects[0].Table != "budget_version_lines" {
		env.t.Fatalf("the save that adds %s recorded %+v, want the new line's row alone", name, effects)
	}
	return added
}

// expectHeld undoes change and wants it refused as stale with nothing changed:
// the tables, the journal and the change, which stays in force.
func (env *budgetEnv) expectHeld(label string, change authored.ChangeID) {
	env.t.Helper()
	state, changes, effects := env.scratch.State(env.t), len(env.scratch.Changes(env.t)), env.scratch.EffectCount(env.t)
	var stale *authored.StaleChangeError
	if err := env.undo(change); !errors.As(err, &stale) || stale.Cause != authored.CauseOwnedRowHeld {
		env.t.Errorf("%s: the undo answered %v, want a refusal for a line it does not restore", label, err)
	}
	if env.scratch.State(env.t) != state || len(env.scratch.Changes(env.t)) != changes || env.scratch.EffectCount(env.t) != effects {
		env.t.Errorf("%s: the refused undo changed the database or the journal", label)
	}
	for _, recorded := range env.scratch.Changes(env.t) {
		if recorded.ID == int64(change) && recorded.UndoneAt != "" {
			env.t.Errorf("%s: change %d is marked undone", label, change)
		}
	}
}

func (env *budgetEnv) mustUndo(label string, change authored.ChangeID) {
	env.t.Helper()
	if err := env.undo(change); err != nil {
		env.t.Fatalf("%s: %v", label, err)
	}
}

func lineNames(budget authored.Budget) []string {
	var names []string
	for _, line := range budget.Lines {
		names = append(names, line.Name)
	}
	slices.Sort(names)
	return names
}

// The second save sends the first one's lines back as they were and adds one,
// so its one effect is a row that is no unit of the first save. Removing the
// version the first save created would take that row along.
func TestTheUndoOfTheSaveThatCreatedAVersionIsRefusedWhileALineALaterSaveAddedStands(t *testing.T) {
	env := newBudgetEnvWithNothing(t)
	before := env.scratch.Canonical(t)
	created := env.save("2026-03", authoredtest.InventedBudget())
	added := env.saveAdding("2026-03", "var", "Farmácia")
	both := env.budget("2026-03")
	if len(both.Lines) != 9 {
		t.Fatalf("the two saves left %d lines, want 9", len(both.Lines))
	}
	env.expectHeld("the undo of the first save", created)
	if got := env.budget("2026-03"); !slices.Equal(got.Lines, both.Lines) {
		t.Errorf("after the refusal 2026-03 reads %v, want %v", lineNames(got), lineNames(both))
	}
	for _, recorded := range env.scratch.Changes(t) {
		if recorded.UndoneAt != "" {
			t.Errorf("after the refusal change %d is marked undone", recorded.ID)
		}
	}
	env.mustUndo("the undo of the second save", added)
	env.mustUndo("the undo of the first save once the second is undone", created)
	if got := env.scratch.Canonical(t); got != before {
		t.Errorf("the two undos left another state than the one before both saves:\n%s", got)
	}
}

func TestOfTwoSavesThatEachAddALineTheEarlierIsUndoneAloneAndTheFirstSaveWaitsForBoth(t *testing.T) {
	for _, earlierFirst := range []bool{true, false} {
		env := newBudgetEnvWithNothing(t)
		before := env.scratch.Canonical(t)
		created := env.save("2026-03", authoredtest.InventedBudget())
		pharmacy := env.saveAdding("2026-03", "var", "Farmácia")
		cinema := env.saveAdding("2026-03", "res", "Cinema")
		env.expectHeld("the undo of the first save while both stand", created)
		first, second, other := pharmacy, cinema, "Cinema"
		if !earlierFirst {
			first, second, other = cinema, pharmacy, "Farmácia"
		}
		env.mustUndo("the undo of one added line", first)
		want := []string{other}
		for _, group := range authoredtest.InventedBudget() {
			for _, line := range group.Lines {
				want = append(want, line.Name)
			}
		}
		slices.Sort(want)
		if got := lineNames(env.budget("2026-03")); !slices.Equal(got, want) {
			t.Errorf("earlier first %v: after the undo 2026-03 has %v, want %v", earlierFirst, got, want)
		}
		env.expectHeld("the undo of the first save while one added line stands", created)
		env.mustUndo("the undo of the other added line", second)
		env.mustUndo("the undo of the first save once both are undone", created)
		if got := env.scratch.Canonical(t); got != before {
			t.Errorf("earlier first %v: the undos left another state than the one before the saves:\n%s", earlierFirst, got)
		}
	}
}

func TestATextOfEveryKindSurvivesWriteChangeAndUndo(t *testing.T) {
	database := openScratch(t, "")
	awkward := "quote \" backslash \\ newline \n tab \t bell \x01 accents ção emoji 😀 separator   end"
	change(t, database, createThing(thing(1, "rent", 1, text(awkward))))
	changed := change(t, database, setThing(1, authored.Row{"note": text("plain")}))
	if got := effectsOf(t, database, changed)[0].before.String; got != `"quote \" backslash \\ newline \u000a tab \u0009 bell \u0001 accents ção emoji 😀 separator `+" "+` end"` {
		t.Errorf("before = %s", got)
	}
	undo(t, database, changed)
	var note string
	if err := database.Pool().QueryRow("SELECT note FROM things").Scan(&note); err != nil {
		t.Fatal(err)
	}
	if note != awkward {
		t.Errorf("note = %q", note)
	}
	err := database.Change(t.Context(), facts("test.write"), setThing(1, authored.Row{"note": text("bad \xff byte")}))
	if err == nil {
		t.Error("a text that is not UTF-8 was journaled")
	}
}
