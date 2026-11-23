package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/budgetrules"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// The rows the reimbursement tests add to the full ledger, beside its PIX from
// a company (tx-salary, March) and from a friend (tx-stale, January, another
// account, before the tracking start). Every name and figure is invented.
const (
	txDinner     = "tx-dinner"
	txDinnerCent = "tx-dinner-cent"
	txBooks      = "tx-books"
	txLunch      = "tx-lunch"

	txPixAna      = "tx-pix-ana"
	txPixWhole    = "tx-pix-whole"
	txPixMore     = "tx-pix-more"
	txPixLate     = "tx-pix-z-late"
	txPixEarly    = "tx-pix-a-early"
	txPixIncome   = "tx-pix-income"
	txPixOff      = "tx-pix-off"
	txPixRefund   = "tx-pix-refund"
	txPixExcluded = "tx-pix-excluded"
	txPixLinked   = "tx-pix-linked"
	txPixOwn      = "tx-pix-own"
	txPayroll     = "tx-payroll"

	anaName = "Ana Exemplo"
)

// reimbursementLedger is the full ledger with purchases of 180.00, 180.01,
// 123.45 and 60.00, and received PIX of every kind the candidates' filter
// tells apart. A row named in without is left out, as a later snapshot that
// no longer lists it.
func reimbursementLedger(t *testing.T, without ...string) *ledgertest.Fixture {
	t.Helper()
	full := newFullLedger(t)
	add := func(id string, at time.Time, description string, amount float64, kind string, raw map[string]any) {
		if !slices.Contains(without, id) {
			full.AddTransaction(ledgertest.Transaction{ID: id, AccountID: accountItau, Date: at, Description: description,
				Amount: amount, Currency: "BRL", Type: kind, Raw: raw})
		}
	}
	purchase := func(id string, day int, description string, amount float64) {
		add(id, utc(2026, time.March, day, 12, 0), description, -amount, "DEBIT", map[string]any{"operationType": "CARTAO"})
	}
	received := func(id string, at time.Time, name, document string, amount float64) {
		add(id, at, "PIX RECEBIDO "+name, amount, "CREDIT",
			map[string]any{"operationType": "PIX", "paymentData": payment("PIX", party(name, document), nil)})
	}
	purchase(txDinner, 11, "RESTAURANTE EXEMPLO", 180)
	purchase(txDinnerCent, 11, "RESTAURANTE EXEMPLO", 180.01)
	purchase(txBooks, 11, "LIVRARIA EXEMPLO", 123.45)
	purchase(txLunch, 15, "RESTAURANTE EXEMPLO", 60)
	received(txPixAna, utc(2026, time.March, 12, 12, 0), anaName, "11122233355", 150)
	received(txPixWhole, utc(2026, time.March, 14, 12, 0), "Bruno Exemplo", "11122233366", 180)
	received(txPixMore, utc(2026, time.March, 16, 12, 0), "Carla Exemplo", "11122233377", 200)
	received(txPixLate, utc(2026, time.March, 13, 18, 0), "Davi Exemplo", "11122233388", 25)
	received(txPixEarly, utc(2026, time.March, 13, 9, 0), "Eva Exemplo", "11122233399", 35)
	received(txPixIncome, utc(2026, time.March, 2, 12, 0), "Firma Exemplo", "33344455000166", 300)
	received(txPixOff, utc(2026, time.February, 20, 12, 0), "Gil Exemplo", "22233344411", 40)
	received(txPixRefund, utc(2026, time.March, 7, 12, 0), "Hugo Exemplo", "22233344422", 45)
	received(txPixExcluded, utc(2026, time.March, 8, 12, 0), "Iris Exemplo", "22233344433", 55)
	received(txPixLinked, utc(2026, time.March, 9, 12, 0), "Jonas Exemplo", "22233344444", 65)
	received(txPixOwn, utc(2026, time.March, 4, 12, 0), "Pessoa Exemplo", "98765432100", 500)
	add(txPayroll, utc(2026, time.March, 6, 12, 0), "FOLHA PAGAMENTO EMPRESA EXEMPLO", 4200, "CREDIT",
		map[string]any{"operationType": "FOLHA_PAGAMENTO"})
	return full
}

// reimbursementView is what the list and the detail answer about a
// transaction's reimbursement, every amount as the server wrote it.
type reimbursementView struct {
	ID                  string          `json:"id"`
	Amount              json.RawMessage `json:"amount"`
	EffectiveAmount     json.RawMessage `json:"effectiveAmount"`
	Reimbursed          bool            `json:"reimbursed"`
	ReimbursementIncome bool            `json:"reimbursementIncome"`
	Line                *string         `json:"line"`
	Pending             json.RawMessage `json:"pending"`
	Reimbursement       json.RawMessage `json:"reimbursement"`
	Candidates          []struct {
		TransactionID string          `json:"transactionId"`
		Name          string          `json:"name"`
		Day           string          `json:"day"`
		Amount        json.RawMessage `json:"amount"`
	} `json:"reimbursementCandidates"`
	History []struct {
		Text string `json:"text"`
	} `json:"history"`
}

func compactJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	return compact.String()
}

func reimbursementDetailOf(t *testing.T, env *readEnv, id string) reimbursementView {
	t.Helper()
	status, body := env.answer("transactions.detail", `["`+id+`"]`)
	var detail *reimbursementView
	if err := json.Unmarshal(body, &detail); status != http.StatusOK || err != nil || detail == nil {
		t.Fatalf("the detail of %s answered %d %s", id, status, body)
	}
	return *detail
}

// reimbursementRowOf is what the list of the transaction's month answers for it.
func reimbursementRowOf(t *testing.T, env *readEnv, month, id string) reimbursementView {
	t.Helper()
	status, body := env.answer("transactions.list", listRaw(month, "month"))
	var rows []reimbursementView
	if err := json.Unmarshal(body, &rows); status != http.StatusOK || err != nil {
		t.Fatalf("transactions.list %s answered %d %s", month, status, body)
	}
	for _, row := range rows {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("%s is not listed in %s", id, month)
	return reimbursementView{}
}

func reimbursementCandidateIDs(view reimbursementView) []string {
	ids := []string{}
	for _, candidate := range view.Candidates {
		ids = append(ids, candidate.TransactionID)
	}
	return ids
}

func newReimbursementEnv(t *testing.T, fixture *ledgertest.Fixture) *countEnv {
	t.Helper()
	return newCountEnv(t, fixture, budgetToSplitIn)
}

// reimbursementsToSeed marks the foreign purchase reimbursable at half of what
// was paid, with no credit linked, beside the coffee of March that the
// friend's PIX already pays back.
func (a authoring) reimbursementsToSeed() error {
	foreign, err := a.key(txForeign)
	if err != nil {
		return err
	}
	return a.writes.Recorder.MarkReimbursable(a.ctx, foreign, money.Cents(5_693))
}

// linkThroughTheStore marks purchase reimbursable at expected and links credit
// to it at its listed amount, in one change, as the link keeps them.
func (env *countEnv) linkThroughTheStore(purchase, credit string, expected money.Cents) {
	env.t.Helper()
	err := inChange(env.t, env.scratch, env.ledger, func(writes Writes) error {
		ctx := env.t.Context()
		bought, err := countedTarget(ctx, writes, purchase)
		if err != nil {
			return err
		}
		paid, err := countedTarget(ctx, writes, credit)
		if err != nil {
			return err
		}
		if err := writes.Recorder.MarkReimbursable(ctx, bought.key, expected); err != nil {
			return err
		}
		_, err = writes.Recorder.LinkReimbursement(ctx, bought.key, paid.key, paid.listed.Amount)
		return err
	})
	if err != nil {
		env.t.Fatal(err)
	}
}

// The candidates of a purchase are the received PIX of the whole ledger that
// may pay it back, in the list's order: by day, newest first, and within a
// day the later instant first.
func TestTheCandidatesAreEveryReceivedPIXOfTheLedgerThatMayPayThePurchaseBackNewestFirst(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	env.putOnLine(txPixIncome, "Salário")
	env.change("transactions.setRefund", `["`+txPixRefund+`",true]`)
	env.change("transactions.setExclusionReason", `["`+txPixExcluded+`","Compra da empresa"]`)
	env.change("transactions.setExcluded", `["`+txPixOff+`",true]`)
	env.linkThroughTheStore(txLunch, txPixLinked, 3_000)

	want := []string{
		txPixMore, txPixWhole, txPixLate, txPixEarly, txPixAna, txSalary, txPixIncome, txPixOff, txStale,
	}
	if got := reimbursementCandidateIDs(reimbursementDetailOf(t, env.readEnv, txDinner)); !slices.Equal(got, want) {
		t.Errorf("the dinner's candidates are %v, want %v", got, want)
	}
	for _, id := range []string{txPayroll, txPixOwn, txPixRefund, txPixExcluded, txRefund, txPixLinked} {
		if slices.Contains(reimbursementCandidateIDs(reimbursementDetailOf(t, env.readEnv, txDinner)), id) {
			t.Errorf("%s is a candidate", id)
		}
	}
	if got := reimbursementCandidateIDs(reimbursementDetailOf(t, env.readEnv, txLunch)); !slices.Contains(got, txPixLinked) {
		t.Errorf("the lunch's own credit is not among its candidates %v", got)
	}
	keys := env.keysOf(txStale)
	if len(keys) != 0 {
		t.Errorf("the friend's PIX of January was indexed by reading the candidates: %v", keys)
	}
}

// keysOf is the key of each id the index has met.
func (env *countEnv) keysOf(ids ...string) map[string]authored.TxKey {
	env.t.Helper()
	view := env.scratch.Database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(env.t.Context(), ids)
	if err != nil {
		env.t.Fatal(err)
	}
	return keys
}

// stateOf is what the database keeps about a listed transaction it has indexed.
func (env *countEnv) stateOf(id string) authored.TransactionState {
	env.t.Helper()
	key, found := env.keysOf(id)[id]
	if !found {
		env.t.Fatalf("%s was never indexed", id)
	}
	view := env.scratch.Database.View()
	defer view.Close()
	kept, err := view.Transactions(env.t.Context(), []authored.TxKey{key})
	if err != nil {
		env.t.Fatal(err)
	}
	return kept[key].State
}

func TestTwoCandidatesOfOneDayAreInTheListsOrderTheLaterInstantFirst(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	got := reimbursementCandidateIDs(reimbursementDetailOf(t, env.readEnv, txDinner))
	late, early := slices.Index(got, txPixLate), slices.Index(got, txPixEarly)
	if late < 0 || early < 0 || early != late+1 {
		t.Errorf("the PIX of 18:00 is at %d and the one of 09:00 at %d of %v, want them side by side, the later first", late, early, got)
	}
}

func TestACandidateIsAnsweredWithItsListedIDItsShownNameItsDayAndItsAmount(t *testing.T) {
	env := newReimbursementEnv(t, reimbursementLedger(t))
	for _, candidate := range reimbursementDetailOf(t, env.readEnv, txDinner).Candidates {
		if candidate.TransactionID != txPixAna {
			continue
		}
		if candidate.Name != anaName || candidate.Day != "2026-03-12" || string(candidate.Amount) != "150.00" {
			t.Errorf("Ana's PIX is answered as %+v", candidate)
		}
		return
	}
	t.Error("Ana's PIX is not a candidate")
}

// Each condition of the filter, on a row built for it: the ledger's rules mark
// only card credits as refunds, so a received PIX the ledger calls a refund is
// a row no snapshot can hold, and the effective mark is checked here.
func TestTheFilterLeavesOutWhatTheLedgerOrThePersonSaysIsNotSomeonesMoneyComingIn(t *testing.T) {
	pix := txrules.Transaction{ID: "tx-x", Amount: 5_000, Kind: txrules.KindPix, Posted: true}
	with := func(change func(*txrules.Transaction)) txrules.Transaction {
		row := pix
		change(&row)
		return row
	}
	cases := []struct {
		label           string
		credit          txrules.Transaction
		state           authored.TransactionState
		linkedElsewhere bool
		want            bool
	}{
		{"a received PIX", pix, authored.TransactionState{}, false, true},
		{"a debit", with(func(row *txrules.Transaction) { row.Amount = -5_000 }), authored.TransactionState{}, false, false},
		{"a credit by card", with(func(row *txrules.Transaction) { row.Kind = txrules.KindCard }), authored.TransactionState{}, false, false},
		{"an own transfer", with(func(row *txrules.Transaction) { row.Movement = txrules.OwnTransfer }), authored.TransactionState{}, false, false},
		{"an unconverted one", with(func(row *txrules.Transaction) { row.Unconverted = true }), authored.TransactionState{}, false, false},
		{"one the ledger calls a refund", with(func(row *txrules.Transaction) { row.Refund = true }), authored.TransactionState{}, false, false},
		{"one the ledger calls a refund and the person does not", with(func(row *txrules.Transaction) { row.Refund = true }),
			authored.TransactionState{Refund: authoredtest.Some(false)}, false, true},
		{"one the person calls a refund", pix, authored.TransactionState{Refund: authoredtest.Some(true)}, false, false},
		{"one excluded with a reason", pix, authored.TransactionState{Exclusion: authoredtest.Some("Empréstimo")}, false, false},
		{"one switched off without a reason", pix, authored.TransactionState{Exclusion: authoredtest.Some("")}, false, true},
		{"one linked to another purchase", pix, authored.TransactionState{}, true, false},
	}
	for _, c := range cases {
		if got := isCandidate(c.credit, c.state, c.linkedElsewhere); got != c.want {
			t.Errorf("%s: a candidate %v, want %v", c.label, got, c.want)
		}
	}
}

// snapshotOver is a router over another snapshot of the ledger and the same
// app's database.
func (env *countEnv) snapshotOver(t *testing.T, fixture *ledgertest.Fixture) *readEnv {
	t.Helper()
	return newReadEnvOver(t, fixture, env.scratch.Database)
}

func outcomeOver(t *testing.T, scratch *authoredtest.Scratch, fixture *ledgertest.Fixture, id string) budgetrules.Outcome {
	t.Helper()
	views := Views{Ledger: viewOf(t, fixture.Path), Authored: scratch.Database.View()}
	defer views.Authored.Close()
	return outcomeOf(t, views, "2026-03", id)
}

// A link applies only while the ledger lists its credit: over a snapshot that
// stopped listing the PIX the dinner counts whole, though the link stays
// kept, and over one that lists it again the link applies with no write.
func TestALinkAppliesOnlyWhileTheLedgerListsItsCredit(t *testing.T) {
	listed, unlisted, listedAgain := reimbursementLedger(t), reimbursementLedger(t, txPixAna), reimbursementLedger(t)
	env := newReimbursementEnv(t, listed)
	env.putOnLine(txDinner, "Mercado")
	env.linkThroughTheStore(txDinner, txPixAna, 9_000)
	stored, journal := env.stateOf(txDinner), env.scratch.Journal(t)
	snapshots := []struct {
		label           string
		fixture         *ledgertest.Fixture
		effective, link string
		reimbursed      bool
		onTheLine       money.Cents
		disposition     budgetrules.Disposition
	}{
		{"listed", listed, "-30.00", `"transactionId":"tx-pix-ana"`, true, 3_000, budgetrules.Reimbursement},
		{"no longer listed", unlisted, "-180.00", `"link":null`, false, 18_000, 0},
		{"listed again", listedAgain, "-30.00", `"transactionId":"tx-pix-ana"`, true, 3_000, budgetrules.Reimbursement},
	}
	for _, snapshot := range snapshots {
		reader := env.snapshotOver(t, snapshot.fixture)
		detail := reimbursementDetailOf(t, reader, txDinner)
		reimbursement := compactJSON(t, detail.Reimbursement)
		if string(detail.EffectiveAmount) != snapshot.effective || detail.Reimbursed != snapshot.reimbursed ||
			!strings.Contains(reimbursement, snapshot.link) || !strings.HasPrefix(reimbursement, `{"expected":90.00,`) {
			t.Errorf("%s: the dinner answers effective %s, reimbursed %v, %s", snapshot.label, detail.EffectiveAmount, detail.Reimbursed, reimbursement)
		}
		if outcome := outcomeOver(t, env.scratch, snapshot.fixture, txDinner); outcome.Lines[0].Amount != snapshot.onTheLine {
			t.Errorf("%s: the dinner's line receives %d, want %d", snapshot.label, outcome.Lines[0].Amount, snapshot.onTheLine)
		}
		if snapshot.disposition != 0 {
			if credit := outcomeOver(t, env.scratch, snapshot.fixture, txPixAna); credit.Disposition != snapshot.disposition {
				t.Errorf("%s: the PIX is %v", snapshot.label, credit.Disposition)
			}
		}
		for _, problem := range monthInvariants(t, reader, env.scratch, snapshot.fixture.Path) {
			t.Errorf("%s: %s", snapshot.label, problem)
		}
		if state := env.stateOf(txDinner); *state.Credit != *stored.Credit || *state.CreditCents != *stored.CreditCents {
			t.Errorf("%s: the stored link is %+v, was %+v", snapshot.label, state, stored)
		}
		if env.scratch.Journal(t) != journal {
			t.Errorf("%s: reading the snapshot recorded a change", snapshot.label)
		}
	}

}

// receivedOn is what budget.view answers that a line of the income group
// received in month.
func receivedOn(t *testing.T, env *readEnv, month, line string) money.Cents {
	t.Helper()
	status, body := env.answer("budget.view", `["`+month+`"]`)
	if status != http.StatusOK {
		t.Fatalf("budget.view %s answered %d %s", month, status, body)
	}
	var view viewAnswer
	decodeKeepingNumbers(t, body, &view)
	var cell struct {
		Amount json.Number `json:"amount"`
	}
	decodeKeepingNumbers(t, view.line(t, "rec", line).Cells[1], &cell)
	cents, err := ParseReais([]byte(cell.Amount))
	if err != nil {
		t.Fatal(err)
	}
	return cents
}

// linkedSnapshot is what the list, the detail and the month function answer
// about a credit linked to the dinner over one snapshot, and the dinner's
// effective amount when the snapshot lists it.
type linkedSnapshot struct {
	row, detail reimbursementView
	outcome     budgetrules.Outcome
	dinner      string
}

func (env *countEnv) linkedOver(t *testing.T, fixture *ledgertest.Fixture, dinnerListed bool, month brday.Month, credit string) (linkedSnapshot, *readEnv) {
	t.Helper()
	reader := env.snapshotOver(t, fixture)
	views := Views{Ledger: viewOf(t, fixture.Path), Authored: env.scratch.Database.View()}
	defer views.Authored.Close()
	seen := linkedSnapshot{
		row: reimbursementRowOf(t, reader, string(month), credit), detail: reimbursementDetailOf(t, reader, credit),
		outcome: outcomeOf(t, views, month, credit),
	}
	if dinnerListed {
		seen.dinner = string(reimbursementDetailOf(t, reader, txDinner).EffectiveAmount)
	}
	return seen, reader
}

// countedSomewhere is what the invariants of a month cannot see, since a
// reimbursement passes both: the credit's amount is received by a line of the
// income group, or shown without a line for the person to place.
func (seen linkedSnapshot) countedSomewhere() bool {
	lines := seen.outcome.Lines
	switch seen.outcome.Disposition {
	case budgetrules.Income:
		return len(lines) == 1 && lines[0].Effect == budgetrules.Received && lines[0].Amount == seen.outcome.Magnitude && seen.row.Line != nil
	case budgetrules.OtherCredit:
		return len(lines) == 0 && seen.row.Line == nil
	}
	return false
}

func linkText(state authored.TransactionState) string {
	if state.Credit == nil || state.CreditCents == nil {
		return "no link"
	}
	return fmt.Sprintf("credit %d of %d cents", *state.Credit, *state.CreditCents)
}

// expectLinkKept fails unless the dinner's stored link is the one it had and
// the journal has not moved: a read never writes the link.
func (env *countEnv) expectLinkKept(t *testing.T, label string, stored authored.TransactionState, journal string) {
	t.Helper()
	if now, before := linkText(env.stateOf(txDinner)), linkText(stored); now != before {
		t.Errorf("%s: the stored link is %s, was %s", label, now, before)
	}
	if env.scratch.Journal(t) != journal {
		t.Errorf("%s: reading the snapshot recorded a change", label)
	}
}

// A credit is a purchase's reimbursement only while the ledger lists the
// purchase: over a snapshot that stopped listing the dinner its PIX is an
// ordinary credit, shown without a line, though the link stays kept, and over
// one that lists the dinner again the PIX pays it back with no write.
func TestALinkedCreditIsAReimbursementOnlyWhileTheLedgerListsItsPurchase(t *testing.T) {
	listed, unlisted, listedAgain := reimbursementLedger(t), reimbursementLedger(t, txDinner), reimbursementLedger(t)
	env := newReimbursementEnv(t, listed)
	env.putOnLine(txDinner, "Mercado")
	env.linkThroughTheStore(txDinner, txPixAna, 9_000)
	stored, journal := env.stateOf(txDinner), env.scratch.Journal(t)
	reimbursement := budgetrules.Outcome{Disposition: budgetrules.Reimbursement, Magnitude: 15_000}
	snapshots := []struct {
		label        string
		fixture      *ledgertest.Fixture
		dinnerListed bool
		outcome      budgetrules.Outcome
		dinner       string
	}{
		{"listed", listed, true, reimbursement, "-30.00"},
		{"no longer listed", unlisted, false, budgetrules.Outcome{Disposition: budgetrules.OtherCredit, Magnitude: 15_000}, ""},
		{"listed again", listedAgain, true, reimbursement, "-30.00"},
	}
	for _, snapshot := range snapshots {
		seen, reader := env.linkedOver(t, snapshot.fixture, snapshot.dinnerListed, "2026-03", txPixAna)
		if seen.row.ReimbursementIncome != snapshot.dinnerListed || seen.detail.ReimbursementIncome != snapshot.dinnerListed {
			t.Errorf("%s: the PIX answers reimbursementIncome %v in the list and %v in its detail, want %v",
				snapshot.label, seen.row.ReimbursementIncome, seen.detail.ReimbursementIncome, snapshot.dinnerListed)
		}
		if !reflect.DeepEqual(seen.outcome, snapshot.outcome) {
			t.Errorf("%s: the PIX is %+v, want %+v", snapshot.label, seen.outcome, snapshot.outcome)
		}
		if seen.dinner != snapshot.dinner {
			t.Errorf("%s: the dinner's effective amount is %q, want %q", snapshot.label, seen.dinner, snapshot.dinner)
		}
		if !snapshot.dinnerListed && !seen.countedSomewhere() {
			t.Errorf("%s: the PIX of 150.00 counts nowhere: %+v, shown on a line %v", snapshot.label, seen.outcome, seen.row.Line != nil)
		}
		for _, problem := range monthInvariants(t, reader, env.scratch, snapshot.fixture.Path) {
			t.Errorf("%s: %s", snapshot.label, problem)
		}
		env.expectLinkKept(t, snapshot.label, stored, journal)
	}
}

// A linked PIX the person had put on an income line counts as that line's
// income, to the cent, while the ledger does not list its purchase.
func TestALinkedCreditOnAnIncomeLineIsItsIncomeWhileItsPurchaseIsNotListed(t *testing.T) {
	listed, unlisted, listedAgain := reimbursementLedger(t), reimbursementLedger(t, txDinner), reimbursementLedger(t)
	env := newReimbursementEnv(t, listed)
	env.putOnLine(txPixAna, "Salário")
	env.linkThroughTheStore(txDinner, txPixAna, 9_000)
	stored, journal := env.stateOf(txDinner), env.scratch.Journal(t)
	salary := env.lineID("2026-03", "Salário")
	income := budgetrules.Outcome{Disposition: budgetrules.Income, Magnitude: 15_000,
		Lines: []budgetrules.LineAmount{{Line: salary, Effect: budgetrules.Received, Amount: 15_000}}}
	reimbursement := budgetrules.Outcome{Disposition: budgetrules.Reimbursement, Magnitude: 15_000}
	snapshots := []struct {
		label        string
		fixture      *ledgertest.Fixture
		dinnerListed bool
		outcome      budgetrules.Outcome
	}{
		{"listed", listed, true, reimbursement},
		{"no longer listed", unlisted, false, income},
		{"listed again", listedAgain, true, reimbursement},
	}
	received := map[string]money.Cents{}
	for _, snapshot := range snapshots {
		seen, reader := env.linkedOver(t, snapshot.fixture, snapshot.dinnerListed, "2026-03", txPixAna)
		if !reflect.DeepEqual(seen.outcome, snapshot.outcome) {
			t.Errorf("%s: the PIX is %+v, want %+v", snapshot.label, seen.outcome, snapshot.outcome)
		}
		if !snapshot.dinnerListed && (seen.row.ReimbursementIncome || !seen.countedSomewhere()) {
			t.Errorf("%s: the PIX counts nowhere: %+v, shown on a line %v, reimbursementIncome %v", snapshot.label, seen.outcome, seen.row.Line != nil, seen.row.ReimbursementIncome)
		}
		received[snapshot.label] = receivedOn(t, reader, "2026-03", "Salário")
		for _, problem := range monthInvariants(t, reader, env.scratch, snapshot.fixture.Path) {
			t.Errorf("%s: %s", snapshot.label, problem)
		}
		env.expectLinkKept(t, snapshot.label, stored, journal)
	}
	if received["no longer listed"] != received["listed"]+15_000 || received["listed again"] != received["listed"] {
		t.Errorf("Salário received %v, want 150.00 more while the dinner is not listed", received)
	}
}

// A credit is judged by the whole ledger, not by the rows of the answer: the
// PIX of February paying the dinner of March is its reimbursement in
// February's figures, and an ordinary credit there once the dinner is gone.
func TestALinkedCreditIsJudgedByTheWholeLedgerNotByTheMonthAsked(t *testing.T) {
	listed, unlisted := reimbursementLedger(t), reimbursementLedger(t, txDinner)
	env := newReimbursementEnv(t, listed)
	env.linkThroughTheStore(txDinner, txPixOff, 2_000)
	seen, _ := env.linkedOver(t, listed, true, "2026-02", txPixOff)
	reimbursement := budgetrules.Outcome{Disposition: budgetrules.Reimbursement, Magnitude: 4_000}
	if !reflect.DeepEqual(seen.outcome, reimbursement) || !seen.row.ReimbursementIncome || !seen.detail.ReimbursementIncome {
		t.Errorf("with the dinner listed in March, February's PIX is %+v, reimbursementIncome %v and %v",
			seen.outcome, seen.row.ReimbursementIncome, seen.detail.ReimbursementIncome)
	}
	if seen.dinner != "-140.00" {
		t.Errorf("the dinner's effective amount is %s, want -140.00", seen.dinner)
	}
	gone, _ := env.linkedOver(t, unlisted, false, "2026-02", txPixOff)
	if gone.outcome.Disposition != budgetrules.OtherCredit || gone.row.ReimbursementIncome {
		t.Errorf("with the dinner gone, February's PIX is %+v, reimbursementIncome %v", gone.outcome, gone.row.ReimbursementIncome)
	}
}

// Which listed credits of an answer are reimbursements, and how many times the
// whole ledger is read to decide it: once when a stored purchase names a
// credit of the answer, wherever that purchase is, and never otherwise. A
// reading that fails fails the answer: a credit taken for an ordinary one
// while its purchase elsewhere still has its amount taken off counts twice.
func TestACreditIsAReimbursementWhileTheWholeLedgerListsThePurchaseThatNamesIt(t *testing.T) {
	listed, unlisted := reimbursementLedger(t), reimbursementLedger(t, txDinner)
	linked := newReimbursementEnv(t, listed)
	linked.linkThroughTheStore(txDinner, txPixAna, 9_000)
	linked.linkThroughTheStore(txLunch, txPixOff, 2_000)
	unnamed := newReimbursementEnv(t, listed)
	unnamed.putOnLine(txPixAna, "Salário")
	unreadable := errors.New("the whole ledger could not be read")
	cases := []struct {
		label    string
		env      *countEnv
		fixture  *ledgertest.Fixture
		month    brday.Month
		credit   string
		want     bool
		readings int
		failure  error
	}{
		{"the purchase listed in the answer", linked, listed, "2026-03", txPixAna, true, 1, nil},
		{"the purchase listed in a month the answer does not cover", linked, listed, "2026-02", txPixOff, true, 1, nil},
		{"the purchase not listed", linked, unlisted, "2026-03", txPixAna, false, 1, nil},
		{"no stored purchase names a credit of the answer", unnamed, listed, "2026-03", txPixAna, false, 0, nil},
		{"the reading fails while a stored purchase names a credit of the answer", linked, listed, "2026-02", txPixOff, false, 1, unreadable},
	}
	for _, c := range cases {
		ctx := t.Context()
		views := Views{Ledger: viewOf(t, c.fixture.Path), Authored: c.env.scratch.Database.View()}
		disposed, err := disposeMonth(ctx, views, c.month)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(disposed.listed))
		for _, row := range disposed.listed {
			ids = append(ids, row.ID)
		}
		keys, err := views.Authored.TransactionKeys(ctx, ids)
		if err != nil {
			t.Fatal(err)
		}
		readings := 0
		counted := func() (wholeListing, error) {
			readings++
			if c.failure != nil {
				return wholeListing{}, c.failure
			}
			return readWholeListing(ctx, views)
		}
		reimbursing, err := linkedCreditsOf(ctx, views.Authored, disposed.listed, keys, counted)
		views.Authored.Close()
		if c.failure != nil {
			if !errors.Is(err, c.failure) || reimbursing != nil || readings != c.readings {
				t.Errorf("%s: answered %v and %v after %d readings, want the reading's error and no map", c.label, reimbursing, err, readings)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		key, indexed := keys[c.credit]
		if !indexed || reimbursing[key] != c.want || readings != c.readings {
			t.Errorf("%s: the credit is a reimbursement %v (indexed %v) after %d readings of the whole ledger, want %v after %d",
				c.label, reimbursing[key], indexed, readings, c.want, c.readings)
		}
	}
}
