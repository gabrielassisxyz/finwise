package api

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

// The collector's yield statement stays in the reader as the reference these
// tests hold the rules to: for a window, the same rows with the same figures
// and the same reasons.

func renderedCents(amount *money.Cents) string {
	if amount == nil {
		return "-"
	}
	return fmt.Sprint(int64(*amount))
}

func renderedText(text *string) string {
	if text == nil {
		return "-"
	}
	return *text
}

func renderedRow(id, name *string, previous, current, transfers, yield *money.Cents, reason *string) string {
	return strings.Join([]string{renderedText(id), renderedText(name), renderedCents(previous),
		renderedCents(current), renderedCents(transfers), renderedCents(yield), renderedText(reason)}, "|")
}

func referenceRows(answered []ledger.PotYield) []string {
	rows := make([]string, 0, len(answered))
	for _, row := range answered {
		rows = append(rows, renderedRow(row.ReserveID, row.ReserveName, row.Previous, row.Current, row.Transfers, row.Yield, row.Reason))
	}
	sort.Strings(rows)
	return rows
}

func ruleRows(answered []potrules.PairYield) []string {
	rows := make([]string, 0, len(answered))
	for _, row := range answered {
		rows = append(rows, renderedRow(row.ReserveID, row.ReserveName, row.Previous, row.Current, row.Transfers, row.Yield, row.Reason))
	}
	sort.Strings(rows)
	return rows
}

// differsFromReference is empty when the rules answer the window as the
// statement does, and otherwise says how many rows differ and in which reasons.
func differsFromReference(reference []ledger.PotYield, got []potrules.PairYield) string {
	want, have := referenceRows(reference), ruleRows(got)
	if strings.Join(want, "\n") == strings.Join(have, "\n") {
		return ""
	}
	return fmt.Sprintf("the statement answers\n  %s\nthe rules answer\n  %s", strings.Join(want, "\n  "), strings.Join(have, "\n  "))
}

func sightedDaysByAccount(t testing.TB, snapshot *ledger.View) map[string][]brday.Day {
	t.Helper()
	days, err := snapshot.BalanceHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	byAccount := map[string][]brday.Day{}
	for _, day := range days {
		byAccount[day.AccountID] = append(byAccount[day.AccountID], day.Day)
	}
	return byAccount
}

func rulesOfLedger(t testing.TB, snapshot *ledger.View, byAccount map[string][]brday.Day) map[string]potrules.AccountYield {
	t.Helper()
	history, err := snapshot.YieldHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var accounts []string
	for accountID := range byAccount {
		accounts = append(accounts, accountID)
	}
	sort.Strings(accounts)
	return yieldRulesOf(history, accounts)
}

// everyReason is each thing the statement can say instead of a figure; the
// fixture ledger must make it say every one, or its agreement proves nothing
// about that reason.
var everyReason = []string{
	potrules.ReasonCoverageGap, potrules.ReasonClosed, potrules.ReasonUnstable, potrules.ReasonCurrency,
	potrules.ReasonAmount, potrules.ReasonTransferRemoved, potrules.ReasonTransferPending,
	potrules.ReasonTransferUnsettled, potrules.ReasonTransferSign, potrules.ReasonUnmatchedTransfer,
	potrules.ReasonNameReused, potrules.ReasonNameChanged,
}

// widestWindow bounds how many sightings apart the two days of a compared
// window are: the statement costs the same for any window, and the fixture's
// differences all show within a few days.
const widestWindow = 6

func TestEveryWindowOfTheFixtureLedgerIsAnsweredAsTheCollectorsStatementAnswersIt(t *testing.T) {
	snapshot := viewOf(t, yieldScenarioLedger(t).Path)
	byAccount := sightedDaysByAccount(t, snapshot)
	rules := rulesOfLedger(t, snapshot, byAccount)
	reasons, windows, known := map[string]int{}, 0, 0

	for accountID, days := range byAccount {
		for first := range days {
			for last := first + 1; last < len(days) && last-first <= widestWindow; last++ {
				reference, err := snapshot.PotYields(t.Context(), accountID, days[first], days[last])
				if err != nil {
					t.Fatal(err)
				}
				if problem := differsFromReference(reference, rules[accountID].Between(days[first], days[last])); problem != "" {
					t.Errorf("%s from %s to %s: %s", accountID, days[first], days[last], problem)
				}
				windows++
				for _, row := range reference {
					if row.Reason != nil {
						reasons[*row.Reason]++
					} else if row.Yield != nil {
						known++
					}
				}
			}
		}
	}
	if windows < 150 || known == 0 {
		t.Errorf("only %d windows and %d known yields were compared", windows, known)
	}
	for _, reason := range everyReason {
		if reasons[reason] == 0 {
			t.Errorf("no window of the fixture ledger makes the statement answer %q", reason)
		}
	}
}
