package ingest_test

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
)

// What the run tells the rules about a transaction and its examples, each fact
// pinned by two histories that differ in that fact alone and that the rules
// answer differently. The rules' own tests cannot see what they were told.

func TestAPersonWithANicknameKeptIsNotAskedAboutAsUnnamed(t *testing.T) {
	cases := []struct {
		name     string
		nickname string
		want     ingest.Counts
		reason   string
	}{
		{"no nickname kept", "", ingest.Counts{Indexed: 1, PendingUnnamedPerson: 1}, "PIX para pessoa sem apelido"},
		{"a nickname kept", "Maria da feira", ingest.Counts{Indexed: 1, PendingNewEstablishment: 1}, "Estabelecimento novo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t, createdOn)
			w.saveBudget("2026-03", marketBudget())
			if c.nickname != "" {
				w.asPerson(func(recorder *authored.Recorder) error {
					return recorder.SetNickname(t.Context(), "doc:11122233344", c.nickname)
				})
			}
			w.night("complete", pix("tx-pix", at(2026, time.March, 16), "PIX ENVIADO MARIA EXEMPLO", -80, "Maria Exemplo", "11122233344"))
			wantCounts(t, c.name, w.run(ingest.TriggerTick), c.want)
			w.wantPending("tx-pix", c.reason, nil, 0)
		})
	}
}

// fiveExamplesOnMercado indexes five of the person's purchases at the Feira on
// Mercado, lets change alter the first of them, and takes in a new purchase
// there. Five is the default minimum, so one example fewer leaves it pending.
func fiveExamplesOnMercado(t *testing.T, change func(w *world, first authored.TxKey)) *world {
	t.Helper()
	w := newWorld(t, createdOn)
	w.saveBudget("2026-03", marketBudget())
	w.examples(contestedShop, w.line("2026-03", "Mercado"), 5, "feira-")
	change(w, w.mustKey("feira-a"))
	w.night("complete", purchase("tx-new", at(2026, time.March, 20), "FEIRA EXEMPLO", 60))
	return w
}

func TestAnExampleExcludedWithAReasonNoLongerCountsAndOneWithoutStillDoes(t *testing.T) {
	cases := []struct {
		name      string
		exclusion *string
		applied   bool
	}{
		{"not excluded", nil, true},
		{"excluded with a reason", someText("Era um presente"), false},
		{"switched off without a reason", someText(""), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := fiveExamplesOnMercado(t, func(w *world, first authored.TxKey) {
				if c.exclusion == nil {
					return
				}
				w.asPerson(func(recorder *authored.Recorder) error {
					_, err := recorder.SetExclusion(t.Context(), first, c.exclusion)
					return err
				})
			})
			w.wantFiveOrFourExamples(c.name, c.applied)
		})
	}
}

func TestASplitExampleNoLongerCounts(t *testing.T) {
	for _, split := range []bool{false, true} {
		name := map[bool]string{false: "not split", true: "split"}[split]
		t.Run(name, func(t *testing.T) {
			w := fiveExamplesOnMercado(t, func(w *world, first authored.TxKey) {
				if !split {
					return
				}
				mercado, delivery := w.line("2026-03", "Mercado"), w.line("2026-03", "Delivery")
				w.asPerson(func(recorder *authored.Recorder) error {
					return recorder.SetSplit(t.Context(), first, []authored.SplitPart{
						{Position: 0, Line: &mercado, Cents: 600}, {Position: 1, Line: &delivery, Cents: 400},
					})
				})
			})
			w.wantFiveOrFourExamples(name, !split)
		})
	}
}

// wantFiveOrFourExamples wants the new purchase applied on Mercado when all
// five examples count, and pending with Mercado at 100 when only four do.
func (w *world) wantFiveOrFourExamples(label string, applied bool) {
	w.t.Helper()
	counts := w.run(ingest.TriggerTick)
	mercado := w.line("2026-03", "Mercado")
	if applied {
		wantCounts(w.t, label, counts, ingest.Counts{Indexed: 1, Applied: 1})
		w.wantApplied("tx-new", mercado, "Mercado")
		return
	}
	wantCounts(w.t, label, counts, ingest.Counts{Indexed: 1, PendingLowConfidence: 1})
	w.wantPending("tx-new", "Baixa confiança", &mercado, 100)
}

func someText(text string) *string { return &text }
