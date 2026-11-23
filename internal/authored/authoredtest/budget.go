package authoredtest

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Group is one group of a budget document.
func Group(key string, lines ...authored.DocumentLine) authored.DocumentGroup {
	return authored.DocumentGroup{Key: key, Lines: lines}
}

// Line is a new line: no id, no flag.
func Line(name string, cents money.Cents) authored.DocumentLine {
	return authored.DocumentLine{Name: name, Cents: cents}
}

// FlaggedLine is a new line with a flag.
func FlaggedLine(name string, cents money.Cents, flag string) authored.DocumentLine {
	return authored.DocumentLine{Name: name, Cents: cents, Flag: flag}
}

// InventedBudget is the budget most tests start from: income of 10,000.00 and
// a flagged extra, two fixed lines, two variable, one provision and one wish.
// Nothing of it is real.
func InventedBudget() authored.Document {
	return authored.Document{
		Group("rec", Line("Salário", 1_000_000), FlaggedLine("Renda extra", 150_000, "Não garantida")),
		Group("fix", Line("Contas da casa", 200_000), Line("Saúde", 50_000)),
		Group("var", Line("Mercado", 40_000), Line("Delivery", 20_000)),
		Group("prov", Line("Remédios", 15_000)),
		Group("res", Line("Viagens", 60_000)),
	}
}

// WithIDs is document with every line that has a name in ids carrying that id,
// so a test can send back what it read.
func WithIDs(document authored.Document, ids map[string]string) authored.Document {
	out := make(authored.Document, len(document))
	for i, group := range document {
		out[i] = authored.DocumentGroup{Key: group.Key, Lines: make([]authored.DocumentLine, len(group.Lines))}
		for j, line := range group.Lines {
			if id, found := ids[line.Name]; found {
				line.ID = &id
			}
			out[i].Lines[j] = line
		}
	}
	return out
}

// SaveBudget saves document from month as the person would and returns the
// change.
func SaveBudget(t testing.TB, database *authored.Database, at time.Time, month brday.Month, document authored.Document) authored.ChangeID {
	t.Helper()
	return recordBudgetChange(t, database, at, "budget.save", func(recorder *authored.Recorder) error {
		return recorder.SaveBudget(t.Context(), month, document)
	})
}

// SaveScenario keeps document under name as the person would.
func SaveScenario(t testing.TB, database *authored.Database, at time.Time, name string, document authored.Document) authored.ChangeID {
	t.Helper()
	return recordBudgetChange(t, database, at, "budget.saveScenario", func(recorder *authored.Recorder) error {
		return recorder.SaveScenario(t.Context(), name, document)
	})
}

// CloseMonth writes the closed month's row and nothing else: a month closed
// with no version of its own, which the store's Recorder.CloseMonth never
// leaves, for the rules that must still hold over one.
func CloseMonth(t testing.TB, database *authored.Database, at time.Time, month brday.Month, closedOn string) authored.ChangeID {
	t.Helper()
	return recordBudgetChange(t, database, at, "scratch.closeMonth", func(recorder *authored.Recorder) error {
		return recorder.Put(t.Context(), "closed_months", authored.Row{"month": authored.Text(string(month)), "closed_on": authored.Text(closedOn)})
	})
}

// PutVersion writes a version straight through the recorder, lines given by
// their rows, for the states a save cannot reach, such as a closed month with a
// version of its own.
func PutVersion(t testing.TB, database *authored.Database, at time.Time, from brday.Month, lines ...authored.BudgetLine) authored.ChangeID {
	t.Helper()
	return recordBudgetChange(t, database, at, "scratch.putVersion", func(recorder *authored.Recorder) error {
		if err := recorder.Put(t.Context(), "budget_versions", authored.Row{"from_month": authored.Text(string(from))}); err != nil {
			return err
		}
		for _, line := range lines {
			flag := authored.Null
			if line.Flag != "" {
				flag = authored.Text(line.Flag)
			}
			err := recorder.Put(t.Context(), "budget_version_lines", authored.Row{
				"from_month": authored.Text(string(from)), "group_key": authored.Text(line.Group),
				"position": authored.Integer(int64(line.Position)), "line_id": authored.Integer(int64(line.ID)),
				"name": authored.Text(line.Name), "cents": authored.Integer(int64(line.Cents)), "flag": flag,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func recordBudgetChange(t testing.TB, database *authored.Database, at time.Time, method string, write func(*authored.Recorder) error) authored.ChangeID {
	t.Helper()
	var id authored.ChangeID
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: method, At: at}
	err := database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		if err := write(recorder); err != nil {
			return err
		}
		var err error
		id, err = recorder.ID(t.Context())
		return err
	})
	if err != nil {
		t.Fatalf("authoredtest: %s: %v", method, err)
	}
	return id
}

// DocumentOf is a budget read back as the document that saves it again: every
// line with its id, in the groups' fixed order.
func DocumentOf(budget authored.Budget) authored.Document {
	document := make(authored.Document, len(authored.GroupKeys))
	for i, key := range authored.GroupKeys {
		document[i] = authored.DocumentGroup{Key: key, Lines: []authored.DocumentLine{}}
	}
	for _, line := range budget.Lines {
		id := line.ID.String()
		at := slices.Index(authored.GroupKeys, line.Group)
		document[at].Lines = append(document[at].Lines, authored.DocumentLine{ID: &id, Name: line.Name, Cents: line.Cents, Flag: line.Flag})
	}
	return document
}

// SeedBudget writes into recorder's change what a person who has kept a budget
// for a while would have: the invented budget from 2026-02, a second version
// from 2026-03 in which Mercado has been renamed Feira and Saúde raised, a
// scenario, and 2026-01 closed as the app closes a month, with a version of its
// own. All five groups, a flag and a renamed line are among it.
func SeedBudget(recorder *authored.Recorder) error {
	ctx := context.Background()
	if err := recorder.SaveBudget(ctx, "2026-02", InventedBudget()); err != nil {
		return err
	}
	view := recorder.View()
	defer view.Close()
	first, err := view.BudgetOf(ctx, "2026-02")
	if err != nil {
		return err
	}
	second := DocumentOf(first)
	for _, group := range second {
		for i := range group.Lines {
			switch group.Lines[i].Name {
			case "Mercado":
				group.Lines[i].Name = "Feira"
			case "Saúde":
				group.Lines[i].Cents = 90_000
			}
		}
	}
	if err := recorder.SaveBudget(ctx, "2026-03", second); err != nil {
		return err
	}
	if err := recorder.SaveScenario(ctx, "Sem viagem", withoutGroup(DocumentOf(first), "res")); err != nil {
		return err
	}
	return recorder.CloseMonth(ctx, "2026-01", "2026-02-03")
}

func withoutGroup(document authored.Document, key string) authored.Document {
	for i := range document {
		if document[i].Key == key {
			document[i].Lines = []authored.DocumentLine{}
		}
	}
	return document
}
