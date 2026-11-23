package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// A snapshot file that is there but is not a database must not look like a bug
// of the server: the person is told the ledger is unavailable.
func TestEveryReadAnswersLedgerUnavailableForAFileThatIsNotADatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pluggy-snapshot.db")
	if err := os.WriteFile(path, []byte("not a SQLite database, only text long enough to fill a header"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := newTestEnv(t, withLedger(path, fixtureNow))
	cookie := env.unlock()

	for _, c := range readCases {
		domain, name := splitMethod(t, c.method)
		response := env.call("/api/"+domain+"/"+name, c.arguments, cookie)
		// review.proposals reads only the app's database and never opens the ledger.
		if c.method == "review.proposals" {
			if response.Code != http.StatusOK || response.Body.String() != "[]" {
				t.Errorf("%s %s: status %d body %s, want 200 []", c.method, c.arguments, response.Code, response.Body.String())
			}
			continue
		}
		expectError(t, c.method+" "+c.arguments, response, http.StatusServiceUnavailable, "ledger_unavailable")
	}
}
