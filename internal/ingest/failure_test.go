package ingest_test

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// sentinel is stored in the invented ledger, and no error text may carry it.
const sentinel = "SENTINELA-7F3A"

// sentinelWorld has taken one night in and has a second one waiting, so a run
// that went on would change something.
func sentinelWorld(t *testing.T) *world {
	t.Helper()
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	first := purchase("tx-"+sentinel, at(2026, time.March, 10), "LOJA "+sentinel, 10)
	w.night("complete", first)
	w.run(ingest.TriggerTick)
	w.night("complete", first, purchase("tx-2-"+sentinel, at(2026, time.March, 11), "LOJA "+sentinel, 11))
	return w
}

func onTheLedger(t *testing.T, path, statement string) {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

// wantFailure runs every trigger and wants each to fail with kind, changing
// nothing and saying nothing of the ledger.
func (w *world) wantFailure(label string, kind ingest.Failure) {
	w.t.Helper()
	before := w.everything()
	for _, trigger := range []ingest.Trigger{ingest.TriggerTick, ingest.TriggerStart, ingest.TriggerBudgetChange, ingest.TriggerCommand} {
		counts, err := ingest.Run(w.t.Context(), w.sources(), trigger, runInstant)
		if got, classified := ingest.FailureOf(err); !classified || got != kind {
			w.t.Errorf("%s, trigger %d: the run answered %v (%v), want the kind %s", label, trigger, err, got, kind)
			continue
		}
		if counts != (ingest.Counts{}) {
			w.t.Errorf("%s: a failed run counted %+v", label, counts)
		}
		if strings.Contains(err.Error(), sentinel) {
			w.t.Errorf("%s: the error's text carries a value of the ledger: %q", label, err)
		}
	}
	if w.everything() != before {
		w.t.Errorf("%s: a failed run changed the app's database", label)
	}
}

func TestAnUnavailableSnapshotChangesNothingAndIsReportedByItsKind(t *testing.T) {
	breaks := map[string]func(w *world){
		"the snapshot missing": func(w *world) {
			if err := os.Remove(w.ledgerPath); err != nil {
				w.t.Fatal(err)
			}
		},
		"a file that is not a database": func(w *world) {
			if err := os.WriteFile(w.ledgerPath, []byte(strings.Repeat("not a database "+sentinel+" ", 400)), 0o600); err != nil {
				w.t.Fatal(err)
			}
		},
		"an unknown user_version":    func(w *world) { onTheLedger(w.t, w.ledgerPath, "PRAGMA user_version = 2") },
		"the projection row deleted": func(w *world) { onTheLedger(w.t, w.ledgerPath, "DELETE FROM projection_state") },
	}
	for name, breakIt := range breaks {
		t.Run(name, func(t *testing.T) {
			w := sentinelWorld(t)
			breakIt(w)
			w.wantFailure(name, ingest.FailureSnapshotUnavailable)
		})
	}
}

func TestARowTheReaderCannotReadFailsTheRunWithoutItsID(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	w.nightWith("complete", func(f *ledgertest.Fixture) {
		f.AddTransaction(purchase("tx-"+sentinel, at(2026, time.March, 10), "LOJA EXEMPLO", 10))
		f.CorruptTransactionDate("tx-"+sentinel, "not a date")
	})
	w.wantFailure("a date that does not parse", ingest.FailureLedger)
}
