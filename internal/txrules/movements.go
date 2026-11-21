package txrules

import "github.com/gabrielassisxyz/finwise/internal/ledger"

// bankBillPayment is how a card bill paid from a bank account reads there. The
// sources wrote no rule for this side; the smallest safe one is a debit whose
// text starts with these words, which on the ledger pairs one-to-one with the
// payments the card side records.
const bankBillPayment = "PAGAMENTO DE FATURA"

// ownDocuments is the set of documents of the person's own identities, as
// digits only, so punctuation never decides a match.
type ownDocuments map[string]bool

func ownDocumentsOf(identities []ledger.Identity) ownDocuments {
	own := make(ownDocuments, len(identities))
	for _, identity := range identities {
		if digits := digitsOf(identity.Document); digits != "" {
			own[digits] = true
		}
	}
	return own
}

// movementOfBankRow recognises a bill payment from the bank side and a transfer
// between the person's own accounts. A boleto is never an own transfer: paying
// a boleto issued in one's own name is still a payment.
func movementOfBankRow(row ledger.Transaction, kind Kind, subtype string, other counterpart, own ownDocuments) (MovementKind, Rule) {
	if row.Type == "DEBIT" && hasPrefixWord(foldText(row.Description), bankBillPayment) {
		return BillPayment, RuleBankBillPayment
	}
	if movesBetweenAccounts(kind, subtype) && other.document != "" && own[other.document] {
		return OwnTransfer, RuleOwnTransfer
	}
	return NotAMovement, ""
}

func movesBetweenAccounts(kind Kind, subtype string) bool {
	return kind == KindPix || subtype == "TED" || subtype == "TEF"
}
