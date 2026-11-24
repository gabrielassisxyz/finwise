package api

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/authored"
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
// fills.
type splitPart struct {
	Line   *string `json:"line"`
	Amount Reais   `json:"amount"`
}

type reimbursementDetail struct {
	Expected *Reais             `json:"expected"`
	Link     *reimbursementLink `json:"link"`
}

type reimbursementLink struct {
	TransactionID string `json:"transactionId"`
	Name          string `json:"name"`
	Day           string `json:"day"`
	Amount        Reais  `json:"amount"`
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
func readTransactionDetail(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	id, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	row, found, err := findListed(ctx, views.Ledger, id)
	if err != nil || !found {
		return nil, err
	}
	rows, err := readAuthoredRows(ctx, views, []txrules.Transaction{row.listed})
	if err != nil {
		return nil, err
	}
	extras, err := readPanelExtras(ctx, views, rows, row.listed)
	if err != nil {
		return nil, err
	}
	return detailFrom(row, rows, extras), nil
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

// panelExtras is what the panel reads beyond the list: the current month's
// budget, which names an establishment's default line, the tags in use, the
// history with what of it can still be taken back, and the credits that may
// pay a purchase back.
type panelExtras struct {
	current    transactionLines
	known      []string
	history    []historyEntry
	candidates []reimbursementCandidate
}

func readPanelExtras(ctx context.Context, views Views, rows authoredRows, listed txrules.Transaction) (panelExtras, error) {
	view := views.Authored
	current, err := view.BudgetOf(ctx, brday.MonthOf(nowOf(ctx)))
	if err != nil {
		return panelExtras{}, err
	}
	known, err := knownTags(ctx, view)
	if err != nil {
		return panelExtras{}, err
	}
	extras := panelExtras{current: transactionLinesOf(current), known: known, history: []historyEntry{}, candidates: []reimbursementCandidate{}}
	kept, key, indexed := rows.keptOf(listed)
	if indexed {
		if extras.history, err = historyOf(ctx, view, key, kept.History); err != nil {
			return panelExtras{}, err
		}
	}
	if sectionsOf(listed).Reimbursement {
		candidates, err := readCandidates(ctx, views, key, indexed)
		if err != nil {
			return panelExtras{}, err
		}
		extras.candidates = candidates.wire()
	}
	return extras, nil
}

// historyOf is a transaction's steps newest first. A step can be taken back
// while nothing changed since in what its change did to this transaction; a
// note only records that something happened, and never can.
func historyOf(ctx context.Context, view *authored.View, key authored.TxKey, steps []authored.HistoryStep) ([]historyEntry, error) {
	ids := make([]authored.StepID, 0, len(steps))
	for _, step := range steps {
		ids = append(ids, step.ID)
	}
	units := authored.TransactionUnits(key, ids)
	history := make([]historyEntry, 0, len(steps))
	for _, step := range slices.Backward(steps) {
		undoable := false
		if step.Kind == authored.StepEffect {
			var err error
			if undoable, err = view.CanRestorePart(ctx, step.Change, units); err != nil {
				return nil, err
			}
		}
		history = append(history, historyEntry{
			ID: step.ID.String(), Text: step.Text, Day: string(step.Day), Reason: textOrNil(step.Reason), Undoable: undoable,
		})
	}
	return history, nil
}

func detailFrom(row listedRow, rows authoredRows, extras panelExtras) transactionDetailWire {
	listed := row.listed
	kept, _, _ := rows.keptOf(listed)
	state := kept.State
	month := rows.months[monthOfDay(listed.Day)]
	shown := month.pendingSuggestion(state, rows.suggestionsOf(listed))
	sections := sectionsOf(listed)
	return transactionDetailWire{
		transactionWire:         rows.transactionOf(listed, row.source, row.around),
		AccountDescription:      describeAccount(listed.AccountID, row.source, row.around),
		Justification:           justificationOf(state, shown),
		AskReason:               state.AskReason,
		Reason:                  latestReason(kept.History),
		Sections:                sections,
		Counterparty:            counterpartyOf(listed, sections, rows.establishments[listed.EstablishmentKey], extras.current),
		OwnTags:                 tagsOf(kept.Tags, nil),
		SuggestedTags:           suggestedTags(extras.known, kept.Tags),
		Note:                    state.Note,
		SplitParts:              splitPartsOf(kept.Split, month),
		SplitRemaining:          splitRemainingOf(listed, kept.Split),
		Reimbursement:           rows.reimbursementOf(state),
		ReimbursementCandidates: extras.candidates,
		Exclusion:               exclusionDetailOf(state.Exclusion),
		RefundLine:              month.nameOf(state.RefundLine),
		History:                 extras.history,
	}
}

// justificationOf is the model's reason for a line, which the panel offers to
// confirm while it is not null. A pending review whose shown suggestion carries
// a justification answers it; otherwise a pending review or a line of the
// model's answers an empty one.
func justificationOf(state authored.TransactionState, shown shownSuggestion) *string {
	if shown.justification != nil {
		text := *shown.justification
		return &text
	}
	if state.PendingReason == "" && state.ClassifiedBy != authored.AuthorModel {
		return nil
	}
	empty := ""
	return &empty
}

// latestReason is the reason on the newest step that has one.
func latestReason(steps []authored.HistoryStep) string {
	for _, step := range slices.Backward(steps) {
		if step.Reason != "" {
			return step.Reason
		}
	}
	return ""
}

func suggestedTags(known []string, own []authored.Tag) []string {
	suggested := []string{}
	for _, tag := range known {
		if !slices.ContainsFunc(own, func(held authored.Tag) bool { return held.Name == tag }) {
			suggested = append(suggested, tag)
		}
	}
	return suggested
}

// splitPartsOf names each part's line in the transaction's own month. One part
// is not a split, as Dispose counts it.
func splitPartsOf(parts []authored.SplitPart, month transactionLines) []splitPart {
	if len(parts) < 2 {
		return nil
	}
	wire := make([]splitPart, 0, len(parts))
	for _, part := range parts {
		wire = append(wire, splitPart{Line: month.nameOf(part.Line), Amount: Reais(part.Cents)})
	}
	return wire
}

// splitRemainingOf is what the parts still lack of what was paid, negative
// when they exceed it.
func splitRemainingOf(listed txrules.Transaction, parts []authored.SplitPart) Reais {
	if len(parts) < 2 {
		return 0
	}
	remaining := listed.Amount
	if remaining < 0 {
		remaining = -remaining
	}
	for _, part := range parts {
		remaining -= part.Cents
	}
	return Reais(remaining)
}

// reimbursementOf answers the link only while it applies, with the amount it
// was linked with, which may be more than the purchase: the panel says what
// arrived, and the effective amount what of it was taken off.
func (rows authoredRows) reimbursementOf(state authored.TransactionState) *reimbursementDetail {
	if !state.Reimbursable {
		return nil
	}
	detail := &reimbursementDetail{}
	if state.ExpectedCents != nil {
		expected := Reais(*state.ExpectedCents)
		detail.Expected = &expected
	}
	if credit, applied := rows.appliedCredit(state); applied {
		detail.Link = &reimbursementLink{
			TransactionID: credit.ID, Name: shownName(rows.establishments, credit.EstablishmentKey, credit.Name),
			Day: string(credit.Day), Amount: Reais(*state.CreditCents),
		}
	}
	return detail
}

func exclusionDetailOf(exclusion *string) *exclusionDetail {
	if exclusion == nil {
		return nil
	}
	return &exclusionDetail{Reason: *exclusion}
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

// counterpartyOf names the default line as the current month's budget does: an
// establishment's default has no month of its own, and is applied from now on.
func counterpartyOf(listed txrules.Transaction, sections panelSections, kept authored.Establishment, current transactionLines) *counterpartyDetail {
	if !sections.Establishment && !sections.Person {
		return nil
	}
	return &counterpartyDetail{
		Key: listed.EstablishmentKey, Nickname: textOrNil(kept.Nickname),
		DefaultLine: current.nameOf(kept.DefaultLine), FixedTags: tagsOf(kept.FixedTags, nil),
	}
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
