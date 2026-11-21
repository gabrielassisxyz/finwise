package txrules

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

func pixRow(direction string, payer, receiver *ledger.Party) ledger.Transaction {
	source := withOperation(bankRow("pix", direction, -100), "PIX")
	source.Description = "Transferência"
	return withPayment(source, "PIX", payer, receiver)
}

func TestPixOutNamesItsReceiverAndIsKeyedByTheirDocument(t *testing.T) {
	cases := []struct {
		name       string
		document   string
		wantPerson bool
	}{
		{"eleven digits is a person", "11122233344", true},
		{"fourteen digits is a company", "11222333000144", false},
		{"punctuated person", "111.222.333-44", true},
		{"punctuated company", "11.222.333/0001-44", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			source := pixRow("DEBIT", party("Payer Name", "99988877766"), party("Receiver Name", c.document))
			got := interpretOne(t, source, testReference())
			if got.Name != "Receiver Name" || got.Person != c.wantPerson {
				t.Fatalf("name %q person %t", got.Name, got.Person)
			}
			if got.EstablishmentKey != "doc:"+digitsOf(c.document) {
				t.Fatalf("key = %q", got.EstablishmentKey)
			}
		})
	}
}

func TestPunctuationDoesNotSplitOneDocumentIntoTwoKeys(t *testing.T) {
	plain := interpretOne(t, pixRow("DEBIT", nil, party("A", "11122233344")), testReference())
	dotted := interpretOne(t, pixRow("DEBIT", nil, party("A", "111.222.333-44")), testReference())
	if plain.EstablishmentKey != dotted.EstablishmentKey {
		t.Fatalf("%q != %q", plain.EstablishmentKey, dotted.EstablishmentKey)
	}
}

func TestPixInIsKeyedByThePayer(t *testing.T) {
	got := interpretOne(t, pixRow("CREDIT", party("Sender", "11122233344"), party("Me", "99988877766")), testReference())
	if got.Name != "Sender" || got.EstablishmentKey != "doc:11122233344" {
		t.Fatalf("name %q key %q", got.Name, got.EstablishmentKey)
	}
}

func TestCounterpartWithoutADocumentIsKeyedByName(t *testing.T) {
	source := pixRow("DEBIT", nil, &ledger.Party{Name: text("  Some   Name ")})
	got := interpretOne(t, source, testReference())
	if got.Name != "Some Name" || got.EstablishmentKey != "name:SOME NAME" || got.Person {
		t.Fatalf("got %q %q %t", got.Name, got.EstablishmentKey, got.Person)
	}
}

func TestCounterpartWithNothingFallsBackToTheDescription(t *testing.T) {
	source := pixRow("DEBIT", nil, nil)
	source.Description = "Transferência enviada"
	got := interpretOne(t, source, testReference())
	if got.Name != "Transferência enviada" || got.EstablishmentKey != "text:TRANSFERENCIA ENVIADA" || got.Person {
		t.Fatalf("got %q %q %t", got.Name, got.EstablishmentKey, got.Person)
	}
}

func TestCardPurchaseIsKeyedByItsNormalisedDescription(t *testing.T) {
	keys := map[string]bool{}
	for index, description := range []string{"ACME STORE 03/10", "acme  store", "ACME STORE"} {
		source := cardRow(string(rune('a'+index)), "DEBIT", 100, 10)
		source.Description = description
		keys[interpretOne(t, source, testReference()).EstablishmentKey] = true
	}
	if len(keys) != 1 {
		t.Fatalf("the three descriptions made %d keys, want 1", len(keys))
	}
}

func TestCardPurchaseNameDropsTheInstalmentMarkerButKeepsCase(t *testing.T) {
	source := cardRow("a", "DEBIT", 100, 10)
	source.Description = "Acme  Store 03/10"
	got := interpretOne(t, source, testReference())
	if got.Name != "Acme Store" || got.RawDescription != "Acme  Store 03/10" {
		t.Fatalf("name %q raw %q", got.Name, got.RawDescription)
	}
}
