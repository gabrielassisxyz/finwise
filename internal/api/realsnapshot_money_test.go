package api

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

// recentBillDays is how far back a card's bill still counts as the card being in
// use.
const recentBillDays = 60

// The card, the pots, the net worth and the home summary over a real snapshot,
// opt-in. Only counts are logged, on the same terms as the test beside it.
func TestTheCardPotsNetWorthAndHomeOverARealSnapshot(t *testing.T) {
	path := realSnapshotPath(t)
	reader, now := ledger.New(path), time.Now()
	ctx := withNow(t.Context(), func() time.Time { return now })
	env := newTestEnv(t, withLedger(path, now))
	run := &realRun{t: t, env: &readEnv{testEnv: env, cookie: env.unlock()}}
	calls := countingYields(t)

	recent := cardsWithARecentBill(t, reader, brday.DayOf(now))
	run.answer("card.overview", `[]`)
	calls.Store(0)
	run.answer("savingsPots.list", includeClosed)
	yieldCallsForOneList := calls.Load()
	run.answer("netWorth.overview", `[]`)
	run.answer("home.summary", `["`+string(brday.MonthOf(now))+`"]`)

	view, err := loadPots(ctx, reader, yieldAskerOf(reader))
	withoutValues(t, "pots", err)
	for _, id := range someDetails(view) {
		run.answer("savingsPots.detail", `["`+id+`"]`)
	}

	known, unknown := yieldMonthCounts(t, reader, view)
	t.Logf("answers_validated=%d answers_not_conforming=%d card_accounts_with_a_bill_in_the_last_%d_days=%d",
		run.answers, run.nonConforming, recentBillDays, recent)
	t.Logf("pots_per_bucket=%s pots_per_yield_base=%s", render(potsPerBucket(view)), render(potsPerBase(view)))
	t.Logf("months_of_known_yield=%d months_of_unknown_yield=%d", known, unknown)
	attributed, unattributed := movementCounts(view)
	t.Logf("movements_attributed=%d movements_unattributed=%d", attributed, unattributed)
	t.Logf("investments_beside_reserve_entries=%d investments_left_out_of_the_total=%d yield_statement_calls_for_one_list=%d",
		view.onReserveAccounts, view.leftOut, yieldCallsForOneList)

	if recent != 1 {
		t.Errorf("%d card accounts have a bill due in the last %d days or later, want exactly one", recent, recentBillDays)
	}
	if run.nonConforming != 0 {
		t.Error("an answer does not conform")
	}
}

// detailsAsked bounds the detail calls: each reads the whole ledger and asks
// the slow yield statement once per account.
const detailsAsked = 4

// someDetails picks the first, the middle and the last pot and the first account
// with movements no pot claimed, so each shape is answered once.
func someDetails(view *potsView) []string {
	var ids []string
	for _, index := range []int{0, len(view.entries) / 2, len(view.entries) - 1} {
		if index >= 0 && index < len(view.entries) && len(ids) < detailsAsked-1 {
			ids = append(ids, view.entries[index].id)
		}
	}
	for accountID := range view.unattributed {
		return append(ids, accountID)
	}
	return ids
}

func cardsWithARecentBill(t *testing.T, reader *ledger.Reader, today brday.Day) int {
	t.Helper()
	accounts, err := reader.Accounts(t.Context())
	withoutValues(t, "accounts", err)
	bills, err := reader.Bills(t.Context())
	withoutValues(t, "bills", err)
	since, _ := shiftedDay(today, -recentBillDays)
	cards := map[string]bool{}
	for _, account := range accounts {
		if account.Type == ledger.AccountCredit {
			cards[account.ID] = false
		}
	}
	for _, bill := range bills {
		if _, isCard := cards[bill.AccountID]; isCard && bill.DueDate >= since {
			cards[bill.AccountID] = true
		}
	}
	recent := 0
	for _, hasOne := range cards {
		if hasOne {
			recent++
		}
	}
	return recent
}

func potsPerBucket(view *potsView) map[string]int {
	counts := map[string]int{}
	for _, entry := range view.entries {
		counts[entry.bucket]++
	}
	return counts
}

func potsPerBase(view *potsView) map[string]int {
	counts := map[string]int{}
	for _, entry := range view.entries {
		if entry.yieldAsked || entry.closed {
			counts[string(entry.base)]++
		}
	}
	return counts
}

func movementCounts(view *potsView) (attributed, unattributed int) {
	for _, entry := range view.entries {
		attributed += len(entry.movements)
	}
	for _, own := range view.unattributed {
		unattributed += len(own)
	}
	return attributed, unattributed
}

// yieldMonthCounts counts, for each open pot with an identity and each month its
// account was sighted in, whether that month's yield is known. It asks the
// statement once per account and month.
func yieldMonthCounts(t *testing.T, reader *ledger.Reader, view *potsView) (known, unknown int) {
	t.Helper()
	for accountID, days := range view.sightings {
		for _, month := range monthsOfDays(days) {
			end := lastDayOf(month)
			rows := rowsOfSpan(t, reader, accountID, days, month, end)
			for _, entry := range view.entries {
				if entry.accountID != accountID || !entry.yieldAsked {
					continue
				}
				pot := potrules.Pot{AccountID: accountID, ReserveID: entry.reserveID, Stable: true}
				if potrules.YieldOfMonth(pot, entry.base, month, rows).Known {
					known++
				} else {
					unknown++
				}
			}
		}
	}
	return known, unknown
}

func rowsOfSpan(t *testing.T, reader *ledger.Reader, accountID string, days []brday.Day, month brday.Month, end brday.Day) []potrules.DayYield {
	t.Helper()
	from, to, ok := yieldSpanOf(days, month, end)
	if !ok {
		return nil
	}
	answered, err := reader.PotYields(context.Background(), accountID, from, to)
	withoutValues(t, "yield", err)
	return dayYieldsOf(answered)
}

func monthsOfDays(days []brday.Day) []brday.Month {
	seen := map[brday.Month]bool{}
	for _, day := range days {
		seen[monthOfDay(day)] = true
	}
	months := make([]brday.Month, 0, len(seen))
	for month := range seen {
		months = append(months, month)
	}
	sort.Slice(months, func(a, b int) bool { return months[a] < months[b] })
	return months
}
