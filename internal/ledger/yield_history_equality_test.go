package ledger_test

import (
	"reflect"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// differingFields names each field of the history in which got differs from want.
func differingFields(want, got ledger.YieldHistory) []string {
	var differing []string
	wanted, gotten := reflect.ValueOf(want), reflect.ValueOf(got)
	for index := range wanted.NumField() {
		if !reflect.DeepEqual(wanted.Field(index).Interface(), gotten.Field(index).Interface()) {
			differing = append(differing, wanted.Type().Field(index).Name)
		}
	}
	return differing
}

// transferEdgesLedger holds what the statement leaves out or tells apart: a
// version of a transfer its run cannot place, a transfer orphaned with its
// parent, one on another account, and a document that stopped being a transfer.
func transferEdgesLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := transferHistoryLedger(t)
	f.AddAccount(ledgertest.Account{ID: "acc-2", ItemID: "item-1", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Other", Currency: "BRL", Balance: 5})
	f.AddTransaction(ledgertest.Transaction{ID: "tx-elsewhere", AccountID: "acc-2", Date: instant(t, transferMoment),
		Description: "Dinheiro retirado Trip", Amount: 3, Currency: "BRL", Type: "CREDIT",
		Raw: map[string]any{"operationType": "TRANSFERENCIA_SALDO_RESERVADO"}})
	f.RecordEvent("transactions", "tx-elsewhere", 1, "parent_gone", "2026-03-18")
	run := f.AddRun(ledgertest.Run{Outcome: "complete", FinishedAt: instant(t, "2026-03-19T06:00:00Z")})
	f.UseRun(run)
	f.ObserveTransaction("tx-twin-posted", "2026-03-19", map[string]any{"amount": -8.0})
	f.MarkAmbiguous("transactions", "tx-twin-posted", run)
	f.ObserveTransaction("tx-coffee-posted", "2026-03-19", map[string]any{"operationType": "TRANSFERENCIA_SALDO_RESERVADO"})
	f.ObserveTransaction("tx-dep", "2026-03-19", map[string]any{"operationType": "PIX", "currencyCode": "USD"})
	f.UseRun(1)
	return f
}

func TestTheOneReadingAnswersWhatTheThreeReadingsAnsweredOverEveryInventedLedger(t *testing.T) {
	versionsLedger := yieldLedger(t,
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 100, CDIRatio: 1}},
		[]ledgertest.Reserve{{ID: "pot-a", Name: "Trip", Amount: 150, CDIRatio: 1}})
	walkReachesBack(t, versionsLedger)
	addReserveTransfer(t, versionsLedger, "tx-dep", "Dinheiro reservado Trip", -50)
	versionsLedger.ObserveTransaction("tx-dep", "2026-03-16", map[string]any{"amount": -40.0})
	versionsLedger.RecordEvent("transactions", "tx-dep", 1, "removed", "2026-03-17")

	ledgers := map[string]*ledgertest.Fixture{
		"transfer history": transferHistoryLedger(t),
		"transfer edges":   transferEdgesLedger(t),
		"versions":         versionsLedger,
		"populated":        populated(t),
		"view":             viewLedger(t),
		"rich":             richLedger(t),
		"empty":            ledgertest.New(t),
	}
	versions, removals, unresolved := 0, 0, 0
	for name, f := range ledgers {
		view := viewOf(t, f.Path)
		old, err := view.YieldHistoryInThreeReadings(t.Context())
		if err != nil {
			t.Fatalf("%s, three readings: %v", name, err)
		}
		got, err := view.YieldHistory(t.Context())
		if err != nil {
			t.Fatalf("%s, one reading: %v", name, err)
		}
		if differing := differingFields(old, got); len(differing) > 0 || !reflect.DeepEqual(old, got) {
			t.Errorf("%s: the one reading differs from the three in %v:\n%+v\nwant\n%+v", name, differing, got, old)
		}
		versions, removals, unresolved = versions+len(got.Transfers), removals+len(got.Removals), unresolved+len(got.Unresolved)
	}
	if versions == 0 || removals == 0 || unresolved == 0 {
		t.Errorf("the ledgers hold %d versions, %d removals and %d unresolved ids, want some of each", versions, removals, unresolved)
	}
}
