package ingest_test

import (
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

func TestASecondRunOverTheSameSnapshotChangesNothing(t *testing.T) {
	w := outcomeWorld(t)
	wantCounts(t, "the first run", w.run(ingest.TriggerTick), outcomeCounts)
	before, changes := w.everything(), len(w.scratch.Changes(t))
	wantCounts(t, "the command right after it", w.run(ingest.TriggerCommand), ingest.Counts{})
	if len(w.scratch.Changes(t)) != changes {
		t.Error("the second run wrote a change")
	}
	if w.everything() != before {
		t.Error("the second run changed the index, meta or what was authored")
	}
}

// sum adds two runs' counts, leaving out whether either stopped.
func sum(a, b ingest.Counts) ingest.Counts {
	return ingest.Counts{
		Indexed: a.Indexed + b.Indexed, Paired: a.Paired + b.Paired, Unpaired: a.Unpaired + b.Unpaired,
		Applied: a.Applied + b.Applied, PendingLowConfidence: a.PendingLowConfidence + b.PendingLowConfidence,
		PendingUnnamedPerson:    a.PendingUnnamedPerson + b.PendingUnnamedPerson,
		PendingNewEstablishment: a.PendingNewEstablishment + b.PendingNewEstablishment,
		Left:                    a.Left + b.Left, NothingToStore: a.NothingToStore + b.NothingToStore,
		AlreadyDecided: a.AlreadyDecided + b.AlreadyDecided,
	}
}

// Both runs read the ledger and pass the stop of step 1 before either takes
// the lock, so the one that waits decides from what it reads under the lock.
func TestTwoRunsAtOnceWriteWhatOneRunWrites(t *testing.T) {
	for _, trigger := range []ingest.Trigger{ingest.TriggerTick, ingest.TriggerCommand} {
		alone := outcomeWorld(t)
		one := alone.run(trigger)
		want := alone.everything()

		w := outcomeWorld(t)
		second, err := authored.Open(t.Context(), w.scratch.Path, authored.Options{Now: func() time.Time { return createdOn }})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { second.Close() })
		var bothRead, done sync.WaitGroup
		bothRead.Add(2)
		var counts [2]ingest.Counts
		var errs [2]error
		for i, handle := range []*authored.Database{w.database(), second} {
			done.Go(func() {
				sources := ingest.Sources{Ledger: ledger.New(w.ledgerPath), Database: handle}
				counts[i], errs[i] = ingest.RunWithSeam(t.Context(), sources, trigger, runInstant, func() {
					bothRead.Done()
					bothRead.Wait()
				})
			})
		}
		done.Wait()
		if errs[0] != nil || errs[1] != nil {
			t.Fatalf("trigger %d: the runs failed: %v, %v", trigger, errs[0], errs[1])
		}
		if got := sum(counts[0], counts[1]); got != sum(one, ingest.Counts{}) {
			t.Errorf("trigger %d: the two runs counted %+v, one run counts %+v", trigger, got, one)
		}
		if changes := w.ruleChanges(); len(changes) > 1 {
			t.Errorf("trigger %d: the two runs wrote %d changes of the rule", trigger, len(changes))
		}
		if w.everything() != want {
			t.Errorf("trigger %d: the two runs left another state than one run", trigger)
		}
	}
}
