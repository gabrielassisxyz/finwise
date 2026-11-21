package api

import (
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// freshInstallMethods are the reads that answer a fixed value or, for payday, a
// balance. Only counts are logged, as everywhere over a real snapshot.
var freshInstallMethods = []string{
	"budget.", "subscriptions.", "review.weekly", "review.payday",
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
// budget, the subscriptions and the two reviews. A salary can move to another
// account, so several accounts having received payroll is normal; what must not
// happen is two accounts sharing the latest payroll instant, which would leave
// "the account the salary lands on" to a tie rule.
func TestTheFreshInstallReadsOverARealSnapshot(t *testing.T) {
	path := realSnapshotPath(t)
	reader, ctx := ledger.New(path), t.Context()
	accounts, err := reader.Accounts(ctx)
	withoutValues(t, "accounts", err)
	rows, err := reader.CleanTransactions(ctx, wholeLedger)
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
}
