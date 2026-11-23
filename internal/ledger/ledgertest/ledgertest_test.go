package ledgertest_test

import (
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func listedIDs(t *testing.T, f *ledgertest.Fixture) []string {
	t.Helper()
	view := ledger.New(f.Path).View()
	defer view.Close()
	rows, err := view.CleanTransactions(t.Context(), brday.Period{First: "1970-01-01", Last: "2999-12-31"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	slices.Sort(ids)
	return ids
}

func coveredFixture(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: "Conta", Currency: "BRL"})
	f.AddAccount(ledgertest.Account{ID: "acc-2", ItemID: "item-1", Type: "BANK", Name: "Outra", Currency: "BRL", CoverageStart: "2026-03-01"})
	add := func(id, account string, at time.Time) {
		f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: account, Date: at, Description: "PADARIA",
			Amount: -10, Currency: "BRL", Type: "DEBIT"})
	}
	add("tx-a", "acc-1", time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC))
	add("tx-b", "acc-2", time.Date(2026, 3, 11, 15, 0, 0, 0, time.UTC))
	add("tx-early", "acc-2", time.Date(2026, 2, 11, 15, 0, 0, 0, time.UTC))
	return f
}

func TestALaterRunCoversTheDocumentsOnlyOnceItHasScopesOfItsOwn(t *testing.T) {
	f := coveredFixture(t)
	want := []string{"tx-a", "tx-b"}
	if got := listedIDs(t, f); !slices.Equal(got, want) {
		t.Fatalf("under run 1 the list is %v, want %v", got, want)
	}

	run := f.AddRun(ledgertest.Run{Outcome: "partial", FinishedAt: time.Date(2026, 3, 16, 3, 0, 0, 0, time.UTC)})
	f.ProjectTo(run)
	if got := listedIDs(t, f); len(got) != 0 {
		t.Fatalf("projected to a run without scopes, the list is %v, want it empty", got)
	}

	f.CoverInRun(run)
	if got := listedIDs(t, f); !slices.Equal(got, want) {
		t.Errorf("covered in run %d, the list is %v, want %v: a lower bound was lost or a scope is missing", run, got, want)
	}
	f.CoverInRun(run)
	if got := listedIDs(t, f); !slices.Equal(got, want) {
		t.Errorf("covering the same run twice changed the list to %v", got)
	}
}
