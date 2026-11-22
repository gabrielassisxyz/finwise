package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// Every text and figure here is invented.

func detailField(t *testing.T, env *settingsEnv, id string, field ...string) string {
	t.Helper()
	return env.fieldOf("subscriptions.detail", `["`+id+`"]`, field...)
}

func createdOf(t *testing.T, env *settingsEnv, transactionID string) createdSubscription {
	t.Helper()
	status, body := env.answer("subscriptions.create", `["`+transactionID+`"]`)
	var created createdSubscription
	if status != http.StatusOK || json.Unmarshal(body, &created) != nil || created.ChangeID == "" {
		t.Fatalf("create %s answered %d %s", transactionID, status, body)
	}
	if problem := conformanceProblem("subscriptions.create", body); problem != "" {
		t.Errorf("create %s does not conform: %s", transactionID, problem)
	}
	return created
}

func storedSubscription(t *testing.T, database *authored.Database, id authored.SubscriptionID) authored.Subscription {
	t.Helper()
	view := database.View()
	defer view.Close()
	subscription, found, err := view.Subscription(t.Context(), id)
	if err != nil || !found {
		t.Fatalf("subscription %s: %t (%v)", id, found, err)
	}
	return subscription
}

// The bead's "done when": a subscription made from a purchase of the full
// ledger is read back, changed by each setter, and every change taken back
// leaves the database and the candidates as they were.
func TestASubscriptionIsCreatedReadBackChangedByEachSetterAndUndoneToTheStateBefore(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	before := env.scratch.Canonical(t)
	_, candidatesBefore := env.answer("subscriptions.candidates", `[]`)

	created := createdOf(t, env, txPending)
	if created.Name != "PADARIA EXEMPLO" {
		t.Errorf("created under the name %q", created.Name)
	}
	changes := []string{created.ChangeID}
	for field, want := range map[string]string{
		"status": `"Analisar"`, "cycle": `"Mensal"`, "amount": "18.50", "nextCharge": `"2026-04-19"`, "reviewOn": `"2026-06-20"`,
		"line": "null", "note": `""`, "charges": `[{"day":"2026-03-19","amount":18.50,"foreign":null}]`,
	} {
		if got := detailField(t, env, "1", field); got != want {
			t.Errorf("the new subscription's %s is %s, want %s", field, got, want)
		}
	}
	_, candidates := env.answer("subscriptions.candidates", `[]`)
	if string(candidates) == string(candidatesBefore) {
		t.Error("the purchase is still a candidate once its establishment has a subscription")
	}

	setters := []struct{ method, arguments, field, want string }{
		{"subscriptions.setStatus", `["1","Manter"]`, "status", `"Manter"`},
		{"subscriptions.setLine", `["1","Viagens"]`, "line", `"Viagens"`},
		{"subscriptions.setReviewDate", `["1","2026-04-01"]`, "reviewOn", `"2026-04-01"`},
		{"subscriptions.setNote", `["1","  Dividida com a casa, plano família "]`, "note", `"  Dividida com a casa, plano família "`},
	}
	for _, setter := range setters {
		changes = append(changes, env.change(setter.method, setter.arguments))
		if got := detailField(t, env, "1", setter.field); got != setter.want {
			t.Errorf("after %s the %s is %s, want %s", setter.method, setter.field, got, setter.want)
		}
	}
	for index := len(changes) - 1; index >= 0; index-- {
		env.undo(changes[index])
	}
	if got := env.scratch.Canonical(t); got != before {
		t.Errorf("after the undos the journaled tables are not what they were:\n%s", differences(before, got))
	}
	if _, candidates := env.answer("subscriptions.candidates", `[]`); string(candidates) != string(candidatesBefore) {
		t.Errorf("after the undo the candidates are %s, want %s", candidates, candidatesBefore)
	}
	expectAnswer(t, env.readEnv, "subscriptions.detail", `["1"]`, `null`)

	createdOf(t, env, txPending)
	expectAnswer(t, env.readEnv, "subscriptions.detail", `["1"]`, `null`)
	if got := detailField(t, env, "2", "name"); got != `"PADARIA EXEMPLO"` {
		t.Errorf("the subscription made again is not a new one: detail 2 names %s", got)
	}
}

// A line stored for the purchase goes with it; a suggestion waiting for
// review is not a line.
func TestANewSubscriptionTakesThePurchasesOwnLineOrNone(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	database := env.scratch.Database
	authoredtest.SaveBudget(t, database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	viagens := budgetLineID(t, database, "2026-03", "Viagens")
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.lines", At: fixtureNow}
	err := database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		bookshop, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen(txInstalment, "2026-03-06", -12_900, "text:LIVRARIA EXEMPLO"))
		if err != nil {
			return err
		}
		if err := authoredtest.WriteState(t.Context(), recorder, bookshop, authored.TransactionState{Line: &viagens, ClassifiedBy: authored.AuthorPerson}); err != nil {
			return err
		}
		bakery, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen(txPending, "2026-03-19", -1_850, "text:PADARIA EXEMPLO"))
		if err != nil {
			return err
		}
		return authoredtest.WriteState(t.Context(), recorder, bakery, authored.TransactionState{
			PendingReason: "Confiança abaixo do limite", PendingLine: &viagens, PendingConfidence: authoredtest.Some[int64](70),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	createdOf(t, env, txInstalment)
	createdOf(t, env, txPending)

	var list listAnswer
	subscriptionAnswer(t, env.readEnv, "subscriptions.list", `[{"grouped":false}]`, &list)
	rows := list.Groups[0].Subscriptions
	if len(rows) != 2 || rows[0].Line == nil || *rows[0].Line != "Viagens" || rows[1].Line != nil {
		t.Fatalf("rows %+v: want the bookshop on Viagens and the bakery on no line", rows)
	}
	bookshop := rows[0]
	if bookshop.Status != "Analisar" || bookshop.Cycle != "Mensal" || bookshop.Amount != "129.00" ||
		bookshop.NextCharge != "2026-04-06" || detailField(t, env, "1", "reviewOn") != `"2026-06-20"` {
		t.Errorf("the bookshop's row %+v", bookshop)
	}
}

func TestCreateAcceptsAnEligiblePurchaseThatIsNotAmongTheTwelveShown(t *testing.T) {
	fixture := newSubscriptionsLedger(t)
	for index, letter := range "ABCDEFGHIJKLM" {
		cardPurchase(fixture, "tx-loja-"+string(letter), november(index+1, 15), "LOJA "+string(letter), float64(10+index))
	}
	env := newSubscriptionsEnv(t, fixture)
	for _, candidate := range candidatesOf(t, env.readEnv) {
		if candidate.TransactionID == "tx-loja-A" {
			t.Fatal("the oldest purchase is among the twelve, so the test would prove nothing")
		}
	}
	if created := createdOf(t, env, "tx-loja-A"); created.Name != "LOJA A" {
		t.Errorf("created %+v", created)
	}
	if got := detailField(t, env, "1", "amount"); got != "10.00" {
		t.Errorf("the subscription's amount is %s", got)
	}
}

func TestCreateAnswersNullAndRecordsNothingForWhatCannotBecomeASubscription(t *testing.T) {
	fixture := newSubscriptionsLedger(t)
	cardPurchase(fixture, "tx-gym", november(5, 15), "ACADEMIA", 99.9)
	cardPurchase(fixture, "tx-gym-later", november(18, 15), "ACADEMIA", 99.9)
	cardPurchase(fixture, "tx-magazine", utc(2026, 10, 30, 15, 0), "REVISTA", 15)
	cardPurchase(fixture, "tx-stream", november(10, 15), "STREAMSVC", 39.9)
	cardRow(fixture, "tx-refund", november(13, 15), "ESTORNO LOJA", 40, "CREDIT", "ESTORNO", nil)
	cardRow(fixture, "tx-annual-fee", november(9, 15), "ANUIDADE", 30, "DEBIT", "TARIFA", nil)
	cardRow(fixture, "tx-paid-bill", november(11, 15), "PAGAMENTO COM SALDO", 200, "DEBIT", "PAGAMENTO", nil)
	fixture.AddTransaction(ledgertest.Transaction{ID: "tx-euro", AccountID: subscriptionsCard, Date: november(14, 15),
		Description: "HOTELSVC", Amount: 15, Currency: "EUR", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "4444333322221111"}}})
	fixture.AddTransaction(ledgertest.Transaction{ID: "tx-bank-debit", AccountID: subscriptionsBank, Date: november(15, 15),
		Description: "LIVRARIA", Amount: -50, Currency: "BRL", Type: "DEBIT", Raw: map[string]any{"operationType": "CARTAO"}})
	env := newSubscriptionsEnv(t, fixture)
	exclude(t, env.scratch.Database, "tx-stream", "2026-11-10", "Cobrança em dobro")
	createdOf(t, env, "tx-gym")

	for _, id := range []string{
		"tx-nobody", "tx-gym", "tx-gym-later", "tx-magazine", "tx-stream", "tx-refund", "tx-annual-fee", "tx-paid-bill",
		"tx-euro", "tx-bank-debit",
	} {
		before := env.scratch.Everything(t)
		if status, body := env.answer("subscriptions.create", `["`+id+`"]`); status != http.StatusOK || string(body) != "null" {
			t.Errorf("create %s answered %d %s, want 200 null", id, status, body)
		}
		if after := env.scratch.Everything(t); after != before {
			t.Errorf("create %s answered null and changed the file:\n%s", id, differences(before, after))
		}
	}
}

func TestTwoCreatesForOneEstablishmentAtOnceMakeOneSubscription(t *testing.T) {
	fixture := newSubscriptionsLedger(t)
	cardPurchase(fixture, "tx-gym", november(5, 15), "ACADEMIA", 99.9)
	cardPurchase(fixture, "tx-gym-later", november(18, 15), "ACADEMIA", 99.9)
	env := newSubscriptionsEnv(t, fixture)
	release := env.scratch.HoldWriteLock(t)
	bodies := make([]string, 2)
	var wait sync.WaitGroup
	for index, id := range []string{"tx-gym", "tx-gym-later"} {
		wait.Go(func() {
			_, body := env.answer("subscriptions.create", `["`+id+`"]`)
			bodies[index] = string(body)
		})
	}
	release()
	wait.Wait()
	nulls := 0
	for _, body := range bodies {
		if body == "null" {
			nulls++
		}
	}
	var list listAnswer
	subscriptionAnswer(t, env.readEnv, "subscriptions.list", `[{"grouped":false}]`, &list)
	if nulls != 1 || len(list.Groups) != 1 || len(list.Groups[0].Subscriptions) != 1 {
		t.Errorf("the two creates answered %q and left %+v, want one subscription and one null", bodies, list.Groups)
	}
}

func TestTheStatusItAlreadyHasIsAnEmptyChangeAndAnotherMovesItsDayToToday(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	database := env.scratch.Database
	id := keepSubscription(t, database, txPending, keptSubscription("text:PADARIA EXEMPLO", "PADARIA EXEMPLO", "2026-03-01", 1_850, authored.StatusReview))
	effects := env.scratch.EffectCount(t)

	same := env.change("subscriptions.setStatus", `["1","Analisar"]`)
	if env.scratch.EffectCount(t) != effects || storedSubscription(t, database, id).StatusDay != "2026-03-01" {
		t.Errorf("the status it had recorded %d effects and left its day at %s", env.scratch.EffectCount(t)-effects, storedSubscription(t, database, id).StatusDay)
	}
	beforeUndo := env.scratch.Canonical(t)
	env.undo(same)
	if env.scratch.Canonical(t) != beforeUndo {
		t.Error("undoing the empty change changed the database")
	}
	env.change("subscriptions.setStatus", `["1","Backup e cancelar"]`)
	if got := storedSubscription(t, database, id); got.Status != authored.StatusBackUpAndCancel || got.StatusDay != "2026-03-20" {
		t.Errorf("after another status: %s since %s, want Backup e cancelar since 2026-03-20", got.Status, got.StatusDay)
	}
}

func TestASetterThatChangesNothingAnswersAChangeWhoseUndoRestoresNothing(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	inventedAuthoredState(t, env.scratch)
	for _, call := range []struct{ method, arguments string }{
		{"subscriptions.setReviewDate", `["1","2026-05-10"]`},
		{"subscriptions.setNote", `["1","Cesta do mês, revisar o plano"]`},
	} {
		effects, before := env.scratch.EffectCount(t), env.scratch.Canonical(t)
		env.undo(env.change(call.method, call.arguments))
		if env.scratch.EffectCount(t) != effects || env.scratch.Canonical(t) != before {
			t.Errorf("%s %s changed something", call.method, call.arguments)
		}
	}
}

func TestEverySetterRefusesASubscriptionTheDatabaseDoesNotHave(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	inventedAuthoredState(t, env.scratch)
	before := env.scratch.State(t)
	for _, id := range []string{"999", "sub-1", "", "01"} {
		for method, argument := range map[string]string{
			"setStatus": `"Manter"`, "setLine": `"Viagens"`, "setReviewDate": `"2026-07-01"`, "setNote": `"plano família"`,
		} {
			response := env.call("/api/subscriptions/"+method, `["`+id+`",`+argument+`]`, env.cookie)
			expectRefused(t, method+" of "+id, response, "not_found")
		}
	}
	expectRefused(t, "an unknown line", env.call("/api/subscriptions/setLine", `["1","Inexistente"]`, env.cookie), "unknown_line")
	expectRefused(t, "the name the line had in February", env.call("/api/subscriptions/setLine", `["1","Mercado"]`, env.cookie), "unknown_line")
	if after := env.scratch.State(t); after != before {
		t.Errorf("a refused setter changed the database:\n%s", differences(before, after))
	}
	env.change("subscriptions.setLine", `["1","Feira"]`)
	if got := detailField(t, env, "1", "line"); got != `"Feira"` {
		t.Errorf("the line March's budget calls Feira reads %s", got)
	}
}

func TestSetLineIsRefusedInAMonthWithNoBudgetButTakesTheFeesLineInOneThatHasOne(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	keepSubscription(t, env.scratch.Database, txPending, keptSubscription("text:PADARIA EXEMPLO", "PADARIA EXEMPLO", "2026-03-19", 1_850, authored.StatusReview))
	for _, name := range []string{"Tarifas e juros", "Viagens"} {
		expectRefused(t, name+" with no budget", env.call("/api/subscriptions/setLine", `["1","`+name+`"]`, env.cookie), "unknown_line")
	}
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-03", authoredtest.InventedBudget())
	env.change("subscriptions.setLine", `["1","Tarifas e juros"]`)
	if got := detailField(t, env, "1", "line"); got != `"Tarifas e juros"` {
		t.Errorf("the fees line reads %s", got)
	}
}

func TestMalformedSetterArgumentsAreBadRequestsAndChangeNothing(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	inventedAuthoredState(t, env.scratch)
	before := env.scratch.State(t)
	for _, call := range []struct{ method, arguments string }{
		{"setReviewDate", `["1","2026-13-01"]`}, {"setReviewDate", `["1","2026-02-30"]`}, {"setReviewDate", `["1","1/7/2026"]`},
		{"setStatus", `["1","Pausar"]`}, {"setStatus", `["1","manter"]`}, {"setStatus", `["1",3]`},
		{"setNote", `["1",null]`}, {"setLine", `[1,"Viagens"]`}, {"create", `[]`}, {"create", `[7]`},
	} {
		response := env.call("/api/subscriptions/"+call.method, call.arguments, env.cookie)
		expectError(t, call.method+" "+call.arguments, response, http.StatusBadRequest, "bad_request")
	}
	if after := env.scratch.State(t); after != before {
		t.Errorf("a bad request changed the database:\n%s", differences(before, after))
	}
}

func TestUndoingACreateIsRefusedWhileALaterStatusStandsAndSucceedsOnceThatIsUndone(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	before := env.scratch.Canonical(t)
	created := createdOf(t, env, txForeign)
	kept := env.change("subscriptions.setStatus", `["1","Manter"]`)
	state := env.scratch.State(t)
	expectRefused(t, "the create's undo", env.call("/api/changes/undo", `["`+created.ChangeID+`"]`, env.cookie), "stale_change")
	if after := env.scratch.State(t); after != state {
		t.Errorf("the refused undo changed the database:\n%s", differences(state, after))
	}
	env.undo(kept)
	env.undo(created.ChangeID)
	if got := env.scratch.Canonical(t); got != before {
		t.Errorf("after both undos:\n%s", differences(before, got))
	}
}

func TestWithTheSnapshotMissingCreateIsUnavailableAndTheSettersStillAnswer(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	keepSubscription(t, scratch.Database, txPending, keptSubscription("text:PADARIA EXEMPLO", "PADARIA EXEMPLO", "2026-03-19", 1_850, authored.StatusReview))
	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), fixtureNow), withDatabase(scratch.Database))
	env := &settingsEnv{readEnv: &readEnv{testEnv: missing, cookie: missing.unlock()}, scratch: scratch}
	before := scratch.State(t)
	expectError(t, "create", env.call("/api/subscriptions/create", `["tx-pending"]`, env.cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	if after := scratch.State(t); after != before {
		t.Errorf("create with the snapshot missing changed the database:\n%s", differences(before, after))
	}
	env.change("subscriptions.setStatus", `["1","Cancelar"]`)
	env.change("subscriptions.setReviewDate", `["1","2026-07-01"]`)
	env.change("subscriptions.setNote", `["1","plano família"]`)
	if got := storedSubscription(t, scratch.Database, 1); got.Status != authored.StatusCancel || got.ReviewOn != brday.Day("2026-07-01") {
		t.Errorf("the setters with the snapshot missing left %+v", got)
	}
}

// A subscription is named as the candidates name its purchase when it is made,
// the nickname included, and keeps that name when the nickname changes.
func TestASubscriptionKeepsTheNameItWasCreatedUnder(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	nickname := func(name string) {
		facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.nickname", At: fixtureNow}
		err := env.scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
			return authoredtest.WriteEstablishment(t.Context(), recorder, keyOfTheBakery, name, nil)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	nickname(nicknameOfTheBakery)
	if created := createdOf(t, env, txPending); created.Name != nicknameOfTheBakery {
		t.Errorf("created under %q, want the nickname %q", created.Name, nicknameOfTheBakery)
	}
	nickname("Padaria Nova")
	if got := detailField(t, env, "1", "name"); got != `"`+nicknameOfTheBakery+`"` {
		t.Errorf("after the nickname changed the subscription is named %s", got)
	}
}
