package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// The months of the full invented ledger.
var fullLedgerMonths = []brday.Month{"2026-01", "2026-02", "2026-03"}

// invariantProblems names what breaks the two invariants of a month's totals:
// every transaction the list answers is in exactly one disposition, so the
// outcomes are as many as the listed ids, in their order, and weigh what they
// weigh; and every debit that adds to lines or leaves something unassigned
// accounts for all of what was paid.
func invariantProblems(month brday.Month, ids []string, amounts []money.Cents, disposed monthDispositions) []string {
	var problems []string
	disposedIDs := make([]string, 0, len(disposed.listed))
	for _, listed := range disposed.listed {
		disposedIDs = append(disposedIDs, listed.ID)
	}
	if len(disposed.outcomes) != len(disposed.listed) || !slices.Equal(disposedIDs, ids) {
		return []string{fmt.Sprintf("%s: %d outcomes for %v, the list answers %v", month, len(disposed.outcomes), disposedIDs, ids)}
	}
	var weighed, listedWeight money.Cents
	for i, outcome := range disposed.outcomes {
		listed := disposed.listed[i]
		if outcome.Disposition < budgetrules.Unconverted || outcome.Disposition > budgetrules.Uncategorised {
			problems = append(problems, fmt.Sprintf("%s: %s is in no disposition", month, listed.ID))
		}
		weighed += outcome.Magnitude
		if !listed.Unconverted {
			listedWeight += max(amounts[i], -amounts[i])
		}
		if problem := debitProblem(listed, outcome); problem != "" {
			problems = append(problems, fmt.Sprintf("%s: %s %s", month, listed.ID, problem))
		}
	}
	if weighed != listedWeight {
		problems = append(problems, fmt.Sprintf("%s: the dispositions weigh %d, the list's transactions %d", month, weighed, listedWeight))
	}
	return problems
}

func debitProblem(listed txrules.Transaction, outcome budgetrules.Outcome) string {
	switch outcome.Disposition {
	case budgetrules.Split, budgetrules.OnALine, budgetrules.Uncategorised:
	default:
		return ""
	}
	counted := outcome.Unassigned + outcome.AppliedReimbursement
	for _, line := range outcome.Lines {
		if line.Effect != budgetrules.Adds {
			return fmt.Sprintf("is %v and takes off a line", outcome.Disposition)
		}
		counted += line.Amount
	}
	if listed.Amount > 0 || counted != outcome.Magnitude {
		return fmt.Sprintf("is %v of %d and accounts for %d", outcome.Disposition, outcome.Magnitude, counted)
	}
	return ""
}

// monthInvariants checks every month of the full ledger over the stores as they are.
func monthInvariants(t *testing.T, env *readEnv, scratch *authoredtest.Scratch, ledgerPath string) []string {
	t.Helper()
	views := Views{Ledger: viewOf(t, ledgerPath), Authored: scratch.Database.View()}
	defer views.Authored.Close()
	var problems []string
	for _, month := range fullLedgerMonths {
		disposed, err := disposeMonth(t.Context(), views, month)
		if err != nil {
			t.Fatal(err)
		}
		ids, amounts := listedCents(t, env, month)
		problems = append(problems, invariantProblems(month, ids, amounts, disposed)...)
	}
	return problems
}

func TestTheMonthsInvariantsHoldAfterEveryRecordedWriteCase(t *testing.T) {
	checked := 0
	for _, c := range writeCases {
		if c.expects != expectsRecorded || c.handlers != nil {
			continue
		}
		scratch := authoredtest.Open(t, fixtureClock)
		full := newFullLedger(t)
		writes := &writeEnv{testEnv: newTestEnv(t, withLedger(full.Path, fixtureNow), withDatabase(scratch.Database)), scratch: scratch}
		writes.cookie = writes.unlock()
		if c.state != nil {
			c.state(t, scratch)
		}
		for _, call := range c.before {
			changeIDOfAnswer(t, writes.post(call.method, call.arguments))
		}
		changeIDOfAnswer(t, writes.post(c.method, c.arguments))
		for _, problem := range monthInvariants(t, &readEnv{testEnv: writes.testEnv, cookie: writes.cookie}, scratch, full.Path) {
			t.Errorf("after %s %s: %s", c.method, c.arguments, problem)
		}
		checked++
	}
	t.Logf("the invariants held after %d recorded cases", checked)
	if checked < 9 {
		t.Errorf("%d recorded cases were checked, fewer than this slice's own", checked)
	}
}

func TestTheInvariantCheckSeesATransactionDroppedOrAPartLost(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	env := newReadEnvOver(t, full, scratch.Database)
	views := Views{Ledger: viewOf(t, full.Path), Authored: scratch.Database.View()}
	defer views.Authored.Close()
	disposed, err := disposeMonth(t.Context(), views, "2026-03")
	if err != nil {
		t.Fatal(err)
	}
	ids, amounts := listedCents(t, env, "2026-03")
	if problems := invariantProblems("2026-03", ids, amounts, disposed); len(problems) != 0 {
		t.Fatalf("the control failed: %v", problems)
	}
	dropped := monthDispositions{listed: disposed.listed[1:], outcomes: disposed.outcomes[1:]}
	if len(invariantProblems("2026-03", ids, amounts, dropped)) == 0 {
		t.Error("a transaction left out of the dispositions went unnoticed")
	}
	lost := monthDispositions{listed: disposed.listed, outcomes: slices.Clone(disposed.outcomes)}
	at := slices.IndexFunc(lost.outcomes, func(outcome budgetrules.Outcome) bool { return outcome.Disposition == budgetrules.Split })
	if at < 0 {
		t.Fatal("March has no split to lose a part of")
	}
	lost.outcomes[at].Lines = lost.outcomes[at].Lines[1:]
	if len(invariantProblems("2026-03", ids, amounts, lost)) == 0 {
		t.Error("a split that lost a part went unnoticed")
	}
}

// panelsFirstSplit is what the panel sends when the split is switched on: the
// whole amount on the transaction's line as the detail names it, then a part
// of zero.
func panelsFirstSplit(t *testing.T, id string, detail countedDetail) string {
	t.Helper()
	line, err := json.Marshal(detail.Line)
	if err != nil {
		t.Fatal(err)
	}
	magnitude := strings.TrimPrefix(string(detail.Amount), "-")
	return `["` + id + `",[{"line":` + string(line) + `,"amount":` + magnitude + `},{"line":null,"amount":0}]]`
}

func TestThePanelsFirstSplitIsAcceptedForEveryPurchaseThatShowsTheSplit(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	env := newReadEnvOver(t, full, scratch.Database)
	var checked, onALine int
	for _, month := range fullLedgerMonths {
		ids, _ := listedCents(t, env, month)
		for _, id := range ids {
			detail := detailOf(t, env, id)
			if !detail.Sections.Split {
				continue
			}
			payload := panelsFirstSplit(t, id, detail)
			if status, body := env.answer("transactions.setSplit", payload); status != http.StatusOK {
				t.Errorf("%s answered %d %s", payload, status, body)
				continue
			}
			line := "-"
			if detail.Line != nil {
				line, onALine = *detail.Line, onALine+1
			}
			after := detailOf(t, env, id)
			want := []string{line + " " + strings.TrimPrefix(string(detail.Amount), "-"), "- 0.00"}
			if !after.Split || !reflect.DeepEqual(partsOf(after), want) || string(after.SplitRemaining) != "0.00" {
				t.Errorf("%s: split %v, parts %v, remaining %s", payload, after.Split, partsOf(after), after.SplitRemaining)
			}
			checked++
		}
	}
	t.Logf("the panel's split was accepted on %d purchases, %d of them on a line", checked, onALine)
	if checked < 10 || onALine < 3 {
		t.Errorf("the panel's split was tried on %d purchases, %d of them on a line", checked, onALine)
	}
}
