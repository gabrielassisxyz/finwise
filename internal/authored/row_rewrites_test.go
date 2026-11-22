package authored_test

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// One change writes one row of a table of things created whole several times,
// as a whole and column by column. Its undo must bring back the row from
// before the change, whichever order the writes came in.
var rowRewrites = []struct {
	name  string
	row   int64
	write func(*authored.Recorder) error
}{
	{"set a column then delete the row", 1, all(
		setThing(1, authored.Row{"amount": integer(600)}),
		remove("things", integer(1)),
	)},
	{"create the row then set a column", 3, all(
		createThing(thing(3, "car", 50, null)),
		setThing(3, authored.Row{"note": text("financed")}),
	)},
	{"set a column, delete, create again with other values", 1, all(
		setThing(1, authored.Row{"amount": integer(600)}),
		remove("things", integer(1)),
		createThing(thing(1, "rent 2027", 900, text("b"))),
	)},
	{"delete, create again, set a column", 1, all(
		remove("things", integer(1)),
		createThing(thing(1, "rent 2027", 900, null)),
		setThing(1, authored.Row{"amount": integer(950)}),
	)},
	{"set a column, delete, create again, set it again", 1, all(
		setThing(1, authored.Row{"amount": integer(600)}),
		remove("things", integer(1)),
		createThing(thing(1, "rent 2027", 700, null)),
		setThing(1, authored.Row{"amount": integer(800), "note": text("c")}),
	)},
}

var seedTwoThings = all(createThing(thing(1, "rent", 500, text("a"))), createThing(thing(2, "gym", 100, null)))

// laterOnAnotherRow is a change in between that touches another row, so the
// undo is still allowed.
var laterOnAnotherRow = setThing(2, authored.Row{"amount": integer(111)})

// expectedDump is the dump of a database that received the seed and, when
// asked, the later change alone: what an undo of the rewrite must leave.
func expectedDump(t *testing.T, withLater bool) string {
	t.Helper()
	database := openScratch(t, "")
	change(t, database, seedTwoThings)
	if withLater {
		change(t, database, laterOnAnotherRow)
	}
	return dump(t, database)
}

func TestAChangeThatWritesOneRowSeveralTimesIsUndoneToTheRowBeforeIt(t *testing.T) {
	for _, rewrite := range rowRewrites {
		for _, withLater := range []bool{false, true} {
			t.Run(rewriteName(rewrite.name, withLater), func(t *testing.T) {
				database := openScratch(t, "")
				change(t, database, seedTwoThings)
				id := change(t, database, rewrite.write)
				if withLater {
					change(t, database, laterOnAnotherRow)
				}
				undo(t, database, id)
				assertDump(t, database, expectedDump(t, withLater))
			})
		}
	}
}

// The same writes taken back as a part, inside a new change, and that change
// undone in turn.
func TestAPartOverARowWrittenSeveralTimesRestoresTheRowBeforeIt(t *testing.T) {
	for _, rewrite := range rowRewrites {
		for _, withLater := range []bool{false, true} {
			t.Run(rewriteName(rewrite.name, withLater), func(t *testing.T) {
				database := openScratch(t, "")
				change(t, database, seedTwoThings)
				id := change(t, database, rewrite.write)
				if withLater {
					change(t, database, laterOnAnotherRow)
				}
				rewritten := dump(t, database)
				rowKey := authored.EncodeKey(integer(rewrite.row))
				if !canRestore(t, database, id, selectKey("things", rowKey)) {
					t.Fatal("the read-only check says the row cannot be taken back")
				}
				part, err := restorePart(t, database, id, selectKey("things", rowKey))
				if err != nil {
					t.Fatalf("the part was refused: %v", err)
				}
				assertDump(t, database, expectedDump(t, withLater))
				undo(t, database, part)
				assertDump(t, database, rewritten)
			})
		}
	}
}

func rewriteName(name string, withLater bool) string {
	if withLater {
		return name + ", with a later change to another row"
	}
	return name
}
