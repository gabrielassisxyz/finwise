package txrules

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

func purchase(id string, amount money.Cents, day int) ledger.Transaction {
	return withOperation(cardRow(id, "DEBIT", amount, day), "PAGAMENTO")
}

func namedTax(id string, amount money.Cents, day int) ledger.Transaction {
	return withOperation(cardRow(id, "DEBIT", amount, day), "OUTROS")
}

func TestNamedTaxRowAttachesToItsPurchaseAndLeavesTheList(t *testing.T) {
	result := Interpret([]ledger.Transaction{
		purchase("buy", 20000, 10),
		namedTax("tax", 700, 12),
	}, testReference())

	if len(result.Listed) != 1 || result.Listed[0].ID != "buy" {
		t.Fatalf("listed = %+v", result.Listed)
	}
	if got := result.Listed[0]; got.Amount != -20700 || got.Tax != -700 {
		t.Fatalf("amount %d tax %d, want -20700 and -700", got.Amount, got.Tax)
	}
	if len(result.Attached) != 1 || result.Attached[0] != (TaxAttachment{TaxRowID: "tax", PurchaseID: "buy", Rule: RuleNamedTax}) {
		t.Fatalf("attached = %+v", result.Attached)
	}
}

func TestNamedTaxRowStaysListedUnlessEveryConditionHolds(t *testing.T) {
	farCard := purchase("buy", 20000, 10)
	cases := []struct {
		name string
		tax  ledger.Transaction
		buy  ledger.Transaction
	}{
		{"one cent over", namedTax("tax", 701, 12), farCard},
		{"one cent under", namedTax("tax", 699, 12), farCard},
		{"four days later", namedTax("tax", 700, 14), farCard},
		{"four days earlier", namedTax("tax", 700, 6), farCard},
		{"another card account", func() ledger.Transaction {
			other := namedTax("tax", 700, 12)
			other.AccountID = otherCardID
			return other
		}(), farCard},
		{"another card of the same account", func() ledger.Transaction {
			other := namedTax("tax", 700, 12)
			other.Card = &ledger.CardMetadata{CardNumber: text("2222")}
			return other
		}(), farCard},
		{"another name", func() ledger.Transaction {
			other := namedTax("tax", 700, 12)
			other.Description = "OTHER STORE"
			return other
		}(), farCard},
		{"the purchase is itself typed OUTROS", namedTax("tax", 700, 12), namedTax("buy", 20000, 10)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := Interpret([]ledger.Transaction{c.buy, c.tax}, testReference())
			if len(result.Listed) != 2 || len(result.Attached) != 0 {
				t.Fatalf("listed %d attached %d, want 2 and 0", len(result.Listed), len(result.Attached))
			}
		})
	}
}

func TestNamedTaxRowAtTheEdgesOfTheWindowAndOfTheRounding(t *testing.T) {
	for _, c := range []struct {
		name     string
		purchase money.Cents
		tax      money.Cents
		days     int
	}{
		{"exactly three days", 20000, 700, 13},
		{"same day", 20000, 700, 10},
		{"rounds half up", 1043, 37, 10},
		{"rounds down below the half", 1042, 36, 10},
	} {
		t.Run(c.name, func(t *testing.T) {
			result := Interpret([]ledger.Transaction{purchase("buy", c.purchase, 10), namedTax("tax", c.tax, c.days)}, testReference())
			if len(result.Attached) != 1 {
				t.Fatalf("not attached: purchase %d tax %d", c.purchase, c.tax)
			}
		})
	}
}

func TestNamedTaxRowWithTwoEquallyGoodPurchasesStaysListed(t *testing.T) {
	result := Interpret([]ledger.Transaction{
		purchase("buy-1", 20000, 10), purchase("buy-2", 20000, 11), namedTax("tax", 700, 11),
	}, testReference())
	if len(result.Attached) != 0 || len(result.Listed) != 3 {
		t.Fatalf("listed %d attached %d", len(result.Listed), len(result.Attached))
	}
}

func TestNamedTaxRowOfAForeignPurchaseUsesTheValueInReais(t *testing.T) {
	foreign := purchase("buy", 1000, 10)
	foreign.Currency, foreign.AmountInAccountCurrency = "USD", cents(20000)
	result := Interpret([]ledger.Transaction{foreign, namedTax("tax", 700, 10)}, testReference())
	if len(result.Attached) != 1 || result.Listed[0].Amount != -20700 {
		t.Fatalf("attached %d amount %d", len(result.Attached), result.Listed[0].Amount)
	}
}

func explicitTax(id string, amount money.Cents, day int) ledger.Transaction {
	tax := namedTax(id, amount, day)
	tax.Description = "IOF INTERNACIONAL - SOME PLACE"
	return tax
}

func foreignPurchase(id string, day int) ledger.Transaction {
	foreign := purchase(id, 1000, day)
	foreign.Currency, foreign.AmountInAccountCurrency = "USD", cents(5500)
	return foreign
}

func TestExplicitTaxAttachesToTheOnlyForeignPurchaseOfTheDay(t *testing.T) {
	result := Interpret([]ledger.Transaction{
		foreignPurchase("abroad", 10), purchase("local", 3000, 10), explicitTax("iof", 190, 10),
	}, testReference())
	if len(result.Attached) != 1 || len(result.Listed) != 2 {
		t.Fatalf("listed %d attached %d", len(result.Listed), len(result.Attached))
	}
	abroad, _ := listedByID(result, "abroad")
	if result.Attached[0].Rule != RuleExplicitTax {
		t.Fatalf("rule = %q", result.Attached[0].Rule)
	}
	if abroad.Amount != -5690 || abroad.Tax != -190 {
		t.Fatalf("amount %d tax %d", abroad.Amount, abroad.Tax)
	}
}

func TestExplicitTaxStaysListedWithTwoForeignPurchasesThatDay(t *testing.T) {
	result := Interpret([]ledger.Transaction{
		foreignPurchase("abroad-1", 10), foreignPurchase("abroad-2", 10), explicitTax("iof", 190, 10),
	}, testReference())
	if len(result.Attached) != 0 || len(result.Listed) != 3 {
		t.Fatalf("listed %d attached %d", len(result.Listed), len(result.Attached))
	}
}

func TestExplicitTaxStaysListedOnAnotherDayOrAnotherCardOrWithoutAForeignPurchase(t *testing.T) {
	otherCard := foreignPurchase("abroad", 10)
	otherCard.AccountID = otherCardID
	for name, rows := range map[string][]ledger.Transaction{
		"another day":     {foreignPurchase("abroad", 11), explicitTax("iof", 190, 10)},
		"another card":    {otherCard, explicitTax("iof", 190, 10)},
		"only local rows": {purchase("local", 3000, 10), explicitTax("iof", 190, 10)},
		"no purchase":     {explicitTax("iof", 190, 10)},
	} {
		t.Run(name, func(t *testing.T) {
			result := Interpret(rows, testReference())
			if len(result.Attached) != 0 || len(result.Listed) != len(rows) {
				t.Fatalf("listed %d attached %d", len(result.Listed), len(result.Attached))
			}
		})
	}
}

func TestExplicitTaxAttachesToAForeignPurchaseTypedOutros(t *testing.T) {
	abroad := foreignPurchase("abroad", 10)
	abroad = withOperation(abroad, "OUTROS")
	result := Interpret([]ledger.Transaction{abroad, explicitTax("iof", 190, 10)}, testReference())
	if len(result.Attached) != 1 {
		t.Fatalf("attached %d, want 1", len(result.Attached))
	}
}

func TestAForeignPurchaseTypedOutrosIsListedAsAnOrdinaryPurchase(t *testing.T) {
	abroad := withOperation(foreignPurchase("abroad", 10), "OUTROS")
	got := interpretOne(t, abroad, testReference())
	if got.Kind != KindCard || got.Amount != -5500 || got.Movement != NotAMovement || got.Refund || got.Fee {
		t.Fatalf("got %+v", got)
	}
}

func TestTwoTaxRowsOnOnePurchaseBothAttach(t *testing.T) {
	result := Interpret([]ledger.Transaction{
		foreignPurchase("abroad", 10), explicitTax("iof-1", 100, 10), explicitTax("iof-2", 90, 10),
	}, testReference())
	got, _ := listedByID(result, "abroad")
	if len(result.Attached) != 2 || got.Tax != -190 || got.Amount != -5690 {
		t.Fatalf("attached %d tax %d amount %d", len(result.Attached), got.Tax, got.Amount)
	}
}
