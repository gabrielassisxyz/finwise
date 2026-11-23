package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// authoredReadCase is a call of a read method over the third state: the full
// invented ledger together with an invented authored state. It is a type of its
// own, built by a constructor of its own, because the interface's equality test
// finds the cases of readCases by their written shape and asks a golden file of
// the empty ledger for each one.
type authoredReadCase struct {
	method    string
	arguments string
	// over is what the case adds to the third state before it is answered, on
	// a router of its own: a read whose subject the third state cannot hold
	// without changing what every other case answers.
	over func(t *testing.T, scratch *authoredtest.Scratch, full *ledgertest.Fixture)
}

func authoredRead(method, arguments string) authoredReadCase {
	return authoredReadCase{method: method, arguments: arguments}
}

func authoredReadOver(method, arguments string, over func(t *testing.T, scratch *authoredtest.Scratch, full *ledgertest.Fixture)) authoredReadCase {
	return authoredReadCase{method: method, arguments: arguments, over: over}
}

// authoredReadCases are answered over the full ledger and the invented authored
// state, validated against the method's result schema and compared with their
// files in testdata/golden/full-authored. A read that does not consult the
// authored store answers here what it answers in full/; a read that merges what
// the person authored has its case here. The two months of the budget's cases
// are served by two versions that name one line differently.
var authoredReadCases = []authoredReadCase{
	authoredRead("transactions.detail", `["tx-fee"]`),
	authoredRead("accounts.detail", `["acc-stale"]`),
	authoredRead("budget.document", `["2026-02"]`),
	authoredRead("budget.document", `["2026-03"]`),
	authoredRead("budget.scenarios", `[]`),
	authoredRead("transactions.filterOptions", `["2026-02"]`),
	authoredRead("transactions.filterOptions", `["2026-03"]`),
	authoredRead("settings.overview", `[]`),
	authoredRead("card.overview", `[]`),
	authoredRead("home.summary", `["2026-03"]`),
	authoredRead("review.closing", `["2026-03"]`),
	authoredRead("savingsPots.list", `[{"includeClosed":false}]`),
	authoredRead("savingsPots.list", `[{"includeClosed":true}]`),
	authoredRead("savingsPots.detail", `["reserve:acc-pots/pot-1"]`),
	authoredRead("netWorth.overview", `[]`),
	authoredRead("transactions.list", `[{"month":"2026-03","period":"month"}]`),
	authoredRead("transactions.list", `[{"month":"2026-03","period":"last-3-months"}]`),
	authoredRead("transactions.list", `[{"month":"2026-02","period":"month"}]`),
	authoredRead("transactions.detail", `["tx-month-end"]`),
	authoredRead("transactions.detail", `["tx-pending"]`),
	authoredRead("transactions.detail", `["tx-last-of-february"]`),
	authoredRead("transactions.detail", `["tx-boleto"]`),
	authoredRead("transactions.detail", `["tx-salary"]`),
	authoredRead("transactions.detail", `["tx-instalment"]`),
	authoredRead("transactions.detail", `["tx-pix-out"]`),
	authoredRead("transactions.detail", `["tx-foreign"]`),
	authoredRead("transactions.detail", `["tx-stale"]`),
	authoredRead("transactions.detail", `["tx-first-of-march"]`),
	// The budget's figures: the month; a draft that gives Feira its old name
	// under its id, adds a line and leaves three groups out; and the closed
	// month before the first version. An empty draft has no case here: its
	// file would be the month's own. Then a line's establishments under each
	// name a month gives it, the fees line and a name no month has.
	authoredRead("budget.view", `["2026-03"]`),
	authoredRead("budget.view", `["2026-03",[{"key":"rec","name":"Receita","lines":[{"id":"2","name":"Salário","amount":10000}]},`+
		`{"key":"var","name":"Variáveis","lines":[{"id":"6","name":"Mercado","amount":500},{"name":"Pets","amount":100}]}]]`),
	authoredRead("budget.view", `["2026-01"]`),
	authoredRead("budget.establishments", `["2026-02","Mercado"]`),
	authoredRead("budget.establishments", `["2026-03","Mercado"]`),
	authoredRead("budget.establishments", `["2026-03","Feira"]`),
	authoredRead("budget.establishments", `["2026-03","Delivery"]`),
	authoredRead("budget.establishments", `["2026-03","Tarifas e juros"]`),
	authoredRead("budget.establishments", `["2026-03","Academia"]`),
	authoredRead("transactions.detail", `["tx-refund"]`),
	authoredReadOver("review.weekly", `["2026-03"]`, queueOverTheThirdState),
	// The home's limit and weekly guides under each version of the invented
	// budget: 2026-02, a past month, and 2026-03, the current one, above. The
	// frame counts what waits for review, as both homes do, and lists the
	// closed months, a second one beside January.
	authoredRead("home.summary", `["2026-02"]`),
	authoredReadOver("shell.overview", `[]`, aSecondClosedMonth),

	authoredRead("review.payday", `["2026-03"]`),
	authoredRead("review.payday", `["2026-04"]`),
	authoredRead("savingsPots.detail", `["investment:inv-it-1"]`),
	authoredRead("savingsPots.detail", `["investment:inv-nu-1"]`),
	authoredReadOver("review.weekly", `["2026-02"]`, queueWithModelSuggestions),
}

// newAuthoredReadEnv is the third state: both stores, the database holding the
// invented authored state.
func newAuthoredReadEnv(t *testing.T) *readEnv {
	t.Helper()
	scratch, full := newAuthoredStores(t)
	return newReadEnvOver(t, full, scratch.Database)
}

// authoredEnvOf is the router a case is answered by: shared, the third state's,
// unless the case adds to it.
func authoredEnvOf(t *testing.T, shared *readEnv, c authoredReadCase) *readEnv {
	t.Helper()
	if c.over == nil {
		return shared
	}
	scratch, full := newAuthoredStores(t)
	c.over(t, scratch, full)
	return newReadEnvOver(t, full, scratch.Database)
}

// authoredCasesOver counts the cases answered by a router of their own.
func authoredCasesOver() int {
	count := 0
	for _, c := range authoredReadCases {
		if c.over != nil {
			count++
		}
	}
	return count
}

// newAuthoredStores are the two stores of the third state: the full ledger, and
// a database holding the invented authored state with what the person said
// about that ledger's transactions, and the subscriptions kept over it, on top.
func newAuthoredStores(t *testing.T) (*authoredtest.Scratch, *ledgertest.Fixture) {
	t.Helper()
	scratch := authoredtest.Open(t, fixtureClock)
	inventedAuthoredState(t, scratch)
	full := newFullLedger(t)
	inventedTransactionAuthorship(t, scratch, full)
	inventedSubscriptions(t, scratch, full)
	return scratch, full
}

func TestTheInventedAuthoredStateHoldsSomething(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	empty := scratch.Canonical(t)
	inventedAuthoredState(t, scratch)

	var tables map[string][]map[string]any
	if err := json.Unmarshal([]byte(scratch.Canonical(t)), &tables); err != nil {
		t.Fatal(err)
	}
	for name, rows := range tables {
		if len(rows) == 0 {
			t.Errorf("the invented authored state leaves %s empty", name)
		}
	}
	if scratch.Canonical(t) == empty || len(scratch.Changes(t)) != 1 {
		t.Errorf("the state is the empty one or recorded %d changes, want 1", len(scratch.Changes(t)))
	}
}

func TestEveryAuthoredReadCaseAnswersAndMatchesItsGoldenFile(t *testing.T) {
	if err := goldenUpdateRefusal(*updateGolden, os.Getenv(snapshotVariable)); err != nil {
		t.Fatal(err)
	}
	if len(authoredReadCases) == 0 {
		t.Fatal("there is no authored read case, so the directory and its comparison do not exist")
	}
	shared := newAuthoredReadEnv(t)
	for _, c := range authoredReadCases {
		status, body := authoredEnvOf(t, shared, c).answer(c.method, c.arguments)
		if status != http.StatusOK {
			t.Errorf("%s %s: status %d", c.method, c.arguments, status)
			continue
		}
		if problem := conformanceProblem(c.method, body); problem != "" {
			t.Errorf("%s %s does not conform: %s", c.method, c.arguments, problem)
		}
		name, err := goldenName(c.method, c.arguments)
		if err != nil {
			t.Fatal(err)
		}
		compareWithGolden(t, filepath.Join("testdata", "golden", "full-authored", name), prettyJSON(t, body))
	}
}

// answersThatDifferWhenThereIsADatabase names the reads that must answer
// differently for a database that holds nothing authored than for no database,
// as "<ledger> <method> <arguments>", each with the reason in a comment. It is
// empty: turning --db on moves no figure. The month the database was created in
// is a fact no database lacks, and the first read that answers it adds its name.
var answersThatDifferWhenThereIsADatabase = []string{}

// emptyDatabaseProblem is empty when an answer over a database that holds
// nothing authored equals the answer with no database, or differs and is listed
// as allowed to; a listed name whose answers are equal is a problem too, so the
// list cannot rot.
func emptyDatabaseProblem(name string, noDatabase, emptyDatabase []byte, allowed []string) string {
	listed := false
	for _, entry := range allowed {
		listed = listed || entry == name
	}
	equal := string(noDatabase) == string(emptyDatabase)
	switch {
	case !equal && !listed:
		return name + ": an empty database answers otherwise than no database"
	case equal && listed:
		return name + ": is listed as answering differently, and both answers are equal"
	}
	return ""
}

func TestAnEmptyDatabaseAnswersWhatNoDatabaseAnswers(t *testing.T) {
	for _, ledgerName := range []string{"full", "empty"} {
		env := newReadEnvOver(t, ledgerNamed(t, ledgerName), authoredtest.Fresh(t, fixtureClock))
		for _, c := range readCases {
			status, body := env.answer(c.method, c.arguments)
			if status != http.StatusOK {
				t.Errorf("%s ledger, %s %s: status %d", ledgerName, c.method, c.arguments, status)
				continue
			}
			path, err := goldenPath(ledgerName, c.method, c.arguments)
			if err != nil {
				t.Fatal(err)
			}
			golden, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			name := ledgerName + " " + c.method + " " + c.arguments
			if problem := emptyDatabaseProblem(name, golden, prettyJSON(t, body), answersThatDifferWhenThereIsADatabase); problem != "" {
				t.Error(problem)
			}
		}
	}
	for _, name := range answersThatDifferWhenThereIsADatabase {
		if emptyDatabaseProblem(name, []byte("a"), []byte("a"), answersThatDifferWhenThereIsADatabase) == "" {
			t.Errorf("%s: a listed name was not checked", name)
		}
	}
}

func TestTheEmptyDatabaseCheckSeesADifferenceAndARottenList(t *testing.T) {
	if emptyDatabaseProblem("full shell.overview []", []byte("1"), []byte("1"), nil) != "" {
		t.Error("equal answers were a problem")
	}
	if emptyDatabaseProblem("full shell.overview []", []byte("1"), []byte("2"), nil) == "" {
		t.Error("different answers went unnoticed")
	}
	if emptyDatabaseProblem("full shell.overview []", []byte("1"), []byte("1"), []string{"full shell.overview []"}) == "" {
		t.Error("a listed name whose answers are equal went unnoticed")
	}
	if emptyDatabaseProblem("full shell.overview []", []byte("1"), []byte("2"), []string{"full shell.overview []"}) != "" {
		t.Error("a listed difference was a problem")
	}
}
