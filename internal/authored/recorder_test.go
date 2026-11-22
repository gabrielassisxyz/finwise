package authored_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

func TestARecordedWriteToATableOutsideTheRegistryWritesNothing(t *testing.T) {
	database := openScratch(t, "")
	before := everything(t, database)
	writes := map[string]func(*authored.Recorder) error{
		"set": func(recorder *authored.Recorder) error {
			return recorder.Set(t.Context(), "plain", key(integer(1)), authored.Row{"note": text("x")})
		},
		"put":    put("plain", authored.Row{"id": integer(1), "note": text("x")}),
		"delete": remove("plain", integer(1)),
		"create": func(recorder *authored.Recorder) error {
			return recorder.Create(t.Context(), "nowhere", authored.Row{"id": integer(1)})
		},
	}
	for name, write := range writes {
		var writeErr error
		err := database.Change(t.Context(), facts("test.write"), func(recorder *authored.Recorder) error {
			writeErr = write(recorder)
			return nil
		})
		var notJournaled *authored.NotJournaledError
		if !errors.As(writeErr, &notJournaled) {
			t.Errorf("%s: err = %v, want NotJournaledError", name, writeErr)
		}
		if err != nil {
			t.Errorf("%s: the change failed: %v", name, err)
		}
	}
	assertUnchanged(t, database, before)
}

func TestAnAttributeWriteRecordsOneEffectPerColumnThatChanges(t *testing.T) {
	for _, kind := range []string{"facts", "things"} {
		t.Run(kind, func(t *testing.T) {
			database := openScratch(t, "")
			var rowKey []authored.Value
			var one, two, same authored.Row
			if kind == "facts" {
				rowKey = key(text("t1"))
				change(t, database, setFacts("t1", authored.Row{"refund": integer(1), "amount": integer(500)}))
				one = authored.Row{"amount": integer(700)}
				two = authored.Row{"amount": integer(900), "refund": integer(0)}
				same = authored.Row{"amount": integer(900), "refund": integer(0)}
			} else {
				rowKey = key(integer(1))
				change(t, database, createThing(thing(1, "rent", 500, null)))
				one = authored.Row{"amount": integer(700)}
				two = authored.Row{"note": text(`he said "hi"`), "amount": integer(900)}
				same = authored.Row{"amount": integer(900), "name": text("rent")}
			}
			set := func(values authored.Row) func(*authored.Recorder) error {
				return func(recorder *authored.Recorder) error { return recorder.Set(t.Context(), kind, rowKey, values) }
			}
			keyText := authored.EncodeKey(rowKey...)

			id := change(t, database, set(one))
			assertEffects(t, effectsOf(t, database, id), []effectRow{
				{change: int64(id), seq: 1, table: kind, key: keyText, column: "amount", before: side("500"), after: side("700")},
			})

			id = change(t, database, set(two))
			want := []effectRow{{change: int64(id), seq: 1, table: kind, key: keyText, column: "amount", before: side("700"), after: side("900")}}
			if kind == "facts" {
				want = []effectRow{
					{change: int64(id), seq: 1, table: kind, key: keyText, column: "refund", before: side("1"), after: side("0")},
					{change: int64(id), seq: 2, table: kind, key: keyText, column: "amount", before: side("700"), after: side("900")},
				}
			} else {
				want = append(want, effectRow{change: int64(id), seq: 2, table: kind, key: keyText, column: "note", before: side("null"), after: side(`"he said \"hi\""`)})
			}
			assertEffects(t, effectsOf(t, database, id), want)

			id = change(t, database, set(same))
			assertEffects(t, effectsOf(t, database, id), nil)
		})
	}
}

func TestTheFirstFactOnAKeyRecordsThatColumnAndNoOwnershipOfTheRow(t *testing.T) {
	database := openScratch(t, "")
	id := change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	assertEffects(t, effectsOf(t, database, id), []effectRow{
		{change: int64(id), seq: 1, table: "facts", key: `["t1"]`, column: "refund", before: side("0"), after: side("1")},
	})
}

func TestCreatingAThingWholeIsRecordedAsTheCreationOfItsRow(t *testing.T) {
	database := openScratch(t, "")
	id := change(t, database, createThing(thing(7, "gym", 9000, null)))
	assertEffects(t, effectsOf(t, database, id), []effectRow{{
		change: int64(id), seq: 1, table: "things", key: "[7]", columnNull: true,
		before: nothing, after: side(`{"id":7,"name":"gym","amount":9000,"note":null}`),
	}})
}

func TestASetRowAppearingAndDisappearingIsOneEffectEach(t *testing.T) {
	database := openScratch(t, "")
	appeared := change(t, database, put("tags", tag("t1", "#trip")))
	assertEffects(t, effectsOf(t, database, appeared), []effectRow{{
		change: int64(appeared), seq: 1, table: "tags", key: `["t1","#trip"]`, columnNull: true,
		before: nothing, after: side(`{"tx":"t1","tag":"#trip"}`),
	}})
	again := change(t, database, put("tags", tag("t1", "#trip")))
	assertEffects(t, effectsOf(t, database, again), nil)
	gone := change(t, database, remove("tags", text("t1"), text("#trip")))
	assertEffects(t, effectsOf(t, database, gone), []effectRow{{
		change: int64(gone), seq: 1, table: "tags", key: `["t1","#trip"]`, columnNull: true,
		before: side(`{"tx":"t1","tag":"#trip"}`), after: nothing,
	}})
}

func TestASetRowWhoseOtherColumnsChangeIsReplacedInOneEffect(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, put("parts", part("t1", 1, text("Mercado"), 300)))
	id := change(t, database, put("parts", part("t1", 1, null, 300)))
	assertEffects(t, effectsOf(t, database, id), []effectRow{{
		change: int64(id), seq: 1, table: "parts", key: `["t1",1]`, columnNull: true,
		before: side(`{"tx":"t1","position":1,"line":"Mercado","amount":300}`),
		after:  side(`{"tx":"t1","position":1,"line":null,"amount":300}`),
	}})
}

func TestAChangeStoresItsFactsAndAnswersItsRowsID(t *testing.T) {
	database := openScratch(t, "")
	var id authored.ChangeID
	err := database.Change(t.Context(), authored.ChangeFacts{
		Author: authored.AuthorPerson, Method: "transactions.setLine", At: time.Date(2026, 12, 1, 1, 30, 0, 0, time.UTC),
	}, func(recorder *authored.Recorder) error {
		var err error
		id, err = recorder.ID(t.Context())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		id                      int64
		at, day, author, method string
		extended                int
		undone                  *string
	}
	if err := database.Pool().QueryRow("SELECT id, at, day, author, method, extended, undone_at FROM changes").
		Scan(&stored.id, &stored.at, &stored.day, &stored.author, &stored.method, &stored.extended, &stored.undone); err != nil {
		t.Fatal(err)
	}
	if stored.id != int64(id) || stored.at != "2026-12-01T01:30:00.000Z" || stored.day != "2026-11-30" ||
		stored.author != "person" || stored.method != "transactions.setLine" || stored.extended != 0 || stored.undone != nil {
		t.Errorf("stored %+v for id %s", stored, id)
	}
	if id.String() != "1" {
		t.Errorf("the handle is %q, want the decimal id", id.String())
	}
}

func TestAChangeByAnAuthorOutsideTheThreeIsRefused(t *testing.T) {
	database := openScratch(t, "")
	ran := false
	for _, author := range []authored.Author{"admin", ""} {
		err := database.Change(t.Context(), authored.ChangeFacts{Author: author, Method: "m", At: changeInstant},
			func(*authored.Recorder) error { ran = true; return nil })
		if !errors.Is(err, authored.ErrInvalidAuthor) {
			t.Errorf("author %q: err = %v", author, err)
		}
	}
	if ran {
		t.Error("a write ran under an author outside the three")
	}
	for _, author := range []authored.Author{authored.AuthorPerson, authored.AuthorRule, authored.AuthorModel} {
		if err := database.Change(t.Context(), authored.ChangeFacts{Author: author, Method: "m", At: changeInstant},
			func(*authored.Recorder) error { return nil }); err != nil {
			t.Errorf("author %q was refused: %v", author, err)
		}
	}
}

// The three endings of a change: no trace, an empty change, a change with effects.
func TestTheRowOfAChangeIsWrittenAtItsFirstEffectOrIDAndOnce(t *testing.T) {
	database := openScratch(t, "")

	if err := database.Change(t.Context(), facts("test.nothing"), func(*authored.Recorder) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := database.Change(t.Context(), facts("test.same"), setFacts("t1", authored.Row{"refund": integer(0)})); err != nil {
		t.Fatal(err)
	}
	if got := journal(t, database); got != "" {
		t.Fatalf("changes that recorded nothing and asked for no id left:\n%s", got)
	}

	empty := change(t, database, func(*authored.Recorder) error { return nil })
	if empty != 1 {
		t.Errorf("the first change kept got id %s, want 1: an earlier one that left no trace took an id", empty)
	}
	if got, want := journal(t, database), "change 1 2026-12-01T01:30:00.000Z 2026-11-30 person test.write 0 -"; got != want {
		t.Errorf("an empty change left:\n%s\nwant:\n%s", got, want)
	}

	var first, again authored.ChangeID
	err := database.Change(t.Context(), facts("test.both"), func(recorder *authored.Recorder) error {
		var err error
		if first, err = recorder.ID(t.Context()); err != nil {
			return err
		}
		if err := recorder.Set(t.Context(), "facts", key(text("t1")), authored.Row{"refund": integer(1)}); err != nil {
			return err
		}
		again, err = recorder.ID(t.Context())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	effectFirst := change(t, database, func(recorder *authored.Recorder) error {
		return recorder.Set(t.Context(), "facts", key(text("t1")), authored.Row{"refund": integer(0)})
	})
	if first != 2 || again != 2 || effectFirst != 3 || countChanges(t, database.Pool()) != 3 {
		t.Errorf("ids %s %s %s over %d rows, want 2 2 3 over 3", first, again, effectFirst, countChanges(t, database.Pool()))
	}
	if len(effectsOf(t, database, 2)) != 1 || len(effectsOf(t, database, 3)) != 1 {
		t.Error("each change should hold its one effect")
	}
}

func TestTheUndoOfAnEmptyChangeChangesNothingAndHappensOnce(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	empty := change(t, database, func(*authored.Recorder) error { return nil })
	before := dump(t, database)
	undo(t, database, empty)
	assertDump(t, database, before)
	var undoneAt string
	if err := database.Pool().QueryRow("SELECT undone_at FROM changes WHERE id = ?", int64(empty)).Scan(&undoneAt); err != nil {
		t.Fatal(err)
	}
	if undoneAt != "2026-12-02T10:00:00.000Z" {
		t.Errorf("undone_at = %q", undoneAt)
	}
	refusedAs(t, database, empty, authored.CauseUndone)
}

func TestAChangeThatFailsAfterTwoWritesLeavesEverythingAsItWas(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, all(setFacts("t1", authored.Row{"refund": integer(1)}), createThing(thing(1, "rent", 500, null))))
	before := everything(t, database)
	failure := errors.New("the third step failed")
	err := database.Change(t.Context(), facts("test.fails"), all(
		setFacts("t1", authored.Row{"amount": integer(9)}),
		put("tags", tag("t1", "#x")),
		func(recorder *authored.Recorder) error {
			return recorder.ExecPlain(t.Context(), "INSERT INTO plain (id, note) VALUES (1, 'beside')")
		},
		func(*authored.Recorder) error { return failure },
	))
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v", err)
	}
	assertUnchanged(t, database, before)
}

// A write the database refuses inside a recorded write fails the change even
// when the handler swallows the error.
func TestAChangeWhoseRecordedWriteFailedIsRolledBackWhateverTheHandlerReturns(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, createThing(thing(1, "rent", 500, null)))
	before := everything(t, database)
	err := database.Change(t.Context(), facts("test.swallows"), func(recorder *authored.Recorder) error {
		_ = recorder.Set(t.Context(), "facts", key(text("t1")), authored.Row{"refund": integer(1)})
		_ = recorder.Create(t.Context(), "things", thing(2, "rent", 1, null))
		return nil
	})
	var failed *authored.TransactionError
	if !errors.As(err, &failed) {
		t.Fatalf("err = %v, want the failed write's TransactionError", err)
	}
	assertUnchanged(t, database, before)
}

func TestARecordedAndAPlainWriteCommitTogether(t *testing.T) {
	database := openScratch(t, "")
	id := change(t, database, all(
		setFacts("t1", authored.Row{"refund": integer(1)}),
		func(recorder *authored.Recorder) error {
			return recorder.ExecPlain(t.Context(), "INSERT INTO plain (id, note) VALUES (1, 'beside')")
		},
	))
	if got := columnValues(t, database.Pool(), "SELECT note FROM plain"); !reflect.DeepEqual(got, []string{"beside"}) {
		t.Errorf("plain holds %v", got)
	}
	if len(effectsOf(t, database, id)) != 1 {
		t.Error("the plain write was recorded as an effect, or the recorded one was lost")
	}
	undo(t, database, id)
	if got := columnValues(t, database.Pool(), "SELECT note FROM plain"); !reflect.DeepEqual(got, []string{"beside"}) {
		t.Errorf("undoing the change took the plain write with it: %v", got)
	}
}

func TestARecorderCannotBeUsedAfterItsTransaction(t *testing.T) {
	database := openScratch(t, "")
	var kept *authored.Recorder
	change(t, database, func(recorder *authored.Recorder) error { kept = recorder; return nil })
	if err := kept.Set(t.Context(), "facts", key(text("t1")), authored.Row{"refund": integer(1)}); !errors.Is(err, authored.ErrRecorderClosed) {
		t.Errorf("err = %v", err)
	}
}

func assertEffects(t testing.TB, got, want []effectRow) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effects:\n%+v\nwant:\n%+v", got, want)
	}
}
