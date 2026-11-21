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

// recentDays is how far back a purchase makes a card the one in use. A bank
// keeps issuing bills for a card nobody uses, so a bill is only counted beside it.
const recentDays = 60

// The card, the pots, the net worth and the home summary over a real snapshot,
// opt-in. Only counts are logged, on the same terms as the test beside it.
func TestTheCardPotsNetWorthAndHomeOverARealSnapshot(t *testing.T) {
	path := realSnapshotPath(t)
	reader, now := ledger.New(path), time.Now()
	ctx := withNow(t.Context(), func() time.Time { return now })
	env := newTestEnv(t, withLedger(path, now))
	run := &realRun{t: t, env: &readEnv{testEnv: env, cookie: env.unlock()}}

	cards, withBill, withPurchase, err := cardCounts(ctx, reader, brday.DayOf(now))
	withoutValues(t, "card counts", err)
	run.answer("card.overview", `[]`)
	run.answer("savingsPots.list", includeClosed)
	run.answer("netWorth.overview", `[]`)
	run.answer("home.summary", `["`+string(brday.MonthOf(now))+`"]`)

	view, err := loadPots(ctx, reader, true)
	withoutValues(t, "pots", err)
	for _, id := range someDetails(view) {
		run.answer("savingsPots.detail", `["`+id+`"]`)
	}

	known, unknown := yieldMonthCounts(t, reader, view)
	t.Logf("answers_validated=%d answers_not_conforming=%d", run.answers, run.nonConforming)
	t.Logf("card_accounts=%d with_a_bill_due_in_the_last_%d_days=%d with_a_purchase_in_the_last_%d_days=%d",
		cards, recentDays, withBill, recentDays, withPurchase)
	t.Logf("pots_per_bucket=%s pots_per_yield_base=%s", render(potsPerBucket(view)), render(potsPerBase(view)))
	t.Logf("months_of_known_yield=%d months_of_unknown_yield=%d", known, unknown)
	attributed, unattributed := movementCounts(view)
	t.Logf("movements_attributed=%d movements_unattributed=%d", attributed, unattributed)
	t.Logf("investments_beside_reserve_entries=%d investments_left_out_of_the_total=%d",
		view.onReserveAccounts, view.leftOut)

	if withPurchase != 1 {
		t.Errorf("%d card accounts have a purchase in the last %d days, want exactly one", withPurchase, recentDays)
	}
	if run.nonConforming != 0 {
		t.Error("an answer does not conform")
	}
}

// detailsAsked bounds the detail calls: each reads the whole ledger.
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

// cardCounts counts the card accounts, those with a bill due in the last
// recentDays days or later, and those with a purchase in the last recentDays days.
func cardCounts(ctx context.Context, reader *ledger.Reader, today brday.Day) (cards, withBill, withPurchase int, err error) {
	around, err := loadContext(ctx, reader)
	if err != nil {
		return 0, 0, 0, err
	}
	bills, err := reader.Bills(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	lastPurchase, err := lastPurchaseDays(ctx, reader, around, today)
	if err != nil {
		return 0, 0, 0, err
	}
	since, _ := shiftedDay(today, -recentDays)
	billed := map[string]bool{}
	for _, bill := range bills {
		billed[bill.AccountID] = billed[bill.AccountID] || bill.DueDate >= since
	}
	for _, account := range around.accounts {
		if account.Type != ledger.AccountCredit {
			continue
		}
		cards++
		if billed[account.ID] {
			withBill++
		}
		if lastPurchase[account.ID] >= since && lastPurchase[account.ID] != "" {
			withPurchase++
		}
	}
	return cards, withBill, withPurchase, nil
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
// account was sighted in, whether that month's yield is known.
func yieldMonthCounts(t *testing.T, reader *ledger.Reader, view *potsView) (known, unknown int) {
	t.Helper()
	history, err := reader.YieldHistory(t.Context())
	withoutValues(t, "yield history", err)
	var accounts []string
	for accountID := range view.sightings {
		accounts = append(accounts, accountID)
	}
	rules := yieldRulesOf(history, accounts)
	for accountID, days := range view.sightings {
		for _, month := range monthsOfDays(days) {
			rows := rowsOfSpan(rules[accountID], days, month, lastDayOf(month))
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

func rowsOfSpan(rules potrules.AccountYield, days []brday.Day, month brday.Month, end brday.Day) []potrules.DayYield {
	from, to, ok := yieldSpanOf(days, month, end)
	if !ok {
		return nil
	}
	return rules.DailyYields(sightingsFromTo(days, from, to))
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

// The counting is checked over invented cards, so that it is known to count a
// purchase and to leave out one older than the window.
func TestTheCardCountsSeparateABillFromAPurchase(t *testing.T) {
	f := cardLedgerWith(t, "card-a", "2026-04-05", "card-b", "2026-05-05")
	addPurchaseOn(f, "card-a", "tx-recent", utc(2026, time.March, 10, 15, 0), 10, "")
	addPurchaseOn(f, "card-b", "tx-older", utc(2025, time.December, 1, 15, 0), 10, "")

	cards, withBill, withPurchase, err := cardCounts(t.Context(), ledgerOf(f), "2026-03-20")
	if err != nil || cards != 2 || withBill != 2 || withPurchase != 1 {
		t.Errorf("cards %d with a bill %d with a purchase %d (%v), want 2, 2 and 1", cards, withBill, withPurchase, err)
	}
}
