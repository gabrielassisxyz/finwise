package txrules

import "github.com/gabrielassisxyz/finwise/internal/money"

const (
	// A named tax row is exactly 3.5% of its purchase, rounded to the cent half
	// up, and falls within 3 days of it (the rate and the window are measured on
	// the ledger, where every such pair lies inside them).
	namedTaxPermille = 35
	namedTaxWindow   = 3
)

// attachTaxRows moves each tax row under the one purchase it belongs to. A tax
// row with no purchase, or with more than one candidate, is not attached: a row
// left in the list is a mistake anyone can see, a row put under the wrong
// purchase is not. Matches are decided on the amounts before any tax is added.
func attachTaxRows(rows []Transaction) (listed []Transaction, attached []TaxAttachment) {
	sameCard := indexByCard(rows)
	owner := make(map[int]int)
	for index, row := range rows {
		if purchase, ok := purchaseOfTax(rows, sameCard[row.cardID], index); ok {
			owner[index] = purchase
		}
	}
	listed = make([]Transaction, 0, len(rows))
	for taxIndex, purchaseIndex := range owner {
		rows[purchaseIndex].Amount += rows[taxIndex].Amount
		rows[purchaseIndex].Tax += rows[taxIndex].Amount
	}
	for index, row := range rows {
		purchaseIndex, isTax := owner[index]
		if !isTax {
			listed = append(listed, row)
			continue
		}
		attached = append(attached, TaxAttachment{TaxRowID: row.ID, PurchaseID: rows[purchaseIndex].ID, Rule: ruleOfTax(row.role)})
	}
	return listed, attached
}

func ruleOfTax(role taxRole) Rule {
	if role == roleExplicitTax {
		return RuleExplicitTax
	}
	return RuleNamedTax
}

func indexByCard(rows []Transaction) map[string][]int {
	index := make(map[string][]int)
	for position, row := range rows {
		if row.cardID != "" {
			index[row.cardID] = append(index[row.cardID], position)
		}
	}
	return index
}

func purchaseOfTax(rows []Transaction, sameCard []int, taxIndex int) (int, bool) {
	tax := rows[taxIndex]
	var matches []int
	for _, candidate := range sameCard {
		if candidate != taxIndex && isTaxOf(tax, rows[candidate]) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		return 0, false
	}
	return matches[0], true
}

func isTaxOf(tax, purchase Transaction) bool {
	switch tax.role {
	case roleExplicitTax:
		return isForeignPurchase(purchase) && purchase.Day == tax.Day
	case roleNamedTax:
		return tax.Original == nil && isDomesticPurchase(purchase) &&
			purchase.EstablishmentKey == tax.EstablishmentKey &&
			daysApart(purchase.Day, tax.Day) <= namedTaxWindow &&
			absolute(tax.Amount) == namedTaxOf(absolute(purchase.Amount))
	}
	return false
}

// isForeignPurchase includes a purchase typed OUTROS, which is still a purchase.
func isForeignPurchase(row Transaction) bool {
	return isPurchase(row) && row.Original != nil
}

// isDomesticPurchase leaves out every row that could itself be a tax row, so a
// tax is never paired with another tax.
func isDomesticPurchase(row Transaction) bool {
	return isPurchase(row) && row.role == roleNone
}

func isPurchase(row Transaction) bool {
	return row.Kind == KindCard && row.Amount < 0 && row.Movement == NotAMovement &&
		!row.Fee && row.role != roleExplicitTax
}

func namedTaxOf(purchase money.Cents) money.Cents {
	return (purchase*namedTaxPermille + 500) / 1000
}
