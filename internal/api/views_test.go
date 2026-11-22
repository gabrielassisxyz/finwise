package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// twoReads is a handler that asks the same question twice and runs between
// in the middle, where a test changes the store under the request.
func twoReads[T any](ask func(context.Context, Views) (T, error), between func()) (ReadHandler, *[2]T) {
	answers := new([2]T)
	handler := func(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
		first, err := ask(ctx, views)
		if err != nil {
			return nil, err
		}
		between()
		second, err := ask(ctx, views)
		if err != nil {
			return nil, err
		}
		answers[0], answers[1] = first, second
		return nil, nil
	}
	return handler, answers
}

func accountCount(ctx context.Context, views Views) (int, error) {
	accounts, err := views.Ledger.Accounts(ctx)
	return len(accounts), err
}

func trackingStartOf(ctx context.Context, views Views) (string, error) {
	month, _, err := views.Authored.TrackingStartMonth(ctx)
	return string(month), err
}

func expectOK(t *testing.T, label string, response interface{ Result() *http.Response }) {
	t.Helper()
	if status := response.Result().StatusCode; status != http.StatusOK {
		t.Fatalf("%s: status %d, want 200", label, status)
	}
}

// The collector renames a new snapshot over the file while a request is
// between two of its queries: the request ends on the snapshot it began on.
func TestASnapshotSwappedDuringARequestIsSeenOnlyByTheNextOne(t *testing.T) {
	first := newFullLedger(t)
	before, err := viewOf(t, first.Path).Accounts(t.Context())
	if err != nil || len(before) == 0 {
		t.Fatalf("the first ledger has %d accounts (%v); the swap could not show", len(before), err)
	}
	swapped := false
	swap := func() {
		if swapped {
			return
		}
		swapped = true
		replacement := ledgertest.NewAt(t, first.Path+".next")
		if err := os.Rename(replacement.Path, first.Path); err != nil {
			t.Fatal(err)
		}
	}
	handler, answers := twoReads(accountCount, swap)
	env := newTestEnv(t, withRead("shell.overview", handler), func(config *Config) { config.Ledger = ledger.New(first.Path) })
	cookie := env.unlock()

	expectOK(t, "the request across the swap", env.call("/api/shell/overview", `[]`, cookie))
	if answers[0] != len(before) || answers[1] != len(before) {
		t.Errorf("across the swap the request saw %d then %d accounts, want %d both times", answers[0], answers[1], len(before))
	}
	expectOK(t, "the next request", env.call("/api/shell/overview", `[]`, cookie))
	if answers[0] != 0 || answers[1] != 0 {
		t.Errorf("the next request saw %d and %d accounts, want the new snapshot's 0", answers[0], answers[1])
	}
}

// openAppDatabase is a scratch finwise.db, closed when the test ends.
func openAppDatabase(t *testing.T) (*authored.Database, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finwise.db")
	database, err := authored.Open(t.Context(), path, authored.Options{Now: func() time.Time { return fixtureNow }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database, path
}

const moveTrackingStart = `UPDATE meta SET value = '2020-01' WHERE key = 'tracking_start_month'`

// commitFromAnotherConnection changes the app's database outside the handle
// the server holds, as another process would.
func commitFromAnotherConnection(t *testing.T, path string) {
	t.Helper()
	query := url.Values{}
	query.Add("_pragma", "busy_timeout(5000)")
	other, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.ExecContext(t.Context(), moveTrackingStart); err != nil {
		t.Fatal(err)
	}
}

func TestACommitToTheAppDatabaseDuringARequestIsSeenOnlyByTheNextOne(t *testing.T) {
	database, path := openAppDatabase(t)
	committed := false
	commit := func() {
		if !committed {
			committed = true
			commitFromAnotherConnection(t, path)
		}
	}
	handler, answers := twoReads(trackingStartOf, commit)
	env := newTestEnv(t, withRead("shell.overview", handler), func(config *Config) { config.Database = database })
	cookie := env.unlock()

	expectOK(t, "the request across the commit", env.call("/api/shell/overview", `[]`, cookie))
	created := string(brday.MonthOf(fixtureNow))
	if answers[0] != created || answers[1] != created {
		t.Errorf("across the commit the request read %q then %q, want %q both times", answers[0], answers[1], created)
	}
	expectOK(t, "the next request", env.call("/api/shell/overview", `[]`, cookie))
	if answers[0] != "2020-01" || answers[1] != "2020-01" {
		t.Errorf("the next request read %q and %q, want the committed 2020-01", answers[0], answers[1])
	}
}

// Whichever way a handler ends, the ledger handle it opened is closed and the
// app database's connection is back in its pool.
func TestEveryEndingOfARequestReleasesBothViews(t *testing.T) {
	endings := map[string]struct {
		end    func() (any, error)
		status int
	}{
		"answered": {func() (any, error) { return map[string]int{"n": 1}, nil }, http.StatusOK},
		"failed":   {func() (any, error) { return nil, errors.New("boom") }, http.StatusInternalServerError},
		"panicked": {func() (any, error) { panic("boom") }, http.StatusInternalServerError},
	}
	for name, ending := range endings {
		t.Run(name, func(t *testing.T) {
			database, _ := openAppDatabase(t)
			var held Views
			inUseDuring := -1
			handler := func(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
				held = views
				if _, err := views.Ledger.Accounts(ctx); err != nil {
					return nil, err
				}
				if _, _, err := views.Authored.TrackingStartMonth(ctx); err != nil {
					return nil, err
				}
				inUseDuring = database.ConnectionsInUse()
				return ending.end()
			}
			env := newTestEnv(t, withRead("shell.overview", handler), func(config *Config) {
				config.Ledger = ledger.New(newFullLedger(t).Path)
				config.Database = database
			})
			if response := env.call("/api/shell/overview", `[]`, env.unlock()); response.Code != ending.status {
				t.Fatalf("status %d, want %d", response.Code, ending.status)
			}
			if inUseDuring != 1 {
				t.Fatalf("during the request the pool lent %d connections, want 1", inUseDuring)
			}
			if inUse := database.ConnectionsInUse(); inUse != 0 {
				t.Errorf("after the request the pool still lends %d connections", inUse)
			}
			if _, err := held.Ledger.Accounts(t.Context()); !errors.Is(err, ledger.ErrViewClosed) {
				t.Errorf("the request's ledger view answered %v after it ended, want ErrViewClosed", err)
			}
			if _, _, err := held.Authored.TrackingStartMonth(t.Context()); !errors.Is(err, authored.ErrViewClosed) {
				t.Errorf("the request's authored view answered %v after it ended, want ErrViewClosed", err)
			}
		})
	}
}

// A handler that asks the ledger nothing does not depend on the snapshot being
// there; one that asks it something learns that it is gone.
func TestOnlyAHandlerThatAsksTheLedgerFailsWhenTheSnapshotIsMissing(t *testing.T) {
	asks := func(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
		return accountCount(ctx, views)
	}
	missing := filepath.Join(t.TempDir(), "pluggy-snapshot.db")
	env := newTestEnv(t, withRead("shell.overview", returnsNothing), withRead("accounts.list", asks),
		func(config *Config) { config.Ledger = ledger.New(missing) })
	cookie := env.unlock()

	response := env.call("/api/shell/overview", `[]`, cookie)
	if response.Code != http.StatusOK || response.Body.String() != "null" {
		t.Errorf("a handler that asks nothing: status %d body %q, want 200 null", response.Code, response.Body.String())
	}
	expectError(t, "a handler that asks", env.call("/api/accounts/list", `[]`, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
}
