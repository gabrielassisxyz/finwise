package api

import (
	"errors"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// fullLedgerProjectedAt is the full invented ledger with its rows produced by
// run, the runs before it complete.
func fullLedgerProjectedAt(t *testing.T, run int64) *ledgertest.Fixture {
	t.Helper()
	full := newFullLedger(t)
	for last := int64(1); last < run; {
		last = full.AddRun(ledgertest.Run{Outcome: "complete", FinishedAt: time.Date(2026, time.March, 20+int(last), 6, 0, 0, 0, time.UTC)})
	}
	if run > 1 {
		full.CoverInRun(run)
		full.ProjectTo(run)
	}
	return full
}

// lookUpKeepingWhatItWrote looks id up in a change that commits whatever the
// lookup wrote, and answers what the lookup answered: a door that refused after
// it touched the index would leave that behind.
func lookUpKeepingWhatItWrote(t *testing.T, scratch *authoredtest.Scratch, fixture *ledgertest.Fixture, id string) (transactionTarget, error) {
	t.Helper()
	var target transactionTarget
	var lookup error
	err := inChange(t, scratch, fixture, func(writes Writes) error {
		target, _, lookup = lookupTarget(t.Context(), writes, id)
		return nil
	})
	if err != nil {
		t.Fatalf("the change around the lookup: %v", err)
	}
	return target, lookup
}

func TestTheIndexDoorOpensOnlyForTheRunTakenInLast(t *testing.T) {
	cases := []struct {
		name             string
		projected, taken int64
		opens            bool
	}{
		{"the run taken in", 2, 2, true},
		{"a newer snapshot", 3, 2, false},
		{"an older snapshot", 2, 3, false},
		{"no run taken in yet", 2, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			full := fullLedgerProjectedAt(t, c.projected)
			scratch := authoredtest.Open(t, fixtureClock)
			if c.taken != 0 {
				authoredtest.TakeRunIn(t, scratch.Database, c.taken)
			}
			before := scratch.Everything(t)

			target, err := lookUpKeepingWhatItWrote(t, scratch, full, txMonthEnd)

			added := linesAdded(before, scratch.Everything(t))
			if c.opens {
				if err != nil || target.key == 0 || len(added["tx_index"]) != 1 || len(added["tx_ids"]) != 1 {
					t.Errorf("over the run taken in the lookup answered key %d (%v) and added %v, want one transaction indexed", target.key, err, added)
				}
				return
			}
			if !errors.Is(err, errSnapshotNotTakenIn) || len(added) != 0 {
				t.Errorf("the lookup answered %v and added %v, want the snapshot refused and nothing written", err, added)
			}
		})
	}
}

// A snapshot that records no run lists no transaction either, so a lookup over
// one finds nothing before it reaches the door; the door is given here a row a
// projected snapshot listed and a snapshot that records no run.
func TestTheDoorRefusesASnapshotThatRecordsNoRunAsAnUnavailableLedgerAndWritesNothing(t *testing.T) {
	listing, unprojected := newFullLedger(t), newFullLedger(t)
	unproject(t, unprojected.Path)
	row, found, err := findListed(t.Context(), viewOf(t, listing.Path), txMonthEnd)
	if err != nil || !found {
		t.Fatalf("the projected snapshot does not list %s: %v", txMonthEnd, err)
	}
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.TakeRunIn(t, scratch.Database, 1)
	before := scratch.Everything(t)

	var door error
	err = inChange(t, scratch, unprojected, func(writes Writes) error {
		_, door = indexListed(t.Context(), writes.Ledger, writes.Recorder, row.listed)
		return nil
	})

	if err != nil || !ledger.IsNotProjected(door) {
		t.Errorf("the door answered %v (change %v), want the snapshot's missing run", door, err)
	}
	if kind, _ := classify(door); kind != kindLedgerUnavailable {
		t.Errorf("a snapshot that records no run is answered %s, want %s", kind, kindLedgerUnavailable)
	}
	if after := scratch.Everything(t); after != before {
		t.Errorf("the door wrote %v", linesAdded(before, after))
	}
}
