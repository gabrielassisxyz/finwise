package txrules

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// destinationsOf counts how many times each input id appears across the three
// destinations, so a row dropped or doubled shows as a count other than one.
func destinationsOf(result Result) map[string]int {
	seen := make(map[string]int)
	for _, listed := range result.Listed {
		seen[listed.ID]++
	}
	for _, attached := range result.Attached {
		seen[attached.TaxRowID]++
	}
	for _, movements := range result.PotMovements {
		for _, movement := range movements {
			seen[movement.ID]++
		}
	}
	return seen
}

func requireEveryRowInExactlyOnePlace(t *testing.T, rows []ledger.Transaction, result Result) {
	t.Helper()
	seen := destinationsOf(result)
	for _, source := range rows {
		if seen[source.ID] != 1 {
			t.Fatalf("row %s is in %d destinations, want exactly 1", source.ID, seen[source.ID])
		}
	}
	if len(seen) != len(rows) {
		t.Fatalf("%d ids in the output for %d input rows", len(seen), len(rows))
	}
	tally := result.Tally
	if !tally.Balanced() || tally.Input != len(rows) || tally.Listed != len(result.Listed) ||
		tally.AttachedAsTax != len(result.Attached) {
		t.Fatalf("tally %+v does not describe %d rows, %d listed, %d attached", tally, len(rows),
			len(result.Listed), len(result.Attached))
	}
}

func TestEveryFixtureRowEndsInExactlyOneDestination(t *testing.T) {
	rows := []ledger.Transaction{
		purchase("buy", 20000, 10), namedTax("tax", 700, 11),
		foreignPurchase("abroad", 12), explicitTax("iof", 190, 12),
		creditRow("Anything", "PAGAMENTO_FATURA"), creditRow("ACME", "OUTROS"),
		bankRow("plain", "DEBIT", -100),
		potRow("pot", "TRANSFERENCIA_SALDO_RESERVADO", "Pot", "DEBIT", -10),
		pixRow("DEBIT", nil, party("Someone", "11122233344")),
	}
	for index := range rows {
		rows[index].ID = fmt.Sprintf("row-%d", index)
	}
	requireEveryRowInExactlyOnePlace(t, rows, Interpret(rows, testReference("11122233344")))
}

func TestEmptyInputGivesAnEmptyBalancedTally(t *testing.T) {
	result := Interpret(nil, testReference())
	if result.Tally != (Tally{}) || !result.Tally.Balanced() {
		t.Fatalf("tally = %+v", result.Tally)
	}
}

// The pools are small on purpose: a random row then often shares a card, a
// name, a day or an amount with another, which is what makes tax pairing
// ambiguous and is where a row would be dropped or counted twice.
var (
	randomAccounts   = []string{bankAccountID, cardAccountID, otherCardID, "acc-unknown"}
	randomOperations = []string{"", "PIX", "OUTROS", "PAGAMENTO", "PAGAMENTO_FATURA", "ESTORNO", "TARIFA",
		"TRANSFERENCIA_SALDO_RESERVADO", "RESGATE_APLIC_FINANCEIRA", "TED", "BOLETO", "NEW_THING"}
	randomDescriptions = []string{"ACME STORE", "acme  store 3/10", "IOF INTERNACIONAL - X", "PAGAMENTO COM SALDO",
		"Rendimentos", "Desc Antecipa Parcelas", "PAGAMENTO DE FATURA", "CANCELAMENTO PARCIAL", ""}
	randomAmounts = []money.Cents{-700, 700, 20000, -20000, 1000, 35, 0}
)

func randomRow(random *rand.Rand, id string) ledger.Transaction {
	source := row(id, randomAccounts[random.Intn(len(randomAccounts))],
		[]string{"DEBIT", "CREDIT", ""}[random.Intn(3)], randomAmounts[random.Intn(len(randomAmounts))], 8+random.Intn(8))
	source.Description = randomDescriptions[random.Intn(len(randomDescriptions))]
	source = withOperation(source, randomOperations[random.Intn(len(randomOperations))])
	if random.Intn(3) == 0 {
		source.Currency, source.AmountInAccountCurrency = "USD", cents(randomAmounts[random.Intn(len(randomAmounts))])
	}
	if random.Intn(2) == 0 {
		source.Card = &ledger.CardMetadata{CardNumber: text([]string{"1111", "2222"}[random.Intn(2)])}
	}
	if random.Intn(3) == 0 {
		source = withPayment(source, "PIX", party("A", "11122233344"), party("B", "11222333000144"))
	}
	return source
}

func TestTallyInvariantHoldsOverRandomInputs(t *testing.T) {
	random := rand.New(rand.NewSource(20261121))
	reference := testReference("11122233344")
	for round := 0; round < 3000; round++ {
		rows := make([]ledger.Transaction, random.Intn(40))
		for index := range rows {
			rows[index] = randomRow(random, fmt.Sprintf("r-%d", index))
		}
		requireEveryRowInExactlyOnePlace(t, rows, Interpret(rows, reference))
	}
}

func TestRandomInputsAttachSomeTaxRowsSoTheInvariantIsNotVacuous(t *testing.T) {
	random := rand.New(rand.NewSource(20261121))
	attached, pots := 0, 0
	for round := 0; round < 3000; round++ {
		rows := make([]ledger.Transaction, random.Intn(40))
		for index := range rows {
			rows[index] = randomRow(random, fmt.Sprintf("r-%d", index))
		}
		result := Interpret(rows, testReference())
		attached += len(result.Attached)
		pots += result.Tally.PotMovements
	}
	if attached == 0 || pots == 0 {
		t.Fatalf("generator never produced an attachment (%d) or a pot movement (%d)", attached, pots)
	}
}
