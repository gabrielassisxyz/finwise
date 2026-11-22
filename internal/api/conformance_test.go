package api

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gabrielassisxyz/finwise/internal/contract"
)

// readCase is one call of a read method. Every registered read handler needs at
// least one, and each is answered over both fixture ledgers, validated against
// the method's result schema and compared with its golden file.
type readCase struct {
	method    string
	arguments string
}

var readCases = []readCase{
	{"shell.overview", `[]`},
	{"accounts.list", `[]`},
	{"accounts.detail", `["acc-itau"]`},
	{"accounts.detail", `["card-itau"]`},
	{"accounts.detail", `["no-such-account"]`},
	{"transactions.list", `[{"month":"2026-02","period":"month"}]`},
	{"transactions.list", `[{"month":"2026-03","period":"month"}]`},
	{"transactions.list", `[{"month":"2026-03","period":"last-3-months"}]`},
	{"transactions.list", `[{"month":"2026-03","period":"year"}]`},
	{"transactions.list", `[{"month":"2025-12","period":"month"}]`},
	{"transactions.filterOptions", `["2026-03"]`},
	{"transactions.detail", `["tx-instalment"]`},
	{"transactions.detail", `["tx-foreign"]`},
	{"transactions.detail", `["tx-month-end"]`},
	{"transactions.detail", `["tx-own-transfer"]`},
	{"transactions.detail", `["tx-pending"]`},
	{"transactions.detail", `["tx-salary"]`},
	{"transactions.detail", `["tx-unconverted"]`},
	{"transactions.detail", `["no-such-transaction"]`},
	{"card.overview", `[]`},
	{"savingsPots.list", `[{"includeClosed":false}]`},
	{"savingsPots.list", `[{"includeClosed":true}]`},
	{"savingsPots.detail", `["reserve:acc-pots/pot-1"]`},
	{"savingsPots.detail", `["no-such-pot"]`},
	{"netWorth.overview", `[]`},
	{"home.summary", `["2026-03"]`},
}

// manifestSchemas compiles each read method's result schema once. The manifest is
// draft-07 and a `$ref` resolves against its root as #/definitions/<Name>, so
// each method's schema is placed in a document that carries the definitions.
type manifestSchemas struct {
	once    sync.Once
	schemas map[string]*jsonschema.Schema
	err     error
}

var resultSchemas manifestSchemas

func (compiled *manifestSchemas) of(method string) (*jsonschema.Schema, error) {
	compiled.once.Do(func() { compiled.schemas, compiled.err = compileResultSchemas() })
	if compiled.err != nil {
		return nil, compiled.err
	}
	schema, ok := compiled.schemas[method]
	if !ok {
		return nil, fmt.Errorf("the manifest has no result schema for %s", method)
	}
	return schema, nil
}

func compileResultSchemas() (map[string]*jsonschema.Schema, error) {
	raw, err := os.ReadFile("../contract/contract.json")
	if err != nil {
		return nil, err
	}
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	manifest, ok := parsed.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("the manifest is not an object")
	}
	schemas := map[string]*jsonschema.Schema{}
	for _, entry := range manifest["methods"].([]any) {
		method := entry.(map[string]any)
		name := method["domain"].(string) + "." + method["name"].(string)
		url := "urn:finwise:result:" + name
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft7)
		document := map[string]any{
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"definitions": manifest["definitions"],
			"allOf":       []any{method["result"]},
		}
		if err := compiler.AddResource(url, document); err != nil {
			return nil, err
		}
		if schemas[name], err = compiler.Compile(url); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return schemas, nil
}

// conformanceProblem is empty when the body is a valid answer of the method.
func conformanceProblem(method string, body []byte) string {
	schema, err := resultSchemas.of(method)
	if err != nil {
		return err.Error()
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return "the answer is not JSON: " + err.Error()
	}
	if err := schema.Validate(instance); err != nil {
		return err.Error()
	}
	return ""
}

func TestEveryRegisteredReadHasAConformanceCase(t *testing.T) {
	covered := map[string]bool{}
	for _, c := range readCases {
		covered[c.method] = true
	}
	for _, name := range NewReadRegistry().Names() {
		if !covered[name] {
			t.Errorf("%s is registered and has no entry in readCases", name)
		}
	}
	registered := map[string]bool{}
	for _, name := range NewReadRegistry().Names() {
		registered[name] = true
	}
	for _, c := range readCases {
		if !registered[c.method] {
			t.Errorf("readCases names %s, which has no handler", c.method)
		}
	}
}

func TestEveryAnswerValidatesAgainstItsResultSchema(t *testing.T) {
	for _, name := range []string{"full", "empty"} {
		env := newReadEnv(t, ledgerNamed(t, name))
		for _, c := range readCases {
			status, body := env.answer(c.method, c.arguments)
			if status != http.StatusOK {
				t.Errorf("%s ledger, %s %s: status %d", name, c.method, c.arguments, status)
				continue
			}
			if problem := conformanceProblem(c.method, body); problem != "" {
				t.Errorf("%s ledger, %s %s does not conform: %s", name, c.method, c.arguments, problem)
			}
		}
	}
}

func TestTheConformanceCheckRejectsAnAnswerThatDoesNotMatch(t *testing.T) {
	good := `{"today":"2026-03-20","pendingCount":0,"sync":{"state":"warning","message":"x"},` +
		`"firstMonth":"2026-01","lastMonth":"2026-03","closedMonths":[]}`
	if problem := conformanceProblem("shell.overview", []byte(good)); problem != "" {
		t.Fatalf("the control answer was refused: %s", problem)
	}
	broken := map[string]string{
		"a renamed field": strings.Replace(good, `"today"`, `"day"`, 1),
		"a missing field": strings.Replace(good, `"pendingCount":0,`, "", 1),
		"an extra field":  strings.Replace(good, `"closedMonths":[]`, `"closedMonths":[],"extra":1`, 1),
		"a null list":     strings.Replace(good, `"closedMonths":[]`, `"closedMonths":null`, 1),
		"a string amount": strings.Replace(good, `"pendingCount":0`, `"pendingCount":"0"`, 1),
	}
	for label, body := range broken {
		if conformanceProblem("shell.overview", []byte(body)) == "" {
			t.Errorf("%s went unnoticed", label)
		}
	}
}

func TestEveryMethodOfTheManifestHasAResultSchema(t *testing.T) {
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range methods {
		if method.Class != contract.ClassRead {
			continue
		}
		if _, err := resultSchemas.of(method.Qualified()); err != nil {
			t.Error(err)
		}
	}
}
