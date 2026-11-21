package txrules

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

const (
	bankAccountID = "acc-bank"
	cardAccountID = "acc-card"
	otherCardID   = "acc-card-2"
)

func testReference(documents ...string) Reference {
	reference := Reference{Accounts: []ledger.Account{
		{ID: bankAccountID, Type: ledger.AccountBank},
		{ID: cardAccountID, Type: ledger.AccountCredit},
		{ID: otherCardID, Type: ledger.AccountCredit},
	}}
	for index, document := range documents {
		reference.Identities = append(reference.Identities, ledger.Identity{ID: "identity-" + string(rune('a'+index)), Document: document})
	}
	return reference
}

func at(t *testing.T, text string) time.Time {
	t.Helper()
	instant, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatal(err)
	}
	return instant
}

func text(value string) *string { return &value }

func cents(value money.Cents) *money.Cents { return &value }

// row builds a ledger row at noon UTC on a March 2026 day. Callers adjust fields.
func row(id, accountID, kind string, amount money.Cents, day int) ledger.Transaction {
	date := time.Date(2026, time.March, day, 15, 0, 0, 0, time.UTC)
	return ledger.Transaction{
		ID: id, AccountID: accountID, Date: date, Day: brday.DayOf(date),
		Description: "ACME STORE", Amount: amount, Currency: "BRL", Type: kind, Status: "POSTED",
	}
}

func bankRow(id, kind string, amount money.Cents) ledger.Transaction {
	return row(id, bankAccountID, kind, amount, 10)
}

func cardRow(id, kind string, amount money.Cents, day int) ledger.Transaction {
	built := row(id, cardAccountID, kind, amount, day)
	built.Card = &ledger.CardMetadata{CardNumber: text("1111")}
	return built
}

func withOperation(source ledger.Transaction, operation string) ledger.Transaction {
	source.OperationType = &operation
	return source
}

func withPayment(source ledger.Transaction, method string, payer, receiver *ledger.Party) ledger.Transaction {
	source.Payment = &ledger.PaymentData{Method: &method, Payer: payer, Receiver: receiver}
	return source
}

func party(name, document string) *ledger.Party {
	return &ledger.Party{Name: text(name), DocumentNumber: text(document)}
}

func interpretOne(t *testing.T, source ledger.Transaction, reference Reference) Transaction {
	t.Helper()
	result := Interpret([]ledger.Transaction{source}, reference)
	if len(result.Listed) != 1 {
		t.Fatalf("row %s was not listed alone: %+v", source.ID, result.Tally)
	}
	return result.Listed[0]
}

func listedByID(result Result, id string) (Transaction, bool) {
	for _, listed := range result.Listed {
		if listed.ID == id {
			return listed, true
		}
	}
	return Transaction{}, false
}
