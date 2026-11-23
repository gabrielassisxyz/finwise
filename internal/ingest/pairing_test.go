package ingest_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// settle puts the transaction of providerID on line, as the person.
func (w *world) settle(providerID string, line authored.LineID) {
	w.t.Helper()
	key := w.mustKey(providerID)
	w.asPerson(func(recorder *authored.Recorder) error {
		return recorder.SettleLine(w.t.Context(), key, line, authored.AuthorPerson)
	})
}

func (w *world) note(providerID, text string) {
	w.t.Helper()
	key := w.mustKey(providerID)
	w.asPerson(func(recorder *authored.Recorder) error {
		return recorder.SetNote(w.t.Context(), key, text)
	})
}

// cardTax is a tax row the bank folds under the purchase of the same name: 3.5%
// of it, rounded half up.
func cardTax(id string, date time.Time, description string, reais float64) ledgertest.Transaction {
	return cardRow(id, date, description, reais, "DEBIT", "OUTROS")
}

func TestAPostedRowKeepsEverythingThePersonDidToThePendingOne(t *testing.T) {
	cases := map[string][]ledgertest.Transaction{
		"alone": {purchase("tx-posted", at(2026, time.March, 12), "PADARIA EXEMPLO", 45.90)},
		"with a tax row folded under it": {
			purchase("tx-posted", at(2026, time.March, 12), "PADARIA EXEMPLO", 45.90),
			cardTax("tx-posted-tax", at(2026, time.March, 12), "PADARIA EXEMPLO", 1.61),
		},
	}
	for name, posted := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t, createdOn)
			w.saveBudget("2026-03", marketBudget())
			w.night("complete", pendingPurchase("tx-pending", at(2026, time.March, 10), "PADARIA EXEMPLO", 45.90))
			w.run(ingest.TriggerTick)
			mercado := w.line("2026-03", "Mercado")
			w.settle("tx-pending", mercado)
			canonical, ruleChanges := w.scratch.Canonical(t), len(w.ruleChanges())

			w.night("complete", posted...)
			wantCounts(t, "the night the charge posts", w.run(ingest.TriggerTick), ingest.Counts{Paired: 1})
			if pending, postedKey := w.mustKey("tx-pending"), w.mustKey("tx-posted"); pending != postedKey {
				t.Fatalf("the posted row has key %d, the pending one %d", postedKey, pending)
			}
			row := w.indexRow("tx-posted")
			if row.Day != "2026-03-12" || row.Cents != -4590 || row.BankPending || !row.Evaluated {
				t.Errorf("the index row is %+v, want the posted day and amount, no longer pending, still evaluated", row)
			}
			if state := w.kept("tx-posted").State; state.Line == nil || *state.Line != mercado || state.ClassifiedBy != authored.AuthorPerson {
				t.Errorf("the posted row resolves to %+v, want the person's line", state)
			}
			if w.scratch.Canonical(t) != canonical {
				t.Error("pairing rewrote what was authored")
			}
			if len(w.ruleChanges()) != ruleChanges {
				t.Error("pairing wrote a change of the rule")
			}
		})
	}
}

func TestAMatchThatIsNotOneToOnePairsNothing(t *testing.T) {
	loja := func(id string, day int, reais float64, pending bool) ledgertest.Transaction {
		if pending {
			return pendingPurchase(id, at(2026, time.March, day), "LOJA EXEMPLO", reais)
		}
		return purchase(id, at(2026, time.March, day), "LOJA EXEMPLO", reais)
	}
	cases := []struct {
		name            string
		pending, posted []ledgertest.Transaction
	}{
		{"one pending, two posted", []ledgertest.Transaction{loja("p-1", 10, 30, true)},
			[]ledgertest.Transaction{loja("q-1", 11, 30, false), loja("q-2", 12, 30, false)}},
		{"two pending, one posted", []ledgertest.Transaction{loja("p-1", 10, 30, true), loja("p-2", 11, 30, true)},
			[]ledgertest.Transaction{loja("q-1", 12, 30, false)}},
		{"no posted with the same amount", []ledgertest.Transaction{loja("p-1", 10, 30, true)},
			[]ledgertest.Transaction{loja("q-1", 11, 31, false)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t, createdOn)
			w.saveBudget("2026-03", marketBudget())
			w.night("complete", c.pending...)
			w.run(ingest.TriggerTick)
			before := map[string]authored.AuthoredTransaction{}
			for _, row := range c.pending {
				w.note(row.ID, "nota de "+row.ID)
				before[row.ID] = w.kept(row.ID)
			}

			w.night("complete", c.posted...)
			counts := w.run(ingest.TriggerTick)
			if counts.Paired != 0 || counts.Unpaired != len(c.pending) || counts.Indexed != len(c.posted) {
				t.Errorf("counts %+v, want nothing paired, %d unpaired and %d evaluated", counts, len(c.pending), len(c.posted))
			}
			for _, row := range c.pending {
				if got := w.kept(row.ID); !reflect.DeepEqual(got, before[row.ID]) {
					t.Errorf("%s holds %+v, want %+v", row.ID, got, before[row.ID])
				}
				for _, other := range c.posted {
					if w.mustKey(other.ID) == w.mustKey(row.ID) {
						t.Errorf("%s was mapped to the key of %s", other.ID, row.ID)
					}
				}
			}
			for _, row := range c.posted {
				if !w.indexRow(row.ID).Evaluated {
					t.Errorf("%s was not evaluated as a new transaction", row.ID)
				}
			}
		})
	}
}

// documentWithout is month's budget as a document, without the line named name.
func (w *world) documentWithout(month brday.Month, name string) authored.Document {
	w.t.Helper()
	view := w.database().View()
	defer view.Close()
	budget, err := view.BudgetOf(w.t.Context(), month)
	if err != nil {
		w.t.Fatal(err)
	}
	document := authoredtest.DocumentOf(budget)
	for i := range document {
		document[i].Lines = slices.DeleteFunc(document[i].Lines, func(line authored.DocumentLine) bool { return line.Name == name })
	}
	return document
}

func TestAChargeThatMovesToAMonthWithoutItsLineLosesTheLineInTheRulesChange(t *testing.T) {
	moves := map[string]struct{ first, next ledgertest.Transaction }{
		"posted under a new id in April": {
			pendingPurchase("tx-1", at(2026, time.March, 31), "MERCADO EXEMPLO", 80),
			purchase("tx-2", at(2026, time.April, 2), "MERCADO EXEMPLO", 80),
		},
		"dated again in April by the bank": {
			purchase("tx-1", at(2026, time.March, 31), "MERCADO EXEMPLO", 80),
			purchase("tx-1", at(2026, time.April, 1), "MERCADO EXEMPLO", 80),
		},
	}
	for name, move := range moves {
		for _, aprilHasIt := range []bool{false, true} {
			t.Run(name+map[bool]string{false: ", April lacks the line", true: ", April has it"}[aprilHasIt], func(t *testing.T) {
				w := newWorld(t, createdOn)
				w.saveBudget("2026-03", marketBudget())
				w.night("complete", move.first)
				w.run(ingest.TriggerTick)
				mercado := w.line("2026-03", "Mercado")
				w.settle("tx-1", mercado)
				if !aprilHasIt {
					w.saveBudget("2026-04", w.documentWithout("2026-03", "Mercado"))
				}
				canonical, ruleChanges := w.scratch.Canonical(t), len(w.ruleChanges())

				w.night("complete", move.next)
				w.run(ingest.TriggerTick)
				if day := w.indexRow("tx-1").Day; monthOf(day) != "2026-04" {
					t.Fatalf("the index row is dated %s, want April", day)
				}
				state := w.kept("tx-1").State
				if aprilHasIt {
					if state.Line == nil || *state.Line != mercado || w.scratch.Canonical(t) != canonical || len(w.ruleChanges()) != ruleChanges {
						t.Errorf("a line April has was touched: %+v, %d changes of the rule", state, len(w.ruleChanges()))
					}
					return
				}
				if state.Line != nil || state.ClassifiedBy != "" || state.Confidence != nil || state.AskReason {
					t.Errorf("the line April lacks stayed: %+v", state)
				}
				changes := w.ruleChanges()
				if len(changes) != ruleChanges+1 || changes[len(changes)-1].Method != ingest.Method {
					t.Errorf("the clearing is not one change of the rule: %+v", changes)
				}
			})
		}
	}
}

func monthOf(day brday.Day) string { return string(day[:7]) }

func TestAVanishedRowIsPairedOnTheNightItsPostedRowFirstAppears(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	w.night("complete", pendingPurchase("tx-pending", at(2026, time.March, 10), "PADARIA EXEMPLO", 12))
	w.run(ingest.TriggerTick)
	w.night("complete")
	wantCounts(t, "the night it vanishes", w.run(ingest.TriggerTick), ingest.Counts{Unpaired: 1})
	w.night("complete", purchase("tx-posted", at(2026, time.March, 15), "PADARIA EXEMPLO", 12))
	wantCounts(t, "the night its posted row appears", w.run(ingest.TriggerTick), ingest.Counts{Paired: 1})
	if w.mustKey("tx-posted") != w.mustKey("tx-pending") {
		t.Error("the posted row was not mapped to the pending row's key")
	}
}

func TestAPostedRowTheIndexAlreadyHoldsIsNeverPaired(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	w.night("complete",
		pendingPurchase("tx-pending", at(2026, time.March, 10), "PADARIA EXEMPLO", 12),
		purchase("tx-posted", at(2026, time.March, 11), "PADARIA EXEMPLO", 12))
	w.run(ingest.TriggerTick)
	w.night("complete", purchase("tx-posted", at(2026, time.March, 11), "PADARIA EXEMPLO", 12))
	wantCounts(t, "the night the pending row vanishes", w.run(ingest.TriggerTick), ingest.Counts{Unpaired: 1})
	if w.mustKey("tx-posted") == w.mustKey("tx-pending") {
		t.Error("a posted row the index already held was paired")
	}
}

func TestAVanishedRowIsCountedOnceAndComesBackAsItWas(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	pending := pendingPurchase("tx-pending", at(2026, time.March, 10), "PADARIA EXEMPLO", 12)
	w.night("complete", pending)
	w.run(ingest.TriggerTick)
	w.note("tx-pending", "comprei pão")
	kept := w.kept("tx-pending")

	w.night("complete")
	wantCounts(t, "the night it vanishes", w.run(ingest.TriggerTick), ingest.Counts{Unpaired: 1})
	wantCounts(t, "the command over the same run", w.run(ingest.TriggerCommand), ingest.Counts{})
	w.night("complete")
	wantCounts(t, "the next night", w.run(ingest.TriggerTick), ingest.Counts{})
	w.night("complete", pending)
	wantCounts(t, "the night it is listed again", w.run(ingest.TriggerTick), ingest.Counts{})
	if got := w.kept("tx-pending"); !reflect.DeepEqual(got, kept) {
		t.Errorf("listed again it holds %+v, want %+v", got, kept)
	}
	if row := w.indexRow("tx-pending"); !row.Evaluated || !row.BankPending {
		t.Errorf("listed again its index row is %+v", row)
	}
}

func TestARowNoRunListedIsCountedUnpairedOnceWhenAFirstRunMeetsItGone(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	w.asPerson(func(recorder *authored.Recorder) error {
		seen := authoredtest.Seen("tx-pending", "2026-03-10", -1_200, "text:PADARIA EXEMPLO")
		seen.BankPending = true
		key, err := recorder.IndexTransaction(t.Context(), seen)
		if err != nil {
			return err
		}
		return recorder.SetNote(t.Context(), key, "vista no app antes de a noite passar")
	})
	w.night("complete")
	wantCounts(t, "the first run", w.run(ingest.TriggerTick), ingest.Counts{Unpaired: 1})
	wantCounts(t, "the command over it", w.run(ingest.TriggerCommand), ingest.Counts{})
	w.night("complete")
	wantCounts(t, "the next night", w.run(ingest.TriggerTick), ingest.Counts{})
}

// Once paired, a key can be listed under both of its ids, when the bank lists
// the pending row again beside the posted one; the index keeps the posted
// row's facts.
func TestAKeyListedUnderBothItsIDsKeepsThePostedRowsFacts(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	pending := pendingPurchase("tx-pending", at(2026, time.March, 10), "PADARIA EXEMPLO", 12)
	posted := purchase("tx-posted", at(2026, time.March, 12), "PADARIA EXEMPLO", 12)
	w.night("complete", pending)
	w.run(ingest.TriggerTick)
	w.night("complete", posted)
	wantCounts(t, "the night it posts", w.run(ingest.TriggerTick), ingest.Counts{Paired: 1})
	w.night("complete", pending, posted)
	wantCounts(t, "the night both are listed", w.run(ingest.TriggerTick), ingest.Counts{})
	if row := w.indexRow("tx-posted"); row.BankPending || row.Day != "2026-03-12" {
		t.Errorf("the index row went back to the pending row's facts: %+v", row)
	}
}

// Only a posted row replaces a pending one: a charge the bank still reports
// pending is a new transaction of its own, never the continuation of another
// pending charge that vanished. The rows are a card fee, which the rules store
// nothing for, so what a row holds is only what was carried to it.
func TestAPendingRowNeverTakesTheKeyOfAVanishedPendingRow(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	fee := func(id string, day int, status string) ledgertest.Transaction {
		row := cardRow(id, at(2026, time.March, day), "ANUIDADE EXEMPLO", 25, "DEBIT", "TARIFA")
		row.Status = status
		return row
	}
	w.night("complete", fee("tx-first", 10, "PENDING"))
	wantCounts(t, "the night the first charge appears", w.run(ingest.TriggerTick), ingest.Counts{Indexed: 1, NothingToStore: 1})
	w.note("tx-first", "contestar a anuidade")
	said := w.kept("tx-first")

	w.night("complete", fee("tx-second", 11, "PENDING"))
	wantCounts(t, "the night a second pending charge appears", w.run(ingest.TriggerTick),
		ingest.Counts{Indexed: 1, Unpaired: 1, NothingToStore: 1})
	if w.mustKey("tx-second") == w.mustKey("tx-first") {
		t.Fatal("the new pending row took the key of the vanished one")
	}
	if kept := w.kept("tx-second"); !reflect.DeepEqual(kept, authored.AuthoredTransaction{}) {
		t.Errorf("the new pending row holds %+v, want nothing", kept)
	}
	if !reflect.DeepEqual(w.kept("tx-first"), said) {
		t.Errorf("the vanished row holds %+v, want what the person said", w.kept("tx-first"))
	}

	// The second charge posts under a third id. Both vanished pending rows are
	// within five days of it, so the match is not one to one and the rules pair
	// nothing: the posted row is a new transaction, and only the second pending
	// row is counted unpaired, the first having been counted the night before.
	w.night("complete", fee("tx-posted", 12, "POSTED"))
	wantCounts(t, "the night it posts", w.run(ingest.TriggerTick), ingest.Counts{Indexed: 1, Unpaired: 1, NothingToStore: 1})
	posted := w.mustKey("tx-posted")
	if posted == w.mustKey("tx-first") || posted == w.mustKey("tx-second") {
		t.Error("the posted row was paired although two vanished pending rows match it")
	}
	if !reflect.DeepEqual(w.kept("tx-first"), said) {
		t.Errorf("the first row holds %+v, want what the person said", w.kept("tx-first"))
	}
}
