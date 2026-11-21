package api

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// A ledger built to make the collector's yield statement answer every reason it
// has, over windows between any two of its sightings. Everything is invented.
//
// acc-y holds one pot ("Trip", pot-a) through the whole of March 2026 and every
// day brings one thing: a deposit, a withdrawal, a pot that opens and one that
// closes, a transfer the bank has not settled, and so on down to a sighting
// with no time at all. acc-z holds another pot and shows that a transfer of one
// account is never counted for the other.

const (
	yieldAccount      = "acc-y"
	yieldOtherAccount = "acc-z"
)

func marchDay(day int) string { return fmt.Sprintf("2026-03-%02d", day) }

func marchAt(day int, clock string) string { return fmt.Sprintf("2026-03-%02dT%s.000Z", day, clock) }

// rawReserve is a pot entry as the bank writes it. An empty id leaves the
// identification out.
func rawReserve(id, name string, amount any) map[string]any {
	entry := map[string]any{"name": name, "availableAmounts": []any{map[string]any{
		"currencyCode": "BRL", "amount": amount,
		"remuneration": map[string]any{"indexer": "CDI", "postFixedIndexerPercentage": 1.0},
	}}}
	if id != "" {
		entry["identification"] = id
	}
	return entry
}

func inDollars(entry map[string]any) map[string]any {
	entry["availableAmounts"].([]any)[0].(map[string]any)["currencyCode"] = "USD"
	return entry
}

func withTwoAmounts(entry map[string]any) map[string]any {
	amounts := entry["availableAmounts"].([]any)
	entry["availableAmounts"] = append(amounts, amounts[0])
	return entry
}

// sightReserves records a sighting of the account on a day of March with the
// provider's own time and the entries it held.
func sightReserves(f *ledgertest.Fixture, account string, day int, updated any, entries ...map[string]any) {
	list := make([]any, 0, len(entries))
	for _, entry := range entries {
		list = append(list, entry)
	}
	f.ObserveAccount(account, marchDay(day), map[string]any{
		"updatedAt": updated, "balance": 10.0, "bankData": map[string]any{"reservedBalances": list},
	})
}

func trip(amount float64) map[string]any { return rawReserve("pot-a", "Trip", amount) }

// walkSince records a complete walk of the account's transactions that reached
// back to the instant, in the first run. It must come before the account's first
// transaction.
func walkSince(t testing.TB, f *ledgertest.Fixture, accountID, since string) {
	t.Helper()
	database, err := sql.Open("sqlite", f.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	_, err = database.Exec(`INSERT INTO scopes (run_id, kind, parent, expected, outcome, infer_absence,
	        status, rows, min_date, min_instant, closed_at, closed_day)
	        VALUES (1, 'transactions', ?, 1, 'complete', 1, 200, 0, '0001-01-01', ?,
	                '2026-03-15T03:00:00.000Z', '2026-03-15')`, accountID, since)
	if err != nil {
		t.Fatal(err)
	}
}

// reserveTransfer is a transfer between an account and its pots, dated at noon
// of one day and first seen by the ledger on another.
func reserveTransfer(id, account string, dated int, seen int, description string, amount float64) ledgertest.Transaction {
	return ledgertest.Transaction{
		ID: id, AccountID: account, Date: utc(2026, time.March, dated, 12, 0), Day: marchDay(seen),
		Description: description, Amount: amount, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "TRANSFERENCIA_SALDO_RESERVADO"},
	}
}

// yieldScenarioLedger is the ledger the equality test runs over.
func yieldScenarioLedger(t testing.TB) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	addYieldAccounts(f)
	walkSince(t, f, yieldAccount, marchAt(3, "00:00:00"))
	walkSince(t, f, yieldOtherAccount, "2026-01-01T00:00:00.000Z")
	addYieldSightings(f)
	addYieldTransfers(f)
	addSecondAccountHistory(f)
	return f
}

func addYieldAccounts(f *ledgertest.Fixture) {
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-y", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	// The default sighting, on the 15th, holds the pot in dollars.
	f.AddAccount(ledgertest.Account{ID: yieldAccount, ItemID: "item-y", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Y", Currency: "BRL", Balance: 10, TransferNumber: "323/0001/1-1",
		Raw: map[string]any{"bankData": map[string]any{"reservedBalances": []any{inDollars(trip(131.6))}}}})
	f.AddAccount(ledgertest.Account{ID: yieldOtherAccount, ItemID: "item-y", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Z", Currency: "BRL", Balance: 10, TransferNumber: "323/0001/2-2",
		Reserves: []ledgertest.Reserve{{ID: "pot-z", Name: "Zed", Amount: 20, CDIRatio: 1}}})
}

// addYieldSightings gives acc-y a sighting for each day of the month but the
// 15th, which the account itself made.
func addYieldSightings(f *ledgertest.Fixture) {
	updated := func(day int) string { return marchAt(day, "02:00:00") }
	twins := []map[string]any{rawReserve("pot-c", "Twin", 1.0), rawReserve("pot-d", "Twin", 2.0)}
	sightReserves(f, yieldAccount, 1, updated(1), trip(100))
	sightReserves(f, yieldAccount, 2, updated(2), trip(100.10))
	sightReserves(f, yieldAccount, 3, updated(3), trip(100.20))
	sightReserves(f, yieldAccount, 4, updated(4), trip(150.50))
	sightReserves(f, yieldAccount, 5, updated(5), trip(130.60))
	sightReserves(f, yieldAccount, 6, updated(6), trip(130.70), rawReserve("pot-b", "Car", 10.0))
	sightReserves(f, yieldAccount, 7, updated(7), trip(130.80))
	sightReserves(f, yieldAccount, 8, updated(8), trip(130.90))
	sightReserves(f, yieldAccount, 9, updated(9), trip(131.00))
	sightReserves(f, yieldAccount, 10, updated(10), trip(131.10))
	sightReserves(f, yieldAccount, 11, updated(11), append([]map[string]any{trip(131.20)}, twins...)...)
	sightReserves(f, yieldAccount, 12, updated(12), append([]map[string]any{trip(131.30)}, twins...)...)
	sightReserves(f, yieldAccount, 13, updated(13), rawReserve("pot-a", "Trip2", 131.40))
	sightReserves(f, yieldAccount, 14, updated(14), rawReserve("pot-a", "Trip2", 131.50),
		rawReserve("", "Loose", 3.0), rawReserve("dup", "Pair", 4.0), rawReserve("dup", "Pair", 5.0))
	addYieldFaultySightings(f, updated)
}

func addYieldFaultySightings(f *ledgertest.Fixture, updated func(int) string) {
	sightReserves(f, yieldAccount, 16, updated(16), trip(131.70), rawReserve("pot-e", "Odd", 10.005))
	sightReserves(f, yieldAccount, 17, updated(17), trip(131.80), rawReserve("pot-e", "Odd", "12.30"))
	sightReserves(f, yieldAccount, 18, updated(18), trip(131.90), withTwoAmounts(rawReserve("pot-f", "Wide", 1.0)))
	sightReserves(f, yieldAccount, 19, updated(19), trip(132.00))
	sightReserves(f, yieldAccount, 20, updated(20), trip(132.10))
	sightReserves(f, yieldAccount, 21, updated(21), trip(132.20))
	f.ObserveAccount(yieldAccount, marchDay(22), map[string]any{"updatedAt": updated(22), "bankData": map[string]any{}})
	// The 23rd is seen by a later run that cannot tell which parent it belongs to.
	f.AddRun(ledgertest.Run{Outcome: "complete", FinishedAt: utc(2026, time.March, 23, 6, 0)})
	f.UseRun(2)
	sightReserves(f, yieldAccount, 23, updated(23), trip(132.30))
	f.MarkAmbiguous("accounts", yieldAccount, 2)
	f.UseRun(1)
	// The 24th and 25th differ by less than the second the statement compares.
	sightReserves(f, yieldAccount, 24, updated(24), trip(132.40))
	sightReserves(f, yieldAccount, 25, "2026-03-24T02:00:00.900Z", trip(132.50))
	sightReserves(f, yieldAccount, 26, nil, trip(132.60))
	sightReserves(f, yieldAccount, 27, updated(27), trip(132.70))
}

func addYieldTransfers(f *ledgertest.Fixture) {
	add := func(transaction ledgertest.Transaction) { f.AddTransaction(transaction) }
	add(reserveTransfer("tx-deposit", yieldAccount, 3, 4, "Dinheiro reservado Trip", -50))
	withdrawal := reserveTransfer("tx-withdrawal", yieldAccount, 4, 5, "Dinheiro retirado Trip", 20)
	add(withdrawal)
	f.ObserveTransaction("tx-withdrawal", marchDay(9), map[string]any{"amount": 25.0})
	pending := reserveTransfer("tx-pending", yieldAccount, 7, 8, "Dinheiro reservado Trip", -5)
	pending.Status = "PENDING"
	add(pending)
	twin := reserveTransfer("tx-twin-pending", yieldAccount, 8, 9, "Dinheiro reservado Trip", -7)
	twin.Status = "PENDING"
	add(twin)
	settled := reserveTransfer("tx-twin-posted", yieldAccount, 8, 9, "Dinheiro reservado Trip", -7)
	settled.Date = utc(2026, time.March, 8, 11, 0)
	add(settled)
	add(reserveTransfer("tx-sign", yieldAccount, 9, 10, "Dinheiro reservado Trip", 5))
	add(reserveTransfer("tx-no-prefix", yieldAccount, 10, 11, "Pix qualquer", -3))
	add(reserveTransfer("tx-ghost", yieldAccount, 10, 11, "Dinheiro reservado Ghost", -3))
	add(reserveTransfer("tx-twin-name", yieldAccount, 11, 12, "Dinheiro reservado Twin", -4))
	add(reserveTransfer("tx-renamed", yieldAccount, 12, 13, "Dinheiro reservado Trip2", -6))
	add(reserveTransfer("tx-loose", yieldAccount, 13, 14, "Dinheiro reservado Loose", -1))
	addYieldTransferFaults(f, add)
}

func addYieldTransferFaults(f *ledgertest.Fixture, add func(ledgertest.Transaction)) {
	dollars := reserveTransfer("tx-dollars", yieldAccount, 17, 18, "Dinheiro reservado Trip", -2)
	dollars.Currency = "USD"
	add(dollars)
	add(reserveTransfer("tx-cents", yieldAccount, 17, 18, "Dinheiro reservado Trip", -2.505))
	add(reserveTransfer("tx-removed", yieldAccount, 18, 19, "Dinheiro reservado Trip", -9))
	f.RecordEvent("transactions", "tx-removed", 2, "removed", marchDay(19))
	add(reserveTransfer("tx-late", yieldAccount, 18, 21, "Dinheiro reservado Trip", -1))
	add(reserveTransfer("tx-other-account", yieldOtherAccount, 3, 4, "Dinheiro reservado Trip", -50))
	// Removed long after the window it is in, which a window ending before then cannot know.
	add(reserveTransfer("tx-removed-later", yieldAccount, 5, 6, "Dinheiro reservado Trip", -2))
	f.RecordEvent("transactions", "tx-removed-later", 2, "removed", marchDay(10))
	// On the very instant of the 20th's sighting: the end of one window, never the start of the next.
	edge := reserveTransfer("tx-edge", yieldAccount, 20, 20, "Dinheiro reservado Trip", -1.5)
	edge.Date = utc(2026, time.March, 20, 2, 0)
	add(edge)
	// An event of the run that saw the transfer says nothing about what came after it.
	add(reserveTransfer("tx-same-run-event", yieldAccount, 19, 20, "Dinheiro retirado Trip", 3))
	f.RecordEvent("transactions", "tx-same-run-event", 1, "parent_gone", marchDay(20))
}

func addSecondAccountHistory(f *ledgertest.Fixture) {
	zed := func(amount float64) map[string]any { return rawReserve("pot-z", "Zed", amount) }
	for day := 1; day <= 5; day++ {
		sightReserves(f, yieldOtherAccount, day, marchAt(day, "02:00:00"), zed(20+float64(day)/10))
	}
	f.AddRun(ledgertest.Run{Outcome: "complete", FinishedAt: utc(2026, time.March, 24, 6, 0)})
	f.UseRun(3)
	sightReserves(f, yieldOtherAccount, 6, marchAt(6, "02:00:00"), zed(20.6))
	f.UseRun(1)
}
