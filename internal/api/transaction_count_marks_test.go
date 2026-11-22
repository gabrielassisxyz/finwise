package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
)

// countedRow is what transactions.list answers about whether a transaction counts.
type countedRow struct {
	ID             string `json:"id"`
	Excluded       bool   `json:"excluded"`
	CountsInTotals bool   `json:"countsInTotals"`
}

func listRowOf(t *testing.T, env *readEnv, month, id string) countedRow {
	t.Helper()
	status, body := env.answer("transactions.list", listRaw(month, "month"))
	var rows []countedRow
	if err := json.Unmarshal(body, &rows); status != http.StatusOK || err != nil {
		t.Fatalf("transactions.list %s answered %d %s", month, status, body)
	}
	for _, row := range rows {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("%s is not listed in %s", id, month)
	return countedRow{}
}

func historyTexts(detail countedDetail) []string {
	texts := []string{}
	for _, step := range detail.History {
		texts = append(texts, step.Text)
	}
	return texts
}

func reasonOf(detail countedDetail) *string {
	if detail.Exclusion == nil {
		return nil
	}
	return &detail.Exclusion.Reason
}

func TestAnExclusionTakesEffectOnlyOnceItHasAReason(t *testing.T) {
	env := newCountEnv(t, ledgerWithAPurchaseToSplit(t), budgetToSplitIn)
	env.putOnLine(txSplitPurchase, "Mercado")
	counting := env.outcome("2026-03", txSplitPurchase)
	steps := []struct {
		method, argument string
		reason           *string
		excluded         bool
		disposition      budgetrules.Disposition
	}{
		{"transactions.setExcluded", `true`, authoredtest.Some(""), false, budgetrules.OnALine},
		{"transactions.setExclusionReason", `"   "`, authoredtest.Some(""), false, budgetrules.OnALine},
		{"transactions.setExclusionReason", `"Compra da empresa"`, authoredtest.Some("Compra da empresa"), true, budgetrules.Excluded},
		{"transactions.setExcluded", `false`, nil, false, budgetrules.OnALine},
	}
	for _, step := range steps {
		label := step.method + " " + step.argument
		env.change(step.method, `["`+txSplitPurchase+`",`+step.argument+`]`)
		detail, row := detailOf(t, env.readEnv, txSplitPurchase), listRowOf(t, env.readEnv, "2026-03", txSplitPurchase)
		if !reflect.DeepEqual(reasonOf(detail), step.reason) || detail.Excluded != step.excluded || detail.CountsInTotals == step.excluded {
			t.Errorf("%s: the detail answers exclusion %v, excluded %v, counts %v", label, reasonOf(detail), detail.Excluded, detail.CountsInTotals)
		}
		if row.Excluded != step.excluded || row.CountsInTotals == step.excluded {
			t.Errorf("%s: the list answers excluded %v, counts %v", label, row.Excluded, row.CountsInTotals)
		}
		if got := env.outcome("2026-03", txSplitPurchase); got.Disposition != step.disposition {
			t.Errorf("%s: the purchase is %v, want %v", label, got.Disposition, step.disposition)
		}
	}
	if got := env.outcome("2026-03", txSplitPurchase); !reflect.DeepEqual(got, counting) {
		t.Errorf("counted again the purchase is %+v, was %+v", got, counting)
	}
	if got := historyTexts(detailOf(t, env.readEnv, txSplitPurchase)); !slices.Equal(got, []string{countedAgainStep}) {
		t.Errorf("the history is %v, want the one step of counting again", got)
	}
}

func TestSwitchingTheExclusionOnEmptiesItsReasonAndAReasonSwitchesItOn(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), startFromTheInventedAuthorship)
	env.change("transactions.setExcluded", `["tx-pix-out",true]`)
	if detail := detailOf(t, env.readEnv, txPixOut); !reflect.DeepEqual(reasonOf(detail), authoredtest.Some("")) || detail.Excluded {
		t.Errorf("switching on an exclusion with a reason left %v, excluded %v", reasonOf(detail), detail.Excluded)
	}
	env.change("transactions.setExclusionReason", `["tx-pending","Compra da empresa"]`)
	if detail := detailOf(t, env.readEnv, txPending); !detail.Excluded || detail.CountsInTotals {
		t.Errorf("a reason given while the exclusion was off left excluded %v, counts %v", detail.Excluded, detail.CountsInTotals)
	}
}

// newestStep is the newest step of a listed transaction, with its key.
func (env *countEnv) newestStep(id string) (authored.TxKey, authored.HistoryStep) {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(env.t.Context(), []string{id})
	if err != nil {
		env.t.Fatal(err)
	}
	kept, err := view.Transactions(env.t.Context(), []authored.TxKey{keys[id]})
	if err != nil || len(kept[keys[id]].History) == 0 {
		env.t.Fatalf("%s has no step: %v", id, err)
	}
	history := kept[keys[id]].History
	return keys[id], history[len(history)-1]
}

func TestTakingBackCountingAgainPutsThePurchaseBackAmongTheExcluded(t *testing.T) {
	env := newCountEnv(t, ledgerWithAPurchaseToSplit(t), budgetToSplitIn)
	env.change("transactions.setExclusionReason", `["`+txSplitPurchase+`","Compra da empresa"]`)
	env.change("transactions.setExcluded", `["`+txSplitPurchase+`",false]`)
	key, step := env.newestStep(txSplitPurchase)
	if step.Text != countedAgainStep {
		t.Fatalf("the newest step is %q", step.Text)
	}
	err := inChange(t, env.scratch, env.ledger, func(writes Writes) error {
		return writes.Recorder.RestorePart(t.Context(), step.Change, authored.TransactionUnits(key, []authored.StepID{step.ID}))
	})
	if err != nil {
		t.Fatalf("taking the step back: %v", err)
	}
	detail := detailOf(t, env.readEnv, txSplitPurchase)
	if !reflect.DeepEqual(reasonOf(detail), authoredtest.Some("Compra da empresa")) || len(detail.History) != 0 {
		t.Errorf("after taking the step back the exclusion is %v and the history %v", reasonOf(detail), historyTexts(detail))
	}
	if got := env.outcome("2026-03", txSplitPurchase).Disposition; got != budgetrules.Excluded {
		t.Errorf("the purchase is %v, want excluded", got)
	}
}

func TestASelectionTakesTheReasonOnceOnEachOfItsTransactions(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	id := env.change("transactions.excludeMany", `[["tx-pending","tx-boleto","tx-pending"],"Marcada em lote"]`)
	if effects := env.scratch.EffectsOf(t, changeNumber(t, id)); len(effects) != 2 {
		t.Errorf("the selection recorded %+v, want one exclusion on each of its two transactions", effects)
	}
	for _, listed := range []string{txPending, txBoleto} {
		if detail := detailOf(t, env.readEnv, listed); !reflect.DeepEqual(reasonOf(detail), authoredtest.Some("Marcada em lote")) || !detail.Excluded {
			t.Errorf("%s: exclusion %v, excluded %v", listed, reasonOf(detail), detail.Excluded)
		}
	}
}

func TestOneUnknownIDRefusesTheWholeSelectionAndChangesNoTableOrTheIndex(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	before := env.scratch.Everything(t)
	status, body := env.answer("transactions.excludeMany", `[["tx-pending","no-such-transaction"],"Marcada em lote"]`)
	if status != http.StatusConflict || string(body) != `{"error":{"kind":"refused","reason":"not_found"}}` {
		t.Errorf("answered %d %s, want 409 not_found", status, body)
	}
	if after := env.scratch.Everything(t); after != before {
		t.Errorf("the refused selection changed the file:\n%s", differences(before, after))
	}
}

// The router rolls back a refused write, so this calls the handler in a change
// that commits whatever it recorded before refusing: a selection checked
// transaction by transaction as it is written would leave the first one behind.
func TestASelectionIsFoundWholeBeforeAnyOfItIsWritten(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	budgetState()(t, scratch)
	before := scratch.Everything(t)
	var refusal error
	err := inChange(t, scratch, newFullLedger(t), func(writes Writes) error {
		args := []json.RawMessage{json.RawMessage(`["tx-pending","no-such-transaction"]`), json.RawMessage(`"Marcada em lote"`)}
		_, refusal = excludeMany(t.Context(), args, writes)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if kind, reason := classify(refusal); kind != kindRefused || reason != reasonNotFound {
		t.Errorf("the selection was answered %s %s, want refused not_found", kind, reason)
	}
	if after := scratch.Everything(t); after != before {
		t.Errorf("the handler wrote before it refused:\n%s", differences(before, after))
	}
}

func TestUndoingASelectionGivesTheMonthsListBackAsItWas(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	env.change("transactions.setExclusionReason", `["tx-fee","Marcada em lote"]`)
	_, before := env.answer("transactions.list", listRaw("2026-03", "month"))
	id := env.change("transactions.excludeMany", `[["tx-pending","tx-boleto","tx-fee"],"Marcada em lote"]`)
	if _, during := env.answer("transactions.list", listRaw("2026-03", "month")); string(during) == string(before) {
		t.Fatal("the selection changed nothing the list answers")
	}
	env.undo(id)
	if _, after := env.answer("transactions.list", listRaw("2026-03", "month")); string(after) != string(before) {
		t.Errorf("after the undo the list is\n%s\nwas\n%s", after, before)
	}
}

func TestARefundWithALineTakesItsAmountOffThatLineAndUnmarkingItKeepsTheLine(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	env.putOnLine(txSalary, "Salário")
	salary, market := env.lineID("2026-03", "Salário"), env.lineID("2026-03", "Mercado")
	income := budgetrules.Outcome{Disposition: budgetrules.Income, Magnitude: 500_000,
		Lines: []budgetrules.LineAmount{{Line: salary, Effect: budgetrules.Received, Amount: 500_000}}}
	if got := env.outcome("2026-03", txSalary); !reflect.DeepEqual(got, income) {
		t.Fatalf("the PIX on Salário is %+v before any mark", got)
	}
	env.change("transactions.setRefund", `["tx-salary",true]`)
	if detail := detailOf(t, env.readEnv, txSalary); !detail.Refund || !slices.Equal(historyTexts(detail), []string{refundMarkedStep}) {
		t.Errorf("marked: refund %v, history %v", detail.Refund, historyTexts(detail))
	}
	env.change("transactions.setRefundLine", `["tx-salary","Mercado"]`)
	detail := detailOf(t, env.readEnv, txSalary)
	if detail.RefundLine == nil || *detail.RefundLine != "Mercado" ||
		!slices.Equal(historyTexts(detail), []string{refundLineStep("Mercado"), refundMarkedStep}) {
		t.Errorf("its line: %v, history %v", detail.RefundLine, historyTexts(detail))
	}
	refund := budgetrules.Outcome{Disposition: budgetrules.Refund, Magnitude: 500_000,
		Lines: []budgetrules.LineAmount{{Line: market, Effect: budgetrules.TakesOff, Amount: 500_000}}}
	if got := env.outcome("2026-03", txSalary); !reflect.DeepEqual(got, refund) {
		t.Errorf("the refund of Mercado is %+v, want %+v", got, refund)
	}
	env.change("transactions.setRefund", `["tx-salary",false]`)
	detail = detailOf(t, env.readEnv, txSalary)
	if detail.Refund || detail.RefundLine == nil || *detail.RefundLine != "Mercado" || historyTexts(detail)[0] != refundUnmarkedStep {
		t.Errorf("unmarked: refund %v, line %v, history %v", detail.Refund, detail.RefundLine, historyTexts(detail))
	}
	if got := env.outcome("2026-03", txSalary); !reflect.DeepEqual(got, income) {
		t.Errorf("unmarked the PIX is %+v, want income on Salário again", got)
	}
}

func TestTheLedgersRefundIsTheMarkUntilThePersonSaysOtherwise(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), nil)
	if !detailOf(t, env.readEnv, txRefund).Refund {
		t.Error("the card's refund is not one with nothing authored")
	}
	env.change("transactions.setRefund", `["tx-refund",false]`)
	if detailOf(t, env.readEnv, txRefund).Refund {
		t.Error("the refund the person unmarked is still one")
	}
	fresh := newCountEnv(t, newFullLedger(t), nil)
	id := fresh.change("transactions.setRefund", `["tx-refund",true]`)
	effects := fresh.scratch.EffectsOf(t, changeNumber(t, id))
	if len(effects) != 2 || effects[0].Column != "refund" || effects[1].Table != "history_steps" {
		t.Errorf("marking what the ledger already calls a refund recorded %+v, want the mark and its step", effects)
	}
}

func TestEachStepHasItsTextAndTheBrazilDayOfItsChangeNewestFirst(t *testing.T) {
	// 01:30 in UTC is still the evening before in Brazil.
	env := &countEnv{settingsEnv: newSettingsEnv(t, newFullLedger(t), time.Date(2026, time.March, 21, 1, 30, 0, 0, time.UTC))}
	budgetState()(t, env.scratch)
	for _, call := range []writeCall{
		{"transactions.setRefund", `["tx-salary",true]`}, {"transactions.setRefundLine", `["tx-salary","Mercado"]`},
		{"transactions.setRefund", `["tx-salary",false]`}, {"transactions.setExclusionReason", `["tx-salary","Compra da empresa"]`},
		{"transactions.setExcluded", `["tx-salary",false]`},
	} {
		env.change(call.method, call.arguments)
	}
	detail := detailOf(t, env.readEnv, txSalary)
	want := []string{countedAgainStep, refundUnmarkedStep, refundLineStep("Mercado"), refundMarkedStep}
	if !slices.Equal(historyTexts(detail), want) {
		t.Errorf("the history is %v, want %v", historyTexts(detail), want)
	}
	for _, step := range detail.History {
		if step.Day != "2026-03-20" {
			t.Errorf("%q is on %s, want the Brazil day 2026-03-20", step.Text, step.Day)
		}
	}
}

func TestAWriteThatChangesNothingIsAnEmptyChangeWithNoStep(t *testing.T) {
	split := `["tx-pending",[{"line":"Mercado","amount":10},{"line":null,"amount":8.5}]]`
	cases := []writeCase{
		{method: "transactions.setSplit", arguments: split, before: []writeCall{{"transactions.setSplit", split}}},
		{method: "transactions.setExcluded", arguments: `["tx-pending",false]`},
		{method: "transactions.setExclusionReason", arguments: `["tx-pending","Compra da empresa"]`,
			before: []writeCall{{"transactions.setExclusionReason", `["tx-pending"," Compra da empresa"]`}}},
		{method: "transactions.setRefund", arguments: `["tx-refund",true]`, before: []writeCall{{"transactions.setRefund", `["tx-refund",true]`}}},
		{method: "transactions.setRefundLine", arguments: `["tx-refund","Mercado"]`,
			before: []writeCall{{"transactions.setRefundLine", `["tx-refund","Mercado"]`}}},
		{method: "transactions.excludeMany", arguments: `[[],"Marcada em lote"]`},
		{method: "transactions.excludeMany", arguments: `[["tx-pending"],"Compra da empresa"]`,
			before: []writeCall{{"transactions.setExclusionReason", `["tx-pending","Compra da empresa"]`}}},
	}
	for _, c := range cases {
		c.expects, c.state = expectsRecorded, budgetState()
		o := runWriteCase(t, c, theRealUndo)
		label := c.method + " " + c.arguments
		if problems := answerProblems(c.method, o); problems != nil {
			t.Fatalf("%s: %v", label, problems)
		}
		if o.changeID == "" || o.changeID == o.originID || o.changesAfter != o.changesBefore+1 || len(o.effects) != 0 {
			t.Errorf("%s: change %q after %q, %d changes before and %d after, effects %+v", label, o.changeID, o.originID, o.changesBefore, o.changesAfter, o.effects)
		}
		if o.undoStatus != http.StatusOK || o.undoBody != "null" || o.canonicalAfterUndo != o.canonicalBefore {
			t.Errorf("%s: the undo answered %d %s and left %s", label, o.undoStatus, o.undoBody, restoredProblem(o.canonicalBefore, o.canonicalAfterUndo))
		}
	}
}

// sixWrites are one call of each write, on transactions every state lists.
var sixWrites = []writeCall{
	{"transactions.setSplit", `["tx-stale",[{"line":"Mercado","amount":30},{"line":null,"amount":20}]]`},
	{"transactions.setExcluded", `["tx-stale",true]`},
	{"transactions.setExclusionReason", `["tx-stale","Compra da empresa"]`},
	{"transactions.excludeMany", `[["tx-stale","tx-pending"],"Marcada em lote"]`},
	{"transactions.setRefund", `["tx-stale",true]`},
	{"transactions.setRefundLine", `["tx-stale","Mercado"]`},
}

func TestWithTheSnapshotMissingEachWriteIsUnavailableAndKeepsNothing(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	budgetState()(t, scratch)
	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), fixtureNow), withDatabase(scratch.Database))
	env := &readEnv{testEnv: missing, cookie: missing.unlock()}
	before := scratch.Everything(t)
	for _, call := range sixWrites {
		domain, name := splitMethod(t, call.method)
		expectError(t, call.method, env.call("/api/"+domain+"/"+name, call.arguments, env.cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	}
	if after := scratch.Everything(t); after != before {
		t.Errorf("a write with the snapshot missing changed the file:\n%s", differences(before, after))
	}
}

// January is closed in the invented state: closing a month freezes its budget,
// and the transactions of it still take every write.
func TestATransactionOfAClosedMonthTakesEachOfTheSixWrites(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), startFromTheInventedAuthorship)
	if status, body := env.answer("budget.save", `["2026-01",`+oneIncome+`]`); status != http.StatusConflict ||
		string(body) != `{"error":{"kind":"refused","reason":"month_closed"}}` {
		t.Fatalf("the control failed: a save for January answered %d %s, so the month is not closed", status, body)
	}
	for _, call := range sixWrites {
		env.change(call.method, call.arguments)
	}
	detail := detailOf(t, env.readEnv, txStale)
	if !detail.Split || !detail.Excluded || !detail.Refund || detail.RefundLine == nil || *detail.RefundLine != "Mercado" {
		t.Errorf("the closed month's credit answers split %v, excluded %v, refund %v, line %v", detail.Split, detail.Excluded, detail.Refund, detail.RefundLine)
	}
}
