package ingest_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// A real ledger is a person's whole financial history. The run below opens it
// read-only, takes it into a scratch database in the test's temporary
// directory, which holds real values as soon as the run writes and is removed
// with the directory, compares everything in memory, fails with fixed sentences
// and logs counts only.

func TestTheIngestOverARealSnapshot(t *testing.T) {
	path := os.Getenv("FINWISE_LEDGER_SNAPSHOT")
	if path == "" {
		t.Skip("FINWISE_LEDGER_SNAPSHOT is not set")
	}
	checkALedgerTakenIn(t, path)
}

// The same check over an invented ledger, so its logic is proved by every run
// of the suite and not only by the one over a real snapshot.
func TestTheRealSnapshotCheckHoldsOverAnInventedLedger(t *testing.T) {
	f := ledgertest.NewAt(t, filepath.Join(t.TempDir(), "pluggy-snapshot.db"))
	addAccounts(f)
	for _, row := range outcomeRows() {
		f.AddTransaction(row)
	}
	f.AddTransaction(purchase("tx-february", at(2026, time.February, 20), "PADARIA EXEMPLO", 9))
	f.AddTransaction(pendingPurchase("tx-pending", at(2026, time.March, 19), "LOJA EXEMPLO", 33))
	checkALedgerTakenIn(t, f.Path)
}

func failedWithoutValues(t *testing.T, step string, err error) {
	t.Helper()
	if kind, classified := ingest.FailureOf(err); classified {
		t.Fatalf("%s failed with the kind %s", step, kind)
	}
	t.Fatalf("%s failed", step)
}

// listedOf is the interpretation a run takes in, read on a view of its own.
func listedOf(t *testing.T, path string) []txrules.Transaction {
	t.Helper()
	view := ledger.New(path).View()
	defer view.Close()
	listed, err := ingest.ListedTransactions(t.Context(), view)
	if err != nil {
		failedWithoutValues(t, "reading the ledger", err)
	}
	return listed
}

// noonOfLatest is noon in Brazil of the latest listed day.
func noonOfLatest(t *testing.T, listed []txrules.Transaction) time.Time {
	t.Helper()
	var latest brday.Day
	for _, transaction := range listed {
		latest = max(latest, transaction.Day)
	}
	day, err := time.Parse(time.DateOnly, string(latest))
	if err != nil {
		t.Fatal("the latest listed day does not parse")
	}
	return day.Add(15 * time.Hour)
}

func checkALedgerTakenIn(t *testing.T, path string) {
	listed := listedOf(t, path)
	if len(listed) == 0 {
		t.Fatal("the ledger lists no transaction")
	}
	clock := noonOfLatest(t, listed)
	start := brday.MonthOf(clock)
	databasePath := filepath.Join(t.TempDir(), "finwise.db")
	database, err := authored.Open(t.Context(), databasePath, authored.Options{Now: func() time.Time { return clock }})
	if err != nil {
		t.Fatal("the scratch database did not open")
	}
	t.Cleanup(func() { database.Close() })
	scratch := &authoredtest.Scratch{Database: database, Path: databasePath}
	run := func(step string, trigger ingest.Trigger) ingest.Counts {
		t.Helper()
		counts, err := ingest.Run(t.Context(), ingest.Sources{Ledger: ledger.New(path), Database: database}, trigger, clock)
		if err != nil {
			failedWithoutValues(t, step, err)
		}
		return counts
	}

	if counts := run("the run with no budget", ingest.TriggerCommand); counts != (ingest.Counts{}) {
		t.Error("a run with no budget counted something")
	}
	authoredtest.SaveBudget(t, database, clock, start, authoredtest.InventedBudget())
	counts := run("the run after the budget", ingest.TriggerBudgetChange)
	t.Logf("indexed=%d paired=%d unpaired=%d applied=%d pending_low_confidence=%d pending_unnamed_person=%d pending_new_establishment=%d left=%d nothing_to_store=%d already_decided=%d",
		counts.Indexed, counts.Paired, counts.Unpaired, counts.Applied, counts.PendingLowConfidence, counts.PendingUnnamedPerson,
		counts.PendingNewEstablishment, counts.Left, counts.NothingToStore, counts.AlreadyDecided)

	fromStart := 0
	for _, transaction := range listed {
		if brday.Month(string(transaction.Day)[:7]) >= start {
			fromStart++
		}
	}
	if counts.Indexed != fromStart {
		t.Error("indexed is not the number of listed transactions from the start month on")
	}
	if counts.Applied != 0 || counts.PendingLowConfidence != 0 {
		t.Error("a line was applied or suggested where nobody has put anything on a line")
	}
	if counts.Outcomes() != counts.Indexed {
		t.Error("the evaluated transactions do not add up across the outcomes")
	}
	view := database.View()
	rows, err := view.IndexedTransactions(t.Context())
	view.Close()
	if err != nil {
		t.Fatal("the index could not be read")
	}
	for _, row := range rows {
		if brday.Month(string(row.Day)[:7]) < start {
			t.Error("an index row is dated before the start month")
			break
		}
	}

	if !run("the tick", ingest.TriggerTick).Stopped {
		t.Error("a tick over the run taken in did not stop")
	}
	changes, canonical := len(scratch.Changes(t)), scratch.Canonical(t)
	run("the command over the run taken in", ingest.TriggerCommand)
	if len(scratch.Changes(t)) != changes {
		t.Error("the command over the run taken in wrote a change")
	}
	if scratch.Canonical(t) != canonical {
		t.Error("the command over the run taken in changed what was authored")
	}
}
