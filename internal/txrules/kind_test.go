package txrules

import "testing"

func TestBankKindReadsPaymentMethodAndOperationTypeTogether(t *testing.T) {
	cases := []struct {
		name        string
		operation   string
		method      string
		wantKind    Kind
		wantSubtype string
		wantFee     bool
	}{
		{"typed PIX", "PIX", "PIX", KindPix, "", false},
		{"typed OUTROS but paid by PIX", "OUTROS", "PIX", KindPix, "", false},
		{"typed BOLETO", "BOLETO", "BOLETO", KindBoleto, "", false},
		{"typed PIX with method OTHER defers to the operation", "PIX", "OTHER", KindPix, "", false},
		{"TED", "TED", "TED", KindOther, "TED", false},
		{"TED under another operation name", "TRANSFERENCIA_NOVA", "TED", KindOther, "TED", false},
		{"TEF under another operation name", "TRANSFERENCIA_MESMA_INSTITUICAO", "TEF", KindOther, "TEF", false},
		{"debit card", "CARTAO", "OTHER", KindOther, "Débito", false},
		{"payroll", "FOLHA_PAGAMENTO", "OTHER", KindOther, "Salário", false},
		{"collection agreement", "CONVENIO_ARRECADACAO", "OTHER", KindOther, "Convênio", false},
		{"service package", "PACOTE_TARIFA_SERVICOS", "OTHER", KindOther, "Tarifa", true},
		{"overdraft interest", "ENCARGOS_JUROS_CHEQUE_ESPECIAL", "OTHER", KindOther, "Juros", true},
		{"yield", "RENDIMENTO_APLIC_FINANCEIRA", "OTHER", KindOther, "Rendimento", false},
		{"redeem", "RESGATE_APLIC_FINANCEIRA", "OTHER", KindOther, "Resgate", false},
		{"typed OUTROS and OTHER", "OUTROS", "OTHER", KindOther, "", false},
		{"an operation no rule knows", "SOMETHING_NEW", "OTHER", KindOther, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			source := withPayment(withOperation(bankRow("r", "DEBIT", -100), c.operation), c.method, nil, nil)
			got := interpretOne(t, source, testReference())
			if got.Kind != c.wantKind || got.Subtype != c.wantSubtype || got.Fee != c.wantFee {
				t.Fatalf("kind %q subtype %q fee %t; want %q %q %t", got.Kind, got.Subtype, got.Fee, c.wantKind, c.wantSubtype, c.wantFee)
			}
		})
	}
}

func TestBankRowWithNothingToGoOnIsOutros(t *testing.T) {
	got := interpretOne(t, bankRow("bare", "DEBIT", -100), testReference())
	if got.Kind != KindOther || got.Subtype != "" || got.Movement != NotAMovement {
		t.Fatalf("got %+v", got)
	}
}

func TestEveryRowOfACardAccountIsACardRow(t *testing.T) {
	for _, operation := range []string{"PAGAMENTO", "OUTROS", "TARIFA", "ESTORNO", "PAGAMENTO_FATURA", "UNKNOWN"} {
		source := withOperation(cardRow("c", "DEBIT", 100, 10), operation)
		if got := interpretOne(t, source, testReference()); got.Kind != KindCard {
			t.Errorf("operation %s: kind %q, want card", operation, got.Kind)
		}
	}
}

func TestCardFeeIsMarkedAsFee(t *testing.T) {
	got := interpretOne(t, withOperation(cardRow("fee", "DEBIT", 1990, 10), "TARIFA"), testReference())
	if !got.Fee || got.Amount != -1990 {
		t.Fatalf("got %+v", got)
	}
}
