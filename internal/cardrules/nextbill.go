package cardrules

import (
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// slipDays is how many days before a closing a purchase can still land on the
// bill after it: a purchase made right before closing sometimes slips.
const slipDays = 2

// NextBillAmount is what the month's spending will charge on the bill after the
// one that closes inside the month. Known is false when that closing date is
// unknown, and Total is then not a figure to show, least of all as zero.
type NextBillAmount struct {
	Known    bool
	Closing  brday.Day
	Total    money.Cents
	Included []Purchase
}

// AmountOnNextBill adds up the purchases of the month that the bill after the
// month's closing will charge: those dated on or after the closing date, and
// those of the two days before it that did not receive the closed bill's id.
//
// Two choices the sources leave open. Only purchases of the month count, since
// the figure answers "what I spent this month will be charged next month", so a
// slip day that falls in the previous month is not added. And a purchase that
// carries the closed bill's id is left out whatever its date, as the id says
// which bill charges it.
//
// Before the closing day has come nothing has been cut off yet, so the amount is
// known and empty; the days just before it are not slipped purchases until the
// bill has closed.
func (card Card) AmountOnNextBill(month brday.Month, today brday.Day, purchases []Purchase) NextBillAmount {
	closing, billID, ok := card.closingWithin(month)
	if !ok {
		return NextBillAmount{}
	}
	result := NextBillAmount{Known: true, Closing: closing}
	if today < closing {
		return result
	}
	slipFrom, ok := addDays(closing, -slipDays)
	if !ok {
		return NextBillAmount{}
	}
	for _, purchase := range purchases {
		if !strings.HasPrefix(string(purchase.Day), string(month)+"-") || purchase.Day < slipFrom {
			continue
		}
		if billID != "" && purchase.BillID != nil && *purchase.BillID == billID {
			continue
		}
		result.Included = append(result.Included, purchase)
		result.Total += purchase.Amount
	}
	return result
}

// closingWithin is the closing date that falls in the month, and the id of the
// ledger bill it belongs to when the ledger already has it.
func (card Card) closingWithin(month brday.Month) (closing brday.Day, billID string, ok bool) {
	for _, candidate := range card.cycles() {
		if candidate.known && strings.HasPrefix(string(candidate.closing), string(month)+"-") {
			return candidate.closing, candidate.billID, true
		}
	}
	return "", "", false
}
