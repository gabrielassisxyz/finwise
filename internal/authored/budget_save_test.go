package authored_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

var budgetClock = time.Date(2026, time.March, 20, 15, 0, 0, 0, time.UTC)

// budgetEnv is an app database whose only version is the invented budget from
// 2026-02, as the examples of the budget's rules start.
type budgetEnv struct {
	t       *testing.T
	scratch *authoredtest.Scratch
	tick    int
}

func newBudgetEnv(t *testing.T) *budgetEnv {
	t.Helper()
	env := &budgetEnv{t: t, scratch: authoredtest.Open(t, func() time.Time { return budgetClock })}
	env.save("2026-02", authoredtest.InventedBudget())
	return env
}

func (env *budgetEnv) at() time.Time {
	env.tick++
	return budgetClock.Add(time.Duration(env.tick) * time.Minute)
}

func (env *budgetEnv) save(month brday.Month, document authored.Document) authored.ChangeID {
	env.t.Helper()
	return authoredtest.SaveBudget(env.t, env.scratch.Database, env.at(), month, document)
}

// try is a save that may be refused; what it returns is the refusal.
func (env *budgetEnv) try(month brday.Month, document authored.Document) error {
	return env.change(func(recorder *authored.Recorder) error {
		return recorder.SaveBudget(env.t.Context(), month, document)
	})
}

func (env *budgetEnv) change(write func(*authored.Recorder) error) error {
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "budget.save", At: env.at()}
	return env.scratch.Database.Change(env.t.Context(), facts, write)
}

func (env *budgetEnv) undo(id authored.ChangeID) error {
	return env.scratch.Database.Undo(env.t.Context(), id, env.at())
}

func (env *budgetEnv) budget(month brday.Month) authored.Budget {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	budget, err := view.BudgetOf(env.t.Context(), month)
	if err != nil {
		env.t.Fatal(err)
	}
	return budget
}

func (env *budgetEnv) line(month brday.Month, name string) (authored.BudgetLine, bool) {
	for _, line := range env.budget(month).Lines {
		if line.Name == name {
			return line, true
		}
	}
	return authored.BudgetLine{}, false
}

func (env *budgetEnv) cents(month brday.Month, name string) money.Cents {
	env.t.Helper()
	line, found := env.line(month, name)
	if !found {
		env.t.Fatalf("%s has no line %s", month, name)
	}
	return line.Cents
}

func (env *budgetEnv) versions() string {
	env.t.Helper()
	rows, err := env.scratch.Inspect(env.t).Query("SELECT from_month FROM budget_versions ORDER BY from_month")
	if err != nil {
		env.t.Fatal(err)
	}
	defer rows.Close()
	var months []string
	for rows.Next() {
		var month string
		if err := rows.Scan(&month); err != nil {
			env.t.Fatal(err)
		}
		months = append(months, month)
	}
	return strings.Join(months, " ")
}

func (env *budgetEnv) lineCount() int {
	env.t.Helper()
	var count int
	if err := env.scratch.Inspect(env.t).QueryRow("SELECT count(*) FROM lines").Scan(&count); err != nil {
		env.t.Fatal(err)
	}
	return count
}

// withHealth is the document in force for month with Saúde at cents.
func (env *budgetEnv) withHealth(month brday.Month, cents money.Cents) authored.Document {
	document := authoredtest.DocumentOf(env.budget(month))
	for _, group := range document {
		for i := range group.Lines {
			if group.Lines[i].Name == "Saúde" {
				group.Lines[i].Cents = cents
			}
		}
	}
	return document
}

func (env *budgetEnv) closeMonths(months ...brday.Month) {
	for _, month := range months {
		authoredtest.CloseMonth(env.t, env.scratch.Database, env.at(), month, "2026-12-30")
	}
}

func TestTheVersionAMonthUsesIsTheLatestThatStartsAtOrBeforeIt(t *testing.T) {
	env := newBudgetEnv(t)
	env.save("2026-05", env.withHealth("2026-05", 70_000))
	for month, want := range map[brday.Month]money.Cents{
		"2026-01": 50_000, "2026-02": 50_000, "2026-04": 50_000, "2026-05": 70_000, "2026-09": 70_000,
	} {
		if got := env.cents(month, "Saúde"); got != want {
			t.Errorf("%s: Saúde = %d, want %d", month, got, want)
		}
	}
	empty := newBudgetEnvWithNothing(t)
	if budget := empty.budget("2026-03"); budget.Exists || budget.HasLines() {
		t.Errorf("a database with no version has a budget: %+v", budget)
	}
}

func newBudgetEnvWithNothing(t *testing.T) *budgetEnv {
	t.Helper()
	return &budgetEnv{t: t, scratch: authoredtest.Open(t, func() time.Time { return budgetClock })}
}

// A table of saves, each over a database that starts from the invented budget
// as its only version, from 2026-02. Every case is also undone and must give
// back the exact state before it.
func TestTheVersionsASaveWrites(t *testing.T) {
	cases := []struct {
		name     string
		prepare  func(*budgetEnv)
		month    brday.Month
		health   money.Cents
		want     map[brday.Month]money.Cents
		versions string
	}{
		{"a save holds from its month on", nil, "2026-03", 90_000,
			map[brday.Month]money.Cents{"2026-04": 90_000, "2026-03": 90_000, "2026-02": 50_000, "2026-01": 50_000},
			"2026-02 2026-03"},
		{"a later version is replaced by an earlier save",
			func(env *budgetEnv) { env.save("2026-04", env.withHealth("2026-04", 70_000)) }, "2026-03", 90_000,
			map[brday.Month]money.Cents{"2026-03": 90_000, "2026-04": 90_000}, "2026-02 2026-03"},
		{"a closed month keeps its budget and the budget resumes after it",
			func(env *budgetEnv) { env.closeMonths("2026-04") }, "2026-03", 90_000,
			map[brday.Month]money.Cents{"2026-03": 90_000, "2026-04": 50_000, "2026-05": 90_000},
			"2026-02 2026-03 2026-04 2026-05"},
		{"two closed months in a row resume after the second",
			func(env *budgetEnv) { env.closeMonths("2026-04", "2026-05") }, "2026-03", 90_000,
			map[brday.Month]money.Cents{"2026-04": 50_000, "2026-05": 50_000, "2026-06": 90_000},
			"2026-02 2026-03 2026-04 2026-05 2026-06"},
		{"a closed month with a version of its own keeps it",
			func(env *budgetEnv) {
				authoredtest.PutVersion(env.t, env.scratch.Database, env.at(), "2026-04", env.withVersionLines("2026-02", 65_000)...)
				env.closeMonths("2026-04")
			}, "2026-03", 90_000,
			map[brday.Month]money.Cents{"2026-03": 90_000, "2026-04": 65_000, "2026-05": 90_000},
			"2026-02 2026-03 2026-04 2026-05"},
		{"a save earlier than every version becomes the first", nil, "2026-01", 90_000,
			map[brday.Month]money.Cents{"2025-12": 90_000, "2026-01": 90_000, "2026-03": 90_000}, "2026-01"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newBudgetEnv(t)
			if c.prepare != nil {
				c.prepare(env)
			}
			before := env.scratch.Canonical(t)
			change := env.save(c.month, env.withHealth(c.month, c.health))
			for month, want := range c.want {
				if got := env.cents(month, "Saúde"); got != want {
					t.Errorf("%s: Saúde = %d, want %d", month, got, want)
				}
			}
			if got := env.versions(); got != c.versions {
				t.Errorf("versions = %q, want %q", got, c.versions)
			}
			for _, effect := range env.scratch.EffectsOf(t, int64(change)) {
				if effect.Table == "lines" {
					t.Errorf("the change has an effect on lines: %+v", effect)
				}
			}
			if err := env.undo(change); err != nil {
				t.Fatalf("undo: %v", err)
			}
			if after := env.scratch.Canonical(t); after != before {
				t.Errorf("the undo left another state:\n%s\nwant\n%s", after, before)
			}
		})
	}
}

// withVersionLines is the lines of the version in force for month, Saúde
// changed, as rows a fixture can write.
func (env *budgetEnv) withVersionLines(month brday.Month, health money.Cents) []authored.BudgetLine {
	lines := env.budget(month).Lines
	for i := range lines {
		if lines[i].Name == "Saúde" {
			lines[i].Cents = health
		}
	}
	return lines
}

func TestAClosedMonthsOwnVersionIsInNoEffect(t *testing.T) {
	env := newBudgetEnv(t)
	authoredtest.PutVersion(t, env.scratch.Database, env.at(), "2026-04", env.withVersionLines("2026-02", 65_000)...)
	env.closeMonths("2026-04")
	change := env.save("2026-03", env.withHealth("2026-03", 90_000))
	for _, effect := range env.scratch.EffectsOf(t, int64(change)) {
		if strings.Contains(effect.Key, `"2026-04"`) {
			t.Errorf("an effect touches the closed month's own version: %+v", effect)
		}
	}
}

func TestASaveForAClosedMonthIsRefusedAndChangesNothing(t *testing.T) {
	env := newBudgetEnv(t)
	env.closeMonths("2026-03")
	before, journal := env.scratch.Everything(t), env.scratch.Journal(t)
	invalid := authored.Document{authoredtest.Group("fix", authoredtest.Line("", 1))}
	for name, document := range map[string]authored.Document{"valid": env.withHealth("2026-03", 1), "invalid": invalid} {
		if err := env.try("2026-03", document); !errors.Is(err, authored.ErrMonthClosed) {
			t.Errorf("%s document: err = %v, want ErrMonthClosed", name, err)
		}
	}
	if env.scratch.Everything(t) != before || env.scratch.Journal(t) != journal {
		t.Error("a refused save changed the database or the journal")
	}
}

func TestSavingTheDocumentJustReadRecordsAnEmptyChangeWhoseUndoChangesNothing(t *testing.T) {
	env := newBudgetEnv(t)
	before := env.scratch.Canonical(t)
	var id authored.ChangeID
	err := env.change(func(recorder *authored.Recorder) error {
		if err := recorder.SaveBudget(t.Context(), "2026-02", authoredtest.DocumentOf(env.budget("2026-02"))); err != nil {
			return err
		}
		var idErr error
		id, idErr = recorder.ID(t.Context())
		return idErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if effects := env.scratch.EffectsOf(t, int64(id)); len(effects) != 0 {
		t.Errorf("the change has %d effects, want none: %+v", len(effects), effects)
	}
	if err := env.undo(id); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if env.scratch.Canonical(t) != before {
		t.Error("undoing an empty change changed the state")
	}
}
