package authoredtest_test

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

type answers struct {
	threshold, examples, day int64
	label                    string
	mode                     authored.ClosingMode
}

func read(t *testing.T, view *authored.View) answers {
	t.Helper()
	ctx := t.Context()
	var got answers
	var err [5]error
	got.threshold, err[0] = view.ConfidenceThreshold(ctx)
	got.examples, err[1] = view.MinExamples(ctx)
	got.label, err[2] = view.CardLabel(ctx)
	got.mode, err[3] = view.CardClosingMode(ctx)
	got.day, err[4] = view.ManualClosingDay(ctx)
	for _, e := range err {
		if e != nil {
			t.Fatal(e)
		}
	}
	return got
}

func TestTheSettingsBuilderSetsAnySubsetAndLeavesTheRestAtTheirDefaults(t *testing.T) {
	at := time.Date(2026, time.March, 20, 15, 0, 0, 0, time.UTC)
	defaults := answers{85, 5, 28, "Físico", authored.ClosingAuto}
	cases := []struct {
		name     string
		settings authoredtest.Settings
		want     answers
	}{
		{"none", authoredtest.Settings{}, defaults},
		{"the threshold", authoredtest.Settings{ConfidenceThreshold: authoredtest.Some[int64](60)},
			answers{60, 5, 28, "Físico", authored.ClosingAuto}},
		{"the label and the day", authoredtest.Settings{CardLabel: authoredtest.Some("Cartão"), ManualClosingDay: authoredtest.Some[int64](10)},
			answers{85, 5, 10, "Cartão", authored.ClosingAuto}},
		{"the mode alone", authoredtest.Settings{ClosingMode: authoredtest.Some(authored.ClosingManual)},
			answers{85, 5, 28, "Físico", authored.ClosingManual}},
		{"all five", authoredtest.Settings{
			ConfidenceThreshold: authoredtest.Some[int64](99), MinExamples: authoredtest.Some[int64](50),
			CardLabel: authoredtest.Some("X"), ClosingMode: authoredtest.Some(authored.ClosingManual),
			ManualClosingDay: authoredtest.Some[int64](1)},
			answers{99, 50, 1, "X", authored.ClosingManual}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := authoredtest.Fresh(t, func() time.Time { return at })
			authoredtest.WithSettings(t, database, at, c.settings)
			view := database.View()
			defer view.Close()
			if got := read(t, view); got != c.want {
				t.Errorf("read %+v, want %+v", got, c.want)
			}
		})
	}
}
