package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

func TestUndoPutsTheTableBackAndAnswersNullWithoutRecordingAChange(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	empty := env.dump()
	id := env.setFact("a", 5)
	if env.dump() == empty {
		t.Fatal("the write changed nothing, so there is nothing to take back")
	}
	recorded := env.scratch.Changes(t)
	effects := env.scratch.EffectCount(t)
	env.clock.Advance(time.Hour)

	response := env.undo(id)

	expectUndone(t, "undo", response)
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("content type %q, want application/json", contentType)
	}
	if problem := conformanceProblem("changes.undo", response.Body.Bytes()); problem != "" {
		t.Errorf("the answer does not conform to the manifest: %s", problem)
	}
	if env.dump() != empty {
		t.Errorf("the table was not put back:\n%s", env.dump())
	}
	after := env.scratch.Changes(t)
	if len(after) != len(recorded) || env.scratch.EffectCount(t) != effects {
		t.Errorf("undo recorded a change of its own: %d changes and %d effects, were %d and %d",
			len(after), env.scratch.EffectCount(t), len(recorded), effects)
	}
	recorded[0].UndoneAt = "2026-01-05T10:00:00.000Z"
	if len(after) != 1 || after[0] != recorded[0] {
		t.Errorf("the change after its undo is %+v, want %+v: marked undone at the request's instant", after, recorded)
	}
}

func TestAWriteThatRecordsNothingButAnswersTheChangeLeavesAnEmptyChangeThatUndoes(t *testing.T) {
	empty := func(ctx context.Context, _ []json.RawMessage, writes Writes) (any, error) {
		return answerWithChange(ctx, writes)
	}
	env := newWriteEnv(t, withWrite(noteMethod, empty))

	id := changeIDOfAnswer(t, env.post(noteMethod, `["tx-1","hello"]`))

	if changes, effects := env.scratch.Changes(t), env.scratch.EffectCount(t); len(changes) != 1 || effects != 0 {
		t.Errorf("%d changes and %d effects, want 1 and 0", len(changes), effects)
	}
	undone := env.post("changes.undo", `["`+id+`"]`)
	if undone.Code != http.StatusOK || undone.Body.String() != "null" {
		t.Errorf("undoing the empty change: status %d body %q, want 200 null", undone.Code, undone.Body.String())
	}
}

// What needs no snapshot works without one, and so does taking it back.
func TestUndoWorksWhileTheSnapshotIsMissing(t *testing.T) {
	absent := ledger.New(filepath.Join(t.TempDir(), "pluggy-snapshot.db"))
	env := newWriteEnv(t, withScratchWriters, func(config *Config) { config.Ledger = absent })
	id := env.setFact("a", 5)

	expectUndone(t, "undo with the snapshot missing", env.undo(id))
}

func TestUndoingTheSameChangeTwiceIsRefused(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	id := env.setFact("a", 5)
	expectUndone(t, "first undo", env.undo(id))
	state := env.scratch.State(t)

	expectRefused(t, "second undo", env.undo(id), "stale_change")

	if env.scratch.State(t) != state {
		t.Error("a refused undo changed the database")
	}
}

// Two tabs undo the same change at once: one takes it back, and the others lost
// the race and are refused.
func TestUndosOfTheSameChangeAtTheSameTimeLeaveOneWinner(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	empty := env.dump()
	id := env.setFact("a", 5)
	const tabs = 4
	start := make(chan struct{})
	answers := make(chan *httptest.ResponseRecorder, tabs)
	for range tabs {
		go func() {
			<-start
			answers <- env.undo(id)
		}()
	}
	close(start)

	won, lost := 0, 0
	for range tabs {
		response := <-answers
		switch {
		case response.Code == http.StatusOK && response.Body.String() == "null":
			won++
		case response.Code == http.StatusConflict && response.Body.String() == `{"error":{"kind":"refused","reason":"stale_change"}}`:
			lost++
		default:
			t.Errorf("an undo answered %d %s", response.Code, response.Body.String())
		}
	}
	if won != 1 || lost != tabs-1 {
		t.Errorf("%d undos won and %d lost, want 1 and %d", won, lost, tabs-1)
	}
	if env.dump() != empty {
		t.Errorf("the table was not put back:\n%s", env.dump())
	}
}

// An id is a handle the API wrote: anything else is the id of no change, even a
// spelling of one that exists.
func TestAnIdTheJournalDoesNotHoldIsStale(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	env.setFact("a", 5)
	state := env.scratch.State(t)
	for _, id := range []string{"999", "nope", "", "01", "+1", " 1", "1 ", "-1", "0", "1.0", "0x1", "9223372036854775808"} {
		expectRefused(t, "undo of "+id, env.undo(id), "stale_change")
	}
	if env.scratch.State(t) != state {
		t.Error("a refused undo changed the database")
	}
}

func TestAnArgumentThatIsNotAStringIsBadRequest(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	env.setFact("a", 5)
	state := env.scratch.State(t)
	for _, body := range []string{`[7]`, `[null]`, `[["1"]]`, `[{"id":"1"}]`, `[true]`, `[]`, `["1","2"]`, `"1"`, `not json`} {
		expectError(t, "undo "+body, env.post("changes.undo", body), http.StatusBadRequest, "bad_request")
	}
	if env.scratch.State(t) != state {
		t.Error("a request that was not understood changed the database")
	}
}

func TestALaterChangeToTheSameUnitMakesTheFirstStale(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	first := env.setFact("a", 5)
	env.setFact("a", 9)
	state := env.scratch.State(t)

	expectRefused(t, "undo of the first", env.undo(first), "stale_change")

	if env.scratch.State(t) != state || !strings.Contains(env.dump(), `"amount":9`) {
		t.Errorf("the later content was not kept:\n%s", env.dump())
	}
}

// The journal's order decides, not the value: after the unit went from B to C
// and back to B, the value is what the first change left, and it is still not
// the first change's to take back.
func TestAValuePutBackByLaterChangesIsStillALaterChange(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	empty := env.dump()
	first := env.setFact("a", 5)
	second := env.setFact("a", 7)
	third := env.setFact("a", 5)
	state := env.scratch.State(t)

	expectRefused(t, "undo of the first", env.undo(first), "stale_change")

	if env.scratch.State(t) != state {
		t.Error("a refused undo changed the database")
	}
	expectUndone(t, "undo of the third", env.undo(third))
	expectUndone(t, "undo of the second", env.undo(second))
	expectUndone(t, "undo of the first, once the later ones are undone", env.undo(first))
	if env.dump() != empty {
		t.Errorf("undoing all three did not put the table back:\n%s", env.dump())
	}
}

func TestUndoingTheLaterChangeFreesTheFirst(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	empty := env.dump()
	first := env.setFact("a", 5)
	second := env.setFact("a", 9)

	expectUndone(t, "undo of the second", env.undo(second))
	if !strings.Contains(env.dump(), `"amount":5`) {
		t.Errorf("undoing the second did not bring the first's content back:\n%s", env.dump())
	}
	expectUndone(t, "undo of the first", env.undo(first))
	if env.dump() != empty {
		t.Errorf("the table was not put back:\n%s", env.dump())
	}
}

func TestAWriteAroundTheRecorderMakesTheLastChangeStaleAndRestoresNothing(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	id := env.setFact("a", 5)
	env.scratch.OverwriteFactAmount(t, "a", 99)
	state := env.scratch.State(t)

	expectRefused(t, "undo after a write around the recorder", env.undo(id), "stale_change")

	if env.scratch.State(t) != state || !strings.Contains(env.dump(), `"amount":99`) {
		t.Errorf("a refused undo restored something:\n%s", env.dump())
	}
}

func TestUndoOverASetTable(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	empty := env.dump()
	added := changeIDOfAnswer(t, env.post(addTagMethod, `["tx-1","mercado"]`))
	removed := changeIDOfAnswer(t, env.post(removeTagMethod, `["tx-1","mercado"]`))
	if env.dump() != empty {
		t.Fatalf("adding and removing the tag left %q", env.dump())
	}

	expectRefused(t, "undo of the addition after the removal", env.undo(added), "stale_change")
	expectUndone(t, "undo of the removal", env.undo(removed))
	if !strings.Contains(env.dump(), `"tag":"mercado"`) {
		t.Errorf("undoing the removal did not bring the tag back:\n%s", env.dump())
	}
	expectUndone(t, "undo of the addition", env.undo(added))
	if env.dump() != empty {
		t.Errorf("the tag is still there:\n%s", env.dump())
	}
}

// Creating a row is itself a unit, so a later change to any column of the row
// makes the change that created it stale.
func TestUndoOverATableOfThingsCreatedWhole(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	empty := env.dump()
	created := changeIDOfAnswer(t, env.post(createThingMethod, `["1","mercado"]`))
	changed := changeIDOfAnswer(t, env.post(setThingMethod, `["1","300"]`))

	expectRefused(t, "undo of the creation after a change to the row", env.undo(created), "stale_change")
	expectUndone(t, "undo of the later change", env.undo(changed))
	expectUndone(t, "undo of the creation", env.undo(created))
	if env.dump() != empty {
		t.Errorf("the row is still there:\n%s", env.dump())
	}
}

// The database can refuse the state an undo would bring back, here a parent
// removed while a child still points at it, and that is as stale as a later
// change would have made it.
func TestAnUndoTheDatabaseRefusesIsStale(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	parent := changeIDOfAnswer(t, env.post(createParentMethod, `["1","pai"]`))
	changeIDOfAnswer(t, env.post(createChildMethod, `["1","1"]`))
	state := env.scratch.State(t)

	expectRefused(t, "undo of a parent with a child", env.undo(parent), "stale_change")

	if env.scratch.State(t) != state {
		t.Error("a refused undo changed the database")
	}
}

func TestUndoThatCannotTakeTheWriteLockAnswersInternalAndRestoresNothing(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	id := env.setFact("a", 5)
	state := env.scratch.State(t)
	release := env.scratch.HoldWriteLock(t)

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	expectError(t, "undo under a held lock", env.postWith(ctx, "changes.undo", `["`+id+`"]`), http.StatusInternalServerError, "internal")

	release()
	if env.scratch.State(t) != state {
		t.Error("an undo that never began changed the database")
	}
	expectUndone(t, "undo once the lock is free", env.undo(id))
}

// The first change after an undo takes the next id: an id is never reused, so
// a stale handle never points at a newer change.
func TestAnUndoneChangesIdIsNotGivenAgain(t *testing.T) {
	env := newWriteEnv(t, withScratchWriters)
	id := env.setFact("a", 5)
	expectUndone(t, "undo", env.undo(id))

	if next := env.setFact("a", 6); next == id {
		t.Errorf("the change after the undo took the id %s again", next)
	}
	changes := env.scratch.Changes(t)
	if len(changes) != 2 || changes[0].UndoneAt == "" || changes[1].UndoneAt != "" {
		t.Errorf("changes %+v, want the first undone and the second not", changes)
	}
}

// The log learns that a write was refused and nothing about why: not the
// reason, not an argument and not the id the argument named.
func TestTheRequestLogOfARefusedWriteHoldsTheKindAndNoReasonArgumentOrId(t *testing.T) {
	refusing := func(context.Context, []json.RawMessage, Writes) (any, error) { return nil, refuse(reasonMonthClosed) }
	env := newWriteEnv(t, withWrite(noteMethod, refusing))
	env.logs.Reset()

	expectRefused(t, "undo of an unknown change", env.undo("424242"), "stale_change")
	expectRefused(t, "a write the app refuses", env.post(noteMethod, `["tx-424242","hello"]`), "month_closed")

	lines := strings.Split(strings.TrimSpace(env.logs.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d log lines for 2 requests:\n%s", len(lines), env.logs.String())
	}
	for i, route := range []string{"/api/changes/undo", "/api/transactions/setNote"} {
		for _, want := range []string{"verb=POST", "route=" + route, "status=409", "duration_ms=", "error_kind=refused"} {
			if !strings.Contains(lines[i], want) {
				t.Errorf("line %d lacks %q: %s", i, want, lines[i])
			}
		}
	}
	for _, reason := range refusalReasons {
		if strings.Contains(env.logs.String(), string(reason)) {
			t.Errorf("the log holds the reason %q", reason)
		}
	}
	for _, secret := range []string{"424242", "hello", "tx-"} {
		if strings.Contains(env.logs.String(), secret) {
			t.Errorf("the log holds %q", secret)
		}
	}
}
