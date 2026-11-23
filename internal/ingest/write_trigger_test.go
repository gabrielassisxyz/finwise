package ingest_test

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ingest"
)

// A write calls the ingest when the snapshot it reads is not the one taken in;
// by the time its call gets the lock another run may have taken that snapshot
// in, and then there is nothing left to do.
func TestTheWriteTriggerTakesANewRunInAndStopsAtTheRunTakenIn(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	w.night("complete", purchase("tx-a", at(2026, time.March, 10), "MERCADO EXEMPLO", 50))

	if counts := w.run(ingest.TriggerWrite); counts.Stopped || counts.Indexed != 1 {
		t.Errorf("the write's run over a snapshot never taken in answered %+v, want the purchase evaluated", counts)
	}
	if run, found := w.lastRun(); !found || run != 1 {
		t.Errorf("the run taken in is %d (%t), want 1", run, found)
	}
	before, changes := w.everything(), len(w.scratch.Changes(t))
	if counts := w.run(ingest.TriggerWrite); !counts.Stopped || counts.Indexed != 0 {
		t.Errorf("the write's run over the run taken in answered %+v, want it stopped", counts)
	}
	if w.everything() != before || len(w.scratch.Changes(t)) != changes {
		t.Error("a write's run that stopped changed the database")
	}

	second := w.night("complete", purchase("tx-a", at(2026, time.March, 10), "MERCADO EXEMPLO", 50),
		purchase("tx-b", at(2026, time.March, 12), "PADARIA EXEMPLO", 9))
	if counts := w.run(ingest.TriggerWrite); counts.Stopped || counts.Indexed != 1 {
		t.Errorf("the write's run over the next night answered %+v, want the new purchase evaluated", counts)
	}
	if run, _ := w.lastRun(); run != second {
		t.Errorf("the run taken in is %d, want the next night's %d", run, second)
	}
}
