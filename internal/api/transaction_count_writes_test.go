package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// txSplitPurchase is the purchase of 180.00 the stand-in's split vectors divide,
// added to the full ledger for these tests alone.
const txSplitPurchase = "tx-split-purchase"

func ledgerWithAPurchaseToSplit(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	full := newFullLedger(t)
	full.AddTransaction(ledgertest.Transaction{ID: txSplitPurchase, AccountID: accountItau, Date: utc(2026, time.March, 11, 12, 0),
		Description: "FARMACIA EXEMPLO", Amount: -180, Currency: "BRL", Type: "DEBIT", Raw: map[string]any{"operationType": "CARTAO"}})
	return full
}

// budgetToSplitIn is the invented budget with Transporte and Lazer among its
// variable lines, saved from 2026-02.
func budgetToSplitIn(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	document := authoredtest.InventedBudget()
	for i := range document {
		if document[i].Key == "var" {
			document[i].Lines = append(document[i].Lines, authoredtest.Line("Transporte", 30_000), authoredtest.Line("Lazer", 20_000))
		}
	}
	authoredtest.SaveBudget(t, scratch.Database, fixtureNow, "2026-02", document)
}

// countEnv is a router over a ledger and an app's database, with the ledger
// at hand for the month function.
type countEnv struct {
	*settingsEnv
	ledger *ledgertest.Fixture
}

func newCountEnv(t *testing.T, fixture *ledgertest.Fixture, state func(*testing.T, *authoredtest.Scratch)) *countEnv {
	t.Helper()
	env := &countEnv{settingsEnv: newSettingsEnv(t, fixture, fixtureNow), ledger: fixture}
	if state != nil {
		state(t, env.scratch)
	}
	return env
}

// putOnLine puts a listed transaction on a line of its month, as the person.
func (env *countEnv) putOnLine(id, line string) {
	env.t.Helper()
	err := inChange(env.t, env.scratch, env.ledger, func(writes Writes) error {
		target, err := targetNamedBy(env.t.Context(), writes, id)
		if err != nil {
			return err
		}
		lines, err := linesOfMonth(env.t.Context(), writes.Authored, monthOfDay(target.listed.Day))
		if err != nil {
			return err
		}
		lineID, found := lines.idOf(line)
		if !found {
			env.t.Fatalf("%s's month has no line %s", id, line)
		}
		return authoredtest.WriteState(env.t.Context(), writes.Recorder, target.key, authored.TransactionState{Line: &lineID, ClassifiedBy: authored.AuthorPerson})
	})
	if err != nil {
		env.t.Fatal(err)
	}
}

// outcome is what the month function decides for a transaction now.
func (env *countEnv) outcome(month brday.Month, id string) budgetrules.Outcome {
	env.t.Helper()
	views := Views{Ledger: viewOf(env.t, env.ledger.Path), Authored: env.scratch.Database.View()}
	defer views.Authored.Close()
	return outcomeOf(env.t, views, month, id)
}

func (env *countEnv) lineID(month brday.Month, name string) budgetrules.LineID {
	env.t.Helper()
	return budgetrules.LineID(budgetLineID(env.t, env.scratch.Database, month, name))
}

// countedDetail is what transactions.detail answers about what a transaction
// counts for. An amount is kept as the server wrote it.
type countedDetail struct {
	Line       *string         `json:"line"`
	Amount     json.RawMessage `json:"amount"`
	Split      bool            `json:"split"`
	SplitParts *[]struct {
		Line   *string         `json:"line"`
		Amount json.RawMessage `json:"amount"`
	} `json:"splitParts"`
	SplitRemaining json.RawMessage `json:"splitRemaining"`
	Excluded       bool            `json:"excluded"`
	CountsInTotals bool            `json:"countsInTotals"`
	Exclusion      *struct {
		Reason string `json:"reason"`
	} `json:"exclusion"`
	Refund     bool    `json:"refund"`
	RefundLine *string `json:"refundLine"`
	History    []struct {
		Text string `json:"text"`
		Day  string `json:"day"`
	} `json:"history"`
	Sections struct {
		Split bool `json:"split"`
	} `json:"sections"`
	Reimbursed bool `json:"reimbursed"`
}

func detailOf(t *testing.T, env *readEnv, id string) countedDetail {
	t.Helper()
	status, body := env.answer("transactions.detail", `["`+id+`"]`)
	var detail *countedDetail
	if err := json.Unmarshal(body, &detail); status != http.StatusOK || err != nil || detail == nil {
		t.Fatalf("the detail of %s answered %d %s", id, status, body)
	}
	return *detail
}

// partsOf is a detail's parts as "line amount" pairs, a part on no line as "-".
func partsOf(detail countedDetail) []string {
	if detail.SplitParts == nil {
		return nil
	}
	parts := []string{}
	for _, part := range *detail.SplitParts {
		line := "-"
		if part.Line != nil {
			line = *part.Line
		}
		parts = append(parts, line+" "+string(part.Amount))
	}
	return parts
}

func (env *countEnv) split(id, parts string) string {
	env.t.Helper()
	return env.change("transactions.setSplit", `["`+id+`",`+parts+`]`)
}

func TestASplitAnswersWhatItsPartsStillLackOrExceed(t *testing.T) {
	env := newCountEnv(t, ledgerWithAPurchaseToSplit(t), budgetToSplitIn)
	cases := []struct {
		parts     string
		want      []string
		remaining string
	}{
		{`[{"line":"Transporte","amount":100},{"line":null,"amount":0}]`, []string{"Transporte 100.00", "- 0.00"}, "80.00"},
		{`[{"line":"Transporte","amount":100},{"line":"Lazer","amount":90}]`, []string{"Transporte 100.00", "Lazer 90.00"}, "-10.00"},
		{`[{"line":"Transporte","amount":100},{"line":"Lazer","amount":80}]`, []string{"Transporte 100.00", "Lazer 80.00"}, "0.00"},
	}
	for _, c := range cases {
		env.split(txSplitPurchase, c.parts)
		detail := detailOf(t, env.readEnv, txSplitPurchase)
		if !detail.Split || !reflect.DeepEqual(partsOf(detail), c.want) || string(detail.SplitRemaining) != c.remaining {
			t.Errorf("%s: split %v, parts %v, remaining %s; want true, %v, %s", c.parts, detail.Split, partsOf(detail), detail.SplitRemaining, c.want, c.remaining)
		}
	}
}

// splitPartsKept is the parts the database keeps for a listed transaction.
func (env *countEnv) splitPartsKept(id string) []authored.SplitPart {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(env.t.Context(), []string{id})
	if err != nil {
		env.t.Fatal(err)
	}
	kept, err := view.Transactions(env.t.Context(), []authored.TxKey{keys[id]})
	if err != nil {
		env.t.Fatal(err)
	}
	return kept[keys[id]].Split
}

func TestNullAnEmptyListOrOnePartRemoveTheSplit(t *testing.T) {
	for _, removal := range []string{`null`, `[]`, `[{"line":"Transporte","amount":180}]`} {
		env := newCountEnv(t, ledgerWithAPurchaseToSplit(t), budgetToSplitIn)
		env.split(txSplitPurchase, `[{"line":"Transporte","amount":180},{"line":null,"amount":0}]`)
		env.split(txSplitPurchase, removal)
		detail := detailOf(t, env.readEnv, txSplitPurchase)
		if detail.Split || detail.SplitParts != nil || string(detail.SplitRemaining) != "0.00" {
			t.Errorf("%s: split %v, parts %v, remaining %s; want false, null, 0.00", removal, detail.Split, partsOf(detail), detail.SplitRemaining)
		}
		if kept := env.splitPartsKept(txSplitPurchase); len(kept) != 0 {
			t.Errorf("%s: the database keeps the parts %+v", removal, kept)
		}
	}
}

func TestEachSplitCountsItsPartsOnTheirLinesAndRemovingItPutsThePurchaseBack(t *testing.T) {
	env := newCountEnv(t, ledgerWithAPurchaseToSplit(t), budgetToSplitIn)
	env.putOnLine(txSplitPurchase, "Mercado")
	transport, health, leisure := env.lineID("2026-03", "Transporte"), env.lineID("2026-03", "Saúde"), env.lineID("2026-03", "Lazer")
	adds := func(line budgetrules.LineID, cents int64) budgetrules.LineAmount {
		return budgetrules.LineAmount{Line: line, Effect: budgetrules.Adds, Amount: money.Cents(cents)}
	}
	split := func(lines []budgetrules.LineAmount, unassigned int64) budgetrules.Outcome {
		return budgetrules.Outcome{Disposition: budgetrules.Split, Magnitude: 18_000, Lines: lines, Unassigned: money.Cents(unassigned)}
	}
	cases := []struct {
		parts string
		want  budgetrules.Outcome
	}{
		{`[{"line":"Transporte","amount":100},{"line":"Saúde","amount":80}]`, split([]budgetrules.LineAmount{adds(transport, 10_000), adds(health, 8_000)}, 0)},
		{`[{"line":"Transporte","amount":100},{"line":null,"amount":80}]`, split([]budgetrules.LineAmount{adds(transport, 10_000)}, 8_000)},
		{`[{"line":"Transporte","amount":100},{"line":"Lazer","amount":90}]`, split([]budgetrules.LineAmount{adds(transport, 10_000), adds(leisure, 9_000)}, -1_000)},
		{`null`, budgetrules.Outcome{Disposition: budgetrules.OnALine, Magnitude: 18_000,
			Lines: []budgetrules.LineAmount{adds(env.lineID("2026-03", "Mercado"), 18_000)}}},
	}
	for _, c := range cases {
		env.split(txSplitPurchase, c.parts)
		if got := env.outcome("2026-03", txSplitPurchase); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v, want %+v", c.parts, got, c.want)
		}
	}
	env.split(txPending, `[{"line":"Transporte","amount":18.5},{"line":null,"amount":0}]`)
	env.split(txPending, `null`)
	if got := env.outcome("2026-03", txPending); got.Disposition != budgetrules.Uncategorised || got.Unassigned != 1_850 {
		t.Errorf("a purchase on no line whose split was removed is %+v, want uncategorised with 18.50 unassigned", got)
	}
}

func TestANegativePartIsABadRequestAndChangesNothing(t *testing.T) {
	env := newCountEnv(t, ledgerWithAPurchaseToSplit(t), budgetToSplitIn)
	before := env.scratch.State(t)
	status, body := env.answer("transactions.setSplit", `["`+txSplitPurchase+`",[{"line":"Transporte","amount":180.01},{"line":null,"amount":-0.01}]]`)
	if status != http.StatusBadRequest || string(body) != `{"error":{"kind":"bad_request"}}` {
		t.Errorf("a part of -0.01 answered %d %s, want 400 bad_request", status, body)
	}
	if after := env.scratch.State(t); after != before {
		t.Errorf("the refused split changed the database:\n%s", differences(before, after))
	}
}

func TestTheFeesLineIsALineOfTheSplitAndTheRefundInAMonthWhoseBudgetHasOne(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	env.split(txPending, `[{"line":"Tarifas e juros","amount":18.5},{"line":null,"amount":0}]`)
	env.change("transactions.setRefundLine", `["tx-refund","Tarifas e juros"]`)
	if got := partsOf(detailOf(t, env.readEnv, txPending)); !reflect.DeepEqual(got, []string{"Tarifas e juros 18.50", "- 0.00"}) {
		t.Errorf("the split on the fees line is %v", got)
	}
	if got := detailOf(t, env.readEnv, txRefund).RefundLine; got == nil || *got != authored.FeesLineName {
		t.Errorf("the refund line is %v, want the fees line", got)
	}
}

// An unconverted purchase's amount is in its own currency, and the parts are
// typed against the amount the panel shows, so what they lack is in that
// currency too; the month counts nothing of it whatever its split.
func TestAnUnconvertedPurchasesSplitLacksInItsOwnCurrencyAndCountsForNothing(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	env.split(txUnconverted, `[{"line":"Mercado","amount":10},{"line":null,"amount":0}]`)
	if got := detailOf(t, env.readEnv, txUnconverted); string(got.Amount) != "-15.00" || string(got.SplitRemaining) != "5.00" {
		t.Errorf("the unconverted purchase of %s answers %s still to split, want 5.00", got.Amount, got.SplitRemaining)
	}
	if got := env.outcome("2026-03", txUnconverted); !reflect.DeepEqual(got, budgetrules.Outcome{Disposition: budgetrules.Unconverted}) {
		t.Errorf("the split unconverted purchase counts %+v, want nothing", got)
	}
}

// A credit's split and a debit's refund mark are kept, as the stand-in keeps
// them, and change nothing the month counts.
func TestASplitOnACreditAndARefundMarkOnADebitAreKeptAndCountForNothing(t *testing.T) {
	env := newCountEnv(t, newFullLedger(t), budgetState())
	before := map[string]budgetrules.Outcome{txRefund: env.outcome("2026-03", txRefund), txPending: env.outcome("2026-03", txPending)}
	env.split(txRefund, `[{"line":"Mercado","amount":20},{"line":null,"amount":10}]`)
	env.change("transactions.setRefund", `["tx-pending",true]`)
	env.change("transactions.setRefundLine", `["tx-pending","Mercado"]`)
	if !detailOf(t, env.readEnv, txRefund).Split || !detailOf(t, env.readEnv, txPending).Refund {
		t.Error("the split of the credit or the mark of the debit was not kept")
	}
	for id, want := range before {
		if got := env.outcome("2026-03", id); !reflect.DeepEqual(got, want) {
			t.Errorf("%s counts %+v after the writes, %+v before", id, got, want)
		}
	}
}

// A selection is found from one reading of the ledger's rows however many ids
// and months it holds, each id once in the order first named and as the single
// lookup finds it; an id the list does not show refuses it whole after that
// same one reading.
func TestASelectionReadsTheLedgersRowsOncePerCall(t *testing.T) {
	ctx := t.Context()
	snapshot := viewOf(t, newFullLedger(t).Path)
	cases := []struct {
		label string
		ids   []string
		want  []string
	}{
		{"three ids in two months, one named twice", []string{txPending, txLastOfFebruary, txPending, txBoleto}, []string{txPending, txLastOfFebruary, txBoleto}},
		{"an id the list does not show", []string{txPending, "no-such-transaction", txLastOfFebruary}, nil},
	}
	for _, c := range cases {
		readings := 0
		counted := func() ([]ledger.Transaction, error) {
			readings++
			return snapshot.CleanTransactions(ctx, wholeLedger)
		}
		rows, err := selectionAmong(ctx, snapshot, c.ids, counted)
		if readings != 1 {
			t.Errorf("%s: the ledger's rows were read %d times, want once", c.label, readings)
		}
		if c.want == nil {
			if kind, reason := classify(err); kind != kindRefused || reason != reasonNotFound || rows != nil {
				t.Errorf("%s: answered %d rows and %v, want the refusal not_found and none", c.label, len(rows), err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var found []string
		months := map[brday.Month]bool{}
		for _, row := range rows {
			found = append(found, row.listed.ID)
			months[monthOfDay(row.listed.Day)] = true
			single, listed, err := findListed(ctx, snapshot, row.listed.ID)
			if err != nil || !listed || !reflect.DeepEqual(single, row) {
				t.Errorf("%s: %s is not found as the single lookup finds it", c.label, row.listed.ID)
			}
		}
		if !slices.Equal(found, c.want) || len(months) != 2 {
			t.Errorf("%s: found %v in %d months, want %v in two", c.label, found, len(months), c.want)
		}
	}
}

// The index is not journaled, so a bulk write that indexed the ids it found
// before reaching one the list does not show would leave their rows behind
// under a caller that keeps the change. The snapshot is recorded as taken in,
// so indexing would succeed and only finding every id first keeps it clean.
func TestEveryBulkWriteFindsItsWholeSelectionBeforeItIndexesAny(t *testing.T) {
	full := newFullLedger(t)
	handlers := []struct {
		method string
		write  WriteHandler
	}{
		{"transactions.addTagToMany", addTagToTransactions},
		{"transactions.excludeMany", excludeMany},
		{"transactions.setLineForMany", setLineForMany},
	}
	for _, h := range handlers {
		scratch := authoredtest.Open(t, fixtureClock)
		budgetState()(t, scratch)
		recordRunTakenIn(t, viewOf(t, full.Path), scratch.Database)
		before := scratch.Everything(t)
		var refusal error
		err := inChange(t, scratch, full, func(writes Writes) error {
			args := rawArguments(t, []string{txPending, txLastOfFebruary, "no-such-transaction"}, "Mercado")
			_, refusal = h.write(t.Context(), args, writes)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if kind, reason := classify(refusal); kind != kindRefused || reason != reasonNotFound {
			t.Errorf("%s answered %v, want the refusal not_found", h.method, refusal)
		}
		if after := scratch.Everything(t); after != before {
			t.Errorf("%s wrote before it refused:\n%s", h.method, differences(before, after))
		}
	}
}
