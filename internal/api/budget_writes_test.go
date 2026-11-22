package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// b0 is the invented budget the examples of the budget's rules start from,
// sent without ids.
const b0 = `[` +
	`{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":10000},{"name":"Renda extra","amount":1500,"flag":"Não garantida"}]},` +
	`{"key":"fix","name":"Fixos e essenciais","lines":[{"name":"Contas da casa","amount":2000},{"name":"Saúde","amount":500,"flag":""}]},` +
	`{"key":"var","name":"Variáveis","lines":[{"name":"Mercado","amount":400},{"name":"Delivery","amount":200}]},` +
	`{"key":"prov","name":"Provisões","lines":[{"name":"Remédios","amount":150}]},` +
	`{"key":"res","name":"Reservas de desejo","lines":[{"name":"Viagens","amount":600}]}]`

// budgetHTTP is a router over the full invented ledger and an app's database,
// with a session open.
type budgetHTTP struct {
	*writeEnv
}

func newBudgetHTTP(t *testing.T) *budgetHTTP {
	t.Helper()
	return &budgetHTTP{newCaseEnv(t, writeCase{})}
}

func (env *budgetHTTP) ok(method, arguments string) []byte {
	env.t.Helper()
	response := env.post(method, arguments)
	if response.Code != http.StatusOK {
		env.t.Fatalf("%s %s: %d %s", method, arguments, response.Code, response.Body)
	}
	return response.Body.Bytes()
}

// document reads the budget of a month as its groups.
func (env *budgetHTTP) document(month string) []map[string]any {
	env.t.Helper()
	var groups []map[string]any
	decodeKeepingNumbers(env.t, env.ok("budget.document", `["`+month+`"]`), &groups)
	return groups
}

// decodeKeepingNumbers reads a body without turning an amount into a float, so
// that the text the wire carries is what a test compares.
func decodeKeepingNumbers(t *testing.T, body []byte, into any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(into); err != nil {
		t.Fatal(err)
	}
}

func linesOf(groups []map[string]any, key string) []map[string]any {
	for _, group := range groups {
		if group["key"] == key {
			var lines []map[string]any
			for _, line := range group["lines"].([]any) {
				lines = append(lines, line.(map[string]any))
			}
			return lines
		}
	}
	return nil
}

func (env *budgetHTTP) lineNamed(month, name string) map[string]any {
	env.t.Helper()
	for _, group := range env.document(month) {
		for _, line := range linesOf([]map[string]any{group}, group["key"].(string)) {
			if line["name"] == name {
				return line
			}
		}
	}
	return nil
}

// amountOf is a line's amount as the wire writes it.
func (env *budgetHTTP) amountOf(month, name string) string {
	env.t.Helper()
	line := env.lineNamed(month, name)
	if line == nil {
		return "missing"
	}
	return string(mustMarshal(env.t, line["amount"]))
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// edited sends back what a month's document says, after change has had its way
// with it, as the text of a save's document. Numbers stay as they were read.
func (env *budgetHTTP) edited(month string, change func(groups []map[string]any)) string {
	env.t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(env.ok("budget.document", `["`+month+`"]`)))
	decoder.UseNumber()
	var groups []map[string]any
	if err := decoder.Decode(&groups); err != nil {
		env.t.Fatal(err)
	}
	change(groups)
	return string(mustMarshal(env.t, groups))
}

func withAmount(name string, amount string) func([]map[string]any) {
	return func(groups []map[string]any) {
		for _, group := range groups {
			for _, line := range group["lines"].([]any) {
				if line.(map[string]any)["name"] == name {
					line.(map[string]any)["amount"] = json.Number(amount)
				}
			}
		}
	}
}

func withRename(from, to string) func([]map[string]any) {
	return func(groups []map[string]any) {
		for _, group := range groups {
			for _, line := range group["lines"].([]any) {
				if line.(map[string]any)["name"] == from {
					line.(map[string]any)["name"] = to
				}
			}
		}
	}
}

func (env *budgetHTTP) closeMonths(months ...string) {
	for _, month := range months {
		authoredtest.CloseMonth(env.t, env.scratch.Database, fixtureNow, brday.Month(month), "2026-03-05")
	}
}

func (env *budgetHTTP) untouched(label string, write func()) {
	env.t.Helper()
	before := env.state()
	write()
	if after := env.state(); after.everything != before.everything || after.journal != before.journal {
		env.t.Errorf("%s changed the file or the journal:\n%s", label, differences(before.everything+before.journal, after.everything+after.journal))
	}
}

var decimalID = regexp.MustCompile(`^[1-9][0-9]*$`)

func TestASavedBudgetIsAnsweredWithAnIDOnEveryLineAndTheSameOnTheNextRead(t *testing.T) {
	env := newBudgetHTTP(t)
	env.ok("budget.save", `["2026-02",`+b0+`]`)
	first := env.ok("budget.document", `["2026-02"]`)
	seen := map[string]bool{}
	var groups []map[string]any
	decodeKeepingNumbers(t, first, &groups)
	for _, group := range groups {
		for _, line := range linesOf([]map[string]any{group}, group["key"].(string)) {
			id, _ := line["id"].(string)
			if !decimalID.MatchString(id) || id == "1" {
				t.Errorf("%v has the id %q", line["name"], id)
			}
			seen[id] = true
		}
	}
	if len(seen) != 8 {
		t.Errorf("%d distinct ids on 8 lines", len(seen))
	}
	for _, month := range []string{"2026-02", "2026-03", "2026-01"} {
		if got := env.ok("budget.document", `["`+month+`"]`); !bytes.Equal(got, first) {
			t.Errorf("%s answers another document than the one saved from 2026-02", month)
		}
	}
	if line := env.lineNamed("2026-02", "Saúde"); line["flag"] != nil {
		t.Errorf("a line saved with the flag \"\" is answered with %v", line["flag"])
	}
	if line := env.lineNamed("2026-02", "Renda extra"); line["flag"] != "Não garantida" {
		t.Errorf("Renda extra has the flag %v", line["flag"])
	}
}

func TestAnAmountIsStoredFromItsDecimalTextAndAnsweredWithTwoPlaces(t *testing.T) {
	env := newBudgetHTTP(t)
	doc := `[{"key":"fix","name":"x","lines":[{"name":"Mercado-a","amount":10.005},{"name":"Mercado-b","amount":1e3},{"name":"Mercado-c","amount":0.1}]}]`
	env.ok("budget.save", `["2026-02",`+doc+`]`)
	for name, want := range map[string]string{"Mercado-a": "10.01", "Mercado-b": "1000.00", "Mercado-c": "0.10"} {
		if got := env.amountOf("2026-02", name); got != want {
			t.Errorf("%s is answered %s, want %s", name, got, want)
		}
	}
}

// One table, run over save and over applyScenario, which are one operation
// under two names.
func TestSavingAndApplyingAScenarioWriteTheSameVersions(t *testing.T) {
	cases := []struct {
		name   string
		closed []string
		month  string
		want   map[string]string
	}{
		{"holds from its month on", nil, "2026-03",
			map[string]string{"2026-04": "900.00", "2026-03": "900.00", "2026-02": "500.00", "2026-01": "500.00"}},
		{"keeps a closed month and resumes after it", []string{"2026-04"}, "2026-03",
			map[string]string{"2026-03": "900.00", "2026-04": "500.00", "2026-05": "900.00"}},
		{"keeps two closed months in a row", []string{"2026-04", "2026-05"}, "2026-03",
			map[string]string{"2026-04": "500.00", "2026-05": "500.00", "2026-06": "900.00"}},
		{"an earlier month becomes the first", nil, "2026-01",
			map[string]string{"2025-12": "900.00", "2026-03": "900.00"}},
	}
	for _, method := range []string{"budget.save", "budget.applyScenario"} {
		for _, c := range cases {
			t.Run(method+" "+c.name, func(t *testing.T) {
				env := newBudgetHTTP(t)
				env.ok("budget.save", `["2026-02",`+b0+`]`)
				env.closeMonths(c.closed...)
				doc := env.edited(c.month, withAmount("Saúde", "900"))
				before := env.state().canonical
				id := changeIDOfAnswer(t, env.post(method, `["`+c.month+`",`+doc+`]`))
				for month, want := range c.want {
					if got := env.amountOf(month, "Saúde"); got != want {
						t.Errorf("%s: Saúde is %s, want %s", month, got, want)
					}
				}
				changes := env.scratch.Changes(t)
				if last := changes[len(changes)-1]; last.Method != method || last.Author != "person" {
					t.Errorf("the change is %+v, want author person and method %s", last, method)
				}
				for _, effect := range env.scratch.EffectsOf(t, changeNumber(t, id)) {
					if effect.Table == "lines" {
						t.Errorf("an effect names lines: %+v", effect)
					}
				}
				if undone := env.undo(id); undone.Code != http.StatusOK {
					t.Fatalf("undo: %d %s", undone.Code, undone.Body)
				}
				if after := env.state().canonical; after != before {
					t.Errorf("the undo left another state:\n%s", differences(before, after))
				}
			})
		}
	}
}

func TestARenameKeepsTheIDAndEachMonthKeepsTheNameItHad(t *testing.T) {
	env := newBudgetHTTP(t)
	env.ok("budget.save", `["2026-02",`+b0+`]`)
	env.closeMonths("2026-04")
	id := env.lineNamed("2026-02", "Mercado")["id"]
	env.ok("budget.save", `["2026-03",`+env.edited("2026-03", withRename("Mercado", "Feira"))+`]`)
	for month, name := range map[string]string{"2026-02": "Mercado", "2026-03": "Feira", "2026-04": "Mercado", "2026-05": "Feira"} {
		line := env.lineNamed(month, name)
		if line == nil || line["id"] != id {
			t.Errorf("%s: %s is %v, want the id %v", month, name, line, id)
		}
	}
}

func TestASaveForAClosedMonthIsRefusedWhateverTheDocumentAndChangesNothing(t *testing.T) {
	for _, method := range []string{"budget.save", "budget.applyScenario"} {
		for name, doc := range map[string]string{"valid": b0, "invalid": twiceTheName} {
			t.Run(method+" "+name, func(t *testing.T) {
				env := newBudgetHTTP(t)
				env.ok("budget.save", `["2026-02",`+b0+`]`)
				env.closeMonths("2026-03")
				before := env.state()
				lines := env.lineCount()
				response := env.post(method, `["2026-03",`+doc+`]`)
				want := `{"error":{"kind":"refused","reason":"month_closed"}}`
				if response.Code != http.StatusConflict || response.Body.String() != want {
					t.Errorf("answered %d %s, want 409 %s", response.Code, response.Body, want)
				}
				after := env.state()
				if after.canonical != before.canonical || after.changes != before.changes || env.lineCount() != lines {
					t.Error("a refused save changed the tables, the journal or the line ids")
				}
			})
		}
	}
}

func (env *budgetHTTP) lineCount() int {
	env.t.Helper()
	var count int
	if err := env.scratch.Inspect(env.t).QueryRow("SELECT count(*) FROM lines").Scan(&count); err != nil {
		env.t.Fatal(err)
	}
	return count
}

func TestADocumentThatIsNotValidIsRefusedByAllThreeWritesAndChangesNothing(t *testing.T) {
	line := func(fields string) string {
		return `[{"key":"fix","name":"Fixos e essenciais","lines":[{` + fields + `}]}]`
	}
	cases := map[string]string{
		"an empty name":            line(`"name":"","amount":1`),
		"a name of three spaces":   line(`"name":"   ","amount":1`),
		"a name twice in a group":  twiceTheName,
		"a name in two groups":     `[{"key":"var","name":"x","lines":[{"name":"Mercado","amount":1}]},{"key":"fix","name":"y","lines":[{"name":"Mercado","amount":1}]}]`,
		"the fees line's name":     line(`"name":"Tarifas e juros","amount":1`),
		"an id that names no line": line(`"id":"999999","name":"Mercado","amount":1`),
		"an id that is text":       line(`"id":"abc","name":"Mercado","amount":1`),
		"an empty id":              line(`"id":"","name":"Mercado","amount":1`),
		"the same id twice": `[{"key":"fix","name":"x","lines":[{"id":"6","name":"Mercado","amount":1},` +
			`{"id":"6","name":"Delivery","amount":1}]}]`,
		"the fees line's id": line(`"id":"1","name":"Mercado","amount":1`),
		"a group twice":      `[{"key":"var","name":"x","lines":[]},{"key":"var","name":"x","lines":[]}]`,
	}
	for name, doc := range cases {
		for _, method := range []string{"budget.save", "budget.applyScenario", "budget.saveScenario"} {
			t.Run(name+" by "+method, func(t *testing.T) {
				env := newBudgetHTTP(t)
				env.ok("budget.save", `["2026-02",`+b0+`]`)
				first := `"2026-03"`
				if method == "budget.saveScenario" {
					first = `"Mercado-cenario"`
				}
				env.untouched(method, func() {
					response := env.post(method, `[`+first+`,`+doc+`]`)
					want := `{"error":{"kind":"refused","reason":"invalid_budget"}}`
					if response.Code != http.StatusConflict || response.Body.String() != want {
						t.Errorf("answered %d %s, want 409 %s", response.Code, response.Body, want)
					}
				})
			})
		}
	}
}

func TestADocumentTheContractRefusesIsABadRequestAndChangesNothing(t *testing.T) {
	line := func(fields string) string {
		return `[{"key":"fix","name":"Fixos e essenciais","lines":[{` + fields + `}]}]`
	}
	docs := map[string]string{
		"an amount of -1":             line(`"name":"Mercado","amount":-1`),
		"an amount of two billion":    line(`"name":"Mercado","amount":2000000000`),
		"a group key of xyz":          `[{"key":"xyz","name":"x","lines":[]}]`,
		"a field the contract lacks":  line(`"name":"Mercado","amount":1,"color":"red"`),
		"an amount that is text":      line(`"name":"Mercado","amount":"1"`),
		"a line with no name":         line(`"amount":1`),
		"a flag that is not text":     line(`"name":"Mercado","amount":1,"flag":3`),
		"a group with no lines field": `[{"key":"fix","name":"x"}]`,
	}
	for name, doc := range docs {
		for _, method := range []string{"budget.save", "budget.applyScenario", "budget.saveScenario"} {
			t.Run(name+" by "+method, func(t *testing.T) {
				env := newBudgetHTTP(t)
				env.ok("budget.save", `["2026-02",`+b0+`]`)
				first := `"2026-03"`
				if method == "budget.saveScenario" {
					first = `"Mercado-cenario"`
				}
				env.untouched(method, func() {
					expectError(t, name, env.post(method, `[`+first+`,`+doc+`]`), http.StatusBadRequest, "bad_request")
				})
			})
		}
	}
	for _, method := range []string{"budget.save", "budget.applyScenario"} {
		env := newBudgetHTTP(t)
		env.untouched(method, func() {
			expectError(t, "a month of 2026-13", env.post(method, `["2026-13",`+b0+`]`), http.StatusBadRequest, "bad_request")
		})
	}
}

// A handler is a plain function a client other than HTTP will call, so its own
// check of a group key stands even though the router never lets one through.
func TestEachWriteHandlerRefusesAnUnknownGroupKeyBelowTheRouter(t *testing.T) {
	doc := json.RawMessage(`[{"key":"xyz","name":"x","lines":[]}]`)
	calls := map[string]struct {
		handler WriteHandler
		args    []json.RawMessage
	}{
		"budget.save":          {saveBudget, []json.RawMessage{json.RawMessage(`"2026-03"`), doc}},
		"budget.applyScenario": {applyScenario, []json.RawMessage{json.RawMessage(`"2026-03"`), doc}},
		"budget.saveScenario":  {saveScenario, []json.RawMessage{json.RawMessage(`"Mercado-cenario"`), doc}},
	}
	for name, call := range calls {
		scratch := authoredtest.Open(t, fixtureClock)
		facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: name, At: fixtureNow}
		err := scratch.Database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
			_, err := call.handler(t.Context(), call.args, Writes{Recorder: recorder, Authored: recorder.View()})
			return err
		})
		var refusal *refusalError
		if !errors.As(err, &refusal) || refusal.reason != reasonInvalidBudget {
			t.Errorf("%s: err = %v, want the refusal invalid_budget", name, err)
		}
	}
}

func TestAGroupLeftOutIsSavedEmptyAndGroupNamesAndOrderAreTheServers(t *testing.T) {
	env := newBudgetHTTP(t)
	env.ok("budget.save", `["2026-02",`+b0+`]`)
	scrambled := `[{"key":"var","name":"Qualquer","lines":[{"name":"Mercado","amount":1}]},` +
		`{"key":"rec","name":"Outra","lines":[{"name":"Salário","amount":2}]}]`
	env.ok("budget.save", `["2026-03",`+scrambled+`]`)
	var keys, names []string
	for _, group := range env.document("2026-03") {
		keys, names = append(keys, group["key"].(string)), append(names, group["name"].(string))
	}
	if got := strings.Join(keys, " "); got != "rec fix var prov res" {
		t.Errorf("keys = %s", got)
	}
	if got := strings.Join(names, "|"); got != "Receita|Fixos e essenciais|Variáveis|Provisões|Reservas de desejo" {
		t.Errorf("names = %s", got)
	}
	if lines := linesOf(env.document("2026-03"), "res"); len(lines) != 0 {
		t.Errorf("res = %v, want none", lines)
	}
}

func TestSavingTheDocumentJustReadAnswersAnEmptyChangeThatUndoesToNothing(t *testing.T) {
	env := newBudgetHTTP(t)
	env.ok("budget.save", `["2026-02",`+b0+`]`)
	before := env.state().canonical
	id := changeIDOfAnswer(t, env.post("budget.save", `["2026-02",`+env.edited("2026-02", func([]map[string]any) {})+`]`))
	if effects := env.scratch.EffectsOf(t, changeNumber(t, id)); len(effects) != 0 {
		t.Errorf("the change has effects: %+v", effects)
	}
	if undone := env.undo(id); undone.Code != http.StatusOK || env.state().canonical != before {
		t.Errorf("undo answered %d or changed the state", undone.Code)
	}
}

func TestScenariosAreListedInTheOrderTheyWereSavedAndReplacedAtTheEnd(t *testing.T) {
	env := newBudgetHTTP(t)
	if got := string(env.ok("budget.scenarios", `[]`)); got != `[]` {
		t.Errorf("a database with no scenario answers %s", got)
	}
	env.ok("budget.save", `["2026-02",`+b0+`]`)
	blank := env.post("budget.saveScenario", `["  ",`+b0+`]`)
	var saved struct{ ChangeID, Name string }
	if err := json.Unmarshal(blank.Body.Bytes(), &saved); err != nil || saved.Name != "Cenário sem nome" || !decimalID.MatchString(saved.ChangeID) {
		t.Errorf("a blank name answered %d %s", blank.Code, blank.Body)
	}
	if got := scenarioNamesOf(t, env); got != "Cenário sem nome" {
		t.Errorf("scenarios = %q", got)
	}
	var trimmed struct{ Name string }
	_ = json.Unmarshal(env.ok("budget.saveScenario", `["  Sem extra ",`+b0+`]`), &trimmed)
	if trimmed.Name != "Sem extra" {
		t.Errorf("the name kept is %q", trimmed.Name)
	}

	other := newBudgetHTTP(t)
	other.ok("budget.save", `["2026-02",`+b0+`]`)
	for _, name := range []string{"A", "B", "C"} {
		other.ok("budget.saveScenario", `["Antes-`+name+`",`+b0+`]`)
	}
	before := other.state().canonical
	again := changeIDOfAnswer(t, other.post("budget.saveScenario", `["Antes-A",`+strings.Replace(b0, `"amount":500`, `"amount":501`, 1)+`]`))
	if got := scenarioNamesOf(t, other); got != "Antes-B Antes-C Antes-A" {
		t.Errorf("order = %q, want B C A", got)
	}
	if got := string(mustMarshal(t, other.scenarioLine("Antes-A", "Saúde")["amount"])); got != "501.00" {
		t.Errorf("A holds %s, want the second document", got)
	}
	other.undo(again)
	if got := scenarioNamesOf(t, other); got != "Antes-A Antes-B Antes-C" || other.state().canonical != before {
		t.Errorf("after the undo: %q", got)
	}
}

func scenarioNamesOf(t *testing.T, env *budgetHTTP) string {
	t.Helper()
	var scenarios []struct{ Name string }
	if err := json.Unmarshal(env.ok("budget.scenarios", `[]`), &scenarios); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, scenario := range scenarios {
		names = append(names, scenario.Name)
	}
	return strings.Join(names, " ")
}

func (env *budgetHTTP) scenarioLine(scenario, name string) map[string]any {
	env.t.Helper()
	var scenarios []struct {
		Name   string
		Budget []map[string]any
	}
	decodeKeepingNumbers(env.t, env.ok("budget.scenarios", `[]`), &scenarios)
	for _, found := range scenarios {
		if found.Name != scenario {
			continue
		}
		for _, group := range found.Budget {
			for _, line := range linesOf([]map[string]any{group}, group["key"].(string)) {
				if line["name"] == name {
					return line
				}
			}
		}
	}
	return nil
}

func TestAScenarioMintsNothingAndKeepsALineWithoutAnIDWithoutOne(t *testing.T) {
	env := newBudgetHTTP(t)
	lines := env.lineCount()
	env.ok("budget.saveScenario", `["Antes",[{"key":"fix","name":"x","lines":[{"name":"Mercado","amount":5}]}]]`)
	if env.lineCount() != lines {
		t.Error("saving a scenario minted an id")
	}
	if line := env.scenarioLine("Antes", "Mercado"); line == nil || line["id"] != nil {
		t.Errorf("the line is %v, want one with no id", line)
	}
}

func TestAScenarioIsAnsweredWithTheNamesTheCurrentMonthGivesItsLines(t *testing.T) {
	env := newBudgetHTTP(t)
	env.ok("budget.save", `["2026-02",`+b0+`]`)
	env.ok("budget.saveScenario", `["Antes",`+env.edited("2026-02", func([]map[string]any) {})+`]`)
	env.ok("budget.save", `["2026-03",`+env.edited("2026-03", withRename("Mercado", "Feira"))+`]`)
	if env.scenarioLine("Antes", "Feira") == nil || env.scenarioLine("Antes", "Mercado") != nil {
		t.Error("the scenario still names the line as it was saved")
	}
}

func menuOf(t *testing.T, env *budgetHTTP, month string) string {
	t.Helper()
	var options struct {
		Lines []struct{ Name, Group string }
	}
	if err := json.Unmarshal(env.ok("transactions.filterOptions", `["`+month+`"]`), &options); err != nil {
		t.Fatal(err)
	}
	var menu []string
	for _, line := range options.Lines {
		menu = append(menu, line.Name+" ("+line.Group+")")
	}
	return strings.Join(menu, ", ")
}

func TestTheLineMenuIsTheLinesOfTheBudgetInForceForTheMonthAsked(t *testing.T) {
	env := newBudgetHTTP(t)
	if got := menuOf(t, env, "2026-03"); got != "" {
		t.Errorf("a database with no version offers %q", got)
	}
	env.ok("budget.save", `["2026-02",[{"key":"rec","name":"a","lines":[]},{"key":"fix","name":"b","lines":[]}]]`)
	if got := menuOf(t, env, "2026-03"); got != "" {
		t.Errorf("a budget with no line offers %q, the fees line included", got)
	}
	env.ok("budget.save", `["2026-03",[{"key":"rec","name":"a","lines":[{"name":"Salário","amount":1}]}]]`)
	if got := menuOf(t, env, "2026-03"); got != "Tarifas e juros (Tarifas e juros), Salário (Receita)" {
		t.Errorf("a budget with one income line offers %q", got)
	}
	env.ok("budget.save", `["2026-02",`+b0+`]`)
	full := "Mercado (Variáveis), Delivery (Variáveis), Contas da casa (Fixos e essenciais), Saúde (Fixos e essenciais), " +
		"Viagens (Reservas de desejo), Remédios (Provisões), Tarifas e juros (Tarifas e juros), Salário (Receita), Renda extra (Receita)"
	for _, month := range []string{"2026-03", "2026-01"} {
		if got := menuOf(t, env, month); got != full {
			t.Errorf("%s offers\n%s\nwant\n%s", month, got, full)
		}
	}
}

func TestTheLineMenuFollowsTheMonthItIsAskedFor(t *testing.T) {
	env := newBudgetHTTP(t)
	env.ok("budget.save", `["2026-02",`+b0+`]`)
	env.ok("budget.save", `["2026-04",`+env.edited("2026-04", withRename("Mercado", "Feira"))+`]`)
	if got := menuOf(t, env, "2026-03"); !strings.HasPrefix(got, "Mercado (") || strings.Contains(got, "Feira") {
		t.Errorf("March offers %q", got)
	}
	if got := menuOf(t, env, "2026-04"); !strings.HasPrefix(got, "Feira (") || strings.Contains(got, "Mercado") {
		t.Errorf("April offers %q", got)
	}
}

func TestTheFeesLineHasTheNameTheServerGivesItsGroup(t *testing.T) {
	if authored.FeesLineName != budgetGroupFees {
		t.Errorf("the store reserves %q, the menu's group is %q", authored.FeesLineName, budgetGroupFees)
	}
}

func TestASavedBudgetIsAnsweredAfterTheServerRestartsWithTheSameIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finwise.db")
	serve := func() (*testEnv, *authored.Database, *http.Cookie) {
		database, err := authored.Open(t.Context(), path, authored.Options{Now: fixtureClock})
		if err != nil {
			t.Fatal(err)
		}
		env := newTestEnv(t, withLedger(newFullLedger(t).Path, fixtureNow), withDatabase(database))
		return env, database, env.unlock()
	}
	first, database, cookie := serve()
	if response := first.call("/api/budget/save", `["2026-02",`+b0+`]`, cookie); response.Code != http.StatusOK {
		t.Fatalf("save: %d %s", response.Code, response.Body)
	}
	saved := first.call("/api/budget/document", `["2026-02"]`, cookie).Body.Bytes()
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	second, reopened, cookie := serve()
	defer reopened.Close()
	if got := second.call("/api/budget/document", `["2026-02"]`, cookie).Body.Bytes(); !bytes.Equal(got, saved) || !bytes.Contains(got, []byte(`"id"`)) {
		t.Errorf("after the restart the document is\n%s\nwas\n%s", got, saved)
	}
}

func TestWithoutADatabaseTheBudgetWritesAreReadOnlyAndTheReadsAnswerAsAFreshInstall(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	for _, method := range []string{"budget.save", "budget.applyScenario"} {
		expectError(t, method, env.call("/api/"+strings.Replace(method, ".", "/", 1), `["2026-03",`+b0+`]`, env.cookie), http.StatusForbidden, "read_only")
	}
	expectError(t, "budget.saveScenario", env.call("/api/budget/saveScenario", `["A",`+b0+`]`, env.cookie), http.StatusForbidden, "read_only")
	expectAnswer(t, env, "budget.document", `["2026-03"]`, emptyBudgetDocumentJSON)
	expectAnswer(t, env, "budget.scenarios", `[]`, `[]`)
	status, body := env.answer("transactions.filterOptions", `["2026-03"]`)
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"lines":[]`)) {
		t.Errorf("the menu answered %d %s", status, body)
	}
}

func TestOverASnapshotThatCannotBeReadTheBudgetReadsAreUnavailableAndAMissingOneStillSaves(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	unreadable := filepath.Join(t.TempDir(), "pluggy-snapshot.db")
	if err := os.WriteFile(unreadable, []byte("not a SQLite database, only text long enough to fill a header"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := newTestEnv(t, withLedger(unreadable, fixtureNow), withDatabase(scratch.Database))
	cookie := env.unlock()
	for method, arguments := range map[string]string{"budget.document": `["2026-03"]`, "budget.scenarios": `[]`, "transactions.filterOptions": `["2026-03"]`} {
		domain, name := splitMethod(t, method)
		expectError(t, method, env.call("/api/"+domain+"/"+name, arguments, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	}

	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "absent.db"), fixtureNow), withDatabase(scratch.Database))
	if response := missing.call("/api/budget/save", `["2026-03",`+b0+`]`, missing.unlock()); response.Code != http.StatusOK {
		t.Errorf("a save with no snapshot answered %d %s", response.Code, response.Body)
	}
}
