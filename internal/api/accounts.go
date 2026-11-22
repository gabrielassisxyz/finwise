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

func readAccountsList(ctx context.Context, _ []json.RawMessage, views Views) (any, error) {
	accounts, err := views.Ledger.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := views.Ledger.CleanTransactions(ctx, wholeLedger)
	if err != nil {
		return nil, err
	}
	today, banks, latest := brday.DayOf(nowOf(ctx)), bankLabels(accounts), latestDayOf(rows)
	list := make([]accountRow, 0, len(accounts))
	for _, account := range orderedAccounts(accounts, banks) {
		list = append(list, accountRowOf(account, banks[account.ID], isIdle(latest[account.ID], today)))
	}
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

// orderedAccounts lists accounts before cards, then by bank, name and id, so the
// order does not depend on how the ledger happens to be stored.
func orderedAccounts(accounts []ledger.Account, banks map[string]string) []ledger.Account {
	ordered := append([]ledger.Account(nil), accounts...)
	sort.SliceStable(ordered, func(a, b int) bool {
		left, right := ordered[a], ordered[b]
		if kindOfAccount(left) != kindOfAccount(right) {
			return kindOfAccount(left) == accountKindBank
		}
		if banks[left.ID] != banks[right.ID] {
			return banks[left.ID] < banks[right.ID]
		}
		if nameOfAccount(left) != nameOfAccount(right) {
			return nameOfAccount(left) < nameOfAccount(right)
		}
		return left.ID < right.ID
	})
	return ordered
}

// readAccountDetail answers null for an unknown id and for a card, which has a
// page of its own.
func readAccountDetail(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	id, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	accounts, err := views.Ledger.Accounts(ctx)
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

// accountLabelsOf is how an account is named wherever a transaction or a filter
// refers to it: a bank account by its bank, a card by its own name. Two accounts
// that would read the same are told apart by their names.
func accountLabelsOf(accounts []ledger.Account, banks map[string]string) map[string]string {
	labels := make(map[string]string, len(accounts))
	taken := map[string]int{}
	for _, account := range accounts {
		labels[account.ID] = plainLabelOf(account, banks[account.ID])
		taken[labels[account.ID]]++
	}
	for _, account := range accounts {
		if taken[labels[account.ID]] > 1 {
			labels[account.ID] = bankAccountDescription(banks[account.ID], nameOfAccount(account))
		}
	}
	return labels
}

func plainLabelOf(account ledger.Account, bank string) string {
	if account.Type == ledger.AccountCredit {
		return nameOfAccount(account)
	}
	return bank
}
