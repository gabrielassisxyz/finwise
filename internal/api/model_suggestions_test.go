package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/classrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// queueWithModelSuggestions adds stored model suggestions over
// queueOverTheThirdState: a usable suggestion for a new establishment, a usable
// suggestion overriding a rule's low-confidence suggestion, and an unusable
// (asked, no line) row that lets the rule's suggestion show again.
func queueWithModelSuggestions(t *testing.T, scratch *authoredtest.Scratch, full *ledgertest.Fixture) {
	t.Helper()
	queueOverTheThirdState(t, scratch, full)
	view := scratch.Database.View()
	keys, err := view.TransactionKeys(t.Context(), []string{txInstalment, txForeign, txBoleto})
	if err != nil {
		t.Fatal(err)
	}
	lines, err := linesOfMonth(t.Context(), view, "2026-03")
	view.Close()
	if err != nil {
		t.Fatal(err)
	}
	feira, ok := lines.idOf("Feira")
	if !ok {
		t.Fatal("March budget lacks Feira")
	}
	delivery, ok := lines.idOf("Delivery")
	if !ok {
		t.Fatal("March budget lacks Delivery")
	}
	confNew, confOverride := int64(82), int64(91)
	whyNew := "Livraria sem histórico, enquadrada em Feira no orçamento."
	whyOverride := "Assinatura de nuvem enquadrada em Delivery pelo histórico."
	askedAt := utc(2026, time.March, 15, 10, 0)
	if err := scratch.Database.RecordModelSuggestions(t.Context(), "test-model", askedAt, []authored.ModelSuggestionInput{
		{Key: keys[txInstalment], Line: &feira, Confidence: &confNew, Justification: &whyNew},
		{Key: keys[txForeign], Line: &delivery, Confidence: &confOverride, Justification: &whyOverride},
		{Key: keys[txBoleto]},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestModelSuggestionShowsInQueueListAndPanelAndFallsBackToRule(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	queueWithModelSuggestions(t, scratch, full)
	env := newReadEnvOver(t, full, scratch.Database)

	_, queueBody := env.answer("review.weekly", `["2026-03"]`)
	if problem := conformanceProblem("review.weekly", queueBody); problem != "" {
		t.Fatalf("review.weekly does not conform: %s", problem)
	}
	var weekly struct {
		Queue struct {
			Items []reviewQueueItem `json:"items"`
		} `json:"queue"`
	}
	if err := json.Unmarshal(queueBody, &weekly); err != nil {
		t.Fatal(err)
	}
	queueByID := map[string]*pendingReview{}
	for _, item := range weekly.Queue.Items {
		queueByID[item.TransactionID] = item.Pending
	}

	_, listBody := env.answer("transactions.list", listRaw("2026-03", "month"))
	var listed []transactionWire
	if err := json.Unmarshal(listBody, &listed); err != nil {
		t.Fatal(err)
	}
	listByID := map[string]*pendingReview{}
	for _, item := range listed {
		listByID[item.ID] = item.Pending
	}

	str := func(s string) *string { return &s }
	num := func(n int) *int { return &n }
	want := map[string]struct {
		pending       pendingReview
		justification string
	}{
		txInstalment: {
			pending: pendingReview{
				Reason:        string(classrules.ReasonNewEstablishment),
				Suggestion:    str("Feira"),
				Confidence:    num(82),
				Justification: str("Livraria sem histórico, enquadrada em Feira no orçamento."),
			},
			justification: "Livraria sem histórico, enquadrada em Feira no orçamento.",
		},
		txForeign: {
			pending: pendingReview{
				Reason:        string(classrules.ReasonLowConfidence),
				Suggestion:    str("Delivery"),
				Confidence:    num(91),
				Justification: str("Assinatura de nuvem enquadrada em Delivery pelo histórico."),
			},
			justification: "Assinatura de nuvem enquadrada em Delivery pelo histórico.",
		},
		txBoleto: {
			pending: pendingReview{
				Reason:        "Confiança abaixo do limite",
				Suggestion:    str("Contas da casa"),
				Confidence:    num(64),
				Justification: nil,
			},
			justification: "",
		},
	}

	for id, expected := range want {
		if !reflect.DeepEqual(queueByID[id], &expected.pending) {
			t.Errorf("queue %s: got %+v, want %+v", id, queueByID[id], expected.pending)
		}
		if !reflect.DeepEqual(listByID[id], &expected.pending) {
			t.Errorf("list %s: got %+v, want %+v", id, listByID[id], expected.pending)
		}
		_, detailBody := env.answer("transactions.detail", `["`+id+`"]`)
		var detail transactionDetailWire
		if err := json.Unmarshal(detailBody, &detail); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(detail.Pending, &expected.pending) {
			t.Errorf("panel %s pending: got %+v, want %+v", id, detail.Pending, expected.pending)
		}
		if detail.Justification == nil || *detail.Justification != expected.justification {
			t.Errorf("panel %s justification: got %v, want %q", id, detail.Justification, expected.justification)
		}
	}
}

func TestConfirmLinePutsPendingTransactionOnModelsSuggestedLineWithoutRecordingQueueAnswer(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	queueOverTheThirdState(t, scratch, full)
	authoredtest.TakeRunIn(t, scratch.Database, 1)

	view := scratch.Database.View()
	keys, err := view.TransactionKeys(t.Context(), []string{txInstalment, txBoleto})
	if err != nil {
		t.Fatal(err)
	}
	lines, err := linesOfMonth(t.Context(), view, "2026-03")
	view.Close()
	if err != nil {
		t.Fatal(err)
	}
	feira, _ := lines.idOf("Feira")
	delivery, _ := lines.idOf("Delivery")
	confNew, confOverride := int64(82), int64(88)
	whyNew := "Livraria sem histórico, enquadrada em Feira no orçamento."
	whyOverride := "Telecom enquadrada em Delivery pelo modelo."
	if err := scratch.Database.RecordModelSuggestions(t.Context(), "test-model", fixtureNow, []authored.ModelSuggestionInput{
		// New establishment with no rule suggestion.
		{Key: keys[txInstalment], Line: &feira, Confidence: &confNew, Justification: &whyNew},
		// Low-confidence review where the rule suggested Contas da casa, overridden by the model.
		{Key: keys[txBoleto], Line: &delivery, Confidence: &confOverride, Justification: &whyOverride},
	}); err != nil {
		t.Fatal(err)
	}

	env := newTestEnv(t, withLedger(full.Path, fixtureNow), withDatabase(scratch.Database))
	cookie := env.unlock()

	for _, c := range []struct{ id, wantLine string }{
		{txInstalment, "Feira"},
		{txBoleto, "Delivery"},
	} {
		response := env.call("/api/transactions/confirmLine", `["`+c.id+`"]`, cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("confirmLine %s answered %d %s", c.id, response.Code, response.Body.String())
		}
		detailResp := env.call("/api/transactions/detail", `["`+c.id+`"]`, cookie)
		var detail transactionDetailWire
		if err := json.Unmarshal(detailResp.Body.Bytes(), &detail); err != nil {
			t.Fatal(err)
		}
		if detail.Line == nil || *detail.Line != c.wantLine || detail.Pending != nil {
			t.Errorf("%s after confirmLine: line=%v pending=%v, want %s and nil", c.id, detail.Line, detail.Pending, c.wantLine)
		}
		if len(detail.History) == 0 || detail.History[0].Text != "Você confirmou "+c.wantLine {
			t.Errorf("%s history = %+v", c.id, detail.History)
		}
	}

	readView := scratch.Database.View()
	defer readView.Close()
	suggestions, err := readView.ModelSuggestions(t.Context(), []authored.TxKey{keys[txInstalment], keys[txBoleto]})
	if err != nil {
		t.Fatal(err)
	}
	for id, key := range keys {
		if suggestions[key].Answer != "" || suggestions[key].AnswerChange != nil {
			t.Errorf("confirmLine on %s recorded a queue answer: %+v", id, suggestions[key])
		}
	}
}

func TestAcceptPendingRecordsAcceptedCorrectedAndNothingForRules(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	queueWithModelSuggestions(t, scratch, full)
	authoredtest.TakeRunIn(t, scratch.Database, 1)
	env := newTestEnv(t, withLedger(full.Path, fixtureNow), withDatabase(scratch.Database))
	cookie := env.unlock()

	// Accept model's suggestion for txInstalment -> accepted + "Você aceitou Feira".
	acceptResp := env.call("/api/review/acceptPending", `["`+txInstalment+`",{"line":"Feira","always":false,"nickname":"","reason":""}]`, cookie)
	if acceptResp.Code != http.StatusOK {
		t.Fatalf("acceptPending accepted: %d %s", acceptResp.Code, acceptResp.Body.String())
	}
	acceptID, ok := changeIDIn(acceptResp.Body.Bytes())
	if !ok {
		t.Fatalf("no changeId in %s", acceptResp.Body.String())
	}
	var detail transactionDetailWire
	if err := json.Unmarshal(env.call("/api/transactions/detail", `["`+txInstalment+`"]`, cookie).Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.History) == 0 || detail.History[0].Text != "Você aceitou Feira" {
		t.Errorf("accepted history = %+v, want Você aceitou Feira", detail.History)
	}

	// Choose another line for txForeign (model suggested Delivery) -> corrected + "Você alterou para Viagens".
	correctResp := env.call("/api/review/acceptPending", `["`+txForeign+`",{"line":"Viagens","always":false,"nickname":"","reason":""}]`, cookie)
	if correctResp.Code != http.StatusOK {
		t.Fatalf("acceptPending corrected: %d %s", correctResp.Code, correctResp.Body.String())
	}
	correctID, ok := changeIDIn(correctResp.Body.Bytes())
	if !ok {
		t.Fatalf("no changeId in %s", correctResp.Body.String())
	}
	detail = transactionDetailWire{}
	if err := json.Unmarshal(env.call("/api/transactions/detail", `["`+txForeign+`"]`, cookie).Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.History) == 0 || detail.History[0].Text != "Você alterou para Viagens" {
		t.Errorf("corrected history = %+v, want Você alterou para Viagens", detail.History)
	}

	// Accept rule's suggestion for txBoleto (model gave no usable answer) -> nothing recorded in model_suggestions.
	ruleResp := env.call("/api/review/acceptPending", `["`+txBoleto+`",{"line":"Contas da casa","always":false,"nickname":"","reason":""}]`, cookie)
	if ruleResp.Code != http.StatusOK {
		t.Fatalf("acceptPending rule: %d %s", ruleResp.Code, ruleResp.Body.String())
	}
	detail = transactionDetailWire{}
	if err := json.Unmarshal(env.call("/api/transactions/detail", `["`+txBoleto+`"]`, cookie).Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.History) == 0 || detail.History[0].Text != "Você aceitou Contas da casa" {
		t.Errorf("rule accept history = %+v, want Você aceitou Contas da casa", detail.History)
	}

	view := scratch.Database.View()
	defer view.Close()
	keys, _ := view.TransactionKeys(t.Context(), []string{txInstalment, txForeign, txBoleto})
	suggestions, err := view.ModelSuggestions(t.Context(), []authored.TxKey{keys[txInstalment], keys[txForeign], keys[txBoleto]})
	if err != nil {
		t.Fatal(err)
	}
	if got := suggestions[keys[txInstalment]]; got.Answer != authored.ModelAnswerAccepted || got.AnswerChange == nil || got.AnswerChange.String() != acceptID {
		t.Errorf("txInstalment suggestion = %+v, want accepted/%s", got, acceptID)
	}
	if got := suggestions[keys[txForeign]]; got.Answer != authored.ModelAnswerCorrected || got.AnswerChange == nil || got.AnswerChange.String() != correctID {
		t.Errorf("txForeign suggestion = %+v, want corrected/%s", got, correctID)
	}
	if got := suggestions[keys[txBoleto]]; got.Answer != "" || got.AnswerChange != nil {
		t.Errorf("txBoleto suggestion = %+v, want no answer recorded for rule suggestion", got)
	}
}

func TestAcceptPendingRecordsNothingWhenModelsLineIsAbsentFromTheMonth(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	queueOverTheThirdState(t, scratch, full)
	authoredtest.TakeRunIn(t, scratch.Database, 1)

	// Give txForeign a model suggestion for plannedLine (only in 2026-04, absent in 2026-03),
	// so the queue shows the rule's suggestion ("Viagens") instead of the model's.
	lines := lineIDsByName(t, scratch.Database)
	absentLine := lines[plannedLine]
	view := scratch.Database.View()
	keys, _ := view.TransactionKeys(t.Context(), []string{txForeign})
	view.Close()
	conf := int64(90)
	why := "Linha de outro mês."
	if err := scratch.Database.RecordModelSuggestions(t.Context(), "test-model", fixtureNow, []authored.ModelSuggestionInput{
		{Key: keys[txForeign], Line: &absentLine, Confidence: &conf, Justification: &why},
	}); err != nil {
		t.Fatal(err)
	}

	env := newTestEnv(t, withLedger(full.Path, fixtureNow), withDatabase(scratch.Database))
	cookie := env.unlock()
	resp := env.call("/api/review/acceptPending", `["`+txForeign+`",{"line":"Viagens","always":false,"nickname":"","reason":""}]`, cookie)
	if resp.Code != http.StatusOK {
		t.Fatalf("acceptPending: %d %s", resp.Code, resp.Body.String())
	}
	readView := scratch.Database.View()
	defer readView.Close()
	suggestions, err := readView.ModelSuggestions(t.Context(), []authored.TxKey{keys[txForeign]})
	if err != nil {
		t.Fatal(err)
	}
	if got := suggestions[keys[txForeign]]; got.Answer != "" || got.AnswerChange != nil {
		t.Errorf("answer recorded when rule's suggestion was shown: %+v", got)
	}
}

func TestStoredSuggestionsMoveNoFigureUntilAccepted(t *testing.T) {
	beforeScratch, beforeFull := newAuthoredStores(t)
	queueOverTheThirdState(t, beforeScratch, beforeFull)
	beforeEnv := newReadEnvOver(t, beforeFull, beforeScratch.Database)

	afterScratch, afterFull := newAuthoredStores(t)
	queueWithModelSuggestions(t, afterScratch, afterFull)
	afterEnv := newReadEnvOver(t, afterFull, afterScratch.Database)

	unchangedReads := []struct{ method, args string }{
		{"shell.overview", `[]`},
		{"home.summary", `["2026-03"]`},
		{"budget.view", `["2026-03"]`},
		{"card.overview", `[]`},
		{"netWorth.overview", `[]`},
		{"review.payday", `["2026-03"]`},
		{"review.closing", `["2026-03"]`},
	}
	for _, c := range unchangedReads {
		_, beforeBody := beforeEnv.answer(c.method, c.args)
		_, afterBody := afterEnv.answer(c.method, c.args)
		if string(beforeBody) != string(afterBody) {
			t.Errorf("%s %s changed after storing model suggestions:\nbefore: %s\nafter:  %s", c.method, c.args, beforeBody, afterBody)
		}
	}

	// In transactions.detail only pending and justification change.
	for _, id := range []string{txInstalment, txForeign} {
		_, beforeBody := beforeEnv.answer("transactions.detail", `["`+id+`"]`)
		_, afterBody := afterEnv.answer("transactions.detail", `["`+id+`"]`)
		var beforeMap, afterMap map[string]any
		_ = json.Unmarshal(beforeBody, &beforeMap)
		_ = json.Unmarshal(afterBody, &afterMap)
		if reflect.DeepEqual(beforeMap["pending"], afterMap["pending"]) {
			t.Errorf("%s: pending did not gain the model suggestion", id)
		}
		if reflect.DeepEqual(beforeMap["justification"], afterMap["justification"]) {
			t.Errorf("%s: justification did not gain the model justification", id)
		}
		delete(beforeMap, "pending")
		delete(afterMap, "pending")
		delete(beforeMap, "justification")
		delete(afterMap, "justification")
		if !reflect.DeepEqual(beforeMap, afterMap) {
			t.Errorf("%s: non-suggestion fields changed in detail", id)
		}
	}
}

func TestReadSuggestionCandidatesExcludesPersonsMovementsAndAskedRows(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	queueOverTheThirdState(t, scratch, full)

	// Mark txBoleto as already asked.
	view := scratch.Database.View()
	keys, err := view.TransactionKeys(t.Context(), []string{txBoleto, txInstalment, txForeign})
	view.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := scratch.Database.RecordModelSuggestions(t.Context(), "test-model", fixtureNow, []authored.ModelSuggestionInput{
		{Key: keys[txBoleto]},
	}); err != nil {
		t.Fatal(err)
	}

	ledgerView := ledger.New(full.Path).View()
	defer ledgerView.Close()
	authoredView := scratch.Database.View()
	defer authoredView.Close()

	candidates, err := ReadSuggestionCandidates(t.Context(), Views{Ledger: ledgerView, Authored: authoredView})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates.Months) != 1 || candidates.Months[0].Month != "2026-03" {
		t.Fatalf("months = %+v, want only 2026-03", candidates.Months)
	}
	var gotKeys []authored.TxKey
	for _, purchase := range candidates.Months[0].Purchases {
		gotKeys = append(gotKeys, purchase.Key)
	}
	// txPixOut and txStale are persons; txBoleto was already asked; txInstalment and txForeign remain.
	wantKeys := []authored.TxKey{keys[txForeign], keys[txInstalment]}
	if !slices.Equal(gotKeys, wantKeys) {
		t.Errorf("candidate keys = %v, want %v (foreign and instalment, newest first)", gotKeys, wantKeys)
	}
	// Examples come from person-classified establishments (no person transfers).
	for _, example := range candidates.Examples {
		if example.Establishment == "Maria Exemplo" || example.Establishment == "Amigo Exemplo" {
			t.Errorf("person transfer leaked into examples: %+v", example)
		}
	}
}
