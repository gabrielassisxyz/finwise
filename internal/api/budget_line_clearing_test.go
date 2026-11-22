package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// lineExamplesHTTP is a router over both stores whose app database holds the
// invented budget as the only version, from 2026-01, and the line examples of
// authoredtest over it, with a session open.
type lineExamplesHTTP struct {
	*writeEnv
	examples authoredtest.LineExamples
}

func newLineExamplesHTTP(t *testing.T, ledgerPath string) *lineExamplesHTTP {
	t.Helper()
	scratch := authoredtest.Open(t, fixtureClock)
	env := newTestEnv(t, withLedger(ledgerPath, fixtureNow), withDatabase(scratch.Database))
	examples := &lineExamplesHTTP{writeEnv: &writeEnv{testEnv: env, scratch: scratch, cookie: env.unlock()}}
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-01", authoredtest.InventedBudget())
	examples.record("scratch.lineExamples", func(recorder *authored.Recorder) error {
		var err error
		examples.examples, err = authoredtest.WriteLineExamples(t.Context(), recorder)
		return err
	})
	return examples
}

// record makes a change through the recorder, as the writes that put a
// transaction on a line will.
func (env *lineExamplesHTTP) record(method string, write func(*authored.Recorder) error) {
	env.t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: method, At: fixtureNow}
	if err := env.scratch.Database.Change(env.t.Context(), facts, write); err != nil {
		env.t.Fatalf("%s: %v", method, err)
	}
}

func (env *lineExamplesHTTP) budget(month brday.Month) authored.Budget {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	budget, err := view.BudgetOf(env.t.Context(), month)
	if err != nil {
		env.t.Fatal(err)
	}
	return budget
}

type postedLine struct {
	ID     *string `json:"id,omitempty"`
	Name   string  `json:"name"`
	Amount Reais   `json:"amount"`
	Flag   string  `json:"flag,omitempty"`
}

type postedGroup struct {
	Key   string       `json:"key"`
	Name  string       `json:"name"`
	Lines []postedLine `json:"lines"`
}

// saveArguments are the arguments of a save for month of the budget in force
// for it as edit leaves it, as a client posts them.
func (env *lineExamplesHTTP) saveArguments(month brday.Month, edit func(authored.Document) authored.Document) string {
	env.t.Helper()
	var groups []postedGroup
	for _, group := range edit(authoredtest.DocumentOf(env.budget(month))) {
		posted := postedGroup{Key: group.Key, Name: group.Key, Lines: []postedLine{}}
		for _, line := range group.Lines {
			posted.Lines = append(posted.Lines, postedLine{ID: line.ID, Name: line.Name, Amount: Reais(line.Cents), Flag: line.Flag})
		}
		groups = append(groups, posted)
	}
	return `["` + string(month) + `",` + string(mustMarshal(env.t, groups)) + `]`
}

func (env *lineExamplesHTTP) names(keys []authored.TxKey) []string {
	names := []string{}
	for _, key := range keys {
		names = append(names, env.examples.Name(key))
	}
	return names
}

// ledgerIsMissing checks that the router really has no snapshot to read.
func (env *lineExamplesHTTP) ledgerIsMissing() {
	env.t.Helper()
	expectError(env.t, "a read of the ledger", env.post("transactions.list", `[{"month":"2026-03","period":"month"}]`),
		http.StatusServiceUnavailable, "ledger_unavailable")
}

func TestBothSavesOfABudgetClearWhatTheMonthsThatLoseALineHold(t *testing.T) {
	fixture := newFullLedger(t)
	for _, method := range []string{"budget.save", "budget.applyScenario"} {
		for _, c := range authoredtest.LineSaveCases() {
			t.Run(method+"/"+c.Name, func(t *testing.T) {
				env := newLineExamplesHTTP(t, fixture.Path)
				if c.Prepare != nil {
					c.Prepare(t, env.scratch.Database, fixtureNow)
				}
				before, canonical := env.scratch.LineReferences(t), env.scratch.Canonical(t)
				id := changeIDOfAnswer(t, env.post(method, env.saveArguments(c.Month, c.Edit)))
				after := env.scratch.LineReferences(t)
				for _, problem := range authoredtest.ClearedProblems(before, after) {
					t.Error(problem)
				}
				if got := env.names(authoredtest.Changed(before, after)); !slices.Equal(got, append([]string{}, c.Changed...)) {
					t.Errorf("the save changed %v, want %v", got, c.Changed)
				}
				for _, effect := range env.scratch.EffectsOf(t, changeNumber(t, id)) {
					if !slices.Contains([]string{"budget_versions", "budget_version_lines", "tx_state", "tx_split_parts"}, effect.Table) {
						t.Errorf("the save has an effect on %s", effect.Table)
					}
				}
				expectUndone(t, "the undo of the save", env.undo(id))
				if got := env.scratch.Canonical(t); got != canonical {
					t.Errorf("the undo left another state:\n%s", differences(canonical, got))
				}
			})
		}
	}
}

func TestASaveWithTheSnapshotMissingClearsWhatItClearsWithTheSnapshotPresent(t *testing.T) {
	effectsOver := func(env *lineExamplesHTTP) []authoredtest.Effect {
		t.Helper()
		id := changeIDOfAnswer(t, env.post("budget.save", env.saveArguments("2026-03", authoredtest.Without("Delivery"))))
		return env.scratch.EffectsOf(t, changeNumber(t, id))
	}
	present := effectsOver(newLineExamplesHTTP(t, newFullLedger(t).Path))
	missingPath := filepath.Join(t.TempDir(), "absent.db")
	missing := newLineExamplesHTTP(t, missingPath)
	missing.ledgerIsMissing()
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("the snapshot is there: %v", err)
	}
	without := effectsOver(missing)
	cleared := 0
	for _, effect := range present {
		if effect.Table == "tx_state" {
			cleared++
		}
	}
	if cleared != 8 {
		t.Errorf("with the snapshot the save has %d effects on tx_state, want 8", cleared)
	}
	if !reflect.DeepEqual(without, present) {
		t.Errorf("without the snapshot the effects are\n%+v\nwant\n%+v", without, present)
	}
}

func TestWithTheSnapshotMissingAnUndoThatWouldTakeALineFromUnderATransactionIsRefused(t *testing.T) {
	env := newLineExamplesHTTP(t, filepath.Join(t.TempDir(), "absent.db"))
	env.ledgerIsMissing()
	added := changeIDOfAnswer(t, env.post("budget.save", env.saveArguments("2026-03", func(document authored.Document) authored.Document {
		document[2].Lines = append(document[2].Lines, authoredtest.Line("Farmácia", 9_000))
		return document
	})))
	var pharmacy authored.LineID
	for _, line := range env.budget("2026-03").Lines {
		if line.Name == "Farmácia" {
			pharmacy = line.ID
		}
	}
	env.record("scratch.setLine", func(recorder *authored.Recorder) error {
		return authoredtest.WriteState(t.Context(), recorder, env.examples.Keys["t7"], authored.TransactionState{Line: &pharmacy, ClassifiedBy: authored.AuthorPerson})
	})
	everything, journal := env.scratch.Everything(t), env.scratch.Journal(t)
	expectRefused(t, "the undo of the save that added the line", env.undo(added), string(reasonStaleChange))
	if env.scratch.Everything(t) != everything || env.scratch.Journal(t) != journal {
		t.Error("the refused undo changed the file")
	}
}

// boletoListedIn is the March boleto as transactions.list answers it.
func boletoListedIn(t *testing.T, env *lineExamplesHTTP) map[string]any {
	t.Helper()
	response := env.post("transactions.list", `[{"month":"2026-03","period":"month"}]`)
	var listed []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil || response.Code != http.StatusOK {
		t.Fatalf("the list answered %d %s", response.Code, response.Body)
	}
	for _, transaction := range listed {
		if transaction["id"] == txBoleto {
			return transaction
		}
	}
	t.Fatalf("the list of 2026-03 has no %s", txBoleto)
	return nil
}

func askReasonOf(t *testing.T, env *lineExamplesHTTP, id string) any {
	t.Helper()
	response := env.post("transactions.detail", `["`+id+`"]`)
	var detail map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil || response.Code != http.StatusOK {
		t.Fatalf("the detail answered %d %s", response.Code, response.Body)
	}
	return detail["askReason"]
}

func TestAfterASaveRemovesItsLineAListedDebitAnswersUncategorisedWithNoQuestion(t *testing.T) {
	fixture := newFullLedger(t)
	env := newLineExamplesHTTP(t, fixture.Path)
	snapshot := viewOf(t, fixture.Path)
	env.record("scratch.setLine", func(recorder *authored.Recorder) error {
		view := recorder.View()
		defer view.Close()
		target, found, err := lookupTarget(t.Context(), Writes{Ledger: snapshot, Authored: view, Recorder: recorder}, txBoleto)
		if err != nil || !found {
			t.Fatalf("the boleto is not listed: %v", err)
		}
		return authoredtest.WriteState(t.Context(), recorder, target.key, authored.TransactionState{
			Line: &env.examples.Delivery, ClassifiedBy: authored.AuthorModel, Confidence: authoredtest.Some[int64](77), AskReason: true,
		})
	})
	if listed := boletoListedIn(t, env); listed["line"] != "Delivery" || listed["classifiedBy"] == nil || listed["confidence"] == nil || askReasonOf(t, env, txBoleto) != true {
		t.Fatalf("before the save the boleto answers %v, %v and a question %v", listed["line"], listed["classifiedBy"], askReasonOf(t, env, txBoleto))
	}
	changeIDOfAnswer(t, env.post("budget.save", env.saveArguments("2026-03", authoredtest.Without("Delivery"))))
	listed := boletoListedIn(t, env)
	for _, field := range []string{"line", "classifiedBy", "confidence"} {
		if listed[field] != nil {
			t.Errorf("after the save the boleto's %s is %v, want null", field, listed[field])
		}
	}
	if asked := askReasonOf(t, env, txBoleto); asked != false {
		t.Errorf("after the save the detail's askReason is %v, want false", asked)
	}
}

func TestTheLineMenuOfTheCurrentMonthKeepsALineALaterSaveRemovesAndIsEmptyOnceItsMonthHasNone(t *testing.T) {
	env := newLineExamplesHTTP(t, newFullLedger(t).Path)
	menu := &budgetHTTP{env.writeEnv}
	changeIDOfAnswer(t, env.post("budget.save", env.saveArguments("2026-04", authoredtest.Without("Delivery"))))
	if got := menuOf(t, menu, "2026-03"); !strings.Contains(got, "Delivery (") {
		t.Errorf("after a save for 2026-04 March offers %q", got)
	}
	changeIDOfAnswer(t, env.post("budget.save", env.saveArguments("2026-03", authoredtest.Emptied)))
	if body := menu.ok("transactions.filterOptions", `["2026-03"]`); !strings.Contains(string(body), `"lines":[]`) {
		t.Errorf("a month with no line offers %s", body)
	}
}
