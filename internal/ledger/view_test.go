package ledger_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// ledgerQueries is what a Reader and a View both answer.
type ledgerQueries interface {
	Accounts(context.Context) ([]ledger.Account, error)
	Bills(context.Context) ([]ledger.Bill, error)
	BalanceHistory(context.Context) ([]ledger.AccountDay, error)
	ReserveHistory(context.Context) ([]ledger.Pot, error)
	Identities(context.Context) ([]ledger.Identity, error)
	InvestmentPrincipals(context.Context) (map[string]money.Cents, error)
	Investments(context.Context) ([]ledger.Investment, error)
	InvestmentHistory(context.Context, string) ([]ledger.InvestmentDay, error)
	Pots(context.Context, brday.Day) ([]ledger.Pot, error)
	PotYields(context.Context, string, brday.Day, brday.Day) ([]ledger.PotYield, error)
	SyncState(context.Context) (ledger.SyncState, error)
	CleanTransactions(context.Context, brday.Period) ([]ledger.Transaction, error)
	YieldHistory(context.Context) (ledger.YieldHistory, error)
}

type queryAnswer struct {
	value any
	err   error
}

// everyAnswer asks all 13 queries, Accounts first, and keeps what each said.
func everyAnswer(t *testing.T, queries ledgerQueries) map[string]queryAnswer {
	t.Helper()
	ctx := t.Context()
	answers := map[string]queryAnswer{}
	record := func(name string) func(any, error) {
		return func(value any, err error) { answers[name] = queryAnswer{value, err} }
	}
	record("Accounts")(queries.Accounts(ctx))
	record("Bills")(queries.Bills(ctx))
	record("BalanceHistory")(queries.BalanceHistory(ctx))
	record("ReserveHistory")(queries.ReserveHistory(ctx))
	record("Identities")(queries.Identities(ctx))
	record("InvestmentPrincipals")(queries.InvestmentPrincipals(ctx))
	record("Investments")(queries.Investments(ctx))
	record("InvestmentHistory")(queries.InvestmentHistory(ctx, "inv-1"))
	record("Pots")(queries.Pots(ctx, "2026-03-15"))
	record("PotYields")(queries.PotYields(ctx, "acc-1", yieldFrom, yieldTo))
	record("SyncState")(queries.SyncState(ctx))
	record("CleanTransactions")(queries.CleanTransactions(ctx, february2026(t)))
	record("YieldHistory")(queries.YieldHistory(ctx))
	return answers
}

// viewLedger is the populated ledger with one earlier sighting of the account,
// so that the pot yield window has two ends and answers rows.
func viewLedger(t *testing.T) *ledgertest.Fixture {
	t.Helper()
	f := populated(t)
	override := ledgertest.ReservesOverride(ledgertest.Reserve{ID: "pot-a", Name: "Trip", Amount: 8.00, CDIRatio: 1.0})
	override["updatedAt"] = "2026-03-10T02:00:00.000Z"
	f.ObserveAccount("acc-1", string(yieldFrom), override)
	return f
}

func renameOver(t *testing.T, replacement *ledgertest.Fixture, path string) {
	t.Helper()
	if err := os.Rename(replacement.Path, path); err != nil {
		t.Fatal(err)
	}
}

// An empty ledger is renamed over the file after the view's first query, so a
// query that ran on a handle of its own would answer from it and differ.
func TestEveryQueryOnOneViewAnswersAsTheReaderDoes(t *testing.T) {
	f := viewLedger(t)
	reader := ledger.New(f.Path)
	want := everyAnswer(t, reader)
	replacement := ledgertest.NewAt(t, f.Path+".next")
	other := everyAnswer(t, ledger.New(replacement.Path))
	for name, answer := range want {
		if answer.err != nil {
			t.Fatalf("reader %s: %v", name, answer.err)
		}
		if reflect.DeepEqual(answer.value, other[name].value) {
			t.Fatalf("reader %s answers the same over both ledgers, so the swap cannot show a stray handle", name)
		}
	}

	view := reader.View()
	defer view.Close()
	if _, err := view.Accounts(t.Context()); err != nil {
		t.Fatal(err)
	}
	renameOver(t, replacement, f.Path)

	got := everyAnswer(t, view)
	if len(got) != 13 {
		t.Fatalf("asked %d queries, want 13", len(got))
	}
	for name, answer := range got {
		if answer.err != nil {
			t.Errorf("view %s: %v", name, answer.err)
			continue
		}
		if !reflect.DeepEqual(answer.value, want[name].value) {
			t.Errorf("view %s = %#v, want the reader's %#v", name, answer.value, want[name].value)
		}
	}
}

func TestAViewKeepsItsSnapshotWhenTheFileIsReplaced(t *testing.T) {
	f := populated(t)
	reader := ledger.New(f.Path)
	view := reader.View()
	defer view.Close()
	accounts, err := view.Accounts(t.Context())
	if err != nil || len(accounts) != 1 || accounts[0].ID != "acc-1" {
		t.Fatalf("first query: %v, %v", accounts, err)
	}

	replacement := ledgertest.NewAt(t, f.Path+".next")
	replacement.AddAccount(ledgertest.Account{ID: "acc-new", ItemID: "item-1", Type: "BANK", Name: "Replaced", Currency: "BRL"})
	replacement.AddTransaction(ledgertest.Transaction{
		ID: "tx-new", AccountID: "acc-new", Date: instant(t, "2026-02-11T12:00:00Z"),
		Description: "Replaced purchase", Amount: -1.00, Currency: "BRL", Type: "DEBIT",
	})
	renameOver(t, replacement, f.Path)

	accounts, err = view.Accounts(t.Context())
	if err != nil || len(accounts) != 1 || accounts[0].ID != "acc-1" {
		t.Errorf("same view after the rename: accounts %v, %v; want acc-1", accounts, err)
	}
	transactions, err := view.CleanTransactions(t.Context(), february2026(t))
	if err != nil || len(transactions) != 1 || transactions[0].ID != "tx-1" {
		t.Errorf("same view after the rename: transactions %v, %v; want tx-1", transactions, err)
	}

	later := reader.View()
	defer later.Close()
	accounts, err = later.Accounts(t.Context())
	if err != nil || len(accounts) != 1 || accounts[0].ID != "acc-new" {
		t.Errorf("view opened after the rename: accounts %v, %v; want acc-new", accounts, err)
	}
	transactions, err = later.CleanTransactions(t.Context(), february2026(t))
	if err != nil || len(transactions) != 1 || transactions[0].ID != "tx-new" {
		t.Errorf("view opened after the rename: transactions %v, %v; want tx-new", transactions, err)
	}
}

func TestAViewKeptOpenLeavesTheSnapshotByteForByteUntouched(t *testing.T) {
	f := viewLedger(t)
	directory := filepath.Dir(f.Path)
	before, filesBefore := fileHash(t, f.Path), listDirectory(t, directory)

	view := ledger.New(f.Path).View()
	for name, answer := range everyAnswer(t, view) {
		if answer.err != nil {
			t.Errorf("%s: %v", name, answer.err)
		}
	}
	if filesOpen := listDirectory(t, directory); !equalStrings(filesBefore, filesOpen) {
		t.Errorf("directory changed while the view was open: %v -> %v", filesBefore, filesOpen)
	}
	if err := view.Close(); err != nil {
		t.Fatal(err)
	}

	if after := fileHash(t, f.Path); after != before {
		t.Errorf("snapshot hash changed: %s -> %s", before, after)
	}
	if filesAfter := listDirectory(t, directory); !equalStrings(filesBefore, filesAfter) {
		t.Errorf("directory changed: %v -> %v", filesBefore, filesAfter)
	}
}

// A read leaves no trace on a handle that could write, and query_only alone
// refuses every write, so neither the test above nor a write attempt with both
// guards on can tell whether the connection mode is read-only. The session guard
// is lifted here, on one connection, to see the mode refuse on its own.
func TestTheOpenPathRefusesAWriteWithTheSessionGuardLifted(t *testing.T) {
	f := viewLedger(t)
	before := fileHash(t, f.Path)
	database, err := ledger.OpenReadOnly(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	connection, err := database.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	if _, err := connection.ExecContext(t.Context(), "PRAGMA query_only = 0"); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(t.Context(), "UPDATE accounts SET balance = 0"); err == nil {
		t.Error("a write succeeded through the open path with query_only off: the connection is not read-only")
	}
	if after := fileHash(t, f.Path); after != before {
		t.Errorf("snapshot hash changed: %s -> %s", before, after)
	}
}

func TestAViewAnswersAnUnusableSnapshotWithTheTypedErrors(t *testing.T) {
	cases := map[string]struct {
		path string
		want func(error) bool
	}{
		"another version": {path: versionTwoSnapshot(t), want: ledger.IsUnsupportedVersion},
		"not a database":  {path: textFile(t), want: ledger.IsUnreadableSnapshot},
		"damaged":         {path: damagedSnapshot(t), want: ledger.IsUnreadableSnapshot},
		"missing": {
			path: filepath.Join(t.TempDir(), "absent.db"),
			want: func(err error) bool { return errors.Is(err, fs.ErrNotExist) },
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			view := ledger.New(test.path).View()
			defer view.Close()
			for query, answer := range everyAnswer(t, view) {
				if !test.want(answer.err) {
					t.Errorf("%s: err = %v (%T), want the %s error", query, answer.err, answer.err, name)
				}
				if holdsRows(reflect.ValueOf(answer.value)) {
					t.Errorf("%s: a row came back: %#v", query, answer.value)
				}
			}
		})
	}
}

func TestAQueryOnAClosedViewIsAnErrorNotAFreshOpen(t *testing.T) {
	f := populated(t)
	reader := ledger.New(f.Path)

	used := reader.View()
	if _, err := used.Accounts(t.Context()); err != nil {
		t.Fatal(err)
	}
	unused := reader.View()
	for name, view := range map[string]*ledger.View{"used": used, "never used": unused} {
		if err := view.Close(); err != nil {
			t.Fatalf("%s: close: %v", name, err)
		}
		if err := view.Close(); err != nil {
			t.Errorf("%s: second close: %v", name, err)
		}
		accounts, err := view.Accounts(t.Context())
		if !errors.Is(err, ledger.ErrViewClosed) || len(accounts) != 0 {
			t.Errorf("%s: query on a closed view = %v, %v; want ErrViewClosed and no row", name, accounts, err)
		}
	}
}

// holdsRows reports whether an answer carries anything: an empty list or map is
// no row, whether or not it is nil.
func holdsRows(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Slice, reflect.Map:
		return value.Len() > 0
	case reflect.Pointer:
		return !value.IsNil()
	case reflect.Struct:
		for index := range value.NumField() {
			if holdsRows(value.Field(index)) {
				return true
			}
		}
		return false
	default:
		return !value.IsZero()
	}
}

func versionTwoSnapshot(t *testing.T) string {
	t.Helper()
	f := populated(t)
	f.SetUserVersion(2)
	return f.Path
}

func textFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pluggy-snapshot.db")
	if err := os.WriteFile(path, []byte("this is text, not a SQLite database, and it is long enough to fill a header"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The header and the first page stay intact, so the file opens and passes the
// version check and the failure comes in the middle of a query.
func damagedSnapshot(t *testing.T) string {
	t.Helper()
	f := viewLedger(t)
	content, err := os.ReadFile(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	const firstPage = 4096
	for index := firstPage; index < len(content); index++ {
		content[index] = 0xFF
	}
	if err := os.WriteFile(f.Path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return f.Path
}
