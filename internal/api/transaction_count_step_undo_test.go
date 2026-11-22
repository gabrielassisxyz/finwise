package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// The steps that the writes of what a transaction counts for leave are taken
// back through transactions.undoHistoryStep, as the panel's "Desfazer" does:
// what the step changed comes back, what was done after it stays, and a step
// whose column was rewritten since is refused.

// countedPanel is what the detail answers about what a transaction counts for,
// with its history as the panel lists it.
type countedPanel struct {
	Excluded  bool `json:"excluded"`
	Exclusion *struct {
		Reason string `json:"reason"`
	} `json:"exclusion"`
	Refund     bool           `json:"refund"`
	RefundLine *string        `json:"refundLine"`
	History    []historyEntry `json:"history"`
}

func (env *countEnv) countedPanelOf(id string) countedPanel {
	env.t.Helper()
	status, body := env.answer("transactions.detail", `["`+id+`"]`)
	var panel *countedPanel
	if err := json.Unmarshal(body, &panel); status != http.StatusOK || err != nil || panel == nil {
		env.t.Fatalf("the detail of %s answered %d %s", id, status, body)
	}
	return *panel
}

// exclusionReason is the reason of the exclusion as a failure message reads it.
func (panel countedPanel) exclusionReason() string {
	if panel.Exclusion == nil {
		return "null"
	}
	return `"` + panel.Exclusion.Reason + `"`
}

func (panel countedPanel) texts() []string {
	texts := []string{}
	for _, entry := range panel.History {
		texts = append(texts, entry.Text)
	}
	return texts
}

func (panel countedPanel) entryNamed(text string) (historyEntry, bool) {
	for _, entry := range panel.History {
		if entry.Text == text {
			return entry, true
		}
	}
	return historyEntry{}, false
}

func (panel countedPanel) lists(stepID string) bool {
	for _, entry := range panel.History {
		if entry.ID == stepID {
			return true
		}
	}
	return false
}

// takeBackTheStep calls transactions.undoHistoryStep on the step of id's
// history with that text, and checks the call against the undoable flag the
// detail gave that step just before: the call succeeds exactly when the flag was
// true, a step taken back is no longer in the history, and a step refused is
// still there with the file as it was. It answers the change the call made, or
// "" for a refusal.
func (env *countEnv) takeBackTheStep(id, text string, wantUndoable bool) string {
	env.t.Helper()
	entry, found := env.countedPanelOf(id).entryNamed(text)
	if !found {
		env.t.Fatalf("the history of %s has no step %q", id, text)
	}
	if entry.Undoable != wantUndoable {
		env.t.Errorf("the detail shows %q as undoable %v, want %v", text, entry.Undoable, wantUndoable)
	}
	canonical, everything := env.scratch.Canonical(env.t), env.scratch.Everything(env.t)

	status, body := env.answer("transactions.undoHistoryStep", `["`+id+`","`+entry.ID+`"]`)

	change, answeredAChange := changeIDIn(body)
	succeeded := status == http.StatusOK && answeredAChange
	if succeeded != entry.Undoable {
		env.t.Errorf("%q was shown as undoable %v and taking it back answered %d %s", text, entry.Undoable, status, body)
	}
	listed := env.countedPanelOf(id).lists(entry.ID)
	if succeeded {
		if listed {
			env.t.Errorf("%q is still in the history after it was taken back", text)
		}
		return change
	}
	if status != http.StatusConflict || string(body) != `{"error":{"kind":"refused","reason":"stale_change"}}` {
		env.t.Errorf("the refusal of %q answered %d %s, want 409 stale_change", text, status, body)
	}
	if !listed {
		env.t.Errorf("%q left the history although taking it back was refused", text)
	}
	if after := env.scratch.Canonical(env.t); after != canonical {
		env.t.Errorf("a refused step's undo changed the canonical dump:\n%s", differences(canonical, after))
	}
	if after := env.scratch.Everything(env.t); after != everything {
		env.t.Errorf("a refused step's undo changed the file:\n%s", differences(everything, after))
	}
	return ""
}

func TestUndoHistoryStepOfCountingAgainPutsThePurchaseBackAmongTheExcluded(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	env.change("transactions.setExclusionReason", `["tx-pending","Compra da empresa"]`)
	env.change("transactions.setExcluded", `["tx-pending",false]`)
	if panel := env.countedPanelOf(txPending); panel.Excluded || panel.Exclusion != nil || !slices.Equal(panel.texts(), []string{countedAgainStep}) {
		t.Fatalf("before taking the step back: excluded %v, exclusion %s, history %v", panel.Excluded, panel.exclusionReason(), panel.texts())
	}

	env.takeBackTheStep(txPending, countedAgainStep, true)

	panel := env.countedPanelOf(txPending)
	if !panel.Excluded || panel.Exclusion == nil || panel.Exclusion.Reason != "Compra da empresa" {
		t.Errorf("after taking the step back: excluded %v and exclusion %s, want excluded with its reason", panel.Excluded, panel.exclusionReason())
	}
	if len(panel.History) != 0 {
		t.Errorf("the history is %v, want it empty", panel.texts())
	}
}

func TestUndoHistoryStepOfTheRefundMarkLeavesTheLineChosenAfterIt(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	env.change("transactions.setRefund", `["tx-salary",true]`)
	env.change("transactions.setRefundLine", `["tx-salary","Mercado"]`)
	line := refundLineStep("Mercado")

	env.takeBackTheStep(txSalary, refundMarkedStep, true)

	panel := env.countedPanelOf(txSalary)
	if panel.Refund || panel.RefundLine == nil || *panel.RefundLine != "Mercado" {
		t.Errorf("after taking the mark back: refund %v and line %s, want no mark and the line Mercado", panel.Refund, shownText(panel.RefundLine))
	}
	if !slices.Equal(panel.texts(), []string{line}) {
		t.Errorf("the history is %v, want the line's step alone", panel.texts())
	}
	if entry, _ := panel.entryNamed(line); !entry.Undoable {
		t.Errorf("the line's step is not undoable after the mark's was taken back: %+v", entry)
	}
}

func TestUndoingAnUndoHistoryStepOfAMarkMarksItAgain(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	env.change("transactions.setRefund", `["tx-salary",true]`)
	mark, _ := env.countedPanelOf(txSalary).entryNamed(refundMarkedStep)

	takenBack := env.takeBackTheStep(txSalary, refundMarkedStep, true)

	if panel := env.countedPanelOf(txSalary); panel.Refund || len(panel.History) != 0 {
		t.Fatalf("after taking the mark back: refund %v, history %v", panel.Refund, panel.texts())
	}
	env.undo(takenBack)

	panel := env.countedPanelOf(txSalary)
	if !panel.Refund {
		t.Error("after undoing the taking back the transaction is not marked as a refund")
	}
	if len(panel.History) != 1 || panel.History[0] != (historyEntry{ID: mark.ID, Text: refundMarkedStep, Day: mark.Day, Undoable: true}) {
		t.Errorf("the history is %+v, want the mark's step back under id %s", panel.History, mark.ID)
	}
}

func TestUndoHistoryStepOfTheFeesLineAsARefundLineLeavesNoLine(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	env.change("transactions.setRefundLine", `["tx-salary","`+authored.FeesLineName+`"]`)
	step := refundLineStep(authored.FeesLineName)
	if panel := env.countedPanelOf(txSalary); panel.RefundLine == nil || *panel.RefundLine != authored.FeesLineName {
		t.Fatalf("before taking the step back the refund line is %s", shownText(panel.RefundLine))
	}

	env.takeBackTheStep(txSalary, step, true)

	panel := env.countedPanelOf(txSalary)
	if panel.RefundLine != nil {
		t.Errorf("after taking the step back the refund line is %s, want none", shownText(panel.RefundLine))
	}
	if len(panel.History) != 0 {
		t.Errorf("the history is %v, want it empty", panel.texts())
	}
}

func TestUndoHistoryStepRefusesAStepWhoseColumnALaterChangeRewrote(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	env.change("transactions.setRefund", `["tx-salary",true]`)
	env.change("transactions.setRefund", `["tx-salary",false]`)

	// takeBackTheStep checks the refusal itself and that nothing changed.
	if change := env.takeBackTheStep(txSalary, refundMarkedStep, false); change != "" {
		t.Errorf("the mark's step was taken back by change %s over the later unmarking", change)
	}

	panel := env.countedPanelOf(txSalary)
	if want := []string{refundUnmarkedStep, refundMarkedStep}; !slices.Equal(panel.texts(), want) {
		t.Errorf("the history is %v, want both steps as they were: %v", panel.texts(), want)
	}
	if entry, _ := panel.entryNamed(refundUnmarkedStep); !entry.Undoable {
		t.Errorf("the later step is not undoable: %+v", entry)
	}
}
