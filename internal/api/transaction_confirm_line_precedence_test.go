package api

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// A purchase can be on a line and also have a review waiting whose suggestion is
// another line. Confirming accepts what the list shows, the line, and not the
// suggestion the panel only offers.
func TestConfirmLineSettlesOnTheLineShownAndNotOnTheSuggestionWaiting(t *testing.T) {
	env := newLineEnv(t, newFullLedger(t), lineVersions)
	market, delivery := marketLine, deliveryLine
	authoredOver(t, env.scratch, newFullLedger(t), func(a authoring) error {
		return a.state(txBoleto, authored.TransactionState{
			Line: &delivery, ClassifiedBy: authored.AuthorModel, Confidence: authoredtest.Some[int64](91),
			PendingReason: "Confiança abaixo do limite", PendingLine: &market, PendingConfidence: authoredtest.Some[int64](64),
		})
	})
	march := budgetIn(t, env.scratch.Database, "2026-03")
	if !hasLine(march.Lines, deliveryLine, "Delivery") || !hasLine(march.Lines, marketLine, "Feira") {
		t.Fatalf("the budget of the purchase's month does not have both lines: %+v", march.Lines)
	}
	shown := env.panelOf(txBoleto)
	if !textIs(shown.Line, "Delivery") || shown.Pending == nil || shown.Pending.Suggestion == nil || *shown.Pending.Suggestion != "Feira" {
		t.Fatalf("the panel before confirming reads %+v, want Delivery with a review suggesting Feira", shown)
	}

	changeIDOfAnswer(t, env.post("transactions.confirmLine", `["tx-boleto"]`))

	if stored := env.storedAbout(txBoleto).State; !lineIs(stored, deliveryLine) {
		t.Errorf("confirming stored %+v, want the line it was on, %d", stored, deliveryLine)
	}
	panel := env.panelOf(txBoleto)
	if !textIs(panel.Line, "Delivery") {
		t.Errorf("the panel answers the line %s after confirming, want Delivery", shownText(panel.Line))
	}
	if !textIs(panel.ClassifiedBy, classifiedByPerson) || panel.Confidence != nil {
		t.Errorf("confirming left classifiedBy %s and a confidence, want the person and none", shownText(panel.ClassifiedBy))
	}
	if panel.Pending != nil {
		t.Errorf("the review is still waiting after confirming: %+v", panel.Pending)
	}
	if len(panel.History) != 1 || panel.History[0].Text != "Você confirmou Delivery" {
		t.Errorf("the history is %+v, want the one step %q", panel.History, "Você confirmou Delivery")
	}
}

// shownText is an optional text as a failure message reads it.
func shownText(text *string) string {
	if text == nil {
		return "null"
	}
	return `"` + *text + `"`
}
