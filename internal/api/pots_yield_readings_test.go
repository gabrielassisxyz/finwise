package api

import (
	"net/http"
	"sync/atomic"
	"testing"
)

// countingYieldReadings makes the router's ledger reader count every reading
// of the ledger's transactions the yield history makes. It goes after the
// option that sets the ledger.
func countingYieldReadings(readings *atomic.Int64) envOption {
	return func(config *Config) {
		config.Ledger = config.Ledger.ObservingYieldReadings(func() { readings.Add(1) })
	}
}

// The two reads whose time was measured for the pots' yields read the ledger's
// transactions once for them in each request, beside the request's own reading
// of the rows, and no read does more. A second request reads again: nothing of
// the first one's reading is kept. The reader counts both kinds of reading at
// once, so neither observer drops the other.
func TestEachRequestReadsTheTransactionsAtMostOnceForThePotsYields(t *testing.T) {
	var yieldReadings, rowReadings atomic.Int64
	env := queueingThirdState(t, countingYieldReadings(&yieldReadings), countingRowReadings(&rowReadings))
	yielding := map[string]bool{"savingsPots.list": true, "netWorth.overview": true}

	for _, c := range readsOfTheTimingTable {
		for request := 1; request <= 2; request++ {
			yieldReadings.Store(0)
			rowReadings.Store(0)
			if status, body := env.answer(c.method, c.arguments); status != http.StatusOK {
				t.Fatalf("%s %s answered %d %s", c.method, c.arguments, status, body)
			}
			got := yieldReadings.Load()
			if yielding[c.method] && (got != 1 || rowReadings.Load() != 1) {
				t.Errorf("request %d of %s read the transactions %d times for the yields and the rows %d times, want once each",
					request, c.method, got, rowReadings.Load())
			}
			if got > 1 {
				t.Errorf("request %d of %s %s read the transactions %d times for the yields, want at most once",
					request, c.method, c.arguments, got)
			}
		}
	}
}
