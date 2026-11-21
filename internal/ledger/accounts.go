package ledger

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// AccountType is the provider's account type, BANK or CREDIT.
type AccountType string

const (
	AccountBank   AccountType = "BANK"
	AccountCredit AccountType = "CREDIT"
)

// Account is a current account of the ledger.
type Account struct {
	ID       string
	ItemID   string
	Type     AccountType
	Subtype  string
	Name     string
	Currency string
	Balance  money.Cents
	// TransferNumber is the raw bankData.transferNumber; its first segment
	// names the bank, which is for the caller to read.
	TransferNumber *string
	// Credit is set on a card account.
	Credit *CreditData
}

// CreditData is what accounts.raw.creditData says about a card.
type CreditData struct {
	CreditLimit          *money.Cents
	AvailableCreditLimit *money.Cents
	BalanceCloseDate     *brday.Day
	BalanceDueDate       *brday.Day
}

type accountRaw struct {
	BankData *struct {
		TransferNumber *string `json:"transferNumber"`
	} `json:"bankData"`
	CreditData *struct {
		CreditLimit          *float64 `json:"creditLimit"`
		AvailableCreditLimit *float64 `json:"availableCreditLimit"`
		BalanceCloseDate     *string  `json:"balanceCloseDate"`
		BalanceDueDate       *string  `json:"balanceDueDate"`
	} `json:"creditData"`
}

// Accounts returns every current account, by id.
func (reader *Reader) Accounts(ctx context.Context) ([]Account, error) {
	var accounts []Account
	err := reader.read(ctx, func(tx *sql.Tx) error {
		var err error
		accounts, err = queryAccountRows(ctx, tx)
		return err
	})
	return accounts, err
}

func queryAccountRows(ctx context.Context, tx *sql.Tx) ([]Account, error) {
	rows, err := tx.QueryContext(ctx, queryAccounts)
	if err != nil {
		return nil, fmt.Errorf("ledger: accounts: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func scanAccount(rows *sql.Rows) (Account, error) {
	var (
		account                          Account
		itemID, kind, subtype, name, cur sql.NullString
		balance                          sql.NullFloat64
		raw                              string
	)
	if err := rows.Scan(&account.ID, &itemID, &kind, &subtype, &name, &balance, &cur, &raw); err != nil {
		return Account{}, fmt.Errorf("ledger: accounts: %w", err)
	}
	account.ItemID, account.Type = itemID.String, AccountType(kind.String)
	account.Subtype, account.Name, account.Currency = subtype.String, name.String, cur.String

	var err error
	if account.Balance, err = requiredCents(balance, "accounts.balance of "+account.ID); err != nil {
		return Account{}, err
	}
	return account, completeAccountFromRaw(&account, raw)
}

func completeAccountFromRaw(account *Account, raw string) error {
	var body accountRaw
	if err := decodeRaw(raw, "accounts", &body); err != nil {
		return err
	}
	if body.BankData != nil {
		account.TransferNumber = body.BankData.TransferNumber
	}
	if body.CreditData == nil {
		return nil
	}
	credit, err := creditDataOf(account.ID, body)
	account.Credit = credit
	return err
}

func creditDataOf(accountID string, body accountRaw) (*CreditData, error) {
	var (
		credit CreditData
		err    error
	)
	source := body.CreditData
	if credit.CreditLimit, err = centsFromJSON(source.CreditLimit, "creditLimit of "+accountID); err != nil {
		return nil, err
	}
	if credit.AvailableCreditLimit, err = centsFromJSON(source.AvailableCreditLimit, "availableCreditLimit of "+accountID); err != nil {
		return nil, err
	}
	if credit.BalanceCloseDate, err = optionalDayOfDatePart(nullable(source.BalanceCloseDate), "balanceCloseDate of "+accountID); err != nil {
		return nil, err
	}
	if credit.BalanceDueDate, err = optionalDayOfDatePart(nullable(source.BalanceDueDate), "balanceDueDate of "+accountID); err != nil {
		return nil, err
	}
	return &credit, nil
}

func nullable(text *string) sql.NullString {
	if text == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *text, Valid: true}
}
