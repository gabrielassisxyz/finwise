package txrules

import "github.com/gabrielassisxyz/finwise/internal/ledger"

const (
	personDocumentDigits = 11
	keyOfDocument        = "doc:"
	keyOfName            = "name:"
	keyOfText            = "text:"
)

// counterpart is the other side of a row. The ledger has no merchant on any
// row, so it comes from the parties of a transfer or, failing that, the text.
type counterpart struct {
	name     string
	key      string
	person   bool
	document string
}

// counterpartOf takes the receiver of a debit and the payer of a credit. A
// document is the best key, a party's name the next, the description the last:
// two descriptions that differ only in case, spacing or a trailing instalment
// marker share a key, and so does a document however it is punctuated.
func counterpartOf(row ledger.Transaction) counterpart {
	other := otherParty(row)
	if other != nil {
		if digits := digitsOf(valueOf(other.DocumentNumber)); digits != "" {
			return counterpart{
				name: nameOrDescription(other, row), key: keyOfDocument + digits,
				person: len(digits) == personDocumentDigits, document: digits,
			}
		}
		if name := collapsedName(other); name != "" {
			return counterpart{name: name, key: keyOfName + establishmentText(name)}
		}
	}
	return counterpart{name: displayName(row.Description), key: keyOfText + establishmentText(row.Description)}
}

func otherParty(row ledger.Transaction) *ledger.Party {
	if row.Payment == nil {
		return nil
	}
	if row.Type == "DEBIT" {
		return row.Payment.Receiver
	}
	return row.Payment.Payer
}

func nameOrDescription(party *ledger.Party, row ledger.Transaction) string {
	if name := collapsedName(party); name != "" {
		return name
	}
	return displayName(row.Description)
}

func collapsedName(party *ledger.Party) string {
	return collapseSpaces(valueOf(party.Name))
}
