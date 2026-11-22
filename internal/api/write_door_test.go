package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// The method a scratch journal handler is registered on: it takes the one string
// that a change's handle is, and answers a ChangeResult-like object.
const extendMethod = "transactions.applyLineToEstablishment"

// withJournal registers a journal handler before the router is built.
func withJournal(method string, handler JournalHandler) envOption {
	return func(config *Config) { config.Reads.RegisterJournal(method, handler) }
}

// extendingWith is a journal handler that extends the change named by its
// argument, made by noteMethod, with what write records.
func extendingWith(write func(context.Context, *authored.Recorder) error) JournalHandler {
	return func(ctx context.Context, args []json.RawMessage, journal Journal, _ *ledger.View) (any, error) {
		text, err := stringArgument(args)
		if err != nil {
			return nil, err
		}
		origin, ok := changeIDOf(text)
		if !ok {
			return nil, refuse(reasonStaleChange)
		}
		err = journal.Extend(ctx, origin, noteMethod, func(recorder *authored.Recorder) error {
			return write(ctx, recorder)
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"changed": 1, "changeId": text}, nil
	}
}

func setFactB(ctx context.Context, recorder *authored.Recorder) error {
	return recorder.Set(ctx, authoredtest.Facts, key(text("b")), authored.Row{"amount": integer(7)})
}

func TestAWriteReadsWhatItWroteThroughItsAuthoredViewInOneRequest(t *testing.T) {
	var held Writes
	var read authored.Row
	var found bool
	inUseDuring := -1
	var env *writeEnv
	handler := func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
		held = writes
		if _, err := setFactAmount("a", 5)(ctx, args, writes); err != nil {
			return nil, err
		}
		var err error
		if read, found, err = writes.Authored.JournaledRow(ctx, authoredtest.Facts, key(text("a"))); err != nil {
			return nil, err
		}
		inUseDuring = env.scratch.Database.ConnectionsInUse()
		return answerWithChange(ctx, writes)
	}
	env = newWriteEnv(t, withWrite(noteMethod, handler))

	changeIDOfAnswer(t, env.post(noteMethod, `["tx-1","hello"]`))

	if !found || read["amount"] != integer(5) {
		t.Errorf("the write read back %v found %v, want the amount it wrote", read, found)
	}
	if inUseDuring != 1 {
		t.Errorf("during the write the pool lent %d connections, want 1: the read ran on a transaction of its own", inUseDuring)
	}
	if _, _, err := held.Authored.JournaledRow(t.Context(), authoredtest.Facts, key(text("a"))); !errors.Is(err, authored.ErrViewClosed) {
		t.Errorf("the write's authored view answered %v after it ended, want ErrViewClosed", err)
	}
}

func TestAJournalHandlerExtendsAChangeAndOneUndoTakesBackBoth(t *testing.T) {
	env := newWriteEnv(t,
		withWrite(noteMethod, setFactAmount("a", 5)),
		withJournal(extendMethod, extendingWith(setFactB)))
	empty := env.dump()
	id := changeIDOfAnswer(t, env.post(noteMethod, `["tx-1","hello"]`))

	response := env.post(extendMethod, `["`+id+`"]`)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d body %s", response.Code, response.Body.String())
	}
	if problem := conformanceProblem(extendMethod, response.Body.Bytes()); problem != "" {
		t.Errorf("the answer does not conform to the manifest: %s", problem)
	}
	if changes, effects := env.scratch.Changes(t), env.scratch.EffectCount(t); len(changes) != 1 || effects != 2 || !changes[0].Extended {
		t.Errorf("the journal holds %+v and %d effects, want one extended change with 2", changes, effects)
	}
	expectUndone(t, "undo", env.undo(id))
	if env.dump() != empty {
		t.Errorf("one undo did not take back both:\n%s", env.dump())
	}
}

func TestAnExtensionTheEngineRefusesIsAnsweredStaleChangeAndChangesNothing(t *testing.T) {
	cases := map[string]func(env *writeEnv) (origin string){
		"the origin undone": func(env *writeEnv) string {
			id := changeIDOfAnswer(t, env.post(noteMethod, `["tx-1","hello"]`))
			expectUndone(t, "undo", env.undo(id))
			return id
		},
		"the origin made by another method": func(env *writeEnv) string {
			return changeIDOfAnswer(t, env.post(reasonMethod, `["tx-1","hello"]`))
		},
		"the origin extended already": func(env *writeEnv) string {
			id := changeIDOfAnswer(t, env.post(noteMethod, `["tx-1","hello"]`))
			if response := env.post(extendMethod, `["`+id+`"]`); response.Code != http.StatusOK {
				t.Fatalf("the first extension: %d %s", response.Code, response.Body.String())
			}
			return id
		},
		"a later change in force on a unit the extension wrote": func(env *writeEnv) string {
			id := changeIDOfAnswer(t, env.post(noteMethod, `["tx-1","hello"]`))
			env.post(setFactMethod, `["b","3"]`)
			return id
		},
	}
	for label, prepare := range cases {
		t.Run(label, func(t *testing.T) {
			env := newWriteEnv(t,
				withScratchWriters,
				withWrite(noteMethod, setFactAmount("a", 5)),
				withWrite(reasonMethod, setFactAmount("a", 5)),
				withJournal(extendMethod, extendingWith(setFactB)))
			origin := prepare(env)
			before := env.scratch.State(t)

			expectRefused(t, label, env.post(extendMethod, `["`+origin+`"]`), "stale_change")

			if after := env.scratch.State(t); after != before {
				t.Errorf("a refused extension changed the database:\n%s\nwas\n%s", after, before)
			}
		})
	}
}

func TestAJournalHandlerQueriesTheLedgerViewAndAMissingSnapshotChangesNothing(t *testing.T) {
	asksTheLedger := func(ctx context.Context, _ []json.RawMessage, journal Journal, ledgerView *ledger.View) (any, error) {
		accounts, err := ledgerView.Accounts(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]int{"accounts": len(accounts)}, nil
	}
	t.Run("a snapshot", func(t *testing.T) {
		env := newWriteEnv(t, withJournal(extendMethod, asksTheLedger), func(config *Config) {
			config.Ledger = ledger.New(newFullLedger(t).Path)
		})
		response := env.post(extendMethod, `["1"]`)
		if response.Code != http.StatusOK || response.Body.String() == `{"accounts":0}` {
			t.Errorf("status %d body %s, want the accounts of the snapshot", response.Code, response.Body.String())
		}
	})
	t.Run("a missing snapshot", func(t *testing.T) {
		env := newWriteEnv(t,
			withWrite(noteMethod, setFactAmount("a", 5)),
			withJournal(extendMethod, func(ctx context.Context, args []json.RawMessage, journal Journal, ledgerView *ledger.View) (any, error) {
				if _, err := ledgerView.Accounts(ctx); err != nil {
					return nil, err
				}
				return extendingWith(setFactB)(ctx, args, journal, ledgerView)
			}),
			func(config *Config) { config.Ledger = ledger.New(filepath.Join(t.TempDir(), "pluggy-snapshot.db")) })
		id := changeIDOfAnswer(t, env.post(noteMethod, `["tx-1","hello"]`))
		before := env.scratch.State(t)

		expectError(t, "a missing snapshot", env.post(extendMethod, `["`+id+`"]`), http.StatusServiceUnavailable, "ledger_unavailable")

		if after := env.scratch.State(t); after != before {
			t.Errorf("an extension that could not read the ledger changed the database:\n%s\nwas\n%s", after, before)
		}
	})
}

func TestTheLedgerViewOfAJournalHandlerIsClosedWhateverTheEnding(t *testing.T) {
	endings := map[string]func() (any, error){
		"answered": func() (any, error) { return map[string]int{"n": 1}, nil },
		"failed":   func() (any, error) { return nil, errors.New("boom") },
		"panicked": func() (any, error) { panic("boom") },
		"refused":  func() (any, error) { return nil, refuse(reasonStaleChange) },
	}
	for name, end := range endings {
		t.Run(name, func(t *testing.T) {
			var held *ledger.View
			handler := func(ctx context.Context, _ []json.RawMessage, _ Journal, ledgerView *ledger.View) (any, error) {
				held = ledgerView
				if _, err := ledgerView.Accounts(ctx); err != nil {
					return nil, err
				}
				return end()
			}
			env := newWriteEnv(t, withJournal(extendMethod, handler), func(config *Config) {
				config.Ledger = ledger.New(newFullLedger(t).Path)
			})

			env.post(extendMethod, `["1"]`)

			if held == nil {
				t.Fatal("the handler was not reached")
			}
			if _, err := held.Accounts(t.Context()); !errors.Is(err, ledger.ErrViewClosed) {
				t.Errorf("the ledger view answered %v after the request ended, want ErrViewClosed", err)
			}
		})
	}
}

func TestArgumentsThatDoNotMatchTheManifestNeverReachAJournalHandler(t *testing.T) {
	calls := 0
	checksNothing := func(context.Context, []json.RawMessage, Journal, *ledger.View) (any, error) {
		calls++
		return nil, nil
	}
	env := newWriteEnv(t, withJournal(extendMethod, checksNothing))
	rejected := map[string]string{
		"a number":              `[1]`,
		"a null":                `[null]`,
		"an object":             `[{}]`,
		"a list":                `[["1"]]`,
		"one argument too many": `["1","2"]`,
		"no argument":           `[]`,
		"a bare object":         `{"id":"1"}`,
	}
	for label, body := range rejected {
		expectError(t, label, env.post(extendMethod, body), http.StatusBadRequest, "bad_request")
	}
	if calls != 0 {
		t.Errorf("the handler was reached %d times by arguments the manifest does not allow", calls)
	}
	if response := env.post(extendMethod, `["1"]`); response.Code != http.StatusOK || calls != 1 {
		t.Errorf("a well formed body: status %d after %d calls, want 200 after 1", response.Code, calls)
	}
}

func TestAPanicInsideAnExtensionAnswersInternalKeepsNothingAndFreesTheLock(t *testing.T) {
	panicking := func(ctx context.Context, recorder *authored.Recorder) error {
		if err := setFactB(ctx, recorder); err != nil {
			return err
		}
		panic("row 4711 exploded")
	}
	env := newWriteEnv(t,
		withWrite(noteMethod, setFactAmount("a", 5)),
		withJournal(extendMethod, extendingWith(panicking)))
	id := changeIDOfAnswer(t, env.post(noteMethod, `["tx-1","hello"]`))
	before := env.scratch.State(t)

	expectError(t, "a panic in the extension", env.post(extendMethod, `["`+id+`"]`), http.StatusInternalServerError, "internal")

	if after := env.scratch.State(t); after != before {
		t.Errorf("a panicking extension kept something:\n%s\nwas\n%s", after, before)
	}
	started := time.Now()
	response := env.post(noteMethod, `["tx-2","again"]`)
	if elapsed := time.Since(started); response.Code != http.StatusOK || elapsed > 2*time.Second {
		t.Errorf("the write after the panic: status %d after %v, want 200 within 2s", response.Code, elapsed)
	}
}
