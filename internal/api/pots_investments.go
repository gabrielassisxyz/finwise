package api

import (
	"context"
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

const (
	statusTotalWithdrawal = "TOTAL_WITHDRAWAL"
	// rateTypeCDI is the provider's word for a rate stated as a percentage of
	// the CDI, which is the only rate this reads as one.
	rateTypeCDI = "CDI"
)

// loadInvestments reads each current investment at its latest day, net of tax,
// and counts the ones that sit beside reserve entries. Some banks back a pot
// with an investment, so one sum can appear as both; an investment is left out
// only when the ledger itself shows it is that sum, never on a guess.
func loadInvestments(ctx context.Context, snapshot *ledger.View, view *potsView, reserves []potEntry) ([]potEntry, error) {
	investments, err := snapshot.Investments(ctx)
	if err != nil {
		return nil, err
	}
	principals, err := snapshot.InvestmentPrincipals(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]potEntry, 0, len(investments))
	view.investmentDays = map[string][]ledger.InvestmentDay{}
	for _, investment := range investments {
		days, err := snapshot.InvestmentHistory(ctx, investment.ID)
		if err != nil {
			return nil, err
		}
		entry, err := investmentEntryOf(investment, days, principals, view)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
		view.investmentDays[entry.id] = days
	}
	return view.withoutBackedInvestments(entries, reserves), nil
}

func investmentEntryOf(investment ledger.Investment, days []ledger.InvestmentDay,
	principals map[string]money.Cents, view *potsView) (potEntry, error) {
	if len(days) == 0 || days[len(days)-1].AmountNet == nil || days[len(days)-1].AmountGross == nil {
		return potEntry{}, errUnreadableAmount
	}
	latest := days[len(days)-1]
	entry := potEntry{
		id: investmentPrefix + investment.ID, itemID: investment.ItemID, name: namedOrNil(&investment.Name),
		bank: view.items[investment.ItemID], bucket: bucketInvested, balance: *latest.AmountNet,
		closed: latest.Status != nil && *latest.Status == statusTotalWithdrawal, base: potrules.BaseUnknown,
		historyStart: monthOfDay(days[0].Day),
		investment:   &investmentFigures{gross: *latest.AmountGross, net: *latest.AmountNet, due: latest.DueDate},
	}
	if entry.bank == "" {
		entry.bank = neutralBankLabel
	}
	if principal, stated := principals[investment.ID]; stated {
		entry.investment.principal = &principal
	}
	if latest.Rate != nil && latest.RateType != nil && *latest.RateType == rateTypeCDI {
		entry.cdiPercent = percentPointer(fmt.Sprintf("%.2f", *latest.Rate))
	}
	return entry, nil
}

// withoutBackedInvestments counts the investments on a connection that holds
// reserve entries and drops those the ledger shows back them: a connection whose
// investments with something in them add up, to the cent, to what its reserve
// entries hold. Anything short of that is kept, and the count says it was seen.
func (view *potsView) withoutBackedInvestments(entries, reserves []potEntry) []potEntry {
	reserved := reservedByItem(reserves)
	held := map[string]money.Cents{}
	for _, entry := range entries {
		if reserved[entry.itemID] > 0 && entry.balance != 0 {
			held[entry.itemID] += entry.balance
		}
	}
	kept := make([]potEntry, 0, len(entries))
	for _, entry := range entries {
		if _, beside := reserved[entry.itemID]; !beside || entry.itemID == "" {
			kept = append(kept, entry)
			continue
		}
		view.onReserveAccounts++
		if reserved[entry.itemID] > 0 && held[entry.itemID] == reserved[entry.itemID] && entry.balance != 0 {
			view.leftOut++
			delete(view.investmentDays, entry.id)
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// reservedByItem is what the open reserve entries of each connection hold. A
// connection with an open reserve entry is in the map even when it holds nothing.
func reservedByItem(reserves []potEntry) map[string]money.Cents {
	reserved := map[string]money.Cents{}
	for _, entry := range reserves {
		if !entry.closed {
			reserved[entry.itemID] += entry.balance
		}
	}
	return reserved
}
