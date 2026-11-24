package contract

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func byQualified(t *testing.T) map[string]Method {
	t.Helper()
	methods, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	index := make(map[string]Method, len(methods))
	for _, method := range methods {
		index[method.Qualified()] = method
	}
	return index
}

func TestLoadReadsEveryMethodOfTheManifest(t *testing.T) {
	methods, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	counts := map[Class]int{}
	for _, method := range methods {
		counts[method.Class]++
	}
	want := map[Class]int{ClassSession: 3, ClassRead: 24, ClassEvaluate: 1, ClassWrite: 56}
	for class, n := range want {
		if counts[class] != n {
			t.Errorf("class %s: %d methods, want %d", class, counts[class], n)
		}
	}
}

func TestSessionMethodsAreClassifiedAsTheServerExpects(t *testing.T) {
	index := byQualified(t)
	want := map[string]Class{
		"session.status":    ClassSession,
		"session.unlock":    ClassSession,
		"session.lock":      ClassSession,
		"session.changePin": ClassWrite,
	}
	for name, class := range want {
		if index[name].Class != class {
			t.Errorf("%s: class %q, want %q", name, index[name].Class, class)
		}
	}
}

func TestArgumentBoundsFollowTheSchema(t *testing.T) {
	index := byQualified(t)
	cases := map[string][2]int{
		"session.status":             {0, 0},
		"session.unlock":             {1, 1},
		"session.changePin":          {3, 3},
		"budget.view":                {1, 2},
		"transactions.filterOptions": {1, 1},
	}
	for name, bounds := range cases {
		got := [2]int{index[name].MinArguments, index[name].MaxArguments}
		if got != bounds {
			t.Errorf("%s: bounds %v, want %v", name, got, bounds)
		}
	}
}

func TestParseRefusesWhatItDoesNotUnderstand(t *testing.T) {
	// Every case differs from a manifest parse accepts in one thing only, so each
	// is refused for its own reason and not for a definition it happens to lack.
	const reasons = `"RefusalReason":{"type":"string","enum":["month_closed"]}`
	definitions := `,"definitions":{` + reasons + `,"Month":{"type":"string"}}}`
	manifest := func(methods string) string { return `{"methods":[` + methods + `]` + definitions }
	method := func(route, class, arguments string) string {
		return `{"domain":"a","name":"b","route":"` + route + `","class":"` + class + `"` + arguments + `}`
	}
	const noArguments = `,"arguments":{"type":"array","maxItems":0}`
	one := method("/api/a/b", "read", noArguments)
	withReasons := func(definition string) string {
		return `{"methods":[` + one + `],"definitions":{"Month":{"type":"string"},"RefusalReason":` + definition + `}}`
	}
	accepted := []string{
		manifest(one),
		manifest(method("/api/a/b", "read", `,"arguments":{"items":[{"$ref":"#/definitions/Month"}]}`)),
	}
	for _, raw := range accepted {
		if _, err := parse([]byte(raw)); err != nil {
			t.Fatalf("a well-formed manifest the cases start from was refused: %v", err)
		}
	}
	cases := map[string]string{
		"unknown class":            manifest(method("/api/a/b", "delete", noArguments)),
		"wrong route":              manifest(method("/api/a/c", "read", noArguments)),
		"no methods":               `{"methods":[]` + definitions,
		"not json":                 `{`,
		"duplicate":                manifest(one + `,` + one),
		"no argument schema":       manifest(method("/api/a/b", "read", ``)),
		"null argument schema":     manifest(method("/api/a/b", "read", `,"arguments":null`)),
		"reference to nothing":     manifest(method("/api/a/b", "read", `,"arguments":{"items":[{"$ref":"#/definitions/Day"}]}`)),
		"reference outside":        manifest(method("/api/a/b", "read", `,"arguments":{"items":[{"$ref":"other.json#/Month"}]}`)),
		"no definitions":           `{"methods":[` + one + `]}`,
		"no reasons":               `{"methods":[` + one + `],"definitions":{"Month":{"type":"string"}}}`,
		"reasons not strings":      withReasons(`{"type":"number","enum":[1]}`),
		"reasons empty":            withReasons(`{"type":"string","enum":[]}`),
		"reasons repeated":         withReasons(`{"type":"string","enum":["not_found","not_found"]}`),
		"reasons behind a $ref":    withReasons(`{"$ref":"#/definitions/Month"}`),
		"reasons of another shape": withReasons(`{"type":"string","enum":[{"reason":"x"}]}`),
	}
	for name, raw := range cases {
		if _, err := parse([]byte(raw)); err == nil {
			t.Errorf("%s: parse accepted it", name)
		}
	}
}

func TestRefusalReasonsAreTheContractsInItsOrder(t *testing.T) {
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	want := []string{
		"month_closed", "future_month", "pending_reviews", "stale_change", "not_found",
		"unknown_line", "invalid_budget", "unstable_pot", "split_with_reimbursement", "proposal_stale",
	}
	if !slices.Equal(manifest.RefusalReasons, want) {
		t.Errorf("reasons %q, want %q", manifest.RefusalReasons, want)
	}
}

func compileArguments(t *testing.T, method Method) *jsonschema.Schema {
	t.Helper()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(method.ArgumentsSchema))
	if err != nil {
		t.Fatalf("%s: arguments schema is not JSON: %v", method.Qualified(), err)
	}
	url := "urn:finwise:arguments:" + method.Qualified()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft7)
	if err := compiler.AddResource(url, document); err != nil {
		t.Fatalf("%s: %v", method.Qualified(), err)
	}
	schema, err := compiler.Compile(url)
	if err != nil {
		t.Fatalf("%s: arguments schema does not compile: %v", method.Qualified(), err)
	}
	return schema
}

func TestEveryArgumentSchemaCompilesOnItsOwn(t *testing.T) {
	methods, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(methods) != 84 {
		t.Fatalf("%d methods, want 84", len(methods))
	}
	for _, method := range methods {
		if len(method.ArgumentsSchema) == 0 {
			t.Errorf("%s: no arguments schema", method.Qualified())
			continue
		}
		compileArguments(t, method)
	}
}

func TestBudgetSaveArgumentsAreCheckedAgainstTheContract(t *testing.T) {
	schema := compileArguments(t, byQualified(t)["budget.save"])
	const budget = `[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":8000}]}]`
	cases := []struct {
		name  string
		body  string
		valid bool
	}{
		{"a budget", `["2026-03",` + budget + `]`, true},
		{"amount as a string", `["2026-03",[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":"8000"}]}]]`, false},
		{"an extra field on the line", `["2026-03",[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":8000,"note":"x"}]}]]`, false},
		{"a third element", `["2026-03",` + budget + `,null]`, false},
	}
	for _, tc := range cases {
		instance, err := jsonschema.UnmarshalJSON(strings.NewReader(tc.body))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if err := schema.Validate(instance); (err == nil) != tc.valid {
			t.Errorf("%s: valid=%v, want %v (%v)", tc.name, err == nil, tc.valid, err)
		}
	}
}
