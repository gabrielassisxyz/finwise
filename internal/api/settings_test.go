package api

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/token"
	"maps"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func init() {
	readCases = append(readCases, readCase{"settings.overview", `[]`})
}

// The defaults the settings are compared with are the store's: the app's
// database is the one place they are stated.
const (
	defaultConfidenceThreshold = authored.DefaultConfidenceThreshold
	defaultMinExamples         = authored.DefaultMinExamples
	defaultCardLabel           = authored.DefaultCardLabel
	defaultManualClosingDay    = authored.DefaultManualClosingDay
	defaultClosingMode         = string(authored.DefaultClosingMode)
)

func settingsOf(t *testing.T, env *readEnv) map[string]any {
	t.Helper()
	return answerOf(t, env, "settings.overview", `[]`)
}

func objectAt(t *testing.T, parent map[string]any, field string) map[string]any {
	t.Helper()
	child, ok := parent[field].(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %v", field, parent[field])
	}
	return child
}

func TestTheSettingsHoldTheDefaultsWhereThePersonHasChosenNothing(t *testing.T) {
	for _, name := range []string{"full", "empty"} {
		overview := settingsOf(t, newReadEnv(t, ledgerNamed(t, name)))
		categorization := objectAt(t, overview, "categorization")
		if categorization["threshold"] != float64(defaultConfidenceThreshold) || categorization["minExamples"] != float64(defaultMinExamples) {
			t.Errorf("%s: threshold %v, minimum of examples %v", name, categorization["threshold"], categorization["minExamples"])
		}
		if categorization["suggestedThreshold"] != nil {
			t.Errorf("%s: a threshold is suggested with nothing to judge it by: %v", name, categorization["suggestedThreshold"])
		}
		closing := objectAt(t, overview, "closing")
		if closing["mode"] != defaultClosingMode || closing["manualDay"] != float64(defaultManualClosingDay) {
			t.Errorf("%s: closing = %v", name, closing)
		}
		if overview["cardLabel"] != defaultCardLabel || defaultCardLabel != "Físico" {
			t.Errorf("%s: cardLabel = %v", name, overview["cardLabel"])
		}
		for _, field := range []string{"establishments", "people"} {
			list, ok := overview[field].([]any)
			if !ok || (len(list) == 0) != (name == "empty") {
				t.Errorf("%s: %s = %v, want [] over the empty ledger alone", name, field, overview[field])
			}
			for _, entry := range list {
				if problem := nothingSetIn(entry.(map[string]any)); problem != "" {
					t.Errorf("%s: %s: %s", name, field, problem)
				}
			}
		}
	}
}

// nothingSetIn is empty for a row of the establishments or the people that
// carries no value of the person's: no nickname, no default line, no fixed tag.
func nothingSetIn(row map[string]any) string {
	if row["nickname"] != nil {
		return "the nickname of " + row["key"].(string) + " is set"
	}
	if line, present := row["defaultLine"]; present && line != nil {
		return "the default line of " + row["key"].(string) + " is set"
	}
	if tags, present := row["fixedTags"]; present && len(tags.([]any)) != 0 {
		return "the fixed tags of " + row["key"].(string) + " are set"
	}
	return ""
}

func TestTheCalibrationBandsTileTheRangeOfTheThresholdAndAreAllUnjudged(t *testing.T) {
	overview := settingsOf(t, newReadEnv(t, newEmptyLedger(t)))
	bands := objectAt(t, overview, "categorization")["calibration"].([]any)
	wantRanges := [][2]float64{{50, 59}, {60, 69}, {70, 79}, {80, 89}, {90, 100}}
	if len(bands) != len(wantRanges) {
		t.Fatalf("%d bands, want %d", len(bands), len(wantRanges))
	}
	for index, band := range bands {
		band := band.(map[string]any)
		if band["from"] != wantRanges[index][0] || band["to"] != wantRanges[index][1] || band["correctionPercent"] != nil {
			t.Errorf("band %d = %v", index, band)
		}
	}
}

func TestTheSettingsReportTheLastCompleteRunAndNoBackup(t *testing.T) {
	f := newFullLedger(t)
	f.AddRun(ledgertest.Run{Outcome: "complete", FinishedAt: utc(2026, time.March, 19, 8, 30)})
	f.AddRun(ledgertest.Run{Outcome: "partial", FinishedAt: utc(2026, time.March, 20, 8, 30)})
	data := objectAt(t, settingsOf(t, newReadEnv(t, f)), "data")

	if data["lastSync"].(map[string]any)["day"] != "2026-03-19" || data["lastSync"].(map[string]any)["time"] != "05:30" {
		t.Errorf("lastSync = %v, want 2026-03-19 05:30, Brazil time", data["lastSync"])
	}
	if backup, present := data["lastBackup"]; !present || backup != nil {
		t.Errorf("lastBackup = %v, want null", backup)
	}
}

func lastBackupOver(t *testing.T, database *authored.Database) any {
	t.Helper()
	env := newTestEnv(t, withLedger(newFullLedger(t).Path, fixtureNow), func(config *Config) { config.Database = database })
	data := objectAt(t, answerOf(t, &readEnv{testEnv: env, cookie: env.unlock()}, "settings.overview", `[]`), "data")
	backup, present := data["lastBackup"]
	if !present {
		t.Fatal("the answer has no lastBackup")
	}
	return backup
}

// The backups are taken while the server holds the database, as the nightly
// job takes them; the first falls on the evening before in Brazil.
func TestTheSettingsShowTheLastBackupInBrazilTime(t *testing.T) {
	database, path := openAppDatabase(t)
	if backup := lastBackupOver(t, database); backup != nil {
		t.Errorf("before any backup lastBackup = %v, want null", backup)
	}
	for _, c := range []struct {
		at        time.Time
		day, time string
	}{
		{utc(2026, time.December, 1, 1, 30), "2026-11-30", "22:30"},
		{utc(2026, time.December, 2, 5, 15), "2026-12-02", "02:15"},
	} {
		options := authored.BackupOptions{Now: func() time.Time { return c.at }}
		if err := authored.Backup(t.Context(), path, filepath.Join(t.TempDir(), "copy.db"), options); err != nil {
			t.Fatalf("backup at %v: %v", c.at, err)
		}
		backup, ok := lastBackupOver(t, database).(map[string]any)
		if !ok || backup["day"] != c.day || backup["time"] != c.time || len(backup) != 2 {
			t.Errorf("after the backup at %v lastBackup = %v, want %s %s", c.at, backup, c.day, c.time)
		}
	}
}

func TestTheSettingsHaveNoLastSyncBeforeTheFirstCompleteRun(t *testing.T) {
	f := ledgertest.NewWithFirstRunOutcome(t, "failed")
	data := objectAt(t, settingsOf(t, newReadEnv(t, f)), "data")
	if sync, present := data["lastSync"]; !present || sync != nil {
		t.Errorf("lastSync = %v, want null", sync)
	}
}

func TestTheSettingsListOneConnectionPerItemOfTheLedger(t *testing.T) {
	data := objectAt(t, settingsOf(t, newReadEnv(t, newFullLedger(t))), "data")
	var listed []string
	for _, connection := range data["connections"].([]any) {
		connection := connection.(map[string]any)
		listed = append(listed, connection["bank"].(string)+"/"+connection["status"].(string))
	}
	want := []string{"Itaú/ok", "Nubank/ok", "Mercado Pago/ok", "C6/ok"}
	if len(listed) != len(want) {
		t.Fatalf("connections = %v, want %v", listed, want)
	}
	for index := range want {
		if listed[index] != want[index] {
			t.Errorf("connection %d = %s, want %s", index, listed[index], want[index])
		}
	}
}

func TestAConnectionWhoseConsentHasLapsedSaysSo(t *testing.T) {
	f := newFullLedger(t)
	lapsed := utc(2026, time.March, 10, 0, 0)
	fresh := utc(2026, time.March, 20, 5, 0)
	f.AddItem(ledgertest.Item{ID: "item-zz-lapsed", Connector: "Pluggy", Status: "UPDATED", ExecutionStatus: "SUCCESS",
		LastUpdated: fresh, NextAutoSync: fresh.Add(24 * time.Hour), ConsentExpiresAt: &lapsed})
	data := objectAt(t, settingsOf(t, newReadEnv(t, f)), "data")
	connections := data["connections"].([]any)

	last := connections[len(connections)-1].(map[string]any)
	if len(connections) != 5 || last["status"] != "consent-expired" || last["bank"] != neutralBankLabel {
		t.Errorf("connections = %v", connections)
	}
	for _, other := range connections[:len(connections)-1] {
		if other.(map[string]any)["status"] != "ok" {
			t.Errorf("a connection with a valid consent is %v", other)
		}
	}
}

func rateText(rate *float64) string {
	if rate == nil {
		return "none"
	}
	return strconv.FormatFloat(*rate, 'f', 2, 64)
}

// The pots are listed by the pots screen, and the settings must not pick a
// bucket a second time.
func TestEachPotOfTheSettingsHasTheBucketTheSavingsPotsListGivesIt(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	status, body := env.answer("savingsPots.list", `[{"includeClosed":false}]`)
	if status != http.StatusOK {
		t.Fatalf("savingsPots.list answered %d", status)
	}
	var groups []struct {
		Bucket string `json:"bucket"`
		Pots   []struct {
			ID         string   `json:"id"`
			Name       *string  `json:"name"`
			Bank       string   `json:"bank"`
			CDIPercent *float64 `json:"cdiPercent"`
		} `json:"pots"`
	}
	if err := json.Unmarshal(body, &groups); err != nil {
		t.Fatal(err)
	}
	type potFigures struct{ bucket, name, bank, cdi string }
	want := map[string]potFigures{}
	for _, group := range groups {
		for _, pot := range group.Pots {
			name := "<none>"
			if pot.Name != nil {
				name = *pot.Name
			}
			want[pot.ID] = potFigures{group.Bucket, name, pot.Bank, rateText(pot.CDIPercent)}
		}
	}

	pots := settingsOf(t, env)["pots"].([]any)
	if len(want) < 4 || len(pots) != len(want) {
		t.Fatalf("the list has %d pots and the settings %d (the control expects at least 4)", len(want), len(pots))
	}
	for _, pot := range pots {
		pot := pot.(map[string]any)
		name := "<none>"
		if pot["name"] != nil {
			name = pot["name"].(string)
		}
		var cdi *float64
		if rate, ok := pot["cdiPercent"].(float64); ok {
			cdi = &rate
		}
		got := potFigures{pot["bucket"].(string), name, pot["bank"].(string), rateText(cdi)}
		if listed, ok := want[pot["id"].(string)]; !ok || got != listed {
			t.Errorf("pot %v: settings %v, list %v", pot["id"], got, listed)
		}
		if links := pot["linkedLines"].([]any); len(links) != 0 {
			t.Errorf("pot %v has linked lines %v", pot["id"], links)
		}
	}
}

func TestTheSettingsLeaveOutAClosedPotAndKeepBothBucketsOfTheFullLedger(t *testing.T) {
	pots := settingsOf(t, newReadEnv(t, newFullLedger(t)))["pots"].([]any)
	buckets := map[string]bool{}
	for _, pot := range pots {
		buckets[pot.(map[string]any)["bucket"].(string)] = true
		if pot.(map[string]any)["id"] == "investment:"+investmentWithdrawn || pot.(map[string]any)["id"] == "reserve:"+accountNubank+"/"+potNubankOld {
			t.Errorf("closed pot %v is listed", pot.(map[string]any)["id"])
		}
	}
	if !buckets[bucketReserved] || !buckets[bucketInvested] {
		t.Errorf("buckets seen: %v", buckets)
	}
}

func TestTheSettingsShowTheCardAndTheDayTheBankClosesItsBill(t *testing.T) {
	cases := []struct {
		name    string
		env     *readEnv
		title   any
		bankDay any
	}{
		{"full", newReadEnv(t, newFullLedger(t)), "Platinum Visa · final 1234", float64(28)},
		{"c6 table", envAt(t, c6Ledger(t), 2026, time.March, 20), "C6 Carbon", float64(3)},
		{"no card", newReadEnv(t, newEmptyLedger(t)), nil, nil},
	}
	for _, c := range cases {
		overview := settingsOf(t, c.env)
		if overview["cardTitle"] != c.title || objectAt(t, overview, "closing")["bankDay"] != c.bankDay {
			t.Errorf("%s: cardTitle %v, bankDay %v", c.name, overview["cardTitle"], objectAt(t, overview, "closing")["bankDay"])
		}
	}
}

func TestTheSettingsTakeTheirCardTitleFromTheCardPage(t *testing.T) {
	env := newReadEnv(t, newFullLedger(t))
	if settingsOf(t, env)["cardTitle"] != answerOf(t, env, "card.overview", `[]`)["title"] {
		t.Error("the settings and the card page name the card differently")
	}
}

// chosenAnswer is what the overview answers for the five settings.
type chosenAnswer struct {
	threshold, minExamples, manualDay, cardLabel, mode any
}

func chosenAnswerOf(t *testing.T, overview map[string]any) chosenAnswer {
	t.Helper()
	categorization, closing := objectAt(t, overview, "categorization"), objectAt(t, overview, "closing")
	return chosenAnswer{categorization["threshold"], categorization["minExamples"], closing["manualDay"], overview["cardLabel"], closing["mode"]}
}

// overInventedState is a router over the full ledger and the invented authored
// state, which sets every setting to a value other than its default.
func overInventedState(t *testing.T) *readEnv {
	t.Helper()
	scratch := authoredtest.Open(t, fixtureClock)
	inventedAuthoredState(t, scratch)
	return newReadEnvOver(t, newFullLedger(t), scratch.Database)
}

func TestWithNothingStoredTheSettingsAnswerTheirDefaults(t *testing.T) {
	want := chosenAnswer{float64(85), float64(5), float64(28), "Físico", "auto"}
	for label, env := range map[string]*readEnv{
		"no database":                    newReadEnv(t, newFullLedger(t)),
		"a database with nothing stored": newReadEnvOver(t, newFullLedger(t), authoredtest.Fresh(t, fixtureClock)),
	} {
		if got := chosenAnswerOf(t, settingsOf(t, env)); got != want {
			t.Errorf("%s: the settings answer %+v, want %+v", label, got, want)
		}
	}
}

func TestTheSettingsAnswerWhatThePersonStored(t *testing.T) {
	overview := settingsOf(t, overInventedState(t))
	want := chosenAnswer{float64(70), float64(12), float64(10), inventedLabel, "manual"}
	if got := chosenAnswerOf(t, overview); got != want {
		t.Errorf("the settings answer %+v, want %+v", got, want)
	}
}

func TestTheCalibrationIsUnjudgedWhateverThePersonSet(t *testing.T) {
	categorization := objectAt(t, settingsOf(t, overInventedState(t)), "categorization")
	bands, err := json.Marshal(categorization["calibration"])
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"correctionPercent":null,"from":50,"to":59},{"correctionPercent":null,"from":60,"to":69},` +
		`{"correctionPercent":null,"from":70,"to":79},{"correctionPercent":null,"from":80,"to":89},` +
		`{"correctionPercent":null,"from":90,"to":100}]`
	if string(bands) != want {
		t.Errorf("calibration %s, want %s", bands, want)
	}
	if suggested, present := categorization["suggestedThreshold"]; !present || suggested != nil {
		t.Errorf("suggestedThreshold = %v, want null", suggested)
	}
}

// The store states the defaults once, so the settings screen and the rules that
// classify below the API cannot disagree on one. The minimum of examples and
// the manual day are numbers too common in the package to look for.
func TestThePackageStatesNoDefaultOfTheSettingsOfItsOwn(t *testing.T) {
	copies := map[string]bool{
		strconv.Quote(authored.DefaultCardLabel):                   true,
		strconv.Quote(string(authored.DefaultClosingMode)):         true,
		strconv.FormatInt(authored.DefaultConfidenceThreshold, 10): true,
	}
	for file, parsed := range sourceFilesOfThePackage(t) {
		ast.Inspect(parsed, func(node ast.Node) bool {
			if literal, ok := node.(*ast.BasicLit); ok && (literal.Kind == token.STRING || literal.Kind == token.INT) && copies[literal.Value] {
				t.Errorf("%s states %s, a default the store states", file, literal.Value)
			}
			return true
		})
	}
}

// The bank's day is shown beside the manual day as what the bank does, so no
// closing setting moves it.
func TestTheBankDayStaysTheBanksWhateverTheClosingSetting(t *testing.T) {
	fixtures := []struct {
		name    string
		ledger  func(t *testing.T) *ledgertest.Fixture
		bankDay string
	}{
		{"full", func(t *testing.T) *ledgertest.Fixture { return newFullLedger(t) }, "28"},
		{"c6 table", c6Ledger, "3"},
		{"no table", noTableLedger, "null"},
		{"no card", func(t *testing.T) *ledgertest.Fixture { return newEmptyLedger(t) }, "null"},
	}
	for _, fixture := range fixtures {
		env := newSettingsEnv(t, fixture.ledger(t), utc(2026, time.March, 20, 15, 0))
		for _, setting := range []string{``, `["auto",12]`, `["manual",10]`, `["manual",25]`, `["manual",1]`, `["auto",25]`} {
			if setting != `` {
				env.change("settings.setClosing", setting)
			}
			if got := env.overviewField("closing", "bankDay"); got != fixture.bankDay {
				t.Errorf("%s, closing %s: bankDay %s, want %s", fixture.name, setting, got, fixture.bankDay)
			}
		}
	}
}

func TestEachPotOfTheSettingsIsInTheBucketThePersonChoseAsTheListShowsIt(t *testing.T) {
	env := potsPlacedEnv(t, newFullLedger(t), map[string]authored.PotBucket{
		potCaixinha: authored.BucketAvailable, potLCI: authored.BucketReserved,
	})
	listed := map[string]any{}
	for id, pot := range potsOf(potListOf(t, env, `[{"includeClosed":false}]`)) {
		listed[id] = pot["bucket"]
	}
	settled := map[string]any{}
	for _, pot := range settingsOf(t, env)["pots"].([]any) {
		settled[pot.(map[string]any)["id"].(string)] = pot.(map[string]any)["bucket"]
	}
	if len(settled) != 5 || !maps.Equal(settled, listed) {
		t.Errorf("the settings place the pots %v, the list %v", settled, listed)
	}
	if settled[potCaixinha] != "Disponível" || settled[potLCI] != "Reservado" || settled[potCDB] != "Investido" {
		t.Errorf("the settings place the pots %v", settled)
	}
}

// budgetLineID is the id the budget in force for a month gives a line's name.
func budgetLineID(t *testing.T, database *authored.Database, month brday.Month, name string) authored.LineID {
	t.Helper()
	view := database.View()
	defer view.Close()
	budget, err := view.BudgetOf(t.Context(), month)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range budget.Lines {
		if line.Name == name {
			return line.ID
		}
	}
	t.Fatalf("the budget of %s has no line %q", month, name)
	return 0
}

// linkedLinesIn is what the settings and a pot's page answer for a pot's lines.
func linkedLinesIn(t *testing.T, env *readEnv, potID string) (settings, page string) {
	t.Helper()
	for _, pot := range settingsOf(t, env)["pots"].([]any) {
		if pot.(map[string]any)["id"] == potID {
			settings = string(mustMarshal(t, pot.(map[string]any)["linkedLines"]))
		}
	}
	return settings, string(mustMarshal(t, answerOf(t, env, "savingsPots.detail", `["`+potID+`"]`)["linkedLines"]))
}

func TestTheLinesOfAPotAreNamedInTheCurrentMonthsBudgetInTheBudgetsOrder(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	// Linked in an order that is neither the budget's nor the lines' ids.
	for _, line := range []authored.LineID{
		budgetLineID(t, scratch.Database, "2026-03", "Viagens"), budgetLineID(t, scratch.Database, "2026-03", "Salário"),
		feesLineID, budgetLineID(t, scratch.Database, "2026-03", "Remédios"),
	} {
		authoredtest.ToggleLink(t, scratch.Database, fixtureNow, line, potCaixinha)
	}
	env := newReadEnvOver(t, newFullLedger(t), scratch.Database)

	settings, page := linkedLinesIn(t, env, potCaixinha)
	if want := `["Salário","Remédios","Viagens","Tarifas e juros"]`; settings != want {
		t.Errorf("the settings name the lines %s, want %s", settings, want)
	}
	if want := `[{"name":"Salário","share":null},{"name":"Remédios","share":null},{"name":"Viagens","share":null},` +
		`{"name":"Tarifas e juros","share":null}]`; page != want {
		t.Errorf("the pot's page names the lines %s, want %s", page, want)
	}
	if settings, page := linkedLinesIn(t, env, potReserva); settings != `[]` || page != `[]` {
		t.Errorf("a pot with no line answers %s and %s", settings, page)
	}
}

// savedFrom2026_03 saves for 2026-03 the budget in force for it, changed by edit.
func savedFrom2026_03(t *testing.T, database *authored.Database, edit func(document authored.Document) authored.Document) {
	t.Helper()
	view := database.View()
	budget, err := view.BudgetOf(t.Context(), "2026-03")
	view.Close()
	if err != nil {
		t.Fatal(err)
	}
	authoredtest.SaveBudget(t, database, fixtureNow, "2026-03", edit(authoredtest.DocumentOf(budget)))
}

func TestARenamedLineShowsItsNewNameAndARemovedOneIsHiddenWhileItsLinkStays(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	remedios := budgetLineID(t, scratch.Database, "2026-03", "Remédios")
	authoredtest.ToggleLink(t, scratch.Database, fixtureNow, remedios, potCaixinha)
	env := newReadEnvOver(t, newFullLedger(t), scratch.Database)

	savedFrom2026_03(t, scratch.Database, func(document authored.Document) authored.Document {
		for _, group := range document {
			for index := range group.Lines {
				if group.Lines[index].Name == "Remédios" {
					group.Lines[index].Name = "Farmácia"
				}
			}
		}
		return document
	})
	if settings, page := linkedLinesIn(t, env, potCaixinha); settings != `["Farmácia"]` || page != `[{"name":"Farmácia","share":null}]` {
		t.Errorf("after the rename the pot's lines are %s and %s", settings, page)
	}

	savedFrom2026_03(t, scratch.Database, func(document authored.Document) authored.Document {
		for index := range document {
			if document[index].Key == "prov" {
				document[index].Lines = []authored.DocumentLine{}
			}
		}
		return document
	})
	if settings, page := linkedLinesIn(t, env, potCaixinha); settings != `[]` || page != `[]` {
		t.Errorf("after the line left the budget the pot's lines are %s and %s", settings, page)
	}
	view := scratch.Database.View()
	defer view.Close()
	if pot, linked, err := view.PotOfLine(t.Context(), remedios); err != nil || !linked || pot != potCaixinha {
		t.Errorf("the link's row is %q %v %v, want it kept on %s", pot, linked, err, potCaixinha)
	}
}

// The fees line has no row in any version, so its id is the one fact of the
// budget's migration the API states itself: it is a line that exists and that
// no document may carry, which is what the built-in line is.
func TestTheFeesLineIDIsTheBuiltInLineOfTheStore(t *testing.T) {
	database := authoredtest.Fresh(t, fixtureClock)
	authoredtest.SaveBudget(t, database, fixtureNow, "2026-02", authoredtest.InventedBudget())
	view := database.View()
	defer view.Close()
	carrying := func(id authored.LineID) error {
		text := id.String()
		return view.ValidateDocument(t.Context(), authored.Document{authoredtest.Group("fix",
			authored.DocumentLine{ID: &text, Name: "Outra", Cents: 100})})
	}
	var invalid *authored.InvalidBudgetError
	if err := carrying(feesLineID); !errors.As(err, &invalid) {
		t.Errorf("a document may carry the line %s: %v", feesLineID, err)
	}
	if err := carrying(budgetLineID(t, database, "2026-02", "Salário")); err != nil {
		t.Fatalf("a document may not carry a line of the budget either, so the check says nothing: %v", err)
	}
	view.Close()
	authoredtest.ToggleLink(t, database, fixtureNow, feesLineID, potCaixinha)
}
