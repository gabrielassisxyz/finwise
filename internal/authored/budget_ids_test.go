package authored_test

import (
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

func (env *budgetEnv) idOf(month brday.Month, name string) authored.LineID {
	env.t.Helper()
	line, found := env.line(month, name)
	if !found {
		env.t.Fatalf("%s has no line %s", month, name)
	}
	return line.ID
}

func (env *budgetEnv) names(month brday.Month, group string) []string {
	var names []string
	for _, line := range env.budget(month).Lines {
		if line.Group == group {
			names = append(names, line.Name)
		}
	}
	return names
}

// renamed is the document in force for month with one line under another name.
func (env *budgetEnv) renamed(month brday.Month, from, to string) authored.Document {
	document := authoredtest.DocumentOf(env.budget(month))
	for _, group := range document {
		for i := range group.Lines {
			if group.Lines[i].Name == from {
				group.Lines[i].Name = to
			}
		}
	}
	return document
}

func TestEveryNewLineGetsADistinctIDAndTheFeesLineNone(t *testing.T) {
	env := newBudgetEnv(t)
	seen := map[authored.LineID]bool{}
	for _, line := range env.budget("2026-02").Lines {
		seen[line.ID] = true
	}
	if len(seen) != 8 {
		t.Fatalf("%d distinct ids for 8 lines", len(seen))
	}
	var fees int64
	if err := env.scratch.Inspect(t).QueryRow("SELECT id FROM lines WHERE builtin = 1").Scan(&fees); err != nil {
		t.Fatal(err)
	}
	if seen[authored.LineID(fees)] {
		t.Error("a line has the fees line's id")
	}
	again := env.budget("2026-02")
	for _, line := range again.Lines {
		if !seen[line.ID] {
			t.Errorf("a second read answers another id for %s", line.Name)
		}
	}
}

func TestARenameKeepsTheIDAndEarlierVersionsKeepTheirName(t *testing.T) {
	env := newBudgetEnv(t)
	env.closeMonths("2026-04")
	id := env.idOf("2026-02", "Mercado")
	env.save("2026-03", env.renamed("2026-03", "Mercado", "Feira"))
	for month, want := range map[brday.Month]string{"2026-02": "Mercado", "2026-03": "Feira", "2026-04": "Mercado", "2026-05": "Feira"} {
		if got := env.idOf(month, want); got != id {
			t.Errorf("%s: %s has id %d, want %d", month, want, got, id)
		}
		if _, found := env.line(month, map[string]string{"Mercado": "Feira", "Feira": "Mercado"}[want]); found {
			t.Errorf("%s answers both names", month)
		}
	}
}

func TestAReorderAndANewLineKeepEveryIDAndTheDraftsOrder(t *testing.T) {
	env := newBudgetEnv(t)
	before := map[string]authored.LineID{}
	var highest authored.LineID
	for _, line := range env.budget("2026-02").Lines {
		before[line.Name] = line.ID
		highest = max(highest, line.ID)
	}
	document := authoredtest.DocumentOf(env.budget("2026-02"))
	fixed := &document[slices.IndexFunc(document, func(g authored.DocumentGroup) bool { return g.Key == "fix" })]
	slices.Reverse(fixed.Lines)
	fixed.Lines = append(fixed.Lines, authoredtest.Line("Nova linha", 1_000))
	env.save("2026-03", document)
	if got := env.names("2026-03", "fix"); !slices.Equal(got, []string{"Saúde", "Contas da casa", "Nova linha"}) {
		t.Errorf("fix = %v", got)
	}
	for name, id := range before {
		if got := env.idOf("2026-03", name); got != id {
			t.Errorf("%s has id %d after the save, want %d", name, got, id)
		}
	}
	if added := env.idOf("2026-03", "Nova linha"); added <= highest {
		t.Errorf("the new line's id %d is not above every id minted before it (%d)", added, highest)
	}
}

func TestANewLineWrittenIntoTwoVersionsHasOneID(t *testing.T) {
	env := newBudgetEnv(t)
	env.closeMonths("2026-04")
	document := authoredtest.DocumentOf(env.budget("2026-02"))
	document[0].Lines = append(document[0].Lines, authoredtest.Line("Bônus", 100))
	env.save("2026-03", document)
	if march, may := env.idOf("2026-03", "Bônus"), env.idOf("2026-05", "Bônus"); march != may {
		t.Errorf("Bônus is %d in March and %d in May", march, may)
	}
	if _, found := env.line("2026-04", "Bônus"); found {
		t.Error("the closed month has the new line")
	}
}

func TestAnIDIsNeverReusedNorTakenBackByAnUndo(t *testing.T) {
	env := newBudgetEnv(t)
	delivery := env.idOf("2026-02", "Delivery")
	document := authoredtest.DocumentOf(env.budget("2026-02"))
	document[2].Lines = document[2].Lines[:1]
	env.save("2026-03", document)
	document = authoredtest.DocumentOf(env.budget("2026-03"))
	document[2].Lines = append(document[2].Lines, authoredtest.Line("Outra", 1))
	minting := env.save("2026-03", document)
	if got := env.idOf("2026-03", "Outra"); got == delivery {
		t.Errorf("a new line took the removed line's id %d", got)
	}
	minted := env.idOf("2026-03", "Outra")
	count := env.lineCount()
	if err := env.undo(minting); err != nil {
		t.Fatal(err)
	}
	if got := env.lineCount(); got != count {
		t.Errorf("lines holds %d rows after the undo, want the %d minted before it", got, count)
	}
	document[2].Lines[len(document[2].Lines)-1].ID = nil
	env.save("2026-03", document)
	if next := env.idOf("2026-03", "Outra"); next <= minted {
		t.Errorf("the next id %d is not above the one an undone save minted (%d)", next, minted)
	}
}

func TestALineMovedToAnotherGroupKeepsItsID(t *testing.T) {
	env := newBudgetEnv(t)
	id := env.idOf("2026-02", "Saúde")
	document := authoredtest.DocumentOf(env.budget("2026-02"))
	document[1].Lines = slices.DeleteFunc(document[1].Lines, func(l authored.DocumentLine) bool { return l.Name == "Saúde" })
	moved := authoredtest.DocumentOf(env.budget("2026-02"))[1].Lines[1]
	document[2].Lines = append(document[2].Lines, moved)
	env.save("2026-03", document)
	line, _ := env.line("2026-03", "Saúde")
	if line.Group != "var" || line.ID != id {
		t.Errorf("Saúde is %+v, want group var and id %d", line, id)
	}
}

func TestTwoLinesThatSwapNamesAreSavedAndUndone(t *testing.T) {
	env := newBudgetEnv(t)
	mercado, delivery := env.idOf("2026-02", "Mercado"), env.idOf("2026-02", "Delivery")
	before := env.scratch.Canonical(t)
	document := env.renamed("2026-02", "Mercado", "tmp")
	document = renameIn(document, "Delivery", "Mercado")
	document = renameIn(document, "tmp", "Delivery")
	change := env.save("2026-02", document)
	if env.idOf("2026-02", "Delivery") != mercado || env.idOf("2026-02", "Mercado") != delivery {
		t.Error("the names did not change hands with their ids kept")
	}
	if err := env.undo(change); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if env.scratch.Canonical(t) != before {
		t.Error("the undo of a swap left another state")
	}
}

func renameIn(document authored.Document, from, to string) authored.Document {
	for _, group := range document {
		for i := range group.Lines {
			if group.Lines[i].Name == from {
				group.Lines[i].Name = to
			}
		}
	}
	return document
}

func TestAnUndoThatWouldGiveBackANameAnotherLineHoldsIsRefusedAndChangesNothing(t *testing.T) {
	env := newBudgetEnv(t)
	first := env.save("2026-02", env.renamed("2026-02", "Mercado", "Feira"))
	document := authoredtest.DocumentOf(env.budget("2026-02"))
	document[2].Lines = append(document[2].Lines, authoredtest.Line("Mercado", 5))
	env.save("2026-02", document)
	before := env.scratch.Everything(t)
	err := env.undo(first)
	var refused *authored.StaleChangeError
	if err == nil || !errors.As(err, &refused) {
		t.Fatalf("undo = %v, want a stale change", err)
	}
	if env.scratch.Everything(t) != before {
		t.Error("a refused undo changed the database")
	}
}

func TestAFlagThatIsEmptyIsNoFlag(t *testing.T) {
	env := newBudgetEnv(t)
	document := authoredtest.DocumentOf(env.budget("2026-02"))
	document[0].Lines[0].Flag = ""
	env.save("2026-03", document)
	if line, _ := env.line("2026-03", "Salário"); line.Flag != "" {
		t.Errorf("Salário has the flag %q", line.Flag)
	}
	if line, _ := env.line("2026-03", "Renda extra"); line.Flag != "Não garantida" {
		t.Errorf("Renda extra has the flag %q", line.Flag)
	}
	var stored int
	if err := env.scratch.Inspect(t).QueryRow("SELECT count(*) FROM budget_version_lines WHERE flag = ''").Scan(&stored); err != nil || stored != 0 {
		t.Errorf("%d lines store an empty flag (%v)", stored, err)
	}
}

func TestADocumentWithoutAGroupSavesItEmpty(t *testing.T) {
	env := newBudgetEnv(t)
	document := authoredtest.DocumentOf(env.budget("2026-02"))
	env.save("2026-03", document[:4])
	if got := env.names("2026-03", "res"); len(got) != 0 {
		t.Errorf("res = %v, want none", got)
	}
	if got := env.names("2026-03", "rec"); len(got) != 2 {
		t.Errorf("rec = %v", got)
	}
}

// Every rule of the three writes that answers invalid_budget, over the writes
// that carry a document: a refusal changes nothing and adds nothing to the
// journal.
func TestADocumentThatIsNotValidIsRefusedByEveryWriteAndChangesNothing(t *testing.T) {
	mercado := func(env *budgetEnv) *string { id := env.idOf("2026-02", "Mercado").String(); return &id }
	cases := []struct {
		name  string
		build func(env *budgetEnv) authored.Document
	}{
		{"an empty name", func(*budgetEnv) authored.Document {
			return authored.Document{authoredtest.Group("fix", authoredtest.Line("", 1))}
		}},
		{"a name of three spaces", func(*budgetEnv) authored.Document {
			return authored.Document{authoredtest.Group("fix", authoredtest.Line("   ", 1))}
		}},
		{"a name twice in one group", func(*budgetEnv) authored.Document {
			return authored.Document{authoredtest.Group("var", authoredtest.Line("Mercado", 1), authoredtest.Line("Mercado", 2))}
		}},
		{"a name in two groups", func(*budgetEnv) authored.Document {
			return authored.Document{authoredtest.Group("var", authoredtest.Line("Mercado", 1)), authoredtest.Group("fix", authoredtest.Line("Mercado", 2))}
		}},
		{"the fees line's name", func(*budgetEnv) authored.Document {
			return authored.Document{authoredtest.Group("fix", authoredtest.Line("Tarifas e juros", 1))}
		}},
		{"an id that names no line", func(*budgetEnv) authored.Document {
			id := "999999"
			return authored.Document{authoredtest.Group("fix", authored.DocumentLine{ID: &id, Name: "X"})}
		}},
		{"an id that is not a number", func(*budgetEnv) authored.Document {
			id := "abc"
			return authored.Document{authoredtest.Group("fix", authored.DocumentLine{ID: &id, Name: "X"})}
		}},
		{"an empty id", func(*budgetEnv) authored.Document {
			id := ""
			return authored.Document{authoredtest.Group("fix", authored.DocumentLine{ID: &id, Name: "X"})}
		}},
		{"an id spelled with a leading zero", func(env *budgetEnv) authored.Document {
			id := "0" + *mercado(env)
			return authored.Document{authoredtest.Group("fix", authored.DocumentLine{ID: &id, Name: "X"})}
		}},
		{"the same id twice", func(env *budgetEnv) authored.Document {
			return authored.Document{authoredtest.Group("fix",
				authored.DocumentLine{ID: mercado(env), Name: "X"}, authored.DocumentLine{ID: mercado(env), Name: "Y"})}
		}},
		{"the fees line's id", func(*budgetEnv) authored.Document {
			id := "1"
			return authored.Document{authoredtest.Group("fix", authored.DocumentLine{ID: &id, Name: "X"})}
		}},
		{"a group key outside the five", func(*budgetEnv) authored.Document {
			return authored.Document{authoredtest.Group("xyz", authoredtest.Line("X", 1))}
		}},
		{"a group key twice", func(*budgetEnv) authored.Document {
			return authored.Document{authoredtest.Group("var", authoredtest.Line("X", 1)), authoredtest.Group("var", authoredtest.Line("Y", 1))}
		}},
	}
	writes := map[string]func(*budgetEnv, authored.Document) error{
		"save": func(env *budgetEnv, document authored.Document) error { return env.try("2026-03", document) },
		"saveScenario": func(env *budgetEnv, document authored.Document) error {
			return env.change(func(recorder *authored.Recorder) error {
				return recorder.SaveScenario(env.t.Context(), "S", document)
			})
		},
	}
	for _, c := range cases {
		for name, write := range writes {
			t.Run(c.name+" by "+name, func(t *testing.T) {
				env := newBudgetEnv(t)
				before, journal := env.scratch.Everything(t), env.scratch.Journal(t)
				err := write(env, c.build(env))
				var invalid *authored.InvalidBudgetError
				if !errors.As(err, &invalid) {
					t.Fatalf("err = %v, want an invalid budget", err)
				}
				if env.scratch.Everything(t) != before || env.scratch.Journal(t) != journal {
					t.Error("a refused write changed the database or the journal")
				}
			})
		}
	}
}

func TestAScenarioIsKeptInTheOrderItWasSavedAndReplacedAtTheEnd(t *testing.T) {
	env := newBudgetEnv(t)
	save := func(name string, health int64) authored.ChangeID {
		return authoredtest.SaveScenario(t, env.scratch.Database, env.at(), name, env.withHealth("2026-02", money.Cents(health)))
	}
	save("A", 100)
	save("B", 200)
	save("C", 300)
	before := env.scratch.Canonical(t)
	replacing := save("A", 400)
	got := env.scenarios()
	if names := scenarioNames(got); !slices.Equal(names, []string{"B", "C", "A"}) {
		t.Errorf("order = %v, want B C A", names)
	}
	if health(got[2]) != 400 {
		t.Errorf("A holds %d, want the second document", health(got[2]))
	}
	if err := env.undo(replacing); err != nil {
		t.Fatal(err)
	}
	got = env.scenarios()
	if names := scenarioNames(got); !slices.Equal(names, []string{"A", "B", "C"}) || health(got[0]) != 100 {
		t.Errorf("after the undo: %v, A holds %d; want A B C with A's first document", names, health(got[0]))
	}
	if env.scratch.Canonical(t) != before {
		t.Error("the undo left another state")
	}
}

func (env *budgetEnv) scenarios() []authored.Scenario {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	scenarios, err := view.Scenarios(env.t.Context(), "2026-03")
	if err != nil {
		env.t.Fatal(err)
	}
	return scenarios
}

func scenarioNames(scenarios []authored.Scenario) []string {
	names := make([]string, len(scenarios))
	for i, scenario := range scenarios {
		names[i] = scenario.Name
	}
	return names
}

func health(scenario authored.Scenario) int64 {
	for _, group := range scenario.Document {
		for _, line := range group.Lines {
			if line.Name == "Saúde" {
				return int64(line.Cents)
			}
		}
	}
	return -1
}

func TestAScenarioMintsNothingAndAnIDlessLineStaysWithoutOne(t *testing.T) {
	env := newBudgetEnv(t)
	count := env.lineCount()
	document := authored.Document{authoredtest.Group("fix", authoredtest.Line("Nova", 5))}
	change := authoredtest.SaveScenario(t, env.scratch.Database, env.at(), "S", document)
	if env.lineCount() != count {
		t.Error("saving a scenario minted an id")
	}
	if line := env.scenarios()[0].Document[1].Lines[0]; line.ID != nil || line.Name != "Nova" {
		t.Errorf("the line is %+v, want one without an id", line)
	}
	for _, effect := range env.scratch.EffectsOf(t, int64(change)) {
		if effect.Table != "scenarios" {
			t.Errorf("a scenario's change has an effect on %s", effect.Table)
		}
	}
}

func TestAScenarioIsAnsweredWithTheCurrentNamesOfItsLinesByID(t *testing.T) {
	env := newBudgetEnv(t)
	authoredtest.SaveScenario(t, env.scratch.Database, env.at(), "S", authoredtest.DocumentOf(env.budget("2026-02")))
	env.save("2026-03", env.renamed("2026-03", "Mercado", "Feira"))
	var names []string
	for _, line := range env.scenarios()[0].Document[2].Lines {
		names = append(names, line.Name)
	}
	if !slices.Equal(names, []string{"Feira", "Delivery"}) {
		t.Errorf("var = %v, want the current name of Mercado", names)
	}
}

func TestAScenarioLineWhoseIDTheMonthLacksKeepsTheNameItWasSavedWith(t *testing.T) {
	env := newBudgetEnv(t)
	authoredtest.SaveScenario(t, env.scratch.Database, env.at(), "S", authoredtest.DocumentOf(env.budget("2026-02")))
	document := authoredtest.DocumentOf(env.budget("2026-02"))
	document[2].Lines = document[2].Lines[:1]
	env.save("2026-03", document)
	var names []string
	for _, line := range env.scenarios()[0].Document[2].Lines {
		names = append(names, line.Name)
		if line.ID == nil {
			t.Error("the id was dropped")
		}
	}
	if !slices.Equal(names, []string{"Mercado", "Delivery"}) {
		t.Errorf("var = %v", names)
	}
}

func TestEveryIDIsParsedAsItIsWritten(t *testing.T) {
	for _, text := range []string{"7", "123456"} {
		id, ok := authored.ParseLineID(text)
		if !ok || id.String() != text {
			t.Errorf("%q parsed as %d, %v", text, id, ok)
		}
	}
	for _, text := range []string{"", "0", "-1", "07", "+7", "abc", "1.0", " 1"} {
		if _, ok := authored.ParseLineID(text); ok {
			t.Errorf("%q was accepted as an id", text)
		}
	}
	if _, ok := authored.ParseLineID(strconv.FormatInt(1<<62, 10) + "0"); ok {
		t.Error("an id past int64 was accepted")
	}
}
