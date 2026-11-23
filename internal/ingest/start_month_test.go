package ingest_test

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func TestNothingBeforeTheTrackingStartMonthIsEvaluated(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-01", marketBudget())
	mercado := w.line("2026-02", "Mercado")
	rows := []ledgertest.Transaction{
		// 23:30 of 28 February in Brazil, and the first minute of 1 March.
		purchase("tx-february", time.Date(2026, time.March, 1, 2, 30, 0, 0, time.UTC), "PADARIA EXEMPLO", 10),
		purchase("tx-march", time.Date(2026, time.March, 1, 3, 0, 0, 0, time.UTC), "PADARIA EXEMPLO", 11),
		purchase("tx-by-hand", at(2026, time.February, 10), "LOJA EXEMPLO", 12),
	}
	w.asPerson(func(recorder *authored.Recorder) error {
		key, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen("tx-by-hand", "2026-02-10", -1_200, "text:LOJA EXEMPLO"))
		if err != nil {
			return err
		}
		return recorder.SettleLine(t.Context(), key, mercado, authored.AuthorPerson)
	})
	for night := range 2 {
		w.night("complete", rows...)
		counts := w.run(ingest.TriggerTick)
		if want := 1 - night; counts.Indexed != want {
			t.Errorf("night %d evaluated %d transactions, want %d", night+1, counts.Indexed, want)
		}
	}
	if _, found := w.key("tx-february"); found {
		t.Error("a transaction of 28 February was indexed")
	}
	if row := w.indexRow("tx-march"); !row.Evaluated {
		t.Errorf("the transaction of 1 March was not evaluated: %+v", row)
	}
	if row := w.indexRow("tx-by-hand"); row.Evaluated || row.Day != "2026-02-10" {
		t.Errorf("the transaction of February the person put on a line reads %+v, want it unevaluated", row)
	}
	if kept := w.kept("tx-by-hand"); kept.State.Line == nil || *kept.State.Line != mercado || len(kept.History) != 0 {
		t.Errorf("the transaction of February the person put on a line holds %+v", kept)
	}
}

func TestATransactionOfAMonthWithoutALineWaitsForOne(t *testing.T) {
	cases := map[string]func(w *world){
		"no budget": func(*world) {},
		"a budget of five empty groups": func(w *world) {
			empty := authored.Document{}
			for _, key := range authored.GroupKeys {
				empty = append(empty, authoredtest.Group(key))
			}
			w.saveBudget("2026-03", empty)
		},
	}
	for name, budget := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t, createdOn)
			budget(w)
			for range 2 {
				w.night("complete", purchase("tx-march", at(2026, time.March, 10), "PADARIA EXEMPLO", 10))
				if counts := w.run(ingest.TriggerTick); counts.Indexed != 0 || counts.Stopped {
					t.Errorf("a tick evaluated or stopped: %+v", counts)
				}
				if _, found := w.key("tx-march"); found {
					t.Fatal("a transaction of a month without a line was indexed")
				}
			}
			w.saveBudget("2026-03", marketBudget())
			if counts := w.run(ingest.TriggerBudgetChange); counts.Indexed != 1 {
				t.Errorf("the run after the budget gained a line evaluated %d transactions, want 1", counts.Indexed)
			}
			if row := w.indexRow("tx-march"); !row.Evaluated {
				t.Errorf("the transaction reads %+v after its month gained a line", row)
			}
		})
	}
}
