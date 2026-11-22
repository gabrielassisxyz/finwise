package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"testing"

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
	for _, month := range []brday.Month{"2026-03", "2026-02"} {
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
			txUnconverted: budgetrules.Unconverted, txRefund: budgetrules.Refund, txPixOut: budgetrules.Excluded,
			txForeign: budgetrules.OnALine, txBoleto: budgetrules.Waiting, txOwnTransfer: budgetrules.Movement,
			txInstalment: budgetrules.Split, txSalary: budgetrules.Income, txFee: budgetrules.Fee,
			txFirstOfMarch: budgetrules.OnALine,
		},
		"2026-02": {txLastOfFebruary: budgetrules.Uncategorised, txMonthEnd: budgetrules.OnALine},
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
		want := []budgetrules.LineAmount{{Line: budgetrules.LineID(feesLineID), Effect: budgetrules.Adds, Amount: 2_500}}
		if outcome.Disposition != budgetrules.Fee || !reflect.DeepEqual(outcome.Lines, want) {
			t.Errorf("the fee is %v on %v, want a fee adding 25.00 to the fees line", outcome.Disposition, outcome.Lines)
		}
		return
	}
	t.Fatal("the fee is not among March's transactions")
}
