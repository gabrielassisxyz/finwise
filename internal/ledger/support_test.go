package ledger_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func instant(t testing.TB, text string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func fileHash(t testing.TB, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func february2026(t testing.TB) brday.Period {
	t.Helper()
	period, err := brday.MonthPeriod("2026-02")
	if err != nil {
		t.Fatal(err)
	}
	return period
}

// everyQuery runs each public query once and returns the error of each.
func everyQuery(reader *ledger.Reader) map[string]error {
	ctx := context.Background()
	period := brday.Period{First: "2026-02-01", Last: "2026-02-28"}
	failure := func(_ any, err error) error { return err }
	return map[string]error{
		"Accounts":          failure(reader.Accounts(ctx)),
		"CleanTransactions": failure(reader.CleanTransactions(ctx, period)),
		"Bills":             failure(reader.Bills(ctx)),
		"Pots":              failure(reader.Pots(ctx, "2026-03-15")),
		"Investments":       failure(reader.Investments(ctx)),
		"InvestmentHistory": failure(reader.InvestmentHistory(ctx, "inv-1")),
		"Identities":        failure(reader.Identities(ctx)),
		"SyncState":         failure(reader.SyncState(ctx)),
	}
}

// populated is a small ledger with at least one row behind every query.
func populated(t testing.TB) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	f.AddItem(ledgertest.Item{
		ID: "item-1", Connector: "Test Bank", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: instant(t, "2026-03-15T02:00:00Z"), NextAutoSync: instant(t, "2026-03-16T02:00:00Z"),
	})
	f.AddAccount(ledgertest.Account{
		ID: "acc-1", ItemID: "item-1", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Test Checking", Currency: "BRL", Balance: 100.25,
		Reserves: []ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 10.10, CDIRatio: 1.0}},
	})
	f.AddTransaction(ledgertest.Transaction{
		ID: "tx-1", AccountID: "acc-1", Date: instant(t, "2026-02-10T12:00:00Z"),
		Description: "Test purchase", Amount: -12.34, Currency: "BRL", Type: "DEBIT",
	})
	f.AddBill(ledgertest.Bill{ID: "bill-1", AccountID: "acc-1", DueDate: "2026-03-10", ClosingDate: "2026-03-03", Total: 50.5})
	f.AddInvestment(ledgertest.Investment{ID: "inv-1", ItemID: "item-1", Type: "FIXED_INCOME", Subtype: "CDB", Name: "Test CDB", Currency: "BRL", Balance: 200})
	f.AddIdentity(ledgertest.Identity{ID: "idn-1", ItemID: "item-1", Document: "00000000000", FullName: "Test Person"})
	return f
}

func asVersionError(err error, target **ledger.UnsupportedVersionError) bool {
	return errors.As(err, target)
}
