package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// The methods the tests register their handlers on. Each takes two strings,
// which is all the handlers need.
const (
	noteMethod   = "transactions.setNote"
	reasonMethod = "transactions.setReason"
)

func TestARecordedWriteLeavesOneChangeWithItsAuthorMethodInstantAndBrazilDay(t *testing.T) {
	env := newWriteEnv(t, withWrite(noteMethod, setFactAmount("a", 5)))
	// 02:30 UTC is still the evening before in Brazil. The jump outlives the
	// session the environment opened, so a new one is opened at the new instant.
	env.clock.set(time.Date(2026, 3, 1, 2, 30, 0, 0, time.UTC))
	env.cookie = env.unlock()

	response := env.post(noteMethod, `["tx-1","hello"]`)

	id := changeIDOfAnswer(t, response)
	if response.Body.String() != `{"changeId":"`+id+`"}` {
		t.Errorf("body %s", response.Body.String())
	}
	if problem := conformanceProblem(noteMethod, response.Body.Bytes()); problem != "" {
		t.Errorf("the answer does not conform to the manifest: %s", problem)
	}
	changes := env.scratch.Changes(t)
	want := authoredtest.Change{ID: 1, At: "2026-03-01T02:30:00.000Z", Day: "2026-02-28", Author: "person", Method: noteMethod}
	if len(changes) != 1 || changes[0] != want || id != "1" {
		t.Errorf("changes %+v and answer id %s, want the one change %+v", changes, id, want)
	}
	if effects := env.scratch.EffectCount(t); effects != 1 {
		t.Errorf("%d effects, want 1", effects)
	}
}

// The clock is read once per request: the instant a change is recorded at is
// the one its handler's context carries, even when the clock moves meanwhile.
func TestAChangeIsRecordedAtTheInstantItsHandlerSees(t *testing.T) {
	var seen []time.Time
	reading := func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
		seen = append(seen, nowOf(ctx), nowOf(ctx))
		return setFactAmount("a", 5)(ctx, args, writes)
	}
	ticks := 0
	moving := func(config *Config) {
		config.Now = func() time.Time {
			ticks++
			return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(ticks) * time.Minute)
		}
	}
	env := newWriteEnv(t, withWrite(noteMethod, reading), moving)

	changeIDOfAnswer(t, env.post(noteMethod, `["tx-1","hello"]`))

	changes := env.scratch.Changes(t)
	if len(seen) != 2 || !seen[0].Equal(seen[1]) || len(changes) != 1 {
		t.Fatalf("the handler saw the instants %v and there are %d changes", seen, len(changes))
	}
	if want := seen[0].UTC().Format("2006-01-02T15:04:05.000Z"); changes[0].At != want {
		t.Errorf("the change was recorded at %s, the handler saw %s", changes[0].At, want)
	}
}

func TestAWriteThatRecordsNothingAndAnswersNullLeavesNoTrace(t *testing.T) {
	nothingToDo := func(context.Context, []json.RawMessage, Writes) (any, error) { return nil, nil }
	env := newWriteEnv(t, withWrite("transactions.confirmLine", nothingToDo))
	before := env.scratch.State(t)

	response := env.post("transactions.confirmLine", `["tx-1"]`)

	if response.Code != http.StatusOK || response.Body.String() != "null" {
		t.Errorf("status %d body %q, want 200 null", response.Code, response.Body.String())
	}
	if changes, effects := env.scratch.Changes(t), env.scratch.EffectCount(t); len(changes) != 0 || effects != 0 {
		t.Errorf("%d changes and %d effects were added by a write that did nothing", len(changes), effects)
	}
	if after := env.scratch.State(t); after != before {
		t.Errorf("the database changed:\n%s\nwas\n%s", after, before)
	}
}

// Each of these fails after recording an effect and the change's row, which are
// then the whole of what a router that commits too early would keep.
func TestAWriteThatFailsAfterRecordingAnEffectKeepsNothing(t *testing.T) {
	cases := map[string]struct {
		fail   func() (any, error)
		status int
		body   string
	}{
		"returns an error": {func() (any, error) { return nil, errors.New("row 4711 exploded") },
			http.StatusInternalServerError, `{"error":{"kind":"internal"}}`},
		"panics": {func() (any, error) { panic("row 4711 exploded") },
			http.StatusInternalServerError, `{"error":{"kind":"internal"}}`},
		"returns a refusal": {func() (any, error) { return nil, refuse(reasonMonthClosed) },
			http.StatusConflict, `{"error":{"kind":"refused","reason":"month_closed"}}`},
		"returns a result that cannot be encoded": {func() (any, error) { return make(chan int), nil },
			http.StatusInternalServerError, `{"error":{"kind":"internal"}}`},
		"returns arguments it cannot read": {func() (any, error) { return nil, &badArgumentsError{} },
			http.StatusBadRequest, `{"error":{"kind":"bad_request"}}`},
	}
	for label, tc := range cases {
		t.Run(label, func(t *testing.T) {
			env := newWriteEnv(t,
				withWrite(noteMethod, failingAfterAnEffect(tc.fail)),
				withWrite(reasonMethod, setFactAmount("b", 1)))
			before := env.scratch.State(t)

			response := env.post(noteMethod, `["tx-1","hello"]`)

			if response.Code != tc.status || response.Body.String() != tc.body {
				t.Errorf("status %d body %s, want %d %s", response.Code, response.Body.String(), tc.status, tc.body)
			}
			if after := env.scratch.State(t); after != before {
				t.Errorf("a failed write kept something:\n%s\nwas\n%s", after, before)
			}
			if strings.Contains(env.logs.String(), "4711") {
				t.Error("the failure's text reached the log")
			}
			// A write made right after must not wait for a lock the failed one held:
			// a deadline stands in for the five seconds the database would wait.
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			next := env.postWith(ctx, reasonMethod, `["tx-2","hello"]`)
			if id := changeIDOfAnswer(t, next); id != "1" {
				t.Errorf("the write after the failure was change %s, want 1: the failed one left a row", id)
			}
		})
	}
}

// Whichever way a write ends, the ledger view it was given is closed, its
// transaction is over and its connection is back in the pool.
func TestEveryEndingOfAWriteReleasesTheLedgerViewAndTheDatabaseConnection(t *testing.T) {
	endings := map[string]func() (any, error){
		"answered":     func() (any, error) { return map[string]int{"n": 1}, nil },
		"failed":       func() (any, error) { return nil, errors.New("boom") },
		"panicked":     func() (any, error) { panic("boom") },
		"refused":      func() (any, error) { return nil, refuse(reasonMonthClosed) },
		"unencodable":  func() (any, error) { return make(chan int), nil },
		"left nothing": func() (any, error) { return nil, nil },
	}
	for name, end := range endings {
		t.Run(name, func(t *testing.T) {
			var env *writeEnv
			var held Writes
			inUseDuring := -1
			handler := func(ctx context.Context, _ []json.RawMessage, writes Writes) (any, error) {
				held = writes
				if _, err := writes.Ledger.Accounts(ctx); err != nil {
					return nil, err
				}
				if _, err := writes.Recorder.ID(ctx); err != nil {
					return nil, err
				}
				inUseDuring = env.scratch.Database.ConnectionsInUse()
				return end()
			}
			env = newWriteEnv(t, withWrite(noteMethod, handler), func(config *Config) {
				config.Ledger = ledger.New(newFullLedger(t).Path)
			})

			env.post(noteMethod, `["tx-1","hello"]`)

			if inUseDuring != 1 {
				t.Fatalf("during the write the pool lent %d connections, want 1", inUseDuring)
			}
			if inUse := env.scratch.Database.ConnectionsInUse(); inUse != 0 {
				t.Errorf("after the write the pool still lends %d connections", inUse)
			}
			if _, err := held.Ledger.Accounts(t.Context()); !errors.Is(err, ledger.ErrViewClosed) {
				t.Errorf("the write's ledger view answered %v after it ended, want ErrViewClosed", err)
			}
			if _, err := held.Recorder.ID(t.Context()); !errors.Is(err, authored.ErrRecorderClosed) {
				t.Errorf("the write's recorder answered %v after it ended, want ErrRecorderClosed", err)
			}
		})
	}
}

func TestAHandlerThatRefusesForAReasonOutsideTheListIsAnsweredInternal(t *testing.T) {
	env := newWriteEnv(t, withWrite(noteMethod, func(context.Context, []json.RawMessage, Writes) (any, error) {
		return nil, refuse("made_up")
	}))
	expectError(t, "an invented reason", env.post(noteMethod, `["tx-1","hello"]`), http.StatusInternalServerError, "internal")
}

// The foreign key is checked when the change commits, so the handler's own
// writes all succeed and only the commit refuses.
func TestAWriteWhoseCommitFailsAnswersInternalAndKeepsNothing(t *testing.T) {
	orphan := func(ctx context.Context, _ []json.RawMessage, writes Writes) (any, error) {
		row := authored.Row{"id": integer(1), "parent_id": integer(99), "label": text("orphan")}
		if err := writes.Recorder.Put(ctx, authoredtest.Children, row); err != nil {
			return nil, err
		}
		return answerWithChange(ctx, writes)
	}
	env := newWriteEnv(t, withWrite(noteMethod, orphan))
	before := env.scratch.State(t)

	response := env.post(noteMethod, `["tx-1","hello"]`)

	expectError(t, "an orphan child", response, http.StatusInternalServerError, "internal")
	if after := env.scratch.State(t); after != before {
		t.Errorf("a write whose commit failed kept something:\n%s\nwas\n%s", after, before)
	}
}

// The wait is ended by the request's own deadline in place of the five seconds
// of the database's busy timeout; either way the begin fails and the handler
// is never reached.
func TestAWriteThatCannotTakeTheWriteLockAnswersInternalAndRecordsNothing(t *testing.T) {
	calls := 0
	counting := func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
		calls++
		return setFactAmount("a", 5)(ctx, args, writes)
	}
	env := newWriteEnv(t, withWrite(noteMethod, counting))
	before := env.scratch.State(t)
	release := env.scratch.HoldWriteLock(t)

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	response := env.postWith(ctx, noteMethod, `["tx-1","hello"]`)

	expectError(t, "a held lock", response, http.StatusInternalServerError, "internal")
	if waited := time.Since(started); waited > 3*time.Second {
		t.Errorf("the write waited %v", waited)
	}
	if calls != 0 {
		t.Errorf("the handler ran %d times without a transaction", calls)
	}
	release()
	if after := env.scratch.State(t); after != before {
		t.Errorf("a write that never began changed the database:\n%s\nwas\n%s", after, before)
	}
	if id := changeIDOfAnswer(t, env.post(noteMethod, `["tx-1","hello"]`)); id != "1" {
		t.Errorf("the write after the lock was released was change %s, want 1", id)
	}
}

func TestAWriteThatNeedsAMissingSnapshotAnswersLedgerUnavailableAndRecordsNothing(t *testing.T) {
	asksTheLedger := func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
		if _, err := setFactAmount("a", 5)(ctx, args, writes); err != nil {
			return nil, err
		}
		if _, err := writes.Ledger.Accounts(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}
	absent := ledger.New(filepath.Join(t.TempDir(), "pluggy-snapshot.db"))
	env := newWriteEnv(t,
		withWrite(noteMethod, asksTheLedger),
		withWrite(reasonMethod, setFactAmount("b", 1)),
		func(config *Config) { config.Ledger = absent })
	before := env.scratch.State(t)

	expectError(t, "a missing snapshot", env.post(noteMethod, `["tx-1","hello"]`), http.StatusServiceUnavailable, "ledger_unavailable")
	if after := env.scratch.State(t); after != before {
		t.Errorf("a write that could not read the ledger kept something:\n%s\nwas\n%s", after, before)
	}

	// What needs no snapshot works without one.
	changeIDOfAnswer(t, env.post(reasonMethod, `["tx-1","hello"]`))
}

// The second write begins while the first holds the lock inside its handler, so
// it has to queue behind it, and each of them is a change of its own.
func TestTwoWritesAtTheSameTimeAreTwoChanges(t *testing.T) {
	inside, proceed := make(chan struct{}), make(chan struct{})
	var once sync.Once
	holdingTheLock := func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
		answer, err := setFactAmount("first", 1)(ctx, args, writes)
		once.Do(func() {
			close(inside)
			<-proceed
		})
		return answer, err
	}
	env := newWriteEnv(t,
		withWrite(noteMethod, holdingTheLock),
		withWrite(reasonMethod, setFactAmount("second", 2)))

	type outcome struct{ response string }
	answers := make(chan outcome, 2)
	send := func(method string) {
		response := env.post(method, `["tx-1","hello"]`)
		if response.Code != http.StatusOK {
			t.Errorf("%s answered %d %s", method, response.Code, response.Body.String())
		}
		answers <- outcome{response.Body.String()}
	}
	go send(noteMethod)
	<-inside
	go send(reasonMethod)
	time.Sleep(100 * time.Millisecond)
	close(proceed)

	first, second := <-answers, <-answers
	if first.response == second.response {
		t.Errorf("both writes answered %s", first.response)
	}
	if changes := env.scratch.Changes(t); len(changes) != 2 {
		t.Errorf("%d changes, want 2", len(changes))
	}
}

// An evaluation is not stored: it runs over the same read-only views as a read,
// with its arguments checked against the manifest first.
func TestAnEvaluationRunsWithItsArgumentsValidatedAndLeavesEveryTableAsItWas(t *testing.T) {
	calls := 0
	var sawTheDatabase bool
	evaluating := func(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
		calls++
		var err error
		_, sawTheDatabase, err = views.Authored.TrackingStartMonth(ctx)
		return map[string]any{"evaluated": true}, err
	}
	scratch := authoredtest.Open(t, newFakeClock().Now)
	env := newTestEnv(t, withDatabase(scratch.Database), func(config *Config) {
		config.Reads.Register("budget.evaluateScenario", evaluating)
	})
	cookie := env.unlock()
	before := scratch.State(t)

	valid := env.call("/api/budget/evaluateScenario", `["2026-03",[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":8000}]}]]`, cookie)
	if valid.Code != http.StatusOK || valid.Body.String() != `{"evaluated":true}` || calls != 1 {
		t.Errorf("a valid evaluation: status %d body %s after %d calls", valid.Code, valid.Body.String(), calls)
	}
	if !sawTheDatabase {
		t.Error("the evaluation was not given the app's database to read")
	}
	for _, body := range []string{
		`["2026-03",[{"key":"nope","name":"Receita","lines":[]}]]`,
		`["2026-03",[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":"8000"}]}]]`,
		`["2026-03"]`,
		`[202603,[]]`,
	} {
		expectError(t, body, env.call("/api/budget/evaluateScenario", body, cookie), http.StatusBadRequest, "bad_request")
	}
	if calls != 1 {
		t.Errorf("the handler ran %d times, want once: arguments that do not match the manifest reached it", calls)
	}
	if after := scratch.State(t); after != before {
		t.Errorf("an evaluation changed the database:\n%s\nwas\n%s", after, before)
	}
}
