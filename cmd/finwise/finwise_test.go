package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
)

const testPIN = "123456"

// testNow is 22:30 on 30 November 2026 in Brazil, the clock of every command a
// test runs.
var testNow = time.Date(2026, 12, 1, 1, 30, 0, 0, time.UTC)

var (
	hashOnce   sync.Once
	storedHash string
)

type outcome struct {
	code           int
	stdout, stderr string
}

func runCommand(ctx context.Context, args []string, stdin string, vars map[string]string, listening func(string)) outcome {
	return runCommandWith(ctx, args, stdin, vars, listening, ingest.Run)
}

// runCommandWith is runCommand with the ingest the command runs.
func runCommandWith(ctx context.Context, args []string, stdin string, vars map[string]string, listening func(string), runIngest ingestFunc) outcome {
	var stdout, stderr bytes.Buffer
	code := run(ctx, args, environment{
		stdin:     strings.NewReader(stdin),
		stdout:    &stdout,
		stderr:    &stderr,
		getenv:    func(name string) string { return vars[name] },
		now:       func() time.Time { return testNow },
		listening: listening,
		runIngest: runIngest,
	})
	return outcome{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// hashOf is the string `finwise hash-pin` prints for the test PIN, made once.
func hashOf(t *testing.T) string {
	t.Helper()
	hashOnce.Do(func() {
		result := runCommand(context.Background(), []string{"hash-pin"}, testPIN+"\n", nil, nil)
		if result.code != 0 {
			t.Fatalf("hash-pin exited %d: %s", result.code, result.stderr)
		}
		storedHash = strings.TrimSpace(result.stdout)
	})
	return storedHash
}

func TestHashPINRefusesAnythingButSixDigits(t *testing.T) {
	for _, input := range []string{"12345\n", "abcdef\n", "1234567\n", "\n", "", "12345a"} {
		result := runCommand(context.Background(), []string{"hash-pin"}, input, nil, nil)
		if result.code == 0 || result.stdout != "" {
			t.Errorf("%q: exit %d, stdout %q", input, result.code, result.stdout)
		}
		if trimmed := strings.TrimSpace(input); trimmed != "" && strings.Contains(result.stderr, trimmed) {
			t.Errorf("%q: the message repeats the input", input)
		}
	}
}

func TestHashPINAcceptsLineEndings(t *testing.T) {
	for _, input := range []string{"123456", "123456\n", "123456\r\n"} {
		if result := runCommand(context.Background(), []string{"hash-pin"}, input, nil, nil); result.code != 0 {
			t.Errorf("%q: exit %d: %s", input, result.code, result.stderr)
		}
	}
}

func TestHashPINPrintsOneLineOfTheStoredForm(t *testing.T) {
	stored := hashOf(t)
	if !strings.HasPrefix(stored, "pbkdf2-sha256$600000$") || strings.Count(stored, "$") != 3 || strings.Contains(stored, " ") {
		t.Errorf("unexpected shape: %d characters, %d fields", len(stored), strings.Count(stored, "$")+1)
	}
}

// refusalVars adds a free port to a refusal case: a server that wrongly
// started would otherwise bind 8080, fail for an unrelated reason on a busy
// machine and pass the test by accident.
func refusalVars(vars map[string]string) map[string]string {
	vars["FINWISE_LISTEN"] = "127.0.0.1:0"
	return vars
}

// cancelledContext makes a server that does start return at once, with code 0,
// which the refusal tests read as the failure it is.
func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestServeRefusesToStartWithoutAUsableHash(t *testing.T) {
	const garbage = "not-a-hash-sentinel"
	cases := map[string]map[string]string{
		"missing":   {"FINWISE_LEDGER": "x"},
		"malformed": {"FINWISE_LEDGER": "x", "FINWISE_PIN_HASH": garbage},
	}
	for name, vars := range cases {
		result := runCommand(cancelledContext(), []string{"serve"}, "", refusalVars(vars), nil)
		if result.code == 0 {
			t.Errorf("%s: exited 0", name)
		}
		if strings.Contains(result.stderr+result.stdout, garbage) {
			t.Errorf("%s: the output repeats the value", name)
		}
	}
}

func TestServeRefusesWithoutALedgerPathOrWithABadTTL(t *testing.T) {
	hash := hashOf(t)
	cases := map[string]map[string]string{
		"no ledger":    {"FINWISE_PIN_HASH": hash},
		"bad ttl":      {"FINWISE_PIN_HASH": hash, "FINWISE_LEDGER": "x", "FINWISE_SESSION_TTL": "soon"},
		"zero ttl":     {"FINWISE_PIN_HASH": hash, "FINWISE_LEDGER": "x", "FINWISE_SESSION_TTL": "0s"},
		"negative ttl": {"FINWISE_PIN_HASH": hash, "FINWISE_LEDGER": "x", "FINWISE_SESSION_TTL": "-1h"},
	}
	for name, vars := range cases {
		result := runCommand(cancelledContext(), []string{"serve"}, "", refusalVars(vars), nil)
		if result.code == 0 {
			t.Errorf("%s: exited 0", name)
		}
		if strings.Contains(result.stderr+result.stdout, hash) {
			t.Errorf("%s: the output repeats the hash", name)
		}
	}
}

func TestUnknownCommandsAndFlagsExitNonZero(t *testing.T) {
	for _, args := range [][]string{nil, {"nope"}, {"serve", "--nope"}} {
		if result := runCommand(context.Background(), args, "", nil, nil); result.code == 0 {
			t.Errorf("%v: exited 0", args)
		}
	}
}

type runningServer struct {
	address string
	done    chan outcome
	stop    context.CancelFunc
}

func startServer(t *testing.T, args []string, vars map[string]string) *runningServer {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	server := &runningServer{done: make(chan outcome, 1), stop: stop}
	ready := make(chan string, 1)
	go func() {
		server.done <- runCommand(ctx, args, "", vars, func(address string) { ready <- address })
	}()
	select {
	case server.address = <-ready:
	case result := <-server.done:
		t.Fatalf("serve exited %d before listening: %s", result.code, result.stderr)
	case <-time.After(30 * time.Second):
		t.Fatal("serve did not start")
	}
	t.Cleanup(stop)
	return server
}

func (server *runningServer) post(t *testing.T, path, body string) (*http.Response, string) {
	t.Helper()
	response, err := http.Post("http://"+server.address+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	text, _ := io.ReadAll(response.Body)
	return response, string(text)
}

func TestServeStartsWithoutASnapshotAndTheHashUnlocks(t *testing.T) {
	vars := map[string]string{"FINWISE_PIN_HASH": hashOf(t)}
	server := startServer(t, []string{"serve", "--listen", "127.0.0.1:0", "--ledger", "/nonexistent/pluggy-snapshot.db"}, vars)

	health, err := http.Get("http://" + server.address + "/healthz")
	if err != nil || health.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %v %v", err, health)
	}
	health.Body.Close()

	_, wrong := server.post(t, "/api/session/unlock", `["654321"]`)
	if wrong != `{"ok":false,"reason":"wrong-pin"}` {
		t.Errorf("a wrong PIN answered %s", wrong)
	}
	response, right := server.post(t, "/api/session/unlock", `["`+testPIN+`"]`)
	if right != `{"ok":true}` {
		t.Errorf("the PIN the hash was made from answered %s", right)
	}
	if cookies := response.Header.Values("Set-Cookie"); len(cookies) != 1 || !strings.HasPrefix(cookies[0], "__Host-finwise-session=") {
		t.Errorf("Set-Cookie: %d lines", len(cookies))
	}

	server.stop()
	select {
	case result := <-server.done:
		if result.code != 0 {
			t.Errorf("graceful shutdown exited %d: %s", result.code, result.stderr)
		}
		for _, secret := range []string{testPIN, "654321", vars["FINWISE_PIN_HASH"]} {
			if strings.Contains(result.stdout+result.stderr, secret) {
				t.Errorf("the output holds %q", secret)
			}
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the server did not stop")
	}
}

func TestDevelopmentFlagDropsTheCookiePrefix(t *testing.T) {
	vars := map[string]string{"FINWISE_PIN_HASH": hashOf(t), "FINWISE_LEDGER": "/nonexistent", "FINWISE_LISTEN": "127.0.0.1:0"}
	server := startServer(t, []string{"serve", "--dev-insecure-cookie"}, vars)
	response, _ := server.post(t, "/api/session/unlock", `["`+testPIN+`"]`)
	cookie := response.Header.Get("Set-Cookie")
	if !strings.HasPrefix(cookie, "finwise-session=") || strings.Contains(cookie, "Secure") {
		t.Errorf("cookie line starts %q and Secure=%v", cookie[:min(len(cookie), 16)], strings.Contains(cookie, "Secure"))
	}
}

func TestEnvironmentSuppliesTheAddressAndALongerSession(t *testing.T) {
	vars := map[string]string{
		"FINWISE_PIN_HASH": hashOf(t), "FINWISE_LEDGER": "/nonexistent",
		"FINWISE_LISTEN": "127.0.0.1:0", "FINWISE_SESSION_TTL": "2h",
	}
	server := startServer(t, []string{"serve"}, vars)
	response, _ := server.post(t, "/api/session/unlock", `["`+testPIN+`"]`)
	if cookie := response.Header.Get("Set-Cookie"); !strings.Contains(cookie, "Max-Age=7200") {
		t.Errorf("the TTL did not reach the cookie")
	}
}

const (
	changedTestPIN = "135790"
	storedLogLine  = "the PIN stored in the database opens the app; FINWISE_PIN_HASH is ignored"
)

// postAs posts with the session cookie a first call gave.
func (server *runningServer) postAs(t *testing.T, cookie *http.Cookie, path, body string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "http://"+server.address+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	text, _ := io.ReadAll(response.Body)
	return string(text)
}

func (server *runningServer) unlockBody(t *testing.T, pin string) (*http.Cookie, string) {
	t.Helper()
	response, body := server.post(t, "/api/session/unlock", `["`+pin+`"]`)
	if cookies := response.Cookies(); len(cookies) == 1 {
		return cookies[0], body
	}
	return nil, body
}

func TestAChangedPINOpensTheAppAfterARestartAndTheEnvironmentsHashIsIgnored(t *testing.T) {
	directory := t.TempDir()
	args := []string{"serve", "--dev-insecure-cookie", "--db", filepath.Join(directory, "finwise.db")}
	vars := serveVars(t)

	first := startServer(t, args, vars)
	cookie, body := first.unlockBody(t, testPIN)
	if body != `{"ok":true}` || cookie == nil {
		t.Fatalf("the environment's PIN did not unlock a fresh database: %s", body)
	}
	if answer := first.postAs(t, cookie, "/api/session/changePin", `["`+testPIN+`","`+changedTestPIN+`","`+changedTestPIN+`"]`); answer != `{"ok":true}` {
		t.Fatalf("the change answered %s", answer)
	}
	firstRun := first.stopGracefully(t)
	if strings.Contains(firstRun.stdout, storedLogLine) {
		t.Error("a fresh database logged that a stored PIN opens the app")
	}

	second := startServer(t, args, vars)
	if _, body := second.unlockBody(t, changedTestPIN); body != `{"ok":true}` {
		t.Errorf("the changed PIN answered %s after a restart", body)
	}
	if _, body := second.unlockBody(t, testPIN); body != `{"ok":false,"reason":"wrong-pin"}` {
		t.Errorf("the old PIN answered %s after a restart", body)
	}
	secondRun := second.stopGracefully(t)
	if count := strings.Count(secondRun.stdout, storedLogLine); count != 1 {
		t.Errorf("the start logged the line %d times, want once", count)
	}
	for _, secret := range []string{testPIN, changedTestPIN, vars["FINWISE_PIN_HASH"]} {
		if strings.Contains(firstRun.stdout+firstRun.stderr+secondRun.stdout+secondRun.stderr, secret) {
			t.Error("the output holds a PIN or the environment's hash")
		}
	}
}

func TestAStoredHashThatDoesNotParseStopsTheStartWithoutNamingIt(t *testing.T) {
	const garbage = "stored-garbage-sentinel"
	path := filepath.Join(t.TempDir(), "finwise.db")
	database, err := authored.Open(t.Context(), path, authored.Options{Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.StorePINHash(t.Context(), garbage, testNow); err != nil {
		t.Fatal(err)
	}
	database.Close()

	vars := refusalVars(serveVars(t))
	result := runCommand(cancelledContext(), []string{"serve", "--db", path}, "", vars, nil)
	if result.code == 0 {
		t.Fatal("serve started over a stored hash that does not parse")
	}
	if !strings.Contains(result.stderr, path) {
		t.Error("the message does not name the database")
	}
	if text := result.stdout + result.stderr; strings.Contains(text, garbage) || strings.Contains(text, storedLogLine) {
		t.Error("the output holds the stored value or says the stored PIN opens the app")
	}
}
