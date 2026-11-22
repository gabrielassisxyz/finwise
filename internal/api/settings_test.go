package api

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
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
			if list, ok := overview[field].([]any); !ok || len(list) != 0 {
				t.Errorf("%s: %s = %v, want []", name, field, overview[field])
			}
		}
	}
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
	scratch := appScratch(t)
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
