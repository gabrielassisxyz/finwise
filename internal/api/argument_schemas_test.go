package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/contract"
)

func TestEveryMethodsArgumentSchemaCompilesAtStart(t *testing.T) {
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := compileArgumentSchemas(methods)
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) == 0 || len(schemas) != len(methods) {
		t.Errorf("%d schemas compiled for %d methods", len(schemas), len(methods))
	}
}

// A schema that does not compile stops the program at start, wherever in the
// manifest it is: the first method, one in the middle, the last.
func TestASchemaThatDoesNotCompileStopsTheRouterAtStart(t *testing.T) {
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	broken := map[string]json.RawMessage{
		"a type no draft has":    json.RawMessage(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"nope"}`),
		"a reference to nothing": json.RawMessage(`{"$schema":"http://json-schema.org/draft-07/schema#","$ref":"#/definitions/Missing"}`),
		"a document that is not": json.RawMessage(`not json`),
		"no document at all":     nil,
	}
	for _, at := range []int{0, len(methods) / 2, len(methods) - 1} {
		for label, document := range broken {
			changed := append([]contract.Method(nil), methods...)
			changed[at].ArgumentsSchema = document
			_, err := NewHandler(Config{Methods: changed, VerifyPIN: func(string) bool { return false }, Now: newFakeClock().Now})
			if err == nil {
				t.Errorf("%s in %s was accepted", label, methods[at].Qualified())
				continue
			}
			if !strings.Contains(err.Error(), methods[at].Qualified()) {
				t.Errorf("the error for %s does not name the method: %v", label, err)
			}
		}
	}
}

// budgetSave is what a handler of budget.save sees: how many times it was
// reached and, for the last document it read, every line's amount in cents.
type budgetSave struct {
	calls int
	cents []int64
}

// savingTheBudget asks for the change's id first, so that a request it refuses
// has a row to take back, and then reads the document the way a real handler
// would: strictly, with every amount as Reais.
func savingTheBudget(into *budgetSave) WriteHandler {
	return func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
		into.calls++
		if _, err := writes.Recorder.ID(ctx); err != nil {
			return nil, err
		}
		var month string
		if err := decodeArgument(args[0], &month); err != nil {
			return nil, err
		}
		var groups []struct {
			Key   string `json:"key"`
			Name  string `json:"name"`
			Lines []struct {
				ID     *string `json:"id"`
				Name   string  `json:"name"`
				Amount Reais   `json:"amount"`
				Flag   *string `json:"flag"`
			} `json:"lines"`
		}
		if err := decodeArgument(args[1], &groups); err != nil {
			return nil, err
		}
		into.cents = nil
		for _, group := range groups {
			for _, line := range group.Lines {
				into.cents = append(into.cents, int64(line.Amount))
			}
		}
		return answerWithChange(ctx, writes)
	}
}

func TestArgumentsThatDoNotMatchTheManifestNeverReachAWriteHandler(t *testing.T) {
	var saved budgetSave
	env := newWriteEnv(t, withWrite("budget.save", savingTheBudget(&saved)))
	before := env.scratch.State(t)
	rejected := map[string]string{
		"an amount sent as a string":         `["2026-03",[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":"8000"}]}]]`,
		"lines sent as an object":            `["2026-03",[{"key":"rec","name":"Receita","lines":{}}]]`,
		"a group key outside the five":       `["2026-03",[{"key":"inv","name":"Investimento","lines":[]}]]`,
		"a line without an amount":           `["2026-03",[{"key":"rec","name":"Receita","lines":[{"name":"Salário"}]}]]`,
		"a line with an extra field":         `["2026-03",[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":8000,"extra":1}]}]]`,
		"a group with an extra field":        `["2026-03",[{"key":"rec","name":"Receita","lines":[],"extra":1}]]`,
		"a group without a name":             `["2026-03",[{"key":"rec","lines":[]}]]`,
		"a document that is an object":       `["2026-03",{}]`,
		"a null document":                    `["2026-03",null]`,
		"a month that is a number":           `[202603,[]]`,
		"a null month":                       `[null,[]]`,
		"one argument too few":               `["2026-03"]`,
		"one argument too many":              `["2026-03",[],[]]`,
		"no argument at all":                 `[]`,
		"an argument list that is an object": `{"month":"2026-03"}`,
	}
	for label, body := range rejected {
		expectError(t, label, env.post("budget.save", body), http.StatusBadRequest, "bad_request")
	}
	if saved.calls != 0 {
		t.Errorf("the handler was reached %d times by arguments the manifest does not allow", saved.calls)
	}
	if after := env.scratch.State(t); after != before {
		t.Errorf("a rejected request left a trace:\n%s\nwas\n%s", after, before)
	}
}

// A schema says a number is a number; whether it is an amount the app takes is
// the handler's to decide, and a refusal there undoes everything the handler
// had already written.
func TestAnAmountTheHandlerRefusesLeavesNoChange(t *testing.T) {
	var saved budgetSave
	env := newWriteEnv(t, withWrite("budget.save", savingTheBudget(&saved)))
	before := env.scratch.State(t)
	for _, amount := range []string{"1e400", "1000000000.01", "-1000000000.01"} {
		body := `["2026-03",[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":` + amount + `}]}]]`
		expectError(t, amount, env.post("budget.save", body), http.StatusBadRequest, "bad_request")
	}
	if saved.calls != 3 {
		t.Errorf("the handler was reached %d times, want 3: a number is something the schema allows", saved.calls)
	}
	if after := env.scratch.State(t); after != before {
		t.Errorf("an amount the handler refused left a trace:\n%s\nwas\n%s", after, before)
	}
}

func TestAWellFormedDocumentReachesTheHandlerWithItsAmountsInCents(t *testing.T) {
	var saved budgetSave
	env := newWriteEnv(t, withWrite("budget.save", savingTheBudget(&saved)))

	response := env.post("budget.save", `["2026-03",[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":8000}]}]]`)

	changeIDOfAnswer(t, response)
	if saved.calls != 1 || len(saved.cents) != 1 || saved.cents[0] != 800000 {
		t.Errorf("the handler read %v in %d calls, want 800000 cents in one", saved.cents, saved.calls)
	}
	if problem := conformanceProblem("budget.save", response.Body.Bytes()); problem != "" {
		t.Errorf("the answer does not conform to the manifest: %s", problem)
	}
}
