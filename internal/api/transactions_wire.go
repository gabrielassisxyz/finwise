package api

import (
	"strconv"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// transactionWire is the contract's Transaction. What only the person can author
// (a budget line, who classified it, tags, a nickname, an exclusion, a split, a
// reimbursement) comes from the app's database, and is the contract's empty
// value where nothing is kept, never a guess.
type transactionWire struct {
	ID                  string         `json:"id"`
	Day                 string         `json:"day"`
	Name                string         `json:"name"`
	OriginalName        string         `json:"originalName"`
	RawDescription      string         `json:"rawDescription"`
	EstablishmentKey    string         `json:"establishmentKey"`
	Kind                string         `json:"kind"`
	Subtype             *string        `json:"subtype"`
	Account             string         `json:"account"`
	AccountLabel        string         `json:"accountLabel"`
	Amount              Reais          `json:"amount"`
	EffectiveAmount     Reais          `json:"effectiveAmount"`
	CountsInTotals      bool           `json:"countsInTotals"`
	Line                *string        `json:"line"`
	ClassifiedBy        *string        `json:"classifiedBy"`
	Confidence          *int           `json:"confidence"`
	BelowThreshold      bool           `json:"belowThreshold"`
	Pending             *pendingReview `json:"pending"`
	Posted              bool           `json:"posted"`
	Tags                []string       `json:"tags"`
	Movement            bool           `json:"movement"`
	Person              bool           `json:"person"`
	Refund              bool           `json:"refund"`
	ReimbursementIncome bool           `json:"reimbursementIncome"`
	Reimbursed          bool           `json:"reimbursed"`
	Excluded            bool           `json:"excluded"`
	Split               bool           `json:"split"`
	Installment         *string        `json:"installment"`
	Bill                *string        `json:"bill"`
	OriginalAmount      *string        `json:"originalAmount"`
	Iof                 *Reais         `json:"iof"`
}

// pendingReview is the contract's shape for a classification waiting for a
// decision. A review the app has no line to propose for carries neither a
// suggestion nor a confidence, and no justification exists until a model
// writes one.
type pendingReview struct {
	Reason        string  `json:"reason"`
	Suggestion    *string `json:"suggestion"`
	Confidence    *int    `json:"confidence"`
	Justification *string `json:"justification"`
}

// transactionFrom carries one interpreted row to the wire as the ledger alone
// has it; authoredRows.transactionOf merges in what was authored. source is
// the ledger row it came from, which holds what the rules do not keep: the
// instalment and the bill.
func transactionFrom(listed txrules.Transaction, source ledger.Transaction, around ledgerContext) transactionWire {
	return transactionWire{
		ID: listed.ID, Day: string(listed.Day), Name: listed.Name, OriginalName: listed.Name,
		RawDescription: listed.RawDescription, EstablishmentKey: listed.EstablishmentKey,
		Kind: string(listed.Kind), Subtype: textOrNil(listed.Subtype),
		Account: listed.AccountID, AccountLabel: around.labels[listed.AccountID],
		Amount: Reais(listed.Amount), EffectiveAmount: Reais(listed.Amount),
		CountsInTotals: listed.Movement == txrules.NotAMovement && !listed.Unconverted,
		Posted:         listed.Posted, Tags: []string{},
		Movement: listed.Movement != txrules.NotAMovement, Person: listed.Person, Refund: listed.Refund,
		Installment: installmentOf(source), Bill: billOf(source, around.billLabels),
		OriginalAmount: originalAmountOf(listed.Original), Iof: taxOf(listed.Tax),
	}
}

func textOrNil(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

func installmentOf(source ledger.Transaction) *string {
	card := source.Card
	if card == nil || card.InstallmentNumber == nil || card.TotalInstallments == nil {
		return nil
	}
	text := strconv.Itoa(*card.InstallmentNumber) + "/" + strconv.Itoa(*card.TotalInstallments)
	return &text
}

// billOf is the label of the bill the provider put on the purchase, when the
// ledger holds that bill. A bill it does not hold yet is not worked out here.
func billOf(source ledger.Transaction, labels map[string]string) *string {
	if source.Card == nil || source.Card.BillID == nil {
		return nil
	}
	label, known := labels[*source.Card.BillID]
	if !known {
		return nil
	}
	return &label
}

// taxOf is the tax as a positive figure, the way a person reads "IOF".
func taxOf(tax money.Cents) *Reais {
	if tax == 0 {
		return nil
	}
	if tax < 0 {
		tax = -tax
	}
	amount := Reais(tax)
	return &amount
}

// originalAmountOf writes a foreign amount the way it is read in Brazil: the
// currency, then the figure with a comma for the decimals.
func originalAmountOf(original *txrules.Original) *string {
	if original == nil {
		return nil
	}
	symbol, known := foreignSymbols[original.Currency]
	if !known {
		symbol = original.Currency
	}
	text := symbol + " " + brazilianDecimal(original.Amount)
	return &text
}

func brazilianDecimal(amount money.Cents) string {
	whole := strconv.FormatInt(int64(amount)/100, 10)
	var grouped strings.Builder
	for index, digit := range whole {
		if index > 0 && (len(whole)-index)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(digit)
	}
	fraction := strconv.FormatInt(int64(amount)%100, 10)
	if len(fraction) < 2 {
		fraction = "0" + fraction
	}
	return grouped.String() + "," + fraction
}
