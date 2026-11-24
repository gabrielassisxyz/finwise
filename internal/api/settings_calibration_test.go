package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// calibrationModel is the model the servers of these tests have configured;
// anotherModel is one an earlier configuration used.
const (
	calibrationModel = "modelo-teste"
	anotherModel     = "outro-modelo"
)

func withModelName(name string) envOption {
	return func(config *Config) { config.ModelName = name }
}

// answeredSuggestion is a stored model suggestion and, when answer is set, the
// person's answer to it in the queue, in a change that undone takes back.
type answeredSuggestion struct {
	model      string
	confidence int64
	answer     authored.ModelAnswer
	undone     bool
}

// answered is count suggestions of the model at one confidence, corrected of
// them corrected and the rest accepted.
func answered(model string, confidence int64, count, corrected int) []answeredSuggestion {
	suggestions := make([]answeredSuggestion, count)
	for index := range suggestions {
		answer := authored.ModelAnswerAccepted
		if index < corrected {
			answer = authored.ModelAnswerCorrected
		}
		suggestions[index] = answeredSuggestion{model: model, confidence: confidence, answer: answer}
	}
	return suggestions
}

func concat(groups ...[]answeredSuggestion) []answeredSuggestion {
	var all []answeredSuggestion
	for _, group := range groups {
		all = append(all, group...)
	}
	return all
}

// answerSuggestions stores each suggestion on a transaction of its own and
// answers it as the queue does, in one call for the whole database: the
// transactions are named by their place in the list.
func answerSuggestions(t *testing.T, database *authored.Database, suggestions []answeredSuggestion) {
	t.Helper()
	view := database.View()
	lines, err := linesOfMonth(t.Context(), view, "2026-03")
	view.Close()
	if err != nil {
		t.Fatal(err)
	}
	line, ok := lines.idOf("Feira")
	if !ok {
		t.Fatal("the budget of March has no Feira")
	}
	keys := make([]authored.TxKey, len(suggestions))
	if err := database.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorRule, Method: "ingest.run", At: fixtureNow}, func(recorder *authored.Recorder) error {
		for index := range suggestions {
			key, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen("answered-"+strconv.Itoa(index), "2026-03-10", -1000, "text:LOJA"))
			if err != nil {
				return err
			}
			keys[index] = key
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for index, suggestion := range suggestions {
		confidence, reason := suggestion.confidence, "Motivo"
		if err := database.RecordModelSuggestions(t.Context(), suggestion.model, fixtureNow, []authored.ModelSuggestionInput{
			{Key: keys[index], Line: &line, Confidence: &confidence, Justification: &reason},
		}); err != nil {
			t.Fatal(err)
		}
		if suggestion.answer == "" {
			continue
		}
		at := fixtureNow.Add(time.Duration(index+1) * time.Minute)
		var change authored.ChangeID
		if err := database.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorPerson, Method: "review.acceptPending", At: at}, func(recorder *authored.Recorder) error {
			if err := recorder.SettleLine(t.Context(), keys[index], line, authored.AuthorPerson); err != nil {
				return err
			}
			if err := recorder.RecordModelAnswer(t.Context(), keys[index], suggestion.answer); err != nil {
				return err
			}
			var err error
			change, err = recorder.ID(t.Context())
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if suggestion.undone {
			if err := database.Undo(t.Context(), change, at.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// answeredSuggestionsOverTheThirdState is the case of the settings with a model
// that has been asked: a band of 25 answers of which 5 were corrected, one of
// 40 of which 1 was, and bands short of 20 or with none, the last short by one.
func answeredSuggestionsOverTheThirdState(t *testing.T, scratch *authoredtest.Scratch, _ *ledgertest.Fixture) {
	t.Helper()
	answerSuggestions(t, scratch.Database, concat(
		answered(calibrationModel, 85, 25, 5),
		answered(calibrationModel, 65, 40, 1),
		answered(calibrationModel, 95, 19, 7),
		answered(anotherModel, 55, 30, 30),
	))
	if err := scratch.Database.RecordModelRun(t.Context(), utc(2026, time.March, 20, 6, 12), true); err != nil {
		t.Fatal(err)
	}
}

// settingsWith is the overview of a server that has the model named configured
// ("" for none), over the invented state with the suggestions answered on top.
func settingsWith(t *testing.T, model string, over func(*testing.T, *authoredtest.Scratch)) map[string]any {
	t.Helper()
	scratch, full := newAuthoredStores(t)
	if over != nil {
		over(t, scratch)
	}
	env := newTestEnv(t, withLedger(full.Path, fixtureNow), withDatabase(scratch.Database), withModelName(model))
	return settingsOf(t, &readEnv{testEnv: env, cookie: env.unlock()})
}

func answering(suggestions []answeredSuggestion) func(*testing.T, *authoredtest.Scratch) {
	return func(t *testing.T, scratch *authoredtest.Scratch) { answerSuggestions(t, scratch.Database, suggestions) }
}

// percentsOf is the five bands as the screen reads them: a whole percent, or
// the string "-" for a band with no rate; it also checks the bands' ranges and
// that no threshold is suggested.
func percentsOf(t *testing.T, overview map[string]any) [5]string {
	t.Helper()
	categorization := objectAt(t, overview, "categorization")
	if suggested, present := categorization["suggestedThreshold"]; !present || suggested != nil {
		t.Errorf("suggestedThreshold = %v, want null", suggested)
	}
	bands, ok := categorization["calibration"].([]any)
	if !ok || len(bands) != 5 {
		t.Fatalf("calibration = %v", categorization["calibration"])
	}
	var percents [5]string
	for index, entry := range bands {
		band := entry.(map[string]any)
		wantFrom := float64(calibrationFirstBand + index*calibrationBandWidth)
		if band["from"] != wantFrom {
			t.Errorf("band %d starts at %v, want %v", index, band["from"], wantFrom)
		}
		percents[index] = "-"
		if percent, judged := band["correctionPercent"].(float64); judged {
			percents[index] = strconv.Itoa(int(percent))
		}
	}
	return percents
}

func TestABandShowsTheCorrectedShareFromTwentyAnswersOnAndIsUnjudgedBefore(t *testing.T) {
	suggestions := concat(
		answered(calibrationModel, 85, 25, 5),
		answered(calibrationModel, 75, 19, 19),
		answered(calibrationModel, 65, 20, 4),
	)
	got := percentsOf(t, settingsWith(t, calibrationModel, answering(suggestions)))
	want := [5]string{"-", "20", "-", "20", "-"}
	if got != want {
		t.Errorf("the bands answer %v, want %v: 25 answers of which 5 corrected is 20, 19 answers are none, exactly 20 answers are enough, no answer is none", got, want)
	}
}

func TestTheShareIsRoundedHalfUpInIntegers(t *testing.T) {
	suggestions := concat(
		answered(calibrationModel, 55, 40, 1),
		answered(calibrationModel, 65, 30, 1),
		answered(calibrationModel, 75, 40, 3),
		answered(calibrationModel, 85, 30, 0),
		answered(calibrationModel, 95, 30, 30),
	)
	got := percentsOf(t, settingsWith(t, calibrationModel, answering(suggestions)))
	want := [5]string{"3", "3", "8", "0", "100"}
	if got != want {
		t.Errorf("the bands answer %v, want %v: 1 of 40 is 2.5, 1 of 30 is 3.3, 3 of 40 is 7.5, none corrected is a true 0", got, want)
	}
}

func TestOnlyTheAnswersOfTheConfiguredModelThatStandChangeABand(t *testing.T) {
	base := answered(calibrationModel, 85, 25, 5)
	noise := concat(
		answered(anotherModel, 85, 40, 40),
		[]answeredSuggestion{{model: calibrationModel, confidence: 85}},
		[]answeredSuggestion{{model: calibrationModel, confidence: 85, answer: authored.ModelAnswerCorrected, undone: true}},
		answered(calibrationModel, 45, 40, 40),
		answered(calibrationModel, 49, 40, 40),
	)
	want := percentsOf(t, settingsWith(t, calibrationModel, answering(base)))
	if want != [5]string{"-", "-", "-", "20", "-"} {
		t.Fatalf("the bands of the base answer %v", want)
	}
	if got := percentsOf(t, settingsWith(t, calibrationModel, answering(concat(base, noise)))); got != want {
		t.Errorf("the bands moved from %v to %v with another model's answers, an unanswered suggestion, an undone answer and a confidence under 50", want, got)
	}
}

func TestACorrectedAnswerThatWasUndoneStopsCounting(t *testing.T) {
	stand := concat(answered(calibrationModel, 85, 20, 0), answered(calibrationModel, 85, 5, 5))
	taken := concat(answered(calibrationModel, 85, 20, 0), []answeredSuggestion{{model: calibrationModel, confidence: 85, answer: authored.ModelAnswerCorrected, undone: true}})
	if got := percentsOf(t, settingsWith(t, calibrationModel, answering(stand))); got[3] != "20" {
		t.Errorf("with 5 corrected of 25 the band answers %v", got)
	}
	if got := percentsOf(t, settingsWith(t, calibrationModel, answering(taken))); got[3] != "0" {
		t.Errorf("a correction taken back was counted: the band answers %v, want 0 of 20", got)
	}
}

func TestWithNoModelConfiguredEveryBandIsUnjudgedAndThereIsNoModel(t *testing.T) {
	overview := settingsWith(t, "", answering(answered(calibrationModel, 85, 25, 5)))
	if got := percentsOf(t, overview); got != [5]string{"-", "-", "-", "-", "-"} {
		t.Errorf("the bands answer %v with no model configured", got)
	}
	if model, present := objectAt(t, overview, "categorization")["model"]; !present || model != nil {
		t.Errorf("model = %v, want null", model)
	}
}

func TestTheNameOfAnotherModelStartsTheChartFromZero(t *testing.T) {
	suggestions := answered(calibrationModel, 85, 25, 5)
	if got := percentsOf(t, settingsWith(t, anotherModel, answering(suggestions))); got != [5]string{"-", "-", "-", "-", "-"} {
		t.Errorf("a model configured now with another name was judged by %s's answers: %v", calibrationModel, got)
	}
	if got := percentsOf(t, settingsWith(t, "Modelo-Teste", answering(suggestions))); got != [5]string{"-", "-", "-", "-", "-"} {
		t.Errorf("a name that differs in case was taken for the same model: %v", got)
	}
}

// What the approved proposals of an agent leave in the journal: changes by the
// author model, on transactions no suggestion of the model covered, each
// followed by the person's. Thirty of them, so that a count of the journal would
// clear the threshold of 20 where the model's own suggestions do not.
func TestTheJournalsModelAuthorIsNotTheModelsSuggestions(t *testing.T) {
	proposals := func(t *testing.T, scratch *authoredtest.Scratch) {
		answerSuggestions(t, scratch.Database, answered(calibrationModel, 85, 25, 5))
		view := scratch.Database.View()
		lines, err := linesOfMonth(t.Context(), view, "2026-03")
		view.Close()
		if err != nil {
			t.Fatal(err)
		}
		line, _ := lines.idOf("Feira")
		for index := range 30 {
			var key authored.TxKey
			at := fixtureNow.Add(time.Hour * time.Duration(index+1))
			if err := scratch.Database.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorRule, Method: "ingest.run", At: at}, func(recorder *authored.Recorder) error {
				var err error
				key, err = recorder.IndexTransaction(t.Context(), authoredtest.Seen("proposed-"+strconv.Itoa(index), "2026-03-10", -1000, "text:LOJA"))
				return err
			}); err != nil {
				t.Fatal(err)
			}
			for _, author := range []authored.Author{authored.AuthorModel, authored.AuthorPerson} {
				if err := scratch.Database.Change(t.Context(), authored.ChangeFacts{Author: author, Method: "test.classify", At: at.Add(time.Minute)}, func(recorder *authored.Recorder) error {
					return recorder.SettleLine(t.Context(), key, line, author)
				}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	want := percentsOf(t, settingsWith(t, calibrationModel, answering(answered(calibrationModel, 85, 25, 5))))
	if got := percentsOf(t, settingsWith(t, calibrationModel, proposals)); got != want {
		t.Errorf("the bands moved from %v to %v with changes by the author model and no suggestion", want, got)
	}
}

func TestTheModelIsNamedAsConfiguredAndHasNoRunBeforeTheFirst(t *testing.T) {
	model := objectAt(t, objectAt(t, settingsWith(t, "Modelo Teste:v2", nil), "categorization"), "model")
	if model["name"] != "Modelo Teste:v2" || model["lastRun"] != nil {
		t.Errorf("model = %v, want its name as configured and no run", model)
	}
}

func TestTheModelsLastRunIsGivenInBrazilTimeWhetherItWorkedOrNot(t *testing.T) {
	cases := []struct {
		name   string
		at     time.Time
		worked bool
		want   map[string]any
	}{
		{"worked", utc(2026, time.November, 23, 6, 12), true, map[string]any{"day": "2026-11-23", "time": "03:12", "worked": true}},
		{"failed", utc(2026, time.November, 23, 6, 12), false, map[string]any{"day": "2026-11-23", "time": "03:12", "worked": false}},
		{"across midnight", utc(2026, time.November, 24, 2, 5), true, map[string]any{"day": "2026-11-23", "time": "23:05", "worked": true}},
	}
	for _, c := range cases {
		record := func(t *testing.T, scratch *authoredtest.Scratch) {
			if err := scratch.Database.RecordModelRun(t.Context(), c.at, c.worked); err != nil {
				t.Fatal(err)
			}
		}
		model := objectAt(t, objectAt(t, settingsWith(t, calibrationModel, record), "categorization"), "model")
		got, ok := model["lastRun"].(map[string]any)
		if !ok || len(got) != len(c.want) {
			t.Errorf("%s: lastRun = %v, want %v", c.name, model["lastRun"], c.want)
			continue
		}
		for field, want := range c.want {
			if got[field] != want {
				t.Errorf("%s: lastRun.%s = %v, want %v", c.name, field, got[field], want)
			}
		}
	}
}

func TestARunIsNotShownWithNoModelConfigured(t *testing.T) {
	record := func(t *testing.T, scratch *authoredtest.Scratch) {
		if err := scratch.Database.RecordModelRun(t.Context(), utc(2026, time.November, 23, 6, 12), false); err != nil {
			t.Fatal(err)
		}
	}
	if model := objectAt(t, settingsWith(t, "", record), "categorization")["model"]; model != nil {
		t.Errorf("model = %v with none configured, want null", model)
	}
}

// The whole way: the queue's answer through the router, then the undo of the
// change that recorded it.
func TestAnAnswerGivenInTheQueueCountsAndStopsCountingWhenItsChangeIsUndone(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	queueWithModelSuggestions(t, scratch, full)
	// txForeign carries a suggestion of "test-model" at confidence 91, on Delivery.
	answerSuggestions(t, scratch.Database, concat(
		answered("test-model", 95, 12, 12),
		answered("test-model", 95, 7, 0),
	))
	authoredtest.TakeRunIn(t, scratch.Database, 1)
	env := newTestEnv(t, withLedger(full.Path, fixtureNow), withDatabase(scratch.Database), withModelName("test-model"))
	cookie := env.unlock()
	read := func() [5]string {
		return percentsOf(t, settingsOf(t, &readEnv{testEnv: env, cookie: cookie}))
	}
	if got := read(); got != [5]string{"-", "-", "-", "-", "-"} {
		t.Fatalf("19 answers already judge a band: %v", got)
	}
	response := env.call("/api/review/acceptPending", `["`+txForeign+`",{"line":"Viagens","always":false,"nickname":"","reason":""}]`, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("acceptPending answered %d %s", response.Code, response.Body.String())
	}
	change, ok := changeIDIn(response.Body.Bytes())
	if !ok {
		t.Fatalf("no change id in %s", response.Body.String())
	}
	// 13 corrected of 20.
	if got := read(); got != [5]string{"-", "-", "-", "-", "65"} {
		t.Fatalf("the 20th answer, a correction, gives %v, want 13 of 20", got)
	}
	if response := env.call("/api/changes/undo", `["`+change+`"]`, cookie); response.Code != http.StatusOK {
		t.Fatalf("undo answered %d %s", response.Code, response.Body.String())
	}
	if got := read(); got != [5]string{"-", "-", "-", "-", "-"} {
		t.Errorf("an answer whose change was undone still counts: %v", got)
	}
}

func TestTheModelAndTheCalibrationAreOnTheWireWithTheirFields(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	answeredSuggestionsOverTheThirdState(t, scratch, full)
	env := newTestEnv(t, withLedger(full.Path, fixtureNow), withDatabase(scratch.Database), withModelName(calibrationModel))
	_, body := (&readEnv{testEnv: env, cookie: env.unlock()}).answer("settings.overview", `[]`)
	var wire struct {
		Categorization struct {
			Calibration []map[string]any `json:"calibration"`
			Model       map[string]any   `json:"model"`
		} `json:"categorization"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if problem := conformanceProblem("settings.overview", body); problem != "" {
		t.Errorf("the answer does not conform to the manifest: %s", problem)
	}
	if _, present := wire.Categorization.Model["lastRun"]; !present {
		t.Errorf("model = %v has no lastRun field", wire.Categorization.Model)
	}
}
