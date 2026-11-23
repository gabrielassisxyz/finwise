package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// The two writes that can give a month its first line, written here rather
// than read from the router's list, so a write added there shows as a call.
func savesABudget(method string) bool {
	return method == "budget.save" || method == "budget.applyScenario"
}

// newCaseEnvCalling is newCaseEnv over the given ledger, with fn as the
// function the router runs after a budget save.
func newCaseEnvCalling(t *testing.T, c writeCase, ledgerPath string, fn func(context.Context, time.Time)) *writeEnv {
	t.Helper()
	scratch := authoredtest.Open(t, fixtureClock)
	options := []envOption{withLedger(ledgerPath, fixtureNow), withDatabase(scratch.Database),
		func(config *Config) { config.AfterBudgetSaved = fn }}
	if c.handlers != nil {
		options = append(options, func(config *Config) {
			config.Reads = NewRegistry()
			registerChanges(config.Reads)
			c.handlers(config.Reads, scratch)
		})
	}
	env := newTestEnv(t, options...)
	return &writeEnv{testEnv: env, scratch: scratch, cookie: env.unlock()}
}

// postInto is post through a recorder the caller holds, so the function run
// after the commit can see whether anything was answered yet.
func (env *writeEnv) postInto(recorder *httptest.ResponseRecorder, method, arguments string) {
	env.t.Helper()
	domain, name := splitMethod(env.t, method)
	request := httptest.NewRequest(http.MethodPost, "/api/"+domain+"/"+name, strings.NewReader(arguments))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(env.cookie)
	env.handler.ServeHTTP(recorder, request)
}

// committedChange is the change of id as another connection reads it, which
// sees only what was committed.
func committedChange(t *testing.T, env *writeEnv, id string) (authoredtest.Change, bool) {
	t.Helper()
	for _, change := range env.scratch.Changes(t) {
		if strconv.FormatInt(change.ID, 10) == id {
			return change, true
		}
	}
	return authoredtest.Change{}, false
}

func TestABudgetSaveRunsTheFunctionOnceAfterItsCommitAndBeforeItsAnswer(t *testing.T) {
	ledgerPath := newFullLedger(t).Path
	for _, method := range []string{"budget.save", "budget.applyScenario"} {
		t.Run(method, func(t *testing.T) {
			type call struct {
				at       time.Time
				changes  []authoredtest.Change
				answered bool
			}
			var calls []call
			var env *writeEnv
			recorder := httptest.NewRecorder()
			env = newCaseEnvCalling(t, writeCase{}, ledgerPath, func(_ context.Context, at time.Time) {
				calls = append(calls, call{at: at, changes: env.scratch.Changes(t), answered: recorder.Body.Len() > 0})
			})
			budgetState()(t, env.scratch)
			before := len(env.scratch.Changes(t))

			env.postInto(recorder, method, `["2026-04",`+oneIncome+`]`)

			id := changeIDOfAnswer(t, recorder)
			if len(calls) != 1 {
				t.Fatalf("the function ran %d times, want once", len(calls))
			}
			got := calls[0]
			if len(got.changes) != before+1 {
				t.Fatalf("the function saw %d changes committed, %d before the save: it ran before the commit", len(got.changes), before)
			}
			saved := got.changes[before]
			if strconv.FormatInt(saved.ID, 10) != id || saved.Method != method {
				t.Errorf("the function saw change %+v committed, the answer names %s", saved, id)
			}
			if got.answered {
				t.Error("the function ran after the save was answered")
			}
			if want := got.at.UTC().Format("2006-01-02T15:04:05.000Z"); saved.At != want || !got.at.Equal(fixtureNow) {
				t.Errorf("the function was given %s, the change was recorded at %s, the request's instant is %s", want, saved.At, fixtureNow)
			}
		})
	}
}

// A function that fails cannot take the save back nor turn its answer into a
// failure: the change is kept and answered with its id.
func TestABudgetSaveIsAnsweredWithItsChangeWhenTheFunctionPanics(t *testing.T) {
	ledgerPath := newFullLedger(t).Path
	for _, method := range []string{"budget.save", "budget.applyScenario"} {
		t.Run(method, func(t *testing.T) {
			env := newCaseEnvCalling(t, writeCase{}, ledgerPath, func(context.Context, time.Time) {
				panic("the ledger could not be read")
			})
			budgetState()(t, env.scratch)

			response := env.post(method, `["2026-04",`+oneIncome+`]`)

			id := changeIDOfAnswer(t, response)
			if change, committed := committedChange(t, env, id); !committed || change.Method != method {
				t.Errorf("the answer names change %s and the database holds %+v (committed %v)", id, change, committed)
			}
		})
	}
}

// Every write case, once for each of its method, mode and reason: only a budget
// save that was answered 200 runs the function, once. A refused save, every
// other write, an evaluation and the journal's methods run it zero times.
func TestOnlyACommittedBudgetSaveRunsTheFunction(t *testing.T) {
	ledgerPath := newFullLedger(t).Path
	seen := map[string]bool{}
	refusedSaves, committedSaves := 0, 0
	for _, c := range allWriteCases() {
		shape := c.method + " " + c.expects.String() + " " + string(c.reason)
		if seen[shape] {
			continue
		}
		seen[shape] = true
		calls := 0
		env := newCaseEnvCalling(t, c, ledgerPath, func(context.Context, time.Time) { calls++ })
		if c.state != nil {
			c.state(t, env.scratch)
		}
		want := 0
		for _, before := range c.before {
			if response := env.post(before.method, before.arguments); response.Code == http.StatusOK && savesABudget(before.method) {
				want++
			}
		}
		response := env.post(c.method, c.arguments)
		switch {
		case savesABudget(c.method) && response.Code == http.StatusOK:
			want++
			committedSaves++
		case savesABudget(c.method):
			refusedSaves++
		}
		if calls != want {
			t.Errorf("%s: the function ran %d times, want %d (answered %d)", shape, calls, want, response.Code)
		}
	}
	if refusedSaves < 2 || committedSaves < 2 {
		t.Errorf("the cases held %d refused and %d committed budget saves; both writes in both outcomes were not seen", refusedSaves, committedSaves)
	}
}
