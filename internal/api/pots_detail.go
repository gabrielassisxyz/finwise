package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

const (
	contributionClosed      = "closed"
	contributionNonePlanned = "none-planned"
)

type potContributionWire struct {
	State string `json:"state"`
}

type potInvestmentWire struct {
	Applied      Reais   `json:"applied"`
	Gross        Reais   `json:"gross"`
	Tax          Reais   `json:"tax"`
	Net          Reais   `json:"net"`
	RedeemableOn *string `json:"redeemableOn"`
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

// potDetailWire is the contract's SavingsPotDetail. A target, a planned
// contribution and the budget lines a pot feeds are the person's to author, so
// they are the contract's empty value.
type potDetailWire struct {
	ID           string              `json:"id"`
	Name         *string             `json:"name"`
	Bank         string              `json:"bank"`
	Bucket       *string             `json:"bucket"`
	CDIPercent   *Percent            `json:"cdiPercent"`
	Balance      Reais               `json:"balance"`
	BelowTarget  any                 `json:"belowTarget"`
	Contribution potContributionWire `json:"contribution"`
	LinkedLines  []any               `json:"linkedLines"`
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
	view, err := loadPots(ctx, views.Ledger, true)
	if err != nil {
		return nil, err
	}
	for _, entry := range view.entries {
		if entry.id == id {
			return view.detailOf(entry), nil
		}
	}
	return view.accountSideDetail(id), nil
}

func (view *potsView) detailOf(entry potEntry) potDetailWire {
	detail := potDetailWire{
		ID: entry.id, Name: entry.name, Bank: entry.bank, Bucket: textPointer(entry.bucket),
		CDIPercent: entry.cdiPercent, Balance: Reais(entry.balance), Contribution: contributionOf(entry),
		LinkedLines: []any{}, Movements: movementsOrEmpty(entry.movements),
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

func contributionOf(entry potEntry) potContributionWire {
	if entry.closed {
		return potContributionWire{State: contributionClosed}
	}
	return potContributionWire{State: contributionNonePlanned}
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
			LinkedLines: []any{}, Movements: movements,
			Yield:        potYieldWire{Month: string(view.month), Days: []potYieldDayWire{}},
			HistoryStart: string(monthOfDay(firstOf(view.sightings[account.ID]))),
		}
	}
	return nil
}
