package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// freshInstallMethods are the reads that answer a fixed value or, for payday, a
// balance, and the two that add to it what the bank's records support. Only
// counts are logged, as everywhere over a real snapshot.
var freshInstallMethods = []string{
	"budget.", "subscriptions.", "review.weekly", "review.payday", "review.closing", "settings.",
}

func isFreshInstallRead(method string) bool {
	for _, prefix := range freshInstallMethods {
		if strings.HasPrefix(method, prefix) {
			return true
		}
	}
	return false
}

// TestTheFreshInstallReadsOverARealSnapshot validates every answer of the
// budget, the subscriptions, the three reviews and the settings. A salary can move to another
// account, so several accounts having received payroll is normal; what must not
// happen is two accounts sharing the latest payroll instant, which would leave
// "the account the salary lands on" to a tie rule.
func TestTheFreshInstallReadsOverARealSnapshot(t *testing.T) {
	path := realSnapshotPath(t)
	snapshot, ctx := viewOf(t, path), t.Context()
	accounts, err := snapshot.Accounts(ctx)
	withoutValues(t, "accounts", err)
	rows, err := snapshot.CleanTransactions(ctx, wholeLedger)
	withoutValues(t, "transactions", err)

	env := newTestEnv(t, withLedger(path, time.Now()))
	run := &realRun{t: t, env: &readEnv{testEnv: env, cookie: env.unlock()}}
	asked := 0
	for _, c := range readCases {
		if isFreshInstallRead(c.method) {
			run.answer(c.method, c.arguments)
			asked++
		}
	}

	found := findSalaryAccount(accounts, rows)
	t.Logf("fresh_install_calls=%d answers_validated=%d answers_not_conforming=%d", asked, run.answers, run.nonConforming)
	t.Logf("bank_accounts_that_ever_received_payroll=%d salary_account_found=%t latest_credit_untied=%t",
		found.Receiving, found.Account != nil, !found.Tied)

	if run.answers != asked || run.nonConforming != 0 {
		t.Error("an answer is missing or does not conform")
	}
	if found.Tied {
		t.Error("two accounts share the latest payroll credit")
	}
	logBankBackedCounts(t, run)
}

// logBankBackedCounts says how much of the settings and of the closing came
// from the bank's records: counts and one yes or no, never a value.
func logBankBackedCounts(t *testing.T, run *realRun) {
	t.Helper()
	var settings struct {
		Pots []json.RawMessage `json:"pots"`
		Data struct {
			Connections []json.RawMessage `json:"connections"`
			LastSync    *json.RawMessage  `json:"lastSync"`
		} `json:"data"`
		CardTitle *string `json:"cardTitle"`
	}
	var closing struct {
		CardBill *json.RawMessage `json:"cardBill"`
	}
	withoutValues(t, "settings answer", json.Unmarshal(run.answer("settings.overview", `[]`), &settings))
	withoutValues(t, "closing answer", json.Unmarshal(run.answer("review.closing", `["2026-03"]`), &closing))
	t.Logf("settings_pots=%d settings_connections=%d settings_has_last_sync=%t settings_has_card=%t closing_shows_card_bill=%t",
		len(settings.Pots), len(settings.Data.Connections), settings.Data.LastSync != nil, settings.CardTitle != nil, closing.CardBill != nil)
}
