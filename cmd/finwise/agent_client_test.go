package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/api"
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

func TestAgentProposeSendsSubmitProposalWithDraftNameFallbackAndToken(t *testing.T) {
	stub := newAnswering(t, http.StatusOK, `{"proposalId":"1","title":"Adicionar #viagem a Loja R$ 10,00"}`, nil)
	token, _ := agentCredentials(t)

	// --name wins over FINWISE_AGENT_NAME.
	vars := map[string]string{agentURLVariable: stub.URL, agentTokenVariable: token, agentNameVariable: "from-env"}
	res1 := runCommand(context.Background(), []string{
		"agent", "propose", "--reason", "Compra na viagem", "--name", "from-flag", "transactions.addTag", `["tx-1","#viagem"]`,
	}, "", vars, nil)
	if res1.code != 0 || res1.stderr != "" || !strings.Contains(res1.stdout, `"proposalId":"1"`) {
		t.Fatalf("propose with --name: exit %d stdout %q stderr %q", res1.code, res1.stdout, res1.stderr)
	}

	// Omitting --name falls back to FINWISE_AGENT_NAME.
	res2 := runCommand(context.Background(), []string{
		"agent", "propose", "--reason", "Compra na viagem", "transactions.addTag", `["tx-1","#viagem"]`,
	}, "", vars, nil)
	if res2.code != 0 || res2.stderr != "" {
		t.Fatalf("propose with env name: exit %d stderr %q", res2.code, res2.stderr)
	}

	stub.mutex.Lock()
	defer stub.mutex.Unlock()
	if len(stub.requests) != 2 {
		t.Fatalf("saw %d requests, want 2", len(stub.requests))
	}
	wantProposers := []string{"from-flag", "from-env"}
	for i, req := range stub.requests {
		if req.Method != http.MethodPost || req.URL.Path != "/api/review/submitProposal" {
			t.Errorf("request %d: %s %s", i, req.Method, req.URL.Path)
		}
		if req.Header.Get("Authorization") != "Bearer "+token || req.Header.Get("Cookie") != "" {
			t.Errorf("request %d: unexpected auth/cookie headers", i)
		}
		var payload []struct {
			Change struct {
				Method    string            `json:"method"`
				Arguments []json.RawMessage `json:"arguments"`
			} `json:"change"`
			Reason   string `json:"reason"`
			Proposer string `json:"proposer"`
		}
		if err := json.Unmarshal([]byte(stub.bodies[i]), &payload); err != nil || len(payload) != 1 {
			t.Fatalf("request %d body %s: %v", i, stub.bodies[i], err)
		}
		if payload[0].Change.Method != "transactions.addTag" || payload[0].Reason != "Compra na viagem" || payload[0].Proposer != wantProposers[i] {
			t.Errorf("request %d payload = %+v, want proposer %q", i, payload[0], wantProposers[i])
		}
	}
}

func TestAgentProposeExitsWithUsageForBadFlagsMethodOrArgumentsAndSendsNothing(t *testing.T) {
	stub := newAnswering(t, http.StatusOK, `null`, nil)
	token, _ := agentCredentials(t)
	vars := map[string]string{agentURLVariable: stub.URL, agentTokenVariable: token}
	const secretReason = "Motivo que nao deve aparecer"
	const secretProposer = "agente-do-chamador"

	cases := map[string][]string{
		"missing reason":        {"agent", "propose", "--name", secretProposer, "transactions.addTag", `["tx-1","#x"]`},
		"blank reason":          {"agent", "propose", "--reason", "  ", "--name", secretProposer, "transactions.addTag", `["tx-1","#x"]`},
		"reason over 500":       {"agent", "propose", "--reason", strings.Repeat("a", 501), "--name", secretProposer, "transactions.addTag", `["tx-1","#x"]`},
		"missing name":          {"agent", "propose", "--reason", secretReason, "transactions.addTag", `["tx-1","#x"]`},
		"name over 40":          {"agent", "propose", "--reason", secretReason, "--name", strings.Repeat("b", 41), "transactions.addTag", `["tx-1","#x"]`},
		"unknown flag":          {"agent", "propose", "--bogus", "x", "--reason", secretReason, "--name", secretProposer, "transactions.addTag", `["tx-1","#x"]`},
		"missing arguments":     {"agent", "propose", "--reason", secretReason, "--name", secretProposer, "transactions.addTag"},
		"too many arguments":    {"agent", "propose", "--reason", secretReason, "--name", secretProposer, "transactions.addTag", `[]`, `[]`},
		"non-proposable method": {"agent", "propose", "--reason", secretReason, "--name", secretProposer, "budget.save", `["2026-03",[]]`},
		"arguments not array":   {"agent", "propose", "--reason", secretReason, "--name", secretProposer, "transactions.addTag", `{}`},
		"arguments null":        {"agent", "propose", "--reason", secretReason, "--name", secretProposer, "transactions.addTag", `null`},
	}
	for label, args := range cases {
		res := runCommand(context.Background(), args, "", vars, nil)
		if res.code != exitUsage || res.stdout != "" || !strings.Contains(res.stderr, agentProposeUsage) {
			t.Errorf("%s: exit %d stdout %q stderr %q", label, res.code, res.stdout, res.stderr)
		}
		if strings.Contains(res.stderr, secretReason) || strings.Contains(res.stderr, secretProposer) {
			t.Errorf("%s: stderr leaked caller input: %q", label, res.stderr)
		}
		if label == "non-proposable method" {
			for _, member := range api.ProposableMethods() {
				if !strings.Contains(res.stderr, member) {
					t.Errorf("non-proposable method error did not list %s: %q", member, res.stderr)
				}
			}
		}
	}
	if stub.seen() != 0 {
		t.Errorf("%d requests were sent for invalid propose calls", stub.seen())
	}
}

func TestAgentProposalsCallsReviewProposalsAndRejectsExtraArguments(t *testing.T) {
	stub := newAnswering(t, http.StatusOK, `[]`, nil)
	token, _ := agentCredentials(t)
	vars := map[string]string{agentURLVariable: stub.URL, agentTokenVariable: token}

	bad := runCommand(context.Background(), []string{"agent", "proposals", "extra"}, "", vars, nil)
	if bad.code != exitUsage || bad.stdout != "" || !strings.Contains(bad.stderr, agentProposalsUsage) {
		t.Errorf("extra arg: exit %d stdout %q stderr %q", bad.code, bad.stdout, bad.stderr)
	}
	if stub.seen() != 0 {
		t.Errorf("extra arg sent %d requests", stub.seen())
	}

	ok := runCommand(context.Background(), []string{"agent", "proposals"}, "", vars, nil)
	if ok.code != 0 || ok.stdout != "[]\n" || ok.stderr != "" {
		t.Fatalf("proposals: exit %d stdout %q stderr %q", ok.code, ok.stdout, ok.stderr)
	}
	stub.mutex.Lock()
	defer stub.mutex.Unlock()
	if len(stub.requests) != 1 || stub.requests[0].URL.Path != "/api/review/proposals" || stub.bodies[0] != "[]" {
		t.Errorf("unexpected request: %+v body %q", stub.requests, stub.bodies)
	}
}
