package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/contract"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

func TestEveryWriteAndEvaluationIsRefusedWithASession(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	refused := methodsOfClass(env, contract.ClassWrite, contract.ClassEvaluate)
	if len(refused) != 56 {
		t.Fatalf("the manifest has %d write and evaluate methods, want 56", len(refused))
	}
	for _, method := range refused {
		for _, body := range []string{`[]`, `not json`, ``, `{"a":1}`} {
			response := env.call(method.Route, body, cookie)
			expectError(t, method.Route+" body "+body, response, http.StatusForbidden, "read_only")
		}
	}
}

func TestAWriteIsRefusedBeforeItsBodyIsRead(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	body := &countingBody{}
	request, _ := http.NewRequest(http.MethodPost, "/api/transactions/setLine", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := newRecorder()
	env.handler.ServeHTTP(recorder, request)
	if body.reads != 0 {
		t.Errorf("the body of a refused write was read %d times", body.reads)
	}
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status %d, want 403", recorder.Code)
	}
}

func TestEveryRouteButTheOpenOnesNeedsASession(t *testing.T) {
	env := newTestEnv(t)
	checked := 0
	for _, method := range env.methods {
		if method.Qualified() == "session.status" || method.Qualified() == "session.unlock" {
			continue
		}
		response := env.call(method.Route, `[]`, nil)
		expectError(t, method.Route, response, http.StatusUnauthorized, "locked")
		checked++
	}
	if checked != len(env.methods)-2 {
		t.Errorf("checked %d routes of %d", checked, len(env.methods))
	}
}

func TestAStaleOrForgedCookieIsLocked(t *testing.T) {
	env := newTestEnv(t)
	forged := &http.Cookie{Name: hostCookieName, Value: "not-a-session"}
	expectError(t, "forged", env.call("/api/shell/overview", `[]`, forged), http.StatusUnauthorized, "locked")
	expectError(t, "forged write", env.call("/api/transactions/setLine", `[]`, forged), http.StatusUnauthorized, "locked")
}

func TestUnknownRoutesAreJSON404(t *testing.T) {
	env := newTestEnv(t)
	for _, path := range []string{"/api/nope/nothing", "/api/", "/api", "/api/shell/overview/", "/elsewhere"} {
		for _, verb := range []string{http.MethodPost, http.MethodGet} {
			response := env.do(request{verb: verb, path: path, body: `[]`})
			expectError(t, verb+" "+path, response, http.StatusNotFound, "unknown_method")
		}
	}
}

func TestAnotherVerbOnAMethodRouteIsBadRequest(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	for _, verb := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		for _, with := range []*http.Cookie{nil, cookie} {
			response := env.do(request{verb: verb, path: "/api/shell/overview", cookie: with})
			expectError(t, verb, response, http.StatusBadRequest, "bad_request")
		}
	}
}

func TestContentTypeMustBeJSON(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	for _, contentType := range []string{"text/plain", "", "application/x-www-form-urlencoded", "application/jsonx"} {
		headers := map[string]string{"Content-Type": contentType}
		response := env.do(request{path: "/api/shell/overview", body: `[]`, headers: headers, cookie: cookie})
		expectError(t, "content type "+contentType, response, http.StatusBadRequest, "bad_request")
	}
	headers := map[string]string{"Content-Type": "application/json; charset=utf-8"}
	response := env.do(request{path: "/api/session/status", body: `[]`, headers: headers})
	if response.Code != http.StatusOK {
		t.Errorf("a charset parameter was refused: status %d", response.Code)
	}
}

func TestContentTypeIsRequiredOnTheOpenRoutesToo(t *testing.T) {
	env := newTestEnv(t)
	headers := map[string]string{"Content-Type": "text/plain"}
	for _, path := range []string{"/api/session/status", "/api/session/unlock"} {
		response := env.do(request{path: path, body: `["` + testPIN + `"]`, headers: headers})
		expectError(t, path, response, http.StatusBadRequest, "bad_request")
	}
	if env.pinTries.Load() != 0 {
		t.Error("a PIN was compared for a request with the wrong content type")
	}
}

func TestABodyOverOneMebibyteIsBadRequest(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	huge := `["` + strings.Repeat("a", maxBodyBytes) + `"]`
	expectError(t, "over", env.call("/api/session/unlock", huge, cookie), http.StatusBadRequest, "bad_request")
	expectError(t, "over read", env.call("/api/shell/overview", huge, cookie), http.StatusBadRequest, "bad_request")
	almost := `["` + strings.Repeat("a", maxBodyBytes-10) + `"]`
	response := env.call("/api/session/unlock", almost, cookie)
	if response.Code != http.StatusOK {
		t.Errorf("a body just under the limit: status %d, want 200", response.Code)
	}
}

func TestBodiesThatAreNotTheMethodsArgumentsAreBadRequest(t *testing.T) {
	env := newTestEnv(t, withRead("budget.view", returnsNothing))
	cookie := env.unlock()
	cases := map[string]string{
		"not json": `nope`, "empty": ``, "an object": `{}`, "null": `null`, "a string": `"x"`,
		"too many": `[1]`, "trailing": `[] []`,
	}
	for name, body := range cases {
		expectError(t, name, env.call("/api/shell/overview", body, cookie), http.StatusBadRequest, "bad_request")
	}
	expectError(t, "too few", env.call("/api/session/unlock", `[]`, nil), http.StatusBadRequest, "bad_request")
	expectError(t, "wrong type", env.call("/api/session/unlock", `[123456]`, nil), http.StatusBadRequest, "bad_request")
	handler := env
	for _, body := range []string{`[]`, `[1,2,3]`} {
		response := handler.call("/api/budget/view", body, cookie)
		expectError(t, "budget.view "+body, response, http.StatusBadRequest, "bad_request")
	}
	for _, body := range []string{`["2026-01"]`, `["2026-01",null]`} {
		if response := handler.call("/api/budget/view", body, cookie); response.Code != http.StatusOK {
			t.Errorf("budget.view %s: status %d, want 200", body, response.Code)
		}
	}
}

func TestACrossSitePostIsForbidden(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	env.pinTries.Store(0)
	headers := map[string]string{"Content-Type": "application/json", "Sec-Fetch-Site": "cross-site"}
	for _, path := range []string{"/api/session/unlock", "/api/shell/overview", "/api/transactions/setLine"} {
		response := env.do(request{path: path, body: `["` + testPIN + `"]`, headers: headers, cookie: cookie})
		if response.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", path, response.Code)
		}
		if errorKindOf(t, response) == "" {
			t.Errorf("%s: the refusal is not an error envelope", path)
		}
	}
	if env.pinTries.Load() != 0 {
		t.Error("a cross-site request reached the PIN comparison")
	}
	same := map[string]string{"Content-Type": "application/json", "Sec-Fetch-Site": "same-origin"}
	if response := env.do(request{path: "/api/session/status", body: `[]`, headers: same}); response.Code != http.StatusOK {
		t.Errorf("same-origin request: status %d, want 200", response.Code)
	}
}

func TestAReadWithoutAHandlerIsInternal(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	expectError(t, "shell.overview", env.call("/api/shell/overview", `[]`, cookie), http.StatusInternalServerError, "internal")
}

func TestAReadHandlerGetsTheContextTheArgumentsAndTheReader(t *testing.T) {
	reader := ledger.New("unused")
	var got struct {
		args   []json.RawMessage
		reader *ledger.Reader
		ctxOK  bool
	}
	record := func(ctx context.Context, args []json.RawMessage, from *ledger.Reader) (any, error) {
		got.args, got.reader, got.ctxOK = args, from, ctx.Err() == nil
		return map[string]any{"id": "x"}, nil
	}
	handler := newTestEnv(t, withRead("transactions.detail", record), func(config *Config) { config.Ledger = reader })
	response := handler.call("/api/transactions/detail", `["abc"]`, handler.unlock())
	if response.Code != http.StatusOK || response.Body.String() != `{"id":"x"}` {
		t.Fatalf("status %d body %s", response.Code, response.Body.String())
	}
	if len(got.args) != 1 || string(got.args[0]) != `"abc"` || got.reader != reader || !got.ctxOK {
		t.Errorf("the handler received %+v", got)
	}
}

func TestAReadThatReturnsNothingAnswersNull(t *testing.T) {
	handler := newTestEnv(t, withRead("shell.overview", returnsNothing))
	response := handler.call("/api/shell/overview", `[]`, handler.unlock())
	if response.Body.String() != "null" || response.Code != http.StatusOK {
		t.Errorf("status %d body %q, want 200 null", response.Code, response.Body.String())
	}
}

func TestReadErrorsAreMappedByClass(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
		kind   string
	}{
		"unsupported version": {&ledger.UnsupportedVersionError{Found: 9}, 503, "ledger_unavailable"},
		"wrapped unsupported": {fmt.Errorf("ctx: %w", &ledger.UnsupportedVersionError{Found: 2}), 503, "ledger_unavailable"},
		"missing snapshot":    {fmt.Errorf("ledger: snapshot: %w", fs.ErrNotExist), 503, "ledger_unavailable"},
		"unreadable snapshot": {fmt.Errorf("ledger: snapshot: %w", fs.ErrPermission), 503, "ledger_unavailable"},
		"anything else":       {errors.New("boom"), 500, "internal"},
	}
	for name, tc := range cases {
		failing := func(context.Context, []json.RawMessage, *ledger.Reader) (any, error) { return nil, tc.err }
		handler := newTestEnv(t, withRead("shell.overview", failing))
		expectError(t, name, handler.call("/api/shell/overview", `[]`, handler.unlock()), tc.status, tc.kind)
	}
}

func TestAPanickingHandlerIsAnsweredInternal(t *testing.T) {
	exploding := func(context.Context, []json.RawMessage, *ledger.Reader) (any, error) { panic("row 4711 exploded") }
	handler := newTestEnv(t, withRead("shell.overview", exploding))
	expectError(t, "panic", handler.call("/api/shell/overview", `[]`, handler.unlock()), http.StatusInternalServerError, "internal")
	if strings.Contains(handler.logs.String(), "4711") {
		t.Error("the panic value reached the log")
	}
}

func TestAnUnserialisableResultIsInternal(t *testing.T) {
	channel := func(context.Context, []json.RawMessage, *ledger.Reader) (any, error) { return make(chan int), nil }
	handler := newTestEnv(t, withRead("shell.overview", channel))
	expectError(t, "chan", handler.call("/api/shell/overview", `[]`, handler.unlock()), http.StatusInternalServerError, "internal")
}

func TestAHandlerCannotBeRegisteredForAWrite(t *testing.T) {
	methods, _ := contract.Load()
	noop := func(context.Context, []json.RawMessage, *ledger.Reader) (any, error) { return nil, nil }
	for _, name := range []string{"transactions.setLine", "budget.evaluateScenario", "session.unlock", "no.such"} {
		reads := NewRegistry()
		reads.Register(name, noop)
		_, err := NewHandler(Config{Methods: methods, Reads: reads, VerifyPIN: func(string) bool { return false }})
		if err == nil {
			t.Errorf("a handler for %s was accepted", name)
		}
	}
}

func TestRegisteringTwiceFailsLoudly(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a second registration did not panic")
		}
	}()
	registry := NewRegistry()
	noop := func(context.Context, []json.RawMessage, *ledger.Reader) (any, error) { return nil, nil }
	registry.Register("shell.overview", noop)
	registry.Register("shell.overview", noop)
}

func TestHealthzAnswers200WithoutASession(t *testing.T) {
	env := newTestEnv(t)
	response := env.do(request{verb: http.MethodGet, path: "/healthz"})
	if response.Code != http.StatusOK {
		t.Errorf("status %d, want 200", response.Code)
	}
	if response.Body.Len() != 0 {
		t.Error("healthz says something about the server")
	}
}

func TestNewHandlerNeedsAPINVerifier(t *testing.T) {
	if _, err := NewHandler(Config{}); err == nil {
		t.Error("a handler without a PIN check was built")
	}
}
