package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func decodeAnswer(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("answer is not an object: %v", err)
	}
	return decoded
}

func overviewOf(t *testing.T, env *readEnv) map[string]any {
	t.Helper()
	status, body := env.answer("shell.overview", `[]`)
	if status != http.StatusOK {
		t.Fatalf("shell.overview answered %d", status)
	}
	return decodeAnswer(t, body)
}

func TestShellOverviewOverTheFullLedger(t *testing.T) {
	overview := overviewOf(t, newReadEnv(t, newFullLedger(t)))
	for field, want := range map[string]any{
		"today": "2026-03-20", "firstMonth": "2026-01", "lastMonth": "2026-03", "pendingCount": float64(0),
	} {
		if overview[field] != want {
			t.Errorf("%s = %v, want %v", field, overview[field], want)
		}
	}
	if closed, ok := overview["closedMonths"].([]any); !ok || len(closed) != 0 {
		t.Errorf("closedMonths = %v, want []", overview["closedMonths"])
	}
}

func TestAConnectionInTroubleWarnsInPortuguese(t *testing.T) {
	sync := overviewOf(t, newReadEnv(t, newFullLedger(t)))["sync"].(map[string]any)
	if sync["state"] != "warning" || sync["message"] != "C6: problema na conexão" {
		t.Errorf("sync = %v", sync)
	}
}

func TestShellOverviewOverTheEmptyLedgerStartsAndEndsInTheCurrentMonth(t *testing.T) {
	overview := overviewOf(t, newReadEnv(t, newEmptyLedger(t)))
	if overview["firstMonth"] != "2026-03" || overview["lastMonth"] != "2026-03" {
		t.Errorf("first %v last %v, want both 2026-03", overview["firstMonth"], overview["lastMonth"])
	}
}

func TestHealthyConnectionsGiveTheDayAndTimeOfTheLastCompleteRun(t *testing.T) {
	f := newEmptyLedger(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-1", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddRun(ledgertest.Run{Outcome: "complete", FinishedAt: utc(2026, time.March, 20, 5, 30)})
	f.AddRun(ledgertest.Run{Outcome: "partial", FinishedAt: utc(2026, time.March, 20, 6, 0)})

	sync := overviewOf(t, newReadEnv(t, f))["sync"].(map[string]any)
	if sync["state"] != "ok" || sync["lastSyncDay"] != "2026-03-20" || sync["lastSyncTime"] != "02:30" {
		t.Errorf("sync = %v, want ok on 2026-03-20 at 02:30 Brazil time", sync)
	}
}

func TestTodayIsTheDayInBrazilAtTheInjectedInstant(t *testing.T) {
	f := newFullLedger(t)
	for _, c := range []struct {
		at               time.Time
		today, lastMonth string
	}{
		{utc(2026, time.March, 1, 2, 30), "2026-02-28", "2026-02"},
		{utc(2026, time.March, 1, 3, 0), "2026-03-01", "2026-03"},
	} {
		env := newTestEnv(t, withLedger(f.Path, c.at))
		overview := overviewOf(t, &readEnv{testEnv: env, cookie: env.unlock()})
		if overview["today"] != c.today || overview["lastMonth"] != c.lastMonth {
			t.Errorf("at %s: today %v last month %v, want %s and %s", c.at.Format(time.RFC3339),
				overview["today"], overview["lastMonth"], c.today, c.lastMonth)
		}
	}
}

func TestTheFirstMonthNeverFollowsTheCurrentOne(t *testing.T) {
	rows := []ledger.Transaction{{Date: utc(2026, time.June, 1, 12, 0)}}
	if got := firstMonthOf(rows, "2026-03"); got != "2026-03" {
		t.Errorf("first month %s, want the current month when every row is later", got)
	}
}

func TestSyncStatusNamesTheFirstProblemAndCountsTheRest(t *testing.T) {
	now := utc(2026, time.March, 20, 15, 0)
	fresh := now.Add(-time.Hour)
	stale := now.Add(-49 * time.Hour)
	expired, soon, later := now.Add(-time.Minute), now.Add(6*24*time.Hour), now.Add(30*24*time.Hour)
	banks := map[string]string{"a": "Itaú", "b": "C6", "c": "Nubank"}
	healthy := ledger.Connection{ItemID: "a", Status: "UPDATED", ExecutionStatus: "SUCCESS", LastUpdated: &fresh}
	with := func(change func(*ledger.Connection)) ledger.Connection {
		connection := healthy
		change(&connection)
		return connection
	}
	cases := []struct {
		name       string
		connection ledger.Connection
		want       string
	}{
		{"status", with(func(c *ledger.Connection) { c.Status = "OUTDATED" }), "Itaú: problema na conexão"},
		{"execution status", with(func(c *ledger.Connection) { c.ExecutionStatus = "ERROR" }), "Itaú: problema na conexão"},
		{"stale", with(func(c *ledger.Connection) { c.LastUpdated = &stale }), "Itaú: sem atualização há mais de 48 horas"},
		{"never updated", with(func(c *ledger.Connection) { c.LastUpdated = nil }), "Itaú: sem atualização há mais de 48 horas"},
		{"consent expired", with(func(c *ledger.Connection) { c.ConsentExpiresAt = &expired }), "Itaú: consentimento expirado"},
		{"consent expiring", with(func(c *ledger.Connection) { c.ConsentExpiresAt = &soon }), "Itaú: consentimento perto de expirar"},
	}
	for _, c := range cases {
		status := syncStatusOf(ledger.SyncState{LastCompleteSync: &fresh, Connections: []ledger.Connection{c.connection}}, now, banks)
		if got, ok := status.(syncWarning); !ok || got.Message != c.want {
			t.Errorf("%s: %+v, want a warning %q", c.name, status, c.want)
		}
	}

	farConsent := with(func(c *ledger.Connection) { c.ConsentExpiresAt = &later })
	if _, ok := syncStatusOf(ledger.SyncState{LastCompleteSync: &fresh, Connections: []ledger.Connection{farConsent}}, now, banks).(syncOK); !ok {
		t.Error("a consent a month away raised a warning")
	}

	two := []ledger.Connection{
		with(func(c *ledger.Connection) { c.Status = "OUTDATED" }),
		with(func(c *ledger.Connection) { c.ItemID = "b"; c.ExecutionStatus = "ERROR" }),
	}
	status := syncStatusOf(ledger.SyncState{LastCompleteSync: &fresh, Connections: two}, now, banks)
	if got := status.(syncWarning).Message; got != "Itaú: problema na conexão (e mais 1)" {
		t.Errorf("two problems gave %q", got)
	}
}

func TestWithoutACompleteRunTheStatusAwaitsTheFirstSync(t *testing.T) {
	status := syncStatusOf(ledger.SyncState{}, utc(2026, time.March, 20, 15, 0), nil)
	if got, ok := status.(syncWarning); !ok || got.Message != "Aguardando a primeira sincronização" {
		t.Errorf("status = %+v", status)
	}
}

func TestAConnectionOfNoKnownBankIsNotAttributedToOne(t *testing.T) {
	now := utc(2026, time.March, 20, 15, 0)
	broken := ledger.Connection{ItemID: "unseen", Status: "OUTDATED", ExecutionStatus: "SUCCESS", LastUpdated: &now}
	got := syncStatusOf(ledger.SyncState{Connections: []ledger.Connection{broken}}, now, map[string]string{})
	if message := got.(syncWarning).Message; message != "Banco não identificado: problema na conexão" {
		t.Errorf("message %q", message)
	}
}

// Two months closed on two days, the later month first, as two requests of
// their own days close them.
func closedTwiceInTheOtherOrder(t *testing.T) *readEnv {
	t.Helper()
	f := newFullLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	for _, closing := range []struct {
		month string
		at    time.Time
	}{{"2026-02", utc(2026, time.March, 2, 15, 0)}, {"2025-12", fixtureNow}} {
		env := newTestEnv(t, withLedger(f.Path, closing.at), withDatabase(scratch.Database))
		changeIDOfAnswer(t, env.call("/api/review/closeMonth", `["`+closing.month+`"]`, env.unlock()))
	}
	return newReadEnvOver(t, f, scratch.Database)
}

func TestTheShellListsTheClosedMonthsOldestFirstAndTheClosingNewestFirst(t *testing.T) {
	env := closedTwiceInTheOtherOrder(t)
	status, body := env.answer("shell.overview", `[]`)
	var overview struct {
		ClosedMonths json.RawMessage `json:"closedMonths"`
	}
	if err := json.Unmarshal(body, &overview); status != http.StatusOK || err != nil {
		t.Fatalf("shell.overview answered %d %s", status, body)
	}
	oldestFirst := `[{"month":"2025-12","closedOn":"2026-03-20"},{"month":"2026-02","closedOn":"2026-03-02"}]`
	if string(overview.ClosedMonths) != oldestFirst {
		t.Errorf("the shell lists %s, want %s", overview.ClosedMonths, oldestFirst)
	}
	status, body = env.answer("review.closing", `["2026-03"]`)
	var closing struct {
		ClosedMonths json.RawMessage `json:"closedMonths"`
	}
	if err := json.Unmarshal(body, &closing); status != http.StatusOK || err != nil {
		t.Fatalf("review.closing answered %d %s", status, body)
	}
	if newestFirst := `[{"month":"2026-02","closedOn":"2026-03-02"},{"month":"2025-12","closedOn":"2026-03-20"}]`; string(closing.ClosedMonths) != newestFirst {
		t.Errorf("the closing lists %s, want %s", closing.ClosedMonths, newestFirst)
	}
}

// aSecondClosedMonth closes 2025-12 beside the third state's January, as the
// app closes a month.
func aSecondClosedMonth(t *testing.T, scratch *authoredtest.Scratch, _ *ledgertest.Fixture) {
	t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: closeMonthMethod, At: utc(2026, time.January, 6, 15, 0)}
	err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		return recorder.CloseMonth(t.Context(), "2025-12", "2026-01-06")
	})
	if err != nil {
		t.Fatal(err)
	}
}
