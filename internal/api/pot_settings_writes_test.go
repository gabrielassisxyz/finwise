package api

import (
	"encoding/json"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// potPlaced is the state of a case in which the person already chose a bucket
// for one pot.
func potPlaced(potID string, bucket authored.PotBucket) func(t *testing.T, scratch *authoredtest.Scratch) {
	return func(t *testing.T, scratch *authoredtest.Scratch) {
		t.Helper()
		authoredtest.SetPotBucket(t, scratch.Database, fixtureNow, potID, bucket)
	}
}

// budgetWithLink is the invented budget from 2026-02 with one of its lines
// already linked to a pot.
func budgetWithLink(line, potID string) func(t *testing.T, scratch *authoredtest.Scratch) {
	return func(t *testing.T, scratch *authoredtest.Scratch) {
		t.Helper()
		budgetState()(t, scratch)
		authoredtest.ToggleLink(t, scratch.Database, fixtureNow, budgetLineID(t, scratch.Database, "2026-03", line), potID)
	}
}

// potSettingsWriteCases are the pots' two writes: a bucket set, one set back to
// the bank's word (which clears the row), a link added, moved and removed, the
// fees line linked, and the three refusals the pots are the first to produce.
var potSettingsWriteCases = []writeCase{
	{method: "settings.setPotBucket", arguments: `["reserve:acc-pots/pot-1","Disponível"]`, expects: expectsRecorded},
	{method: "settings.setPotBucket", arguments: `["investment:inv-it-1","Reservado"]`, expects: expectsRecorded},
	{method: "settings.setPotBucket", arguments: `["investment:inv-it-1","Investido"]`, expects: expectsRecorded,
		state: potPlaced(potLCI, authored.BucketReserved)},
	{method: "settings.toggleLinkedLine", arguments: `["reserve:acc-pots/pot-1","Remédios"]`, expects: expectsRecorded, state: budgetState()},
	{method: "settings.toggleLinkedLine", arguments: `["reserve:acc-nu/pot-nu-1","Remédios"]`, expects: expectsRecorded,
		state: budgetWithLink("Remédios", potCaixinha)},
	{method: "settings.toggleLinkedLine", arguments: `["reserve:acc-pots/pot-1","Viagens"]`, expects: expectsRecorded,
		state: budgetWithLink("Viagens", potCaixinha)},
	{method: "settings.toggleLinkedLine", arguments: `["investment:inv-it-1","Tarifas e juros"]`, expects: expectsRecorded, state: budgetState()},
	{method: "settings.setPotBucket", arguments: `["slot:acc-nu/1","Disponível"]`, expects: expectsRefusal, reason: reasonUnstablePot},
	{method: "settings.toggleLinkedLine", arguments: `["slot:acc-nu/9","Remédios"]`, expects: expectsRefusal, reason: reasonUnstablePot,
		state: budgetState()},
	{method: "settings.setPotBucket", arguments: `["reserve:acc-nu/no-such-pot","Investido"]`, expects: expectsRefusal, reason: reasonNotFound},
	{method: "settings.toggleLinkedLine", arguments: `["acc-pots","Remédios"]`, expects: expectsRefusal, reason: reasonNotFound,
		state: budgetState()},
	{method: "settings.toggleLinkedLine", arguments: `["reserve:acc-pots/pot-1","Mercado"]`, expects: expectsRefusal,
		reason: reasonUnknownLine, state: startFromTheInventedState},
}

// withInventedBudget saves the invented budget from 2026-02, so that the budget
// in force for the current month has Remédios, Viagens and Salário.
func withInventedBudget(t *testing.T, env *settingsEnv) *settingsEnv {
	t.Helper()
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	return env
}

// potReads are the answers that place a pot or name its lines, as the server
// wrote them.
func potReads(t *testing.T, env *settingsEnv, potID string) map[string]string {
	t.Helper()
	reads := map[string]string{}
	for _, call := range []writeCall{
		{"savingsPots.list", `[{"includeClosed":false}]`}, {"savingsPots.list", includeClosed},
		{"savingsPots.detail", `["` + potID + `"]`}, {"settings.overview", `[]`},
		{"netWorth.overview", `[]`}, {"home.summary", `["2026-03"]`},
	} {
		status, body := env.answer(call.method, call.arguments)
		if status != http.StatusOK {
			t.Fatalf("%s %s answered %d", call.method, call.arguments, status)
		}
		reads[call.method+" "+call.arguments] = string(body)
	}
	return reads
}

func journalCounts(t *testing.T, scratch *authoredtest.Scratch) [2]int {
	t.Helper()
	return [2]int{len(scratch.Changes(t)), scratch.EffectCount(t)}
}

// expectPotRefused posts a write that must be refused for reason, and checks that
// no journaled table changed and the journal gained no row.
func expectPotRefused(t *testing.T, env *settingsEnv, method, arguments, reason string) {
	t.Helper()
	canonical, counts := env.scratch.Canonical(t), journalCounts(t, env.scratch)
	domain, name := splitMethod(t, method)
	expectRefused(t, method+" "+arguments, env.call("/api/"+domain+"/"+name, arguments, env.cookie), reason)
	if env.scratch.Canonical(t) != canonical || journalCounts(t, env.scratch) != counts {
		t.Errorf("%s %s was refused and changed the database", method, arguments)
	}
}

func settingsPotOf(t *testing.T, env *readEnv, potID string) map[string]any {
	t.Helper()
	for _, pot := range settingsOf(t, env)["pots"].([]any) {
		if pot.(map[string]any)["id"] == potID {
			return pot.(map[string]any)
		}
	}
	t.Fatalf("the settings list no pot %s", potID)
	return nil
}

func availableItemNames(worth map[string]any) []string {
	var names []string
	for _, item := range worth["buckets"].([]any)[0].(map[string]any)["items"].([]any) {
		names = append(names, item.(map[string]any)["name"].(string))
	}
	return names
}

var bankAccountItems = []string{"Banco não identificado · Conta Digital", "C6 · Conta C6", "Itaú · Conta Corrente", "Mercado Pago",
	"Nubank · Conta Nubank"}

// Over chartLedger, whose figures of today are the full ledger's and whose
// February point holds the Mercado Pago pot and the Itaú investment.
func TestABucketChangeMovesThePotInEveryReadAndItsUndoBringsEveryAnswerBack(t *testing.T) {
	cases := []struct {
		label, arguments, potID, bucket string
		groups                          []potGroup
		parts                           [3]int64
		february                        chartPoint
		available                       []string
	}{
		{"a reserve made available", `["reserve:acc-pots/pot-1","Disponível"]`, potCaixinha, "Disponível",
			[]potGroup{
				{"Reservado", 127580, []string{potReserva, potSlot}},
				{"Investido", 1000000, []string{potLCI, potCDB}},
				{"Disponível", 80000, []string{potCaixinha}},
			},
			[3]int64{285966, 127580, 1000000}, chartPoint{"2026-02", 76000, 100000, 600000},
			append(slices.Clone(bankAccountItems), "Caixinha da casa")},
		{"an investment reserved", `["investment:inv-it-1","Reservado"]`, potLCI, "Reservado",
			[]potGroup{
				{"Reservado", 907580, []string{potLCI, potCaixinha, potReserva, potSlot}},
				{"Investido", 300000, []string{potCDB}},
			},
			[3]int64{205966, 907580, 300000}, chartPoint{"2026-02", 36000, 740000, 0}, bankAccountItems},
	}
	for _, c := range cases {
		env := newSettingsEnv(t, chartLedger(t), fixtureNow)
		before, canonical := potReads(t, env, c.potID), env.scratch.Canonical(t)
		id := env.change("settings.setPotBucket", c.arguments)

		expectPotGroups(t, c.label, potGroupsOf(t, potListOf(t, env.readEnv, `[{"includeClosed":false}]`)), c.groups)
		worth := answerOf(t, env.readEnv, "netWorth.overview", `[]`)
		totals := bucketTotals(t, worth)
		if got := [3]int64{totals["Disponível"], totals["Reservado"], totals["Investido"]}; got != c.parts || cents(worth["total"].(float64)) != 1413546 {
			t.Errorf("%s: the net worth is %v in %v, want 14135.46 in %v", c.label, worth["total"], got, c.parts)
		}
		if names := availableItemNames(worth); !slices.Equal(names, c.available) {
			t.Errorf("%s: Disponível lists %v, want %v", c.label, names, c.available)
		}
		points := chartPointsOf(t, worth)
		if len(points) != 3 || points[1] != c.february || points[2] != (chartPoint{"2026-03", c.parts[0], c.parts[1], c.parts[2]}) {
			t.Errorf("%s: the chart is %+v, want February %+v and March at the parts %v", c.label, points, c.february, c.parts)
		}
		home := answerOf(t, env.readEnv, "home.summary", `["2026-03"]`)["netWorth"].(map[string]any)
		if got := [3]int64{cents(home["available"].(float64)), cents(home["reserved"].(float64)), cents(home["invested"].(float64))}; got != c.parts {
			t.Errorf("%s: the home shows %v, want %v", c.label, got, c.parts)
		}
		page := answerOf(t, env.readEnv, "savingsPots.detail", `["`+c.potID+`"]`)
		if page["bucket"] != c.bucket || settingsPotOf(t, env.readEnv, c.potID)["bucket"] != c.bucket {
			t.Errorf("%s: the pot's page says %v and the settings %v, want %s", c.label, page["bucket"],
				settingsPotOf(t, env.readEnv, c.potID)["bucket"], c.bucket)
		}

		env.undo(id)
		after := potReads(t, env, c.potID)
		for call, answer := range before {
			if after[call] != answer {
				t.Errorf("%s: after the undo %s answers\n%s\nwas\n%s", c.label, call, after[call], answer)
			}
		}
		if env.scratch.Canonical(t) != canonical {
			t.Errorf("%s: after the undo the journaled tables are not what they were", c.label)
		}
	}
}

func TestAClosedPotIsOneTheLedgerHasAndTakesBothChoices(t *testing.T) {
	env := withInventedBudget(t, newSettingsEnv(t, newFullLedger(t), fixtureNow))
	env.change("settings.setPotBucket", `["investment:inv-nu-2","Disponível"]`)
	groups := potGroupsOf(t, potListOf(t, env.readEnv, includeClosed))
	if last := groups[len(groups)-1]; last.bucket != "Disponível" || !slices.Equal(last.ids, []string{potCDBWithdrawn}) {
		t.Errorf("the list with the closed pots ends with %+v", last)
	}
	closedReserve := "reserve:" + accountNubank + "/" + potNubankOld
	env.change("settings.toggleLinkedLine", `["`+closedReserve+`","Remédios"]`)
	if lines := env.fieldOf("savingsPots.detail", `["`+closedReserve+`"]`, "linkedLines"); lines != `[{"name":"Remédios","share":null}]` {
		t.Errorf("the closed pot's page names the lines %s", lines)
	}
}

func TestSettingTheBucketAPotAlreadyHasRecordsAnEmptyChange(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	env.change("settings.setPotBucket", `["reserve:acc-pots/pot-1","Disponível"]`)
	canonical := env.scratch.Canonical(t)
	// The bucket stored, and for two pots with none stored the bank's own word.
	for _, arguments := range []string{
		`["reserve:acc-pots/pot-1","Disponível"]`, `["reserve:acc-nu/pot-nu-1","Reservado"]`, `["investment:inv-nu-1","Investido"]`,
	} {
		id := env.change("settings.setPotBucket", arguments)
		if effects := env.scratch.EffectsOf(t, changeNumber(t, id)); len(effects) != 0 {
			t.Errorf("%s: change %s recorded %d effects, want none", arguments, id, len(effects))
		}
		if env.scratch.Canonical(t) != canonical {
			t.Errorf("%s changed a journaled table", arguments)
		}
	}
}

func TestAPotIsOneThePotsReadListsAndASlotIsNeverOne(t *testing.T) {
	env := withInventedBudget(t, newSettingsEnv(t, newFullLedger(t), fixtureNow))
	refusals := map[string]string{
		"slot:acc-nu/1": "unstable_pot", "slot:acc-nu/9": "unstable_pot",
		"reserve:acc-nu/no-such-pot": "not_found", "investment:no-such": "not_found", accountPots: "not_found",
	}
	for potID, reason := range refusals {
		expectPotRefused(t, env, "settings.setPotBucket", `["`+potID+`","Disponível"]`, reason)
		expectPotRefused(t, env, "settings.toggleLinkedLine", `["`+potID+`","Remédios"]`, reason)
	}
	// The account's own page exists, and it is still not a pot.
	if page := answerOf(t, env.readEnv, "savingsPots.detail", `["`+accountNubank+`"]`); page["id"] != accountNubank {
		t.Fatalf("the Nubank account has no page of its own: %v", page)
	}
	expectPotRefused(t, env, "settings.setPotBucket", `["`+accountNubank+`","Disponível"]`, "not_found")
}

func TestAnInvestmentTheLedgerShowsBacksTheReservesIsNotAPot(t *testing.T) {
	env := newSettingsEnv(t, backedLedger(t, 400), fixtureNow)
	expectPotRefused(t, env, "settings.setPotBucket", `["investment:inv-a","Disponível"]`, "not_found")
	env.change("settings.setPotBucket", `["reserve:acc-nu/pot-a","Disponível"]`)
}

// Three versions: Mercado from 2026-02, renamed Feira from 2026-03, and a line
// added from 2026-04. The current month is 2026-03.
func TestALineIsNamedAsTheBudgetInForceForTheCurrentMonthNamesIt(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	database := env.scratch.Database
	authoredtest.SaveBudget(t, database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	savedFrom2026_03(t, database, func(document authored.Document) authored.Document {
		for _, group := range document {
			for index := range group.Lines {
				if group.Lines[index].Name == "Mercado" {
					group.Lines[index].Name = "Feira"
				}
			}
		}
		return document
	})
	view := database.View()
	march, err := view.BudgetOf(t.Context(), "2026-03")
	view.Close()
	if err != nil {
		t.Fatal(err)
	}
	april := authoredtest.DocumentOf(march)
	for index := range april {
		if april[index].Key == "res" {
			april[index].Lines = append(april[index].Lines, authoredtest.Line("Cinema", 5_000))
		}
	}
	authoredtest.SaveBudget(t, database, fixtureNow, "2026-04", april)
	for _, name := range []string{"Mercado", "Cinema", ""} {
		expectPotRefused(t, env, "settings.toggleLinkedLine", `["reserve:acc-pots/pot-1","`+name+`"]`, "unknown_line")
	}
	env.change("settings.toggleLinkedLine", `["reserve:acc-pots/pot-1","Feira"]`)
	env.change("settings.toggleLinkedLine", `["reserve:acc-pots/pot-1","Tarifas e juros"]`)
	if lines := settingsLinesOf(t, env.readEnv, potCaixinha); lines != `["Feira","Tarifas e juros"]` {
		t.Errorf("the pot's lines are %s", lines)
	}
}

func settingsLinesOf(t *testing.T, env *readEnv, potID string) string {
	t.Helper()
	return string(mustMarshal(t, settingsPotOf(t, env, potID)["linkedLines"]))
}

func TestTheFeesLineIsALineOnlyWhileTheCurrentMonthsBudgetHasOne(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	expectPotRefused(t, env, "settings.toggleLinkedLine", `["reserve:acc-pots/pot-1","Tarifas e juros"]`, "unknown_line")
	authoredtest.SaveBudget(t, env.scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	authoredtest.PutVersion(t, env.scratch.Database, fixtureNow, "2026-03")
	for _, name := range []string{"Tarifas e juros", "Salário"} {
		expectPotRefused(t, env, "settings.toggleLinkedLine", `["reserve:acc-pots/pot-1","`+name+`"]`, "unknown_line")
	}
}

// potLinkRows are the rows of pot_lines for a line, as the canonical dump holds
// them.
func potLinkRows(t *testing.T, scratch *authoredtest.Scratch, line authored.LineID) []map[string]any {
	t.Helper()
	var tables map[string][]map[string]any
	if err := json.Unmarshal([]byte(scratch.Canonical(t)), &tables); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	for _, row := range tables["pot_lines"] {
		if row["line_id"] == float64(line) {
			rows = append(rows, row)
		}
	}
	return rows
}

func TestAToggleLinksALineUnlinksItAndMovesItFromAnotherPotInOneChange(t *testing.T) {
	env := withInventedBudget(t, newSettingsEnv(t, newFullLedger(t), fixtureNow))
	lines := func(potID string) string {
		page := env.fieldOf("savingsPots.detail", `["`+potID+`"]`, "linkedLines")
		return settingsLinesOf(t, env.readEnv, potID) + " " + page
	}
	env.change("settings.toggleLinkedLine", `["reserve:acc-pots/pot-1","Remédios"]`)
	if got := lines(potCaixinha); got != `["Remédios"] [{"name":"Remédios","share":null}]` {
		t.Errorf("after the link the pot's lines are %s", got)
	}
	env.change("settings.toggleLinkedLine", `["reserve:acc-pots/pot-1","Remédios"]`)
	if got := lines(potCaixinha); got != `[] []` {
		t.Errorf("after the second toggle the pot's lines are %s", got)
	}

	env.change("settings.toggleLinkedLine", `["reserve:acc-pots/pot-1","Remédios"]`)
	changes := len(env.scratch.Changes(t))
	moved := env.change("settings.toggleLinkedLine", `["reserve:acc-nu/pot-nu-1","Remédios"]`)
	if got := lines(potCaixinha) + " " + lines(potReserva); got != `[] [] ["Remédios"] [{"name":"Remédios","share":null}]` {
		t.Errorf("after the move the two pots' lines are %s", got)
	}
	remedios := budgetLineID(t, env.scratch.Database, "2026-03", "Remédios")
	if rows := potLinkRows(t, env.scratch, remedios); len(env.scratch.Changes(t)) != changes+1 || len(rows) != 1 || rows[0]["pot_id"] != potReserva {
		t.Errorf("the move made %d changes and left the rows %v", len(env.scratch.Changes(t))-changes, rows)
	}
	env.undo(moved)
	if got := lines(potCaixinha) + " " + lines(potReserva); got != `["Remédios"] [{"name":"Remédios","share":null}] [] []` {
		t.Errorf("after the undo of the move the two pots' lines are %s", got)
	}
}

func TestThePotIsCheckedBeforeTheLine(t *testing.T) {
	env := withInventedBudget(t, newSettingsEnv(t, newFullLedger(t), fixtureNow))
	expectPotRefused(t, env, "settings.toggleLinkedLine", `["slot:acc-nu/1","Nenhuma"]`, "unstable_pot")
	expectPotRefused(t, env, "settings.toggleLinkedLine", `["investment:no-such","Nenhuma"]`, "not_found")
	expectPotRefused(t, env, "settings.toggleLinkedLine", `["reserve:acc-pots/pot-1","Nenhuma"]`, "unknown_line")
}

func TestABucketOutsideTheThreeIsABadRequestAboveAndBelowTheRouter(t *testing.T) {
	env := newSettingsEnv(t, newFullLedger(t), fixtureNow)
	env.expectRefusedAndNothingKept("settings.setPotBucket", `["reserve:acc-pots/pot-1","Outro"]`)
	err := writeBelowTheRouter(t, env.scratch.Database, setPotBucket, `["reserve:acc-pots/pot-1","Outro"]`)
	if kind, _ := classify(err); kind != kindBadRequest {
		t.Errorf("below the router the bucket Outro is answered %s", kind)
	}
}

func TestWithTheSnapshotMissingBothWritesAreUnavailableAndKeepNothing(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "pluggy-snapshot.db"), fixtureNow), withDatabase(scratch.Database))
	env := &settingsEnv{readEnv: &readEnv{testEnv: missing, cookie: missing.unlock()}, scratch: scratch}
	before := scratch.State(t)
	for method, arguments := range map[string]string{
		"settings.setPotBucket": `["reserve:acc-pots/pot-1","Disponível"]`, "settings.toggleLinkedLine": `["reserve:acc-pots/pot-1","Remédios"]`,
	} {
		domain, name := splitMethod(t, method)
		expectError(t, method, env.call("/api/"+domain+"/"+name, arguments, env.cookie), http.StatusServiceUnavailable, "ledger_unavailable")
	}
	if after := scratch.State(t); after != before {
		t.Errorf("a write with the snapshot missing changed the database:\n%s", differences(before, after))
	}
	// A slot's id is refused for what it is, before the snapshot is asked.
	expectPotRefused(t, env, "settings.setPotBucket", `["slot:acc-nu/1","Disponível"]`, "unstable_pot")
}

func TestABucketAndALinkAreAnsweredAfterTheDatabaseIsOpenedAgain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finwise.db")
	ledgerPath := newFullLedger(t).Path
	open := func() *authored.Database {
		database, err := authored.Open(t.Context(), path, authored.Options{Now: fixtureClock})
		if err != nil {
			t.Fatal(err)
		}
		return database
	}
	first := open()
	firstEnv := newTestEnv(t, withLedger(ledgerPath, fixtureNow), withDatabase(first))
	stores := &settingsEnv{readEnv: &readEnv{testEnv: firstEnv, cookie: firstEnv.unlock()}}
	stores.change("budget.save", `["2026-02",`+b0+`]`)
	stores.change("settings.setPotBucket", `["reserve:acc-pots/pot-1","Disponível"]`)
	stores.change("settings.toggleLinkedLine", `["reserve:acc-pots/pot-1","Remédios"]`)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second := open()
	t.Cleanup(func() { second.Close() })
	secondEnv := newTestEnv(t, withLedger(ledgerPath, fixtureNow), withDatabase(second))
	again := &readEnv{testEnv: secondEnv, cookie: secondEnv.unlock()}
	pot := settingsPotOf(t, again, potCaixinha)
	if got := map[string]any{"bucket": pot["bucket"], "lines": string(mustMarshal(t, pot["linkedLines"]))}; !maps.Equal(got,
		map[string]any{"bucket": "Disponível", "lines": `["Remédios"]`}) {
		t.Errorf("after the database was opened again the pot is %v", got)
	}
}
