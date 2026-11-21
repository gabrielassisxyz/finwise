package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

type panelSections struct {
	Establishment bool `json:"establishment"`
	Person        bool `json:"person"`
	Split         bool `json:"split"`
	Reimbursement bool `json:"reimbursement"`
	Refund        bool `json:"refund"`
}

type counterpartyDetail struct {
	Key         string   `json:"key"`
	Nickname    *string  `json:"nickname"`
	DefaultLine *string  `json:"defaultLine"`
	FixedTags   []string `json:"fixedTags"`
}

// The shapes below are the contract's, for the parts of the panel a person
// fills. They are always empty for now and exist so the types say what null and
// [] stand for.
type splitPart struct {
	Line   *string `json:"line"`
	Amount Reais   `json:"amount"`
}

type reimbursementDetail struct {
	Expected *Reais `json:"expected"`
	Link     *struct {
		TransactionID string `json:"transactionId"`
		Name          string `json:"name"`
		Day           string `json:"day"`
		Amount        Reais  `json:"amount"`
	} `json:"link"`
}

type reimbursementCandidate struct {
	TransactionID string `json:"transactionId"`
	Name          string `json:"name"`
	Day           string `json:"day"`
	Amount        Reais  `json:"amount"`
}

type exclusionDetail struct {
	Reason string `json:"reason"`
}

type historyEntry struct {
	ID       string  `json:"id"`
	Text     string  `json:"text"`
	Day      string  `json:"day"`
	Reason   *string `json:"reason"`
	Undoable bool    `json:"undoable"`
}

type transactionDetailWire struct {
	transactionWire
	AccountDescription      string                   `json:"accountDescription"`
	Justification           *string                  `json:"justification"`
	AskReason               bool                     `json:"askReason"`
	Reason                  string                   `json:"reason"`
	Sections                panelSections            `json:"sections"`
	Counterparty            *counterpartyDetail      `json:"counterparty"`
	OwnTags                 []string                 `json:"ownTags"`
	SuggestedTags           []string                 `json:"suggestedTags"`
	Note                    string                   `json:"note"`
	SplitParts              []splitPart              `json:"splitParts"`
	SplitRemaining          Reais                    `json:"splitRemaining"`
	Reimbursement           *reimbursementDetail     `json:"reimbursement"`
	ReimbursementCandidates []reimbursementCandidate `json:"reimbursementCandidates"`
	Exclusion               *exclusionDetail         `json:"exclusion"`
	RefundLine              *string                  `json:"refundLine"`
	History                 []historyEntry           `json:"history"`
}

// readTransactionDetail answers null for an id the list does not show: an
// unknown one, a tax row that sits under its purchase, a pot movement.
//
// The row is interpreted in the same month window the list uses, so the panel
// can never disagree with the line it was opened from.
func readTransactionDetail(ctx context.Context, args []json.RawMessage, reader *ledger.Reader) (any, error) {
	id, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	all, err := reader.CleanTransactions(ctx, wholeLedger)
	if err != nil {
		return nil, err
	}
	source, found := rowWithID(all, id)
	if !found {
		return nil, nil
	}
	around, err := loadContext(ctx, reader)
	if err != nil {
		return nil, err
	}
	view, err := interpretMonthOf(all, source, around)
	if err != nil {
		return nil, err
	}
	for _, listed := range view.listed {
		if listed.ID == id {
			return detailFrom(listed, source, around), nil
		}
	}
	return nil, nil
}

func rowWithID(rows []ledger.Transaction, id string) (ledger.Transaction, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return ledger.Transaction{}, false
}

func interpretMonthOf(all []ledger.Transaction, source ledger.Transaction, around ledgerContext) (interpreted, error) {
	month, err := brday.MonthPeriod(brday.MonthOf(source.Date))
	if err != nil {
		return interpreted{}, err
	}
	wide, err := widened(month, taxPairingDays)
	if err != nil {
		return interpreted{}, err
	}
	return interpretRows(rowsAround(all, wide), around, month)
}

func detailFrom(listed txrules.Transaction, source ledger.Transaction, around ledgerContext) transactionDetailWire {
	sections := sectionsOf(listed)
	return transactionDetailWire{
		transactionWire:         transactionFrom(listed, source, around),
		AccountDescription:      describeAccount(listed.AccountID, source, around),
		Sections:                sections,
		Counterparty:            counterpartyOf(listed, sections),
		OwnTags:                 []string{},
		SuggestedTags:           []string{},
		ReimbursementCandidates: []reimbursementCandidate{},
		History:                 []historyEntry{},
	}
}

// sectionsOf says which parts of the panel apply to a kind of transaction.
func sectionsOf(listed txrules.Transaction) panelSections {
	spending := listed.Amount < 0 && listed.Movement == txrules.NotAMovement
	isMovement := listed.Movement != txrules.NotAMovement
	return panelSections{
		Establishment: !listed.Person && !isMovement,
		Person:        listed.Person && !isMovement,
		Split:         spending,
		Reimbursement: spending,
		Refund:        listed.Amount > 0,
	}
}

func counterpartyOf(listed txrules.Transaction, sections panelSections) *counterpartyDetail {
	if !sections.Establishment && !sections.Person {
		return nil
	}
	return &counterpartyDetail{Key: listed.EstablishmentKey, FixedTags: []string{}}
}

// describeAccount names the account in the panel's subtitle; a card carries the
// last digits of the card the row was paid with.
func describeAccount(accountID string, source ledger.Transaction, around ledgerContext) string {
	for _, account := range around.accounts {
		if account.ID != accountID {
			continue
		}
		if account.Type != ledger.AccountCredit {
			return bankAccountDescription(around.banks[account.ID], nameOfAccount(account))
		}
		if digits := lastDigitsOf(source); digits != "" {
			return cardAccountDescription(nameOfAccount(account), digits)
		}
		return nameOfAccount(account)
	}
	return around.labels[accountID]
}

// lastDigitsOf is the final four digits of the card number of a row, or nothing
// when the row names no card or fewer than four digits.
func lastDigitsOf(source ledger.Transaction) string {
	if source.Card == nil || source.Card.CardNumber == nil {
		return ""
	}
	var digits []rune
	for _, r := range *source.Card.CardNumber {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) < 4 {
		return ""
	}
	return string(digits[len(digits)-4:])
}
