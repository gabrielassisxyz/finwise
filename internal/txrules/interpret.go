package txrules

import (
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// Interpret turns the rows of a period into the list the app shows. Every input
// row ends in exactly one place: the list, under a purchase as its tax, or in
// the pot movements of its account. A row no rule recognises is listed as an
// ordinary transaction.
//
// A tax row is paired only with purchases among the rows given, so a period
// that cuts between a purchase and its tax leaves the tax listed. Give it the
// days around the period when that matters.
func Interpret(rows []ledger.Transaction, reference Reference) Result {
	accounts := indexAccounts(reference.Accounts)
	own := ownDocumentsOf(reference.Identities)
	result := Result{PotMovements: make(map[string][]PotMovement)}
	candidates := make([]Transaction, 0, len(rows))
	for _, row := range rows {
		account := accounts[row.AccountID]
		if movement, ok := potMovementOf(row, account); ok {
			result.PotMovements[row.AccountID] = append(result.PotMovements[row.AccountID], movement)
			result.Tally.PotMovements++
			continue
		}
		candidates = append(candidates, interpretRow(row, account, own))
	}
	result.Listed, result.Attached = attachTaxRows(candidates)
	result.Tally.Input = len(rows)
	result.Tally.Listed = len(result.Listed)
	result.Tally.AttachedAsTax = len(result.Attached)
	return result
}

func indexAccounts(accounts []ledger.Account) map[string]*ledger.Account {
	index := make(map[string]*ledger.Account, len(accounts))
	for position := range accounts {
		index[accounts[position].ID] = &accounts[position]
	}
	return index
}

func interpretRow(row ledger.Transaction, account *ledger.Account, own ownDocuments) Transaction {
	other := counterpartOf(row)
	original, unconverted := originalOf(row)
	built := Transaction{
		ID: row.ID, AccountID: row.AccountID, Day: brday.DayOf(row.Date),
		Amount: signedAmountInReais(row, account), Original: original, Unconverted: unconverted,
		Name: other.name, RawDescription: row.Description, EstablishmentKey: other.key,
		Person: other.person, Posted: row.Status != "PENDING",
	}
	if isCardAccount(account) {
		applyCardRow(&built, row)
	} else {
		applyBankRow(&built, row, other, own)
	}
	return built
}

func applyCardRow(built *Transaction, row ledger.Transaction) {
	treatment := treatCardRow(row)
	built.Kind = KindCard
	built.Movement, built.Rule, built.Refund = treatment.movement, treatment.rule, treatment.refund
	built.Fee, built.role = treatment.fee, treatment.role
	built.cardID = row.AccountID + "/" + cardNumberOf(row)
}

func applyBankRow(built *Transaction, row ledger.Transaction, other counterpart, own ownDocuments) {
	built.Kind, built.Subtype, built.Fee = kindOfBankRow(row)
	built.Movement, built.Rule = movementOfBankRow(row, built.Kind, built.Subtype, other, own)
}

func cardNumberOf(row ledger.Transaction) string {
	if row.Card == nil {
		return ""
	}
	return valueOf(row.Card.CardNumber)
}
