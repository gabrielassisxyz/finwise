package api

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

// monthsPerAccount is how many of an account's latest months are compared; each
// costs the collector's statement several seconds.
const monthsPerAccount = 3

// The months a person sees add the days, where they used to come from one
// window. They agree unless a transfer reached the ledger after the window it
// belongs to closed.
func TestTheMonthsYieldFromItsDaysAgreesWithTheCollectorsOneWindowOverARealSnapshot(t *testing.T) {
	reader := ledger.New(realSnapshotPath(t))
	now := time.Now()
	ctx := withNow(t.Context(), func() time.Time { return now })
	view, err := loadPots(ctx, viewOf(t, realSnapshotPath(t)), true)
	withoutValues(t, "pots", err)
	rules := rulesOfLedger(t, reader, view.sightings)

	outcomes := map[string]int{}
	for _, entry := range view.entries {
		if !entry.yieldAsked {
			continue
		}
		days := view.sightings[entry.accountID]
		months := monthsOfDays(days)
		for _, month := range months[max(0, len(months)-monthsPerAccount):] {
			outcomes[compareMonth(t, reader, rules[entry.accountID], entry, days, month)]++
		}
	}
	t.Logf("pot_months_by_outcome=%s", render(outcomes))
}

func compareMonth(t *testing.T, reader *ledger.Reader, rules potrules.AccountYield, entry potEntry, days []brday.Day, month brday.Month) string {
	t.Helper()
	from, to, ok := yieldSpanOf(days, month, lastDayOf(month))
	if !ok {
		return "no_span"
	}
	reference, err := reader.PotYields(t.Context(), entry.accountID, from, to)
	withoutValues(t, "the collector's statement", err)
	var oneWindow []potrules.DayYield
	for _, row := range reference {
		if row.ReserveID != nil {
			oneWindow = append(oneWindow, potrules.DayYield{AccountID: entry.accountID, ReserveID: *row.ReserveID, From: from, To: to, Yield: row.Yield})
		}
	}
	pot := potrules.Pot{AccountID: entry.accountID, ReserveID: entry.reserveID, Stable: true}
	before := potrules.YieldOfMonth(pot, entry.base, month, oneWindow)
	after := potrules.YieldOfMonth(pot, entry.base, month, rules.DailyYields(sightingsFromTo(days, from, to)))
	switch {
	case before.Known && after.Known && before.Total == after.Total:
		return "same_known_figure"
	case before.Known && after.Known:
		return "different_known_figures"
	case !before.Known && !after.Known:
		return "unknown_in_both"
	case before.Known:
		return "known_only_by_the_one_window"
	}
	return "known_only_by_the_days"
}
