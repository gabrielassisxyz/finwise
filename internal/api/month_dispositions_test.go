package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// listedCents is a month's list as transactions.list answers it: the ids in
// order and each amount in cents, read without passing through a float.
func listedCents(t *testing.T, env *readEnv, month brday.Month) ([]string, []money.Cents) {
	t.Helper()
	status, body := env.answer("transactions.list", listRaw(string(month), "month"))
	if status != http.StatusOK {
		t.Fatalf("transactions.list %s answered %d", month, status)
	}
	var rows []struct {
		ID     string          `json:"id"`
		Amount json.RawMessage `json:"amount"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatal(err)
	}
	ids, amounts := []string{}, []money.Cents{}
	for _, row := range rows {
		cents, err := ParseReais(row.Amount)
		if err != nil {
			t.Fatal(err)
		}
		ids, amounts = append(ids, row.ID), append(amounts, cents)
	}
	return ids, amounts
}

func TestAMonthsDispositionsCountAndWeighWhatTheListAnswersForThatMonth(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	env := newReadEnvOver(t, full, scratch.Database)
	views := Views{Ledger: viewOf(t, full.Path), Authored: scratch.Database.View()}
	defer views.Authored.Close()
	for _, month := range []brday.Month{"2026-03", "2026-02", "2026-01"} {
		disposed, err := disposeMonth(t.Context(), views, month)
		if err != nil {
			t.Fatalf("%s: %v", month, err)
		}
		ids, amounts := listedCents(t, env, month)
		var disposedIDs []string
		for _, listed := range disposed.listed {
			disposedIDs = append(disposedIDs, listed.ID)
		}
		if len(disposed.outcomes) != len(ids) || !slices.Equal(disposedIDs, ids) {
			t.Errorf("%s: %d outcomes for %v, want one for each of the list's %v", month, len(disposed.outcomes), disposedIDs, ids)
			continue
		}
		var weighed, listedWeight money.Cents
		for i, outcome := range disposed.outcomes {
			weighed += outcome.Magnitude
			// A row with no value in reais weighs nothing, as budgetrules has it.
			if ids[i] != txUnconverted {
				listedWeight += max(amounts[i], -amounts[i])
			}
		}
		if weighed != listedWeight || listedWeight == 0 {
			t.Errorf("%s: the dispositions weigh %d, the list's transactions %d", month, weighed, listedWeight)
		}
	}
}

func TestAMonthsDispositionsFollowWhatWasAuthoredAboutEachTransaction(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	views := Views{Ledger: viewOf(t, full.Path), Authored: scratch.Database.View()}
	defer views.Authored.Close()
	want := map[brday.Month]map[string]budgetrules.Disposition{
		"2026-03": {
			txPending: budgetrules.Uncategorised, txCardPayment: budgetrules.Movement, txBankBill: budgetrules.Movement,
			txUnconverted: budgetrules.Unconverted, txRefund: budgetrules.OtherCredit, txPixOut: budgetrules.Excluded,
			txForeign: budgetrules.OnALine, txBoleto: budgetrules.Waiting, txOwnTransfer: budgetrules.Movement,
			txInstalment: budgetrules.Split, txSalary: budgetrules.Refund, txFee: budgetrules.Fee,
			txFirstOfMarch: budgetrules.OnALine,
		},
		"2026-02": {txLastOfFebruary: budgetrules.Uncategorised, txMonthEnd: budgetrules.OnALine},
		"2026-01": {txStale: budgetrules.Reimbursement},
	}
	for month, dispositions := range want {
		disposed, err := disposeMonth(t.Context(), views, month)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]budgetrules.Disposition{}
		for i, listed := range disposed.listed {
			got[listed.ID] = disposed.outcomes[i].Disposition
		}
		if !reflect.DeepEqual(got, dispositions) {
			t.Errorf("%s: dispositions %v, want %v", month, got, dispositions)
		}
	}
}

func TestAFeeARulePutOnTheFeesLineIsAFeeOnThatLine(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	views := Views{Ledger: viewOf(t, full.Path), Authored: scratch.Database.View()}
	defer views.Authored.Close()
	disposed, err := disposeMonth(t.Context(), views, "2026-03")
	if err != nil {
		t.Fatal(err)
	}
	for i, listed := range disposed.listed {
		if listed.ID != txFee {
			continue
		}
		outcome := disposed.outcomes[i]
		want := []budgetrules.LineAmount{{Line: budgetrules.LineID(authored.FeesLineID), Effect: budgetrules.Adds, Amount: 2_500}}
		if outcome.Disposition != budgetrules.Fee || !reflect.DeepEqual(outcome.Lines, want) {
			t.Errorf("the fee is %v on %v, want a fee adding 25.00 to the fees line", outcome.Disposition, outcome.Lines)
		}
		return
	}
	t.Fatal("the fee is not among March's transactions")
}

// outcomeOf is what the month function decided for one of the month's
// transactions.
func outcomeOf(t *testing.T, views Views, month brday.Month, id string) budgetrules.Outcome {
	t.Helper()
	disposed, err := disposeMonth(t.Context(), views, month)
	if err != nil {
		t.Fatal(err)
	}
	for i, listed := range disposed.listed {
		if listed.ID == id {
			return disposed.outcomes[i]
		}
	}
	t.Fatalf("%s is not among %s's transactions", id, month)
	return budgetrules.Outcome{}
}

func TestThePersonsRefundMarksAndALinkedCreditMoveOnlyTheLinesTheyName(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	views := Views{Ledger: viewOf(t, full.Path), Authored: scratch.Database.View()}
	defer views.Authored.Close()
	lines := lineIDsByName(t, scratch.Database)
	line := func(name string) budgetrules.LineID { return budgetrules.LineID(lines[name]) }
	cases := []struct {
		label string
		month brday.Month
		id    string
		want  budgetrules.Outcome
	}{
		{"a credit on Salário the person calls a refund of Viagens takes off Viagens alone", "2026-03", txSalary,
			budgetrules.Outcome{Disposition: budgetrules.Refund, Magnitude: 500_000,
				Lines: []budgetrules.LineAmount{{Line: line("Viagens"), Effect: budgetrules.TakesOff, Amount: 500_000}}}},
		{"a refund by the ledger's rule the person says is not one is another credit", "2026-03", txRefund,
			budgetrules.Outcome{Disposition: budgetrules.OtherCredit, Magnitude: 3_000}},
		{"a purchase of 10.00 with a credit of 50.00 linked counts nothing past what was paid", "2026-03", txFirstOfMarch,
			budgetrules.Outcome{Disposition: budgetrules.OnALine, Magnitude: 1_000, AppliedReimbursement: 1_000,
				Lines: []budgetrules.LineAmount{{Line: line("Feira"), Effect: budgetrules.Adds, Amount: 0}}}},
		{"the linked credit is the reimbursement and counts for no line, income included", "2026-01", txStale,
			budgetrules.Outcome{Disposition: budgetrules.Reimbursement, Magnitude: 5_000}},
	}
	for _, c := range cases {
		if got := outcomeOf(t, views, c.month, c.id); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v, want %+v", c.label, got, c.want)
		}
	}
}

func TestASplitAddsEachPartToItsLineAndLeavesTheRestUnassigned(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	views := Views{Ledger: viewOf(t, full.Path), Authored: scratch.Database.View()}
	defer views.Authored.Close()
	lines := lineIDsByName(t, scratch.Database)
	want := budgetrules.Outcome{
		Disposition: budgetrules.Split, Magnitude: 12_900, Unassigned: 2_900,
		Lines: []budgetrules.LineAmount{
			{Line: budgetrules.LineID(lines["Viagens"]), Effect: budgetrules.Adds, Amount: 6_000},
			{Line: budgetrules.LineID(lines["Saúde"]), Effect: budgetrules.Adds, Amount: 4_000},
		},
	}
	if got := outcomeOf(t, views, "2026-03", txInstalment); !reflect.DeepEqual(got, want) {
		t.Errorf("the split of 129.00 into 60.00, 40.00 and 20.00 on no line: %+v, want %+v", got, want)
	}
}
