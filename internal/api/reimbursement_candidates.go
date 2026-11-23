package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// A reimbursement ties a purchase to a credit that may have arrived in any
// month, before the app's database existed included, so the credits are read
// from the whole ledger in one pass, as the card reads its purchases.

// listedCredits is every credit the whole ledger lists now, newest first, with
// the key the index gave each one it has met.
type listedCredits struct {
	rows []txrules.Transaction
	keys map[string]authored.TxKey
}

func readListedCredits(ctx context.Context, views Views) (listedCredits, error) {
	whole, _, err := listedLedger(ctx, views.Ledger)
	if err != nil {
		return listedCredits{}, err
	}
	credits := listedCredits{}
	ids := []string{}
	for _, listed := range whole.listed {
		if listed.Amount > 0 {
			credits.rows = append(credits.rows, listed)
			ids = append(ids, listed.ID)
		}
	}
	if credits.keys, err = views.Authored.TransactionKeys(ctx, ids); err != nil {
		return listedCredits{}, err
	}
	return credits, nil
}

// linkedCreditRows is the listed row of each credit a purchase among kept
// links. A credit the ledger no longer lists has no row: its link is kept and
// applies again once the credit is listed under its key.
func linkedCreditRows(ctx context.Context, views Views, kept map[authored.TxKey]authored.AuthoredTransaction) (map[authored.TxKey]txrules.Transaction, error) {
	wanted := map[authored.TxKey]bool{}
	for _, transaction := range kept {
		if credit := transaction.State.Credit; credit != nil {
			wanted[*credit] = true
		}
	}
	found := map[authored.TxKey]txrules.Transaction{}
	if len(wanted) == 0 {
		return found, nil
	}
	credits, err := readListedCredits(ctx, views)
	if err != nil {
		return nil, err
	}
	for _, credit := range credits.rows {
		key, indexed := credits.keys[credit.ID]
		if _, taken := found[key]; indexed && wanted[key] && !taken {
			found[key] = credit
		}
	}
	return found, nil
}

// candidateCredits are the received PIX that may pay a purchase back, newest
// first, with what is kept about the establishments and people behind them.
type candidateCredits struct {
	rows           []txrules.Transaction
	establishments map[string]authored.Establishment
}

// readCandidates finds the candidates of the purchase at key; indexed is false
// for a purchase the index has never met, which no credit can be linked to.
func readCandidates(ctx context.Context, views Views, purchase authored.TxKey, indexed bool) (candidateCredits, error) {
	credits, err := readListedCredits(ctx, views)
	if err != nil {
		return candidateCredits{}, err
	}
	ids := make([]string, 0, len(credits.rows))
	for _, credit := range credits.rows {
		ids = append(ids, credit.ID)
	}
	kept, err := views.Authored.Transactions(ctx, keysInOrder(ids, credits.keys))
	if err != nil {
		return candidateCredits{}, err
	}
	links, err := views.Authored.CreditLinks(ctx)
	if err != nil {
		return candidateCredits{}, err
	}
	candidates := candidateCredits{rows: []txrules.Transaction{}}
	for _, credit := range credits.rows {
		key, creditIndexed := credits.keys[credit.ID]
		var state authored.TransactionState
		linkedElsewhere := false
		if creditIndexed {
			owner, linked := links[key]
			state, linkedElsewhere = kept[key].State, linked && !(indexed && owner == purchase)
		}
		if isCandidate(credit, state, linkedElsewhere) {
			candidates.rows = append(candidates.rows, credit)
		}
	}
	if candidates.establishments, err = establishmentsOf(ctx, views.Authored, candidates.rows); err != nil {
		return candidateCredits{}, err
	}
	return candidates, nil
}

// isCandidate is a received PIX that counts as someone's money coming in: not
// a movement between own accounts, in reais, not left out of the totals with a
// reason, not a refund by the person's word or, failing it, the ledger's, and
// not the reimbursement of another purchase. One switched off without a
// reason still counts, and one on an income line may be a reimbursement first
// filed as income.
func isCandidate(credit txrules.Transaction, state authored.TransactionState, linkedElsewhere bool) bool {
	switch {
	case credit.Amount <= 0, credit.Kind != txrules.KindPix, credit.Movement != txrules.NotAMovement, credit.Unconverted:
		return false
	case exclusionOf(state.Exclusion) == budgetrules.ExcludedWithReason, isRefundNow(credit, state), linkedElsewhere:
		return false
	}
	return true
}

func isRefundNow(credit txrules.Transaction, state authored.TransactionState) bool {
	if state.Refund != nil {
		return *state.Refund
	}
	return credit.Refund
}

// withID is the candidate listed under id.
func (candidates candidateCredits) withID(id string) (txrules.Transaction, bool) {
	for _, credit := range candidates.rows {
		if credit.ID == id {
			return credit, true
		}
	}
	return txrules.Transaction{}, false
}

func (candidates candidateCredits) nameOf(credit txrules.Transaction) string {
	return shownName(candidates.establishments, credit.EstablishmentKey, credit.Name)
}

func (candidates candidateCredits) wire() []reimbursementCandidate {
	wire := make([]reimbursementCandidate, 0, len(candidates.rows))
	for _, credit := range candidates.rows {
		wire = append(wire, reimbursementCandidate{
			TransactionID: credit.ID, Name: candidates.nameOf(credit), Day: string(credit.Day), Amount: Reais(credit.Amount),
		})
	}
	return wire
}
