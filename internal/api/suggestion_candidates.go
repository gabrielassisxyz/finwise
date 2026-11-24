package api

import (
	"context"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// SuggestionPurchase is a pending purchase at an establishment that has never
// been asked of a model.
type SuggestionPurchase struct {
	Key         authored.TxKey
	Month       brday.Month
	Description string
	Cents       money.Cents
	Day         brday.Day
}

// SuggestionLine is a budget line of a month as the model may name it.
type SuggestionLine struct {
	ID   authored.LineID
	Name string
}

// SuggestionExample is one of the person's own past decisions: an
// establishment's shown name and the line they gave its newest transaction.
type SuggestionExample struct {
	Establishment string
	LineID        authored.LineID
	Line          string
}

// SuggestionMonth is one month's unasked pending purchases with that month's
// budget lines.
type SuggestionMonth struct {
	Month     brday.Month
	Lines     []SuggestionLine
	Purchases []SuggestionPurchase
}

// SuggestionCandidates is everything the model suggestion step needs from the
// ledger and the app's database: the unasked pending purchases grouped by
// month, and the person's own recent establishment decisions as examples.
type SuggestionCandidates struct {
	Months   []SuggestionMonth
	Examples []SuggestionExample
}

// ReadSuggestionCandidates applies the single definition of waiting for review
// (pendingTransactions) and returns every money-out purchase at an
// establishment (not a movement, not a person) that has no row in
// model_suggestions, grouped by month with that month's line names and the
// person's own recent establishment decisions.
func ReadSuggestionCandidates(ctx context.Context, views Views) (SuggestionCandidates, error) {
	pending, err := pendingTransactions(ctx, views, wholeLedger.Last)
	if err != nil || len(pending) == 0 {
		return SuggestionCandidates{}, err
	}
	var eligible []pendingTransaction
	var eligibleKeys []authored.TxKey
	for _, item := range pending {
		if !eligibleForModel(item.listed) {
			continue
		}
		eligible = append(eligible, item)
		eligibleKeys = append(eligibleKeys, item.key)
	}
	if len(eligible) == 0 {
		return SuggestionCandidates{}, nil
	}
	asked, err := views.Authored.ModelSuggestions(ctx, eligibleKeys)
	if err != nil {
		return SuggestionCandidates{}, err
	}
	byMonth := map[brday.Month][]SuggestionPurchase{}
	for _, item := range eligible {
		if _, seen := asked[item.key]; seen {
			continue
		}
		month := monthOfDay(item.listed.Day)
		byMonth[month] = append(byMonth[month], SuggestionPurchase{
			Key:         item.key,
			Month:       month,
			Description: item.listed.RawDescription,
			Cents:       item.listed.Amount,
			Day:         item.listed.Day,
		})
	}
	if len(byMonth) == 0 {
		return SuggestionCandidates{}, nil
	}
	months, err := buildSuggestionMonths(ctx, views.Authored, byMonth)
	if err != nil {
		return SuggestionCandidates{}, err
	}
	examples, err := readSuggestionExamples(ctx, views)
	if err != nil {
		return SuggestionCandidates{}, err
	}
	return SuggestionCandidates{Months: months, Examples: examples}, nil
}

func eligibleForModel(listed txrules.Transaction) bool {
	return listed.Amount < 0 && listed.Movement == txrules.NotAMovement && !listed.Person
}

func buildSuggestionMonths(ctx context.Context, view *authored.View, byMonth map[brday.Month][]SuggestionPurchase) ([]SuggestionMonth, error) {
	keys := make([]brday.Month, 0, len(byMonth))
	for month := range byMonth {
		keys = append(keys, month)
	}
	slices.Sort(keys)
	months := make([]SuggestionMonth, 0, len(keys))
	for _, month := range keys {
		lines, err := linesOfMonth(ctx, view, month)
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			continue
		}
		converted := make([]SuggestionLine, 0, len(lines))
		for _, line := range lines {
			converted = append(converted, SuggestionLine{ID: line.id, Name: line.name})
		}
		months = append(months, SuggestionMonth{
			Month:     month,
			Lines:     converted,
			Purchases: byMonth[month],
		})
	}
	return months, nil
}

func readSuggestionExamples(ctx context.Context, views Views) ([]SuggestionExample, error) {
	decisions, err := views.Authored.PersonExamples(ctx)
	if err != nil || len(decisions) == 0 {
		return nil, err
	}
	whole, _, err := listedLedger(ctx, views.Ledger)
	if err != nil {
		return nil, err
	}
	newestListed := map[string]txrules.Transaction{}
	for _, listed := range whole.listed {
		if _, seen := newestListed[listed.EstablishmentKey]; !seen {
			newestListed[listed.EstablishmentKey] = listed
		}
	}
	estKeys := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		estKeys = append(estKeys, decision.EstablishmentKey)
	}
	establishments, err := views.Authored.Establishments(ctx, estKeys)
	if err != nil {
		return nil, err
	}
	monthCache := map[brday.Month]monthLines{}
	examples := make([]SuggestionExample, 0, len(decisions))
	for _, decision := range decisions {
		listed, found := newestListed[decision.EstablishmentKey]
		if !found || listed.Person || listed.Movement != txrules.NotAMovement {
			continue
		}
		month := monthOfDay(decision.Day)
		lines, cached := monthCache[month]
		if !cached {
			lines, err = linesOfMonth(ctx, views.Authored, month)
			if err != nil {
				return nil, err
			}
			monthCache[month] = lines
		}
		lineName, ok := lines.nameOf(decision.Line)
		if !ok {
			continue
		}
		examples = append(examples, SuggestionExample{
			Establishment: shownName(establishments, decision.EstablishmentKey, listed.Name),
			LineID:        decision.Line,
			Line:          lineName,
		})
	}
	return examples, nil
}
