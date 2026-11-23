package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// answeringWith is a server that says what it is told for any request and
// records what it was sent.
type answeringWith struct {
	*httptest.Server
	mutex    sync.Mutex
	requests []*http.Request
	bodies   []string
}

func newAnswering(t *testing.T, status int, body string, headers map[string]string) *answeringWith {
	t.Helper()
	stub := &answeringWith{}
	stub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ := io.ReadAll(r.Body)
		stub.mutex.Lock()
		stub.requests, stub.bodies = append(stub.requests, r), append(stub.bodies, string(sent))
		stub.mutex.Unlock()
		for name, value := range headers {
			w.Header().Set(name, value)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(stub.Close)
	return stub
}

func (stub *answeringWith) seen() int {
	stub.mutex.Lock()
	defer stub.mutex.Unlock()
	return len(stub.requests)
}

func TestAgentReadSendsTheMethodsRouteTheArgumentsAndTheTokenAndNothingMore(t *testing.T) {
	stub := newAnswering(t, http.StatusOK, `{"ok":true}`, nil)
	token, _ := agentCredentials(t)
	for _, base := range []string{stub.URL, stub.URL + "/", stub.URL + "/finwise//"} {
		vars := map[string]string{agentURLVariable: base, agentTokenVariable: token}
		result := runCommand(context.Background(), []string{"agent", "read", "transactions.detail", ` ["tx-1"] `}, "", vars, nil)
		if result.code != 0 || result.stdout != `{"ok":true}`+"\n" || result.stderr != "" {
			t.Fatalf("%s: exit %d, standard output %q, standard error %q", base, result.code, result.stdout, result.stderr)
		}
	}
	runCommand(context.Background(), []string{"agent", "read", "accounts.list"}, "", map[string]string{agentURLVariable: stub.URL, agentTokenVariable: token}, nil)
	stub.mutex.Lock()
	defer stub.mutex.Unlock()
	wantPaths := []string{"/api/transactions/detail", "/api/transactions/detail", "/finwise/api/transactions/detail", "/api/accounts/list"}
	wantBodies := []string{` ["tx-1"] `, ` ["tx-1"] `, ` ["tx-1"] `, `[]`}
	for index, request := range stub.requests {
		if request.Method != http.MethodPost || request.URL.Path != wantPaths[index] || stub.bodies[index] != wantBodies[index] {
			t.Errorf("request %d: %s %s with body %q", index, request.Method, request.URL.Path, stub.bodies[index])
		}
		if request.Header.Get("Authorization") != "Bearer "+token || request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Cookie") != "" {
			t.Errorf("request %d: headers %v", index, request.Header)
		}
	}
}

func TestAgentReadFollowsNoRedirectAndSendsTheTokenNowhereElse(t *testing.T) {
	elsewhere := newAnswering(t, http.StatusOK, `{"leaked":true}`, nil)
	token, _ := agentCredentials(t)
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		redirecting := newAnswering(t, status, "", map[string]string{"Location": elsewhere.URL + "/api/accounts/list"})
		vars := map[string]string{agentURLVariable: redirecting.URL, agentTokenVariable: token}
		result := runCommand(context.Background(), []string{"agent", "read", "accounts.list"}, "", vars, nil)
		if result.code != exitFailure || result.stdout != "" || result.stderr != "finwise: accounts.list: the answer is not the API's\n" {
			t.Errorf("status %d: exit %d, standard output %q, standard error %q", status, result.code, result.stdout, result.stderr)
		}
		if redirecting.seen() != 1 {
			t.Errorf("status %d: the redirecting server saw %d requests, want one", status, redirecting.seen())
		}
	}
	if elsewhere.seen() != 0 {
		t.Errorf("the redirect was followed: %d requests reached the other server", elsewhere.seen())
	}
}

func TestAgentReadPrintsAFailureAsTheContractsKindAndReasonOrAsNotTheAPIs(t *testing.T) {
	token, _ := agentCredentials(t)
	const sentinel = "SERVER-SAID-THIS-9d2c"
	cases := map[string]struct {
		status int
		body   string
		stdout string
		stderr string
	}{
		"null":                       {200, `null`, "null\n", ""},
		"a list":                     {200, `[1,2]`, "[1,2]\n", ""},
		"a plain kind":               {503, `{"error":{"kind":"ledger_unavailable"}}`, "", "finwise: accounts.list answered ledger_unavailable\n"},
		"a refusal":                  {409, `{"error":{"kind":"refused","reason":"month_closed"}}`, "", "finwise: accounts.list answered refused (month_closed)\n"},
		"a kind of its own":          {500, `{"error":{"kind":"` + sentinel + `"}}`, "", "finwise: accounts.list: the answer is not the API's\n"},
		"a reason of its own":        {409, `{"error":{"kind":"refused","reason":"` + sentinel + `"}}`, "", "finwise: accounts.list: the answer is not the API's\n"},
		"a refusal with no reason":   {409, `{"error":{"kind":"refused"}}`, "", "finwise: accounts.list: the answer is not the API's\n"},
		"a reason on a plain kind":   {403, `{"error":{"kind":"read_only","reason":"month_closed"}}`, "", "finwise: accounts.list: the answer is not the API's\n"},
		"text":                       {200, sentinel, "", "finwise: accounts.list: the answer is not the API's\n"},
		"a page":                     {502, `<html>` + sentinel + `</html>`, "", "finwise: accounts.list: the answer is not the API's\n"},
		"a failure that is not JSON": {500, ``, "", "finwise: accounts.list: the answer is not the API's\n"},
	}
	for label, c := range cases {
		stub := newAnswering(t, c.status, c.body, nil)
		vars := map[string]string{agentURLVariable: stub.URL, agentTokenVariable: token}
		result := runCommand(context.Background(), []string{"agent", "read", "accounts.list"}, "", vars, nil)
		wantCode := exitFailure
		if c.stderr == "" {
			wantCode = 0
		}
		if result.code != wantCode || result.stdout != c.stdout || result.stderr != c.stderr {
			t.Errorf("%s: exit %d, standard output %q, standard error %q", label, result.code, result.stdout, result.stderr)
		}
	}
}

func TestTheClientGivesUpOnAServerThatDoesNotAnswer(t *testing.T) {
	release := make(chan struct{})
	stalled := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer stalled.Close()
	defer close(release)
	started := time.Now()
	_, _, err := postRead(context.Background(), newAgentClient(100*time.Millisecond), stalled.URL, "token", []byte("[]"))
	if err == nil || time.Since(started) > 10*time.Second {
		t.Errorf("the client waited %v and ended with %v", time.Since(started), err)
	}
	if agentTimeout != 60*time.Second {
		t.Errorf("the client gives up after %v, the server's WriteTimeout is a minute", agentTimeout)
	}
}
