package api

import (
	"encoding/json"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// The reads that follow the payday transfers: a pot's page, the net worth and
// the budget's investment made, over the worked example.

const (
	pendingOnPage     = `{"state":"pending"}`
	nonePlannedOnPage = `{"state":"none-planned"}`
	closedOnPage      = `{"state":"closed"}`
	doneOnPage        = `{"state":"done","doneOn":"2026-03-20"}`
)

func (env *paydayEnv) contributionOf(potID string) string {
	env.t.Helper()
	return env.fieldOf("savingsPots.detail", `["`+potID+`"]`, "contribution")
}

func (env *paydayEnv) expectContributions(label string, want map[string]string) {
	env.t.Helper()
	for potID, state := range want {
		if got := env.contributionOf(potID); got != state {
			env.t.Errorf("%s: the page of %s answers %s, want %s", label, potID, got, state)
		}
	}
}

func (env *paydayEnv) netWorthPlan() string {
	env.t.Helper()
	return env.fieldOf("netWorth.overview", `[]`, "contribution") + " " + env.fieldOf("netWorth.overview", `[]`, "investmentRate")
}

// madeIn is the investment line's second cell of budget.view, as the server
// wrote it.
func (env *paydayEnv) madeIn(arguments string) string {
	env.t.Helper()
	var groups []struct {
		Key   string `json:"key"`
		Lines []struct {
			Cells []json.RawMessage `json:"cells"`
		} `json:"lines"`
	}
	if err := json.Unmarshal([]byte(env.fieldOf("budget.view", arguments, "groups")), &groups); err != nil {
		env.t.Fatal(err)
	}
	for _, group := range groups {
		if group.Key == "inv" {
			return string(group.Lines[0].Cells[1])
		}
	}
	env.t.Fatal("budget.view has no investment group")
	return ""
}

func madeCell(amount string) string { return `{"type":"money","amount":` + amount + `}` }

func TestTheWorkedExampleReachesEveryReadThatFollowsTheTransfers(t *testing.T) {
	env := newPaydayEnv(t, 800_000)
	env.expectContributions("before any mark", map[string]string{
		examplePotOne: pendingOnPage, exampleNubankPot: pendingOnPage, examplePotSeven: pendingOnPage, exampleClosedPot: closedOnPage,
		exampleFreePot: nonePlannedOnPage, exampleSlot: nonePlannedOnPage, exampleInvestment: pendingOnPage, exampleInvestment2: pendingOnPage, exampleWithdrawn: closedOnPage,
	})
	if got := env.netWorthPlan(); got != `{"planned":4180.00,"done":0.00,"progress":0} {"month":"2026-03","planned":37,"actual":0}` {
		t.Errorf("before any mark the net worth answers %s", got)
	}
	if got := env.madeIn(`["2026-03"]`); got != madeCell("0.00") {
		t.Errorf("before the investment is marked it is made %s", got)
	}

	env.mark("2026-03", "pot:"+examplePotOne)
	env.expectContributions("after the pot's mark", map[string]string{
		examplePotOne: doneOnPage, exampleNubankPot: pendingOnPage, exampleInvestment: pendingOnPage,
	})
	if got := env.netWorthPlan(); got != `{"planned":4180.00,"done":240.00,"progress":6} {"month":"2026-03","planned":37,"actual":0}` {
		t.Errorf("after the pot's mark the net worth answers %s", got)
	}

	env.mark("2026-03", "investment")
	env.expectContributions("after the investment's mark", map[string]string{
		exampleInvestment: doneOnPage, exampleInvestment2: doneOnPage, exampleWithdrawn: closedOnPage, examplePotSeven: pendingOnPage,
	})
	if got := env.netWorthPlan(); got != `{"planned":4180.00,"done":3170.00,"progress":76} {"month":"2026-03","planned":37,"actual":37}` {
		t.Errorf("after the investment's mark the net worth answers %s", got)
	}
	if got := env.madeIn(`["2026-03"]`); got != madeCell("2930.00") {
		t.Errorf("after the investment is marked it is made %s", got)
	}
	if got := transferNamed(t, env.transfers("2026-03"), "investment"); got.State != "done" {
		t.Errorf("the payday reads the investment %+v", got)
	}
}

// The page follows the current month: a mark of another month, the one the
// rest of the app may be showing, changes nothing on it.
func TestAPotsPageFollowsTheCurrentMonthWhateverMonthIsMarked(t *testing.T) {
	env := newPaydayEnv(t, 800_000)
	env.mark("2026-04", "pot:"+examplePotOne)
	env.mark("2026-02", "investment")
	env.expectContributions("with marks of other months", map[string]string{examplePotOne: pendingOnPage, exampleInvestment: pendingOnPage})
	env.mark("2026-03", "pot:"+examplePotOne)
	env.expectContributions("with the current month's mark", map[string]string{examplePotOne: doneOnPage})
}

func TestUnderANegativeResidualNoInvestmentIsPlanned(t *testing.T) {
	env := newPaydayEnv(t, 500_000)
	env.expectContributions("under the negative residual", map[string]string{
		exampleInvestment: nonePlannedOnPage, exampleInvestment2: nonePlannedOnPage, examplePotOne: pendingOnPage,
	})
	if got := env.netWorthPlan(); got != `{"planned":1250.00,"done":0.00,"progress":0} {"month":"2026-03","planned":-1,"actual":0}` {
		t.Errorf("under the negative residual the net worth answers %s", got)
	}
}

// An investment a line is linked to has a transfer of its own, which comes
// before the shared one.
func TestAnInvestmentWithALinkedLineFollowsItsOwnTransfer(t *testing.T) {
	env := newPaydayEnv(t, 800_000)
	authoredtest.ToggleLink(t, env.scratch.Database, fixtureNow, env.lines["Revisão do carro"], exampleInvestment2)
	env.mark("2026-03", "investment")
	env.expectContributions("with the shared transfer marked", map[string]string{exampleInvestment: doneOnPage, exampleInvestment2: pendingOnPage})
	env.mark("2026-03", "pot:"+exampleInvestment2)
	env.expectContributions("with its own transfer marked", map[string]string{exampleInvestment2: doneOnPage})
}

func TestTheProgressOfTheMonthsPlanIsRoundedHalfUpAndARateNeedsAnIncome(t *testing.T) {
	env := &paydayEnv{settingsEnv: newSettingsEnv(t, newPaydayLedger(t), fixtureNow), lines: map[string]authored.LineID{}}
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-03", authored.Document{
		authoredtest.Group("fix", authoredtest.Line("Aluguel", 100_000)),
		authoredtest.Group("prov", authoredtest.Line("Um", 100), authoredtest.Line("Sete", 700)),
	})
	env.lines["Um"] = budgetLineID(t, env.scratch.Database, "2026-03", "Um")
	env.mark("2026-03", env.lineKey("Um"))
	if got := env.netWorthPlan(); got != `{"planned":8.00,"done":1.00,"progress":13} {"month":"2026-03","planned":null,"actual":null}` {
		t.Errorf("1.00 marked of 8.00 with no income answers %s", got)
	}
}

func TestTheInvestmentIsMadeWhenItsTransferIsMarkedOrTheMonthIsClosed(t *testing.T) {
	env := newPaydayEnv(t, 800_000)
	richDraft := `["2026-03",[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":1000}]}]]`
	poorDraft := `["2026-03",[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":5000}]},` +
		`{"key":"fix","name":"Fixos","lines":[{"name":"Aluguel","amount":5070}]}]]`
	if got := env.madeIn(richDraft); got != madeCell("0.00") {
		t.Errorf("a draft before the mark is made %s", got)
	}
	env.mark("2026-03", "investment")
	for arguments, want := range map[string]string{richDraft: madeCell("1000.00"), poorDraft: madeCell("0.00")} {
		if got := env.madeIn(arguments); got != want {
			t.Errorf("budget.view %s made %s, want %s", arguments, got, want)
		}
	}

	closedEnv := newPaydayEnv(t, 800_000)
	authoredtest.CloseMonth(t, closedEnv.scratch.Database, fixtureNow, "2026-03", "2026-03-20")
	if got := closedEnv.madeIn(`["2026-03"]`); got != madeCell("2930.00") {
		t.Errorf("a closed month with nothing marked is made %s", got)
	}
	closedEnv.mark("2026-03", "investment")
	if got := closedEnv.madeIn(`["2026-03"]`); got != madeCell("2930.00") {
		t.Errorf("a closed month with its investment marked is made %s", got)
	}
	poorer := newPaydayEnv(t, 500_000)
	authoredtest.CloseMonth(t, poorer.scratch.Database, fixtureNow, "2026-03", "2026-03-20")
	if got := poorer.madeIn(`["2026-03"]`); got != madeCell("-70.00") {
		t.Errorf("a closed month past its income is made %s", got)
	}
}

// Over the shared full ledger, whose pot has movements of its own.
func TestMarkingAPotsTransferLeavesItsMovementsAsTheyWere(t *testing.T) {
	env := &paydayEnv{settingsEnv: newSettingsEnv(t, newFullLedger(t), fixtureNow)}
	budgetWithLink("Remédios", potCaixinha)(t, env.scratch)
	before := env.fieldOf("savingsPots.detail", `["`+potCaixinha+`"]`, "movements")
	if before == "[]" {
		t.Fatal("the pot has no movement, so the test would prove nothing")
	}
	env.mark("2026-03", "pot:"+potCaixinha)
	if got := env.contributionOf(potCaixinha); got != doneOnPage {
		t.Fatalf("the pot's page answers %s after its mark", got)
	}
	if after := env.fieldOf("savingsPots.detail", `["`+potCaixinha+`"]`, "movements"); after != before {
		t.Errorf("the movements went from\n%s\nto\n%s", before, after)
	}
	if got := env.fieldOf("savingsPots.detail", `["acc-nu"]`, "contribution"); got != nonePlannedOnPage {
		t.Errorf("the page of the account's unclaimed movements answers %s", got)
	}
}
