package ingest_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// The establishments of the outcome ledger, by the key the rules give them.
const (
	defaultShop   = "text:PADARIA PADRAO"
	exampledShop  = "text:MERCADO EXEMPLO"
	contestedShop = "text:FEIRA EXEMPLO"
)

// outcomeRows hold one transaction for each outcome of the rules.
func outcomeRows() []ledgertest.Transaction {
	day := func(n int) time.Time { return at(2026, time.March, n) }
	bill := ledgertest.Transaction{ID: "tx-bill", AccountID: bankAccount, Date: day(11), Description: "PAGAMENTO DE FATURA CARTAO",
		Amount: -500, Currency: "BRL", Type: "DEBIT", Raw: map[string]any{"operationType": "OUTROS"}}
	unconverted := cardRow("tx-unconverted", day(12), "HOTEL EXEMPLO", 40, "DEBIT", "PAGAMENTO")
	unconverted.Currency = "EUR"
	return []ledgertest.Transaction{
		pix("tx-own-transfer", day(10), "PIX ENVIADO PESSOA EXEMPLO", -300, "Pessoa Exemplo", ownDocument),
		bill,
		unconverted,
		cardRow("tx-refund", day(13), "ESTORNO LIVRARIA EXEMPLO", 30, "CREDIT", "ESTORNO"),
		cardRow("tx-fee", day(14), "ANUIDADE EXEMPLO", 25, "DEBIT", "TARIFA"),
		cardRow("tx-discount", day(14), "DESC ANTECIPA PARCELAS", 5, "CREDIT", "OUTROS"),
		purchase("tx-default", day(15), "PADARIA PADRAO", 18),
		purchase("tx-examples", day(15), "MERCADO EXEMPLO", 120),
		purchase("tx-contested", day(16), "FEIRA EXEMPLO", 60),
		pix("tx-person", day(16), "PIX ENVIADO MARIA EXEMPLO", -80, "Maria Exemplo", "11122233344"),
		purchase("tx-never-seen", day(17), "LIVRARIA EXEMPLO", 70),
		pix("tx-received", day(18), "PIX RECEBIDO JOAO EXEMPLO", 150, "Joao Exemplo", "55566677788"),
	}
}

// outcomeWorld is a ledger of outcomeRows over a budget for March, with a
// default line for one shop, five examples on one line for another and five of
// six for a third.
func outcomeWorld(t *testing.T) *world {
	t.Helper()
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	mercado, delivery := w.line("2026-03", "Mercado"), w.line("2026-03", "Delivery")
	w.asPerson(func(recorder *authored.Recorder) error {
		return recorder.SetDefaultLine(t.Context(), defaultShop, delivery)
	})
	w.examples(exampledShop, mercado, 5, "example-")
	w.examples(contestedShop, mercado, 5, "contested-")
	w.examples(contestedShop, delivery, 1, "contested-other-")
	w.night("complete", outcomeRows()...)
	return w
}

var outcomeCounts = ingest.Counts{
	Indexed: 12, Applied: 2, PendingLowConfidence: 1, PendingUnnamedPerson: 2, PendingNewEstablishment: 1,
	Left: 4, NothingToStore: 2,
}

func TestEachOutcomeOfTheRulesIsWhatTheRunWrites(t *testing.T) {
	w := outcomeWorld(t)
	before := w.scratch.Canonical(t)
	counts := w.run(ingest.TriggerTick)
	wantCounts(t, "the run", counts, outcomeCounts)
	if counts.Outcomes() != counts.Indexed {
		t.Errorf("%d transactions were evaluated and %d outcomes counted", counts.Indexed, counts.Outcomes())
	}

	mercado, delivery := w.line("2026-03", "Mercado"), w.line("2026-03", "Delivery")
	w.wantApplied("tx-default", delivery, "Delivery")
	w.wantApplied("tx-examples", mercado, "Mercado")
	w.wantPending("tx-contested", "Baixa confiança", &mercado, 83)
	w.wantPending("tx-person", "PIX para pessoa sem apelido", nil, 0)
	w.wantPending("tx-received", "PIX para pessoa sem apelido", nil, 0)
	w.wantPending("tx-never-seen", "Estabelecimento novo", nil, 0)
	for _, id := range []string{"tx-own-transfer", "tx-bill", "tx-unconverted", "tx-refund", "tx-fee", "tx-discount"} {
		if kept := w.kept(id); !reflect.DeepEqual(kept, authored.AuthoredTransaction{}) {
			t.Errorf("%s holds %+v, want nothing authored", id, kept)
		}
	}
	for _, row := range outcomeRows() {
		if !w.indexRow(row.ID).Evaluated {
			t.Errorf("%s is not marked evaluated", row.ID)
		}
	}

	changes := w.ruleChanges()
	if len(changes) != 1 || changes[0].Method != ingest.Method {
		t.Fatalf("the rule wrote %+v, want one change", changes)
	}
	if err := w.database().Undo(t.Context(), authored.ChangeID(changes[0].ID), runInstant.Add(time.Hour)); err != nil {
		t.Fatalf("undo the run's change: %v", err)
	}
	if w.scratch.Canonical(t) != before {
		t.Error("undoing the run's change left something it authored")
	}
	if !w.indexRow("tx-examples").Evaluated {
		t.Error("undoing the run's change cleared what the rules met")
	}
}

func (w *world) wantApplied(providerID string, line authored.LineID, name string) {
	w.t.Helper()
	kept := w.kept(providerID)
	state := kept.State
	if state.Line == nil || *state.Line != line || state.ClassifiedBy != authored.AuthorRule || state.Confidence != nil ||
		state.PendingReason != "" || state.PendingLine != nil || state.AskReason {
		w.t.Errorf("%s holds %+v, want line %d by the rule and nothing pending", providerID, state, line)
	}
	if len(kept.History) != 1 || kept.History[0].Kind != authored.StepEffect || kept.History[0].Text != ingest.RuleStepPrefix+name {
		w.t.Errorf("%s has the history %+v, want the one step of the rule naming %s", providerID, kept.History, name)
	}
}

func (w *world) wantPending(providerID, reason string, suggestion *authored.LineID, confidence int64) {
	w.t.Helper()
	kept := w.kept(providerID)
	state := kept.State
	if state.PendingReason != reason || state.Line != nil || len(kept.History) != 0 {
		w.t.Errorf("%s holds %+v with history %+v, want it pending for %q", providerID, state, kept.History, reason)
	}
	if suggestion == nil {
		if state.PendingLine != nil || state.PendingConfidence != nil {
			w.t.Errorf("%s carries a suggestion: %+v", providerID, state)
		}
		return
	}
	if state.PendingLine == nil || *state.PendingLine != *suggestion || state.PendingConfidence == nil || *state.PendingConfidence != confidence {
		w.t.Errorf("%s suggests %v at %v, want %d at %d", providerID, state.PendingLine, state.PendingConfidence, *suggestion, confidence)
	}
}

func TestWhatThePersonDecidedBeforeTheLockIsLeftAlone(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	mercado, delivery := w.line("2026-03", "Mercado"), w.line("2026-03", "Delivery")
	rows := []ledgertest.Transaction{
		purchase("tx-on-a-line", at(2026, time.March, 10), "PADARIA PADRAO", 10),
		purchase("tx-in-review", at(2026, time.March, 11), "PADARIA PADRAO", 11),
		purchase("tx-just-now", at(2026, time.March, 12), "PADARIA PADRAO", 12),
	}
	w.night("complete", rows...)
	w.asPerson(func(recorder *authored.Recorder) error {
		if err := recorder.SetDefaultLine(t.Context(), defaultShop, delivery); err != nil {
			return err
		}
		online, err := recorder.IndexTransaction(t.Context(), seenOf(rows[0]))
		if err != nil {
			return err
		}
		if err := recorder.SettleLine(t.Context(), online, mercado, authored.AuthorPerson); err != nil {
			return err
		}
		inReview, err := recorder.IndexTransaction(t.Context(), seenOf(rows[1]))
		if err != nil {
			return err
		}
		return recorder.SetPendingReview(t.Context(), inReview, "Estabelecimento novo", nil, nil)
	})
	betweenReadAndLock := func() {
		w.asPerson(func(recorder *authored.Recorder) error {
			key, err := recorder.IndexTransaction(t.Context(), seenOf(rows[2]))
			if err != nil {
				return err
			}
			return recorder.SettleLine(t.Context(), key, mercado, authored.AuthorPerson)
		})
	}
	counts, err := ingest.RunWithSeam(t.Context(), w.sources(), ingest.TriggerTick, runInstant, betweenReadAndLock)
	if err != nil {
		t.Fatal(err)
	}
	wantCounts(t, "the run", counts, ingest.Counts{Indexed: 3, AlreadyDecided: 3})
	for _, id := range []string{"tx-on-a-line", "tx-just-now"} {
		kept := w.kept(id)
		if kept.State.Line == nil || *kept.State.Line != mercado || kept.State.ClassifiedBy != authored.AuthorPerson || len(kept.History) != 0 {
			t.Errorf("%s holds %+v, want the person's line and no step", id, kept)
		}
	}
	w.wantPending("tx-in-review", "Estabelecimento novo", nil, 0)
	for _, row := range rows {
		if !w.indexRow(row.ID).Evaluated {
			t.Errorf("%s is not marked evaluated", row.ID)
		}
	}
	if len(w.ruleChanges()) != 0 {
		t.Error("the rule wrote a change over transactions already decided")
	}
}

// seenOf is a card purchase as the index takes it in.
func seenOf(row ledgertest.Transaction) authored.SeenTransaction {
	return authored.SeenTransaction{
		ProviderID: row.ID, AccountID: row.AccountID, Day: "2026-03-10", Cents: -100,
		EstablishmentKey: "text:" + row.Description,
	}
}

func TestARulesLineTakenBackStaysTakenBack(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	delivery := w.line("2026-03", "Delivery")
	w.asPerson(func(recorder *authored.Recorder) error {
		return recorder.SetDefaultLine(t.Context(), defaultShop, delivery)
	})
	rows := []ledgertest.Transaction{
		purchase("tx-taken-back", at(2026, time.March, 10), "PADARIA PADRAO", 10),
		purchase("tx-kept", at(2026, time.March, 11), "PADARIA PADRAO", 11),
	}
	w.night("complete", rows...)
	wantCounts(t, "the first run", w.run(ingest.TriggerTick), ingest.Counts{Indexed: 2, Applied: 2})

	key, history := w.mustKey("tx-taken-back"), w.kept("tx-taken-back").History
	w.asPerson(func(recorder *authored.Recorder) error {
		return recorder.TakeBackStep(t.Context(), key, history[0], []authored.StepID{history[0].ID})
	})
	for night := 2; night <= 3; night++ {
		w.night("complete", rows...)
		wantCounts(t, "a later night", w.run(ingest.TriggerTick), ingest.Counts{})
	}
	if kept := w.kept("tx-taken-back"); kept.State.Line != nil || len(kept.History) != 0 {
		t.Errorf("the line taken back came back: %+v", kept)
	}
	w.wantApplied("tx-kept", delivery, "Delivery")
}

func TestASettingChangedActsOnlyOnWhatArrivesAfterIt(t *testing.T) {
	cases := []struct {
		name       string
		mercado    int
		delivery   int
		confidence int64
		setting    func(*world)
	}{
		{"the threshold lowered from 85 to 80", 5, 1, 83, func(w *world) {
			w.settings(func(recorder *authored.Recorder) error { return recorder.SetConfidenceThreshold(w.t.Context(), 80) })
		}},
		{"the minimum of examples lowered from 5 to 4", 4, 0, 100, func(w *world) {
			w.settings(func(recorder *authored.Recorder) error { return recorder.SetMinExamples(w.t.Context(), 4) })
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t, createdOn)
			w.saveBudget("2026-03", marketBudget())
			mercado, delivery := w.line("2026-03", "Mercado"), w.line("2026-03", "Delivery")
			w.examples(contestedShop, mercado, c.mercado, "mercado-")
			w.examples(contestedShop, delivery, c.delivery, "delivery-")
			first := purchase("tx-first", at(2026, time.March, 10), "FEIRA EXEMPLO", 60)
			w.night("complete", first)
			wantCounts(t, "before the setting", w.run(ingest.TriggerTick), ingest.Counts{Indexed: 1, PendingLowConfidence: 1})
			w.wantPending("tx-first", "Baixa confiança", &mercado, c.confidence)

			c.setting(w)
			w.night("complete", first, purchase("tx-second", at(2026, time.March, 11), "FEIRA EXEMPLO", 61))
			wantCounts(t, "after the setting", w.run(ingest.TriggerTick), ingest.Counts{Indexed: 1, Applied: 1})
			w.wantPending("tx-first", "Baixa confiança", &mercado, c.confidence)
			w.wantApplied("tx-second", mercado, "Mercado")
		})
	}
}

func (w *world) settings(write func(*authored.Recorder) error) {
	w.t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "settings.setThreshold", At: runInstant}
	if err := w.database().Change(w.t.Context(), facts, write); err != nil {
		w.t.Fatal(err)
	}
}

func TestARewordedRowKeepsWhatItHasUnderItsNewKey(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	w.night("complete", purchase("tx-reworded", at(2026, time.March, 10), "PADARIA ANTIGA", 10))
	w.run(ingest.TriggerTick)
	w.note("tx-reworded", "a padaria mudou de nome")
	kept := w.kept("tx-reworded")

	w.night("complete",
		purchase("tx-reworded", at(2026, time.March, 10), "PADARIA NOVA", 10),
		purchase("tx-new", at(2026, time.March, 12), "PADARIA NOVA", 14))
	wantCounts(t, "the night the bank rewords it", w.run(ingest.TriggerTick), ingest.Counts{Indexed: 1, PendingNewEstablishment: 1})
	if got := w.kept("tx-reworded"); !reflect.DeepEqual(got, kept) {
		t.Errorf("the reworded row holds %+v, want %+v", got, kept)
	}
	if key := w.indexRow("tx-reworded").EstablishmentKey; key != "text:PADARIA NOVA" {
		t.Errorf("the reworded row's index row carries %q", key)
	}
	w.wantPending("tx-new", "Estabelecimento novo", nil, 0)
}

func TestARunThatAuthorsNothingWritesNoChangeAndStillRecordsItsRun(t *testing.T) {
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	changes := len(w.scratch.Changes(t))
	w.night("complete", outcomeRows()[:2]...)
	wantCounts(t, "the run", w.run(ingest.TriggerTick), ingest.Counts{Indexed: 2, Left: 2})
	if got := len(w.scratch.Changes(t)); got != changes {
		t.Errorf("a run that authored nothing wrote %d changes", got-changes)
	}
	if run, found := w.lastRun(); !found || run != 1 {
		t.Errorf("the run taken in is %d (%t), want 1", run, found)
	}
}
