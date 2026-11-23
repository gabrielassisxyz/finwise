package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/api"
	"github.com/gabrielassisxyz/finwise/internal/contract"
)

const loggedLine = "an agent token opens the reads"

// agentCredentials runs `finwise agent-token` and returns what it printed.
func agentCredentials(t *testing.T) (token, hash string) {
	t.Helper()
	result := runCommand(context.Background(), []string{"agent-token"}, "", nil, nil)
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("agent-token exited %d with %q on standard error", result.code, result.stderr)
	}
	lines := strings.Split(strings.TrimSuffix(result.stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("agent-token printed %d lines, want 2", len(lines))
	}
	token, hash = strings.TrimPrefix(lines[0], "FINWISE_AGENT_TOKEN="), strings.TrimPrefix(lines[1], "FINWISE_AGENT_TOKEN_HASH=")
	if token == lines[0] || hash == lines[1] {
		t.Fatalf("agent-token's lines do not start with the variables' names")
	}
	return token, hash
}

func TestAgentTokenPrintsTheTokenAndItsHashAsTwoLinesAndNothingElse(t *testing.T) {
	result := runCommand(context.Background(), []string{"agent-token"}, "", nil, nil)
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit %d, standard error %q", result.code, result.stderr)
	}
	lines := strings.Split(result.stdout, "\n")
	if len(lines) != 3 || lines[2] != "" {
		t.Fatalf("the output is not two lines ending in a newline: %d parts", len(lines))
	}
	token, hash := agentCredentials(t)
	if len(token) != 43 {
		t.Errorf("a token of %d characters, want 43", len(token))
	}
	parsed, err := api.ParseAgentTokenHash(hash)
	if err != nil {
		t.Fatalf("the printed hash does not parse: %v", err)
	}
	if !parsed.Verify(token) {
		t.Error("the printed hash does not verify the printed token")
	}
	changed := []byte(token)
	changed[0] ^= 1
	if parsed.Verify(string(changed)) {
		t.Error("the hash verifies a token with one character changed")
	}
	if another, _ := agentCredentials(t); another == token {
		t.Error("two runs printed the same token")
	}
}

func TestAgentTokenTakesNoArguments(t *testing.T) {
	result := runCommand(context.Background(), []string{"agent-token", "extra"}, "", nil, nil)
	if result.code != exitUsage || result.stdout != "" || !strings.Contains(result.stderr, "usage: finwise agent-token") {
		t.Errorf("exit %d, standard output %q, standard error %q", result.code, result.stdout, result.stderr)
	}
}

func TestServeRefusesAnAgentTokenHashThatDoesNotParseAndNamesOnlyTheVariable(t *testing.T) {
	token, hash := agentCredentials(t)
	for label, value := range map[string]string{
		"garbage": "garbage-hash-sentinel", "the token itself": token, "a truncated hash": hash[:len(hash)-4], "the wrong scheme": "sha512" + strings.TrimPrefix(hash, "sha256"),
	} {
		vars := refusalVars(serveVars(t))
		vars[agentTokenHashVariable] = value
		result := runCommand(cancelledContext(), []string{"serve"}, "", vars, nil)
		if result.code != exitFailure {
			t.Errorf("%s: exit %d, want %d", label, result.code, exitFailure)
		}
		if !strings.Contains(result.stderr, agentTokenHashVariable) {
			t.Errorf("%s: the message does not name the variable", label)
		}
		if text := result.stdout + result.stderr; strings.Contains(text, value) || strings.Contains(text, token) {
			t.Errorf("%s: the output holds the value", label)
		}
	}
}

func TestServeLogsOnceThatAnAgentTokenOpensTheReadsAndNothingWithoutOne(t *testing.T) {
	token, hash := agentCredentials(t)
	with := serveVars(t)
	with[agentTokenHashVariable] = hash
	for label, vars := range map[string]map[string]string{"with the hash": with, "without it": serveVars(t)} {
		server := startServer(t, []string{"serve", "--dev-insecure-cookie"}, vars)
		result := server.stopGracefully(t)
		want := 0
		if label == "with the hash" {
			want = 1
		}
		if count := strings.Count(result.stdout, loggedLine); count != want {
			t.Errorf("%s: the start logged the line %d times, want %d", label, count, want)
		}
		for _, secret := range []string{token, hash} {
			if strings.Contains(result.stdout+result.stderr, secret) {
				t.Errorf("%s: the output holds the token or its hash", label)
			}
		}
	}
}

func TestAServerWithoutAHashLocksARequestThatCarriesAHeaderAndAnswersTheCookieAsBefore(t *testing.T) {
	token, _ := agentCredentials(t)
	server := startServer(t, []string{"serve", "--dev-insecure-cookie"}, serveVars(t))
	cookie, body := server.unlockBody(t, testPIN)
	if cookie == nil || body != `{"ok":true}` {
		t.Fatalf("the cookie session no longer opens: %s", body)
	}
	if _, answer := server.postWithHeader(t, token, "/api/session/status", `[]`, cookie); answer != `{"error":{"kind":"locked"}}` {
		t.Errorf("a header with a cookie answered %s, want locked", answer)
	}
	if answer := server.postAs(t, cookie, "/api/session/status", `[]`); !strings.Contains(answer, `"locked":false`) {
		t.Errorf("the cookie alone answered %s", answer)
	}
}

// postWithHeader posts with the token as the Authorization header, and a cookie
// when one is given.
func (server *runningServer) postWithHeader(t *testing.T, token, path, body string, cookie *http.Cookie) (int, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "http://"+server.address+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	text, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(text)
}

// agentServer is the real server over an invented snapshot, started with the
// hash of a token made for the agent.
type agentServer struct {
	*runningServer
	files servedFiles
	token string
}

func startAgentServer(t *testing.T) agentServer {
	t.Helper()
	files := newServedFiles(t)
	inventedNight(t, files.ledgerPath, 1, novemberRows(), nil)
	budgetedDatabase(t, files.databasePath)
	token, hash := agentCredentials(t)
	vars := serveVars(t)
	vars[agentTokenHashVariable] = hash
	taken := runsTakenIn(t, files.databasePath)
	server := startServer(t, files.serveArgs("--dev-insecure-cookie"), vars)
	waitFor(t, "the start's run", func() bool { return slices.Equal(taken(), []string{"1"}) })
	return agentServer{runningServer: server, files: files, token: token}
}

func (server agentServer) variables(token string) map[string]string {
	return map[string]string{agentURLVariable: "http://" + server.address, agentTokenVariable: token}
}

func (server agentServer) read(args ...string) outcome {
	return runCommand(context.Background(), append([]string{"agent", "read"}, args...), "", server.variables(server.token), nil)
}

func (server agentServer) propose(extraVars map[string]string, args ...string) outcome {
	vars := server.variables(server.token)
	for k, v := range extraVars {
		vars[k] = v
	}
	return runCommand(context.Background(), append([]string{"agent", "propose"}, args...), "", vars, nil)
}

func (server agentServer) proposals() outcome {
	return runCommand(context.Background(), []string{"agent", "proposals"}, "", server.variables(server.token), nil)
}

func TestAgentReadPrintsTheServersAnswerAndNothingElse(t *testing.T) {
	server := startAgentServer(t)

	accounts := server.read("accounts.list")
	if accounts.code != 0 || accounts.stderr != "" || !strings.HasSuffix(accounts.stdout, "\n") {
		t.Fatalf("accounts.list exited %d with %q on standard error", accounts.code, accounts.stderr)
	}
	if !strings.Contains(accounts.stdout, "Conta "+ledgerSentinel) {
		t.Error("the answer does not hold the invented account's name")
	}
	cookie, _ := server.unlockBody(t, testPIN)
	if screen := server.postAs(t, cookie, "/api/accounts/list", `[]`); strings.TrimSuffix(accounts.stdout, "\n") != screen {
		t.Error("the agent's answer is not the one the screen gets")
	}

	missing := server.read("transactions.detail", `["no-such-id"]`)
	if missing.code != 0 || missing.stdout != "null\n" || missing.stderr != "" {
		t.Errorf("an unknown transaction: exit %d, standard output %q, standard error %q", missing.code, missing.stdout, missing.stderr)
	}
	found := server.read("transactions.detail", `["`+firstPurchase+`"]`)
	if found.code != 0 || !strings.Contains(found.stdout, "PADARIA") {
		t.Errorf("a listed transaction: exit %d, standard error %q", found.code, found.stderr)
	}
}

func TestAgentReadFailsWithAFixedSentenceThatQuotesNothingTheServerOrTheCallerSaid(t *testing.T) {
	server := startAgentServer(t)
	another, _ := agentCredentials(t)
	free := unusedAddress(t)
	cases := map[string]struct {
		result outcome
		stderr string
	}{
		"a wrong token": {runCommand(context.Background(), []string{"agent", "read", "accounts.list"}, "", server.variables(another), nil),
			"finwise: accounts.list answered locked\n"},
		"no server": {runCommand(context.Background(), []string{"agent", "read", "accounts.list"}, "",
			map[string]string{agentURLVariable: "http://" + free, agentTokenVariable: server.token}, nil),
			"finwise: could not reach the server at http://" + free + "\n"},
		"no token": {runCommand(context.Background(), []string{"agent", "read", "accounts.list"}, "",
			map[string]string{agentURLVariable: "http://" + server.address}, nil),
			"finwise: FINWISE_AGENT_TOKEN is not set\n"},
		"a token with a line break": {runCommand(context.Background(), []string{"agent", "read", "accounts.list"}, "",
			server.variables(server.token+"\n"), nil),
			"finwise: FINWISE_AGENT_TOKEN holds characters a token never has\n"},
		"a bad argument count": {server.read("transactions.detail"),
			"finwise: transactions.detail answered bad_request\n"},
	}
	for label, c := range cases {
		if c.result.code != exitFailure || c.result.stdout != "" || c.result.stderr != c.stderr {
			t.Errorf("%s: exit %d, standard output %q, standard error %q", label, c.result.code, c.result.stdout, c.result.stderr)
		}
	}
}

func TestAgentReadRefusesAnAddressThatWouldPutTheTokenOnANetworkInClear(t *testing.T) {
	server := startAgentServer(t)
	for _, address := range []string{
		"http://192.0.2.1:8080", "http://example.test", "http://localhost.example.test", "http://[2001:db8::1]:80", "http://198.51.100.7",
		"ftp://127.0.0.1", "127.0.0.1:8080", "http://", "http://user:hunter2@127.0.0.1:8080", "http://127.0.0.1:8080?x=1", "http://127.0.0.1:8080#x",
	} {
		vars := map[string]string{agentURLVariable: address, agentTokenVariable: server.token}
		// The addresses are the documentation ranges and reserved names, which no
		// network answers, and the deadline keeps a rule that regressed from
		// waiting on one.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		result := runCommand(ctx, []string{"agent", "read", "accounts.list"}, "", vars, nil)
		cancel()
		if result.code != exitFailure || result.stdout != "" || !strings.Contains(result.stderr, agentURLVariable) {
			t.Errorf("%q: exit %d, standard output %q, standard error %q", address, result.code, result.stdout, result.stderr)
		}
		if strings.Contains(result.stderr, "hunter2") || strings.Contains(result.stderr, server.token) {
			t.Errorf("%q: the message repeats a secret", address)
		}
	}
}

func TestTheAddressRuleAcceptsHTTPSAnywhereAndHTTPOnlyForThisMachine(t *testing.T) {
	for address, want := range map[string]bool{
		"https://example.test": true, "https://[::1]:8443/finwise": true, "https://203.0.113.9": true,
		"http://localhost:8080/": true, "http://LOCALHOST": true, "http://127.0.0.1": true, "http://127.1.2.3:8080": true,
		"http://[::1]:8080": true, "http://[::ffff:127.0.0.1]:8080": true,
		"http://example.test": false, "http://127.0.0.1.example.test": false, "http://[::1%25lo]:8080": false, "http://0.0.0.0:8080": false,
		"http://192.168.0.5:8080": false, "https://": false, "https://u@example.test": false, "file:///tmp/x": false,
	} {
		if got := usableAgentAddress(address); got != want {
			t.Errorf("%s: usable %v, want %v", address, got, want)
		}
	}
}

func TestAgentReadExitsWithUsageForWhatIsNotAReadOfTheManifestOrNotAJSONArray(t *testing.T) {
	var requests int
	var mutex sync.Mutex
	stub := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		mutex.Lock()
		requests++
		mutex.Unlock()
	}))
	defer stub.Close()
	vars := map[string]string{agentURLVariable: stub.URL, agentTokenVariable: "unused"}
	cases := map[string][]string{
		"no verb": {"agent"}, "another verb": {"agent", "write", "accounts.list"}, "no method": {"agent", "read"}, "a write": {"agent", "read", "budget.save"},
		"an evaluation": {"agent", "read", "budget.evaluateScenario"}, "the session's status": {"agent", "read", "session.status"},
		"a name the manifest lacks": {"agent", "read", "no.such"}, "an object": {"agent", "read", "accounts.list", "{}"},
		"not JSON": {"agent", "read", "accounts.list", "not json"}, "null": {"agent", "read", "accounts.list", "null"},
		"a string": {"agent", "read", "accounts.list", `"x"`}, "one too many": {"agent", "read", "accounts.list", "[]", "[]"},
		"an empty method": {"agent", "read", ""}, "the route": {"agent", "read", "/api/accounts/list"},
	}
	for label, args := range cases {
		result := runCommand(context.Background(), args, "", vars, nil)
		if result.code != exitUsage || result.stdout != "" || !strings.Contains(result.stderr, "usage: finwise agent read <method> [<json-arguments>]") {
			t.Errorf("%s: exit %d, standard output %q, standard error %q", label, result.code, result.stdout, result.stderr)
		}
	}
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range methods {
		if method.Class == contract.ClassRead {
			continue
		}
		if result := runCommand(context.Background(), []string{"agent", "read", method.Qualified()}, "", vars, nil); result.code != exitUsage {
			t.Errorf("%s (%s): exit %d, want %d", method.Qualified(), method.Class, result.code, exitUsage)
		}
	}
	mutex.Lock()
	defer mutex.Unlock()
	if requests != 0 {
		t.Errorf("%d requests were sent for what is not a read", requests)
	}
}

func TestTheClientTakesEveryReadOfTheManifestAndNoOtherMethod(t *testing.T) {
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range methods {
		_, problem := agentCallOf([]string{method.Qualified()})
		if (method.Class == contract.ClassRead) != (problem == "") {
			t.Errorf("%s (%s): problem %q", method.Qualified(), method.Class, problem)
		}
	}
}

// What a client prints about a failure is the contract's own kind or nothing:
// the plain kinds are the manifest's, which this test reads from disk.
func TestThePlainErrorKindsTheClientKnowsAreTheManifests(t *testing.T) {
	raw, err := os.ReadFile("../../internal/contract/contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Definitions map[string]struct {
			Enum []string `json:"enum"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	want := manifest.Definitions[`Exclude<ApiErrorKind,("network"|"refused")>`].Enum
	if len(want) == 0 || !slices.Equal(want, plainErrorKinds) {
		t.Errorf("the client knows %v, the manifest has %v", plainErrorKinds, want)
	}
}

func TestARawWriteWithTheTokenIsRefusedAndTheJournalHoldsNoChange(t *testing.T) {
	server := startAgentServer(t)
	database := inspectDatabase(t, server.files.databasePath)
	changes := func(query string) []string { return columnValues(t, database, query) }
	const byPerson = "SELECT COUNT(*) FROM changes WHERE author = 'person'"
	const everyChange = "SELECT COUNT(*) FROM changes"
	before, byPersonBefore := changes(everyChange), changes(byPerson)

	for _, body := range []string{`["` + firstPurchase + `","by the agent"]`, `[]`, `not json`} {
		status, answer := server.postWithHeader(t, server.token, "/api/transactions/setNote", body, nil)
		if status != http.StatusForbidden || answer != `{"error":{"kind":"read_only"}}` {
			t.Errorf("body %q: status %d answer %s, want 403 read_only", body, status, answer)
		}
	}
	for _, path := range []string{"/api/session/lock", "/api/session/unlock", "/api/session/status", "/api/budget/evaluateScenario", "/api/review/closeMonth"} {
		if status, answer := server.postWithHeader(t, server.token, path, `[]`, nil); status != http.StatusForbidden || answer != `{"error":{"kind":"read_only"}}` {
			t.Errorf("%s: status %d answer %s, want 403 read_only", path, status, answer)
		}
	}
	if after := changes(everyChange); !slices.Equal(before, after) || !slices.Equal(changes(byPerson), byPersonBefore) {
		t.Errorf("the journal holds %v changes after the refused writes, %v before", after, before)
	}

	// The same write by the person's session is recorded, which is what makes the
	// count above mean something.
	cookie, _ := server.unlockBody(t, testPIN)
	if answer := server.postAs(t, cookie, "/api/transactions/setNote", `["`+firstPurchase+`","by the person"]`); !strings.Contains(answer, "changeId") {
		t.Fatalf("the person's write answered %s", answer)
	}
	if slices.Equal(changes(byPerson), byPersonBefore) {
		t.Errorf("the person's write left the %v changes by the person as they were", byPersonBefore)
	}
}

func TestNothingTheClientOrTheServerWritesHoldsTheTokenOrAValueOfTheLedger(t *testing.T) {
	server := startAgentServer(t)
	free := unusedAddress(t)
	another, _ := agentCredentials(t)
	runs := []outcome{
		server.read("accounts.list"), server.read("transactions.detail", `["`+firstPurchase+`"]`), server.read("transactions.detail", `["no-such-id"]`),
		server.read("budget.save"), server.read("no.such"), server.read("accounts.list", "{}"), server.read(), server.read("transactions.detail"),
		runCommand(context.Background(), []string{"agent", "read", "accounts.list"}, "", server.variables(another), nil),
		runCommand(context.Background(), []string{"agent", "read", "accounts.list"}, "", map[string]string{agentURLVariable: "http://" + free, agentTokenVariable: server.token}, nil),
		runCommand(context.Background(), []string{"agent", "read", "accounts.list"}, "", map[string]string{agentURLVariable: "http://example.test", agentTokenVariable: server.token}, nil),
		runCommand(context.Background(), []string{"agent", "read", "accounts.list"}, "", map[string]string{agentURLVariable: "http://" + server.address}, nil),
		runCommand(context.Background(), []string{"agent"}, "", server.variables(server.token), nil),
	}
	for index, result := range runs {
		for _, value := range append([]string{server.token, another}, leakedValues...) {
			if strings.Contains(strings.ToLower(result.stderr), strings.ToLower(value)) {
				t.Errorf("run %d: standard error holds %q", index, value)
			}
		}
	}
	status, _ := server.postWithHeader(t, server.token, "/api/transactions/setNote", `["`+firstPurchase+`","x"]`, nil)
	if status != http.StatusForbidden {
		t.Fatalf("a refused write answered %d", status)
	}
	log := server.stopGracefully(t)
	if !strings.Contains(log.stdout, "caller=agent") {
		t.Error("the server's log does not name the agent as a caller")
	}
	for _, secret := range []string{server.token, "sha256$", another} {
		if strings.Contains(log.stdout+log.stderr, secret) {
			t.Errorf("the server's output holds %q", secret)
		}
	}
}

func TestAgentProposalEndToEndApproveUndoAndRefuse(t *testing.T) {
	server := startAgentServer(t)
	cookie, _ := server.unlockBody(t, testPIN)

	// 1. Agent proposes adding a tag.
	sub1 := server.propose(nil, "--reason", "Compra na semana da viagem", "--name", "revisor-cli", "transactions.addTag", `["`+firstPurchase+`","#viagem"]`)
	if sub1.code != 0 || sub1.stderr != "" {
		t.Fatalf("propose addTag exited %d stderr %q stdout %q", sub1.code, sub1.stderr, sub1.stdout)
	}
	var submitted1 struct {
		ProposalID string `json:"proposalId"`
		Title      string `json:"title"`
	}
	if err := json.Unmarshal([]byte(sub1.stdout), &submitted1); err != nil || submitted1.ProposalID == "" {
		t.Fatalf("unexpected submit stdout %q: %v", sub1.stdout, err)
	}

	// 2. Session sees the proposal in review.weekly with its proposer.
	weekly := server.postAs(t, cookie, "/api/review/weekly", `["2026-11"]`)
	if !strings.Contains(weekly, `"proposer":"revisor-cli"`) || !strings.Contains(weekly, submitted1.Title) {
		t.Errorf("review.weekly did not list proposal: %s", weekly)
	}

	// 3. Agent token cannot approve it.
	if status, answer := server.postWithHeader(t, server.token, "/api/review/approve", `["`+submitted1.ProposalID+`"]`, nil); status != http.StatusForbidden || answer != `{"error":{"kind":"read_only"}}` {
		t.Errorf("approve with token: %d %s, want 403 read_only", status, answer)
	}

	// 4. Session approves it, records change under model, adds history note.
	approvedBody := server.postAs(t, cookie, "/api/review/approve", `["`+submitted1.ProposalID+`"]`)
	var approved struct {
		ChangeID string `json:"changeId"`
		Title    string `json:"title"`
	}
	if err := json.Unmarshal([]byte(approvedBody), &approved); err != nil || approved.ChangeID == "" {
		t.Fatalf("approve answered %s: %v", approvedBody, err)
	}
	detail := server.postAs(t, cookie, "/api/transactions/detail", `["`+firstPurchase+`"]`)
	if !strings.Contains(detail, "Proposto por revisor-cli · você aprovou") {
		t.Errorf("detail missing approval history note: %s", detail)
	}

	// 5. Agent reads proposals and sees approved with changeId and undone: false.
	list1 := server.proposals()
	if list1.code != 0 || list1.stderr != "" {
		t.Fatalf("proposals exited %d stderr %q", list1.code, list1.stderr)
	}
	if !strings.Contains(list1.stdout, `"outcome":"approved"`) || !strings.Contains(list1.stdout, `"changeId":"`+approved.ChangeID+`"`) || !strings.Contains(list1.stdout, `"undone":false`) {
		t.Errorf("proposals after approve = %s", list1.stdout)
	}

	// 6. Session undoes the approved change -> proposals now reports undone: true.
	if undo := server.postAs(t, cookie, "/api/changes/undo", `["`+approved.ChangeID+`"]`); undo != "null" {
		t.Fatalf("undo answered %s, want null", undo)
	}
	list2 := server.proposals()
	if list2.code != 0 || !strings.Contains(list2.stdout, `"undone":true`) {
		t.Errorf("proposals after undo = %s", list2.stdout)
	}

	// 7. Agent proposes a note using FINWISE_AGENT_NAME, session refuses with a reason, agent reads reason back.
	sub2 := server.propose(map[string]string{agentNameVariable: "agente-env"}, "--reason", "Lembrete útil", "transactions.setNote", `["`+firstPurchase+`","Nota sugerida"]`)
	if sub2.code != 0 || sub2.stderr != "" {
		t.Fatalf("propose setNote exited %d stderr %q", sub2.code, sub2.stderr)
	}
	var submitted2 struct {
		ProposalID string `json:"proposalId"`
		Title      string `json:"title"`
	}
	if err := json.Unmarshal([]byte(sub2.stdout), &submitted2); err != nil || submitted2.ProposalID == "" {
		t.Fatalf("unexpected second submit stdout %q", sub2.stdout)
	}
	refusedBody := server.postAs(t, cookie, "/api/review/refuse", `["`+submitted2.ProposalID+`","Não precisa dessa nota"]`)
	var refused struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(refusedBody), &refused); err != nil || refused.Title != submitted2.Title {
		t.Errorf("refuse answered %s, want title %q", refusedBody, submitted2.Title)
	}
	list3 := server.proposals()
	if list3.code != 0 || !strings.Contains(list3.stdout, `"outcome":"refused"`) || !strings.Contains(list3.stdout, `"refusal":"Não precisa dessa nota"`) || !strings.Contains(list3.stdout, `"proposer":"agente-env"`) {
		t.Errorf("proposals after refuse = %s", list3.stdout)
	}

	// 8. A refused submission reports only fixed contract words on stderr, leaking nothing.
	missing := server.propose(nil, "--reason", "Teste", "--name", "revisor", "transactions.addTag", `["no-such-tx","#x"]`)
	if missing.code != exitFailure || missing.stdout != "" || missing.stderr != "finwise: review.submitProposal answered refused (not_found)\n" {
		t.Errorf("refused propose: exit %d stdout %q stderr %q", missing.code, missing.stdout, missing.stderr)
	}
	for _, run := range []outcome{sub1, list1, list2, sub2, list3, missing} {
		for _, value := range append([]string{server.token}, leakedValues...) {
			if strings.Contains(strings.ToLower(run.stderr), strings.ToLower(value)) {
				t.Errorf("stderr leaked %q: %q", value, run.stderr)
			}
		}
	}

	log := server.stopGracefully(t)
	for _, secret := range []string{server.token, "sha256$", ledgerSentinel} {
		if strings.Contains(log.stdout+log.stderr, secret) {
			t.Errorf("server output leaked %q", secret)
		}
	}
}

// unusedAddress is a loopback address nothing listens on: it was free a moment ago.
func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}
