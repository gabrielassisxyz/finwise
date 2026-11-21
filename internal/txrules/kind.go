package txrules

import "github.com/gabrielassisxyz/finwise/internal/ledger"

// bankSubtype is how the bank's operationType reads in the interface, and
// whether it is a fee or interest. An operationType missing here is an ordinary
// "Outros" row with no subtype, which is the safe default.
type bankSubtype struct {
	label string
	fee   bool
}

var bankSubtypes = map[string]bankSubtype{
	"TED":                            {label: "TED"},
	"TEF":                            {label: "TEF"},
	"CARTAO":                         {label: "Débito"},
	"FOLHA_PAGAMENTO":                {label: "Salário"},
	"CONVENIO_ARRECADACAO":           {label: "Convênio"},
	"PACOTE_TARIFA_SERVICOS":         {label: "Tarifa", fee: true},
	"ENCARGOS_JUROS_CHEQUE_ESPECIAL": {label: "Juros", fee: true},
	"RENDIMENTO_APLIC_FINANCEIRA":    {label: "Rendimento"},
	"RESGATE_APLIC_FINANCEIRA":       {label: "Resgate"},
}

// kindOfBankRow reads paymentMethod and operationType together, because
// operationType alone lies: a row typed OUTROS may carry paymentMethod PIX.
// paymentMethod OTHER says nothing, so it defers to operationType.
func kindOfBankRow(row ledger.Transaction) (kind Kind, subtype string, fee bool) {
	method, operation := valueOf(paymentMethodOf(row)), valueOf(row.OperationType)
	switch {
	case names(method, operation, "PIX"):
		return KindPix, "", false
	case names(method, operation, "BOLETO"):
		return KindBoleto, "", false
	}
	if method == "TED" || method == "TEF" {
		return KindOther, method, false
	}
	found := bankSubtypes[operation]
	return KindOther, found.label, found.fee
}

// names is true when the payment method says word, or when it says nothing
// useful and the operation type does.
func names(method, operation, word string) bool {
	return method == word || (method == "" || method == "OTHER") && operation == word
}

func paymentMethodOf(row ledger.Transaction) *string {
	if row.Payment == nil {
		return nil
	}
	return row.Payment.Method
}

func valueOf(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}

func isCardAccount(account *ledger.Account) bool {
	return account != nil && account.Type == ledger.AccountCredit
}
