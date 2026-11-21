package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"testing"
	"time"
)

func TestTheHomeSummaryHasNoLimitNoWeeksAndNothingPendingOverAMonthWithPurchases(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	status, body := env.answer("transactions.list", `[{"month":"2026-03","period":"month"}]`)
	if status != http.StatusOK || len(body) < 100 {
		t.Fatalf("the month has no purchases to be set against: status %d", status)
	}

	home := answerOf(t, env, "home.summary", `["2026-03"]`)

	limit := home["limit"].(map[string]any)
	if limit["cap"].(float64) != 0 || limit["spent"].(float64) != 0 {
		t.Errorf("the limit is %v, want cap 0 and spent 0 until a budget exists", limit)
	}
	if len(home["weeks"].([]any)) != 0 || home["pendingCount"].(float64) != 0 {
		t.Errorf("weeks %v pending %v", home["weeks"], home["pendingCount"])
	}
}

func TestTheHomeNetWorthIsTheOneTheNetWorthScreenShows(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	home := answerOf(t, env, "home.summary", `["2026-03"]`)["netWorth"].(map[string]any)
	totals := bucketTotals(t, answerOf(t, env, "netWorth.overview", `[]`))
	if cents(home["available"].(float64)) != totals["Disponível"] || cents(home["reserved"].(float64)) != totals["Reservado"] ||
		cents(home["invested"].(float64)) != totals["Investido"] {
		t.Errorf("the home shows %v and the net worth screen %v", home, totals)
	}
}

func TestTheHomeOffersThePayAheadFigureOfTheCardsNextBill(t *testing.T) {
	f := c6Ledger(t)
	addCardPurchase(f, "tx-on-closing", utc(2026, time.March, 4, 15, 0), 50, "")
	addCardPurchase(f, "tx-later", utc(2026, time.March, 12, 15, 0), 20, "")

	home := answerOf(t, envAt(t, f, 2026, time.March, 20), "home.summary", `["2026-03"]`)

	payAhead, ok := home["payAhead"].(map[string]any)
	if !ok || payAhead["amount"].(float64) != 70 || payAhead["cardLabel"] != "C6 Carbon" {
		t.Errorf("pay ahead %v, want 70 on the C6 card", home["payAhead"])
	}
	card := answerOf(t, envAt(t, f, 2026, time.March, 20), "card.overview", `[]`)
	if card["payAhead"].(map[string]any)["amount"] != payAhead["amount"] {
		t.Error("the home and the card page disagree on what falls on the next bill")
	}
}

func TestTheHomeOffersNoPayAheadBeforeThereIsAnythingToPay(t *testing.T) {
	home := answerOf(t, envAt(t, c6Ledger(t), 2026, time.March, 20), "home.summary", `["2026-03"]`)
	if home["payAhead"] != nil {
		t.Errorf("pay ahead %v with nothing bought since the closing", home["payAhead"])
	}
}

func TestTheHomeRefusesAMonthThatIsNotOne(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, arguments := range []string{`["2026-13"]`, `["march"]`, `[1]`, `[]`} {
		if status, _ := env.answer("home.summary", arguments); status == http.StatusOK {
			t.Errorf("%s was answered", arguments)
		}
	}
}

func TestOverTheEmptyLedgerTheHomeIsAllZeroAndOffersNoPayAhead(t *testing.T) {
	home := answerOf(t, newReadEnv(t, newEmptyLedger(t)), "home.summary", `["2026-03"]`)
	worth := home["netWorth"].(map[string]any)
	if worth["available"].(float64) != 0 || worth["reserved"].(float64) != 0 || worth["invested"].(float64) != 0 || home["payAhead"] != nil {
		t.Errorf("the empty ledger's home is %v", home)
	}
}

func TestTheHomeSummaryDoesNotWorkOutAYieldItDoesNotShow(t *testing.T) {
	view, err := loadPots(withNow(t.Context(), func() time.Time { return fixtureNow }), ledgerOf(nightlyLedger(t, 6)), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range view.entries {
		if entry.yield.Known || len(entry.yield.Days) != 0 {
			t.Errorf("%s was given a yield the home does not show", entry.id)
		}
	}
}

// The summary shows no yield, so its handler must not ask for one. Nothing it
// answers shows the difference, so the call itself is what is checked.
func TestTheHomeHandlerLoadsThePotsWithoutAYield(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "home.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name, isName := call.Fun.(*ast.Ident); !isName || name.Name != "loadPots" {
			return true
		}
		calls++
		if flag, isName := call.Args[2].(*ast.Ident); !isName || flag.Name != "false" {
			t.Errorf("home.go loads the pots with %v as its yield flag, want false", call.Args[2])
		}
		return true
	})
	if calls != 1 {
		t.Errorf("home.go calls loadPots %d times, want once", calls)
	}
}

func TestTheHomeOffersNoPayAheadWhenTheNextBillsAmountIsNotKnown(t *testing.T) {
	home := answerOf(t, envAt(t, noTableLedger(t), 2026, time.March, 20), "home.summary", `["2026-03"]`)
	if home["payAhead"] != nil {
		t.Errorf("home offers a pay-ahead figure for an amount that is not known: %v", home["payAhead"])
	}
}
