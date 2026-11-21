package ledger_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func TestEveryQueryRefusesAnUnsupportedVersion(t *testing.T) {
	f := populated(t)
	f.SetUserVersion(2)

	for name, err := range everyQuery(ledger.New(f.Path)) {
		if !ledger.IsUnsupportedVersion(err) {
			t.Errorf("%s: err = %v, want the unsupported-version error", name, err)
		}
	}
}

func TestUnsupportedVersionErrorNamesTheVersionFound(t *testing.T) {
	f := populated(t)
	f.SetUserVersion(7)

	_, err := ledger.New(f.Path).Accounts(t.Context())
	var versionErr *ledger.UnsupportedVersionError
	if !asVersionError(err, &versionErr) || versionErr.Found != 7 {
		t.Fatalf("err = %v, want UnsupportedVersionError{Found: 7}", err)
	}
}

func TestUnsupportedVersionYieldsNoRows(t *testing.T) {
	f := populated(t)
	f.SetUserVersion(2)
	reader := ledger.New(f.Path)

	accounts, _ := reader.Accounts(t.Context())
	transactions, _ := reader.CleanTransactions(t.Context(), february2026(t))
	if len(accounts) != 0 || len(transactions) != 0 {
		t.Fatalf("rows came back from an unsupported snapshot: %d accounts, %d transactions",
			len(accounts), len(transactions))
	}
}

func TestSnapshotOpenPathRefusesWrites(t *testing.T) {
	f := populated(t)
	database, err := ledger.OpenReadOnly(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	statements := []string{
		"CREATE TABLE intruder (a INTEGER)",
		"INSERT INTO runs (started_at, started_day, outcome, code_version, item_ids) VALUES ('x', 'x', 'running', 'x', '[]')",
		"UPDATE accounts SET balance = 0",
		"PRAGMA user_version = 9",
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err == nil {
			t.Errorf("write succeeded through the read-only open path: %s", statement)
		}
	}
}

func TestQueriesLeaveTheSnapshotByteForByteUntouched(t *testing.T) {
	f := populated(t)
	directory := filepath.Dir(f.Path)
	before, filesBefore := fileHash(t, f.Path), listDirectory(t, directory)

	for name, err := range everyQuery(ledger.New(f.Path)) {
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	if after := fileHash(t, f.Path); after != before {
		t.Errorf("snapshot hash changed: %s -> %s", before, after)
	}
	if filesAfter := listDirectory(t, directory); !equalStrings(filesBefore, filesAfter) {
		t.Errorf("directory changed: %v -> %v", filesBefore, filesAfter)
	}
}

func TestEachCallSeesTheFileThatIsThereNow(t *testing.T) {
	f := populated(t)
	reader := ledger.New(f.Path)
	accounts, err := reader.Accounts(t.Context())
	if err != nil || len(accounts) != 1 || accounts[0].ID != "acc-1" {
		t.Fatalf("first call: %v, %v", accounts, err)
	}

	replacement := ledgertest.NewAt(t, f.Path+".next")
	replacement.AddAccount(ledgertest.Account{ID: "acc-new", ItemID: "item-1", Type: "BANK", Name: "Replaced", Currency: "BRL"})
	if err := os.Rename(replacement.Path, f.Path); err != nil {
		t.Fatal(err)
	}

	accounts, err = reader.Accounts(t.Context())
	if err != nil || len(accounts) != 1 || accounts[0].ID != "acc-new" {
		t.Fatalf("second call after the rename: %v, %v", accounts, err)
	}
}

func TestMissingSnapshotIsAnErrorNotAnEmptyLedger(t *testing.T) {
	reader := ledger.New(filepath.Join(t.TempDir(), "absent.db"))
	if _, err := reader.Accounts(t.Context()); err == nil {
		t.Fatal("a missing snapshot returned no error")
	}
}

func TestEveryQueryNamesAFileThatIsNotADatabaseAsUnreadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pluggy-snapshot.db")
	if err := os.WriteFile(path, []byte("this is text, not a SQLite database, and it is long enough to fill a header"), 0o600); err != nil {
		t.Fatal(err)
	}

	for name, err := range everyQuery(ledger.New(path)) {
		if !ledger.IsUnreadableSnapshot(err) {
			t.Errorf("%s: err = %T, want the unreadable-snapshot error", name, err)
		}
		if ledger.IsUnsupportedVersion(err) {
			t.Errorf("%s: a file that is not a database was reported as another version", name)
		}
	}
}

// The header and the first page are intact, so the file opens and passes the
// version check; the pages that hold the rows are overwritten, so the failure
// comes in the middle of a query.
func TestADamagedDatabaseIsUnreadableToo(t *testing.T) {
	f := populated(t)
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

	reader := ledger.New(f.Path)
	if _, err := reader.Accounts(t.Context()); !ledger.IsUnreadableSnapshot(err) {
		t.Errorf("Accounts: err = %T, want the unreadable-snapshot error", err)
	}
	if _, err := reader.CleanTransactions(t.Context(), february2026(t)); !ledger.IsUnreadableSnapshot(err) {
		t.Errorf("CleanTransactions: err = %T, want the unreadable-snapshot error", err)
	}
}

func TestAMissingFileAndAnotherVersionAreNotUnreadable(t *testing.T) {
	if _, err := ledger.New(filepath.Join(t.TempDir(), "absent.db")).Accounts(t.Context()); ledger.IsUnreadableSnapshot(err) {
		t.Error("a missing file was reported as unreadable")
	}
	f := populated(t)
	f.SetUserVersion(2)
	if _, err := ledger.New(f.Path).Accounts(t.Context()); ledger.IsUnreadableSnapshot(err) {
		t.Error("another schema version was reported as unreadable")
	}
}

func listDirectory(t testing.TB, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
