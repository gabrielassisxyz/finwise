package authored_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// newExamplesBudgetEnv is the state the close's examples start from: one
// version from 2026-01 holding Mercado 400,00 and Delivery 200,00.
func newExamplesBudgetEnv(t *testing.T) *budgetEnv {
	t.Helper()
	env := newBudgetEnvWithNothing(t)
	env.save("2026-01", authored.Document{authoredtest.Group("var", authoredtest.Line("Mercado", 40_000), authoredtest.Line("Delivery", 20_000))})
	return env
}

// january is the document in force for 2026-01, its lines carrying their ids,
// changed by edit.
func (env *budgetEnv) january(edit func(authored.Document) authored.Document) authored.Document {
	return edit(authoredtest.DocumentOf(env.budget("2026-01")))
}

func unchanged(document authored.Document) authored.Document { return document }

func withLineAdded(document authored.Document) authored.Document {
	for i := range document {
		if document[i].Key == "var" {
			document[i].Lines = append(document[i].Lines, authoredtest.Line("Farmácia", 5_000))
		}
	}
	return document
}

// mercadoFlagged is the document with Mercado carrying a flag and nothing else
// changed.
func mercadoFlagged(document authored.Document) authored.Document {
	return eachMercado(document, func(line *authored.DocumentLine) { line.Flag = "Não garantida" })
}

// mercadoUnderANewID is the document with Mercado sent as a new line of the same
// name and amount in the same place, which the save gives an id of its own.
func mercadoUnderANewID(document authored.Document) authored.Document {
	return eachMercado(document, func(line *authored.DocumentLine) { line.ID = nil })
}

func eachMercado(document authored.Document, change func(*authored.DocumentLine)) authored.Document {
	for i := range document {
		for j := range document[i].Lines {
			if document[i].Lines[j].Name == "Mercado" {
				change(&document[i].Lines[j])
			}
		}
	}
	return document
}

// differOnlyIn says whether two budgets' lines differ, and are equal once blank
// has cleared the one field a case changes.
func differOnlyIn(a, b []authored.BudgetLine, blank func(*authored.BudgetLine)) bool {
	cleared := func(lines []authored.BudgetLine) []authored.BudgetLine {
		out := slices.Clone(lines)
		for i := range out {
			blank(&out[i])
		}
		return out
	}
	return !slices.Equal(a, b) && slices.Equal(cleared(a), cleared(b))
}

func deliveryFirst(document authored.Document) authored.Document {
	for i := range document {
		if document[i].Key == "var" {
			slices.Reverse(document[i].Lines)
		}
	}
	return document
}

func (env *budgetEnv) mustClose(month brday.Month) authored.ChangeID {
	env.t.Helper()
	id, err := env.closeOn(month, "2026-03-20")
	if err != nil {
		env.t.Fatalf("closing %s: %v", month, err)
	}
	return id
}

// expectRefusedByTheClosedMonths undoes change and wants it refused by a
// registered check, with nothing changed and the change not undone.
func (env *budgetEnv) expectRefusedByTheClosedMonths(label string, change authored.ChangeID) {
	env.t.Helper()
	before := env.scratch.State(env.t)
	err := env.undo(change)
	var stale *authored.StaleChangeError
	if !errors.As(err, &stale) || stale.Cause != authored.CauseCheck {
		env.t.Errorf("%s: the undo answered %v, want a refusal by a check", label, err)
	}
	if env.scratch.State(env.t) != before {
		env.t.Errorf("%s: the refused undo changed the database or the journal", label)
	}
}

// A month saved before it is closed takes its budget from that save, which the
// close finds and keeps; the save's undo would hand the closed month back to
// January's version. Every way the two differ refuses the undo, and none does
// when they are equal.
func TestAnUndoThatWouldMoveAClosedMonthsBudgetIsRefusedWhateverMoves(t *testing.T) {
	cases := []struct {
		name    string
		edit    func(authored.Document) authored.Document
		refused bool
		// alone, for a case that changes one field, clears that field: what the
		// save left must differ from January's budget in it and in nothing else.
		alone func(*authored.BudgetLine)
	}{
		{"an amount", authoredtest.WithCents("Mercado", 50_000), true, nil},
		{"a name", authoredtest.Renamed("Mercado", "Feira"), true, nil},
		{"the order of two lines", deliveryFirst, true, nil},
		{"a line added", withLineAdded, true, nil},
		{"a line removed", authoredtest.Without("Delivery"), true, nil},
		{"a flag alone", mercadoFlagged, true, func(line *authored.BudgetLine) { line.Flag = "" }},
		{"an id alone", mercadoUnderANewID, true, func(line *authored.BudgetLine) { line.ID = 0 }},
		{"nothing", unchanged, false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newExamplesBudgetEnv(t)
			save := env.save("2026-02", env.january(c.edit))
			kept := env.budget("2026-02")
			if c.alone != nil && !differOnlyIn(kept.Lines, env.budget("2026-01").Lines, c.alone) {
				t.Fatalf("the save left %+v, which does not differ from January's %+v in that field alone", kept.Lines, env.budget("2026-01").Lines)
			}
			env.mustClose("2026-02")
			if !c.refused {
				if err := env.undo(save); err != nil {
					t.Fatalf("the undo of a save that leaves the closed month's budget as it was: %v", err)
				}
				if got := env.budget("2026-02"); got.From != "2026-01" || !slices.Equal(got.Lines, kept.Lines) {
					t.Errorf("the closed month now reads %+v, want January's equal lines", got)
				}
				return
			}
			env.expectRefusedByTheClosedMonths("the undo of the save", save)
			if got := env.budget("2026-02"); !slices.Equal(got.Lines, kept.Lines) {
				t.Errorf("after the refusal the closed month reads %+v", got.Lines)
			}
		})
	}
}

// The closed month keeps its version and only its lines move back: the
// version's own row is in no unit of the undo.
func TestAnUndoThatWouldChangeTheLinesOfAClosedMonthsOwnVersionIsRefused(t *testing.T) {
	env := newExamplesBudgetEnv(t)
	env.save("2026-02", env.january(authoredtest.WithCents("Mercado", 45_000)))
	raise := env.save("2026-02", env.january(authoredtest.WithCents("Mercado", 50_000)))
	env.mustClose("2026-02")
	env.expectRefusedByTheClosedMonths("the undo of the raise", raise)
}

func TestAnUndoThatMovesOnlyAnOpenMonthOrTheMonthItReopensPasses(t *testing.T) {
	open := newExamplesBudgetEnv(t)
	open.mustClose("2026-02")
	april := open.save("2026-04", open.january(authoredtest.WithCents("Mercado", 90_000)))
	if err := open.undo(april); err != nil {
		t.Errorf("the undo of a save for an open month: %v", err)
	}

	reopened := newExamplesBudgetEnv(t)
	closing := reopened.mustClose("2026-02")
	reopened.save("2026-01", reopened.january(authoredtest.WithCents("Mercado", 45_000)))
	if err := reopened.undo(closing); err != nil {
		t.Fatalf("the undo of the close, which reopens the month on another budget: %v", err)
	}
	if got := reopened.cents("2026-02", "Mercado"); got != 45_000 {
		t.Errorf("the reopened month reads Mercado %d, want January's 45000", got)
	}
}

// Removing a version takes with it a line a later save added there, which is
// no unit of the undo; the closed month had it, so the undo is refused even
// though what January holds equals the lines the undo names.
func TestAVersionsLineThatNoUnitNamesStillCountsAsWhatTheClosedMonthHad(t *testing.T) {
	env := newExamplesBudgetEnv(t)
	first := env.save("2026-02", env.january(unchanged))
	added := env.save("2026-02", withLineAdded(authoredtest.DocumentOf(env.budget("2026-02"))))
	if effects := env.scratch.EffectsOf(t, int64(added)); len(effects) != 1 {
		t.Fatalf("the second save recorded %+v, want the added line alone", effects)
	}
	env.mustClose("2026-02")
	env.expectRefusedByTheClosedMonths("the undo of the first save", first)
}

func TestTheRestoreOfAPartIsRefusedByTheClosedMonthsCheckAndLeavesTheChangeOpen(t *testing.T) {
	env := newExamplesBudgetEnv(t)
	save := env.save("2026-02", env.january(authoredtest.WithCents("Mercado", 50_000)))
	env.mustClose("2026-02")
	before := env.scratch.Canonical(t)
	budgetUnits := func(unit authored.Unit) bool {
		return unit.Table == "budget_versions" || unit.Table == "budget_version_lines"
	}
	err := env.change(func(recorder *authored.Recorder) error {
		err := recorder.RestorePart(t.Context(), save, budgetUnits)
		var stale *authored.StaleChangeError
		if !errors.As(err, &stale) || stale.Cause != authored.CauseCheck {
			t.Errorf("the restore answered %v, want a refusal by a check", err)
		}
		_, err = recorder.ID(t.Context())
		return err
	})
	if err != nil {
		t.Fatalf("the change around the refused restore: %v", err)
	}
	if after := env.scratch.Canonical(t); after != before {
		t.Errorf("the refused restore changed the tables:\n%s", after)
	}
}
