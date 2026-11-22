package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// writeEnv is a router over a scratch app database, with the one handler of the
// journal registered and a session already open. A test registers the write
// handlers it needs on top.
type writeEnv struct {
	*testEnv
	scratch *authoredtest.Scratch
	cookie  *http.Cookie
}

func newWriteEnv(t *testing.T, options ...envOption) *writeEnv {
	t.Helper()
	scratch := authoredtest.Open(t, newFakeClock().Now)
	all := append([]envOption{withDatabase(scratch.Database), func(config *Config) { registerChanges(config.Reads) }}, options...)
	env := newTestEnv(t, all...)
	return &writeEnv{testEnv: env, scratch: scratch, cookie: env.unlock()}
}

// post calls a method with the arguments as the JSON array a client posts.
func (env *writeEnv) post(method, arguments string) *httptest.ResponseRecorder {
	env.t.Helper()
	return env.postWith(nil, method, arguments)
}

// postWith is post with a context of the request's own, which a test gives a
// deadline to end a wait the server would otherwise sit out.
func (env *writeEnv) postWith(ctx context.Context, method, arguments string) *httptest.ResponseRecorder {
	env.t.Helper()
	domain, name := splitMethod(env.t, method)
	return env.do(request{path: "/api/" + domain + "/" + name, body: arguments, cookie: env.cookie, ctx: ctx})
}

// changeIDOfAnswer reads the handle out of a ChangeResult.
func changeIDOfAnswer(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status %d body %s, want 200", response.Code, response.Body.String())
	}
	var result struct {
		ChangeID string `json:"changeId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.ChangeID == "" {
		t.Fatalf("body %s is not a ChangeResult: %v", response.Body.String(), err)
	}
	return result.ChangeID
}

// expectRefused checks the exact body of a refusal.
func expectRefused(t *testing.T, label string, response *httptest.ResponseRecorder, reason string) {
	t.Helper()
	want := `{"error":{"kind":"refused","reason":"` + reason + `"}}`
	if response.Code != http.StatusConflict || response.Body.String() != want {
		t.Errorf("%s: status %d body %s, want 409 %s", label, response.Code, response.Body.String(), want)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("%s: content type %q, want application/json", label, contentType)
	}
}

func (clock *fakeClock) set(instant time.Time) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.current = instant
}

func text(value string) authored.Value { return authored.Text(value) }

func integer(value int64) authored.Value { return authored.Integer(value) }

func key(values ...authored.Value) []authored.Value { return values }

// answerWithChange is what a write that must answer a change id does last: it
// asks the recorder for the id, which writes the change's row if no effect has.
func answerWithChange(ctx context.Context, writes Writes) (any, error) {
	id, err := writes.Recorder.ID(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{"changeId": id.String()}, nil
}

// setFactAmount is a write handler that records one effect: the amount of a
// scratch fact, which it answers as a ChangeResult.
func setFactAmount(fact string, amount int64) WriteHandler {
	return func(ctx context.Context, _ []json.RawMessage, writes Writes) (any, error) {
		values := authored.Row{"amount": integer(amount)}
		if err := writes.Recorder.Set(ctx, authoredtest.Facts, key(text(fact)), values); err != nil {
			return nil, err
		}
		return answerWithChange(ctx, writes)
	}
}

// failingAfterAnEffect is a write handler that records an effect and the
// change's row, and then does what fail says.
func failingAfterAnEffect(fail func(context.Context, Writes) (any, error)) WriteHandler {
	return func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
		if _, err := setFactAmount("x", 5)(ctx, args, writes); err != nil {
			return nil, err
		}
		return fail(ctx, writes)
	}
}

// The scratch writers: write methods of two strings that the tests borrow, each
// registered with a handler that records one kind of change on the scratch
// tables. They are how the undo tests make changes of every shape.
const (
	setFactMethod      = "settings.setNickname"
	addTagMethod       = "transactions.addTag"
	removeTagMethod    = "transactions.removeTag"
	createThingMethod  = "subscriptions.setNote"
	setThingMethod     = "subscriptions.setLine"
	createParentMethod = "settings.setDefaultLine"
	createChildMethod  = "settings.toggleLinkedLine"
)

func twoStrings(args []json.RawMessage) (first, second string, err error) {
	if len(args) != 2 {
		return "", "", &badArgumentsError{}
	}
	if err := decodeArgument(args[0], &first); err != nil {
		return "", "", err
	}
	if err := decodeArgument(args[1], &second); err != nil {
		return "", "", err
	}
	return first, second, nil
}

func twoNumbersOf(args []json.RawMessage) (first, second int64, err error) {
	a, b, err := twoStrings(args)
	if err != nil {
		return 0, 0, err
	}
	if first, err = strconv.ParseInt(a, 10, 64); err != nil {
		return 0, 0, &badArgumentsError{}
	}
	if second, err = strconv.ParseInt(b, 10, 64); err != nil {
		return 0, 0, &badArgumentsError{}
	}
	return first, second, nil
}

// scratchWriter turns what a write records into a handler that answers the
// change it made.
func scratchWriter(record func(ctx context.Context, recorder *authored.Recorder, args []json.RawMessage) error) WriteHandler {
	return func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
		if err := record(ctx, writes.Recorder, args); err != nil {
			return nil, err
		}
		return answerWithChange(ctx, writes)
	}
}

// withScratchWriters registers every scratch writer.
func withScratchWriters(config *Config) {
	registry := config.Reads
	registry.RegisterWrite(setFactMethod, scratchWriter(func(ctx context.Context, recorder *authored.Recorder, args []json.RawMessage) error {
		fact, amount, err := twoStrings(args)
		if err != nil {
			return err
		}
		cents, err := strconv.ParseInt(amount, 10, 64)
		if err != nil {
			return &badArgumentsError{}
		}
		return recorder.Set(ctx, authoredtest.Facts, key(text(fact)), authored.Row{"amount": integer(cents)})
	}))
	registry.RegisterWrite(addTagMethod, scratchWriter(func(ctx context.Context, recorder *authored.Recorder, args []json.RawMessage) error {
		tx, tag, err := twoStrings(args)
		if err != nil {
			return err
		}
		return recorder.Put(ctx, authoredtest.Tags, authored.Row{"tx": text(tx), "tag": text(tag)})
	}))
	registry.RegisterWrite(removeTagMethod, scratchWriter(func(ctx context.Context, recorder *authored.Recorder, args []json.RawMessage) error {
		tx, tag, err := twoStrings(args)
		if err != nil {
			return err
		}
		return recorder.Delete(ctx, authoredtest.Tags, key(text(tx), text(tag)))
	}))
	registry.RegisterWrite(createThingMethod, scratchWriter(func(ctx context.Context, recorder *authored.Recorder, args []json.RawMessage) error {
		id, name, err := twoNumbersOrName(args)
		if err != nil {
			return err
		}
		return recorder.Create(ctx, authoredtest.Things, authored.Row{"id": integer(id), "name": text(name), "amount": integer(0)})
	}))
	registry.RegisterWrite(setThingMethod, scratchWriter(func(ctx context.Context, recorder *authored.Recorder, args []json.RawMessage) error {
		id, amount, err := twoNumbersOf(args)
		if err != nil {
			return err
		}
		return recorder.Set(ctx, authoredtest.Things, key(integer(id)), authored.Row{"amount": integer(amount)})
	}))
	registry.RegisterWrite(createParentMethod, scratchWriter(func(ctx context.Context, recorder *authored.Recorder, args []json.RawMessage) error {
		id, name, err := twoNumbersOrName(args)
		if err != nil {
			return err
		}
		return recorder.Put(ctx, authoredtest.Parents, authored.Row{"id": integer(id), "name": text(name)})
	}))
	registry.RegisterWrite(createChildMethod, scratchWriter(func(ctx context.Context, recorder *authored.Recorder, args []json.RawMessage) error {
		id, parent, err := twoNumbersOf(args)
		if err != nil {
			return err
		}
		return recorder.Put(ctx, authoredtest.Children, authored.Row{"id": integer(id), "parent_id": integer(parent), "label": text("child")})
	}))
}

// twoNumbersOrName reads an id and a name.
func twoNumbersOrName(args []json.RawMessage) (int64, string, error) {
	id, name, err := twoStrings(args)
	if err != nil {
		return 0, "", err
	}
	number, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0, "", &badArgumentsError{}
	}
	return number, name, nil
}

func (env *writeEnv) setFact(fact string, cents int) string {
	env.t.Helper()
	return changeIDOfAnswer(env.t, env.post(setFactMethod, `["`+fact+`","`+strconv.Itoa(cents)+`"]`))
}

func (env *writeEnv) undo(id string) *httptest.ResponseRecorder {
	env.t.Helper()
	return env.post("changes.undo", `["`+id+`"]`)
}

// dump is the scratch tables' content in the canonical form the engine's own
// tests compare.
func (env *writeEnv) dump() string {
	env.t.Helper()
	content, err := env.scratch.Database.Dump(env.t.Context())
	if err != nil {
		env.t.Fatal(err)
	}
	return content
}

// expectUndone checks that an undo succeeded: 200 and null.
func expectUndone(t *testing.T, label string, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK || response.Body.String() != "null" {
		t.Errorf("%s: status %d body %q, want 200 null", label, response.Code, response.Body.String())
	}
}
