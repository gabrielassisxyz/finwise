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
// budget, the subscriptions and the two reviews, and requires that exactly one
// bank account has ever received a payroll credit: the payday names one account
// and a second would make that choice a guess.
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

	chosen, receiving := findSalaryAccount(accounts, rows)
	t.Logf("fresh_install_calls=%d answers_validated=%d answers_not_conforming=%d", asked, run.answers, run.nonConforming)
	t.Logf("bank_accounts_with_a_payroll_credit=%d salary_account_found=%t", receiving, chosen != nil)

	if run.answers != asked || run.nonConforming != 0 {
		t.Error("an answer is missing or does not conform")
	}
	if receiving != 1 {
		t.Errorf("%d bank accounts have received a payroll credit, want exactly 1", receiving)
	}
}
