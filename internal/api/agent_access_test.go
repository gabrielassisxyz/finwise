package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/contract"
)

// withAgentToken makes the router know the hash of an agent's token.
func withAgentToken(hash AgentTokenHash) envOption {
	return func(config *Config) { config.VerifyAgentToken = hash.Verify }
}

// withEveryReadAnswered gives every method of class `read` a handler that notes
// it was called and answers null, so a walk of the manifest does not depend on
// what any one read needs from a ledger.
func withEveryReadAnswered(called map[string]int) envOption {
	return func(config *Config) {
		methods, err := contract.Load()
		if err != nil {
			panic(err)
		}
		for _, method := range methods {
			if method.Class != contract.ClassRead {
				continue
			}
			name := method.Qualified()
			config.Reads.Register(name, func(context.Context, []json.RawMessage, Views) (any, error) {
				called[name]++
				return nil, nil
			})
		}
	}
}

// bodyAcceptedFor is a body the router accepts for the method: the minimum number
// of arguments, each null, and the one month that the schema-checked read takes.
func bodyAcceptedFor(method contract.Method) string {
	if validatedBySchema(method) {
		return `["2026-01"]`
	}
	return "[" + strings.TrimSuffix(strings.Repeat("null,", method.MinArguments), ",") + "]"
}

func (env *testEnv) callWith(path, body, authorization string, cookie *http.Cookie) *httptest.ResponseRecorder {
	env.t.Helper()
	return env.do(request{path: path, body: body, cookie: cookie,
		headers: map[string]string{"Content-Type": "application/json", authorizationHeader: authorization}})
}

// readsOfBodyAs posts a route with a body that counts its reads, carrying the
// Authorization header and no cookie.
func readsOfBodyAs(env *testEnv, route, authorization string) (reads, status int) {
	body := &countingBody{}
	request, _ := http.NewRequest(http.MethodPost, route, body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(authorizationHeader, authorization)
	recorder := newRecorder()
	env.handler.ServeHTTP(recorder, request)
	return body.reads, recorder.Code
}

func bearer(token string) string { return "Bearer " + token }

func TestTheTokenOpensEveryReadOfTheManifest(t *testing.T) {
	token, hash := newTokenAndHash(t)
	called := map[string]int{}
	env := newTestEnv(t, withAgentToken(hash), withEveryReadAnswered(called))
	reads := methodsOfClass(env, contract.ClassRead)
	if len(reads) == 0 {
		t.Fatal("the manifest has no read")
	}
	for _, method := range reads {
		response := env.callWith(method.Route, bodyAcceptedFor(method), bearer(token), nil)
		if response.Code != http.StatusOK || response.Body.String() != "null" {
			t.Errorf("%s: status %d body %s, want 200 null", method.Qualified(), response.Code, response.Body.String())
		}
		if called[method.Qualified()] != 1 {
			t.Errorf("%s: its handler was called %d times, want once", method.Qualified(), called[method.Qualified()])
		}
		if cookies := response.Result().Cookies(); len(cookies) != 0 {
			t.Errorf("%s: the answer sets a cookie", method.Qualified())
		}
	}
}

func TestTheTokenAnswersAReadAsTheCookieDoes(t *testing.T) {
	token, hash := newTokenAndHash(t)
	for _, name := range []string{"full", "empty"} {
		env := newTestEnv(t, withLedger(ledgerNamed(t, name).Path, fixtureNow), withAgentToken(hash))
		cookie := env.unlock()
		for _, c := range readCases {
			domain, method := splitMethod(t, c.method)
			path := "/api/" + domain + "/" + method
			withCookie := env.call(path, c.arguments, cookie)
			withToken := env.callWith(path, c.arguments, bearer(token), nil)
			if withToken.Code != withCookie.Code || !bytes.Equal(withToken.Body.Bytes(), withCookie.Body.Bytes()) {
				t.Errorf("%s ledger, %s %s: the token's answer (%d) differs from the cookie's (%d)", name, c.method, c.arguments, withToken.Code, withCookie.Code)
			}
		}
	}
}

func TestTheTokenIsRefusedAsReadOnlyForEveryWriteEvaluationAndSessionMethodBeforeTheBody(t *testing.T) {
	token, hash := newTokenAndHash(t)
	scratch := authoredtest.Open(t, newFakeClock().Now)
	env := newTestEnv(t, withAgentToken(hash), withDatabase(scratch.Database), func(config *Config) { config.Reads = NewReadRegistry() })
	cookie := env.unlock()
	before := scratch.State(t)
	refused := methodsOfClass(env, contract.ClassWrite, contract.ClassEvaluate, contract.ClassSession)
	if len(refused) != writesAndEvaluationsInTheManifest+3 {
		t.Fatalf("the manifest has %d methods that are not reads, want %d", len(refused), writesAndEvaluationsInTheManifest+3)
	}
	sessionStatus := false
	for _, method := range refused {
		sessionStatus = sessionStatus || method.Qualified() == "session.status"
		for _, body := range postBodies {
			response := env.callWith(method.Route, body, bearer(token), nil)
			expectError(t, method.Qualified()+" body "+body, response, http.StatusForbidden, "read_only")
			if response.Header().Get("Set-Cookie") != "" {
				t.Errorf("%s: the refusal sets a cookie", method.Qualified())
			}
		}
		if reads, status := readsOfBodyAs(env, method.Route, bearer(token)); reads != 0 || status != http.StatusForbidden {
			t.Errorf("%s: the body was read %d times and the status was %d, want 0 and 403", method.Qualified(), reads, status)
		}
		withCookieToo := env.callWith(method.Route, `[]`, bearer(token), cookie)
		expectError(t, method.Qualified()+" with a cookie", withCookieToo, http.StatusForbidden, "read_only")
	}
	if !sessionStatus {
		t.Error("session.status is not among the methods refused")
	}
	if after := scratch.State(t); after != before {
		t.Errorf("a refused call changed the database:\n%s\nwas\n%s", after, before)
	}
}

func TestTheTokenCannotEndOrOpenASession(t *testing.T) {
	token, hash := newTokenAndHash(t)
	called := map[string]int{}
	env := newTestEnv(t, withAgentToken(hash), withEveryReadAnswered(called))
	cookie := env.unlock()
	env.pinTries.Store(0)

	expectError(t, "lock with a cookie", env.callWith("/api/session/lock", `[]`, bearer(token), cookie), http.StatusForbidden, "read_only")
	expectError(t, "unlock", env.callWith("/api/session/unlock", `["`+testPIN+`"]`, bearer(token), nil), http.StatusForbidden, "read_only")

	if env.pinTries.Load() != 0 {
		t.Error("the PIN was compared for a request that carried the token")
	}
	if response := env.call("/api/shell/overview", `[]`, cookie); response.Code != http.StatusOK {
		t.Errorf("the session that was there is gone: status %d", response.Code)
	}
}

func TestAHeaderThatIsNotTheTokenIsLockedWhateverCookieComesWithIt(t *testing.T) {
	token, hash := newTokenAndHash(t)
	other, _ := newTokenAndHash(t)
	headers := map[string]string{
		"a wrong token":             bearer(other),
		"the token minus a letter":  bearer(token[:len(token)-1]),
		"another scheme":            "Basic " + token,
		"a made-up scheme":          "Token " + token,
		"no space":                  "Bearer" + token,
		"two spaces":                "Bearer  " + token,
		"no token":                  "Bearer ",
		"the scheme alone":          "Bearer",
		"the token with no scheme":  token,
		"an empty header":           "",
		"the token and a trailer":   bearer(token + " "),
		"the token and another one": bearer(token + "," + other),
	}
	environments := map[string]*testEnv{
		"a server with the hash":    newTestEnv(t, withAgentToken(hash), withEveryReadAnswered(map[string]int{})),
		"a server with no hash":     newTestEnv(t, withEveryReadAnswered(map[string]int{})),
		"a server with another one": newTestEnv(t, withAgentToken(hashOfAnotherToken(t)), withEveryReadAnswered(map[string]int{})),
	}
	routes := []string{"/api/shell/overview", "/api/transactions/setNote", "/api/session/status", "/api/session/unlock", "/api/session/lock"}
	for serverName, env := range environments {
		cookie := env.unlock()
		for headerName, header := range headers {
			for _, route := range routes {
				for _, sent := range []*http.Cookie{nil, cookie} {
					label := serverName + ", " + headerName + ", " + route
					expectError(t, label, env.callWith(route, `[]`, header, sent), http.StatusUnauthorized, "locked")
				}
			}
		}
		if env.pinTries.Load() != 1 {
			t.Errorf("%s: the PIN was compared %d times, want the one that opened the session", serverName, env.pinTries.Load())
		}
	}
	// With the token itself the same server and the same cookie open the read.
	env := environments["a server with the hash"]
	if response := env.callWith("/api/shell/overview", `[]`, bearer(token), nil); response.Code != http.StatusOK {
		t.Errorf("the right token was refused: status %d", response.Code)
	}
}

func hashOfAnotherToken(t testing.TB) AgentTokenHash {
	t.Helper()
	_, hash := newTokenAndHash(t)
	return hash
}

func TestTwoAuthorizationHeadersAreLockedEvenWhenOneIsTheToken(t *testing.T) {
	token, hash := newTokenAndHash(t)
	env := newTestEnv(t, withAgentToken(hash), withEveryReadAnswered(map[string]int{}))
	for index, order := range [][]string{{bearer(token), bearer(token)}, {bearer(token), "Basic abc"}, {"Basic abc", bearer(token)}} {
		request := httptest.NewRequest(http.MethodPost, "/api/shell/overview", strings.NewReader(`[]`))
		request.Header.Set("Content-Type", "application/json")
		for _, value := range order {
			request.Header.Add(authorizationHeader, value)
		}
		response := httptest.NewRecorder()
		env.handler.ServeHTTP(response, request)
		expectError(t, "two headers, case "+string(rune('a'+index)), response, http.StatusUnauthorized, "locked")
	}
}

func TestTheSchemeIsMatchedWithoutRegardToCase(t *testing.T) {
	token, hash := newTokenAndHash(t)
	env := newTestEnv(t, withAgentToken(hash), withEveryReadAnswered(map[string]int{}))
	for _, scheme := range []string{"bearer", "BEARER", "Bearer"} {
		if response := env.callWith("/api/shell/overview", `[]`, scheme+" "+token, nil); response.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200", scheme, response.Code)
		}
	}
}

func TestACookieWithoutTheHeaderIsAnsweredAsBeforeWhateverTheServerHolds(t *testing.T) {
	_, hash := newTokenAndHash(t)
	for name, options := range map[string][]envOption{"with a hash": {withAgentToken(hash)}, "with no hash": nil} {
		env := newTestEnv(t, append(options, withEveryReadAnswered(map[string]int{}))...)
		expectError(t, name+" locked", env.call("/api/shell/overview", `[]`, nil), http.StatusUnauthorized, "locked")
		cookie := env.unlock()
		if response := env.call("/api/shell/overview", `[]`, cookie); response.Code != http.StatusOK {
			t.Errorf("%s: a read with a live cookie answered %d", name, response.Code)
		}
		if status := env.call("/api/session/status", `[]`, cookie); status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"locked":false`) {
			t.Errorf("%s: the session's status answered %d %s", name, status.Code, status.Body.String())
		}
		if len(env.call("/api/shell/overview", `[]`, cookie).Result().Cookies()) == 0 {
			t.Errorf("%s: a cookie request is no longer renewed", name)
		}
	}
}

func TestTheSchemaCheckedReadIsCheckedForTheAgentToo(t *testing.T) {
	token, hash := newTokenAndHash(t)
	env := newTestEnv(t, withAgentToken(hash), withEveryReadAnswered(map[string]int{}))
	for _, body := range []string{`[]`, `[1,2,3]`, `{}`, `not json`} {
		expectError(t, "budget.view "+body, env.callWith("/api/budget/view", body, bearer(token), nil), http.StatusBadRequest, "bad_request")
	}
	for _, body := range []string{`["2026-01"]`, `["2026-01",null]`} {
		if response := env.callWith("/api/budget/view", body, bearer(token), nil); response.Code != http.StatusOK {
			t.Errorf("budget.view %s: status %d, want 200", body, response.Code)
		}
	}
	headers := map[string]string{"Content-Type": "text/plain", authorizationHeader: bearer(token)}
	expectError(t, "content type", env.do(request{path: "/api/shell/overview", body: `[]`, headers: headers}), http.StatusBadRequest, "bad_request")
}

func TestAnAgentRequestOnAnotherVerbIsBadRequestAndAnUnknownRouteIsNotFound(t *testing.T) {
	token, hash := newTokenAndHash(t)
	env := newTestEnv(t, withAgentToken(hash), withEveryReadAnswered(map[string]int{}))
	headers := map[string]string{"Content-Type": "application/json", authorizationHeader: bearer(token)}
	expectError(t, "GET", env.do(request{verb: http.MethodGet, path: "/api/shell/overview", headers: headers}), http.StatusBadRequest, "bad_request")
	expectError(t, "unknown", env.do(request{path: "/api/nope/nothing", body: `[]`, headers: headers}), http.StatusNotFound, "unknown_method")
}
