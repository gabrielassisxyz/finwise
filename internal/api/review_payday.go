package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// salaryOperation is the provider's operation type of a payroll credit.
const salaryOperation = "FOLHA_PAGAMENTO"

// paydayView answers the one figure the bank supports: the balance of the
// account the salary lands on. The cushion comes from a budget that does not
// exist yet, so there is nothing to compare the balance with, and the
// difference and its verdict are left unknown rather than saying "covered".
type paydayView struct {
	Cushion      Reais   `json:"cushion"`
	Balance      *Reais  `json:"balance"`
	BalanceLabel *string `json:"balanceLabel"`
	Difference   *Reais  `json:"difference"`
	Covered      *bool   `json:"covered"`
	Transfers    []any   `json:"transfers"`
}

func readPayday(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	if _, err := onlyMonthArgument(args); err != nil {
		return nil, err
	}
	accounts, err := views.Ledger.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := views.Ledger.CleanTransactions(ctx, wholeLedger)
	if err != nil {
		return nil, err
	}
	view := paydayView{Transfers: []any{}}
	if salary := findSalaryAccount(accounts, rows).Account; salary != nil {
		balance := Reais(salary.Balance)
		label := bankAccountDescription(bankLabels(accounts)[salary.ID], nameOfAccount(*salary))
		view.Balance, view.BalanceLabel = &balance, &label
	}
	return view, nil
}

// salaryFinding says where the salary lands and how sure that is. Receiving is
// how many bank accounts have ever received a payroll credit: a salary can move
// to another account, so more than one is normal. Tied is set when a second
// account has a credit at the very instant the chosen one's latest does, which
// leaves "latest" with no single answer.
type salaryFinding struct {
	Account   *ledger.Account
	Receiving int
	Tied      bool
}

// findSalaryAccount is the bank account with the latest payroll credit. A credit
// still pending has not landed yet. On a tie of instants the lower account id
// wins, so the choice does not depend on how the ledger is stored.
func findSalaryAccount(accounts []ledger.Account, rows []ledger.Transaction) salaryFinding {
	bank := map[string]*ledger.Account{}
	for position := range accounts {
		if accounts[position].Type == ledger.AccountBank {
			bank[accounts[position].ID] = &accounts[position]
		}
	}
	latest := map[string]time.Time{}
	for _, row := range rows {
		if bank[row.AccountID] == nil || !isSalaryCredit(row) {
			continue
		}
		if row.Date.After(latest[row.AccountID]) {
			latest[row.AccountID] = row.Date
		}
	}
	found := salaryFinding{Receiving: len(latest)}
	var chosenAt time.Time
	for id, at := range latest {
		switch {
		case found.Account == nil || at.After(chosenAt):
			found.Account, chosenAt, found.Tied = bank[id], at, false
		case at.Equal(chosenAt):
			found.Tied = true
			if id < found.Account.ID {
				found.Account = bank[id]
			}
		}
	}
	return found
}

func isSalaryCredit(row ledger.Transaction) bool {
	return row.Type == "CREDIT" && row.Status != "PENDING" &&
		row.OperationType != nil && *row.OperationType == salaryOperation
}
