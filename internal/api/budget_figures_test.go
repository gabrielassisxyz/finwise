package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/contract"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

const (
	keyOfSupermarket = "text:SUPERMERCADO A"
	keyOfWholesale   = "text:ATACADO C"
	keyOfFair        = "text:FEIRA B"
	keyOfDelivery    = "text:IFOOD"
)

type establishmentAnswer struct {
	Key    string      `json:"key"`
	Name   string      `json:"name"`
	Amount json.Number `json:"amount"`
	Count  int         `json:"count"`
}

func establishmentsAnswer(t *testing.T, env *readEnv, arguments string) []establishmentAnswer {
	t.Helper()
	status, body := env.answer("budget.establishments", arguments)
	if status != http.StatusOK {
		t.Fatalf("budget.establishments %s answered %d %s", arguments, status, body)
	}
	if problem := conformanceProblem("budget.establishments", body); problem != "" {
		t.Errorf("budget.establishments %s does not conform: %s", arguments, problem)
	}
	var answer []establishmentAnswer
	decodeKeepingNumbers(t, body, &answer)
	return answer
}

func sumOf(t *testing.T, establishments []establishmentAnswer) money.Cents {
	t.Helper()
	var sum money.Cents
	for _, establishment := range establishments {
		cents, err := ParseReais([]byte(establishment.Amount))
		if err != nil {
			t.Fatal(err)
		}
		sum += cents
	}
	return sum
}

func TestALinesEstablishmentsAreThePlacesThatMakeItUpBiggestFirst(t *testing.T) {
	example := newBudgetExample(t)
	cases := map[string][]establishmentAnswer{
		`["2026-03","Mercado"]`: {
			{keyOfSupermarket, "Supermercado A", "330.00", 3}, {keyOfWholesale, "Atacado C", "200.00", 1}, {keyOfFair, "Feira B", "70.00", 1},
		},
		`["2026-03","Delivery"]`: {{keyOfDelivery, "iFood", "230.00", 1}, {keyOfWholesale, "Atacado C", "60.00", 1}},
		`["2026-03","Lazer"]`:    {},
	}
	for arguments, want := range cases {
		if got := establishmentsAnswer(t, example.readEnv, arguments); !slices.Equal(got, want) {
			t.Errorf("%s: %+v, want %+v", arguments, got, want)
		}
	}
	market := example.view(`["2026-03"]`).line(t, "var", "Mercado")
	spent, err := ParseReais(market.Cells[1][len(`{"type":"money","amount":`) : len(market.Cells[1])-1])
	if err != nil {
		t.Fatal(err)
	}
	if sum := sumOf(t, establishmentsAnswer(t, example.readEnv, `["2026-03","Mercado"]`)); sum != spent || spent != 60_000 {
		t.Errorf("Mercado's establishments add up to %d, the view shows %d spent", sum, spent)
	}
}

func TestAnEstablishmentIsShownByTheNicknameThePersonGaveIt(t *testing.T) {
	example := newBudgetExample(t)
	authorOver(t, example.scratch, viewOf(t, example.ledger.Path), example.lines, laterCorrection, "scratch.nickname", func(a authoring) error {
		return authoredtest.WriteEstablishment(a.ctx, a.writes.Recorder, keyOfWholesale, "Atacadão do bairro", nil)
	})
	got := establishmentsAnswer(t, example.readEnv, `["2026-03","Delivery"]`)
	want := []establishmentAnswer{{keyOfDelivery, "iFood", "230.00", 1}, {keyOfWholesale, "Atacadão do bairro", "60.00", 1}}
	if !slices.Equal(got, want) {
		t.Errorf("with a nickname: %+v, want %+v", got, want)
	}
}

// The fees line is named in every month, a month with no line among them.
func TestTheFeesLinesEstablishmentsAnswerInEveryMonth(t *testing.T) {
	example := newBudgetExample(t)
	fees := establishmentsAnswer(t, example.readEnv, `["2026-03","Tarifas e juros"]`)
	if len(fees) != 1 || fees[0].Amount != "25.00" || fees[0].Count != 1 {
		t.Errorf("the fees line: %+v, want the fee's establishment with 25.00", fees)
	}
	noBudget := newReadEnvOver(t, example.ledger, authoredtest.Fresh(t, fixtureClock))
	if got := establishmentsAnswer(t, noBudget, `["2026-03","Tarifas e juros"]`); !slices.Equal(got, fees) {
		t.Errorf("with no budget the fees line answers %+v, want %+v", got, fees)
	}
}

func TestALineIsNamedAsTheBudgetOfTheMonthAskedNamesIt(t *testing.T) {
	example := newBudgetExample(t)
	renamed, err := draftArgument(json.RawMessage(example.draftOf("2026-03", withRename("Mercado", "Feira"))))
	if err != nil {
		t.Fatal(err)
	}
	authoredtest.SaveBudget(t, example.scratch.Database, fixtureNow, "2026-04", renamed)
	if got := establishmentsAnswer(t, example.readEnv, `["2026-03","Feira"]`); len(got) != 0 {
		t.Errorf("March has no line called Feira, and it answers %+v", got)
	}
	want := []establishmentAnswer{{keyOfFair, "Feira B", "40.00", 1}}
	if got := establishmentsAnswer(t, example.readEnv, `["2026-04","Feira"]`); !slices.Equal(got, want) {
		t.Errorf("April's Feira: %+v, want %+v", got, want)
	}
}

func TestAScenarioIsWeighedAgainstTheBudgetOfTheMonthAsked(t *testing.T) {
	example := newBudgetExample(t)
	draft := example.draftOf("2026-03", withAmount("Renda extra", "0"))
	evaluate := func(month string) *http.Response {
		return example.call("/api/budget/evaluateScenario", `["`+month+`",`+draft+`]`, example.cookie).Result()
	}
	want := `{"current":{"investment":4230.00,"rate":50},"scenario":{"investment":3730.00,"rate":47},"difference":-500.00,` +
		`"changed":[[false,true],[false,false],[false,false],[false],[false]]}`
	response := example.call("/api/budget/evaluateScenario", `["2026-03",`+draft+`]`, example.cookie)
	if response.Code != http.StatusOK || response.Body.String() != want {
		t.Errorf("evaluateScenario answered %d %s, want %s", response.Code, response.Body, want)
	}
	raised, err := draftArgument(json.RawMessage(example.draftOf("2026-03", withAmount("Aluguel", "3000.00"))))
	if err != nil {
		t.Fatal(err)
	}
	authoredtest.SaveBudget(t, example.scratch.Database, fixtureNow, "2026-04", raised)
	var april struct {
		Current struct {
			Investment json.Number `json:"investment"`
		} `json:"current"`
	}
	answer := evaluate("2026-04")
	if err := json.NewDecoder(answer.Body).Decode(&april); err != nil || april.Current.Investment != "3730.00" {
		t.Errorf("weighed against April, whose rent is higher, the current investment is %s, want 3730.00", april.Current.Investment)
	}
}

func TestAnEvaluationReadsNoLedgerAndNeedsTheDatabase(t *testing.T) {
	example := newBudgetExample(t)
	draft := example.draftOf("2026-03", withAmount("Renda extra", "0"))
	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), fixtureNow), withDatabase(example.scratch.Database))
	response := missing.call("/api/budget/evaluateScenario", `["2026-03",`+draft+`]`, missing.unlock())
	if response.Code != http.StatusOK || conformanceProblem("budget.evaluateScenario", response.Body.Bytes()) != "" {
		t.Errorf("with the snapshot missing the evaluation answered %d %s", response.Code, response.Body)
	}
	without := newTestEnv(t, withLedger(example.ledger.Path, fixtureNow))
	if reads, status := readsOfBody(without, "/api/budget/evaluateScenario", without.unlock()); reads != 0 || status != http.StatusForbidden {
		t.Errorf("without the database the body was read %d times and the status was %d, want 0 and 403", reads, status)
	}
}

func TestOverASnapshotThatCannotBeReadTheFiguresAreUnavailable(t *testing.T) {
	example := newBudgetExample(t)
	path := filepath.Join(t.TempDir(), "pluggy-snapshot.db")
	if err := os.WriteFile(path, []byte("not a SQLite database, only text long enough to fill a header"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := newTestEnv(t, withLedger(path, fixtureNow), withDatabase(example.scratch.Database))
	cookie := env.unlock()
	calls := map[string]string{
		"/api/budget/view":           `["2026-03"]`,
		"/api/budget/establishments": `["2026-03","Mercado"]`,
	}
	for route, arguments := range calls {
		expectError(t, route+" "+arguments, env.call(route, arguments, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	}
	for _, arguments := range []string{`["2026-03",` + example.draftOf("2026-03") + `]`, `["2026-03",[]]`} {
		expectError(t, "a draft", env.call("/api/budget/view", arguments, cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	}
	expectError(t, "a name the month lacks", env.call("/api/budget/establishments", `["2026-03","Lazer"]`, cookie),
		http.StatusServiceUnavailable, "ledger_unavailable")
}

func TestTheBudgetViewsArgumentsAreCheckedByTheSchemaBeforeItsHandler(t *testing.T) {
	example := newBudgetExample(t)
	calls := 0
	counting := func(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
		calls++
		return readBudgetView(ctx, args, views)
	}
	env := newTestEnv(t, func(config *Config) {
		config.Ledger, config.Now, config.Database = ledger.New(example.ledger.Path), fixtureClock, example.scratch.Database
	}, withRead("budget.view", counting))
	cookie := env.unlock()
	line := func(fields string) string { return `[{"key":"var","name":"Variáveis","lines":[{` + fields + `}]}]` }
	bySchema := []string{`0`, `"x"`, `[{"key":"xyz","name":"X","lines":[]}]`, line(`"id2":"6","name":"Mercado","amount":1`), line(`"name":"Mercado"`)}
	for _, draft := range bySchema {
		expectError(t, "the draft "+draft, env.call("/api/budget/view", `["2026-03",`+draft+`]`, cookie), http.StatusBadRequest, "bad_request")
	}
	if calls != 0 {
		t.Errorf("the handler was called %d times for drafts the schema refuses", calls)
	}
	byHandler := []string{
		`["2026-03",` + line(`"name":"Mercado","amount":-1`) + `]`,
		`["2026-03",` + line(`"name":"Mercado","amount":2000000000`) + `]`,
		`["2026-03",` + line(`"name":"Mercado","amount":1e400`) + `]`,
		`["2026-13"]`,
	}
	for _, arguments := range byHandler {
		expectError(t, arguments, env.call("/api/budget/view", arguments, cookie), http.StatusBadRequest, "bad_request")
	}
	if response := env.call("/api/budget/view", `["2026-03",`+line(`"name":"Mercado","amount":1`)+`]`, cookie); response.Code != http.StatusOK {
		t.Errorf("a draft the schema takes answered %d", response.Code)
	}
}

func TestOnlyTheBudgetViewAmongTheReadsIsCheckedByTheSchema(t *testing.T) {
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	var validated []string
	for _, method := range methods {
		if validatedBySchema(method) {
			validated = append(validated, method.Qualified())
		}
	}
	if !slices.Equal(validated, []string{"budget.view"}) {
		t.Errorf("the reads checked by the schema are %v, want budget.view alone", validated)
	}
	calls := 0
	env := newTestEnv(t, withRead("budget.establishments", func(context.Context, []json.RawMessage, Views) (any, error) {
		calls++
		return []any{}, nil
	}))
	if response := env.call("/api/budget/establishments", `["2026-03",5]`, env.unlock()); response.Code != http.StatusOK || calls != 1 {
		t.Errorf("another read with arguments its schema refuses answered %d after %d calls of its handler, want 200 after 1",
			response.Code, calls)
	}
}
