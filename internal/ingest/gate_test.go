package ingest_test

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// gateWorld is a world over one ledger that a test advances in place, run by
// run, with a budget for March and one purchase of March.
func gateWorld(t *testing.T) (*world, *ledgertest.Fixture) {
	t.Helper()
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	f := ledgertest.NewAt(t, w.ledgerPath)
	addAccounts(f)
	f.AddTransaction(purchase("tx-mercado", at(2026, time.March, 10), "MERCADO EXEMPLO", 50))
	return w, f
}

// addRun adds a run that ended outcome, with scopes of its own, and projects it
// when asked.
func addRun(f *ledgertest.Fixture, outcome string, day int, project bool) int64 {
	run := f.AddRun(ledgertest.Run{Outcome: outcome, FinishedAt: at(2026, time.March, day)})
	if project {
		f.CoverInRun(run)
		f.ProjectTo(run)
	}
	return run
}

// tick runs as the timer and checks whether it stopped and which run is taken in.
func (w *world) tick(label string, wantStopped bool, wantRun int64) {
	w.t.Helper()
	changes := len(w.scratch.Changes(w.t))
	counts := w.run(ingest.TriggerTick)
	if counts.Stopped != wantStopped {
		w.t.Errorf("%s: stopped is %t, want %t", label, counts.Stopped, wantStopped)
	}
	if run, found := w.lastRun(); !found || run != wantRun {
		w.t.Errorf("%s: the run taken in is %d (%t), want %d", label, run, found, wantRun)
	}
	if wantStopped && len(w.scratch.Changes(w.t)) != changes {
		w.t.Errorf("%s: a run that stopped wrote a change", label)
	}
}

func TestATickTakesInEachProjectedRunOnceWhateverItsOutcome(t *testing.T) {
	for _, order := range [][2]string{{"partial", "complete"}, {"complete", "partial"}} {
		t.Run(order[0]+" then "+order[1], func(t *testing.T) {
			w, f := gateWorld(t)
			w.tick("the first tick", false, 1)
			if counts := w.run(ingest.TriggerTick); !counts.Stopped || counts.Indexed != 0 {
				t.Errorf("a second tick on run 1 answered %+v, want it stopped", counts)
			}
			if len(w.ruleChanges()) != 1 {
				t.Errorf("the rule wrote %d changes, want the first tick's one", len(w.ruleChanges()))
			}

			second := addRun(f, order[0], 17, true)
			w.tick("a tick on the projected run "+order[0], false, second)
			third := addRun(f, order[1], 18, false)
			w.tick("a tick while the "+order[1]+" run is not projected", true, second)
			f.CoverInRun(third)
			f.ProjectTo(third)
			w.tick("a tick once the "+order[1]+" run is projected", false, third)
			w.tick("a tick on it again", true, third)
		})
	}
}

func TestAFailedRunThatIsNotProjectedChangesNothing(t *testing.T) {
	w, f := gateWorld(t)
	w.run(ingest.TriggerTick)
	before := w.everything()
	addRun(f, "failed", 17, false)
	w.tick("a tick after the failed run", true, 1)
	if w.everything() != before {
		t.Error("a tick after a failed run that was not projected changed the app's database")
	}
}

func TestABudgetChangeAndTheCommandGoOnOverTheRunTakenIn(t *testing.T) {
	for _, trigger := range []ingest.Trigger{ingest.TriggerBudgetChange, ingest.TriggerCommand} {
		w, _ := gateWorld(t)
		w.run(ingest.TriggerStart)
		if counts := w.run(ingest.TriggerStart); !counts.Stopped {
			t.Errorf("a second start on the run taken in went on: %+v", counts)
		}
		if counts := w.run(trigger); counts.Stopped {
			t.Errorf("trigger %d stopped over the run taken in", trigger)
		}
		if run, _ := w.lastRun(); run != 1 {
			t.Errorf("trigger %d left run %d taken in", trigger, run)
		}
	}
}

func TestASnapshotOlderThanTheRunTakenInIsNeverTakenIn(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	w.night("complete", purchase("tx-a", at(2026, time.March, 10), "MERCADO EXEMPLO", 50))
	w.night("complete", purchase("tx-a", at(2026, time.March, 11), "MERCADO EXEMPLO", 50))
	w.run(ingest.TriggerTick)
	before := w.everything()

	w.nights = 0
	w.night("complete", purchase("tx-a", at(2026, time.March, 10), "MERCADO EXEMPLO", 50),
		purchase("tx-b", at(2026, time.March, 12), "PADARIA EXEMPLO", 9))
	for _, trigger := range []ingest.Trigger{ingest.TriggerTick, ingest.TriggerBudgetChange, ingest.TriggerCommand} {
		if counts := w.run(trigger); !counts.Stopped {
			t.Errorf("trigger %d took in an older snapshot: %+v", trigger, counts)
		}
	}
	if w.everything() != before {
		t.Error("an older snapshot moved the index, meta or what was authored")
	}
}

// A caller that read the older snapshot takes the lock only after another took
// a newer one in; it must find that out under the lock and move nothing back.
func TestACallerHoldingAnOlderSnapshotMovesNothingBackUnderTheLock(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	w.night("complete", purchase("tx-a", at(2026, time.March, 10), "MERCADO EXEMPLO", 50))
	w.run(ingest.TriggerTick)
	w.night("complete", purchase("tx-a", at(2026, time.March, 10), "MERCADO EXEMPLO", 50))
	var newer string
	takeTheNewerOneIn := func() {
		w.night("complete", purchase("tx-a", at(2026, time.March, 11), "MERCADO EXEMPLO", 50),
			purchase("tx-b", at(2026, time.March, 12), "PADARIA EXEMPLO", 9))
		w.run(ingest.TriggerTick)
		newer = w.everything()
	}
	counts, err := ingest.RunWithSeam(t.Context(), w.sources(), ingest.TriggerCommand, runInstant, takeTheNewerOneIn)
	if err != nil {
		t.Fatal(err)
	}
	if !counts.Stopped {
		t.Errorf("the caller holding the older snapshot went on: %+v", counts)
	}
	if run, _ := w.lastRun(); run != 3 {
		t.Errorf("the run taken in is %d, want the newer 3", run)
	}
	if w.everything() != newer {
		t.Error("the caller holding the older snapshot moved the index, meta or what was authored")
	}
}
