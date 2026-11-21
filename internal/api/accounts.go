package api

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// An account with nothing from the bank for longer than this is shown faded.
const idleAfterDays = 30

const (
	accountKindBank = "Conta"
	accountKindCard = "Cartão"

	// bucketAvailable is where a bank account counts in the net worth. It is a
	// rule of the contract, not something a person chooses: only a pot has a
	// bucket of its own.
	bucketAvailable = "Disponível"
)

type accountRow struct {
	ID     string `json:"id"`
	Bank   string `json:"bank"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Amount Reais  `json:"amount"`
	Idle   bool   `json:"idle"`
}

type accountDetail struct {
	ID            string `json:"id"`
	Bank          string `json:"bank"`
	Name          string `json:"name"`
	LedgerAccount string `json:"ledgerAccount"`
	Bucket        string `json:"bucket"`
	Balance       Reais  `json:"balance"`
}

func registerAccounts(registry *Registry) {
	registry.Register("accounts.list", readAccountsList)
	registry.Register("accounts.detail", readAccountDetail)
}

func readAccountsList(ctx context.Context, _ []json.RawMessage, reader *ledger.Reader) (any, error) {
	accounts, err := reader.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := reader.CleanTransactions(ctx, wholeLedger)
	if err != nil {
		return nil, err
	}
	today, banks, latest := brday.DayOf(nowOf(ctx)), bankLabels(accounts), latestDayOf(rows)
	list := make([]accountRow, 0, len(accounts))
	for _, account := range accounts {
		list = append(list, accountRowOf(account, banks[account.ID], isIdle(latest[account.ID], today)))
	}
	sortAccountRows(list)
	return list, nil
}

func accountRowOf(account ledger.Account, bank string, idle bool) accountRow {
	return accountRow{
		ID: account.ID, Bank: bank, Name: nameOfAccount(account), Kind: kindOfAccount(account),
		Amount: Reais(account.Balance), Idle: idle,
	}
}

func kindOfAccount(account ledger.Account) string {
	if account.Type == ledger.AccountCredit {
		return accountKindCard
	}
	return accountKindBank
}

func nameOfAccount(account ledger.Account) string {
	if account.Name != "" {
		return account.Name
	}
	if account.Type == ledger.AccountCredit {
		return unnamedCard
	}
	return unnamedAccount
}

// sortAccountRows lists accounts before cards, then by bank, name and id, so the
// order does not depend on how the ledger happens to be stored.
func sortAccountRows(rows []accountRow) {
	sort.SliceStable(rows, func(a, b int) bool {
		left, right := rows[a], rows[b]
		if left.Kind != right.Kind {
			return left.Kind == accountKindBank
		}
		if left.Bank != right.Bank {
			return left.Bank < right.Bank
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.ID < right.ID
	})
}

// readAccountDetail answers null for an unknown id and for a card, which has a
// page of its own.
func readAccountDetail(ctx context.Context, args []json.RawMessage, reader *ledger.Reader) (any, error) {
	id, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	accounts, err := reader.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	banks := bankLabels(accounts)
	for _, account := range accounts {
		if account.ID != id || account.Type == ledger.AccountCredit {
			continue
		}
		return accountDetail{
			ID: account.ID, Bank: banks[account.ID], Name: nameOfAccount(account),
			LedgerAccount: account.ID, Bucket: bucketAvailable, Balance: Reais(account.Balance),
		}, nil
	}
	return nil, nil
}

func latestDayOf(rows []ledger.Transaction) map[string]brday.Day {
	latest := map[string]brday.Day{}
	for _, row := range rows {
		if row.Day > latest[row.AccountID] {
			latest[row.AccountID] = row.Day
		}
	}
	return latest
}

// isIdle is true for an account with no row at all.
func isIdle(latest, today brday.Day) bool {
	if latest == "" {
		return true
	}
	return daysBetween(latest, today) > idleAfterDays
}

func daysBetween(from, to brday.Day) int {
	start, errStart := time.Parse("2006-01-02", string(from))
	end, errEnd := time.Parse("2006-01-02", string(to))
	if errStart != nil || errEnd != nil {
		return 0
	}
	return int(end.Sub(start) / (24 * time.Hour))
}
