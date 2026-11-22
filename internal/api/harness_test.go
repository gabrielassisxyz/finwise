package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// withLedger serves every registered read handler over the snapshot at path,
// with the server's clock fixed at now.
func withLedger(path string, now time.Time) envOption {
	return func(config *Config) {
		config.Reads = NewReadRegistry()
		config.Ledger = ledger.New(path)
		config.Now = func() time.Time { return now }
	}
}

// viewOf is a view of the snapshot at path, closed when the test ends.
func viewOf(t testing.TB, path string) *ledger.View {
	t.Helper()
	view := ledger.New(path).View()
	t.Cleanup(func() { view.Close() })
	return view
}

// readEnv is a router over a fixture ledger with a session already open.
type readEnv struct {
	*testEnv
	cookie *http.Cookie
}

func newReadEnv(t *testing.T, fixture *ledgertest.Fixture) *readEnv {
	t.Helper()
	env := newTestEnv(t, withLedger(fixture.Path, fixtureNow))
	return &readEnv{testEnv: env, cookie: env.unlock()}
}

// answer calls a read method with the arguments as the JSON array a client posts.
func (env *readEnv) answer(method, arguments string) (status int, body []byte) {
	env.t.Helper()
	domain, name := splitMethod(env.t, method)
	response := env.call("/api/"+domain+"/"+name, arguments, env.cookie)
	return response.Code, response.Body.Bytes()
}

func splitMethod(t *testing.T, method string) (domain, name string) {
	t.Helper()
	for index := range method {
		if method[index] == '.' {
			return method[:index], method[index+1:]
		}
	}
	t.Fatalf("%q is not domain.name", method)
	return "", ""
}
