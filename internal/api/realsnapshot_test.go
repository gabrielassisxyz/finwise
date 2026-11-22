package api

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// The handlers over a real snapshot, opt-in. Only counts are logged: the
// snapshot is a person's financial history and no value from it (an amount, a
// description, a name, a document, an id, a month) may reach a log or a failure
// message. A label this package wrote itself, such as a bank's name, is not a
// value from the snapshot. A schema error can quote the value it rejected, so
// the text of a conformance failure is withheld here too.

// detailsPerMonth bounds the detail calls: each reads the whole ledger.
const detailsPerMonth = 3

func realSnapshotPath(t *testing.T) string {
	t.Helper()
	path := os.Getenv(snapshotVariable)
	if path == "" {
		t.Skip(snapshotVariable + " is not set")
	}
	return path
}

// withoutValues fails a step without the error's text, which may carry a row id.
func withoutValues(t *testing.T, step string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s failed (%T); the message is withheld because it may name a real value", step, err)
	}
}

// realRun counts what a run over a real snapshot did, and answers through the
// router with a session open.
type realRun struct {
	t             *testing.T
	env           *readEnv
	answers       int
	nonConforming int
}

// answer calls a read method, requires 200 and a body that conforms, and counts it.
func (run *realRun) answer(method, arguments string) []byte {
	run.t.Helper()
	status, body := run.env.answer(method, arguments)
	if status != http.StatusOK {
		run.t.Errorf("%s answered %d", method, status)
		return nil
	}
	run.answers++
	if conformanceProblem(method, body) != "" {
		run.nonConforming++
		run.t.Errorf("%s gave an answer that does not conform (the message is withheld)", method)
	}
	return body
}

func TestEveryHandlerOverARealSnapshot(t *testing.T) {
	path := realSnapshotPath(t)
	snapshot, ctx := viewOf(t, path), t.Context()
	around, err := loadContext(ctx, snapshot)
	withoutValues(t, "context", err)
	all, err := snapshot.CleanTransactions(ctx, wholeLedger)
	withoutValues(t, "transactions", err)

	env := newTestEnv(t, withLedger(path, time.Now()))
	run := &realRun{t: t, env: &readEnv{testEnv: env, cookie: env.unlock()}}
	months := monthsOf(all)

	run.answer("shell.overview", `[]`)
	run.answer("accounts.list", `[]`)
	run.answer("transactions.filterOptions", `[]`)
	detailsAsked := run.answerEveryAccountDetail(around.accounts)

	served, balanced := run.serveEveryMonth(snapshot, around, months)
	whole := txrules.Interpret(all, around.reference)

	t.Logf("months=%d answers_validated=%d answers_not_conforming=%d", len(months), run.answers, run.nonConforming)
	t.Logf("listed_per_month_summed=%d listed_whole_ledger=%d tally_balanced_every_month=%t", served.listed, whole.Tally.Listed, balanced)
	t.Logf("input_rows_whole_ledger=%d attached_as_tax=%d pot_movements=%d", whole.Tally.Input, whole.Tally.AttachedAsTax, whole.Tally.PotMovements)
	t.Logf("accounts=%d detail_asked=%d accounts_per_bank=%s", len(around.accounts), detailsAsked, render(countLabels(around.banks)))
	t.Logf("signals=%s rows_unconverted=%d details_checked=%d", render(signalsOf(around.accounts)), unconvertedIn(whole), served.details)

	if served.listed != whole.Tally.Listed {
		t.Errorf("the months list %d lines and the ledger read whole lists %d", served.listed, whole.Tally.Listed)
	}
	if run.nonConforming != 0 || !balanced {
		t.Error("an answer does not conform or a month's tally does not add up")
	}
}

type servedMonths struct {
	listed, details int
}

// serveEveryMonth asks for every month the ledger holds and checks that the
// widened read of each places every row it read.
func (run *realRun) serveEveryMonth(snapshot *ledger.View, around ledgerContext, months []brday.Month) (servedMonths, bool) {
	var served servedMonths
	balanced := true
	for _, month := range months {
		period, err := brday.MonthPeriod(month)
		withoutValues(run.t, "month period", err)
		wide, err := widened(period, taxPairingDays)
		withoutValues(run.t, "widened period", err)
		rows, err := snapshot.CleanTransactions(run.t.Context(), wide)
		withoutValues(run.t, "widened read", err)
		tally := txrules.Interpret(rows, around.reference).Tally
		if !tally.Balanced() || tally.Input != len(rows) {
			balanced = false
			run.t.Errorf("a month's tally does not add up: %+v for %d rows", tally, len(rows))
		}

		body := run.answer("transactions.list", listRaw(string(month), periodMonth))
		ids := idsInList(run.t, body)
		served.listed += len(ids)
		served.details += run.answerSomeDetails(ids)
	}
	run.answer("transactions.list", listRaw(string(months[len(months)-1]), periodLastQuarter))
	run.answer("transactions.list", listRaw(string(months[len(months)-1]), periodYear))
	run.answer("transactions.detail", `["an-id-the-ledger-does-not-hold"]`)
	return served, balanced
}

func (run *realRun) answerSomeDetails(ids []string) int {
	var picked []string
	for _, index := range []int{0, len(ids) / 2, len(ids) - 1} {
		if index >= 0 && index < len(ids) && len(picked) < detailsPerMonth {
			picked = append(picked, ids[index])
		}
	}
	for _, id := range picked {
		run.answer("transactions.detail", `["`+id+`"]`)
	}
	return len(picked)
}

func (run *realRun) answerEveryAccountDetail(accounts []ledger.Account) int {
	for _, account := range accounts {
		run.answer("accounts.detail", `["`+account.ID+`"]`)
	}
	return len(accounts)
}

func idsInList(t *testing.T, body []byte) []string {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatal("the list is not JSON")
	}
	return idsOf(rows)
}

func monthsOf(rows []ledger.Transaction) []brday.Month {
	seen := map[brday.Month]bool{}
	for _, row := range rows {
		seen[brday.MonthOf(row.Date)] = true
	}
	months := make([]brday.Month, 0, len(seen))
	for month := range seen {
		months = append(months, month)
	}
	sort.Slice(months, func(a, b int) bool { return months[a] < months[b] })
	return months
}

func unconvertedIn(result txrules.Result) int {
	count := 0
	for _, listed := range result.Listed {
		if listed.Unconverted {
			count++
		}
	}
	return count
}

func signalsOf(accounts []ledger.Account) map[string]int {
	signals := map[string]int{}
	for _, account := range accounts {
		signals[signalOf(account)]++
	}
	return signals
}

func signalOf(account ledger.Account) string {
	coded, hasCode := bankOfCode(account.TransferNumber)
	named := banksNamedIn(account.Name)
	switch {
	case hasCode && len(named) == 0:
		return "code only"
	case !hasCode && len(named) == 1:
		return "name only"
	case hasCode && len(named) == 1 && named[coded]:
		return "code and name agree"
	case hasCode && len(named) >= 1:
		return "signals disagree"
	case !hasCode && len(named) == 0:
		return "no signal"
	}
	return "several names"
}

func countLabels(labels map[string]string) map[string]int {
	counts := map[string]int{}
	for _, label := range labels {
		counts[label]++
	}
	return counts
}

func render(values map[string]int) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, strings.ReplaceAll(name, " ", "_")+"="+strconv.Itoa(values[name]))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}
