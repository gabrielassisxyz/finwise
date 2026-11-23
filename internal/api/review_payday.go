package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// salaryOperation is the provider's operation type of a payroll credit.
const salaryOperation = "FOLHA_PAGAMENTO"

// paydayView sets the balance of the account the salary lands on against the
// cushion of the month's budget. A month whose budget has no line has no
// cushion to compare with, so the difference and its verdict are left unknown
// rather than saying "covered" against a zero that is only an absence.
type paydayView struct {
	Cushion      Reais                `json:"cushion"`
	Balance      *Reais               `json:"balance"`
	BalanceLabel *string              `json:"balanceLabel"`
	Difference   *Reais               `json:"difference"`
	Covered      *bool                `json:"covered"`
	Transfers    []paydayTransferWire `json:"transfers"`
}

// paydayTransferWire is the contract's PaydayTransfer. Nothing marks a transfer
// by itself yet, so a done one was always marked by the person.
type paydayTransferWire struct {
	Key          string  `json:"key"`
	Name         string  `json:"name"`
	Amount       Reais   `json:"amount"`
	State        string  `json:"state"`
	DoneOn       *string `json:"doneOn"`
	MarkedByHand bool    `json:"markedByHand"`
}

const (
	transferPending = "pending"
	transferDone    = "done"
)

func readPayday(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	month, err := onlyMonthArgument(args)
	if err != nil {
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
	planned, err := transfersOfMonth(ctx, views, month)
	if err != nil {
		return nil, err
	}
	cushion := budgetrules.Cushion(planned.plan)
	view := paydayView{Cushion: Reais(cushion), Transfers: paydayTransfersWire(planned.transfers)}
	if salary := findSalaryAccount(accounts, rows).Account; salary != nil {
		balance := Reais(salary.Balance)
		label := bankAccountDescription(bankLabels(accounts)[salary.ID], nameOfAccount(*salary))
		view.Balance, view.BalanceLabel = &balance, &label
		if planned.budget.HasLines() {
			difference := balance - Reais(cushion)
			covered := difference >= 0
			view.Difference, view.Covered = &difference, &covered
		}
	}
	return view, nil
}

func paydayTransfersWire(transfers []budgetrules.MarkedTransfer) []paydayTransferWire {
	wire := make([]paydayTransferWire, 0, len(transfers))
	for _, transfer := range transfers {
		shown := paydayTransferWire{Key: transfer.Key, Name: transfer.Name, Amount: Reais(transfer.Amount), State: transferPending}
		if transfer.Done() {
			shown.State, shown.DoneOn, shown.MarkedByHand = transferDone, textPointer(string(transfer.DoneOn)), true
		}
		wire = append(wire, shown)
	}
	return wire
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
