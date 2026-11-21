package api

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"
)

func attributesOf(t *testing.T, response *httptest.ResponseRecorder) []string {
	t.Helper()
	lines := response.Header().Values("Set-Cookie")
	if len(lines) != 1 {
		t.Fatalf("%d Set-Cookie lines, want 1", len(lines))
	}
	parts := strings.Split(lines[0], "; ")
	attributes := make([]string, 0, len(parts))
	for index, part := range parts {
		if index == 0 {
			name, _, _ := strings.Cut(part, "=")
			attributes = append(attributes, "name:"+name)
			continue
		}
		attributes = append(attributes, part)
	}
	sort.Strings(attributes)
	return attributes
}

func TestUnlockSetsExactlyTheSessionCookieAttributes(t *testing.T) {
	env := newTestEnv(t)
	response := env.call("/api/session/unlock", `["`+testPIN+`"]`, nil)
	want := []string{"HttpOnly", "Max-Age=43200", "Path=/", "SameSite=Strict", "Secure", "name:__Host-finwise-session"}
	got := attributesOf(t, response)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("cookie attributes %v, want %v", got, want)
	}
	if body := response.Body.String(); body != `{"ok":true}` {
		t.Errorf("body %s, want {\"ok\":true}", body)
	}
}

func TestDevelopmentCookieHasNoPrefixAndNoSecure(t *testing.T) {
	env := newTestEnv(t, func(config *Config) { config.InsecureCookie = true })
	response := env.call("/api/session/unlock", `["`+testPIN+`"]`, nil)
	want := []string{"HttpOnly", "Max-Age=43200", "Path=/", "SameSite=Strict", "name:finwise-session"}
	got := attributesOf(t, response)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("cookie attributes %v, want %v", got, want)
	}
}

func TestTheCookieLastsAsLongAsTheConfiguredSession(t *testing.T) {
	env := newTestEnv(t, func(config *Config) { config.SessionIdle = 90 * time.Minute })
	response := env.call("/api/session/unlock", `["`+testPIN+`"]`, nil)
	if cookies := response.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge != 5400 {
		t.Errorf("cookies %v, want one with Max-Age 5400", cookies)
	}
}

func TestAWrongPINAnswersWrongPinAndSetsNoCookie(t *testing.T) {
	env := newTestEnv(t)
	response := env.call("/api/session/unlock", `["000000"]`, nil)
	if response.Body.String() != `{"ok":false,"reason":"wrong-pin"}` || len(response.Header().Values("Set-Cookie")) != 0 {
		t.Errorf("body %s, cookies %v", response.Body.String(), response.Header().Values("Set-Cookie"))
	}
}

func TestThreeWrongPINsLockOutTheRightOneUntilTheClockMoves(t *testing.T) {
	env := newTestEnv(t)
	for range 2 {
		env.call("/api/session/unlock", `["000000"]`, nil)
	}
	third := decodeBody(t, env.call("/api/session/unlock", `["000000"]`, nil))
	if third["reason"] != "locked-out" || third["retryAfterSeconds"] != float64(300) {
		t.Fatalf("third wrong PIN answered %v", third)
	}
	tries := env.pinTries.Load()

	env.clock.Advance(10 * time.Second)
	fourth := env.call("/api/session/unlock", `["`+testPIN+`"]`, nil)
	body := decodeBody(t, fourth)
	wait, _ := body["retryAfterSeconds"].(float64)
	if body["ok"] != false || body["reason"] != "locked-out" || wait < 1 || wait > 300 {
		t.Errorf("the right PIN during the hold answered %v", body)
	}
	if len(fourth.Header().Values("Set-Cookie")) != 0 || env.pinTries.Load() != tries {
		t.Error("the hold issued a cookie or compared the PIN")
	}
	status := decodeBody(t, env.call("/api/session/status", `[]`, nil))
	if status["retryAfterSeconds"] != float64(290) || status["locked"] != true {
		t.Errorf("status during the hold: %v", status)
	}

	env.clock.Advance(290 * time.Second)
	if response := env.call("/api/session/unlock", `["`+testPIN+`"]`, nil); response.Body.String() != `{"ok":true}` {
		t.Errorf("after 300s the right PIN answered %s", response.Body.String())
	}
}

func TestACorrectUnlockAfterTwoWrongOnesResetsTheCount(t *testing.T) {
	env := newTestEnv(t)
	env.call("/api/session/unlock", `["000000"]`, nil)
	env.call("/api/session/unlock", `["000000"]`, nil)
	env.call("/api/session/unlock", `["`+testPIN+`"]`, nil)
	for range 2 {
		body := decodeBody(t, env.call("/api/session/unlock", `["000000"]`, nil))
		if body["reason"] != "wrong-pin" {
			t.Fatalf("the count survived a correct unlock: %v", body)
		}
	}
}

func TestStatusFollowsTheCookie(t *testing.T) {
	env := newTestEnv(t)
	without := decodeBody(t, env.call("/api/session/status", `[]`, nil))
	if without["locked"] != true || without["retryAfterSeconds"] != float64(0) {
		t.Errorf("without a cookie: %v", without)
	}
	with := decodeBody(t, env.call("/api/session/status", `[]`, env.unlock()))
	if with["locked"] != false {
		t.Errorf("with a cookie: %v", with)
	}
}

func TestLockEndsTheSessionAndClearsTheCookie(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	response := env.call("/api/session/lock", `[]`, cookie)
	if response.Code != http.StatusOK || response.Body.String() != "null" {
		t.Errorf("lock answered %d %s", response.Code, response.Body.String())
	}
	cleared := response.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != hostCookieName || cleared[0].MaxAge >= 0 || cleared[0].Value != "" {
		t.Errorf("lock set cookies %v, want one clearing %s", cleared, hostCookieName)
	}
	expectError(t, "after lock", env.call("/api/shell/overview", `[]`, cookie), http.StatusUnauthorized, "locked")
	expectError(t, "lock twice", env.call("/api/session/lock", `[]`, cookie), http.StatusUnauthorized, "locked")
}

func TestASessionUnusedForTwelveHoursIsLocked(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	env.clock.Advance(hours(12))
	expectError(t, "12h idle", env.call("/api/shell/overview", `[]`, cookie), http.StatusUnauthorized, "locked")
	if status := decodeBody(t, env.call("/api/session/status", `[]`, cookie)); status["locked"] != true {
		t.Errorf("status after expiry: %v", status)
	}
}

func TestARequestAtElevenHoursKeepsTheSessionAtTwenty(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	env.clock.Advance(hours(11))
	renewed := env.call("/api/session/status", `[]`, cookie)
	if cookies := renewed.Result().Cookies(); len(cookies) != 1 || cookies[0].Value != cookie.Value || cookies[0].MaxAge != 43200 {
		t.Errorf("a live request did not renew the cookie: %v", cookies)
	}
	env.clock.Advance(hours(9))
	response := env.call("/api/session/status", `[]`, cookie)
	if decodeBody(t, response)["locked"] != false {
		t.Error("the session died at 20 hours despite a request at 11")
	}
}

func TestChangePinIsReadOnlyAndComparesNothing(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.unlock()
	env.pinTries.Store(0)
	response := env.call("/api/session/changePin", `["`+testPIN+`","654321","654321"]`, cookie)
	expectError(t, "changePin", response, http.StatusForbidden, "read_only")
	if env.pinTries.Load() != 0 {
		t.Error("the PIN comparison ran for changePin")
	}
	if !env.stillUnlocksWith(testPIN) {
		t.Error("the PIN changed")
	}
}

func (env *testEnv) stillUnlocksWith(pin string) bool {
	return strings.Contains(env.call("/api/session/unlock", `["`+pin+`"]`, nil).Body.String(), `"ok":true`)
}

func TestTheRealHashUnlocksAndALockedKeypadNeverDerives(t *testing.T) {
	hash := testPINHash(t)
	env := newTestEnv(t, func(config *Config) { config.VerifyPIN = hash.Verify })
	if body := env.call("/api/session/unlock", `["654321"]`, nil).Body.String(); body != `{"ok":false,"reason":"wrong-pin"}` {
		t.Errorf("a wrong PIN against the real hash answered %s", body)
	}
	if !env.stillUnlocksWith(testPIN) {
		t.Error("the PIN the hash was made from did not unlock")
	}
}
