package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

type netWorthItemWire struct {
	Name   *string `json:"name"`
	Detail string  `json:"detail"`
	Amount Reais   `json:"amount"`
}

type netWorthBucketWire struct {
	Bucket string             `json:"bucket"`
	Total  Reais              `json:"total"`
	Items  []netWorthItemWire `json:"items"`
}

type netWorthPointWire struct {
	Month     string `json:"month"`
	Available Reais  `json:"available"`
	Reserved  Reais  `json:"reserved"`
	Invested  Reais  `json:"invested"`
	Total     Reais  `json:"total"`
}

type netWorthChartWire struct {
	Max    Reais               `json:"max"`
	Months []string            `json:"months"`
	Points []netWorthPointWire `json:"points"`
}

type monthContributionWire struct {
	Planned  Reais `json:"planned"`
	Done     Reais `json:"done"`
	Progress int   `json:"progress"`
}

type investmentRateWire struct {
	Month   string `json:"month"`
	Planned *int   `json:"planned"`
	Actual  *int   `json:"actual"`
}

type potYieldItemWire struct {
	Name       *string  `json:"name"`
	CDIPercent *Percent `json:"cdiPercent"`
	Amount     *Reais   `json:"amount"`
}

type netWorthYieldsWire struct {
	Pots  []potYieldItemWire `json:"pots"`
	Total *Reais             `json:"total"`
}

type netWorthOverviewWire struct {
	Total          Reais                 `json:"total"`
	Buckets        []netWorthBucketWire  `json:"buckets"`
	Chart          netWorthChartWire     `json:"chart"`
	HistoryStart   string                `json:"historyStart"`
	Contribution   monthContributionWire `json:"contribution"`
	InvestmentRate investmentRateWire    `json:"investmentRate"`
	Yields         netWorthYieldsWire    `json:"yields"`
}

// netWorthParts are the three parts of the net worth. The total is their sum
// and nothing else, so it cannot disagree with them.
type netWorthParts struct {
	available, reserved, invested money.Cents
}

func (parts netWorthParts) total() money.Cents {
	return parts.available + parts.reserved + parts.invested
}

func registerNetWorth(registry *Registry) {
	registry.Register("netWorth.overview", readNetWorthOverview)
}

func readNetWorthOverview(ctx context.Context, _ []json.RawMessage, reader *ledger.Reader) (any, error) {
	view, err := loadPots(ctx, reader, true)
	if err != nil {
		return nil, err
	}
	return view.netWorth(), nil
}

// partsOf adds up the bank accounts' balances as available, and each pot where
// its bucket says: until the person assigns one, the bank's own word for it.
func (view *potsView) partsOf() netWorthParts {
	var parts netWorthParts
	for _, account := range view.accounts {
		if account.Type == ledger.AccountBank {
			parts.available += account.Balance
		}
	}
	for _, entry := range view.entries {
		switch entry.bucket {
		case bucketAvailable:
			parts.available += entry.balance
		case bucketReserved:
			parts.reserved += entry.balance
		case bucketInvested:
			parts.invested += entry.balance
		}
	}
	return parts
}

func (view *potsView) netWorth() netWorthOverviewWire {
	parts := view.partsOf()
	chart, historyStart := view.chart()
	return netWorthOverviewWire{
		Total: Reais(parts.total()), Buckets: view.bucketsOf(parts), Chart: chart, HistoryStart: string(historyStart),
		Contribution:   monthContributionWire{},
		InvestmentRate: investmentRateWire{Month: string(view.month)},
		Yields:         view.yields(),
	}
}

// bucketsOf lists what makes each bucket up. An item with nothing in it that is
// closed is not listed, and every item listed adds up to the bucket's total.
func (view *potsView) bucketsOf(parts netWorthParts) []netWorthBucketWire {
	buckets := []netWorthBucketWire{
		{Bucket: bucketAvailable, Total: Reais(parts.available), Items: view.accountItems()},
		{Bucket: bucketReserved, Total: Reais(parts.reserved), Items: []netWorthItemWire{}},
		{Bucket: bucketInvested, Total: Reais(parts.invested), Items: []netWorthItemWire{}},
	}
	for _, entry := range view.entries {
		if entry.closed && entry.balance == 0 {
			continue
		}
		for index := range buckets {
			if buckets[index].Bucket == entry.bucket {
				buckets[index].Items = append(buckets[index].Items,
					netWorthItemWire{Name: entry.name, Detail: entry.bank, Amount: Reais(entry.balance)})
			}
		}
	}
	return buckets
}

func (view *potsView) accountItems() []netWorthItemWire {
	items := []netWorthItemWire{}
	for _, account := range orderedAccounts(view.accounts, view.banks) {
		if account.Type != ledger.AccountBank {
			continue
		}
		name := bankAccountDescription(view.banks[account.ID], nameOfAccount(account))
		items = append(items, netWorthItemWire{Name: &name, Detail: bankAccountItemDetail, Amount: Reais(account.Balance)})
	}
	return items
}

// yields lists each open pot's yield for the month. The total adds only what is
// known, and only when everything known is on one base: Nubank reports net of
// tax and Mercado Pago gross, and the two are never added into one figure.
func (view *potsView) yields() netWorthYieldsWire {
	wire := netWorthYieldsWire{Pots: []potYieldItemWire{}}
	var known []potEntry
	for _, entry := range view.entries {
		if entry.closed {
			continue
		}
		amount := monthlyYieldOf(entry)
		wire.Pots = append(wire.Pots, potYieldItemWire{Name: entry.name, CDIPercent: entry.cdiPercent, Amount: reaisPointer(amount)})
		if amount != nil {
			known = append(known, entry)
		}
	}
	wire.Total = reaisPointer(sumOnOneBase(known))
	return wire
}

// sumOnOneBase is nil when nothing is known and when the known yields are on
// more than one base.
func sumOnOneBase(known []potEntry) *money.Cents {
	if len(known) == 0 {
		return nil
	}
	var total money.Cents
	bases := map[potrules.Base]bool{}
	for _, entry := range known {
		bases[entry.base] = true
		total += *monthlyYieldOf(entry)
	}
	if len(bases) != 1 {
		return nil
	}
	return &total
}

func lastDayOf(month brday.Month) brday.Day {
	period, err := brday.MonthPeriod(month)
	if err != nil {
		return ""
	}
	return period.Last
}
