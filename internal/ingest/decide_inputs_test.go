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
