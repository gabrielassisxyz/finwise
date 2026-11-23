package api

import (
	"net/http"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// The steps that linking and unlinking a reimbursement leave are taken back
// through transactions.undoHistoryStep, as the panel's "Desfazer" does. Taking
// a step back restores what its change did to the purchase alone, so the
// credit's review, which the link settled, stays settled.

var linkedToAna = linkedReimbursementStep(anaName, 15_000)

// awaitReviewOf puts a listed credit in the review queue with a suggestion on a
// line its month's budget has.
func (env *countEnv) awaitReviewOf(id string) {
	env.t.Helper()
	line := budgetLineID(env.t, env.scratch.Database, "2026-03", "Renda extra")
	err := inChange(env.t, env.scratch, env.ledger, func(writes Writes) error {
		target, err := targetNamedBy(env.t.Context(), writes, id)
		if err != nil {
			return err
		}
		return authoredtest.WriteState(env.t.Context(), writes.Recorder, target.key, authored.TransactionState{
			PendingReason: "Estabelecimento novo", PendingLine: &line, PendingConfidence: authoredtest.Some[int64](55),
		})
	})
	if err != nil {
		env.t.Fatal(err)
	}
}

func TestUndoHistoryStepOfTheLinkRemovesTheLinkAndLeavesTheCreditsReviewSettled(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	env.awaitReviewOf(txPixAna)
	env.linkDinnerTo(txPixAna)

	env.takeBackTheStep(txDinner, linkedToAna, true)

	if got := env.reimbursementOf(txDinner); got != `{"expected":90.00,"link":null}` {
		t.Errorf("after taking the link's step back the dinner answers %s", got)
	}
	if texts := env.countedPanelOf(txDinner).texts(); len(texts) != 0 {
		t.Errorf("the dinner's history is %v, want it empty", texts)
	}
	credit := reimbursementDetailOf(t, env.readEnv, txPixAna)
	if string(credit.Pending) != "null" || credit.ReimbursementIncome {
		t.Errorf("the PIX answers pending %s and reimbursementIncome %v, want its review still settled and no purchase paid", credit.Pending, credit.ReimbursementIncome)
	}
	if !slices.Contains(reimbursementCandidateIDs(reimbursementDetailOf(t, env.readEnv, txLunch)), txPixAna) {
		t.Error("the PIX is not a candidate of the lunch once the link's step was taken back")
	}
}

func TestUndoHistoryStepOfTheUnlinkLinksTheCreditAgain(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	env.linkDinnerTo(txPixAna)
	env.change("transactions.unlinkReimbursement", `["`+txDinner+`"]`)

	env.takeBackTheStep(txDinner, unlinkedReimbursementStep, true)

	detail := reimbursementDetailOf(t, env.readEnv, txDinner)
	want := `{"expected":90.00,"link":{"transactionId":"tx-pix-ana","name":"Ana Exemplo","day":"2026-03-12","amount":150.00}}`
	if got := compactJSON(t, detail.Reimbursement); got != want || string(detail.EffectiveAmount) != "-30.00" || !detail.Reimbursed {
		t.Errorf("after taking the unlinking back the dinner answers %s, effective %s, reimbursed %v", got, detail.EffectiveAmount, detail.Reimbursed)
	}
	if texts := env.countedPanelOf(txDinner).texts(); !slices.Equal(texts, []string{linkedToAna}) {
		t.Errorf("the history is %v, want the link's step alone", texts)
	}
}

func TestUndoingTheTakingBackOfTheLinksStepLinksAgainUnderTheSameStep(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	env.linkDinnerTo(txPixAna)
	step, _ := env.countedPanelOf(txDinner).entryNamed(linkedToAna)

	takenBack := env.takeBackTheStep(txDinner, linkedToAna, true)
	env.undo(takenBack)

	if got := compactJSON(t, reimbursementDetailOf(t, env.readEnv, txDinner).Reimbursement); got == `{"expected":90.00,"link":null}` {
		t.Errorf("after undoing the taking back the dinner answers %s, want the link back", got)
	}
	history := env.countedPanelOf(txDinner).History
	if len(history) != 1 || history[0].ID != step.ID || history[0].Text != linkedToAna || !history[0].Undoable {
		t.Errorf("the history is %+v, want the link's step back under id %s and undoable", history, step.ID)
	}
	if !reimbursementRowOf(t, env.readEnv, "2026-03", txPixAna).ReimbursementIncome {
		t.Error("the PIX does not pay the dinner back again")
	}
}

func TestUndoHistoryStepRefusesTheLinksStepOnceTheUnlinkingRewroteIt(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	env.linkDinnerTo(txPixAna)
	env.change("transactions.unlinkReimbursement", `["`+txDinner+`"]`)

	// takeBackTheStep checks the refusal itself and that nothing changed.
	if change := env.takeBackTheStep(txDinner, linkedToAna, false); change != "" {
		t.Errorf("the link's step was taken back by change %s over the later unlinking", change)
	}

	if texts := env.countedPanelOf(txDinner).texts(); !slices.Equal(texts, []string{unlinkedReimbursementStep, linkedToAna}) {
		t.Errorf("the history is %v, want both steps as they were", texts)
	}
	if got := env.reimbursementOf(txDinner); got != `{"expected":90.00,"link":null}` {
		t.Errorf("the dinner answers %s, want it unlinked still", got)
	}
}

// The one step whose flag and whose taking back disagree: the history judges
// a step only by what changed since in the purchase's own facts, and cannot
// see that the database's unique credit key refuses to give the credit, which
// now pays another purchase, back to this one. The flag says yes, the call is
// refused as stale and nothing changes.
func TestTheUnlinksStepIsShownUndoableWhileItsCreditPaysAnotherPurchaseAndTakingItBackIsRefused(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	env.linkDinnerTo(txPixAna)
	env.change("transactions.unlinkReimbursement", `["`+txDinner+`"]`)
	env.change("transactions.setReimbursable", `["`+txLunch+`",true]`)
	env.change("transactions.linkReimbursement", `["`+txLunch+`","`+txPixAna+`"]`)

	step, found := env.countedPanelOf(txDinner).entryNamed(unlinkedReimbursementStep)
	if !found || !step.Undoable {
		t.Fatalf("the unlinking's step is %+v, want it shown as undoable", step)
	}
	canonical, everything := env.scratch.Canonical(t), env.scratch.Everything(t)
	dinner, lunch := env.reimbursementOf(txDinner), env.reimbursementOf(txLunch)

	status, body := env.answer("transactions.undoHistoryStep", `["`+txDinner+`","`+step.ID+`"]`)

	if status != http.StatusConflict || string(body) != refusedWith(reasonStaleChange) {
		t.Errorf("taking back the unlinking answered %d %s, want 409 stale_change", status, body)
	}
	if after := env.scratch.Canonical(t); after != canonical {
		t.Errorf("the refusal changed the canonical dump:\n%s", differences(canonical, after))
	}
	if after := env.scratch.Everything(t); after != everything {
		t.Errorf("the refusal changed the file:\n%s", differences(everything, after))
	}
	if !env.countedPanelOf(txDinner).lists(step.ID) {
		t.Error("the unlinking's step left the history although taking it back was refused")
	}
	if env.reimbursementOf(txDinner) != dinner || env.reimbursementOf(txLunch) != lunch {
		t.Errorf("after the refusal the purchases answer %s and %s, were %s and %s", env.reimbursementOf(txDinner), env.reimbursementOf(txLunch), dinner, lunch)
	}
}
