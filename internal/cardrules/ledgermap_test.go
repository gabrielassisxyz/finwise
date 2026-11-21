package cardrules_test

import (
	"github.com/gabrielassisxyz/finwise/internal/cardrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// The tests feed the rules from the real reader, so that what they pin is what
// the app will do with the ledger and not with a hand-built shape.

func cardBillsOf(bills []ledger.Bill) []cardrules.Bill {
	converted := make([]cardrules.Bill, 0, len(bills))
	for _, bill := range bills {
		converted = append(converted, cardrules.Bill{ID: bill.ID, Due: bill.DueDate, Closing: bill.ClosingDate})
	}
	return converted
}

func purchaseOf(transaction ledger.Transaction) cardrules.Purchase {
	purchase := cardrules.Purchase{ID: transaction.ID, Day: transaction.Day, Amount: transaction.Amount}
	if transaction.Card != nil {
		purchase.BillID = transaction.Card.BillID
	}
	return purchase
}
