package api

import (
	"net/http"
	"testing"
)

func init() {
	readCases = append(readCases,
		readCase{"subscriptions.list", `[{"grouped":true}]`},
		readCase{"subscriptions.list", `[{"grouped":false}]`},
		readCase{"subscriptions.detail", `["sub-1"]`},
		readCase{"subscriptions.candidates", `[]`},
		readCase{"subscriptions.recurring", `[]`},
	)
}

const emptySubscriptionList = `{"monthlyTotal":0.00,"annualTotal":0.00,"groups":[],"suggestion":null}`

func TestSubscriptionsAnswerAsAFreshInstallOverBothLedgers(t *testing.T) {
	for _, name := range []string{"full", "empty"} {
		env := newReadEnv(t, ledgerNamed(t, name))
		expectAnswer(t, env, "subscriptions.list", `[{"grouped":true}]`, emptySubscriptionList)
		expectAnswer(t, env, "subscriptions.list", `[{"grouped":false}]`, emptySubscriptionList)
		expectAnswer(t, env, "subscriptions.candidates", `[]`, `[]`)
		expectAnswer(t, env, "subscriptions.recurring", `[]`, `[]`)
	}
}

// The full ledger holds a card purchase that looks like a subscription; it is
// not offered, because detecting them belongs with the side that stores them.
func TestARepeatingLookingPurchaseIsNotOfferedAsACandidate(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	expectAnswer(t, env, "subscriptions.candidates", `[]`, `[]`)
	expectAnswer(t, env, "subscriptions.recurring", `[]`, `[]`)
}

func TestSubscriptionDetailIsNullForAnyId(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, id := range []string{"sub-1", "", "tx-foreign", accountItau} {
		expectAnswer(t, env, "subscriptions.detail", `["`+id+`"]`, `null`)
	}
}

func TestSubscriptionsRefuseArgumentsThatAreNotWhatTheyTake(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	calls := []struct{ path, body string }{
		{"/api/subscriptions/list", `[{}]`},
		{"/api/subscriptions/list", `[{"grouped":"yes"}]`},
		{"/api/subscriptions/list", `[{"grouped":true,"extra":1}]`},
		{"/api/subscriptions/detail", `[5]`},
	}
	for _, call := range calls {
		response := env.call(call.path, call.body, env.cookie)
		expectError(t, call.path+" "+call.body, response, http.StatusBadRequest, "bad_request")
	}
}
