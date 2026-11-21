package txrules

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

func potRow(id, operation, description string, direction string, amount money.Cents) ledger.Transaction {
	source := withOperation(bankRow(id, direction, 0), operation)
	source.Description = description
	source.Amount = amount
	return source
}

func TestPotRowsLeaveTheListAndAreReturnedPerAccount(t *testing.T) {
	otherBank := potRow("other-account", "TRANSFERENCIA_SALDO_RESERVADO", "Pot", "DEBIT", -300)
	otherBank.AccountID = "acc-bank-2"
	result := Interpret([]ledger.Transaction{
		potRow("into-pot", "TRANSFERENCIA_SALDO_RESERVADO", "Dinheiro reservado", "DEBIT", -1500),
		potRow("yield", "RESGATE_APLIC_FINANCEIRA", "Rendimentos do mês", "CREDIT", 42),
		otherBank,
		bankRow("plain", "DEBIT", -100),
	}, testReference())

	if len(result.Listed) != 1 || result.Listed[0].ID != "plain" {
		t.Fatalf("listed = %+v", result.Listed)
	}
	mine := result.PotMovements[bankAccountID]
	if len(mine) != 2 || mine[0].ID != "into-pot" || mine[1].ID != "yield" {
		t.Fatalf("pot movements of the account = %+v", mine)
	}
	if mine[0].Amount != -1500 || mine[0].Yield || mine[1].Amount != 42 || !mine[1].Yield {
		t.Fatalf("amounts or yield flags wrong: %+v", mine)
	}
	if got := result.PotMovements["acc-bank-2"]; len(got) != 1 || got[0].ID != "other-account" {
		t.Fatalf("pot movements of the other account = %+v", got)
	}
	if result.Tally.PotMovements != 3 || result.Tally.Listed != 1 {
		t.Fatalf("tally = %+v", result.Tally)
	}
}

func TestARedeemThatIsNotAYieldStaysListed(t *testing.T) {
	for _, description := range []string{"Resgate de aplicação", "Pix recebido Rendimentos", ""} {
		source := potRow("redeem", "RESGATE_APLIC_FINANCEIRA", description, "CREDIT", 5000)
		got := interpretOne(t, source, testReference())
		if got.Subtype != "Resgate" {
			t.Errorf("%q: subtype %q", description, got.Subtype)
		}
	}
}

func TestYieldWordOnAnotherOperationDoesNotMakeAPotMovement(t *testing.T) {
	source := potRow("other", "OUTROS", "Rendimentos", "CREDIT", 5000)
	if got := Interpret([]ledger.Transaction{source}, testReference()); got.Tally.PotMovements != 0 {
		t.Fatalf("tally = %+v", got.Tally)
	}
}
