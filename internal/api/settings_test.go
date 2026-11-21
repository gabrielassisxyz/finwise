package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func init() {
	readCases = append(readCases, readCase{"settings.overview", `[]`})
}

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
