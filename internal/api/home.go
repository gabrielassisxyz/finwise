package api

import (
	"context"
	"encoding/json"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

type monthLimitWire struct {
	Cap   Reais `json:"cap"`
	Spent Reais `json:"spent"`
}

type netWorthSummaryWire struct {
	Available Reais `json:"available"`
	Reserved  Reais `json:"reserved"`
	Invested  Reais `json:"invested"`
}

type homePayAheadWire struct {
	Amount    Reais  `json:"amount"`
	CardLabel string `json:"cardLabel"`
}

// homeSummaryWire is the contract's HomeSummary. The limit and the week guides
// come from a budget, which nobody has made, and the pending count from
// reviews, which nobody has done: they are the contract's empty value.
type homeSummaryWire struct {
	Limit        monthLimitWire      `json:"limit"`
	Weeks        []any               `json:"weeks"`
	NetWorth     netWorthSummaryWire `json:"netWorth"`
	PendingCount int                 `json:"pendingCount"`
	PayAhead     *homePayAheadWire   `json:"payAhead"`
}

func registerHome(registry *Registry) {
	registry.Register("home.summary", readHomeSummary)
}

func readHomeSummary(ctx context.Context, args []json.RawMessage, reader *ledger.Reader) (any, error) {
	month, err := onlyMonthArgument(args)
	if err != nil {
		return nil, err
	}
	// The summary shows the parts and not the yield, so the statement is not asked.
	view, err := loadPots(ctx, reader, nil)
	if err != nil {
		return nil, err
	}
	card, err := loadCard(ctx, reader)
	if err != nil {
		return nil, err
	}
	parts := view.partsOf()
	return homeSummaryWire{
		Weeks:    []any{},
		NetWorth: netWorthSummaryWire{Available: Reais(parts.available), Reserved: Reais(parts.reserved), Invested: Reais(parts.invested)},
		PayAhead: payAheadOfHome(card, month),
	}, nil
}

// payAheadOfHome is offered only while there is something to pay ahead: the
// card's next-bill amount is known and above zero.
func payAheadOfHome(card *cardView, month brday.Month) *homePayAheadWire {
	if card == nil {
		return nil
	}
	next := card.nextBill(month)
	if !next.Known || next.Total >= 0 {
		return nil
	}
	return &homePayAheadWire{Amount: Reais(-next.Total), CardLabel: nameOfAccount(card.account)}
}
