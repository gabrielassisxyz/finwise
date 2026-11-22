package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// writeEnv is a router over a scratch app database with a session already open.
// A test registers the write handlers it needs.
type writeEnv struct {
	*testEnv
	scratch *authoredtest.Scratch
	cookie  *http.Cookie
}

func newWriteEnv(t *testing.T, options ...envOption) *writeEnv {
	t.Helper()
	scratch := authoredtest.Open(t, newFakeClock().Now)
	all := append([]envOption{withDatabase(scratch.Database)}, options...)
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
func failingAfterAnEffect(fail func() (any, error)) WriteHandler {
	return func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
		if _, err := setFactAmount("x", 5)(ctx, args, writes); err != nil {
			return nil, err
		}
		return fail()
	}
}
