package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/contract"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// testEnv is one handler with an injected clock, a counting PIN check and a
// log buffer, so a test can see what the server did without a real clock.
type testEnv struct {
	t        *testing.T
	handler  http.Handler
	clock    *fakeClock
	reads    *Registry
	logs     *bytes.Buffer
	pinTries atomic.Int32
	methods  []contract.Method
}

type envOption func(*Config)

// newTestEnv builds the router. Given a ledger and the app's database, it
// records the snapshot's run as taken in first, as the server's ingest would
// have, so a write's lookup reads a snapshot taken in.
func newTestEnv(t *testing.T, options ...envOption) *testEnv {
	t.Helper()
	return buildTestEnv(t, true, options)
}

// newTestEnvNotTakenIn is newTestEnv leaving the database as it is, for a test
// of what a write does while the snapshot it reads is not the one taken in.
func newTestEnvNotTakenIn(t *testing.T, options ...envOption) *testEnv {
	t.Helper()
	return buildTestEnv(t, false, options)
}

func buildTestEnv(t *testing.T, takenIn bool, options []envOption) *testEnv {
	t.Helper()
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	env := &testEnv{t: t, clock: newFakeClock(), reads: NewRegistry(), logs: &bytes.Buffer{}, methods: methods}
	config := Config{
		Methods: methods,
		Reads:   env.reads,
		Now:     env.clock.Now,
		Logger:  slog.New(slog.NewTextHandler(env.logs, nil)),
		VerifyPIN: func(attempt string) bool {
			env.pinTries.Add(1)
			return attempt == testPIN
		},
	}
	for _, option := range options {
		option(&config)
	}
	if takenIn {
		takeTheSnapshotIn(t, config)
	}
	env.handler, err = NewHandler(config)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return env
}

type request struct {
	verb    string
	path    string
	body    string
	headers map[string]string
	cookie  *http.Cookie
	// ctx is the request's context; a test gives one a deadline to end a wait
	// the server would otherwise sit out.
	ctx context.Context
}

func (env *testEnv) do(req request) *httptest.ResponseRecorder {
	env.t.Helper()
	if req.verb == "" {
		req.verb = http.MethodPost
	}
	httpRequest := httptest.NewRequest(req.verb, req.path, strings.NewReader(req.body))
	if req.headers == nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	for name, value := range req.headers {
		httpRequest.Header.Set(name, value)
	}
	if req.cookie != nil {
		httpRequest.AddCookie(req.cookie)
	}
	if req.ctx != nil {
		httpRequest = httpRequest.WithContext(req.ctx)
	}
	recorder := httptest.NewRecorder()
	env.handler.ServeHTTP(recorder, httpRequest)
	return recorder
}

func (env *testEnv) call(path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	env.t.Helper()
	return env.do(request{path: path, body: body, cookie: cookie})
}

// unlock signs in with the right PIN and returns the cookie it was given.
func (env *testEnv) unlock() *http.Cookie {
	env.t.Helper()
	response := env.call("/api/session/unlock", `["`+testPIN+`"]`, nil)
	for _, cookie := range response.Result().Cookies() {
		if cookie.MaxAge > 0 {
			return &http.Cookie{Name: cookie.Name, Value: cookie.Value}
		}
	}
	env.t.Fatalf("unlock gave no cookie: status %d", response.Code)
	return nil
}

func errorKindOf(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body is not an error envelope: status %d", response.Code)
	}
	return envelope.Error.Kind
}

func expectError(t *testing.T, label string, response *httptest.ResponseRecorder, status int, kind string) {
	t.Helper()
	if response.Code != status || errorKindOf(t, response) != kind {
		t.Errorf("%s: status %d kind %q, want %d %q", label, response.Code, errorKindOf(t, response), status, kind)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("%s: content type %q, want application/json", label, contentType)
	}
}

func decodeBody(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("body is not a JSON object: %v", err)
	}
	return decoded
}

func hours(n int) time.Duration { return time.Duration(n) * time.Hour }

func methodsOfClass(env *testEnv, classes ...contract.Class) []contract.Method {
	var picked []contract.Method
	for _, method := range env.methods {
		for _, class := range classes {
			if method.Class == class {
				picked = append(picked, method)
			}
		}
	}
	return picked
}

// withRead registers a read handler before the router is built.
func withRead(method string, handler ReadHandler) envOption {
	return func(config *Config) { config.Reads.Register(method, handler) }
}

// withWrite registers a write handler before the router is built.
func withWrite(method string, handler WriteHandler) envOption {
	return func(config *Config) { config.Reads.RegisterWrite(method, handler) }
}

// takeTheSnapshotIn records the projected run of the router's snapshot as the
// run its database took in. Without both stores, or over a snapshot that cannot
// say its run, there is nothing to record, and the test reads that snapshot as
// it is.
func takeTheSnapshotIn(t *testing.T, config Config) {
	t.Helper()
	if config.Ledger == nil || config.Database == nil {
		return
	}
	view := config.Ledger.View()
	defer view.Close()
	recordRunTakenIn(t, view, config.Database)
}

// recordRunTakenIn records the projected run of snapshot as the run database
// took in, for a state built by hand as writes over that snapshot would have
// left it. It must be called outside any change of database.
func recordRunTakenIn(t testing.TB, snapshot *ledger.View, database *authored.Database) {
	t.Helper()
	if run, err := snapshot.ProjectedRun(t.Context()); err == nil {
		authoredtest.TakeRunIn(t, database, run)
	}
}

// withTakeIn is what the router calls to take a snapshot in before a write.
func withTakeIn(fn func(context.Context, time.Time) error) envOption {
	return func(config *Config) { config.TakeSnapshotIn = fn }
}

// withDatabase serves over the app's database, which is what lets a write run.
func withDatabase(database *authored.Database) envOption {
	return func(config *Config) { config.Database = database }
}

func returnsNothing(context.Context, []json.RawMessage, Views) (any, error) { return nil, nil }

func newRecorder() *httptest.ResponseRecorder { return httptest.NewRecorder() }

type countingBody struct{ reads int }

func (body *countingBody) Read([]byte) (int, error) {
	body.reads++
	return 0, io.EOF
}
