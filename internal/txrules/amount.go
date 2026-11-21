package txrules

import (
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

const reais = "BRL"

// signedAmountInReais gives the app's amount: negative is money out on every
// account type. The row's type is the direction; the stored sign is inverted
// between BANK and CREDIT accounts, so it is only a fallback for a type that is
// neither DEBIT nor CREDIT, read through the account's convention.
func signedAmountInReais(row ledger.Transaction, account *ledger.Account) money.Cents {
	magnitude := absolute(valueInReais(row))
	switch row.Type {
	case "DEBIT":
		return -magnitude
	case "CREDIT":
		return magnitude
	}
	stored := valueInReais(row)
	if account != nil && account.Type == ledger.AccountCredit {
		return -stored
	}
	return stored
}

// valueInReais is the amount in the account currency when the provider gives
// one, which is how a foreign purchase is stated in reais.
func valueInReais(row ledger.Transaction) money.Cents {
	if row.AmountInAccountCurrency != nil {
		return *row.AmountInAccountCurrency
	}
	return row.Amount
}

// originalOf keeps what a foreign row was stated in. A row with no value in
// reais is marked unconverted instead of passing a foreign amount off as reais.
func originalOf(row ledger.Transaction) (original *Original, unconverted bool) {
	if row.Currency == "" || row.Currency == reais {
		return nil, false
	}
	return &Original{Amount: absolute(row.Amount), Currency: row.Currency},
		row.AmountInAccountCurrency == nil
}

func absolute(amount money.Cents) money.Cents {
	if amount < 0 {
		return -amount
	}
	return amount
}
