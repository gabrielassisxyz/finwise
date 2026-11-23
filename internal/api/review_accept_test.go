package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/classrules"
)

// reviewsToAccept leaves three rows of the full ledger waiting for review: the
// boleto suggesting Contas da casa, the instalment at a place never seen and
// the PIX to a person with no nickname.
func reviewsToAccept(a authoring) error {
	recorder := a.writes.Recorder
	waiting := []struct {
		id, reason string
		line       *authored.LineID
		confidence *int64
	}{
		{txBoleto, string(classrules.ReasonLowConfidence), a.line("Contas da casa"), authoredtest.Some[int64](64)},
		{txInstalment, string(classrules.ReasonNewEstablishment), nil, nil},
		{txPixOut, string(classrules.ReasonUnnamedPerson), nil, nil},
	}
	for _, review := range waiting {
		key, err := a.key(review.id)
		if err != nil {
			return err
		}
		if err := recorder.SetPendingReview(a.ctx, key, review.reason, review.line, review.confidence); err != nil {
			return err
		}
	}
	return nil
}

// reviewAcceptWriteCases are the accept in the harness: as suggested, on
// another line with a reason and the rule, and for a person with a nickname,
// each recorded and undone; a transaction that does not wait; and a line its
// month lacks.
var reviewAcceptWriteCases = []writeCase{
	{method: acceptPendingMethod, arguments: `["tx-boleto",{"line":"Contas da casa","always":false,"nickname":"","reason":""}]`,
		expects: expectsRecorded, state: states(lineVersions, overTheFullLedger(reviewsToAccept))},
	{method: acceptPendingMethod, arguments: `["tx-instalment",{"line":"Feira","always":true,"nickname":"","reason":"  vai ao mercado "}]`,
		expects: expectsRecorded, state: states(lineVersions, overTheFullLedger(reviewsToAccept))},
	{method: acceptPendingMethod, arguments: `["tx-pix-out",{"line":"Contas da casa","always":true,"nickname":" Aluguel ","reason":""}]`,
		expects: expectsRecorded, state: states(lineVersions, overTheFullLedger(reviewsToAccept))},
	{method: acceptPendingMethod, arguments: `["tx-salary",{"line":"Salário","always":false,"nickname":"","reason":""}]`,
		expects: expectsNothing, state: states(lineVersions, overTheFullLedger(reviewsToAccept))},
	{method: acceptPendingMethod, arguments: `["tx-boleto",{"line":"Inexistente","always":true,"nickname":"","reason":""}]`,
		expects: expectsRefusal, reason: reasonUnknownLine, state: states(lineVersions, overTheFullLedger(reviewsToAccept))},
}

func (env *writeEnv) accept(id string, answer pendingAnswer) *httptest.ResponseRecorder {
	env.t.Helper()
	return env.post(acceptPendingMethod, argumentsOf(id, answer))
}

// accepted is the answer of an accept that recorded a change.
func (env *writeEnv) accepted(id string, answer pendingAnswer) pendingAccepted {
	env.t.Helper()
	response := env.accept(id, answer)
	var result pendingAccepted
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.ChangeID == "" {
		env.t.Fatalf("the accept of %s answered %d %s", id, response.Code, response.Body.String())
	}
	if problem := conformanceProblem(acceptPendingMethod, response.Body.Bytes()); problem != "" {
		env.t.Errorf("the accept of %s does not conform: %s", id, problem)
	}
	return result
}

func nameIn(t *testing.T, row map[string]json.RawMessage) string {
	t.Helper()
	var name string
	if err := json.Unmarshal(row["name"], &name); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestAcceptingTheSuggestionPutsTheTransactionOnItsLineAndCountsIt(t *testing.T) {
	env, _ := reviewExamples(t)
	before := env.queueOf("2026-03").Resolved
	shown := nameIn(t, env.listedIn("2026-03")[reviewP1])
	result := env.accepted(reviewP1, pendingAnswer{Line: "Restaurantes"})
	if result.Name != shown {
		t.Errorf("the answer names %q, want the list's %q", result.Name, shown)
	}
	queue := env.queueOf("2026-03")
	if !slices.Equal(queue.ids(), []string{reviewP2, reviewP3}) || queue.Resolved != before+1 {
		t.Errorf("after the accept the queue holds %v and resolved %d, want P2 and P3 and %d", queue.ids(), queue.Resolved, before+1)
	}
	row := env.listedIn("2026-03")[reviewP1]
	for field, want := range map[string]string{"line": `"Restaurantes"`, "classifiedBy": `"Você"`, "confidence": "null", "pending": "null"} {
		if string(row[field]) != want {
			t.Errorf("the list answers %s %s, want %s", field, row[field], want)
		}
	}
	if history := env.panelOf(reviewP1).History; len(history) != 1 || history[0].Text != "Você aceitou Restaurantes" || history[0].Reason != nil {
		t.Errorf("the history is %+v, want the one step Você aceitou Restaurantes", history)
	}
}

func TestAcceptingAnotherLineKeepsTheTrimmedReasonOnItsStep(t *testing.T) {
	env, _ := reviewExamples(t)
	env.accepted(reviewP1, pendingAnswer{Line: "Mercado", Reason: "  vai ao mercado "})
	panel := env.panelOf(reviewP1)
	if len(panel.History) != 1 || panel.History[0].Text != "Você alterou para Mercado" || !textIs(panel.History[0].Reason, "vai ao mercado") {
		t.Errorf("the history is %+v, want Você alterou para Mercado with the reason", panel.History)
	}
	if panel.Reason != "vai ao mercado" || !textIs(panel.Line, "Mercado") {
		t.Errorf("the panel answers reason %q on %v, want vai ao mercado on Mercado", panel.Reason, panel.Line)
	}
}

func TestAlwaysForAnEstablishmentGivesItTheLineAndANoteAfterTheStep(t *testing.T) {
	env, _ := reviewExamples(t)
	shown := nameIn(t, env.listedIn("2026-03")[reviewP3])
	result := env.accepted(reviewP3, pendingAnswer{Line: "Mercado", Always: true, Nickname: "Loja"})
	if result.Name != shown {
		t.Errorf("the answer names %q, want the establishment's %q", result.Name, shown)
	}
	panel := env.panelOf(reviewP3)
	if panel.Counterparty == nil || !textIs(panel.Counterparty.DefaultLine, "Mercado") || panel.Counterparty.Nickname != nil {
		t.Errorf("the establishment is %+v, want Mercado as its line and no nickname", panel.Counterparty)
	}
	history := panel.History
	if len(history) != 2 || history[0].Text != "Regra criada: "+shown+" sempre em Mercado" || history[0].Undoable ||
		history[1].Text != "Você alterou para Mercado" || !history[1].Undoable {
		t.Errorf("the history, newest first, is %+v, want the note and then the step", history)
	}
	if again := env.storedAbout(reviewShopAgain); again.State.Line != nil {
		t.Errorf("another transaction of the establishment moved to line %d", *again.State.Line)
	}
}

// With a rule and a reason, the reason is the step's: the note records only
// that a rule was made.
func TestTheReasonOfAnAnswerWithARuleIsOnTheStepAndNotOnTheNote(t *testing.T) {
	env, _ := reviewExamples(t)
	env.accepted(reviewP3, pendingAnswer{Line: "Mercado", Always: true, Reason: "vai ao mercado"})
	history := env.panelOf(reviewP3).History
	if len(history) != 2 || history[0].Reason != nil || !textIs(history[1].Reason, "vai ao mercado") {
		t.Errorf("the history is %+v, want the reason on the step alone", history)
	}
}

func TestANicknameForAPersonNamesEveryTransactionOfThePersonAndAlwaysIsIgnored(t *testing.T) {
	env, _ := reviewExamples(t)
	result := env.accepted(reviewP2, pendingAnswer{Line: "Contas da casa", Always: true, Nickname: " Aluguel "})
	if result.Name != "Aluguel" {
		t.Errorf("the answer names %q, want Aluguel", result.Name)
	}
	listed := env.listedIn("2026-03")
	for _, id := range []string{reviewP2, reviewPixAgain} {
		if name := nameIn(t, listed[id]); name != "Aluguel" {
			t.Errorf("the list names %s %q, want Aluguel", id, name)
		}
	}
	panel := env.panelOf(reviewP2)
	if panel.Counterparty == nil || panel.Counterparty.DefaultLine != nil || len(panel.History) != 1 {
		t.Errorf("the person is %+v with the history %+v, want no default line and no note", panel.Counterparty, panel.History)
	}
	if rows := tableRows(t, env.scratch, "establishments"); len(rows) != 1 || rows[0] != `key="`+keyOfTheReviewFriend+`" nickname="Aluguel" default_line_id=NULL` {
		t.Errorf("establishments hold %v, want the person's nickname and no line", rows)
	}
}

func TestABlankNicknameLeavesThePersonsNicknameInPlace(t *testing.T) {
	env, _ := reviewExamples(t)
	changeIDOfAnswer(t, env.post("transactions.setNickname", argumentsOf(reviewP2, "Joaninha")))
	result := env.accepted(reviewP2, pendingAnswer{Line: "Contas da casa", Nickname: "   "})
	panel := env.panelOf(reviewP2)
	if result.Name != "Joaninha" || panel.Counterparty == nil || !textIs(panel.Counterparty.Nickname, "Joaninha") {
		t.Errorf("the answer names %q and the person is %+v, want Joaninha kept", result.Name, panel.Counterparty)
	}
}

// renamedLines saves the budget from 2026-02 with line L named Mercado, then
// from 2026-03 with L renamed Feira and a new M named Mercado, and leaves the
// February purchase waiting for review. It answers the ids of L and M.
func renamedLines(t *testing.T) (*writeEnv, authored.LineID, authored.LineID) {
	t.Helper()
	fixture := newReviewLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, utc(2026, time.February, 1, 15, 0), "2026-02", authored.Document{
		authoredtest.Group("var", authoredtest.Line("Mercado", 90_000)),
	})
	february := budgetIn(t, scratch.Database, "2026-02").Lines[0]
	march := authoredtest.DocumentOf(budgetIn(t, scratch.Database, "2026-02"))
	group(march, "var").Lines[0].Name = "Feira"
	group(march, "var").Lines = append(group(march, "var").Lines, authoredtest.Line("Mercado", 50_000))
	authoredtest.SaveBudget(t, scratch.Database, utc(2026, time.March, 1, 15, 0), "2026-03", march)
	var renamed authored.LineID
	for _, line := range budgetIn(t, scratch.Database, "2026-03").Lines {
		if line.Name == "Mercado" {
			renamed = line.ID
		}
	}
	rulesLeave(t, scratch, fixture.Path, waitingReview{reviewFebruary, string(classrules.ReasonNewEstablishment), "", nil})
	return newReviewEnv(t, fixture.Path, scratch, fixtureNow), february.ID, renamed
}

func TestAlwaysAfterARenameIsTheLineTheTransactionWentOnAndNotTheNameToday(t *testing.T) {
	env, old, renamed := renamedLines(t)
	if old == renamed {
		t.Fatal("the two Mercados are one line, so the test would prove nothing")
	}
	env.accepted(reviewFebruary, pendingAnswer{Line: "Mercado", Always: true})
	stored := env.storedAbout(reviewFebruary)
	if !lineIs(stored.State, old) {
		t.Errorf("the purchase is on %v, want February's Mercado %d", stored.State.Line, old)
	}
	if line := establishmentLine(t, env.scratch, "text:FEIRA EXEMPLO"); line == nil || *line != old {
		t.Errorf("the default line is %v, want %d and not %d", line, old, renamed)
	}
	if panel := env.panelOf(reviewFebruary); panel.Counterparty == nil || !textIs(panel.Counterparty.DefaultLine, "Feira") {
		t.Errorf("the panel names the default line %+v, want Feira", panel.Counterparty)
	}
}

func TestAlwaysWithALineTheCurrentMonthLacksIsStoredAndShownAsNone(t *testing.T) {
	fixture := newReviewLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, utc(2026, time.February, 1, 15, 0), "2026-02", authored.Document{
		authoredtest.Group("var", authoredtest.Line("Mercado", 90_000), authoredtest.Line("Papelaria", 5_000)),
	})
	authoredtest.SaveBudget(t, scratch.Database, utc(2026, time.March, 1, 15, 0), "2026-03", authored.Document{
		authoredtest.Group("var", authoredtest.Line("Mercado", 90_000)),
	})
	rulesLeave(t, scratch, fixture.Path, waitingReview{reviewFebruary, string(classrules.ReasonNewEstablishment), "", nil})
	env := newReviewEnv(t, fixture.Path, scratch, fixtureNow)
	stationery := budgetIn(t, scratch.Database, "2026-02").Lines[1]
	env.accepted(reviewFebruary, pendingAnswer{Line: "Papelaria", Always: true})
	if stored := env.storedAbout(reviewFebruary); !lineIs(stored.State, stationery.ID) {
		t.Errorf("the purchase is on %v, want Papelaria %d", stored.State.Line, stationery.ID)
	}
	if line := establishmentLine(t, scratch, "text:FEIRA EXEMPLO"); line == nil || *line != stationery.ID {
		t.Errorf("the default line is %v, want Papelaria %d", line, stationery.ID)
	}
	if panel := env.panelOf(reviewFebruary); panel.Counterparty == nil || panel.Counterparty.DefaultLine != nil {
		t.Errorf("the panel names the default line %+v, want none while March lacks it", panel.Counterparty)
	}
}

func establishmentLine(t *testing.T, scratch *authoredtest.Scratch, key string) *authored.LineID {
	t.Helper()
	view := scratch.Database.View()
	defer view.Close()
	kept, err := view.Establishments(t.Context(), []string{key})
	if err != nil {
		t.Fatal(err)
	}
	return kept[key].DefaultLine
}

func TestAcceptingWhatDoesNotWaitForReviewAnswersNullAndRecordsNothing(t *testing.T) {
	env, _ := reviewExamples(t)
	env.accepted(reviewP1, pendingAnswer{Line: "Restaurantes"})
	without := newReviewEnv(t, newReviewLedger(t, reviewP2).Path, env.scratch, fixtureNow)
	cases := []struct {
		label string
		env   *writeEnv
		id    string
	}{
		{"an id the ledger does not list", env, "no-such-tx"},
		{"a listed transaction with no review", env, reviewSettled},
		{"P1 a second time", env, reviewP1},
		{"a review whose row the snapshot does not list", without, reviewP2},
	}
	for _, c := range cases {
		before := env.scratch.Journal(t)
		response := c.env.accept(c.id, pendingAnswer{Line: "Mercado", Always: true, Nickname: "Joaninha", Reason: "porque sim"})
		if response.Code != http.StatusOK || response.Body.String() != "null" {
			t.Errorf("%s: answered %d %s, want 200 null", c.label, response.Code, response.Body.String())
		}
		if after := env.scratch.Journal(t); after != before {
			t.Errorf("%s: the journal changed:\n%s", c.label, differences(before, after))
		}
	}
}

func TestAcceptingAReviewOfAMonthWhoseBudgetHasNoLineAnswersNull(t *testing.T) {
	fixture := newReviewLedger(t)
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, utc(2026, time.February, 1, 15, 0), "2026-02", authoredtest.Emptied(reviewBudget()))
	savedReviewBudget(t, scratch)
	rulesLeave(t, scratch, fixture.Path, waitingReview{reviewFebruary, string(classrules.ReasonNewEstablishment), "", nil})
	env := newReviewEnv(t, fixture.Path, scratch, fixtureNow)
	before := scratch.Journal(t)
	response := env.accept(reviewFebruary, pendingAnswer{Line: "Mercado"})
	if response.Code != http.StatusOK || response.Body.String() != "null" || scratch.Journal(t) != before {
		t.Errorf("answered %d %s and the journal changed: %v, want 200 null and no row", response.Code, response.Body.String(), scratch.Journal(t) != before)
	}
}

func TestALineTheTransactionsMonthLacksIsRefusedAndChangesNoTable(t *testing.T) {
	env, _ := reviewExamples(t)
	for _, answer := range []pendingAnswer{{Line: "Lazer"}, {Line: "Lazer", Always: true}, {Line: ""}, {Line: "", Always: true}} {
		before := env.scratch.Everything(t)
		expectRefused(t, "the line "+answer.Line, env.accept(reviewP3, answer), string(reasonUnknownLine))
		if after := env.scratch.Everything(t); after != before {
			t.Errorf("the refusal of %+v changed the file:\n%s", answer, differences(before, after))
		}
	}
}

func TestTheAcceptAnswersAMissingSnapshotABadAnswerAndNoDatabaseAsEveryWriteDoes(t *testing.T) {
	env, _ := reviewExamples(t)
	missing := newReviewEnv(t, filepath.Join(t.TempDir(), "absent.db"), env.scratch, fixtureNow)
	before := env.scratch.Everything(t)
	expectError(t, "a missing snapshot", missing.accept(reviewP1, pendingAnswer{Line: "Restaurantes"}), http.StatusServiceUnavailable, "ledger_unavailable")
	for _, body := range []string{
		`["tx-p1",{"line":"Restaurantes","always":false,"nickname":"","reason":"","extra":1}]`,
		`["tx-p1",{"line":"Restaurantes","always":false,"nickname":""}]`,
		`["tx-p1"]`,
	} {
		expectError(t, body, env.post(acceptPendingMethod, body), http.StatusBadRequest, "bad_request")
	}
	if after := env.scratch.Everything(t); after != before {
		t.Errorf("a failed accept changed the file:\n%s", differences(before, after))
	}
	// With no review kept, the definition of what waits never reads the ledger.
	nothingWaits := authoredtest.Open(t, fixtureClock)
	savedReviewBudget(t, nothingWaits)
	expectError(t, "a missing snapshot with nothing waiting",
		newReviewEnv(t, filepath.Join(t.TempDir(), "absent.db"), nothingWaits, fixtureNow).accept(reviewP1, pendingAnswer{Line: "Restaurantes"}),
		http.StatusServiceUnavailable, "ledger_unavailable")
	readOnly := newTestEnv(t, withLedger(newReviewLedger(t).Path, fixtureNow))
	if reads, status := readsOfBody(readOnly, "/api/review/acceptPending", readOnly.unlock()); reads != 0 || status != http.StatusForbidden {
		t.Errorf("without the database the accept answered %d after %d reads of its body, want 403 after none", status, reads)
	}
}

// acceptShapes are the three answers the undo is checked over, each from a
// state where the place already had a line and the person a nickname.
var acceptShapes = []struct {
	id     string
	answer pendingAnswer
}{
	{reviewP1, pendingAnswer{Line: "Restaurantes"}},
	{reviewP3, pendingAnswer{Line: "Mercado", Always: true, Reason: "vai ao mercado"}},
	{reviewP2, pendingAnswer{Line: "Contas da casa", Nickname: "Aluguel"}},
}

func TestOneUndoTakesTheWholeAcceptBack(t *testing.T) {
	for _, shape := range acceptShapes {
		env, _ := reviewExamples(t)
		changeIDOfAnswer(t, env.post("transactions.setDefaultLine", argumentsOf(reviewP3, "Restaurantes")))
		changeIDOfAnswer(t, env.post("transactions.setNickname", argumentsOf(reviewP2, "Joaninha")))
		canonical, queue, panel := env.scratch.Canonical(t), env.queueOf("2026-03"), env.panelOf(shape.id)
		result := env.accepted(shape.id, shape.answer)
		if after := env.queueOf("2026-03"); slices.Contains(after.ids(), shape.id) || after.Resolved != queue.Resolved+1 {
			t.Errorf("%s: after the accept the queue is %+v", shape.id, after)
		}
		expectUndone(t, shape.id, env.undo(result.ChangeID))
		if got := env.scratch.Canonical(t); got != canonical {
			t.Errorf("%s: the undo left the journaled tables otherwise:\n%s", shape.id, differences(canonical, got))
		}
		if got := env.queueOf("2026-03"); !slices.Equal(got.ids(), queue.ids()) || got.Resolved != queue.Resolved {
			t.Errorf("%s: after the undo the queue is %+v, want %+v", shape.id, got, queue)
		}
		got := env.panelOf(shape.id)
		if len(got.History) != len(panel.History) || got.Reason != panel.Reason || !reflect.DeepEqual(got.Counterparty, panel.Counterparty) {
			t.Errorf("%s: after the undo the panel is %+v, want %+v", shape.id, got, panel)
		}
	}
}

// Taking the accept's step back restores what it did to the transaction alone:
// the review is back, the place's line and the person's nickname stay, and the
// accept's own undo is stale.
func TestTakingTheAcceptsStepBackLeavesTheRuleAndTheNicknameAndMakesItsUndoStale(t *testing.T) {
	for _, shape := range []struct {
		id     string
		answer pendingAnswer
		kept   func(counterparty *counterpartyDetail) bool
	}{
		{reviewP3, pendingAnswer{Line: "Mercado", Always: true}, func(c *counterpartyDetail) bool { return c != nil && textIs(c.DefaultLine, "Mercado") }},
		{reviewP2, pendingAnswer{Line: "Contas da casa", Nickname: "Aluguel"}, func(c *counterpartyDetail) bool { return c != nil && textIs(c.Nickname, "Aluguel") }},
	} {
		env, _ := reviewExamples(t)
		resolved := env.queueOf("2026-03").Resolved
		result := env.accepted(shape.id, shape.answer)
		history := env.panelOf(shape.id).History
		env.undoStep(shape.id, history[len(history)-1].ID)
		queue := env.queueOf("2026-03")
		if !slices.Equal(queue.ids(), []string{reviewP1, reviewP2, reviewP3}) || queue.Resolved != resolved {
			t.Errorf("%s: after the step was taken back the queue is %+v, want all three and resolved %d", shape.id, queue, resolved)
		}
		if panel := env.panelOf(shape.id); !shape.kept(panel.Counterparty) {
			t.Errorf("%s: the step's undo took the rule or the nickname away: %+v", shape.id, panel.Counterparty)
		}
		before := env.scratch.Everything(t)
		expectRefused(t, shape.id, env.undo(result.ChangeID), string(reasonStaleChange))
		if after := env.scratch.Everything(t); after != before {
			t.Errorf("%s: the stale undo changed the file:\n%s", shape.id, differences(before, after))
		}
	}
}

func TestAnAcceptTakenBackAndAnsweredAgainCountsOnce(t *testing.T) {
	env, _ := reviewExamples(t)
	if got := env.queueOf("2026-03").Resolved; got != 0 {
		t.Fatalf("resolved starts at %d", got)
	}
	env.accepted(reviewP1, pendingAnswer{Line: "Restaurantes"})
	if got := env.queueOf("2026-03"); got.Resolved != 1 || slices.Contains(got.ids(), reviewP1) {
		t.Errorf("after the accept the queue is %+v, want resolved 1 without P1", got)
	}
	env.undoStep(reviewP1, env.panelOf(reviewP1).History[0].ID)
	if got := env.queueOf("2026-03"); got.Resolved != 0 || !slices.Equal(got.ids(), []string{reviewP1, reviewP2, reviewP3}) {
		t.Errorf("after the step was taken back the queue is %+v, want resolved 0 with P1 back first", got)
	}
	env.accepted(reviewP1, pendingAnswer{Line: "Mercado"})
	if got := env.queueOf("2026-03"); got.Resolved != 1 || slices.Contains(got.ids(), reviewP1) {
		t.Errorf("after the second accept the queue is %+v, want resolved 1 without P1", got)
	}
}

// The accept is a third door to the nickname and the default line: it reaches
// them through the store functions the panel's writes end in, and never
// through the lookup by name in the current month.
func TestTheAcceptReachesTheNicknameAndTheDefaultLineThroughThePanelsStoreFunctions(t *testing.T) {
	accept := storeCallsOf(t, "review_accept.go", "recordAnswer")
	panel := slices.Concat(storeCallsOf(t, "establishment_writes.go", "setTransactionNickname"),
		storeCallsOf(t, "establishment_writes.go", "storeDefaultLine"))
	for _, call := range panel {
		if !slices.Contains(accept, call) {
			t.Errorf("the accept does not reach the store through %s, as the panel does; it calls %v", call, accept)
		}
	}
	if slices.Contains(accept, "storeDefaultLine") {
		t.Error("the accept looks its line up again by name in the current month")
	}
}
