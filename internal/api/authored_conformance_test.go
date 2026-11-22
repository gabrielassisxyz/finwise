package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// authoredReadCase is a call of a read method over the third state: the full
// invented ledger together with an invented authored state. It is a type of its
// own, built by a constructor of its own, because the interface's equality test
// finds the cases of readCases by their written shape and asks a golden file of
// the empty ledger for each one.
type authoredReadCase struct {
	method    string
	arguments string
}

func authoredRead(method, arguments string) authoredReadCase {
	return authoredReadCase{method: method, arguments: arguments}
}

// authoredReadCases are answered over the full ledger and the invented authored
// state, validated against the method's result schema and compared with their
// files in testdata/golden/full-authored. A read that does not consult the
// authored store answers its twin in full/; a read that merges what the person
// authored arrives with its case here.
var authoredReadCases = []authoredReadCase{
	authoredRead("transactions.detail", `["tx-fee"]`),
	authoredRead("accounts.detail", `["acc-stale"]`),
	authoredRead("settings.overview", `[]`),
}

// newAuthoredReadEnv is the third state: both stores, the database holding the
// invented authored state.
func newAuthoredReadEnv(t *testing.T) *readEnv {
	t.Helper()
	scratch := appScratch(t)
	inventedAuthoredState(t, scratch)
	return newReadEnvOver(t, newFullLedger(t), scratch.Database)
}

func TestTheInventedAuthoredStateHoldsSomething(t *testing.T) {
	scratch := appScratch(t)
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
	env := newAuthoredReadEnv(t)
	for _, c := range authoredReadCases {
		status, body := env.answer(c.method, c.arguments)
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
