package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Transaction is a row of the clean list. The amount carries the provider's
// sign, which differs between BANK and CREDIT accounts; reading it is for the
// caller.
type Transaction struct {
	ID        string
	AccountID string
	// Date is the UTC instant; Day is the Brazil day it falls on.
	Date        time.Time
	Day         brday.Day
	Description string
	Amount      money.Cents
	// Currency is the currency of Amount, which for a foreign purchase is not BRL.
	Currency string
	// Type is the provider's direction, DEBIT or CREDIT.
	Type     string
	Category *string

	Status                  string
	OperationType           *string
	AmountInAccountCurrency *money.Cents
	DescriptionRaw          *string
	Payment                 *PaymentData
	Card                    *CardMetadata
}

// PaymentData is raw.paymentData, present on bank rows.
type PaymentData struct {
	Method   *string
	Payer    *Party
	Receiver *Party
}

// Party is one side of a transfer.
type Party struct {
	Name           *string
	DocumentNumber *string
}

// CardMetadata is raw.creditCardMetadata, present on card rows.
type CardMetadata struct {
	CardNumber        *string
	BillID            *string
	InstallmentNumber *int
	TotalInstallments *int
	PurchaseDate      *time.Time
	// BillForecastDate is the provider's text as it comes (a YYYY-MM month on
	// the cards seen so far), not a date.
	BillForecastDate *string
	FeeType          *string
	PayeeMCC         *int
}

type partyRaw struct {
	Name           *string `json:"name"`
	DocumentNumber *struct {
		Value *string `json:"value"`
	} `json:"documentNumber"`
}

type transactionRaw struct {
	Status                  string   `json:"status"`
	OperationType           *string  `json:"operationType"`
	AmountInAccountCurrency *float64 `json:"amountInAccountCurrency"`
	DescriptionRaw          *string  `json:"descriptionRaw"`
	PaymentData             *struct {
		PaymentMethod *string   `json:"paymentMethod"`
		Payer         *partyRaw `json:"payer"`
		Receiver      *partyRaw `json:"receiver"`
	} `json:"paymentData"`
	CreditCardMetadata *struct {
		CardNumber        *string `json:"cardNumber"`
		BillID            *string `json:"billId"`
		InstallmentNumber *int    `json:"installmentNumber"`
		TotalInstallments *int    `json:"totalInstallments"`
		PurchaseDate      *string `json:"purchaseDate"`
		BillForecastDate  *string `json:"billForecastDate"`
		FeeType           *string `json:"feeType"`
		PayeeMCC          *int    `json:"payeeMCC"`
	} `json:"creditCardMetadata"`
}

// CleanTransactions returns the clean list for the days of a period, newest
// first. The period is in Brazil days and converted to UTC bounds here.
func (view *View) CleanTransactions(ctx context.Context, period brday.Period) ([]Transaction, error) {
	start, end, err := period.Bounds()
	if err != nil {
		return nil, err
	}
	var transactions []Transaction
	err = view.read(ctx, func(tx *sql.Tx) error {
		var err error
		transactions, err = queryCleanTransactionRows(ctx, tx, start, end)
		return err
	})
	return transactions, err
}

// The bounds are applied after parsing rather than in SQL: a date that does not
// parse must fail the call, where a SQL comparison would drop its row unseen.
func queryCleanTransactionRows(ctx context.Context, tx *sql.Tx, start, end time.Time) ([]Transaction, error) {
	rows, err := tx.QueryContext(ctx, queryCleanTransactions)
	if err != nil {
		return nil, fmt.Errorf("ledger: clean transactions: %w", err)
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		transaction, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		if !transaction.Date.Before(start) && transaction.Date.Before(end) {
			transactions = append(transactions, transaction)
		}
	}
	return transactions, rows.Err()
}

func scanTransaction(rows *sql.Rows) (Transaction, error) {
	var (
		transaction              Transaction
		date, description        sql.NullString
		currency, kind, category sql.NullString
		amount                   sql.NullFloat64
		raw                      string
	)
	if err := rows.Scan(&transaction.ID, &transaction.AccountID, &date, &amount, &description,
		&currency, &kind, &category, &raw); err != nil {
		return Transaction{}, fmt.Errorf("ledger: transactions: %w", err)
	}
	field := func(name string) string { return "transactions." + name + " of " + transaction.ID }

	instant, err := parseInstant(date.String, field("date"))
	if err != nil {
		return Transaction{}, err
	}
	transaction.Date, transaction.Day = instant, brday.DayOf(instant)
	transaction.Description, transaction.Currency = description.String, currency.String
	transaction.Type, transaction.Category = kind.String, stringPointer(category)

	if transaction.Amount, err = requiredCents(amount, field("amount")); err != nil {
		return Transaction{}, err
	}
	return transaction, completeTransactionFromRaw(&transaction, raw)
}

func completeTransactionFromRaw(transaction *Transaction, raw string) error {
	var body transactionRaw
	if err := decodeRaw(raw, "transactions", &body); err != nil {
		return err
	}
	transaction.Status = body.Status
	transaction.OperationType = body.OperationType
	transaction.DescriptionRaw = body.DescriptionRaw
	transaction.Payment = paymentOf(body)

	var err error
	transaction.AmountInAccountCurrency, err = centsFromJSON(body.AmountInAccountCurrency,
		"amountInAccountCurrency of "+transaction.ID)
	if err != nil {
		return err
	}
	transaction.Card, err = cardOf(transaction.ID, body)
	return err
}

func paymentOf(body transactionRaw) *PaymentData {
	if body.PaymentData == nil {
		return nil
	}
	return &PaymentData{
		Method:   body.PaymentData.PaymentMethod,
		Payer:    partyOf(body.PaymentData.Payer),
		Receiver: partyOf(body.PaymentData.Receiver),
	}
}

func partyOf(raw *partyRaw) *Party {
	if raw == nil {
		return nil
	}
	party := &Party{Name: raw.Name}
	if raw.DocumentNumber != nil {
		party.DocumentNumber = raw.DocumentNumber.Value
	}
	return party
}

func cardOf(transactionID string, body transactionRaw) (*CardMetadata, error) {
	source := body.CreditCardMetadata
	if source == nil {
		return nil, nil
	}
	card := &CardMetadata{
		CardNumber:        source.CardNumber,
		BillID:            source.BillID,
		InstallmentNumber: source.InstallmentNumber,
		TotalInstallments: source.TotalInstallments,
		BillForecastDate:  source.BillForecastDate,
		FeeType:           source.FeeType,
		PayeeMCC:          source.PayeeMCC,
	}
	purchased, err := optionalInstant(nullable(source.PurchaseDate), "purchaseDate of "+transactionID)
	card.PurchaseDate = purchased
	return card, err
}
