package suggest_test

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/suggest"
)

func TestARunWithNothingToAskRecordsNoRunAndLeavesTheLastOneAlone(t *testing.T) {
	model := newModelAnswering(t)
	config := suggest.Config{URL: model.URL, Model: "model-x"}

	t.Run("a ledger with no pending purchase leaves no last run", func(t *testing.T) {
		sources := scopeWorld(t, nil)
		counts, err := suggest.Run(t.Context(), sources, config, func() time.Time { return scopeNow })
		if err != nil || counts != (suggest.Counts{}) {
			t.Fatalf("counts = %+v err = %v, want nothing asked", counts, err)
		}
		view := sources.Database.View()
		run, found, err := view.LastModelRun(t.Context())
		view.Close()
		if err != nil || found {
			t.Errorf("last model run = %+v found=%v err=%v, want none", run, found, err)
		}
	})

	t.Run("a run after everything was asked keeps the instant of the one that asked", func(t *testing.T) {
		sources := scopeWorld(t, newEstablishmentPurchase)
		if _, err := suggest.Run(t.Context(), sources, config, func() time.Time { return scopeNow }); err != nil {
			t.Fatal(err)
		}
		asked := len(model.requests())
		counts, err := suggest.Run(t.Context(), sources, config, func() time.Time { return scopeNow.Add(time.Hour) })
		if err != nil || counts != (suggest.Counts{}) || len(model.requests()) != asked {
			t.Fatalf("second run: counts = %+v err = %v requests = %d, want nothing asked", counts, err, len(model.requests())-asked)
		}
		view := sources.Database.View()
		run, found, err := view.LastModelRun(t.Context())
		view.Close()
		if err != nil || !found || !run.OK || !run.At.Equal(scopeNow) {
			t.Errorf("last model run = %+v found=%v err=%v, want the first run's instant %v", run, found, err, scopeNow)
		}
	})
}
