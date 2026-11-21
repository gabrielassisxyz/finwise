package ledger_test

import (
	"os"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// The whole query set over a real snapshot, opt-in. Only counts are logged: the
// snapshot is a person's financial history and no value from it may reach a log.
func TestEveryQueryRunsOverARealSnapshot(t *testing.T) {
	path := os.Getenv("FINWISE_LEDGER_SNAPSHOT")
	if path == "" {
		t.Skip("FINWISE_LEDGER_SNAPSHOT is not set")
	}
	reader, ctx := ledger.New(path), t.Context()

	accounts, err := reader.Accounts(ctx)
	if err != nil {
		failWithoutValues(t, "accounts", err)
	}
	period := brday.Period{First: "1970-01-01", Last: "2999-12-31"}
	transactions, err := reader.CleanTransactions(ctx, period)
	if err != nil {
		failWithoutValues(t, "transactions", err)
	}
	bills, err := reader.Bills(ctx)
	if err != nil {
		failWithoutValues(t, "bills", err)
	}
	pots, err := reader.Pots(ctx, "2999-12-31")
	if err != nil {
		failWithoutValues(t, "pots", err)
	}
	investments, err := reader.Investments(ctx)
	if err != nil {
		failWithoutValues(t, "investments", err)
	}
	historyDays := 0
	for _, investment := range investments {
		days, err := reader.InvestmentHistory(ctx, investment.ID)
		if err != nil {
			failWithoutValues(t, "investment history", err)
		}
		historyDays += len(days)
	}
	identities, err := reader.Identities(ctx)
	if err != nil {
		failWithoutValues(t, "identities", err)
	}
	state, err := reader.SyncState(ctx)
	if err != nil {
		failWithoutValues(t, "sync state", err)
	}

	t.Logf("accounts=%d transactions=%d bills=%d pots=%d investments=%d investment_days=%d identities=%d connections=%d last_complete_sync_present=%t",
		len(accounts), len(transactions), len(bills), len(pots), len(investments), historyDays,
		len(identities), len(state.Connections), state.LastCompleteSync != nil)
}

// An error message names the row it failed on, and a real row's id is a real
// value, so a failure reports where it happened and what kind it was, no more.
func failWithoutValues(t *testing.T, query string, err error) {
	t.Helper()
	t.Fatalf("%s failed (%T); the message is withheld because it may name a real id", query, err)
}
