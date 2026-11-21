// Package txrules interprets the rows the ledger reader returns. The reader is
// faithful and interprets nothing; this package decides what a row means to the
// app: its sign, its day, its kind, who the other side is, and which rows are
// not transactions of their own. It does no SQL and no I/O.
package txrules

import (
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Kind is the contract's transaction kind (web/src/api/contract/transactions.ts).
type Kind string

const (
	KindCard   Kind = "cartao"
	KindPix    Kind = "pix"
	KindBoleto Kind = "boleto"
	KindOther  Kind = "outros"
)

// MovementKind says why a listed row is not spending or income. The empty value
// is an ordinary transaction.
type MovementKind string

const (
	NotAMovement MovementKind = ""
	// BillPayment is a card bill paid, seen from the card or from the bank account.
	BillPayment MovementKind = "bill-payment"
	// OwnTransfer is money moved between accounts of the same person.
	OwnTransfer MovementKind = "own-transfer"
)

// Rule names the rule that decided a row's treatment, so a count of rows per
// rule can be taken from the output and a person can be told why a row moved.
// A row no rule recognised has none.
type Rule string

const (
	RuleCardBillPayment      Rule = "card-bill-payment"
	RuleCardBillPaymentText  Rule = "card-bill-payment-text"
	RuleBankBillPayment      Rule = "bank-bill-payment"
	RuleOwnTransfer          Rule = "own-transfer"
	RuleEarlyPaymentDiscount Rule = "early-payment-discount"
	RuleRefundTyped          Rule = "refund-typed"
	RuleRefundKeyword        Rule = "refund-keyword"
	RuleRefundKeywordless    Rule = "refund-keywordless"
	RuleNamedTax             Rule = "named-tax"
	RuleExplicitTax          Rule = "explicit-tax"
)

// Original is a foreign purchase as the card stated it, kept for display. The
// amount is a magnitude.
type Original struct {
	Amount   money.Cents
	Currency string
}

// Transaction is one line of the list the app shows.
type Transaction struct {
	// ID is the ledger row's id; a purchase keeps its own when a tax row is
	// attached to it.
	ID        string
	AccountID string
	Day       brday.Day
	Kind      Kind
	// Subtype is the bank's own type for what is neither card, PIX nor boleto,
	// in the words the interface uses; empty when the bank gives none.
	Subtype string
	// Amount is in reais and signed: negative is money out, on every account type.
	// It includes any tax row attached to the purchase.
	Amount money.Cents
	// Tax is the part of Amount that came from attached tax rows, with Amount's sign.
	Tax      money.Cents
	Original *Original
	// Unconverted is set on a foreign row the ledger gives no value in reais
	// for: Amount is then in Original's currency and must not be summed as reais.
	Unconverted bool

	Name           string
	RawDescription string
	// EstablishmentKey groups every row of one establishment or one person. It
	// may be a document number and is never to be logged.
	EstablishmentKey string
	// Person is set when the other side is a person, by an 11-digit document.
	Person bool

	Movement MovementKind
	// Rule is set when a rule, not the default, decided Movement, Refund or Fee.
	Rule   Rule
	Refund bool
	// Fee marks fees and interest, and the credit that discounts them.
	Fee    bool
	Posted bool

	role   taxRole
	cardID string
}

// taxRole marks a card debit that may be the tax of another row. It is only a
// candidate until a purchase is found; an unpaired candidate stays listed.
type taxRole int

const (
	roleNone taxRole = iota
	// roleNamedTax is a debit typed OUTROS: a tax row carries its purchase's name.
	roleNamedTax
	// roleExplicitTax is a debit whose text says it is the international tax.
	roleExplicitTax
)

// PotMovement is a savings-pot row that left the general list.
type PotMovement struct {
	ID          string
	AccountID   string
	Day         brday.Day
	Amount      money.Cents
	Description string
	// Yield is set on a Mercado Pago yield row, unset on a transfer in or out.
	Yield bool
}

// Tally places every input row in exactly one of three destinations.
type Tally struct {
	Input         int
	Listed        int
	AttachedAsTax int
	PotMovements  int
}

// Balanced reports whether the three destinations account for every input row.
func (tally Tally) Balanced() bool {
	return tally.Listed+tally.AttachedAsTax+tally.PotMovements == tally.Input
}

// TaxAttachment records which purchase absorbed a tax row.
type TaxAttachment struct {
	TaxRowID   string
	PurchaseID string
	Rule       Rule
}

// Result is what interpreting a period gives back.
type Result struct {
	Listed []Transaction
	// Attached lists the tax rows that left the list, with the purchase each went under.
	Attached []TaxAttachment
	// PotMovements are keyed by the account they belong to.
	PotMovements map[string][]PotMovement
	Tally        Tally
}

// Reference is what a row needs from the rest of the ledger.
type Reference struct {
	Accounts   []ledger.Account
	Identities []ledger.Identity
}
