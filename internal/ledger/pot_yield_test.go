package ledger_test

import (
	"database/sql"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

const (
	yieldFrom      brday.Day = "2026-03-10"
	yieldTo        brday.Day = "2026-03-15"
	transferMoment           = "2026-03-12T12:00:00Z"
)

// yieldLedger is an account sighted on two days, with the reserves each
// sighting held, so that the window between them can be read.
func yieldLedger(t *testing.T, before, after []ledgertest.Reserve) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	f.AddItem(ledgertest.Item{
		ID: "item-1", Connector: "Test Bank", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: instant(t, "2026-03-15T02:00:00Z"), NextAutoSync: instant(t, "2026-03-16T02:00:00Z"),
	})
	f.AddAccount(ledgertest.Account{
		ID: "acc-1", ItemID: "item-1", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Test Checking", Currency: "BRL", Balance: 10, Reserves: after,
	})
	override := ledgertest.ReservesOverride(before...)
	override["updatedAt"] = "2026-03-10T02:00:00.000Z"
	f.ObserveAccount("acc-1", string(yieldFrom), override)
	return f
}

// addReserveTransfer adds a transfer the way the bank words it. A deposit is
// negative on the checking account.
func addReserveTransfer(t *testing.T, f *ledgertest.Fixture, id, description string, amount float64) {
	t.Helper()
	f.AddTransaction(ledgertest.Transaction{
		ID: id, AccountID: "acc-1", Date: instant(t, transferMoment), Description: description,
		Amount: amount, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "TRANSFERENCIA_SALDO_RESERVADO"},
	})
}

// walkReachesBack records that the account's transaction walk went back past
// the start of the window, which the query requires before it trusts a transfer.
// Call it before the first transaction is added: the fixture reuses the scope it
// finds, and a scope cannot be changed once closed.
func walkReachesBack(t *testing.T, f *ledgertest.Fixture) {
	t.Helper()
	database, err := sql.Open("sqlite", f.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	_, err = database.Exec(`INSERT INTO scopes (run_id, kind, parent, expected, outcome, infer_absence,
	        status, rows, min_date, min_instant, closed_at, closed_day)
	        VALUES (1, 'transactions', 'acc-1', 1, 'complete', 1, 200, 0, '2026-01-01',
	                '2026-01-01T00:00:00.000Z', '2026-03-15T03:00:00.000Z', '2026-03-15')`)
	if err != nil {
		t.Fatal(err)
	}
}

func yieldOf(t *testing.T, f *ledgertest.Fixture, name string) ledger.PotYield {
	t.Helper()
	yields, err := ledger.New(f.Path).PotYields(t.Context(), "acc-1", yieldFrom, yieldTo)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range yields {
		if candidate.ReserveName != nil && *candidate.ReserveName == name {
			return candidate
		}
	}
	t.Fatalf("no yield row for %q among %d rows", name, len(yields))
	return ledger.PotYield{}
}

func wantCents(t *testing.T, label string, got *money.Cents, want money.Cents) {
	t.Helper()
	if got == nil || *got != want {
		t.Errorf("%s = %v, want %d", label, got, want)
	}
}

func TestDepositWithNoYieldReportsAZeroYieldNotTheDeposit(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 150, CDIRatio: 1}})
	walkReachesBack(t, f)
	addReserveTransfer(t, f, "tx-dep", "Dinheiro reservado Trip", -50)

	got := yieldOf(t, f, "Trip")
	if got.Reason != nil {
		t.Fatalf("yield is unknown (%s), want known", *got.Reason)
	}
	wantCents(t, "yield", got.Yield, 0)
	wantCents(t, "transfers", got.Transfers, -5000)
	wantCents(t, "previous", got.Previous, 10000)
	wantCents(t, "current", got.Current, 15000)
}

func TestYieldIsTheBalanceChangeLeftAfterTheDeposit(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 150.30, CDIRatio: 1}})
	walkReachesBack(t, f)
	addReserveTransfer(t, f, "tx-dep", "Dinheiro reservado Trip", -50)

	wantCents(t, "yield", yieldOf(t, f, "Trip").Yield, 30)
}

func TestAReserveOpenedInsideTheWindowStartsAtZero(t *testing.T) {
	f := yieldLedger(t, []ledgertest.Reserve{},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 0.45, CDIRatio: 1}})
	walkReachesBack(t, f)

	got := yieldOf(t, f, "Trip")
	if got.Previous != nil {
		t.Errorf("previous = %d, want none for a reserve that did not exist", *got.Previous)
	}
	wantCents(t, "yield", got.Yield, 45)
}

func TestAReserveClosedInsideTheWindowHasUnknownYield(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{})
	walkReachesBack(t, f)

	got := yieldOf(t, f, "Trip")
	if got.Yield != nil || got.Reason == nil || *got.Reason != "closed or missing at D2" {
		t.Fatalf("yield=%v reason=%v, want unknown with reason 'closed or missing at D2'", got.Yield, got.Reason)
	}
}

func TestAWalkThatDoesNotReachBackMakesEveryYieldUnknown(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 150, CDIRatio: 1}})
	addReserveTransfer(t, f, "tx-dep", "Dinheiro reservado Trip", -50)

	got := yieldOf(t, f, "Trip")
	if got.Yield != nil || got.Reason == nil || *got.Reason != "coverage gap" {
		t.Fatalf("yield=%v reason=%v, want unknown with reason 'coverage gap'", got.Yield, got.Reason)
	}
}

func TestATransferNamingNoReserveIsAnUnknownRowOfItsOwn(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}})
	walkReachesBack(t, f)
	addReserveTransfer(t, f, "tx-lost", "Dinheiro reservado Gone", -20)

	yields, err := ledger.New(f.Path).PotYields(t.Context(), "acc-1", yieldFrom, yieldTo)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range yields {
		if candidate.ReserveID == nil && candidate.Reason != nil && *candidate.Reason == "unmatched transfer" {
			return
		}
	}
	t.Fatalf("no row carries the reason 'unmatched transfer' among %d rows", len(yields))
}

func TestAnEntryWithoutAnIdentityIsUnknown(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{Name: "Loose", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{Name: "Loose", Amount: 101, CDIRatio: 1}})
	walkReachesBack(t, f)

	got := yieldOf(t, f, "Loose")
	if got.ReserveID != nil || got.Yield != nil || got.Reason == nil || *got.Reason != "unstable" {
		t.Fatalf("id=%v yield=%v reason=%v, want no id, unknown, 'unstable'", got.ReserveID, got.Yield, got.Reason)
	}
}

func TestNoRowWhenADayHasNoSighting(t *testing.T) {
	f := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 101, CDIRatio: 1}})
	walkReachesBack(t, f)

	yields, err := ledger.New(f.Path).PotYields(t.Context(), "acc-1", "2026-03-01", "2026-03-05")
	if err != nil {
		t.Fatal(err)
	}
	if len(yields) != 0 {
		t.Fatalf("got %d rows for a window before every sighting, want none", len(yields))
	}
}

func TestAWindowThatDoesNotIncreaseIsRefused(t *testing.T) {
	f := yieldLedger(t, []ledgertest.Reserve{}, []ledgertest.Reserve{})
	reader := ledger.New(f.Path)
	for _, window := range [][2]brday.Day{{yieldTo, yieldFrom}, {yieldFrom, yieldFrom}, {"nonsense", yieldTo}} {
		if _, err := reader.PotYields(t.Context(), "acc-1", window[0], window[1]); err == nil {
			t.Errorf("window %v was accepted", window)
		}
	}
}
