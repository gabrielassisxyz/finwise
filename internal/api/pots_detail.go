package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

const (
	contributionClosed      = "closed"
	contributionNonePlanned = "none-planned"
	contributionPending     = "pending"
	contributionDone        = "done"
)

// potContributionWire is the contract's PotContribution, a union whose only
// member with a day is done: the others have no doneOn at all.
type potContributionWire struct {
	State  string  `json:"state"`
	DoneOn *string `json:"doneOn,omitempty"`
}

type potInvestmentWire struct {
	Applied      Reais   `json:"applied"`
	Gross        Reais   `json:"gross"`
	Tax          Reais   `json:"tax"`
	Net          Reais   `json:"net"`
	RedeemableOn *string `json:"redeemableOn"`
}

// potLinkedLineWire is a budget line the pot feeds. Its share of the balance is
// null: nothing splits a pot's balance between the lines it feeds yet.
type potLinkedLineWire struct {
	Name  string `json:"name"`
	Share *Reais `json:"share"`
}

type potYieldDayWire struct {
	Day    string `json:"day"`
	Amount *Reais `json:"amount"`
}

type potYieldWire struct {
	Month string            `json:"month"`
	Total *Reais            `json:"total"`
	Days  []potYieldDayWire `json:"days"`
}

// potDetailWire is the contract's SavingsPotDetail. A target is the person's to
// author and nothing stores one yet, so it is the contract's empty value.
type potDetailWire struct {
	ID           string              `json:"id"`
	Name         *string             `json:"name"`
	Bank         string              `json:"bank"`
	Bucket       *string             `json:"bucket"`
	CDIPercent   *Percent            `json:"cdiPercent"`
	Balance      Reais               `json:"balance"`
	BelowTarget  any                 `json:"belowTarget"`
	Contribution potContributionWire `json:"contribution"`
	LinkedLines  []potLinkedLineWire `json:"linkedLines"`
	Investment   *potInvestmentWire  `json:"investment"`
	Movements    []potMovementWire   `json:"movements"`
	Yield        potYieldWire        `json:"yield"`
	HistoryStart string              `json:"historyStart"`
}

// readPotDetail answers null for an id that is neither a pot nor a bank account
// with movements that no pot could claim.
func readPotDetail(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	id, err := stringArgument(args)
	if err != nil {
		return nil, err
	}
	view, err := loadPotsWithChoices(ctx, views, true)
	if err != nil {
		return nil, err
	}
	for _, entry := range view.entries {
		if entry.id != id {
			continue
		}
		current, err := view.currentTransfers(ctx, views.Authored)
		if err != nil {
			return nil, err
		}
		return view.detailOf(entry, current.transfers), nil
	}
	return view.accountSideDetail(id), nil
}

func (view *potsView) detailOf(entry potEntry, transfers []budgetrules.MarkedTransfer) potDetailWire {
	detail := potDetailWire{
		ID: entry.id, Name: entry.name, Bank: entry.bank, Bucket: textPointer(entry.bucket),
		CDIPercent: entry.cdiPercent, Balance: Reais(entry.balance), Contribution: contributionOf(entry, transfers),
		LinkedLines: linkedLinesOf(entry), Movements: movementsOrEmpty(entry.movements),
		Yield:        potYieldWire{Month: string(view.month), Total: reaisPointer(monthlyYieldOf(entry)), Days: yieldDaysOf(entry)},
		HistoryStart: string(entry.historyStart),
	}
	if figures := entry.investment; figures != nil && figures.principal != nil {
		detail.Investment = &potInvestmentWire{
			Applied: Reais(*figures.principal), Gross: Reais(figures.gross), Tax: Reais(figures.gross - figures.net),
			Net: Reais(figures.net),
		}
		if figures.due != nil {
			detail.Investment.RedeemableOn = textPointer(string(*figures.due))
		}
	}
	return detail
}

// yieldDaysOf lists the rows behind the month's yield, oldest first. A day the
// ledger cannot support is listed with a null amount, never a zero, so the
// page can say which day broke the month; the month's total is then null too.
func yieldDaysOf(entry potEntry) []potYieldDayWire {
	days := []potYieldDayWire{}
	if !entry.yieldAsked {
		return days
	}
	for _, row := range entry.yield.Days {
		days = append(days, potYieldDayWire{Day: string(row.To), Amount: reaisPointer(row.Yield)})
	}
	return days
}

func linkedLinesOf(entry potEntry) []potLinkedLineWire {
	lines := make([]potLinkedLineWire, 0, len(entry.linkedLines))
	for _, name := range entry.linkedLines {
		lines = append(lines, potLinkedLineWire{Name: name})
	}
	return lines
}

// contributionOf follows the current month's transfers, whatever month the rest
// of the app is looking at: the pot's own transfer, else, for an investment, the
// one every investment shares. A mark is the person's word that the transfer
// was made, so it adds no movement to the page.
func contributionOf(entry potEntry, transfers []budgetrules.MarkedTransfer) potContributionWire {
	if entry.closed {
		return potContributionWire{State: contributionClosed}
	}
	transfer, planned := budgetrules.TransferOfPot(transfers, entry.id, entry.investment != nil)
	switch {
	case !planned:
		return potContributionWire{State: contributionNonePlanned}
	case transfer.Done():
		return potContributionWire{State: contributionDone, DoneOn: textPointer(string(transfer.DoneOn))}
	}
	return potContributionWire{State: contributionPending}
}

func movementsOrEmpty(movements []potMovementWire) []potMovementWire {
	if movements == nil {
		return []potMovementWire{}
	}
	return movements
}

// accountSideDetail shows what moved between a bank account and its pots that no
// pot could claim. It is reached by the account's id, since the list has no row
// for an account, and it is null for an account with nothing unclaimed.
func (view *potsView) accountSideDetail(accountID string) any {
	movements := view.unattributed[accountID]
	if len(movements) == 0 {
		return nil
	}
	for _, account := range view.accounts {
		if account.ID != accountID || account.Type != ledger.AccountBank {
			continue
		}
		name := nameOfAccount(account)
		return potDetailWire{
			ID: account.ID, Name: &name, Bank: view.banks[account.ID], Bucket: textPointer(bucketAvailable),
			Balance: Reais(account.Balance), Contribution: potContributionWire{State: contributionNonePlanned},
			LinkedLines: []potLinkedLineWire{}, Movements: movements,
			Yield:        potYieldWire{Month: string(view.month), Days: []potYieldDayWire{}},
			HistoryStart: string(monthOfDay(firstOf(view.sightings[account.ID]))),
		}
	}
	return nil
}
