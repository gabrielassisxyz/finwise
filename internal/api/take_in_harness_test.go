package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// Everything here is invented: the accounts, the shops and every amount. A test
// writes each night's snapshot beside the ledger's path and renames it over, as
// the collector does, and the real ingest feeds the app's database.

const (
	nightsBank = "acc-conta-noites"
	nightsCard = "acc-cartao-noites"
)

var (
	// nightsCreated is when the app's database is created: its tracking start
	// month is 2026-03.
	nightsCreated = time.Date(2026, time.March, 2, 15, 0, 0, 0, time.UTC)
	// nightsNow is the server's clock and the instant of every run.
	nightsNow = time.Date(2026, time.April, 20, 15, 0, 0, 0, time.UTC)
)

// takeIn is what the router is given to take a snapshot in.
type takeIn int

const (
	// theRealIngest runs ingest.Run over the world's two files.
	theRealIngest takeIn = iota
	// anIdleTakeIn answers success and does nothing.
	anIdleTakeIn
	// aFailingTakeIn answers an error and does nothing.
	aFailingTakeIn
	// noTakeIn leaves the router without one.
	noTakeIn
	// aNewerNightThenTheIngest writes the next night over the ledger, as the
	// collector may between the two attempts of a write, and then runs the real
	// ingest.
	aNewerNightThenTheIngest
)

// nightsWorld is a router over a ledger path whose snapshot a test replaces
// night by night and over an app's database the real ingest feeds.
type nightsWorld struct {
	*writeEnv
	ledgerPath string
	nights     int
	// takeIns counts the calls of the take-in, whatever it is; afterTakeIn is
	// the canonical dump right after the last call returned.
	takeIns     int
	afterTakeIn string
	// runs counts the runs of each recorded write's handler.
	runs map[string]int
	// lastRows are the rows of the last night written.
	lastRows []ledgertest.Transaction
}

func newNightsWorld(t *testing.T, kind takeIn, options ...envOption) *nightsWorld {
	t.Helper()
	w := &nightsWorld{ledgerPath: filepath.Join(t.TempDir(), "pluggy-snapshot.db"), runs: map[string]int{}}
	scratch := authoredtest.Open(t, func() time.Time { return nightsCreated })
	all := append([]envOption{withLedger(w.ledgerPath, nightsNow), w.countingWrites, withDatabase(scratch.Database),
		withTakeIn(w.takeInOf(t, kind, scratch))}, options...)
	env := newTestEnvNotTakenIn(t, all...)
	w.writeEnv = &writeEnv{testEnv: env, scratch: scratch, cookie: env.unlock()}
	return w
}

func (w *nightsWorld) takeInOf(t *testing.T, kind takeIn, scratch *authoredtest.Scratch) func(context.Context, time.Time) error {
	if kind == noTakeIn {
		return nil
	}
	return func(ctx context.Context, at time.Time) error {
		w.takeIns++
		defer func() { w.afterTakeIn = scratch.Canonical(t) }()
		switch kind {
		case anIdleTakeIn:
			return nil
		case aFailingTakeIn:
			return errors.New("the take-in fails")
		case aNewerNightThenTheIngest:
			w.night(w.lastRows...)
		}
		_, err := ingest.Run(ctx, ingest.Sources{Ledger: ledger.New(w.ledgerPath), Database: scratch.Database}, ingest.TriggerWrite, at)
		return err
	}
}

// countingWrites counts every run of each recorded write's handler.
func (w *nightsWorld) countingWrites(config *Config) {
	for method, handler := range config.Reads.writes {
		config.Reads.writes[method] = func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
			w.runs[method]++
			return handler(ctx, args, writes)
		}
	}
}

// night writes the next night's snapshot, holding rows, and renames it over the
// ledger. Night n has runs 1 to n, the last projected; it answers that run.
func (w *nightsWorld) night(rows ...ledgertest.Transaction) int64 {
	w.t.Helper()
	next, run := w.nightBeside(rows...)
	w.replaceTheSnapshotWith(next)
	return run
}

// nightBeside writes the next night's snapshot beside the ledger and answers
// its path and its run, for a test that renames it over the ledger later.
func (w *nightsWorld) nightBeside(rows ...ledgertest.Transaction) (string, int64) {
	w.t.Helper()
	w.nights++
	w.lastRows = rows
	next := ledgertest.NewAt(w.t, fmt.Sprintf("%s.night-%d", w.ledgerPath, w.nights))
	next.AddIdentity(ledgertest.Identity{ID: "identity-noites", ItemID: "item-noites", Document: "987.654.321-00", FullName: "Pessoa Exemplo"})
	next.AddAccount(ledgertest.Account{ID: nightsBank, ItemID: "item-noites", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Exemplo", Currency: "BRL", Balance: 1000})
	next.AddAccount(ledgertest.Account{ID: nightsCard, ItemID: "item-noites", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Cartão Exemplo", Currency: "BRL", Balance: 500})
	for _, row := range rows {
		next.AddTransaction(row)
	}
	run := int64(1)
	for n := 2; n <= w.nights; n++ {
		run = next.AddRun(ledgertest.Run{Outcome: "complete", FinishedAt: time.Date(2026, time.April, n, 6, 0, 0, 0, time.UTC)})
	}
	if run > 1 {
		next.CoverInRun(run)
		next.ProjectTo(run)
	}
	return next.Path, run
}

func (w *nightsWorld) replaceTheSnapshotWith(next string) {
	w.t.Helper()
	if err := os.Rename(next, w.ledgerPath); err != nil {
		w.t.Fatal(err)
	}
}

// unproject deletes the record of the run the rows of the snapshot at path
// came from.
func unproject(t *testing.T, path string) {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec("DELETE FROM projection_state"); err != nil {
		t.Fatal(err)
	}
}

// ingest runs the real ingest outside any request, as the server's timer does.
func (w *nightsWorld) ingest(trigger ingest.Trigger) ingest.Counts {
	w.t.Helper()
	counts, err := ingest.Run(w.t.Context(), ingest.Sources{Ledger: ledger.New(w.ledgerPath), Database: w.scratch.Database}, trigger, nightsNow)
	if err != nil {
		w.t.Fatalf("the ingest: %v", err)
	}
	return counts
}

func (w *nightsWorld) saveBudget(month brday.Month, document authored.Document) {
	w.t.Helper()
	authoredtest.SaveBudget(w.t, w.scratch.Database, nightsNow, month, document)
}

// write posts a write that records a change and answers its id.
func (w *nightsWorld) write(method, arguments string) string {
	w.t.Helper()
	return changeIDOfAnswer(w.t, w.post(method, arguments))
}

func (w *nightsWorld) undoAccepted(label, id string) {
	w.t.Helper()
	expectUndone(w.t, label, w.undo(id))
}

// key is the key the index gives a provider id; found is false for one it never met.
func (w *nightsWorld) key(providerID string) (authored.TxKey, bool) {
	w.t.Helper()
	view := w.scratch.Database.View()
	defer view.Close()
	keys, err := view.TransactionKeys(w.t.Context(), []string{providerID})
	if err != nil {
		w.t.Fatal(err)
	}
	key, found := keys[providerID]
	return key, found
}

func (w *nightsWorld) kept(providerID string) authored.AuthoredTransaction {
	w.t.Helper()
	key, found := w.key(providerID)
	if !found {
		w.t.Fatalf("the index never met %s", providerID)
	}
	view := w.scratch.Database.View()
	defer view.Close()
	kept, err := view.Transactions(w.t.Context(), []authored.TxKey{key})
	if err != nil {
		w.t.Fatal(err)
	}
	return kept[key]
}

func (w *nightsWorld) index() []authored.IndexedTransaction {
	w.t.Helper()
	view := w.scratch.Database.View()
	defer view.Close()
	rows, err := view.IndexedTransactions(w.t.Context())
	if err != nil {
		w.t.Fatal(err)
	}
	return rows
}

// line is the id of the line named name in month's budget.
func (w *nightsWorld) line(month brday.Month, name string) authored.LineID {
	w.t.Helper()
	view := w.scratch.Database.View()
	defer view.Close()
	lines, err := view.LinesOfMonth(w.t.Context(), month)
	if err != nil {
		w.t.Fatal(err)
	}
	for _, line := range lines {
		if line.Name == name {
			return line.ID
		}
	}
	w.t.Fatalf("%s has no line %s", month, name)
	return 0
}

// changesSince is the changes recorded after the first count of them.
func (w *nightsWorld) changesSince(count int) []authoredtest.Change {
	w.t.Helper()
	return w.scratch.Changes(w.t)[count:]
}

// requireTheRulesChangeBefore checks that the changes since count are one
// change of the ingest by the rule and then the person's change id.
func (w *nightsWorld) requireTheRulesChangeBefore(count int, id string) {
	w.t.Helper()
	since := w.changesSince(count)
	if len(since) != 2 || since[0].Author != string(authored.AuthorRule) || since[0].Method != ingest.Method ||
		since[1].Author != string(authored.AuthorPerson) || changeIDText(since[1].ID) != id {
		w.t.Errorf("the changes since the night were %+v, want one of %s by the rule and then the person's %s", since, ingest.Method, id)
	}
}

func changeIDText(id int64) string { return authored.ChangeID(id).String() }

// The invented rows. A card purchase's amount is the bank's, positive.

func nightsPurchase(id string, date time.Time, description string, reais float64) ledgertest.Transaction {
	return ledgertest.Transaction{ID: id, AccountID: nightsCard, Date: date, Description: description,
		Amount: reais, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "1111222233334444"}}}
}

func nightsPendingPurchase(id string, date time.Time, description string, reais float64) ledgertest.Transaction {
	row := nightsPurchase(id, date, description, reais)
	row.Status = "PENDING"
	return row
}

func onDay(month time.Month, day int) time.Time {
	return time.Date(2026, month, day, 15, 0, 0, 0, time.UTC)
}

// nightsBudget is a budget whose variable group holds Mercado and Delivery,
// and the other lines given.
func nightsBudget(more ...authored.DocumentLine) authored.Document {
	return authored.Document{
		authoredtest.Group("rec", authoredtest.Line("Salário", 500_000)),
		authoredtest.Group("var", append([]authored.DocumentLine{authoredtest.Line("Mercado", 80_000), authoredtest.Line("Delivery", 20_000)}, more...)...),
	}
}
