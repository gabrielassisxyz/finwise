package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

const (
	accountSalaryOld  = "acc-salary-old"
	accountSalaryNew  = "acc-salary-new"
	accountSalaryCard = "card-salary"
)

// newSalariedLedger holds two bank accounts that have each received a payroll
// credit, the newer one on the account whose id sorts last, plus every shape
// that must not count: a later credit of another kind, a payroll debit, a
// payroll row on a card, and a payroll credit still pending. It stays apart from
// the shared fixtures because a payroll row there would change every golden file
// that lists transactions.
func newSalariedLedger(t testing.TB) *ledgertest.Fixture {
	t.Helper()
	f := ledgertest.New(t)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-old", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddItem(ledgertest.Item{ID: "item-new", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour)})
	f.AddAccount(ledgertest.Account{ID: accountSalaryOld, ItemID: "item-old", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Corrente", Currency: "BRL", Balance: 2468.13, TransferNumber: "341/0001/10000-1"})
	f.AddAccount(ledgertest.Account{ID: accountSalaryNew, ItemID: "item-new", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Salário", Currency: "BRL", Balance: 777.77, TransferNumber: "323/0001/20000-2"})
	f.AddAccount(ledgertest.Account{ID: accountSalaryCard, ItemID: "item-old", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Cartão Exemplo", Currency: "BRL", Balance: 90})
	payroll := func(id, account string, at time.Time, amount float64, kind, status string) {
		f.AddTransaction(ledgertest.Transaction{ID: id, AccountID: account, Date: at, Description: "FOLHA EXEMPLO",
			Amount: amount, Currency: "BRL", Type: kind, Status: status, Raw: map[string]any{"operationType": "FOLHA_PAGAMENTO"}})
	}
	payroll("pay-old", accountSalaryOld, utc(2026, time.January, 5, 12, 0), 5000, "CREDIT", "")
	payroll("pay-new", accountSalaryNew, utc(2026, time.February, 5, 12, 0), 5200, "CREDIT", "")
	payroll("pay-debit", accountSalaryOld, utc(2026, time.March, 12, 12, 0), -100, "DEBIT", "")
	payroll("pay-card", accountSalaryCard, utc(2026, time.March, 15, 12, 0), 100, "CREDIT", "")
	payroll("pay-pending", accountSalaryOld, utc(2026, time.March, 18, 12, 0), 5200, "CREDIT", "PENDING")
	f.AddTransaction(ledgertest.Transaction{ID: "pix-later", AccountID: accountSalaryOld, Date: utc(2026, time.March, 10, 12, 0),
		Description: "PIX RECEBIDO AMIGO EXEMPLO", Amount: 80, Currency: "BRL", Type: "CREDIT",
		Raw: map[string]any{"operationType": "PIX"}})
	f.AddTransaction(ledgertest.Transaction{ID: "purchase", AccountID: accountSalaryOld, Date: utc(2026, time.March, 11, 12, 0),
		Description: "PADARIA EXEMPLO", Amount: -42, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "CARTAO"}})
	return f
}

func TestThePaydayShowsTheBalanceOfTheAccountThatReceivedTheLatestPayroll(t *testing.T) {
	env := newReadEnv(t, newSalariedLedger(t))
	expectAnswer(t, env, "review.payday", `["2026-03"]`,
		`{"cushion":0.00,"balance":777.77,"balanceLabel":"Mercado Pago · Conta Salário","difference":null,"covered":null,"transfers":[]}`)
}

func TestEveryReviewAnswerConformsOverTheSalariedLedger(t *testing.T) {
	env := newReadEnv(t, newSalariedLedger(t))
	for _, c := range []readCase{{"review.payday", `["2026-03"]`}, {"review.weekly", `["2026-03"]`}} {
		status, body := env.answer(c.method, c.arguments)
		if status != http.StatusOK {
			t.Fatalf("%s answered %d", c.method, status)
		}
		if problem := conformanceProblem(c.method, body); problem != "" {
			t.Errorf("%s does not conform: %s", c.method, problem)
		}
	}
	expectAnswer(t, env, "review.weekly", `["2026-03"]`, emptyWeeklyReview)
}

func rowOf(id, account string, day int, operation, kind, status string) ledger.Transaction {
	return ledger.Transaction{ID: id, AccountID: account, Date: utc(2026, time.March, day, 12, 0),
		Type: kind, Status: status, OperationType: &operation}
}

func bankAccountNamed(id string, balance money.Cents) ledger.Account {
	return ledger.Account{ID: id, Type: ledger.AccountBank, Balance: balance}
}

func TestTheSalaryAccountIsTheOneWithTheLatestPayrollCredit(t *testing.T) {
	accounts := []ledger.Account{bankAccountNamed("a", 1), bankAccountNamed("b", 2)}
	rows := []ledger.Transaction{
		rowOf("1", "a", 20, salaryOperation, "CREDIT", ""),
		rowOf("2", "b", 5, salaryOperation, "CREDIT", ""),
	}
	found := findSalaryAccount(accounts, rows)
	if found.Account == nil || found.Account.ID != "a" || found.Receiving != 2 || found.Tied {
		t.Errorf("found %+v, want a among 2, untied", found)
	}
	reversed := []ledger.Transaction{rows[1], rows[0]}
	if found = findSalaryAccount(accounts, reversed); found.Account == nil || found.Account.ID != "a" {
		t.Error("the choice depends on the order the rows are read in")
	}
}

func TestTheSalaryAccountOfATieIsTheLowerIdAndTheTieIsReported(t *testing.T) {
	accounts := []ledger.Account{bankAccountNamed("b", 2), bankAccountNamed("a", 1)}
	rows := []ledger.Transaction{
		rowOf("1", "b", 5, salaryOperation, "CREDIT", ""),
		rowOf("2", "a", 5, salaryOperation, "CREDIT", ""),
	}
	for attempt := 0; attempt < 20; attempt++ {
		if found := findSalaryAccount(accounts, rows); found.Account == nil || found.Account.ID != "a" || !found.Tied {
			t.Fatalf("a tie was not settled by the account id and reported: %+v", found)
		}
	}
}

// A tie among older credits does not matter: only the latest instant decides.
func TestATieBelowTheLatestCreditIsNotATie(t *testing.T) {
	accounts := []ledger.Account{bankAccountNamed("a", 1), bankAccountNamed("b", 2), bankAccountNamed("c", 3)}
	rows := []ledger.Transaction{
		rowOf("1", "a", 5, salaryOperation, "CREDIT", ""),
		rowOf("2", "b", 5, salaryOperation, "CREDIT", ""),
		rowOf("3", "c", 9, salaryOperation, "CREDIT", ""),
	}
	for attempt := 0; attempt < 20; attempt++ {
		if found := findSalaryAccount(accounts, rows); found.Account.ID != "c" || found.Tied || found.Receiving != 3 {
			t.Fatalf("got %+v, want c among 3, untied", found)
		}
	}
}

func TestOnlyASettledPayrollCreditOnABankAccountCounts(t *testing.T) {
	accounts := []ledger.Account{bankAccountNamed("bank", 1), {ID: "card", Type: ledger.AccountCredit}}
	rows := []ledger.Transaction{
		rowOf("1", "bank", 5, "PIX", "CREDIT", ""),
		rowOf("2", "bank", 6, salaryOperation, "DEBIT", ""),
		rowOf("3", "bank", 7, salaryOperation, "CREDIT", "PENDING"),
		rowOf("4", "card", 8, salaryOperation, "CREDIT", ""),
		rowOf("5", "elsewhere", 9, salaryOperation, "CREDIT", ""),
		{ID: "6", AccountID: "bank", Date: utc(2026, time.March, 10, 12, 0), Type: "CREDIT"},
	}
	if found := findSalaryAccount(accounts, rows); found.Account != nil || found.Receiving != 0 || found.Tied {
		t.Errorf("found %+v with no payroll credit that counts", found)
	}
}

func TestTheSalariedLedgerHasTwoReceivingAccountsAndOneChosen(t *testing.T) {
	fixture := newSalariedLedger(t)
	reader := ledger.New(fixture.Path)
	accounts, err := reader.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := reader.CleanTransactions(t.Context(), wholeLedger)
	if err != nil {
		t.Fatal(err)
	}
	found := findSalaryAccount(accounts, rows)
	if found.Account == nil || found.Account.ID != accountSalaryNew || found.Receiving != 2 || found.Tied {
		t.Errorf("found %+v, want %s among 2, untied", found, accountSalaryNew)
	}
}
