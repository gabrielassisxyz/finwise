package cardrules

import "github.com/gabrielassisxyz/finwise/internal/brday"

// PlacementSource says what a purchase's bill was worked out from.
type PlacementSource string

const (
	// FromBillID: the provider put the bill's id on the purchase. It is the
	// answer even when the purchase's date says otherwise.
	FromBillID PlacementSource = "billId"
	// FromDate: no id on the purchase, so the first bill closing after its day.
	FromDate PlacementSource = "date"
	// SourceUnknown: neither an id nor a closing date to compare the day with.
	SourceUnknown PlacementSource = "unknown"
)

// maxCycleDays is the longest a purchase can precede the closing of its own bill:
// a cycle is one month, and the closing date moves a few days with the weekday.
// A larger gap means a bill is missing between the purchase and the one found.
const maxCycleDays = 35

// Placement is the bill a purchase falls on. BillID is empty for a predicted bill
// that the ledger does not have yet, and Due is its nominal due date (empty when
// the purchase names a bill the ledger does not hold).
type Placement struct {
	Source    PlacementSource
	BillID    string
	Due       brday.Day
	Predicted bool
}

// PlacePurchase works out the bill of a purchase. The provider's own
// creditCardMetadata.billForecastDate is never an input: on this card it carries
// only a month and its cut does not follow the real closing.
func (card Card) PlacePurchase(purchase Purchase) Placement {
	if purchase.BillID != nil && *purchase.BillID != "" {
		return card.placementOfBillID(*purchase.BillID)
	}
	for _, candidate := range card.cycles() {
		if !candidate.known {
			return Placement{Source: SourceUnknown}
		}
		if candidate.closing <= purchase.Day {
			continue
		}
		if gap, ok := daysBetween(purchase.Day, candidate.closing); !ok || gap > maxCycleDays {
			return Placement{Source: SourceUnknown}
		}
		return Placement{
			Source: FromDate, BillID: candidate.billID, Due: candidate.due, Predicted: candidate.predicted,
		}
	}
	return Placement{Source: SourceUnknown}
}

func (card Card) placementOfBillID(billID string) Placement {
	placement := Placement{Source: FromBillID, BillID: billID}
	for _, bill := range card.Bills {
		if bill.ID == billID {
			placement.Due = bill.Due
		}
	}
	return placement
}
