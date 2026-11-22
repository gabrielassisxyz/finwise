package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

const (
	changePinRoute = "/api/session/changePin"
	newTestPIN     = "135790"
	thirdTestPIN   = "246802"
	wrongTestPIN   = "000000"
)

// pinEnv is a router over a scratch app database whose store of the PIN a test
// can make fail.
type pinEnv struct {
	*testEnv
	scratch *authoredtest.Scratch
	store   *flakyPINStore
}

type flakyPINStore struct {
	inner  PINStore
	mutex  sync.Mutex
	failed bool
}

func (store *flakyPINStore) failNext() {
	store.mutex.Lock()
	store.failed = true
	store.mutex.Unlock()
}

func (store *flakyPINStore) StorePINHash(ctx context.Context, hash string, at time.Time) error {
	store.mutex.Lock()
	failed := store.failed
	store.mutex.Unlock()
	if failed {
		return context.DeadlineExceeded
	}
	return store.inner.StorePINHash(ctx, hash, at)
}

func newPinEnv(t *testing.T, options ...envOption) *pinEnv {
	t.Helper()
	scratch := authoredtest.Open(t, newFakeClock().Now)
	store := &flakyPINStore{inner: scratch.Database}
	all := append([]envOption{withDatabase(scratch.Database), func(config *Config) { config.PINStore = store }}, options...)
	return &pinEnv{testEnv: newTestEnv(t, all...), scratch: scratch, store: store}
}

// session signs in with pin and returns the cookie.
func (env *testEnv) session(pin string) *http.Cookie {
	env.t.Helper()
	response := env.call("/api/session/unlock", `["`+pin+`"]`, nil)
	for _, cookie := range response.Result().Cookies() {
		if cookie.MaxAge > 0 {
			return &http.Cookie{Name: cookie.Name, Value: cookie.Value}
		}
	}
	env.t.Fatalf("no session: status %d body %s", response.Code, response.Body.String())
	return nil
}

func (env *testEnv) change(cookie *http.Cookie, current, next, repeat string) *httptest.ResponseRecorder {
	env.t.Helper()
	return env.call(changePinRoute, `["`+current+`","`+next+`","`+repeat+`"]`, cookie)
}

func (env *testEnv) isLocked(cookie *http.Cookie) bool {
	env.t.Helper()
	return decodeBody(env.t, env.call("/api/session/status", `[]`, cookie))["locked"] == true
}

func (env *testEnv) expectFault(label string, response *httptest.ResponseRecorder, reason string) {
	env.t.Helper()
	want := `{"ok":false,"reason":"` + reason + `"}`
	if response.Code != http.StatusOK || response.Body.String() != want {
		env.t.Errorf("%s: %d %s, want 200 %s", label, response.Code, response.Body.String(), want)
	}
}

func (env *pinEnv) storedRows() int {
	env.t.Helper()
	var rows int
	if err := env.scratch.Inspect(env.t).QueryRowContext(env.t.Context(), "SELECT count(*) FROM pin").Scan(&rows); err != nil {
		env.t.Fatal(err)
	}
	return rows
}

func (env *pinEnv) expectJournalUntouched() {
	env.t.Helper()
	if changes := env.scratch.Changes(env.t); len(changes) != 0 || env.scratch.EffectCount(env.t) != 0 {
		env.t.Errorf("the journal gained %d changes and %d effects", len(changes), env.scratch.EffectCount(env.t))
	}
}

func expectOneClearingCookie(t *testing.T, label string, response *httptest.ResponseRecorder) {
	t.Helper()
	lines := response.Header().Values("Set-Cookie")
	if len(lines) != 1 {
		t.Fatalf("%s: %d Set-Cookie lines, want exactly one", label, len(lines))
	}
	if !strings.HasPrefix(lines[0], hostCookieName+"=;") || !strings.Contains(lines[0], "Max-Age=0") {
		t.Errorf("%s: the cookie is not the clearing one", label)
	}
}

func TestChangingThePINAnswersOkAndStoresOneHashAndNoJournal(t *testing.T) {
	env := newPinEnv(t)
	cookie := env.unlock()
	env.clock.Advance(3 * time.Hour)
	response := env.change(cookie, testPIN, newTestPIN, newTestPIN)
	if response.Code != http.StatusOK || response.Body.String() != `{"ok":true}` {
		t.Fatalf("answered %d %s", response.Code, response.Body.String())
	}
	var hash, changedAt string
	row := env.scratch.Inspect(t).QueryRowContext(t.Context(), "SELECT hash, changed_at FROM pin")
	if err := row.Scan(&hash, &changedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePINHash(hash); err != nil {
		t.Errorf("the stored hash is not one: %v", err)
	}
	if strings.Contains(hash, newTestPIN) || changedAt != "2026-01-05T12:00:00.000Z" {
		t.Errorf("changed_at %s, want the test clock's instant 2026-01-05T12:00:00.000Z", changedAt)
	}
	if env.storedRows() != 1 {
		t.Errorf("pin holds %d rows, want 1", env.storedRows())
	}
	env.expectJournalUntouched()
}

func TestTheNewPINOpensTheAppAtOnceWithoutARestartAndTheOldOneDoesNot(t *testing.T) {
	env := newPinEnv(t)
	cookie := env.unlock()
	env.change(cookie, testPIN, newTestPIN, newTestPIN)
	env.call("/api/session/lock", `[]`, cookie)
	env.expectFault("the old PIN", env.call("/api/session/unlock", `["`+testPIN+`"]`, nil), "wrong-pin")
	if body := env.call("/api/session/unlock", `["`+newTestPIN+`"]`, nil).Body.String(); body != `{"ok":true}` {
		t.Errorf("the new PIN answered %s", body)
	}
	again := env.session(newTestPIN)
	env.expectFault("the old PIN as current", env.change(again, testPIN, thirdTestPIN, thirdTestPIN), "wrong-current")
	if body := env.change(again, newTestPIN, thirdTestPIN, thirdTestPIN).Body.String(); body != `{"ok":true}` {
		t.Errorf("a second change with the new PIN as current answered %s", body)
	}
	if env.session(thirdTestPIN) == nil {
		t.Error("the second new PIN does not open the app")
	}
}

func TestEverySessionButTheCallersEndsWhenThePINChanges(t *testing.T) {
	env := newPinEnv(t)
	first, caller, third := env.unlock(), env.unlock(), env.unlock()
	if body := env.change(caller, testPIN, newTestPIN, newTestPIN).Body.String(); body != `{"ok":true}` {
		t.Fatalf("answered %s", body)
	}
	if env.isLocked(caller) {
		t.Error("the caller's session ended")
	}
	for label, other := range map[string]*http.Cookie{"first": first, "third": third} {
		expectError(t, label+" session, any route", env.call("/api/shell/overview", `[]`, other), http.StatusUnauthorized, "locked")
		expectError(t, label+" session, lock", env.call("/api/session/lock", `[]`, other), http.StatusUnauthorized, "locked")
	}
}

func TestTheFaultsAreReportedInOrderAndChangeNothing(t *testing.T) {
	env := newPinEnv(t)
	cookie, other := env.unlock(), env.unlock()
	for _, next := range []string{"12345", "1234567", "12345a", ""} {
		env.expectFault("new "+next, env.change(cookie, testPIN, next, next), "invalid-new")
	}
	env.expectFault("repeat differs", env.change(cookie, testPIN, newTestPIN, "135791"), "mismatch")
	env.expectFault("wrong current", env.change(cookie, wrongTestPIN, newTestPIN, newTestPIN), "wrong-current")
	env.expectFault("wrong current before the other faults", env.change(cookie, wrongTestPIN, "12", "34"), "wrong-current")
	if env.storedRows() != 0 {
		t.Error("a refused change stored a PIN")
	}
	if env.isLocked(cookie) || env.isLocked(other) {
		t.Error("a refused change ended a session")
	}
	if !env.stillUnlocksWith(testPIN) || env.stillUnlocksWith(newTestPIN) {
		t.Error("the PIN that opens the app changed")
	}
	env.expectJournalUntouched()
}

func TestTheThirdWrongCurrentPINAnswersLockedWithOneClearingCookie(t *testing.T) {
	env := newPinEnv(t)
	cookie, other := env.unlock(), env.unlock()
	for range 2 {
		env.expectFault("wrong current", env.change(cookie, wrongTestPIN, newTestPIN, newTestPIN), "wrong-current")
		if env.isLocked(cookie) {
			t.Fatal("a wrong current PIN ended the session before the third")
		}
	}
	third := env.change(cookie, wrongTestPIN, newTestPIN, newTestPIN)
	expectError(t, "third wrong current PIN", third, http.StatusUnauthorized, "locked")
	if third.Body.String() != `{"error":{"kind":"locked"}}` {
		t.Errorf("body %s", third.Body.String())
	}
	expectOneClearingCookie(t, "third wrong current PIN", third)
	expectError(t, "the caller afterwards", env.call("/api/shell/overview", `[]`, cookie), http.StatusUnauthorized, "locked")
	if env.isLocked(other) {
		t.Error("the other session ended with the caller's")
	}
	if status := decodeBody(t, env.call("/api/session/status", `[]`, other)); status["retryAfterSeconds"] != float64(300) {
		t.Errorf("status %v, want retryAfterSeconds 300", status)
	}
	env.expectJournalUntouched()
}

func TestAChangeDuringTheHoldComparesNothingAndEndsTheCallersSession(t *testing.T) {
	env := newPinEnv(t)
	caller, bystander := env.unlock(), env.unlock()
	for range 2 {
		env.call("/api/session/unlock", `["`+wrongTestPIN+`"]`, nil)
	}
	env.call("/api/session/unlock", `["`+wrongTestPIN+`"]`, nil)
	tries := env.pinTries.Load()
	response := env.change(caller, testPIN, newTestPIN, newTestPIN)
	expectError(t, "right PIN during the hold", response, http.StatusUnauthorized, "locked")
	expectOneClearingCookie(t, "during the hold", response)
	if env.pinTries.Load() != tries {
		t.Error("the PIN was compared during the hold")
	}
	if !env.isLocked(caller) || env.isLocked(bystander) {
		t.Error("the caller's session must end and another one stay")
	}
	env.clock.Advance(300 * time.Second)
	again := env.session(testPIN)
	if body := env.change(again, testPIN, newTestPIN, newTestPIN).Body.String(); body != `{"ok":true}` {
		t.Errorf("after the hold the change answered %s", body)
	}
}

func TestAWrongCurrentPINCountsInTheLockoutThatTheKeypadShares(t *testing.T) {
	t.Run("two wrong unlocks then one wrong current PIN lock", func(t *testing.T) {
		env := newPinEnv(t)
		cookie := env.unlock()
		for range 2 {
			env.call("/api/session/unlock", `["`+wrongTestPIN+`"]`, nil)
		}
		expectError(t, "wrong current", env.change(cookie, wrongTestPIN, newTestPIN, newTestPIN), http.StatusUnauthorized, "locked")
	})
	t.Run("two wrong current PINs then one wrong unlock hold the keypad", func(t *testing.T) {
		env := newPinEnv(t)
		cookie := env.unlock()
		for range 2 {
			env.change(cookie, wrongTestPIN, newTestPIN, newTestPIN)
		}
		body := env.call("/api/session/unlock", `["`+wrongTestPIN+`"]`, nil).Body.String()
		if body != `{"ok":false,"reason":"locked-out","retryAfterSeconds":300}` {
			t.Errorf("answered %s", body)
		}
	})
	t.Run("a right current PIN clears the count even when the new one is refused", func(t *testing.T) {
		env := newPinEnv(t)
		cookie := env.unlock()
		env.change(cookie, wrongTestPIN, newTestPIN, newTestPIN)
		env.expectFault("invalid new", env.change(cookie, testPIN, "12", "12"), "invalid-new")
		for range 2 {
			env.expectFault("wrong unlock", env.call("/api/session/unlock", `["`+wrongTestPIN+`"]`, nil), "wrong-pin")
		}
		if !env.stillUnlocksWith(testPIN) {
			t.Error("the keypad was held after two wrong PINs")
		}
	})
}

func TestArgumentsThatAreNotThreeStringsAreRefusedBeforeAnyPINIsComparedOrCounted(t *testing.T) {
	env := newPinEnv(t)
	cookie := env.unlock()
	for range 2 {
		env.change(cookie, wrongTestPIN, newTestPIN, newTestPIN)
	}
	tries := env.pinTries.Load()
	for _, body := range []string{`["` + testPIN + `",135790,"135790"]`, `["` + testPIN + `","135790"]`} {
		expectError(t, body, env.call(changePinRoute, body, cookie), http.StatusBadRequest, "bad_request")
	}
	if env.pinTries.Load() != tries {
		t.Error("a PIN was compared for arguments the schema refuses")
	}
	expectError(t, "the third wrong one still locks", env.change(cookie, wrongTestPIN, newTestPIN, newTestPIN), http.StatusUnauthorized, "locked")
}

func TestAFailedStoreAnswersInternalAndLeavesTheOldPINAndEverySession(t *testing.T) {
	env := newPinEnv(t)
	cookie, other := env.unlock(), env.unlock()
	env.store.failNext()
	expectError(t, "failed store", env.change(cookie, testPIN, newTestPIN, newTestPIN), http.StatusInternalServerError, "internal")
	if env.storedRows() != 0 {
		t.Error("a PIN was stored")
	}
	if !env.stillUnlocksWith(testPIN) || env.stillUnlocksWith(newTestPIN) {
		t.Error("the PIN that opens the app is not the old one")
	}
	if env.isLocked(cookie) || env.isLocked(other) {
		t.Error("a session ended")
	}
}

func TestChangingThePINWorksWhileTheSnapshotIsMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.db")
	env := newPinEnv(t, func(config *Config) {
		config.Reads = NewReadRegistry()
		config.Ledger = ledger.New(missing)
	})
	cookie := env.unlock()
	expectError(t, "a read", env.call("/api/shell/overview", `[]`, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	if body := env.change(cookie, testPIN, newTestPIN, newTestPIN).Body.String(); body != `{"ok":true}` {
		t.Errorf("answered %s", body)
	}
}

func TestWithoutADatabaseChangingThePINIsReadOnlyBeforeItsBodyIsRead(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	tries := env.pinTries.Load()
	if reads, status := readsOfBody(env, changePinRoute, cookie); reads != 0 || status != http.StatusForbidden {
		t.Errorf("the body was read %d times and the status was %d, want 0 and 403", reads, status)
	}
	expectError(t, "with a body", env.change(cookie, testPIN, newTestPIN, newTestPIN), http.StatusForbidden, "read_only")
	if env.pinTries.Load() != tries {
		t.Error("a PIN was compared")
	}
}

func TestWithoutASessionChangingThePINIsLocked(t *testing.T) {
	env := newPinEnv(t)
	tries := env.pinTries.Load()
	expectError(t, "no cookie", env.change(nil, testPIN, newTestPIN, newTestPIN), http.StatusUnauthorized, "locked")
	expectError(t, "forged cookie", env.change(&http.Cookie{Name: hostCookieName, Value: "forged"}, testPIN, newTestPIN, newTestPIN), http.StatusUnauthorized, "locked")
	if env.pinTries.Load() != tries || env.storedRows() != 0 {
		t.Error("a request without a session compared or stored a PIN")
	}
}

func TestTheRouterHandsChangePinToTheSessionService(t *testing.T) {
	env := newPinEnv(t, func(config *Config) { config.Reads = NewReadRegistry() })
	cookie := env.unlock()
	for _, name := range sessionServiceWrites {
		domain, method := splitMethod(t, name)
		response := env.call("/api/"+domain+"/"+method, `["`+testPIN+`","`+newTestPIN+`","`+newTestPIN+`"]`, cookie)
		if response.Body.String() != `{"ok":true}` {
			t.Errorf("%s answered %d %s: the router did not hand it to the session service", name, response.Code, response.Body.String())
		}
	}
}

func TestAChangeAndTwoUnlocksAtOnceEachSeeTheOldHashOrTheNewOne(t *testing.T) {
	env := newPinEnv(t)
	cookie := env.unlock()
	var wait sync.WaitGroup
	answers := make([]string, 3)
	run := func(index int, work func() string) {
		wait.Add(1)
		go func() {
			defer wait.Done()
			answers[index] = work()
		}()
	}
	run(0, func() string { return env.change(cookie, testPIN, newTestPIN, newTestPIN).Body.String() })
	run(1, func() string { return env.call("/api/session/unlock", `["`+testPIN+`"]`, nil).Body.String() })
	run(2, func() string { return env.call("/api/session/unlock", `["`+newTestPIN+`"]`, nil).Body.String() })
	wait.Wait()
	if answers[0] != `{"ok":true}` {
		t.Errorf("the change answered %s", answers[0])
	}
	for _, answer := range answers[1:] {
		if answer != `{"ok":true}` && answer != `{"ok":false,"reason":"wrong-pin"}` {
			t.Errorf("an unlock answered %s", answer)
		}
	}
	if !env.stillUnlocksWith(newTestPIN) || env.stillUnlocksWith(testPIN) {
		t.Error("once the change answered, the server did not verify against the stored hash alone")
	}
}

func TestNoPINAppearsInTheLogTheAnswersOrTheJournal(t *testing.T) {
	const (
		right    = "718293"
		wrong    = "406517"
		fresh    = "582940"
		differs  = "582941"
		invalid  = "58294x"
		lastGood = "930417"
		guess    = "264801"
	)
	env := newPinEnv(t, func(config *Config) {
		config.VerifyPIN = func(attempt string) bool { return attempt == right }
	})
	var seen []string
	note := func(response *httptest.ResponseRecorder) {
		seen = append(seen, response.Body.String())
		for name, values := range response.Header() {
			if name != "Set-Cookie" {
				seen = append(seen, values...)
			}
		}
	}
	cookie := env.session(right)
	note(env.change(cookie, wrong, fresh, fresh))
	note(env.change(cookie, right, fresh, differs))
	note(env.change(cookie, right, invalid, invalid))
	env.store.failNext()
	note(env.change(cookie, right, fresh, fresh))
	env.store.mutex.Lock()
	env.store.failed = false
	env.store.mutex.Unlock()
	note(env.change(cookie, right, lastGood, lastGood))
	for range 3 {
		note(env.change(cookie, guess, fresh, fresh))
	}
	note(env.call(changePinRoute, `["`+right+`",135790,"`+fresh+`"]`, cookie))
	text := strings.Join(append(seen, env.logs.String()), "\n")
	for _, secret := range []string{right, wrong, fresh, differs, invalid, lastGood, guess} {
		if strings.Contains(text, secret) {
			t.Errorf("a PIN typed in this test is in a log line or an answer")
		}
	}
	env.expectJournalUntouched()
}
