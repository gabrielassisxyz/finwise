package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/classrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// What an agent proposes for a transaction that waits for review is the queue's
// suggestion for it, above the model's and the rule's; the person answers it
// by settling the review, and whatever settles the review closes it. The
// examples are the review's: P1 waits with the rule suggesting Restaurantes at
// 58, P2 is a PIX to a person and P3 a purchase at a place seen before.

const (
	agentName   = "agente-revisor"
	agentReason = "O recibo no e-mail é da conta de luz"
	pendingOfP1 = `{"reason":"Baixa confiança","suggestion":"Restaurantes","confidence":58,"justification":null}`
	agentOfP1   = `{"reason":"Baixa confiança","suggestion":"Contas da casa","confidence":null,"justification":"` + agentReason + `"}`
)

func (env *writeEnv) propose(id, line, reason string) *httptest.ResponseRecorder {
	env.t.Helper()
	body, err := json.Marshal([]any{map[string]any{
		"change":   map[string]any{"method": proposedLineMethod, "arguments": []string{id, line}},
		"reason":   reason,
		"proposer": agentName,
	}})
	if err != nil {
		env.t.Fatal(err)
	}
	return env.post("review.submitProposal", string(body))
}

// proposed is a proposal that was stored, and its id.
func (env *writeEnv) proposed(id, line, reason string) string {
	env.t.Helper()
	response := env.propose(id, line, reason)
	var stored submittedProposal
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &stored) != nil || stored.ProposalID == "" {
		env.t.Fatalf("the proposal of %s on %s answered %d %s", id, line, response.Code, response.Body.String())
	}
	if problem := conformanceProblem("review.submitProposal", response.Body.Bytes()); problem != "" {
		env.t.Errorf("the proposal does not conform: %s", problem)
	}
	return stored.ProposalID
}

func (env *writeEnv) proposalRecord(id string) proposalRecordWire {
	env.t.Helper()
	response := env.post("review.proposals", `[]`)
	var records []proposalRecordWire
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &records) != nil {
		env.t.Fatalf("review.proposals answered %d %s", response.Code, response.Body.String())
	}
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	env.t.Fatalf("review.proposals lists no proposal %s: %+v", id, records)
	return proposalRecordWire{}
}

// shownFor is what the queue, the list and the panel each show as the review
// of id, as written.
func (env *writeEnv) shownFor(id string) (queue, list, panel string) {
	env.t.Helper()
	for _, item := range env.queueOf("2026-03").Items {
		if item.TransactionID == id {
			queue = compactJSON(env.t, item.Pending)
		}
	}
	list = compactJSON(env.t, env.listedIn("2026-03")[id]["pending"])
	response := env.post("transactions.detail", `["`+id+`"]`)
	var detail struct {
		Pending json.RawMessage `json:"pending"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &detail) != nil {
		env.t.Fatalf("the panel of %s answered %d %s", id, response.Code, response.Body.String())
	}
	return queue, list, compactJSON(env.t, detail.Pending)
}

func (env *writeEnv) expectShown(id, want string) {
	env.t.Helper()
	queue, list, panel := env.shownFor(id)
	if queue != want || list != want || panel != want {
		env.t.Errorf("%s shows\n  queue %s\n  list  %s\n  panel %s\nwant %s everywhere", id, queue, list, panel, want)
	}
}

func (env *writeEnv) justificationOfThePanel(id string) *string {
	env.t.Helper()
	var detail transactionDetailWire
	if err := json.Unmarshal(env.post("transactions.detail", `["`+id+`"]`).Body.Bytes(), &detail); err != nil {
		env.t.Fatal(err)
	}
	return detail.Justification
}

// theModelSays stores a suggestion of the configured model for id.
func (env *writeEnv) theModelSays(id, line string, confidence int64, why string) {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(env.t.Context(), []string{id})
	if err != nil {
		env.t.Fatal(err)
	}
	lines, err := linesOfMonth(env.t.Context(), view, "2026-03")
	if err != nil {
		env.t.Fatal(err)
	}
	named, found := lines.idOf(line)
	if !found {
		env.t.Fatalf("March has no line %s", line)
	}
	err = env.scratch.Database.RecordModelSuggestions(env.t.Context(), "test-model", fixtureNow, []authored.ModelSuggestionInput{
		{Key: keys[id], Line: &named, Confidence: &confidence, Justification: &why},
	})
	if err != nil {
		env.t.Fatal(err)
	}
}

func (env *writeEnv) modelRowOf(id string) authored.ModelSuggestion {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(env.t.Context(), []string{id})
	if err != nil {
		env.t.Fatal(err)
	}
	rows, err := view.ModelSuggestions(env.t.Context(), []authored.TxKey{keys[id]})
	if err != nil {
		env.t.Fatal(err)
	}
	return rows[keys[id]]
}

func (env *writeEnv) changeOf(id string) authoredChange {
	env.t.Helper()
	for _, change := range env.scratch.Changes(env.t) {
		if strconv.FormatInt(change.ID, 10) == id {
			return authoredChange{author: change.Author, method: change.Method}
		}
	}
	env.t.Fatalf("the journal holds no change %s", id)
	return authoredChange{}
}

type authoredChange struct{ author, method string }

func TestALineAnAgentProposesIsWhatTheQueueTheListAndThePanelSuggestAndChangesNothing(t *testing.T) {
	env, _ := reviewExamples(t)
	env.expectShown(reviewP1, pendingOfP1)
	canonical, journal := env.scratch.Canonical(t), env.scratch.Journal(t)

	id := env.proposed(reviewP1, "Contas da casa", agentReason)

	if env.scratch.Canonical(t) != canonical || env.scratch.Journal(t) != journal {
		t.Errorf("submitting recorded a change or moved a journaled table:\n%s", differences(canonical, env.scratch.Canonical(t)))
	}
	env.expectShown(reviewP1, agentOfP1)
	if got := env.justificationOfThePanel(reviewP1); got == nil || *got != agentReason {
		t.Errorf("the panel's justification is %v, want the agent's reason", got)
	}
	env.expectShown(reviewP2, `{"reason":"PIX para pessoa sem apelido","suggestion":null,"confidence":null,"justification":null}`)
	if record := env.proposalRecord(id); record.Outcome != "waiting" || record.Change == nil || record.Change.Method != proposedLineMethod || record.Proposer == nil || *record.Proposer != agentName {
		t.Errorf("review.proposals lists %+v, want the waiting proposal of transactions.setLine", record)
	}
	var weekly struct {
		Approvals []approvalRequest `json:"approvals"`
	}
	if err := json.Unmarshal(env.post("review.weekly", `["2026-03"]`).Body.Bytes(), &weekly); err != nil || len(weekly.Approvals) != 0 {
		t.Errorf("the approvals card lists %+v (%v), want none: the proposal is the queue's", weekly.Approvals, err)
	}
	if line := env.storedAbout(reviewP1).State.Line; line != nil {
		t.Errorf("the transaction is on line %d, but nobody accepted anything", *line)
	}
}

func TestTheAgentsLineShowsAboveTheModelsAndTheRulesAndTheModelShowsAgainWhenItIsClosed(t *testing.T) {
	env, _ := reviewExamples(t)
	env.theModelSays(reviewP1, "Mercado", 90, "o estabelecimento vende mantimentos")
	model := `{"reason":"Baixa confiança","suggestion":"Mercado","confidence":90,"justification":"o estabelecimento vende mantimentos"}`
	env.expectShown(reviewP1, model)

	id := env.proposed(reviewP1, "Contas da casa", agentReason)
	env.expectShown(reviewP1, agentOfP1)

	answer := env.accepted(reviewP1, pendingAnswer{Line: "Restaurantes"})
	record := env.proposalRecord(id)
	if record.Outcome != "refused" || record.Refusal != nil || record.ChangeID != nil || record.StaleReason != nil {
		t.Errorf("after the person picked another line the proposal reads %+v, want refused with no text", record)
	}
	if row := env.modelRowOf(reviewP1); row.Answer != "" || row.AnswerChange != nil {
		t.Errorf("an answer was recorded on the model's row while the agent's line was shown: %+v", row)
	}

	if response := env.undo(answer.ChangeID); response.Code != http.StatusOK {
		t.Fatalf("the undo answered %d %s", response.Code, response.Body.String())
	}
	env.expectShown(reviewP1, model)
	if record := env.proposalRecord(id); record.Outcome != "refused" {
		t.Errorf("the undo reopened the proposal: %+v", record)
	}
}

func TestAcceptingTheAgentsLineIsThePersonsChangeAndLeavesTheModelsRowAlone(t *testing.T) {
	env, _ := reviewExamples(t)
	env.theModelSays(reviewP1, "Mercado", 90, "o estabelecimento vende mantimentos")
	id := env.proposed(reviewP1, "Contas da casa", agentReason)

	answer := env.accepted(reviewP1, pendingAnswer{Line: "Contas da casa"})

	if change := env.changeOf(answer.ChangeID); change.author != "person" || change.method != acceptPendingMethod {
		t.Errorf("the accept is recorded as %+v, want the person's review.acceptPending", change)
	}
	if history := env.panelOf(reviewP1).History; len(history) == 0 || history[0].Text != "Você aceitou Contas da casa" {
		t.Errorf("the history reads %+v, want Você aceitou Contas da casa", history)
	}
	if line := env.storedAbout(reviewP1).State.Line; line == nil || env.storedAbout(reviewP1).State.ClassifiedBy != authored.AuthorPerson {
		t.Errorf("the transaction is on %v by %q, want a line by the person", line, env.storedAbout(reviewP1).State.ClassifiedBy)
	}
	record := env.proposalRecord(id)
	if record.Outcome != "approved" || record.ChangeID == nil || *record.ChangeID != answer.ChangeID || record.Undone || record.Refusal != nil {
		t.Errorf("the proposal reads %+v, want approved by change %s", record, answer.ChangeID)
	}
	if row := env.modelRowOf(reviewP1); row.Answer != "" || row.AnswerChange != nil {
		t.Errorf("accepting the agent's line recorded an answer on the model's row: %+v", row)
	}

	if response := env.undo(answer.ChangeID); response.Code != http.StatusOK {
		t.Fatalf("the undo answered %d %s", response.Code, response.Body.String())
	}
	env.expectShown(reviewP1, `{"reason":"Baixa confiança","suggestion":"Mercado","confidence":90,"justification":"o estabelecimento vende mantimentos"}`)
	if record := env.proposalRecord(id); record.Outcome != "approved" || !record.Undone {
		t.Errorf("after the undo the proposal reads %+v, want approved and undone", record)
	}
}

func TestAcceptingThePersonsOwnLineAfterTheRuleWasShownIsStillAChangeOfLine(t *testing.T) {
	env, _ := reviewExamples(t)
	env.proposed(reviewP1, "Contas da casa", agentReason)
	env.accepted(reviewP1, pendingAnswer{Line: "Mercado"})
	if history := env.panelOf(reviewP1).History; len(history) == 0 || history[0].Text != "Você alterou para Mercado" {
		t.Errorf("the history reads %+v, want Você alterou para Mercado", history)
	}
}

func TestSubmittingALineStoresNothingForAnUnknownLineANonWaitingTransactionOrAnUnknownOne(t *testing.T) {
	env, _ := reviewExamples(t)
	before := env.scratch.Everything(t)

	expectRefused(t, "a line its month lacks", env.propose(reviewP1, "Inexistente", agentReason), "unknown_line")
	expectError(t, "a transaction that waits for nothing", env.propose(reviewSettled, "Mercado", agentReason), http.StatusForbidden, "read_only")
	if response := env.propose("tx-nobody", "Mercado", agentReason); response.Code != http.StatusOK || response.Body.String() != "null" {
		t.Errorf("an unknown transaction answered %d %s, want 200 null", response.Code, response.Body.String())
	}
	if after := env.scratch.Everything(t); after != before {
		t.Errorf("a refused proposal changed the file:\n%s", differences(before, after))
	}
}

func TestASecondProposalClosesTheFirstAsStaleAndIsTheOneTheQueueShows(t *testing.T) {
	env, _ := reviewExamples(t)
	first := env.proposed(reviewP1, "Contas da casa", agentReason)
	other := env.proposed(reviewP3, "Mercado", "Compra de mantimentos")
	second := env.proposed(reviewP1, "Mercado", "Reli o recibo: é mantimento")

	if record := env.proposalRecord(first); record.Outcome != "stale" || record.StaleReason != nil || record.ChangeID != nil || record.Refusal != nil || record.AnsweredAt == nil {
		t.Errorf("the first proposal reads %+v, want stale with no reason", record)
	}
	if record := env.proposalRecord(second); record.Outcome != "waiting" {
		t.Errorf("the second proposal reads %+v, want waiting", record)
	}
	if record := env.proposalRecord(other); record.Outcome != "waiting" {
		t.Errorf("a proposal for another transaction reads %+v, want waiting", record)
	}
	env.expectShown(reviewP1, `{"reason":"Baixa confiança","suggestion":"Mercado","confidence":null,"justification":"Reli o recibo: é mantimento"}`)
}

func TestConfirmingTheLineTheQueueShowsApprovesTheProposalAndTheLineChosenInThePanelRefusesIt(t *testing.T) {
	env, _ := reviewExamples(t)
	confirmed := env.proposed(reviewP1, "Contas da casa", agentReason)
	chosen := env.proposed(reviewP3, "Mercado", "Compra de mantimentos")

	change := changeIDOfAnswer(t, env.post("transactions.confirmLine", `["`+reviewP1+`"]`))
	if line := env.panelOf(reviewP1).Line; line == nil || *line != "Contas da casa" {
		t.Errorf("confirming put the transaction on %v, want the proposed line", line)
	}
	if record := env.proposalRecord(confirmed); record.Outcome != "approved" || record.ChangeID == nil || *record.ChangeID != change {
		t.Errorf("the confirmed proposal reads %+v, want approved by change %s", record, change)
	}

	env.lineChange(reviewP3, "Restaurantes")
	if record := env.proposalRecord(chosen); record.Outcome != "refused" || record.Refusal != nil || record.ChangeID != nil {
		t.Errorf("after the panel chose another line the proposal reads %+v, want refused with no text", record)
	}
	env.expectNothingWaitsFor(reviewP1, reviewP3)
}

func TestASelectionPutOnALineClosesEachProposalByItsOwnLine(t *testing.T) {
	env, _ := reviewExamples(t)
	other := env.proposed(reviewP1, "Contas da casa", agentReason)
	same := env.proposed(reviewP3, "Mercado", "Compra de mantimentos")

	change := changeIDOfAnswer(t, env.post("transactions.setLineForMany", `[["`+reviewP1+`","`+reviewP3+`"],"Mercado"]`))

	if record := env.proposalRecord(other); record.Outcome != "refused" || record.Refusal != nil {
		t.Errorf("the proposal of another line reads %+v, want refused with no text", record)
	}
	if record := env.proposalRecord(same); record.Outcome != "approved" || record.ChangeID == nil || *record.ChangeID != change {
		t.Errorf("the proposal of the line chosen reads %+v, want approved by change %s", record, change)
	}
}

func (env *writeEnv) expectNothingWaitsFor(ids ...string) {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(env.t.Context(), ids)
	if err != nil {
		env.t.Fatal(err)
	}
	var all []authored.TxKey
	for _, key := range keys {
		all = append(all, key)
	}
	waiting, err := view.QueueProposals(env.t.Context(), all)
	if err != nil || len(waiting) != 0 {
		env.t.Errorf("proposals still wait for %v: %+v (%v)", ids, waiting, err)
	}
}

func TestAllPurchasesHereClosesTheProposalOfAWaitingSiblingByWhereItMovesIt(t *testing.T) {
	for _, c := range []struct{ line, outcome string }{{"Mercado", "approved"}, {"Restaurantes", "stale"}} {
		t.Run(c.outcome, func(t *testing.T) {
			env, _ := reviewExamples(t)
			id := env.proposed(reviewP3, c.line, "Compra de mantimentos")
			origin := env.lineChange(reviewShopAgain, "Mercado").ChangeID

			if applied := env.applyToEstablishment(origin); applied.Changed != 1 {
				t.Fatalf("all purchases here moved %d, want the waiting sibling", applied.Changed)
			}

			record := env.proposalRecord(id)
			if record.Outcome != c.outcome || record.Refusal != nil || record.StaleReason != nil {
				t.Errorf("the proposal reads %+v, want %s with no text", record, c.outcome)
			}
			if c.outcome == "approved" && (record.ChangeID == nil || *record.ChangeID != origin) {
				t.Errorf("the proposal was approved by change %v, want the extended change %s", record.ChangeID, origin)
			}
			if c.outcome == "stale" && record.ChangeID != nil {
				t.Errorf("a stale proposal carries change %v", record.ChangeID)
			}
		})
	}
}

func TestLinkingTheCreditAsAReimbursementClosesItsProposalAsStale(t *testing.T) {
	env, full := newProposalBench(t)
	rulesLeave(t, env.scratch, full.Path, waitingReview{"tx-pix-in", string(classrules.ReasonUnnamedPerson), "", nil})
	if response := env.post("transactions.setReimbursable", `["tx-last-of-february",true]`); response.Code != http.StatusOK {
		t.Fatalf("marking the purchase answered %d %s", response.Code, response.Body.String())
	}
	id := env.proposed("tx-pix-in", "Feira", "É a venda de um item usado")

	if response := env.post("transactions.linkReimbursement", `["tx-last-of-february","tx-pix-in"]`); response.Code != http.StatusOK {
		t.Fatalf("the link answered %d %s", response.Code, response.Body.String())
	}

	if record := env.proposalRecord(id); record.Outcome != "stale" || record.StaleReason != nil || record.ChangeID != nil {
		t.Errorf("the credit's proposal reads %+v, want stale with no reason", record)
	}
}

func TestTheApprovalsCardsMethodsLeaveAQueueProposalWaiting(t *testing.T) {
	env, _ := reviewExamples(t)
	id := env.proposed(reviewP1, "Contas da casa", agentReason)
	before := env.scratch.Everything(t)

	for _, call := range []struct{ method, arguments string }{
		{"review.approve", `["` + id + `"]`},
		{"review.refuse", `["` + id + `","não"]`},
	} {
		if response := env.post(call.method, call.arguments); response.Code != http.StatusOK || response.Body.String() != "null" {
			t.Errorf("%s answered %d %s, want 200 null", call.method, response.Code, response.Body.String())
		}
	}
	if after := env.scratch.Everything(t); after != before {
		t.Errorf("the card's methods changed the file:\n%s", differences(before, after))
	}
	env.expectShown(reviewP1, agentOfP1)
}

const (
	queueProposalArguments = `[{"change":{"method":"transactions.setLine","arguments":["tx-boleto","Delivery"]},` +
		`"reason":"Recibo da entrega no e-mail","proposer":"agente-revisor"}]`
	queueProposalUnknownLine = `[{"change":{"method":"transactions.setLine","arguments":["tx-boleto","Inexistente"]},` +
		`"reason":"Recibo da entrega no e-mail","proposer":"agente-revisor"}]`
)

// queueProposalWriteCases are the agent's line in the harness: its submission,
// which changes the proposals alone, the accept that answers it, recorded and
// undone as any accept is, and the line the month lacks.
var queueProposalWriteCases = []writeCase{
	{method: "review.submitProposal", arguments: queueProposalArguments, expects: expectsProposal,
		state: states(lineVersions, overTheFullLedger(reviewsToAccept))},
	{method: acceptPendingMethod, arguments: `["tx-boleto",{"line":"Delivery","always":false,"nickname":"","reason":""}]`, expects: expectsRecorded,
		state: states(lineVersions, overTheFullLedger(reviewsToAccept)), before: []writeCall{{"review.submitProposal", queueProposalArguments}}},
	{method: "review.submitProposal", arguments: queueProposalUnknownLine, expects: expectsRefusal, reason: reasonUnknownLine,
		state: states(lineVersions, overTheFullLedger(reviewsToAccept))},
}

// aLineProposedOverTheThirdState leaves the dollar purchase waiting for a
// review in which the rules suggest Delivery, and a line an agent proposes for
// it that is neither the rules' nor the model's.
func aLineProposedOverTheThirdState(t *testing.T, scratch *authoredtest.Scratch, full *ledgertest.Fixture) {
	t.Helper()
	rulesLeave(t, scratch, full.Path, waitingReview{txUnconverted, string(classrules.ReasonLowConfidence), "Delivery", authoredtest.Some[int64](61)})
	view := scratch.Database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(t.Context(), []string{txUnconverted})
	if err != nil {
		t.Fatal(err)
	}
	lines, err := linesOfMonth(t.Context(), view, "2026-03")
	if err != nil {
		t.Fatal(err)
	}
	feira, found := lines.idOf("Feira")
	if !found {
		t.Fatal("March has no line Feira")
	}
	basedOn, err := view.NewestChangeID(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	authoredtest.InsertQueueProposal(t, scratch.Database, authored.ProposalDraft{
		SubmittedAt: fixtureNow, Proposer: agentName, Method: proposedLineMethod,
		Arguments: `["` + txUnconverted + `","Feira"]`, Reason: agentReason, Title: "Pôr a compra em dólar em Feira", BasedOn: basedOn,
	}, keys[txUnconverted], feira)
}

func init() {
	writeCases = append(writeCases, queueProposalWriteCases...)
	// Over the third state: the queue, whose answer is the same for any month
	// since the queue is the account's, and the panel of the waiting purchase,
	// each with the agent's line shown.
	authoredReadCases = append(authoredReadCases,
		authoredReadOver("review.weekly", `["2026-05"]`, aLineProposedOverTheThirdState),
		authoredReadOver("transactions.detail", `["tx-unconverted"]`, aLineProposedOverTheThirdState),
	)
	secrets = append(secrets, "Recibo da entrega", "transactions.setLine")
}
