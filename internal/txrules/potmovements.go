package txrules

import (
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

const (
	operationPotTransfer = "TRANSFERENCIA_SALDO_RESERVADO"
	operationPotRedeem   = "RESGATE_APLIC_FINANCEIRA"
	// A redeem row whose text starts with this word is a pot's yield paid out;
	// any other redeem is an ordinary row of the account.
	potYieldWord = "RENDIMENTOS"
)

// potMovementOf recognises the rows that belong to a savings pot and leave the
// general list.
func potMovementOf(row ledger.Transaction, account *ledger.Account) (PotMovement, bool) {
	operation := valueOf(row.OperationType)
	isYield := operation == operationPotRedeem && hasPrefixWord(foldText(row.Description), potYieldWord)
	if operation != operationPotTransfer && !isYield {
		return PotMovement{}, false
	}
	return PotMovement{
		ID: row.ID, AccountID: row.AccountID, Day: brday.DayOf(row.Date),
		Amount: signedAmountInReais(row, account), Description: row.Description, Yield: isYield,
	}, true
}
