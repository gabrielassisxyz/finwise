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
	echo := func(_ context.Context, args []json.RawMessage, _ *ledger.Reader) (any, error) {
		return map[string]string{"echo": string(args[0])}, nil
	}
	failing := func(context.Context, []json.RawMessage, *ledger.Reader) (any, error) {
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
	failing := func(context.Context, []json.RawMessage, *ledger.Reader) (any, error) {
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
