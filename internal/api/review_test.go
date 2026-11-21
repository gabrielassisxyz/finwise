package api

import (
	"net/http"
	"testing"
)

func init() {
	readCases = append(readCases,
		readCase{"review.weekly", `["2026-03"]`},
		readCase{"review.payday", `["2026-03"]`},
	)
}

const (
	emptyWeeklyReview = `{"limit":{"cap":0.00,"spent":0.00,"remaining":0.00,"over":false,"fill":0},` +
		`"week":null,"queue":{"items":[],"resolved":0},"approvals":[],"warnings":[]}`
	paydayWithNoSalaryAccount = `{"cushion":0.00,"balance":null,"balanceLabel":null,"difference":null,"covered":null,"transfers":[]}`
)

// The full ledger has purchases in March, and still nothing is spent against a
// cap that does not exist.
func TestTheWeeklyReviewHasACapOfZeroOverAMonthWithPurchases(t *testing.T) {
	for _, name := range []string{"full", "empty"} {
		env := newReadEnv(t, ledgerNamed(t, name))
		expectAnswer(t, env, "review.weekly", `["2026-03"]`, emptyWeeklyReview)
	}
}

func TestThePaydayHasTheContractsEmptyValueWhenNoSalaryEverLanded(t *testing.T) {
	for _, name := range []string{"full", "empty"} {
		env := newReadEnv(t, ledgerNamed(t, name))
		expectAnswer(t, env, "review.payday", `["2026-03"]`, paydayWithNoSalaryAccount)
	}
}

func TestTheReviewMethodsRefuseAMonthThatIsNotOne(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, path := range []string{"/api/review/weekly", "/api/review/payday"} {
		for _, body := range []string{`["2026-13"]`, `["March"]`, `[3]`, `[null]`} {
			expectError(t, path+" "+body, env.call(path, body, env.cookie), http.StatusBadRequest, "bad_request")
		}
	}
}
