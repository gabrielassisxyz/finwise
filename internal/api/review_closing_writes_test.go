package api

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

func init() {
	writeCases = append(writeCases, closingWriteCases...)
	secrets = append(secrets,
		// The steps the closing's write cases tick.
		"leftover", "pending",
	)
}

// closingWriteCases tick a step of the invented state's open March, and keep
// and cancel the third state's subscription under review, each recorded and
// undone; then every call that answers null: the first step and a closed
// month's, subscriptions the database does not have, a keep already made and
// a cancel of one already cancelled.
var closingWriteCases = []writeCase{
	{method: markStepDoneMethod, arguments: `["2026-03","leftover"]`, expects: expectsRecorded, state: startFromTheInventedState},
	{method: keepSubscriptionMethod, arguments: `["2026-03","4"]`, expects: expectsRecorded, state: subscriptionsKept},
	{method: cancelSubscriptionMethod, arguments: `["4"]`, expects: expectsRecorded, state: subscriptionsKept},

	{method: markStepDoneMethod, arguments: `["2026-03","pending"]`, expects: expectsNothing, state: startFromTheInventedState},
	{method: markStepDoneMethod, arguments: `["2026-01","leftover"]`, expects: expectsNothing, state: startFromTheInventedState},
	{method: keepSubscriptionMethod, arguments: `["2026-03","999"]`, expects: expectsNothing, state: subscriptionsKept},
	{method: keepSubscriptionMethod, arguments: `["2026-03","sub-1"]`, expects: expectsNothing, state: subscriptionsKept},
	{method: keepSubscriptionMethod, arguments: `["2026-03",""]`, expects: expectsNothing, state: subscriptionsKept},
	{method: keepSubscriptionMethod, arguments: `["2026-03","4"]`, expects: expectsNothing, state: subscriptionsKept,
		before: []writeCall{{keepSubscriptionMethod, `["2026-03","4"]`}}},
	{method: cancelSubscriptionMethod, arguments: `["999"]`, expects: expectsNothing, state: subscriptionsKept},
	{method: cancelSubscriptionMethod, arguments: `["sub-1"]`, expects: expectsNothing, state: subscriptionsKept},
	{method: cancelSubscriptionMethod, arguments: `[""]`, expects: expectsNothing, state: subscriptionsKept},
	{method: cancelSubscriptionMethod, arguments: `["2"]`, expects: expectsNothing, state: subscriptionsKept},
}

// expectNothingRecorded posts a write that must answer null and leave the
// journal as it was.
func expectNothingRecorded(t *testing.T, env *settingsEnv, method, arguments string) {
	t.Helper()
	counts := journalCounts(t, env.scratch)
	status, body := env.answer(method, arguments)
	if status != http.StatusOK || string(body) != "null" {
		t.Errorf("%s %s answered %d %s, want null", method, arguments, status, body)
	}
	if after := journalCounts(t, env.scratch); after != counts {
		t.Errorf("%s %s moved the journal from %v to %v", method, arguments, counts, after)
	}
}

func TestTickingAStepKeepsItsRowForThatMonthAloneAndTheUndoTakesItBack(t *testing.T) {
	example := newClosingExample(t, 350)
	before := example.scratch.Canonical(t)
	id := example.change(markStepDoneMethod, `["2026-03","leftover"]`)
	effects := example.scratch.EffectsOf(t, changeNumber(t, id))
	if len(effects) != 1 || effects[0].Table != "closing_steps" || effects[0].Before != "" ||
		!strings.Contains(effects[0].After, `"2026-03"`) || !strings.Contains(effects[0].After, `"leftover"`) {
		t.Errorf("the change's effects are %+v, want the row (2026-03, leftover) appearing", effects)
	}
	if march, april := closingIn(t, example.readEnv, "2026-03"), closingIn(t, example.readEnv, "2026-04"); march.stepState("leftover") != "done" ||
		april.stepState("leftover") != "todo" {
		t.Errorf("leftover reads %s in 2026-03 and %s in 2026-04", march.stepState("leftover"), april.stepState("leftover"))
	}
	example.undo(id)
	if after := example.scratch.Canonical(t); after != before {
		t.Errorf("the undo left the tables otherwise:\n%s", differences(before, after))
	}
	if march := closingIn(t, example.readEnv, "2026-03"); march.stepState("leftover") != "todo" {
		t.Errorf("after the undo leftover reads %s", march.stepState("leftover"))
	}
}

func TestTickingAStepAgainAnswersAChangeThatHoldsNothing(t *testing.T) {
	example := newClosingExample(t, 350)
	first := example.change(markStepDoneMethod, `["2026-03","leftover"]`)
	marked := example.scratch.Canonical(t)
	again := example.change(markStepDoneMethod, `["2026-03","leftover"]`)
	if again == first {
		t.Fatalf("the second tick answered the first change %s", first)
	}
	if effects := example.scratch.EffectsOf(t, changeNumber(t, again)); len(effects) != 0 {
		t.Errorf("the second tick recorded %+v", effects)
	}
	example.undo(again)
	if after := example.scratch.Canonical(t); after != marked {
		t.Errorf("the undo of the empty change moved the tables:\n%s", differences(marked, after))
	}
}

func TestTheStepsThatFollowTheDataAndAClosedMonthsStepsAnswerNullAndRecordNothing(t *testing.T) {
	example := newClosingExample(t, 350)
	authoredtest.CloseMonth(t, example.scratch.Database, fixtureNow, "2026-02", "2026-03-02")
	expectNothingRecorded(t, example.settingsEnv, markStepDoneMethod, `["2026-03","pending"]`)
	expectNothingRecorded(t, example.settingsEnv, markStepDoneMethod, `["2026-03","close"]`)
	for _, step := range closingStepOrder {
		expectNothingRecorded(t, example.settingsEnv, markStepDoneMethod, `["2026-02","`+step+`"]`)
	}
	for _, arguments := range []string{`["2026-03","xyz"]`, `["2026-13","leftover"]`, `["2026-03"]`} {
		before := example.scratch.State(t)
		expectError(t, arguments, example.call("/api/review/markStepDone", arguments, example.cookie), http.StatusBadRequest, "bad_request")
		if after := example.scratch.State(t); after != before {
			t.Errorf("%s changed the database:\n%s", arguments, differences(before, after))
		}
	}
}

// A future month, a month before any budget and the snapshot gone are no
// reason to refuse a tick.
func TestATickReadsNoLedgerAndNoRuleRefusesItsMonth(t *testing.T) {
	example := newClosingExample(t, 350)
	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), fixtureNow), withDatabase(example.scratch.Database))
	alone := &settingsEnv{readEnv: &readEnv{testEnv: missing, cookie: missing.unlock()}, scratch: example.scratch}
	for _, month := range []string{"2026-03", "2027-01", "2025-06"} {
		alone.change(markStepDoneMethod, `["`+month+`","report"]`)
	}
	ticked := 0
	for _, month := range []brday.Month{"2026-03", "2027-01", "2025-06"} {
		view := example.scratch.Database.View()
		steps, err := view.ClosingStepsDone(t.Context(), month)
		view.Close()
		if err != nil {
			t.Fatal(err)
		}
		if steps["report"] {
			ticked++
		}
	}
	if ticked != 3 {
		t.Errorf("%d of the three months kept the tick", ticked)
	}
}

func decided(subscription authored.Subscription) string {
	return string(subscription.Status) + " " + string(subscription.StatusDay) + " " + string(subscription.ReviewOn)
}

func dueIDs(seen closingSeen) []string {
	ids := []string{}
	for _, due := range seen.SubscriptionsToReview {
		ids = append(ids, due.ID)
	}
	return ids
}

func TestKeepingASubscriptionReviewsItSixMonthsAfterTheMonthsEndAndTheUndoPutsItBack(t *testing.T) {
	env := newSubscriptionsToReviewEnv(t)
	before := env.scratch.Canonical(t)
	if got := decided(storedSubscription(t, env.scratch.Database, 1)); got != "Analisar 2026-03-02 2026-03-15" {
		t.Fatalf("S1 starts as %s", got)
	}
	status, body := env.answer(keepSubscriptionMethod, `["2026-03","1"]`)
	id, answered := changeIDIn(body)
	if want := `{"changeId":"` + id + `","name":"Streaming","nextReview":"2026-09-30"}`; status != http.StatusOK || !answered || string(body) != want {
		t.Fatalf("the keep answered %d %s, want %s", status, body, want)
	}
	if got := decided(storedSubscription(t, env.scratch.Database, 1)); got != "Manter 2026-03-20 2026-09-30" {
		t.Errorf("after the keep S1 is %s", got)
	}
	if got := dueIDs(closingIn(t, env.readEnv, "2026-03")); strings.Join(got, ",") != "4" {
		t.Errorf("March lists %v after the keep", got)
	}
	env.undo(id)
	if got := decided(storedSubscription(t, env.scratch.Database, 1)); got != "Analisar 2026-03-02 2026-03-15" {
		t.Errorf("after the undo S1 is %s", got)
	}
	if after := env.scratch.Canonical(t); after != before {
		t.Errorf("the undo left the tables otherwise:\n%s", differences(before, after))
	}
}

func TestTheNextReviewKeepsTheLastDayHeldToAShorterMonth(t *testing.T) {
	for month, want := range map[brday.Month]brday.Day{
		"2026-11": "2027-05-30", "2026-03": "2026-09-30", "2026-08": "2027-02-28", "2026-02": "2026-08-28",
	} {
		if got, err := nextReviewAfter(month); err != nil || got != want {
			t.Errorf("a keep in %s reviews on %s (%v), want %s", month, got, err, want)
		}
	}
}

func TestCancellingASubscriptionMarksItTodayAndTheUndoRestoresBoth(t *testing.T) {
	env := newSubscriptionsToReviewEnv(t)
	before := env.scratch.Canonical(t)
	status, body := env.answer(cancelSubscriptionMethod, `["4"]`)
	id, answered := changeIDIn(body)
	if want := `{"changeId":"` + id + `","name":"Jornal"}`; status != http.StatusOK || !answered || string(body) != want {
		t.Fatalf("the cancel answered %d %s, want %s", status, body, want)
	}
	if got := decided(storedSubscription(t, env.scratch.Database, 4)); got != "Cancelar 2026-03-20 2026-03-31" {
		t.Errorf("after the cancel S4 is %s", got)
	}
	if got := dueIDs(closingIn(t, env.readEnv, "2026-03")); strings.Join(got, ",") != "1" {
		t.Errorf("March lists %v after the cancel", got)
	}
	env.undo(id)
	if got := decided(storedSubscription(t, env.scratch.Database, 4)); got != "Manter 2026-03-02 2026-03-31" {
		t.Errorf("after the undo S4 is %s", got)
	}
	if after := env.scratch.Canonical(t); after != before {
		t.Errorf("the undo left the tables otherwise:\n%s", differences(before, after))
	}
}

// The subscription screen's setters refuse an unknown id with not_found; these
// two answer null, as their results allow, and so does a decision already made.
func TestAnUnknownSubscriptionOrADecisionAlreadyMadeAnswersNullAndRecordsNothing(t *testing.T) {
	env := newSubscriptionsToReviewEnv(t)
	for _, id := range []string{"999", "sub-1", "", "07"} {
		expectNothingRecorded(t, env, keepSubscriptionMethod, `["2026-03","`+id+`"]`)
		expectNothingRecorded(t, env, cancelSubscriptionMethod, `["`+id+`"]`)
	}
	env.change(keepSubscriptionMethod, `["2026-03","1"]`)
	expectNothingRecorded(t, env, keepSubscriptionMethod, `["2026-03","1"]`)
	expectNothingRecorded(t, env, cancelSubscriptionMethod, `["2"]`)
	// Kept already, S3 still takes the review day of another month.
	if id := env.change(keepSubscriptionMethod, `["2026-04","3"]`); len(env.scratch.EffectsOf(t, changeNumber(t, id))) != 1 {
		t.Errorf("a keep of the kept S3 recorded %+v, want its review day alone", env.scratch.EffectsOf(t, changeNumber(t, id)))
	}
	if got := decided(storedSubscription(t, env.scratch.Database, 3)); got != "Manter 2026-03-02 2026-10-30" {
		t.Errorf("S3 is %s", got)
	}
}

func TestUndoingAKeepWhileALaterStatusIsInForceIsStale(t *testing.T) {
	env := newSubscriptionsToReviewEnv(t)
	kept := env.change(keepSubscriptionMethod, `["2026-03","1"]`)
	env.change("subscriptions.setStatus", `["1","Backup e cancelar"]`)
	before := env.scratch.State(t)
	expectRefused(t, "the undo of the keep", env.call("/api/changes/undo", `["`+kept+`"]`, env.cookie), string(reasonStaleChange))
	if after := env.scratch.State(t); after != before {
		t.Errorf("the refused undo changed the database:\n%s", differences(before, after))
	}
}

func TestKeepingAndCancellingReadNoLedgerAndAClosedMonthDoesNotRefuseThem(t *testing.T) {
	env := newSubscriptionsToReviewEnv(t)
	authoredtest.CloseMonth(t, env.scratch.Database, fixtureNow, "2026-02", "2026-03-02")
	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), fixtureNow), withDatabase(env.scratch.Database))
	alone := &settingsEnv{readEnv: &readEnv{testEnv: missing, cookie: missing.unlock()}, scratch: env.scratch}
	alone.change(keepSubscriptionMethod, `["2026-02","1"]`)
	alone.change(cancelSubscriptionMethod, `["4"]`)
	if got := decided(storedSubscription(t, env.scratch.Database, 1)); got != "Manter 2026-03-20 2026-08-28" {
		t.Errorf("S1 is %s", got)
	}
	if got := decided(storedSubscription(t, env.scratch.Database, 4)); got != "Cancelar 2026-03-20 2026-03-31" {
		t.Errorf("S4 is %s", got)
	}
}

func TestWithoutADatabaseTheClosingsWritesAreReadOnlyBeforeTheirBodyIsRead(t *testing.T) {
	env := newTestEnv(t, withLedger(subscriptionsLedger(t).Path, fixtureNow))
	cookie := env.unlock()
	for _, method := range []string{markStepDoneMethod, keepSubscriptionMethod, cancelSubscriptionMethod} {
		domain, name := splitMethod(t, method)
		if reads, status := readsOfBody(env, "/api/"+domain+"/"+name, cookie); reads != 0 || status != http.StatusForbidden {
			t.Errorf("%s read its body %d times and answered %d, want 0 and 403", method, reads, status)
		}
	}
}
