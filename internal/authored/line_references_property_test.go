package authored_test

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// The months a random history saves, closes and dates its transactions in,
// before, at and after the versions it writes.
var historyMonths = []brday.Month{"2025-11", "2025-12", "2026-01", "2026-02", "2026-03", "2026-04", "2026-05", "2026-06", "2026-07"}

const historySeeds = 30

// lineHistory is one random history over an app database: invented budgets and
// transactions, nothing of them real. steps is what reproduces a failure along
// with the seed.
type lineHistory struct {
	*lineEnv
	random *rand.Rand
	seed   uint64
	closed []brday.Month
	names  int
	steps  []string
}

func newLineHistory(t *testing.T, seed uint64) *lineHistory {
	t.Helper()
	history := &lineHistory{
		lineEnv: &lineEnv{t: t, scratch: authoredtest.Open(t, func() time.Time { return lineClock })},
		random:  rand.New(rand.NewPCG(seed, seed*7919)), seed: seed,
	}
	first := historyMonths[1+history.random.IntN(3)]
	history.record("the invented budget saved for %s", first)
	authoredtest.SaveBudget(t, history.scratch.Database, history.at(), first, authoredtest.InventedBudget())
	for range 2 {
		history.saveAt(history.openMonth())
	}
	history.indexTransactions(12)
	return history
}

func (history *lineHistory) record(format string, args ...any) {
	history.steps = append(history.steps, fmt.Sprintf(format, args...))
}

// fail reports with what reproduces it: the seed and every step so far.
func (history *lineHistory) fail(format string, args ...any) {
	history.t.Helper()
	history.t.Errorf("seed %d, after %s:\n%s", history.seed, strings.Join(history.steps, "; "), fmt.Sprintf(format, args...))
}

func (history *lineHistory) month() brday.Month {
	return historyMonths[history.random.IntN(len(historyMonths))]
}

func (history *lineHistory) openMonth() brday.Month {
	for {
		if month := history.month(); !slices.Contains(history.closed, month) {
			return month
		}
	}
}

func (history *lineHistory) linesOf(month brday.Month) []authored.LineID {
	var lines []authored.LineID
	for _, line := range history.budget(month).Lines {
		lines = append(lines, line.ID)
	}
	if len(lines) > 0 {
		lines = append(lines, authoredtest.FeesLine)
	}
	return lines
}

// indexTransactions writes count transactions on random days, each holding
// random references to the lines its month has, so the history starts with
// none stray, and random facts that name no line, which no save may touch.
func (history *lineHistory) indexTransactions(count int) {
	history.t.Helper()
	history.change("scratch.randomTransactions", func(recorder *authored.Recorder) error {
		for i := range count {
			month := history.month()
			day := brday.Day(fmt.Sprintf("%s-%02d", month, 1+history.random.IntN(28)))
			key, err := recorder.IndexTransaction(history.t.Context(), authoredtest.Seen(fmt.Sprintf("random-%d", i), day, -money.Cents(100+history.random.IntN(90_000)), "name:loja-aleatoria"))
			if err != nil {
				return err
			}
			state, split := history.randomReferences(history.linesOf(month))
			if err := authoredtest.WriteState(history.t.Context(), recorder, key, state); err != nil {
				return err
			}
			if err := authoredtest.WriteSplit(history.t.Context(), recorder, key, split); err != nil {
				return err
			}
			if history.random.IntN(2) == 0 {
				if err := authoredtest.AddTag(history.t.Context(), recorder, key, "viagem"); err != nil {
					return err
				}
			}
		}
		return nil
	})
	history.record("%d transactions indexed", count)
}

func (history *lineHistory) randomReferences(lines []authored.LineID) (authored.TransactionState, []authored.SplitPart) {
	random := history.random
	state := authored.TransactionState{Note: "nota", Reimbursable: random.IntN(2) == 0}
	if random.IntN(3) == 0 {
		state.Exclusion = authoredtest.Some("pessoal")
	}
	if random.IntN(3) == 0 {
		state.ExpectedCents = authoredtest.Some(money.Cents(100 * random.IntN(500)))
	}
	if len(lines) == 0 {
		return state, nil
	}
	pick := func() *authored.LineID { return authoredtest.Some(lines[random.IntN(len(lines))]) }
	if random.IntN(2) == 0 {
		authors := []authored.Author{authored.AuthorPerson, authored.AuthorRule, authored.AuthorModel}
		state.Line, state.ClassifiedBy, state.AskReason = pick(), authors[random.IntN(3)], random.IntN(2) == 0
		state.Confidence = authoredtest.Some(int64(random.IntN(101)))
	}
	if random.IntN(3) == 0 {
		state.PendingReason = "Baixa confiança"
		if random.IntN(2) == 0 {
			state.PendingLine, state.PendingConfidence = pick(), authoredtest.Some(int64(random.IntN(101)))
		}
	}
	if random.IntN(3) == 0 {
		state.Refund, state.RefundLine = authoredtest.Some(true), pick()
	}
	if random.IntN(3) != 0 {
		return state, nil
	}
	var split []authored.SplitPart
	for position := range 2 + random.IntN(2) {
		part := authored.SplitPart{Position: int64(position), Cents: money.Cents(100 * (1 + random.IntN(50)))}
		if random.IntN(4) != 0 {
			part.Line = pick()
		}
		split = append(split, part)
	}
	return state, split
}

// randomDocument is the budget in force for month with one to three random
// edits: a line removed, added, renamed, moved to another group or given
// another amount.
func (history *lineHistory) randomDocument(month brday.Month) (authored.Document, string) {
	document := authoredtest.DocumentOf(history.budget(month))
	var edits []string
	for range 1 + history.random.IntN(3) {
		history.names++
		group := &document[history.random.IntN(len(document))]
		if len(group.Lines) == 0 || history.random.IntN(5) == 0 {
			group.Lines = append(group.Lines, authoredtest.Line(fmt.Sprintf("Linha %d", history.names), 10_000))
			edits = append(edits, "a line added to "+group.Key)
			continue
		}
		at := history.random.IntN(len(group.Lines))
		line := group.Lines[at]
		switch history.random.IntN(4) {
		case 0:
			group.Lines = slices.Delete(group.Lines, at, at+1)
			edits = append(edits, line.Name+" removed")
		case 1:
			group.Lines[at].Name = fmt.Sprintf("Nome %d", history.names)
			edits = append(edits, line.Name+" renamed")
		case 2:
			group.Lines = slices.Delete(group.Lines, at, at+1)
			target := &document[history.random.IntN(len(document))]
			target.Lines = append(target.Lines, line)
			edits = append(edits, line.Name+" moved to "+target.Key)
		default:
			group.Lines[at].Cents += 100
			edits = append(edits, line.Name+" given another amount")
		}
	}
	return document, strings.Join(edits, ", ")
}

// saveAt saves a random document for month and checks the rule against what
// the save leaves. A closed month is refused and must change nothing.
func (history *lineHistory) saveAt(month brday.Month) (authored.ChangeID, bool) {
	history.t.Helper()
	document, edits := history.randomDocument(month)
	history.record("a save for %s with %s", month, edits)
	before, state := history.scratch.LineReferences(history.t), history.scratch.State(history.t)
	var id authored.ChangeID
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: saveMethods[history.random.IntN(2)], At: history.at()}
	err := history.scratch.Database.Change(history.t.Context(), facts, func(recorder *authored.Recorder) error {
		if err := recorder.SaveBudget(history.t.Context(), month, document); err != nil {
			return err
		}
		var err error
		id, err = recorder.ID(history.t.Context())
		return err
	})
	switch {
	case errors.Is(err, authored.ErrMonthClosed):
		if history.scratch.State(history.t) != state {
			history.fail("a refused save changed the database or the journal")
		}
		return 0, false
	case err != nil:
		history.t.Fatalf("seed %d: %v", history.seed, err)
	}
	after := history.scratch.LineReferences(history.t)
	for _, problem := range authoredtest.ClearedProblems(before, after) {
		history.fail("%s", problem)
	}
	return id, true
}

func (history *lineHistory) closeMonth() {
	history.t.Helper()
	month := history.openMonth()
	history.record("%s closed with a version of its own", month)
	authoredtest.CloseWithItsOwnVersion(month)(history.t, history.scratch.Database, history.at())
	history.closed = append(history.closed, month)
}

func TestAfterEveryRandomSaveNoReferenceIsStrayAndEveryOtherHolds(t *testing.T) {
	for seed := uint64(1); seed <= historySeeds; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			history := newLineHistory(t, seed)
			for range 8 {
				if history.random.IntN(5) == 0 && len(history.closed) < 3 {
					history.closeMonth()
					continue
				}
				history.saveAt(history.month())
			}
			before := history.scratch.Canonical(t)
			last, saved := history.saveAt(history.openMonth())
			if !saved {
				t.Fatalf("seed %d: the last save, for an open month, was refused", seed)
			}
			if err := history.undo(last); err != nil {
				history.fail("the undo of the last save: %v", err)
			}
			if history.scratch.Canonical(t) != before {
				history.fail("the undo of the last save did not return the dump taken before it")
			}
		})
	}
}

// undoAny undoes a random change of the journal that is not undone yet, the
// history's own fixtures included. An undo that succeeds must leave no
// reference stray; one that is refused must change nothing.
func (history *lineHistory) undoAny() {
	history.t.Helper()
	var open []authoredtest.Change
	for _, change := range history.scratch.Changes(history.t) {
		if change.UndoneAt == "" {
			open = append(open, change)
		}
	}
	if len(open) == 0 {
		return
	}
	picked := open[history.random.IntN(len(open))]
	history.record("the undo of change %d (%s)", picked.ID, picked.Method)
	state := history.scratch.State(history.t)
	err := history.undo(authored.ChangeID(picked.ID))
	var stale *authored.StaleChangeError
	switch {
	case errors.As(err, &stale):
		if history.scratch.State(history.t) != state {
			history.fail("a refused undo changed the database or the journal")
		}
	case err != nil:
		history.t.Fatalf("seed %d: %v", history.seed, err)
	default:
		for _, problem := range history.scratch.LineReferences(history.t).StrayProblems() {
			history.fail("%s", problem)
		}
	}
}

func TestAfterEveryRandomUndoNoReferenceIsStrayAndARefusedOneChangesNothing(t *testing.T) {
	for seed := uint64(1); seed <= historySeeds; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			history := newLineHistory(t, seed)
			for range 12 {
				switch roll := history.random.IntN(10); {
				case roll < 4:
					history.undoAny()
				case roll == 4 && len(history.closed) < 3:
					history.closeMonth()
				default:
					history.saveAt(history.month())
				}
			}
		})
	}
}
