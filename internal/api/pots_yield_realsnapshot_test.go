package api

import (
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

// sampleVariable overrides how many pairs are compared; each costs the
// collector's statement several seconds.
const (
	sampleVariable = "FINWISE_YIELD_PAIRS"
	defaultSampled = 24
	// maxSightingsApart bounds how far the sample reaches for pairs of a ledger
	// with few sightings.
	maxSightingsApart = 6
)

type samplePair struct {
	accountID string
	from, to  brday.Day
}

// A kind of difference, never a value: the ledger is a person's financial
// history and nothing from it reaches a log.
const (
	sameAnswer     = "equal"
	differsInCount = "row_count"
	differsInKind  = "reason"
	differsInValue = "figure"
)

func differenceKind(reference []ledger.PotYield, got []potrules.PairYield) string {
	if len(reference) != len(got) {
		return differsInCount
	}
	if differsFromReference(reference, got) == "" {
		return sameAnswer
	}
	if sortedReasons(referenceReasons(reference)) != sortedReasons(ruleReasons(got)) {
		return differsInKind
	}
	return differsInValue
}

func referenceReasons(rows []ledger.PotYield) []string {
	reasons := make([]string, 0, len(rows))
	for _, row := range rows {
		reasons = append(reasons, renderedText(row.Reason))
	}
	return reasons
}

func ruleReasons(rows []potrules.PairYield) []string {
	reasons := make([]string, 0, len(rows))
	for _, row := range rows {
		reasons = append(reasons, renderedText(row.Reason))
	}
	return reasons
}

func sortedReasons(reasons []string) string {
	sort.Strings(reasons)
	joined := ""
	for _, reason := range reasons {
		joined += reason + ";"
	}
	return joined
}

// sampledPairs takes pairs of consecutive sightings, evenly spread over the
// accounts that have a sighting of reserves, from the first of them on. A young
// ledger has fewer such pairs than asked for, and the pairs two sightings apart,
// then three, make up the rest: the statement answers any window the same way.
func sampledPairs(byAccount map[string][]brday.Day, rules map[string]potrules.AccountYield, wanted int) []samplePair {
	var accounts []string
	for accountID := range byAccount {
		accounts = append(accounts, accountID)
	}
	sort.Strings(accounts)
	var all []samplePair
	for apart := 1; len(all) < wanted && apart <= maxSightingsApart; apart++ {
		for _, accountID := range accounts {
			sighted := rules[accountID].Sightings
			for index := apart; index < len(byAccount[accountID]); index++ {
				from, to := byAccount[accountID][index-apart], byAccount[accountID][index]
				if len(sighted) > 0 && to >= sighted[0].Day {
					all = append(all, samplePair{accountID, from, to})
				}
			}
		}
	}
	if len(all) <= wanted {
		return all
	}
	picked := make([]samplePair, 0, wanted)
	for index := 0; index < wanted; index++ {
		picked = append(picked, all[index*len(all)/wanted])
	}
	return picked
}

func sampleSize(t *testing.T) int {
	t.Helper()
	text := os.Getenv(sampleVariable)
	if text == "" {
		return defaultSampled
	}
	size, err := strconv.Atoi(text)
	if err != nil || size < 1 {
		t.Fatalf("%s is not a positive number", sampleVariable)
	}
	return size
}

func TestPotYieldRowsEqualTheCollectorsStatementOverARealSnapshot(t *testing.T) {
	snapshot := viewOf(t, realSnapshotPath(t))
	byAccount := sightedDaysByAccount(t, snapshot)
	rules := rulesOfLedger(t, snapshot, byAccount)
	pairs := sampledPairs(byAccount, rules, sampleSize(t))

	kinds, withReason, knownPairs := map[string]int{}, map[string]int{}, 0
	for _, pair := range pairs {
		reference, err := snapshot.PotYields(t.Context(), pair.accountID, pair.from, pair.to)
		withoutValues(t, "the collector's statement", err)
		kinds[differenceKind(reference, rules[pair.accountID].Between(pair.from, pair.to))]++
		seen, unknown := map[string]bool{}, false
		for _, row := range reference {
			if row.Reason != nil && !seen[*row.Reason] {
				seen[*row.Reason] = true
				withReason[*row.Reason]++
			}
			unknown = unknown || row.Yield == nil
		}
		if !unknown && len(reference) > 0 {
			knownPairs++
		}
	}
	t.Logf("pairs_compared=%d outcomes=%s", len(pairs), render(kinds))
	t.Logf("pairs_with_every_row_known=%d pairs_where_the_statement_gives_each_reason=%s", knownPairs, render(withReason))
	if len(pairs) < 20 {
		t.Errorf("%d pairs were compared, want at least 20", len(pairs))
	}
	if kinds[sameAnswer] != len(pairs) {
		t.Errorf("%d of %d pairs differ from the collector's statement", len(pairs)-kinds[sameAnswer], len(pairs))
	}
}
