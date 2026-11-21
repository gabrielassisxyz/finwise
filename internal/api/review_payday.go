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

func readPayday(ctx context.Context, args []json.RawMessage, reader *ledger.Reader) (any, error) {
	if _, err := onlyMonthArgument(args); err != nil {
		return nil, err
	}
	accounts, err := reader.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := reader.CleanTransactions(ctx, wholeLedger)
	if err != nil {
		return nil, err
	}
	view := paydayView{Transfers: []any{}}
	if salary, _ := findSalaryAccount(accounts, rows); salary != nil {
		balance := Reais(salary.Balance)
		label := bankAccountDescription(bankLabels(accounts)[salary.ID], nameOfAccount(*salary))
		view.Balance, view.BalanceLabel = &balance, &label
	}
	return view, nil
}

// findSalaryAccount is the bank account with the latest payroll credit, and how
// many bank accounts have received one at all. A credit still pending has not
// landed yet. On a tie of instants the lower account id wins, so the choice does
// not depend on how the ledger is stored.
func findSalaryAccount(accounts []ledger.Account, rows []ledger.Transaction) (chosen *ledger.Account, receiving int) {
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
	var chosenAt time.Time
	for id, at := range latest {
		receiving++
		if chosen == nil || at.After(chosenAt) || (at.Equal(chosenAt) && id < chosen.ID) {
			chosen, chosenAt = bank[id], at
		}
	}
	return chosen, receiving
}

func isSalaryCredit(row ledger.Transaction) bool {
	return row.Type == "CREDIT" && row.Status != "PENDING" &&
		row.OperationType != nil && *row.OperationType == salaryOperation
}
