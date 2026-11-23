package ingest_test

import (
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

// Everything here is invented: the accounts, the people, the shops and every
// amount. A test builds each night's snapshot at a second path and renames it
// over the first, as the collector does.

const (
	bankAccount = "acc-bank"
	cardAccount = "acc-card"
	// ownDocument is the person's own document, so a PIX to it is an own transfer.
	ownDocument = "98765432100"
)

// createdOn is when the app's database of most tests is created: its tracking
// start month is 2026-03.
var createdOn = time.Date(2026, time.March, 5, 15, 0, 0, 0, time.UTC)

// runInstant is the clock of the runs and of the person's writes.
var runInstant = time.Date(2026, time.April, 20, 15, 0, 0, 0, time.UTC)

func at(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 15, 0, 0, 0, time.UTC)
}

// world is a ledger file and an app database that the runs of a test bring
// together.
type world struct {
	t          *testing.T
	ledgerPath string
	scratch    *authoredtest.Scratch
	nights     int
}

func newWorld(t *testing.T, created time.Time) *world {
	t.Helper()
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "finwise.db")
	database, err := authored.Open(t.Context(), databasePath, authored.Options{Now: func() time.Time { return created }})
	if err != nil {
		t.Fatalf("open the app's database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return &world{
		t: t, ledgerPath: filepath.Join(directory, "pluggy-snapshot.db"),
		scratch: &authoredtest.Scratch{Database: database, Path: databasePath},
	}
}

func (w *world) database() *authored.Database { return w.scratch.Database }

func (w *world) sources() ingest.Sources {
	return ingest.Sources{Ledger: ledger.New(w.ledgerPath), Database: w.database()}
}

func addAccounts(f *ledgertest.Fixture) {
	f.AddIdentity(ledgertest.Identity{ID: "identity-1", ItemID: "item-1", Document: "987.654.321-00", FullName: "Pessoa Exemplo"})
	f.AddAccount(ledgertest.Account{ID: bankAccount, ItemID: "item-1", Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta Exemplo", Currency: "BRL", Balance: 1000})
	f.AddAccount(ledgertest.Account{ID: cardAccount, ItemID: "item-1", Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Cartão Exemplo", Currency: "BRL", Balance: 500})
}

// night writes the next night's snapshot, holding rows, and renames it over
// the ledger. Night n has runs 1 to n, the last of them ending outcome and
// projected.
func (w *world) night(outcome string, rows ...ledgertest.Transaction) int64 {
	w.t.Helper()
	return w.nightWith(outcome, func(next *ledgertest.Fixture) {
		for _, row := range rows {
			next.AddTransaction(row)
		}
	})
}

// nightWith is night with the rows left to fill.
func (w *world) nightWith(outcome string, fill func(*ledgertest.Fixture)) int64 {
	w.t.Helper()
	w.nights++
	next := ledgertest.NewAt(w.t, w.ledgerPath+".next")
	addAccounts(next)
	fill(next)
	run := int64(1)
	for n := 2; n <= w.nights; n++ {
		ended := "complete"
		if n == w.nights {
			ended = outcome
		}
		run = next.AddRun(ledgertest.Run{Outcome: ended, FinishedAt: at(2026, time.March, 15+n)})
	}
	if run > 1 {
		next.CoverInRun(run)
		next.ProjectTo(run)
	}
	if err := os.Rename(next.Path, w.ledgerPath); err != nil {
		w.t.Fatal(err)
	}
	return run
}

// run runs the ingest and fails the test on an error.
func (w *world) run(trigger ingest.Trigger) ingest.Counts {
	w.t.Helper()
	counts, err := ingest.Run(w.t.Context(), w.sources(), trigger, runInstant)
	if err != nil {
		w.t.Fatalf("run: %v", err)
	}
	return counts
}

// asPerson records write as one change of the person.
func (w *world) asPerson(write func(*authored.Recorder) error) {
	w.t.Helper()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "transactions.setLine", At: runInstant}
	if err := w.database().Change(w.t.Context(), facts, write); err != nil {
		w.t.Fatalf("a write of the person: %v", err)
	}
}

func (w *world) saveBudget(month brday.Month, document authored.Document) {
	w.t.Helper()
	authoredtest.SaveBudget(w.t, w.database(), runInstant, month, document)
}

// line is the id of the line named name in month's budget.
func (w *world) line(month brday.Month, name string) authored.LineID {
	w.t.Helper()
	view := w.database().View()
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

// key is the key the index gives a provider id; found is false for one it
// never met.
func (w *world) key(providerID string) (authored.TxKey, bool) {
	w.t.Helper()
	view := w.database().View()
	defer view.Close()
	keys, err := view.TransactionKeys(w.t.Context(), []string{providerID})
	if err != nil {
		w.t.Fatal(err)
	}
	key, found := keys[providerID]
	return key, found
}

func (w *world) mustKey(providerID string) authored.TxKey {
	w.t.Helper()
	key, found := w.key(providerID)
	if !found {
		w.t.Fatalf("the index never met %s", providerID)
	}
	return key
}

func (w *world) kept(providerID string) authored.AuthoredTransaction {
	w.t.Helper()
	key := w.mustKey(providerID)
	view := w.database().View()
	defer view.Close()
	kept, err := view.Transactions(w.t.Context(), []authored.TxKey{key})
	if err != nil {
		w.t.Fatal(err)
	}
	return kept[key]
}

func (w *world) indexRow(providerID string) authored.IndexedTransaction {
	w.t.Helper()
	key := w.mustKey(providerID)
	for _, row := range w.index() {
		if row.Key == key {
			return row
		}
	}
	w.t.Fatalf("no index row for %s", providerID)
	return authored.IndexedTransaction{}
}

func (w *world) index() []authored.IndexedTransaction {
	w.t.Helper()
	view := w.database().View()
	defer view.Close()
	rows, err := view.IndexedTransactions(w.t.Context())
	if err != nil {
		w.t.Fatal(err)
	}
	return rows
}

func (w *world) lastRun() (int64, bool) {
	w.t.Helper()
	view := w.database().View()
	defer view.Close()
	run, found, err := view.LastIngestedRun(w.t.Context())
	if err != nil {
		w.t.Fatal(err)
	}
	return run, found
}

// ruleChanges is the changes the rule wrote.
func (w *world) ruleChanges() []authoredtest.Change {
	w.t.Helper()
	var rule []authoredtest.Change
	for _, change := range w.scratch.Changes(w.t) {
		if change.Author == string(authored.AuthorRule) {
			rule = append(rule, change)
		}
	}
	return rule
}

func (w *world) everything() string {
	w.t.Helper()
	return w.scratch.Everything(w.t)
}

// The invented rows. A card purchase's amount is the bank's, positive.

func cardRow(id string, date time.Time, description string, reais float64, kind, operation string) ledgertest.Transaction {
	return ledgertest.Transaction{ID: id, AccountID: cardAccount, Date: date, Description: description,
		Amount: reais, Currency: "BRL", Type: kind,
		Raw: map[string]any{"operationType": operation, "creditCardMetadata": map[string]any{"cardNumber": "1111222233334444"}}}
}

func purchase(id string, date time.Time, description string, reais float64) ledgertest.Transaction {
	return cardRow(id, date, description, reais, "DEBIT", "PAGAMENTO")
}

func pendingPurchase(id string, date time.Time, description string, reais float64) ledgertest.Transaction {
	row := purchase(id, date, description, reais)
	row.Status = "PENDING"
	return row
}

func party(name, document string) map[string]any {
	return map[string]any{"name": name, "documentNumber": map[string]any{"type": "CPF", "value": document}}
}

// pix is a PIX on the bank account to or from a person or a company.
func pix(id string, date time.Time, description string, reais float64, name, document string) ledgertest.Transaction {
	kind, payer, receiver := "DEBIT", map[string]any(nil), party(name, document)
	if reais > 0 {
		kind, payer, receiver = "CREDIT", party(name, document), nil
	}
	return ledgertest.Transaction{ID: id, AccountID: bankAccount, Date: date, Description: description,
		Amount: reais, Currency: "BRL", Type: kind,
		Raw: map[string]any{"operationType": "PIX", "paymentData": map[string]any{"paymentMethod": "PIX", "payer": payer, "receiver": receiver}}}
}

// marketBudget is a budget whose variable group holds Mercado and Delivery.
func marketBudget() authored.Document {
	return authored.Document{
		authoredtest.Group("rec", authoredtest.Line("Salário", 500_000)),
		authoredtest.Group("var", authoredtest.Line("Mercado", 80_000), authoredtest.Line("Delivery", 20_000)),
	}
}

// examples indexes count transactions of establishment that the person put on
// line, on days of month, that no ledger lists.
func (w *world) examples(establishment string, line authored.LineID, count int, prefix string) {
	w.t.Helper()
	w.asPerson(func(recorder *authored.Recorder) error {
		for i := range count {
			seen := authoredtest.Seen(prefix+string(rune('a'+i)), brday.Day("2026-03-0"+string(rune('1'+i))), -1_000, establishment)
			key, err := recorder.IndexTransaction(w.t.Context(), seen)
			if err != nil {
				return err
			}
			if err := recorder.SettleLine(w.t.Context(), key, line, authored.AuthorPerson); err != nil {
				return err
			}
		}
		return nil
	})
}

func wantCounts(t *testing.T, label string, got, want ingest.Counts) {
	t.Helper()
	if got != want {
		t.Errorf("%s: counts %+v, want %+v", label, got, want)
	}
}
