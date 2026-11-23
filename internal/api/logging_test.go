package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

const (
	bodySentinel  = "SENTINEL-BODY-5e1f"
	pinSentinel   = "SENTINEL-PIN-77c2"
	pathSentinel  = "SENTINEL-PATH-9a0b"
	rowIDSentinel = "SENTINEL-ROW-31d8"
	errorSentinel = "SENTINEL-ERR-c04e"
)

func TestTheLogHoldsNoBodyPINCookiePathOrErrorText(t *testing.T) {
	echo := func(_ context.Context, args []json.RawMessage, _ Views) (any, error) {
		return map[string]string{"echo": string(args[0])}, nil
	}
	failing := func(context.Context, []json.RawMessage, Views) (any, error) {
		return nil, errors.New("row " + rowIDSentinel + " " + errorSentinel)
	}
	env := newTestEnv(t, withRead("transactions.detail", echo), withRead("shell.overview", failing))
	cookie := env.unlock()

	env.call("/api/transactions/detail", `["`+bodySentinel+`"]`, cookie)
	env.call("/api/session/unlock", `["`+pinSentinel+`"]`, cookie)
	env.call("/api/shell/overview", `[]`, cookie)
	env.call("/api/"+pathSentinel+"/x?q="+bodySentinel, `[]`, cookie)
	env.do(request{path: "/api/transactions/detail", body: `["` + bodySentinel + `"]`, cookie: cookie,
		headers: map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + bodySentinel}})

	logged := env.logs.String()
	for _, secret := range []string{bodySentinel, pinSentinel, pathSentinel, rowIDSentinel, errorSentinel, cookie.Value, testPIN} {
		if strings.Contains(logged, secret) {
			t.Errorf("the log contains %q", secret)
		}
	}
}

func TestTheLogLineCarriesVerbRouteStatusDurationAndKind(t *testing.T) {
	failing := func(context.Context, []json.RawMessage, Views) (any, error) {
		return nil, &ledger.UnsupportedVersionError{Found: 3}
	}
	env := newTestEnv(t, withRead("shell.overview", failing))
	cookie := env.unlock()
	env.logs.Reset()
	env.call("/api/shell/overview", `[]`, cookie)
	env.do(request{verb: http.MethodGet, path: "/api/nope/nothing"})

	lines := strings.Split(strings.TrimSpace(env.logs.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d log lines for 2 requests", len(lines))
	}
	for _, want := range []string{"verb=POST", "route=/api/shell/overview", "status=503", "duration_ms=", "error_kind=ledger_unavailable"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("first line lacks %q: %s", want, lines[0])
		}
	}
	for _, want := range []string{"verb=GET", "route=unmatched", "status=404", "error_kind=unknown_method"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("second line lacks %q: %s", want, lines[1])
		}
	}
}

func TestTheLogNamesTheAgentAsCallerForARequestItsTokenOpenedAndHoldsNoToken(t *testing.T) {
	token, hash := newTokenAndHash(t)
	wrong, _ := newTokenAndHash(t)
	env := newTestEnv(t, withAgentToken(hash), withEveryReadAnswered(map[string]int{}))
	cookie := env.unlock()
	env.logs.Reset()

	env.callWith("/api/shell/overview", `[]`, bearer(token), nil)
	env.callWith("/api/transactions/setNote", `["a","b"]`, bearer(token), nil)
	env.callWith("/api/session/status", `[]`, bearer(token), nil)
	env.callWith("/api/shell/overview", `[]`, bearer(wrong), nil)
	env.callWith("/api/shell/overview", `[]`, bearer(wrong), cookie)
	env.call("/api/shell/overview", `[]`, cookie)
	env.call("/api/shell/overview", `[]`, nil)

	lines := strings.Split(strings.TrimSpace(env.logs.String()), "\n")
	if len(lines) != 7 {
		t.Fatalf("%d log lines for 7 requests", len(lines))
	}
	wantAgent := []bool{true, true, true, false, false, false, false}
	for index, line := range lines {
		if got := strings.Contains(line, "caller=agent"); got != wantAgent[index] {
			t.Errorf("line %d has caller=agent %v, want %v: %s", index, got, wantAgent[index], line)
		}
		if strings.Contains(line, "caller=") && !strings.Contains(line, "caller=agent") {
			t.Errorf("line %d names another caller: %s", index, line)
		}
	}
	for _, want := range []string{"status=200", "route=/api/shell/overview"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("the agent's read line lacks %q: %s", want, lines[0])
		}
	}
	if !strings.Contains(lines[1], "status=403") || !strings.Contains(lines[1], "error_kind=read_only") {
		t.Errorf("the agent's refused write line is not a 403 read_only: %s", lines[1])
	}
	for _, secret := range []string{token, wrong, cookie.Value, "sha256$"} {
		if strings.Contains(env.logs.String(), secret) {
			t.Error("the log holds a token, a cookie or a hash")
		}
	}
}
