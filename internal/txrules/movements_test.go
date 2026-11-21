package txrules

import "testing"

const (
	ownPersonDocument = "55566677788"
	strangerDocument  = "11122233344"
)

func TestTransferToAnOwnDocumentIsAMovementAndToAnyoneElseIsNot(t *testing.T) {
	reference := testReference(ownPersonDocument)
	cases := []struct {
		name     string
		document string
		want     MovementKind
	}{
		{"own document", ownPersonDocument, OwnTransfer},
		{"own document punctuated", "555.666.777-88", OwnTransfer},
		{"another document", strangerDocument, NotAMovement},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := interpretOne(t, pixRow("DEBIT", nil, party("Someone", c.document)), reference)
			if got.Movement != c.want || got.Kind != KindPix || (c.want == OwnTransfer) != (got.Rule == RuleOwnTransfer) {
				t.Fatalf("movement %q kind %q rule %q, want %q pix", got.Movement, got.Kind, got.Rule, c.want)
			}
		})
	}
}

func TestIncomingTransferFromAnOwnDocumentIsAMovement(t *testing.T) {
	got := interpretOne(t, pixRow("CREDIT", party("Me", ownPersonDocument), nil), testReference(ownPersonDocument))
	if got.Movement != OwnTransfer {
		t.Fatalf("movement = %q", got.Movement)
	}
}

func TestOnlyTheCounterpartCountsNotTheOwnerOnTheOtherSide(t *testing.T) {
	source := pixRow("DEBIT", party("Me", ownPersonDocument), party("Someone", strangerDocument))
	if got := interpretOne(t, source, testReference(ownPersonDocument)); got.Movement != NotAMovement {
		t.Fatalf("movement = %q: my own document as payer of a debit does not make it a transfer", got.Movement)
	}
}

func TestTEDToAnOwnDocumentIsAMovementButABoletoIsNot(t *testing.T) {
	reference := testReference(ownPersonDocument)
	ted := withPayment(withOperation(bankRow("ted", "DEBIT", -100), "TED"), "TED", nil, party("Me", ownPersonDocument))
	boleto := withPayment(withOperation(bankRow("boleto", "DEBIT", -100), "BOLETO"), "BOLETO", nil, party("Me", ownPersonDocument))
	if got := interpretOne(t, ted, reference); got.Movement != OwnTransfer {
		t.Errorf("TED movement = %q", got.Movement)
	}
	if got := interpretOne(t, boleto, reference); got.Movement != NotAMovement {
		t.Errorf("boleto movement = %q", got.Movement)
	}
}

func TestNoIdentitiesMeansNoOwnTransfers(t *testing.T) {
	got := interpretOne(t, pixRow("DEBIT", nil, party("Someone", ownPersonDocument)), testReference())
	if got.Movement != NotAMovement {
		t.Fatalf("movement = %q", got.Movement)
	}
}

func TestBankSideBillPaymentIsAMovementOnlyAsADebitStartingWithTheWords(t *testing.T) {
	cases := []struct {
		name        string
		description string
		direction   string
		want        MovementKind
	}{
		{"debit", "PAGAMENTO DE FATURA", "DEBIT", BillPayment},
		{"debit with a suffix", "Pagamento de fatura - Card", "DEBIT", BillPayment},
		{"not at the start", "REF PAGAMENTO DE FATURA", "DEBIT", NotAMovement},
		{"a credit", "PAGAMENTO DE FATURA", "CREDIT", NotAMovement},
		{"another payment", "PAGAMENTO DE BOLETO", "DEBIT", NotAMovement},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			source := withOperation(bankRow("r", c.direction, -100), "OUTROS")
			source.Description = c.description
			if got := interpretOne(t, source, testReference()); got.Movement != c.want || (c.want == BillPayment) != (got.Rule == RuleBankBillPayment) {
				t.Fatalf("movement = %q rule %q, want %q", got.Movement, got.Rule, c.want)
			}
		})
	}
}
