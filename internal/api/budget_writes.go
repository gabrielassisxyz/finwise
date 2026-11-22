package api

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// A budget document on the way in: the groups' names and order are ignored,
// because the five groups always exist, in the fixed order and under the fixed
// names. An amount is read with ParseReais, never as a float.
type draftLine struct {
	ID     *string `json:"id"`
	Name   string  `json:"name"`
	Amount Reais   `json:"amount"`
	Flag   *string `json:"flag"`
}

type draftGroup struct {
	Key   string      `json:"key"`
	Name  string      `json:"name"`
	Lines []draftLine `json:"lines"`
}

// A budget document on the way out.
type documentLineWire struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Amount Reais  `json:"amount"`
	Flag   string `json:"flag,omitempty"`
}

type savedScenarioWire struct {
	Name   string           `json:"name"`
	Budget []budgetGroupDoc `json:"budget"`
}

type changeResult struct {
	ChangeID string `json:"changeId"`
}

type scenarioSaved struct {
	ChangeID string `json:"changeId"`
	Name     string `json:"name"`
}

// documentOf is the budget in force for a month as the contract carries it,
// the five groups in the fixed order with the lines of that version in theirs.
func documentOf(budget authored.Budget) []budgetGroupDoc {
	groups := emptyBudgetDocument()
	for _, line := range budget.Lines {
		at := slices.Index(authored.GroupKeys, line.Group)
		groups[at].Lines = append(groups[at].Lines, documentLineWire{
			ID: line.ID.String(), Name: line.Name, Amount: Reais(line.Cents), Flag: line.Flag,
		})
	}
	return groups
}

func documentOfScenario(document authored.Document) []budgetGroupDoc {
	groups := emptyBudgetDocument()
	for _, group := range document {
		at := slices.Index(authored.GroupKeys, group.Key)
		if at < 0 {
			continue
		}
		for _, line := range group.Lines {
			wire := documentLineWire{Name: line.Name, Amount: Reais(line.Cents), Flag: line.Flag}
			if line.ID != nil {
				wire.ID = *line.ID
			}
			groups[at].Lines = append(groups[at].Lines, wire)
		}
	}
	return groups
}

// draftArgument reads a budget document. A negative amount is a client's bug
// and so is refused as arguments, before the month or the document is judged.
func draftArgument(raw json.RawMessage) (authored.Document, error) {
	var groups []draftGroup
	if err := decodeArgument(raw, &groups); err != nil {
		return nil, err
	}
	document := make(authored.Document, len(groups))
	for i, group := range groups {
		document[i] = authored.DocumentGroup{Key: group.Key, Lines: make([]authored.DocumentLine, len(group.Lines))}
		for j, line := range group.Lines {
			if line.Amount < 0 {
				return nil, &badArgumentsError{}
			}
			found := authored.DocumentLine{ID: line.ID, Name: line.Name, Cents: money.Cents(line.Amount)}
			if line.Flag != nil {
				found.Flag = *line.Flag
			}
			document[i].Lines[j] = found
		}
	}
	return document, nil
}

// budgetRefusal turns what the store refuses into the reason the person reads.
func budgetRefusal(err error) error {
	var invalid *authored.InvalidBudgetError
	switch {
	case errors.Is(err, authored.ErrMonthClosed):
		return refuse(reasonMonthClosed)
	case errors.As(err, &invalid):
		return refuse(reasonInvalidBudget)
	}
	return err
}

// saveBudget makes the document the budget from a month on.
func saveBudget(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	return saveFromMonth(ctx, args, writes)
}

// applyScenario is the same operation under its own method name: the scenario
// is a document the client already holds.
func applyScenario(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	return saveFromMonth(ctx, args, writes)
}

func saveFromMonth(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	month, err := monthArgument(args[0])
	if err != nil {
		return nil, err
	}
	document, err := draftArgument(args[1])
	if err != nil {
		return nil, err
	}
	if err := writes.Recorder.SaveBudget(ctx, month, document); err != nil {
		return nil, budgetRefusal(err)
	}
	id, err := writes.Recorder.ID(ctx)
	if err != nil {
		return nil, err
	}
	return changeResult{ChangeID: id.String()}, nil
}

// saveScenario keeps a document under a name, which is stored trimmed; a blank
// one is called what the person reads for an unnamed scenario.
func saveScenario(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
	if len(args) != 2 {
		return nil, &badArgumentsError{}
	}
	var name *string
	if err := decodeArgument(args[0], &name); err != nil || name == nil {
		return nil, &badArgumentsError{}
	}
	document, err := draftArgument(args[1])
	if err != nil {
		return nil, err
	}
	kept := strings.TrimSpace(*name)
	if kept == "" {
		kept = unnamedScenario
	}
	if err := writes.Recorder.SaveScenario(ctx, kept, document); err != nil {
		return nil, budgetRefusal(err)
	}
	id, err := writes.Recorder.ID(ctx)
	if err != nil {
		return nil, err
	}
	return scenarioSaved{ChangeID: id.String(), Name: kept}, nil
}
