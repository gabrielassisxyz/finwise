package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

type potRowWire struct {
	ID           string   `json:"id"`
	Name         *string  `json:"name"`
	Bank         string   `json:"bank"`
	Bucket       *string  `json:"bucket"`
	Balance      Reais    `json:"balance"`
	CDIPercent   *Percent `json:"cdiPercent"`
	MonthlyYield *Reais   `json:"monthlyYield"`
	Maturity     *string  `json:"maturity"`
	Target       any      `json:"target"`
	Closed       bool     `json:"closed"`
}

type potGroupWire struct {
	Bucket   *string      `json:"bucket"`
	Subtotal Reais        `json:"subtotal"`
	Pots     []potRowWire `json:"pots"`
}

type potsListOptions struct {
	IncludeClosed *bool `json:"includeClosed"`
}

func registerPots(registry *Registry) {
	registry.Register("savingsPots.list", readPotsList)
	registry.Register("savingsPots.detail", readPotDetail)
}

func readPotsList(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	includeClosed, err := includeClosedOf(args)
	if err != nil {
		return nil, err
	}
	view, err := loadPotsWithChoices(ctx, views, true)
	if err != nil {
		return nil, err
	}
	return view.groups(includeClosed), nil
}

func includeClosedOf(args []json.RawMessage) (bool, error) {
	if len(args) != 1 {
		return false, &badArgumentsError{}
	}
	var options potsListOptions
	if err := decodeArgument(args[0], &options); err != nil || options.IncludeClosed == nil {
		return false, &badArgumentsError{}
	}
	return *options.IncludeClosed, nil
}

// groups lists the pots by bucket, reserved, invested and then available, as
// the stand-in orders them. The pots are never in a group of their own for want
// of a bucket: until the person assigns one, a pot is where the bank says it is.
func (view *potsView) groups(includeClosed bool) []potGroupWire {
	groups := []potGroupWire{}
	for _, bucket := range []string{bucketReserved, bucketInvested, bucketAvailable} {
		group := potGroupWire{Bucket: textPointer(bucket), Pots: []potRowWire{}}
		var subtotal money.Cents
		for _, entry := range view.entries {
			if entry.bucket != bucket || (entry.closed && !includeClosed) {
				continue
			}
			subtotal += entry.balance
			group.Pots = append(group.Pots, view.rowOf(entry))
		}
		group.Subtotal = Reais(subtotal)
		if len(group.Pots) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

func textPointer(text string) *string { return &text }

func (view *potsView) rowOf(entry potEntry) potRowWire {
	row := potRowWire{
		ID: entry.id, Name: entry.name, Bank: entry.bank, Bucket: textPointer(entry.bucket),
		Balance: Reais(entry.balance), CDIPercent: entry.cdiPercent, Closed: entry.closed,
		MonthlyYield: reaisPointer(monthlyYieldOf(entry)),
	}
	if entry.investment != nil && entry.investment.due != nil {
		row.Maturity = textPointer(string(*entry.investment.due))
	}
	return row
}

func reaisPointer(cents *money.Cents) *Reais {
	if cents == nil {
		return nil
	}
	amount := Reais(*cents)
	return &amount
}
