package api

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

func init() {
	writeCases = append(writeCases, reviewWarningWriteCases...)
	answerOnlyNull = append(answerOnlyNull, "review.runWarningAction", "review.propose")
	secrets = append(secrets,
		// The ids of the warnings of the cases, and the ids of proposals nothing makes.
		"charged-after-cancel", "silence-pair", "teto", "nope", "preco", "price:",
	)
}

// subscriptionsKept is the third state's authored side: the invented state, what
// the person said of the full ledger's transactions, and the subscriptions
// kept over it, one of which charged again after it was marked to cancel.
func subscriptionsKept(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	inventedAuthoredState(t, scratch)
	full := newFullLedger(t)
	inventedTransactionAuthorship(t, scratch, full)
	inventedSubscriptions(t, scratch, full)
}

// reviewWarningWriteCases are a dismissal of the one warning the third state
// holds, which is recorded and undone, and every call that answers null: a
// dismissal of what no warning holds, and the four methods nothing produces.
var reviewWarningWriteCases = []writeCase{
	{method: "review.dismissWarning", arguments: `["charged-after-cancel:2:2026-03-10"]`, expects: expectsRecorded, state: subscriptionsKept},

	{method: "review.dismissWarning", arguments: `["nope"]`, expects: expectsNothing, state: subscriptionsKept},
	{method: "review.dismissWarning", arguments: `["price:2:2026-03-10"]`, expects: expectsNothing, state: subscriptionsKept},
	{method: "review.dismissWarning", arguments: `["charged-after-cancel:2:2026-03-10"]`, expects: expectsNothing, state: subscriptionsKept,
		before: []writeCall{{"review.dismissWarning", `["charged-after-cancel:2:2026-03-10"]`}}},
	{method: "review.runWarningAction", arguments: `["price:1:2026-11-22","silence-pair"]`, expects: expectsNothing, state: subscriptionsKept},
	{method: "review.approve", arguments: `["teto"]`, expects: expectsNothing, state: startFromTheInventedState},
	{method: "review.approve", arguments: `["nope"]`, expects: expectsNothing},
	{method: "review.refuse", arguments: `["teto"]`, expects: expectsNothing, state: startFromTheInventedState},
	{method: "review.refuse", arguments: `["nope"]`, expects: expectsNothing},
	{method: "review.propose", arguments: `["teto"]`, expects: expectsNothing, state: startFromTheInventedState},
	{method: "review.propose", arguments: `["nope"]`, expects: expectsNothing},
}
