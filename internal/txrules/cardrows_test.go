package txrules

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

func creditRow(description, operation string) ledger.Transaction {
	source := cardRow("credit", "CREDIT", -4000, 10)
	source.Description = description
	if operation != "" {
		source = withOperation(source, operation)
	}
	return source
}

func TestCardCreditsAreSortedByTheirOwnTextAndDirection(t *testing.T) {
	cases := []struct {
		name         string
		description  string
		operation    string
		wantMovement MovementKind
		wantRefund   bool
		wantFee      bool
	}{
		{"bill payment typed PAGAMENTO_FATURA", "Anything", "PAGAMENTO_FATURA", BillPayment, false, false},
		{"bill paid from balance", "PAGAMENTO COM SALDO", "OUTROS", BillPayment, false, false},
		{"bill paid from balance in other case", "Pagamento  com saldo", "OUTROS", BillPayment, false, false},
		{"early payment discount", "Desc Antecipa Parcelas", "OUTROS", NotAMovement, false, true},
		{"typed ESTORNO", "Anything", "ESTORNO", NotAMovement, true, false},
		{"partial cancellation", "CANCELAMENTO PARCIAL", "OUTROS", NotAMovement, true, false},
		{"refund keyword with no type", "Estorno de compra", "", NotAMovement, true, false},
		{"keyword-less credit typed OUTROS is a refund", "ACME STORE", "OUTROS", NotAMovement, true, false},
		{"payment named in a credit with no type is a bill payment", "Prefix Pagamento recebido", "", BillPayment, false, false},
		{"credit with no type and no keyword stays ordinary", "ACME STORE", "", NotAMovement, false, false},
		{"a word that only contains the keyword", "PAGAMENTOS ACME", "", NotAMovement, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := interpretOne(t, creditRow(c.description, c.operation), testReference())
			if got.Movement != c.wantMovement || got.Refund != c.wantRefund || got.Fee != c.wantFee {
				t.Fatalf("movement %q refund %t fee %t; want %q %t %t", got.Movement, got.Refund, got.Fee,
					c.wantMovement, c.wantRefund, c.wantFee)
			}
			if got.Amount != 4000 {
				t.Fatalf("a credit must be positive, got %d", got.Amount)
			}
		})
	}
}

func TestCardDebitsWithTheSameWordsAreNotSortedAsCredits(t *testing.T) {
	for _, description := range []string{"Desc Antecipa Parcelas", "Estorno", "PAGAMENTO COM SALDO X"} {
		source := cardRow("debit", "DEBIT", 4000, 10)
		source.Description = description
		got := interpretOne(t, withOperation(source, "PAGAMENTO"), testReference())
		if got.Refund || got.Fee {
			t.Errorf("%q: a debit is neither a refund nor a fee credit: %+v", description, got)
		}
	}
}

func TestOrdinaryCardPurchaseIsListedUntouched(t *testing.T) {
	got := interpretOne(t, purchase("buy", 4500, 10), testReference())
	if got.Movement != NotAMovement || got.Refund || got.Fee || got.Tax != 0 || got.Amount != -4500 {
		t.Fatalf("got %+v", got)
	}
}

func TestEachCardRuleNamesItselfOnTheRowItDecided(t *testing.T) {
	cases := []struct {
		description string
		operation   string
		want        Rule
	}{
		{"Anything", "PAGAMENTO_FATURA", RuleCardBillPayment},
		{"PAGAMENTO COM SALDO", "OUTROS", RuleCardBillPayment},
		{"Desc Antecipa Parcelas", "OUTROS", RuleEarlyPaymentDiscount},
		{"Anything", "ESTORNO", RuleRefundTyped},
		{"CANCELAMENTO PARCIAL", "OUTROS", RuleRefundKeyword},
		{"ACME STORE", "OUTROS", RuleRefundKeywordless},
		{"Prefix Pagamento recebido", "", RuleCardBillPaymentText},
		{"ACME STORE", "", ""},
	}
	for _, c := range cases {
		if got := interpretOne(t, creditRow(c.description, c.operation), testReference()); got.Rule != c.want {
			t.Errorf("%q typed %q: rule %q, want %q", c.description, c.operation, got.Rule, c.want)
		}
	}
}
