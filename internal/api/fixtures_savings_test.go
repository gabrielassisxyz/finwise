package api

import (
	"database/sql"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// What the full ledger holds for the pots, the investments and the net worth,
// added to it and never changing what was there. Everything is invented.

const (
	accountNubank = "acc-nu"
	potNubankMain = "pot-nu-1"
	potNubankOld  = "pot-nu-old"

	investmentNubank    = "inv-nu-1"
	investmentWithdrawn = "inv-nu-2"
	investmentItau      = "inv-it-1"

	txNubankDepositMatched   = "tx-nu-deposit-matched"
	txNubankDepositUnmatched = "tx-nu-deposit-unmatched"
)

// walkReachesBack records that the account's transaction walk went back past
// every sighting, which the yield statement requires before it trusts a
// transfer. Call it before the account's first transaction is added: a scope
// cannot be changed once it is closed.
func walkReachesBack(t testing.TB, f *ledgertest.Fixture, accountID string) {
	t.Helper()
	database, err := sql.Open("sqlite", f.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	_, err = database.Exec(`INSERT INTO scopes (run_id, kind, parent, expected, outcome, infer_absence,
	        status, rows, min_date, min_instant, closed_at, closed_day)
	        VALUES (1, 'transactions', ?, 1, 'complete', 1, 200, 0, '0001-01-01',
	                '2026-01-01T00:00:00.000Z', '2026-03-15T03:00:00.000Z', '2026-03-15')`, accountID)
	if err != nil {
		t.Fatal(err)
	}
}

func sightedAt(day string, balance float64, reserves ...ledgertest.Reserve) map[string]any {
	override := ledgertest.ReservesOverride(reserves...)
	override["updatedAt"] = day + "T02:00:00.000Z"
	override["balance"] = balance
	return override
}

// addFixtureSavings gives the Mercado Pago account two earlier sightings, so
// that its pot has a month's yield and its transfer is placed on it, and adds a
// Nubank account with two pots, one of them without an identity and one that
// closed, two deposits, and investments on its connection and on the Itaú one.
func addFixtureSavings(t testing.TB, f *ledgertest.Fixture) {
	t.Helper()
	f.ObserveAccount(accountPots, "2026-03-08", sightedAt("2026-03-08", 300.1,
		ledgertest.Reserve{ID: "pot-1", Name: "Caixinha da casa", Amount: 590, CDIRatio: 1.1}))
	f.ObserveAccount(accountPots, "2026-03-10", sightedAt("2026-03-10", 300.1,
		ledgertest.Reserve{ID: "pot-1", Name: "Caixinha da casa", Amount: 790, CDIRatio: 1.1}))
	addNubankAccount(t, f)
	addFixtureInvestments(f)
}

func addNubankAccount(t testing.TB, f *ledgertest.Fixture) {
	t.Helper()
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-nu", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddAccount(ledgertest.Account{ID: accountNubank, ItemID: "item-nu", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Nubank", Currency: "BRL", Balance: 500, TransferNumber: "260/0001/30000-3",
		Reserves: []ledgertest.Reserve{
			{ID: potNubankMain, Name: "Reserva", Amount: 1225.8, CDIRatio: 1},
			{Name: "Sem identidade", Amount: 50, CDIRatio: 1},
		}})
	f.ObserveAccount(accountNubank, "2026-01-20", sightedAt("2026-01-20", 120,
		ledgertest.Reserve{ID: potNubankMain, Name: "Reserva", Amount: 900, CDIRatio: 1}))
	f.ObserveAccount(accountNubank, "2026-02-10", sightedAt("2026-02-10", 260,
		ledgertest.Reserve{ID: potNubankMain, Name: "Reserva", Amount: 1000, CDIRatio: 1}))
	f.ObserveAccount(accountNubank, "2026-03-13", sightedAt("2026-03-13", 400,
		ledgertest.Reserve{ID: potNubankMain, Name: "Reserva", Amount: 1100, CDIRatio: 1},
		ledgertest.Reserve{ID: potNubankOld, Name: "Antiga", Amount: 20, CDIRatio: 1}))
	f.ObserveAccount(accountNubank, "2026-03-14", sightedAt("2026-03-14", 450,
		ledgertest.Reserve{ID: potNubankMain, Name: "Reserva", Amount: 1150, CDIRatio: 1},
		ledgertest.Reserve{Name: "Sem identidade", Amount: 50, CDIRatio: 1}))
	walkReachesBack(t, f, accountNubank)
	addDeposit(f, txNubankDepositMatched, 50)
	addDeposit(f, txNubankDepositUnmatched, 75)
}

// addDeposit puts money into the "Reserva" pot on 13 March: negative on the account.
func addDeposit(f *ledgertest.Fixture, id string, amount float64) {
	f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: accountNubank,
		Date: utc(2026, time.March, 13, 12, 0), Day: "2026-03-13", Description: "Dinheiro reservado Reserva",
		Amount: -amount, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "TRANSFERENCIA_SALDO_RESERVADO"}})
}

func addFixtureInvestments(f *ledgertest.Fixture) {
	f.AddInvestment(ledgertest.Investment{ID: investmentNubank, ItemID: "item-nu", Type: "FIXED_INCOME", Subtype: "CDB",
		Name: "CDB Exemplo", Currency: "BRL", Balance: 3000,
		Raw: map[string]any{"amount": 3100, "amountOriginal": 2800, "taxes": 100, "rate": 115, "rateType": "CDI",
			"dueDate": "2028-03-15T00:00:00.000Z"}})
	f.ObserveInvestment(investmentNubank, "2026-03-14", map[string]any{
		"amount": 3090, "amountWithdrawal": 2992, "balance": 2992, "taxes": 98})
	f.AddInvestment(ledgertest.Investment{ID: investmentWithdrawn, ItemID: "item-nu", Type: "FIXED_INCOME", Subtype: "CDB",
		Name: "CDB encerrado", Currency: "BRL", Balance: 0,
		Raw: map[string]any{"amount": 0, "amountOriginal": 0, "taxes": 0, "rate": 100, "rateType": "CDI",
			"status": "TOTAL_WITHDRAWAL"}})
	f.AddInvestment(ledgertest.Investment{ID: investmentItau, ItemID: "item-itau", Type: "FIXED_INCOME", Subtype: "LCI",
		Name: "LCI Exemplo", Currency: "BRL", Balance: 7000,
		Raw: map[string]any{"amount": 7000, "amountOriginal": 6500, "taxes": 0, "rate": 95, "rateType": "CDI"}})
}
