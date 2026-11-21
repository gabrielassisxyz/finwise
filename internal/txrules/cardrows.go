package txrules

import (
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// operationType cannot be the rule across banks: on one card OUTROS is almost
// only tax, on another it is everything the bank does not classify. Each card
// row is sorted by its own text and direction instead, and a row no rule
// recognises stays an ordinary purchase or credit.
const (
	explicitTaxKeyword   = "IOF INTERNACIONAL"
	billPaidFromBalance  = "PAGAMENTO COM SALDO"
	earlyPaymentDiscount = "DESC ANTECIPA PARCELAS"
	billPaymentWord      = "PAGAMENTO"
)

// Stems of the words a refund carries. The partial cancellation on one card has
// no keyword in the sources; "CANCEL" is the smallest stem its row is known by.
var refundStems = []string{"ESTORN", "CANCEL", "DEVOLU"}

type cardTreatment struct {
	movement MovementKind
	rule     Rule
	refund   bool
	fee      bool
	role     taxRole
}

func treatCardRow(row ledger.Transaction) cardTreatment {
	operation, folded := valueOf(row.OperationType), foldText(row.Description)
	if operation == "PAGAMENTO_FATURA" || hasPrefixWord(folded, billPaidFromBalance) {
		return cardTreatment{movement: BillPayment, rule: RuleCardBillPayment}
	}
	if row.Type == "CREDIT" {
		return treatCardCredit(operation, folded)
	}
	return treatCardDebit(operation, folded)
}

func treatCardCredit(operation, folded string) cardTreatment {
	switch {
	case operation == "ESTORNO":
		return cardTreatment{refund: true, rule: RuleRefundTyped}
	case hasAnyStem(folded, refundStems):
		return cardTreatment{refund: true, rule: RuleRefundKeyword}
	case hasPrefixWord(folded, earlyPaymentDiscount):
		return cardTreatment{fee: true, rule: RuleEarlyPaymentDiscount}
	// Open rule: a credit with no operationType whose text names a payment is a
	// bill payment seen from the card. Without it such a credit would pass as
	// income.
	case operation == "" && containsWord(folded, billPaymentWord):
		return cardTreatment{movement: BillPayment, rule: RuleCardBillPaymentText}
	// The user confirmed that a credit with no keyword typed OUTROS is a refund.
	case operation == "OUTROS":
		return cardTreatment{refund: true, rule: RuleRefundKeywordless}
	}
	return cardTreatment{}
}

func treatCardDebit(operation, folded string) cardTreatment {
	switch {
	case hasPrefixWord(folded, explicitTaxKeyword):
		return cardTreatment{role: roleExplicitTax}
	case operation == "TARIFA":
		return cardTreatment{fee: true}
	case operation == "OUTROS":
		return cardTreatment{role: roleNamedTax}
	}
	return cardTreatment{}
}

func hasAnyStem(folded string, stems []string) bool {
	for _, word := range strings.Fields(folded) {
		for _, stem := range stems {
			if strings.HasPrefix(word, stem) {
				return true
			}
		}
	}
	return false
}
