package api

import (
	"net/http"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// paydayWriteCases are the mark in the harness over the invented state: the
// transfer into the linked investment, still pending, recorded and undone; the
// investment, already marked; and a transfer's name sent as its key.
var paydayWriteCases = []writeCase{
	{method: markTransferDoneMethod, arguments: `["2026-03","pot:investment:inv-it-1"]`, expects: expectsRecorded, state: startFromTheInventedState},
	{method: markTransferDoneMethod, arguments: `["2026-03","investment"]`, expects: expectsNothing, state: startFromTheInventedState},
	{method: markTransferDoneMethod, arguments: `["2026-03","LCI Exemplo"]`, expects: expectsNothing, state: startFromTheInventedState},
}

func (env *paydayEnv) mark(month, key string) string {
	env.t.Helper()
	return env.change(markTransferDoneMethod, `["`+month+`","`+key+`"]`)
}

// expectNothingMarked posts a mark that must answer null and leave the journal
// as it was.
func (env *paydayEnv) expectNothingMarked(month, key string) {
	env.t.Helper()
	counts := journalCounts(env.t, env.scratch)
	status, body := env.answer(markTransferDoneMethod, `["`+month+`","`+key+`"]`)
	if status != http.StatusOK || string(body) != "null" {
		env.t.Errorf("marking %q for %s answered %d %s, want null", key, month, status, body)
	}
	if after := journalCounts(env.t, env.scratch); after != counts {
		env.t.Errorf("marking %q for %s moved the journal from %v to %v", key, month, counts, after)
	}
}

func transferNamed(t *testing.T, transfers []transfer, key string) transfer {
	t.Helper()
	index := slices.IndexFunc(transfers, func(candidate transfer) bool { return candidate.Key == key })
	if index < 0 {
		t.Fatalf("no transfer under %s in %+v", key, transfers)
	}
	return transfers[index]
}

func TestMarkingAPotsTransferDatesItTodayAndLeavesTheOtherReservaPending(t *testing.T) {
	env := newPaydayEnv(t, 800_000)
	changes := len(env.scratch.Changes(t))
	id := env.mark("2026-03", "pot:"+examplePotOne)
	if got := len(env.scratch.Changes(t)); got != changes+1 {
		t.Errorf("the journal gained %d changes, want 1", got-changes)
	}
	if marks := transferMarks(t, env.scratch); !slices.Equal(marks, []string{"2026-03 pot:" + examplePotOne + " 2026-03-20"}) {
		t.Errorf("transfer_marks = %v", marks)
	}
	if effects := env.scratch.EffectsOf(t, changeNumber(t, id)); len(effects) != 1 || effects[0].Table != "transfer_marks" {
		t.Errorf("the change's effects are %+v, want one row of transfer_marks", effects)
	}
	transfers := env.transfers("2026-03")
	done, other := transferNamed(t, transfers, "pot:"+examplePotOne), transferNamed(t, transfers, "pot:"+exampleNubankPot)
	if done.State != "done" || done.DoneOn == nil || *done.DoneOn != "2026-03-20" || !done.MarkedByHand {
		t.Errorf("the marked transfer reads %+v", done)
	}
	if other.Name != "Reserva" || other.State != "pending" || other.DoneOn != nil || other.MarkedByHand {
		t.Errorf("the other Reserva reads %+v", other)
	}
}

func TestAMarkOfATransferTheMonthDoesNotListOrHasMarkedRecordsNothing(t *testing.T) {
	env := newPaydayEnv(t, 800_000)
	env.mark("2026-03", "pot:"+examplePotOne)
	env.expectNothingMarked("2026-03", "pot:"+examplePotOne)
	for _, key := range []string{"pot:nope", env.lineKey("Material escolar"), "", "Reserva", "pot:" + exampleClosedPot, "pot:" + exampleGonePot} {
		env.expectNothingMarked("2026-03", key)
	}
	poorer := newPaydayEnv(t, 500_000)
	poorer.expectNothingMarked("2026-03", "investment")
	if marks := transferMarks(t, env.scratch); len(marks) != 1 {
		t.Errorf("transfer_marks = %v, want the one mark", marks)
	}
}

func TestUndoingAMarkPutsTheTransferBackAndItCanBeMarkedAgain(t *testing.T) {
	env := newPaydayEnv(t, 800_000)
	first := env.mark("2026-03", "investment")
	env.undo(first)
	if marks := transferMarks(t, env.scratch); len(marks) != 0 {
		t.Errorf("after the undo transfer_marks = %v", marks)
	}
	if got := transferNamed(t, env.transfers("2026-03"), "investment"); got.State != "pending" || got.DoneOn != nil {
		t.Errorf("after the undo the investment reads %+v", got)
	}
	if again := env.mark("2026-03", "investment"); again == first {
		t.Errorf("marking again answered the undone change %s", first)
	}
	if status, _ := env.answer("changes.undo", `["`+first+`"]`); status != http.StatusConflict {
		t.Errorf("a second undo of the same change answered %d, want 409", status)
	}
}

// The same budget version is in force for 2026-03 and 2026-04.
func TestAMarkIsOfItsMonthAlone(t *testing.T) {
	env := newPaydayEnv(t, 800_000)
	env.mark("2026-03", "pot:"+examplePotOne)
	if got := transferNamed(t, env.transfers("2026-04"), "pot:"+examplePotOne); got.State != "pending" {
		t.Errorf("the mark of 2026-03 shows in 2026-04: %+v", got)
	}
	env.mark("2026-04", "pot:"+examplePotOne)
	want := []string{"2026-03 pot:" + examplePotOne + " 2026-03-20", "2026-04 pot:" + examplePotOne + " 2026-03-20"}
	if marks := transferMarks(t, env.scratch); !slices.Equal(marks, want) {
		t.Errorf("transfer_marks = %v, want %v", marks, want)
	}
}

func TestAMarkOfABadMonthIsABadRequestAndOfAMissingSnapshotUnavailable(t *testing.T) {
	env := newPaydayEnv(t, 800_000)
	before := env.scratch.State(t)
	expectError(t, "2026-13", env.call("/api/review/markTransferDone", `["2026-13","investment"]`, env.cookie),
		http.StatusBadRequest, "bad_request")

	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), fixtureNow), withDatabase(env.scratch.Database))
	expectError(t, "a missing snapshot", missing.call("/api/review/markTransferDone", `["2026-03","investment"]`, missing.unlock()),
		http.StatusServiceUnavailable, "ledger_unavailable")
	if after := env.scratch.State(t); after != before {
		t.Errorf("a refused mark changed the database:\n%s", differences(before, after))
	}

	// With no line linked, deriving the month's transfers asks the ledger
	// nothing, so only the write's own question finds the snapshot missing.
	unlinked := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, unlinked.Database, fixtureNow, "2026-03", cushionBudget(70_000, 10_000))
	unlinkedBefore := unlinked.State(t)
	alone := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), fixtureNow), withDatabase(unlinked.Database))
	expectError(t, "a missing snapshot and no link", alone.call("/api/review/markTransferDone", `["2026-03","investment"]`, alone.unlock()),
		http.StatusServiceUnavailable, "ledger_unavailable")
	if after := unlinked.State(t); after != unlinkedBefore {
		t.Errorf("a refused mark with no link changed the database:\n%s", differences(unlinkedBefore, after))
	}

	without := newTestEnv(t, withLedger(newPaydayLedger(t).Path, fixtureNow))
	if reads, status := readsOfBody(without, "/api/review/markTransferDone", without.unlock()); reads != 0 || status != http.StatusForbidden {
		t.Errorf("without a database the mark read its body %d times and answered %d, want 0 and 403", reads, status)
	}
}

// Nothing removes a mark but its undo: a mark whose transfer leaves the list
// stays stored and counts for nothing.
func TestAMarkStaysStoredWhenItsPotStopsCountingAndCountsForNothing(t *testing.T) {
	env := newPaydayEnv(t, 800_000)
	env.mark("2026-03", "pot:"+examplePotOne)
	for _, line := range []string{"IPTU", "Seguro do carro"} {
		authoredtest.ToggleLink(t, env.scratch.Database, fixtureNow, env.lines[line], examplePotOne)
	}
	transfers := env.transfers("2026-03")
	for _, line := range []string{"IPTU", "Seguro do carro"} {
		if got := transferNamed(t, transfers, env.lineKey(line)); got.State != "pending" || got.DoneOn != nil {
			t.Errorf("%s, unlinked, reads %+v", line, got)
		}
	}
	if slices.ContainsFunc(transfers, func(candidate transfer) bool { return candidate.Key == "pot:"+examplePotOne }) {
		t.Error("the pot's transfer is still listed with no line linked to it")
	}
	if marks := transferMarks(t, env.scratch); !slices.Equal(marks, []string{"2026-03 pot:" + examplePotOne + " 2026-03-20"}) {
		t.Errorf("transfer_marks = %v, want the mark kept", marks)
	}
}
