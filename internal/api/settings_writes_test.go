package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// startFromTheInventedState is the state of a write case that changes what a
// person who has used the app already set.
func startFromTheInventedState(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	inventedAuthoredState(t, scratch)
}

// settingsEnv is a router over a ledger and an app's database created by
// authoredtest, with a session open and the clock at the given instant.
type settingsEnv struct {
	*readEnv
	scratch *authoredtest.Scratch
}

func newSettingsEnv(t *testing.T, fixture *ledgertest.Fixture, at time.Time) *settingsEnv {
	t.Helper()
	scratch := authoredtest.Open(t, fixtureClock)
	env := newTestEnv(t, withLedger(fixture.Path, at), withDatabase(scratch.Database))
	return &settingsEnv{readEnv: &readEnv{testEnv: env, cookie: env.unlock()}, scratch: scratch}
}

// change posts a write that must be answered with a change, and answers its id.
func (env *settingsEnv) change(method, arguments string) string {
	env.t.Helper()
	status, body := env.answer(method, arguments)
	id, ok := changeIDIn(body)
	if status != http.StatusOK || !ok {
		env.t.Fatalf("%s %s answered %d %s, want a change", method, arguments, status, body)
	}
	return id
}

func (env *settingsEnv) undo(id string) {
	env.t.Helper()
	if status, body := env.answer("changes.undo", `["`+id+`"]`); status != http.StatusOK || string(body) != "null" {
		env.t.Fatalf("the undo of %s answered %d %s", id, status, body)
	}
}

// overviewField is one field of settings.overview as the server wrote it.
func (env *settingsEnv) overviewField(path ...string) string {
	env.t.Helper()
	return env.fieldOf("settings.overview", `[]`, path...)
}

// fieldOf is one field of a read's answer as the server wrote it; a field under
// a null is null.
func (env *settingsEnv) fieldOf(method, arguments string, path ...string) string {
	env.t.Helper()
	status, body := env.answer(method, arguments)
	if status != http.StatusOK {
		env.t.Fatalf("%s answered %d", method, status)
	}
	raw := json.RawMessage(body)
	for _, field := range path {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			env.t.Fatalf("%s is not an object: %v", strings.Join(path, "."), err)
		}
		if object == nil {
			return "null"
		}
		if raw = object[field]; raw == nil {
			env.t.Fatalf("%s %s has no %s", method, arguments, strings.Join(path, "."))
		}
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		env.t.Fatal(err)
	}
	return compact.String()
}

// expectRefusedAndNothingKept posts arguments a write does not take.
func (env *settingsEnv) expectRefusedAndNothingKept(method, arguments string) {
	env.t.Helper()
	before := env.scratch.State(env.t)
	response := env.call("/api/settings/"+strings.TrimPrefix(method, "settings."), arguments, env.cookie)
	expectError(env.t, method+" "+arguments, response, http.StatusBadRequest, "bad_request")
	if after := env.scratch.State(env.t); after != before {
		env.t.Errorf("%s %s changed the database:\n%s", method, arguments, differences(before, after))
	}
}

func TestTheThresholdIsAnsweredAsTypedRoundedHalfUpAndHeldTo50To99(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	for _, row := range []struct{ typed, answered string }{
		{"10", "50"}, {"100", "99"}, {"72.6", "73"}, {"72.5", "73"}, {"72.4", "72"}, {"1000", "99"},
	} {
		env.change("settings.setThreshold", "["+row.typed+"]")
		if got := env.overviewField("categorization", "threshold"); got != row.answered {
			t.Errorf("setThreshold(%s): the overview answers %s, want %s", row.typed, got, row.answered)
		}
	}
	for _, refused := range []string{`["73"]`, `[null]`, `[2e9]`} {
		env.expectRefusedAndNothingKept("settings.setThreshold", refused)
	}
}

func TestTheMinimumOfExamplesIsAnsweredHeldTo1To50(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	for _, row := range []struct{ typed, answered string }{{"0", "1"}, {"80", "50"}, {"12", "12"}} {
		env.change("settings.setMinExamples", "["+row.typed+"]")
		if got := env.overviewField("categorization", "minExamples"); got != row.answered {
			t.Errorf("setMinExamples(%s): the overview answers %s, want %s", row.typed, got, row.answered)
		}
	}
}

func TestTheClosingKeepsTheModeAndTheHeldDayTogether(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	for _, row := range []struct{ arguments, closing string }{
		{`["manual",31]`, `{"mode":"manual","bankDay":28,"manualDay":28}`},
		{`["manual",0]`, `{"mode":"manual","bankDay":28,"manualDay":1}`},
		{`["auto",12]`, `{"mode":"auto","bankDay":28,"manualDay":12}`},
	} {
		env.change("settings.setClosing", row.arguments)
		if got := env.overviewField("closing"); got != row.closing {
			t.Errorf("setClosing(%s): closing %s, want %s", row.arguments, got, row.closing)
		}
	}
	env.expectRefusedAndNothingKept("settings.setClosing", `["weekly",10]`)
	env.expectRefusedAndNothingKept("settings.setClosing", `["manual","10"]`)
}

func TestUndoingTheClosingPutsBackTheModeAndTheDay(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	before := env.scratch.Canonical(t)
	env.undo(env.change("settings.setClosing", `["manual",10]`))
	if got, want := env.overviewField("closing"), `{"mode":"auto","bankDay":28,"manualDay":28}`; got != want {
		t.Errorf("after the undo the closing is %s, want %s", got, want)
	}
	if after := env.scratch.Canonical(t); after != before {
		t.Errorf("the undo left:\n%s", differences(before, after))
	}
}

func TestTheCardLabelIsKeptTrimmedAndAnEmptyOneStaysEmpty(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	first := env.change("settings.setCardLabel", `["  Virtual "]`)
	if got := env.overviewField("cardLabel"); got != `"Virtual"` {
		t.Errorf("the label is %s, want \"Virtual\"", got)
	}
	blank := env.change("settings.setCardLabel", `["   "]`)
	if got := env.overviewField("cardLabel"); got != `""` {
		t.Errorf("a blank label is %s, want \"\"", got)
	}
	env.undo(blank)
	env.undo(first)
	if got := env.overviewField("cardLabel"); got != `"Físico"` {
		t.Errorf("after both undos the label is %s, want the default", got)
	}
}

// settingsWriteCalls are one call of each of the four writes.
var settingsWriteCalls = []writeCall{
	{"settings.setThreshold", `[60]`},
	{"settings.setMinExamples", `[7]`},
	{"settings.setCardLabel", `["Virtual"]`},
	{"settings.setClosing", `["manual",22]`},
}

func TestEachWriteIsOneChangeOfThePersonOnTheSettingsAlone(t *testing.T) {
	for _, call := range settingsWriteCalls {
		env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
		id := env.change(call.method, call.arguments)
		changes := env.scratch.Changes(t)
		if len(changes) != 1 || changes[0].Author != string(authored.AuthorPerson) || changes[0].Method != call.method || changes[0].ID != changeNumber(t, id) {
			t.Errorf("%s: the journal holds %+v, want change %s by the person and of this method", call.method, changes, id)
		}
		effects := env.scratch.EffectsOf(t, changeNumber(t, id))
		for _, effect := range effects {
			if effect.Table != "settings" {
				t.Errorf("%s: an effect on %s", call.method, effect.Table)
			}
		}
		if len(effects) == 0 {
			t.Errorf("%s: no effect", call.method)
		}
	}
}

// A write that leaves the stored value as it was is still answered with a
// change, as its result cannot be null, and that change has nothing to undo.
func TestAWriteOfTheValueStoredAlreadyIsAnEmptyChangeWhoseUndoChangesNothing(t *testing.T) {
	cases := []writeCase{
		{method: "settings.setThreshold", arguments: `[73]`, before: []writeCall{{"settings.setThreshold", `[72.6]`}}},
		{method: "settings.setClosing", arguments: `["manual",10]`, before: []writeCall{{"settings.setClosing", `["manual",10]`}}},
	}
	for _, c := range cases {
		c.expects = expectsRecorded
		o := runWriteCase(t, c, theRealUndo)
		label := c.method + " " + c.arguments
		if problems := answerProblems(c.method, o); problems != nil {
			t.Fatalf("%s: %v", label, problems)
		}
		if o.changeID == "" || o.changeID == o.originID || o.changesAfter != o.changesBefore+1 {
			t.Errorf("%s: answered change %q after %q, with %d changes before and %d after", label, o.changeID, o.originID, o.changesBefore, o.changesAfter)
		}
		if len(o.effects) != 0 {
			t.Errorf("%s: the change recorded %+v", label, o.effects)
		}
		if o.undoStatus != http.StatusOK || o.undoBody != "null" {
			t.Errorf("%s: the undo answered %d %s", label, o.undoStatus, o.undoBody)
		}
		if problem := restoredProblem(o.canonicalBefore, o.canonicalAfterUndo); problem != "" {
			t.Errorf("%s: %s", label, problem)
		}
	}
}

func TestWithTheSnapshotMissingEachWriteIsKept(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "pluggy-snapshot.db")
	scratch := authoredtest.Open(t, fixtureClock)
	env := newTestEnv(t, withLedger(missing, fixtureNow), withDatabase(scratch.Database))
	stores := &settingsEnv{readEnv: &readEnv{testEnv: env, cookie: env.unlock()}, scratch: scratch}
	for _, call := range settingsWriteCalls {
		stores.change(call.method, call.arguments)
	}
	view := scratch.Database.View()
	defer view.Close()
	if got := readAllSettings(t, view); got != (storedSettings{60, 7, 22, "Virtual", authored.ClosingManual}) {
		t.Errorf("the store holds %+v", got)
	}
	if status, _ := stores.answer("settings.overview", `[]`); status != http.StatusServiceUnavailable {
		t.Errorf("settings.overview answered %d with the snapshot missing, want 503", status)
	}
}

type storedSettings struct {
	threshold, minExamples, manualDay int64
	cardLabel                         string
	mode                              authored.ClosingMode
}

func readAllSettings(t *testing.T, view *authored.View) storedSettings {
	t.Helper()
	chosen, err := chosenSettingsOf(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	return storedSettings{chosen.threshold, chosen.minExamples, chosen.manualDay, chosen.cardLabel, chosen.closingMode}
}

func TestASettingChangedOverHTTPIsAnsweredAfterTheDatabaseIsOpenedAgain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finwise.db")
	ledgerPath := newFullLedger(t).Path
	open := func() *authored.Database {
		database, err := authored.Open(t.Context(), path, authored.Options{Now: fixtureClock})
		if err != nil {
			t.Fatal(err)
		}
		return database
	}
	first := open()
	firstEnv := newTestEnv(t, withLedger(ledgerPath, fixtureNow), withDatabase(first))
	stores := &settingsEnv{readEnv: &readEnv{testEnv: firstEnv, cookie: firstEnv.unlock()}}
	stores.change("settings.setThreshold", `[60]`)
	stores.change("settings.setClosing", `["manual",22]`)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second := open()
	t.Cleanup(func() { second.Close() })
	secondEnv := newTestEnv(t, withLedger(ledgerPath, fixtureNow), withDatabase(second))
	again := &settingsEnv{readEnv: &readEnv{testEnv: secondEnv, cookie: secondEnv.unlock()}}
	if got := again.overviewField("categorization", "threshold"); got != "60" {
		t.Errorf("after the database was opened again the threshold is %s, want 60", got)
	}
	if got := again.overviewField("closing"); got != `{"mode":"manual","bankDay":28,"manualDay":22}` {
		t.Errorf("after the database was opened again the closing is %s", got)
	}
}

// The handlers below the router: each number of the settings is read without a
// float, rounded a half upward as the interface's Math.round rounds it, of
// either sign, and then held to its range. What the JSON does not hold as a
// number, or holds beyond a billion, is refused and nothing is stored.
func TestEachSettingsHandlerRoundsHoldsAndRefusesAsTheInterfaceDoes(t *testing.T) {
	threshold := func(view *authored.View) any { return mustRead(t, view.ConfidenceThreshold) }
	examples := func(view *authored.View) any { return mustRead(t, view.MinExamples) }
	day := func(view *authored.View) any { return mustRead(t, view.ManualClosingDay) }
	label := func(view *authored.View) any { return mustRead(t, view.CardLabel) }
	rows := []struct {
		handler   WriteHandler
		arguments string
		read      func(*authored.View) any
		want      any
	}{
		{setThreshold, `[10]`, threshold, int64(50)}, {setThreshold, `[100]`, threshold, int64(99)},
		{setThreshold, `[72.6]`, threshold, int64(73)}, {setThreshold, `[72.5]`, threshold, int64(73)},
		{setThreshold, `[72.4]`, threshold, int64(72)}, {setThreshold, `[1000]`, threshold, int64(99)},
		{setThreshold, `[-72.5]`, threshold, int64(50)}, {setThreshold, `[49.5]`, threshold, int64(50)},
		{setThreshold, `[98.5]`, threshold, int64(99)}, {setThreshold, `[7.25e1]`, threshold, int64(73)},
		{setThreshold, `[1e9]`, threshold, int64(99)}, {setThreshold, `[-1e9]`, threshold, int64(50)},
		{setMinExamples, `[0]`, examples, int64(1)}, {setMinExamples, `[80]`, examples, int64(50)},
		{setMinExamples, `[12]`, examples, int64(12)}, {setMinExamples, `[12.5]`, examples, int64(13)},
		{setMinExamples, `[-12.5]`, examples, int64(1)}, {setMinExamples, `[0.5]`, examples, int64(1)},
		{setMinExamples, `[50.4]`, examples, int64(50)},
		{setClosing, `["manual",31]`, day, int64(28)}, {setClosing, `["manual",0]`, day, int64(1)},
		{setClosing, `["auto",22.5]`, day, int64(23)}, {setClosing, `["manual",27.5]`, day, int64(28)},
		{setClosing, `["auto",-0.5]`, day, int64(1)},
		{setCardLabel, `["  Virtual "]`, label, "Virtual"}, {setCardLabel, `["   "]`, label, ""},
		{setCardLabel, `["\ufeffVirtual\u00a0\u3000"]`, label, "Virtual"},
		{setCardLabel, `["\u0085Virtual"]`, label, "\u0085Virtual"},
	}
	for _, row := range rows {
		database := authoredtest.Fresh(t, fixtureClock)
		if err := writeBelowTheRouter(t, database, row.handler, row.arguments); err != nil {
			t.Errorf("%s: %v", row.arguments, err)
			continue
		}
		view := database.View()
		if got := row.read(view); got != row.want {
			t.Errorf("%s: stored %v, want %v", row.arguments, got, row.want)
		}
		view.Close()
	}
	for _, refused := range []struct {
		handler   WriteHandler
		arguments string
	}{
		{setThreshold, `["73"]`}, {setThreshold, `[null]`}, {setThreshold, `[true]`}, {setThreshold, `[2e9]`},
		{setThreshold, `[-2e9]`}, {setThreshold, `[1000000000.5]`}, {setMinExamples, `["12"]`}, {setMinExamples, `[1e10]`},
		{setClosing, `["weekly",10]`}, {setClosing, `[null,10]`}, {setClosing, `["manual","10"]`}, {setClosing, `["manual",3e9]`},
		{setCardLabel, `[null]`}, {setCardLabel, `[7]`},
	} {
		database := authoredtest.Fresh(t, fixtureClock)
		before, err := database.CanonicalDump(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		err = writeBelowTheRouter(t, database, refused.handler, refused.arguments)
		if kind, _ := classify(err); kind != kindBadRequest {
			t.Errorf("%s: answered %v as %s, want bad_request", refused.arguments, err, kind)
		}
		if after, err := database.CanonicalDump(t.Context()); err != nil || after != before {
			t.Errorf("%s: the refusal changed the database (%v)", refused.arguments, err)
		}
	}
}

func mustRead[T any](t *testing.T, read func(context.Context) (T, error)) T {
	t.Helper()
	value, err := read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// writeBelowTheRouter runs a handler in a change of its own, as the router
// does, on arguments split the way the router splits them.
func writeBelowTheRouter(t *testing.T, database *authored.Database, handler WriteHandler, arguments string) error {
	t.Helper()
	var args []json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		t.Fatal(err)
	}
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "settings.below-the-router", At: fixtureNow}
	return database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		_, err := handler(t.Context(), args, Writes{Authored: recorder.View(), Recorder: recorder})
		return err
	})
}
