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

// The card purchases of March in the full ledger, newest first: the pending
// bakery, the foreign cloud service with its tax, the bookshop's instalment.
const fullLedgerCandidates = `[{"transactionId":"tx-pending","name":"PADARIA EXEMPLO","amount":18.50},` +
	`{"transactionId":"tx-foreign","name":"CLOUDSVC SUBSCRIPTION","amount":113.85},` +
	`{"transactionId":"tx-instalment","name":"LIVRARIA EXEMPLO","amount":129.00}]`

// Without an app's database nothing is a subscription, so the list has no
// group, and the candidates come from the ledger alone.
func TestSubscriptionsAnswerAsAFreshInstallOverBothLedgers(t *testing.T) {
	for name, candidates := range map[string]string{"full": fullLedgerCandidates, "empty": `[]`} {
		env := newReadEnv(t, ledgerNamed(t, name))
		expectAnswer(t, env, "subscriptions.list", `[{"grouped":true}]`, emptySubscriptionList)
		expectAnswer(t, env, "subscriptions.list", `[{"grouped":false}]`, emptySubscriptionList)
		expectAnswer(t, env, "subscriptions.candidates", `[]`, candidates)
		expectAnswer(t, env, "subscriptions.recurring", `[]`, `[]`)
	}
}

// Of the full ledger's rows only March's card purchases are offered: the
// unconverted euro purchase, the card's refund and its bill payment, every
// bank row and February's purchase are not. Nothing spots a repeating
// purchase, so the recurring list stays empty.
func TestOnlyThisMonthsCardPurchasesOfTheFullLedgerAreCandidates(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	expectAnswer(t, env, "subscriptions.candidates", `[]`, fullLedgerCandidates)
	expectAnswer(t, env, "subscriptions.recurring", `[]`, `[]`)
}

func TestSubscriptionDetailIsNullForAnyIdWithoutADatabase(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, id := range []string{"sub-1", "", "1", "tx-foreign", accountItau} {
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
