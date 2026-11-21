package txrules

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

func TestDirectionComesFromTypeWhateverSignWasStored(t *testing.T) {
	cases := []struct {
		name      string
		account   string
		direction string
		stored    money.Cents
		want      money.Cents
	}{
		{"bank debit stored negative", bankAccountID, "DEBIT", -5000, -5000},
		{"bank debit stored positive", bankAccountID, "DEBIT", 5000, -5000},
		{"bank credit stored positive", bankAccountID, "CREDIT", 5000, 5000},
		{"bank credit stored negative", bankAccountID, "CREDIT", -5000, 5000},
		{"card debit stored positive", cardAccountID, "DEBIT", 5000, -5000},
		{"card debit stored negative", cardAccountID, "DEBIT", -5000, -5000},
		{"card credit stored negative", cardAccountID, "CREDIT", -5000, 5000},
		{"card credit stored positive", cardAccountID, "CREDIT", 5000, 5000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := interpretOne(t, row("r", c.account, c.direction, c.stored, 10), testReference())
			if got.Amount != c.want {
				t.Fatalf("amount = %d, want %d", got.Amount, c.want)
			}
		})
	}
}

func TestUnknownTypeFallsBackToTheStoredSignOfTheAccountType(t *testing.T) {
	bank := interpretOne(t, row("b", bankAccountID, "", -700, 10), testReference())
	card := interpretOne(t, row("c", cardAccountID, "", 700, 10), testReference())
	if bank.Amount != -700 || card.Amount != -700 {
		t.Fatalf("bank %d, card %d; both want -700 (money out)", bank.Amount, card.Amount)
	}
}

func TestForeignPurchaseIsInReaisAndKeepsItsOriginal(t *testing.T) {
	foreign := cardRow("usd", "DEBIT", 1000, 10)
	foreign.Currency = "USD"
	foreign.AmountInAccountCurrency = cents(5500)

	got := interpretOne(t, foreign, testReference())

	if got.Amount != -5500 {
		t.Fatalf("amount = %d, want -5500", got.Amount)
	}
	if got.Original == nil || got.Original.Amount != 1000 || got.Original.Currency != "USD" || got.Unconverted {
		t.Fatalf("original = %+v unconverted = %t", got.Original, got.Unconverted)
	}
}

func TestForeignRowWithoutAValueInReaisIsFlaggedNotPassedOffAsReais(t *testing.T) {
	foreign := cardRow("usd", "DEBIT", 1000, 10)
	foreign.Currency = "USD"

	got := interpretOne(t, foreign, testReference())

	if !got.Unconverted || got.Amount != -1000 {
		t.Fatalf("amount = %d unconverted = %t", got.Amount, got.Unconverted)
	}
}

func TestRowInReaisHasNoOriginal(t *testing.T) {
	if got := interpretOne(t, cardRow("brl", "DEBIT", 1000, 10), testReference()); got.Original != nil || got.Unconverted {
		t.Fatalf("original = %+v unconverted = %t", got.Original, got.Unconverted)
	}
}

func TestDayIsTheBrazilDayNotTheUTCDay(t *testing.T) {
	edge := bankRow("edge", "DEBIT", -100)
	edge.Date = at(t, "2026-03-01T02:30:00Z")
	edge.Day = "2026-03-01"

	if got := interpretOne(t, edge, testReference()); got.Day != "2026-02-28" {
		t.Fatalf("day = %s, want 2026-02-28", got.Day)
	}
}

func TestPendingRowIsNotPosted(t *testing.T) {
	pending := bankRow("p", "DEBIT", -100)
	pending.Status = "PENDING"
	if interpretOne(t, pending, testReference()).Posted {
		t.Fatal("a PENDING row must not be posted")
	}
	if !interpretOne(t, bankRow("q", "DEBIT", -100), testReference()).Posted {
		t.Fatal("a POSTED row must be posted")
	}
}
