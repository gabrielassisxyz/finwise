package api

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

func init() {
	writeCases = append(writeCases, subscriptionWriteCases...)
	answerOnlyNull = append(answerOnlyNull, "subscriptions.acceptSuggestion", "subscriptions.rejectSuggestion")
	secrets = append(secrets,
		// What the subscriptions' cases and their invented state hold.
		"CLOUDSVC", "Manter", "Cancelar", "2026-07", "plano", "Inexistente", "sub-", "Migrar",
	)
}

// instalmentOnALine is the invented budget with the bookshop's instalment put
// on Viagens by the person, as a line the purchase carries into the
// subscription made from it.
func instalmentOnALine(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	budgetState()(t, scratch)
	line := budgetLineID(t, scratch.Database, "2026-03", "Viagens")
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "scratch.lineOfTheBookshop", At: fixtureNow}
	err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		key, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen(txInstalment, "2026-03-06", -12_900, "text:LIVRARIA EXEMPLO"))
		if err != nil {
			return err
		}
		return authoredtest.WriteState(t.Context(), recorder, key, authored.TransactionState{Line: &line, ClassifiedBy: authored.AuthorPerson})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// cloudSubscribed is the invented state with the cloud service's
// establishment already kept as a subscription.
func cloudSubscribed(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	inventedAuthoredState(t, scratch)
	keepSubscription(t, scratch.Database, txForeign,
		keptSubscription("text:CLOUDSVC SUBSCRIPTION", "CLOUDSVC SUBSCRIPTION", "2026-03-10", 11_385, authored.StatusKeep))
}

// subscriptionWriteCases are the seven writes: a subscription made from a
// purchase with no line and from one with a line, each of the four setters on
// the invented subscription, every purchase that cannot become one, the two
// answers to a suggestion nothing makes, and the refusals of an unknown
// subscription and an unknown line.
var subscriptionWriteCases = []writeCase{
	{method: "subscriptions.create", arguments: `["tx-foreign"]`, expects: expectsRecorded},
	{method: "subscriptions.create", arguments: `["tx-pending"]`, expects: expectsRecorded, state: startFromTheInventedState},
	{method: "subscriptions.create", arguments: `["tx-instalment"]`, expects: expectsRecorded, state: instalmentOnALine},
	{method: "subscriptions.setStatus", arguments: `["1","Cancelar"]`, expects: expectsRecorded, state: startFromTheInventedState},
	{method: "subscriptions.setLine", arguments: `["1","Viagens"]`, expects: expectsRecorded, state: startFromTheInventedState},
	{method: "subscriptions.setLine", arguments: `["1","Tarifas e juros"]`, expects: expectsRecorded, state: startFromTheInventedState},
	{method: "subscriptions.setReviewDate", arguments: `["1","2026-07-01"]`, expects: expectsRecorded, state: startFromTheInventedState},
	{method: "subscriptions.setNote", arguments: `["1","Trocar pelo plano família"]`, expects: expectsRecorded, state: startFromTheInventedState},

	{method: "subscriptions.create", arguments: `["no-such-transaction"]`, expects: expectsNothing},
	{method: "subscriptions.create", arguments: `["tx-month-end"]`, expects: expectsNothing},
	{method: "subscriptions.create", arguments: `["tx-refund"]`, expects: expectsNothing},
	{method: "subscriptions.create", arguments: `["tx-card-payment"]`, expects: expectsNothing},
	{method: "subscriptions.create", arguments: `["tx-unconverted"]`, expects: expectsNothing},
	{method: "subscriptions.create", arguments: `["tx-foreign-tax"]`, expects: expectsNothing},
	{method: "subscriptions.create", arguments: `["tx-fee"]`, expects: expectsNothing},
	{method: "subscriptions.create", arguments: `["tx-boleto"]`, expects: expectsNothing},
	{method: "subscriptions.create", arguments: `["tx-foreign"]`, expects: expectsNothing, state: cloudSubscribed},
	{method: "subscriptions.acceptSuggestion", arguments: `[]`, expects: expectsNothing, state: startFromTheInventedState},
	{method: "subscriptions.rejectSuggestion", arguments: `[]`, expects: expectsNothing, state: startFromTheInventedState},

	{method: "subscriptions.setStatus", arguments: `["999","Manter"]`, expects: expectsRefusal, reason: reasonNotFound, state: startFromTheInventedState},
	{method: "subscriptions.setLine", arguments: `["sub-1","Viagens"]`, expects: expectsRefusal, reason: reasonNotFound, state: startFromTheInventedState},
	{method: "subscriptions.setReviewDate", arguments: `["","2026-07-01"]`, expects: expectsRefusal, reason: reasonNotFound, state: startFromTheInventedState},
	{method: "subscriptions.setNote", arguments: `["999","Trocar pelo plano família"]`, expects: expectsRefusal, reason: reasonNotFound, state: startFromTheInventedState},
	{method: "subscriptions.setLine", arguments: `["1","Inexistente"]`, expects: expectsRefusal, reason: reasonUnknownLine, state: startFromTheInventedState},
}
