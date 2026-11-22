package ledger_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// snapshotAt builds an invented snapshot with one account at path, creating the
// directory it lives in.
func snapshotAt(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	f := ledgertest.NewAt(t, path)
	f.AddItem(ledgertest.Item{ID: "item-1", Connector: "Test Bank", Status: "UPDATED", ExecutionStatus: "SUCCESS"})
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: "Test Checking", Currency: "BRL", Balance: 10})
}

func accountIDs(t *testing.T, path string) []string {
	t.Helper()
	view := viewOf(t, path)
	accounts, err := view.Accounts(t.Context())
	if err != nil {
		t.Fatalf("accounts of %q: %v", path, err)
	}
	ids := make([]string, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids
}

func TestARelativePathOpensTheSnapshotThatItsAbsoluteFormOpens(t *testing.T) {
	cases := map[string]string{
		"a directory":     filepath.Join("data", "snapshot.db"),
		"dot slash":       "." + string(filepath.Separator) + filepath.Join("data", "snapshot.db"),
		"working folder":  "snapshot.db",
		"dot slash, bare": "." + string(filepath.Separator) + "snapshot.db",
	}
	for name, relative := range cases {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			absolute := filepath.Join(directory, relative)
			snapshotAt(t, absolute)
			t.Chdir(directory)

			want := accountIDs(t, absolute)
			if !slices.Equal(want, []string{"acc-1"}) {
				t.Fatalf("the absolute path gave %v", want)
			}
			if got := accountIDs(t, relative); !slices.Equal(got, want) {
				t.Errorf("%q gave %v, the absolute path %v", relative, got, want)
			}
		})
	}
}

func TestAMissingSnapshotNamedRelativelyIsTheErrorOfAMissingOneNamedAbsolutely(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	for _, relative := range []string{"absent.db", filepath.Join("data", "absent.db"), "." + string(filepath.Separator) + "absent.db"} {
		for _, path := range []string{relative, filepath.Join(directory, relative)} {
			view := viewOf(t, path)
			_, err := view.Accounts(t.Context())
			if !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("%q: err = %v (%T), want fs.ErrNotExist", path, err, err)
			}
			if ledger.IsUnreadableSnapshot(err) || ledger.IsUnsupportedVersion(err) {
				t.Errorf("%q: a missing file was typed as another failure: %v", path, err)
			}
		}
	}
}

func TestADirectoryWithURICharactersOpensAbsolutelyAndRelatively(t *testing.T) {
	const awkward = "a b#c?d%e"
	// The fixture opens its own file through a plain path, which SQLite's
	// driver cannot parse here, so the file is built elsewhere and moved in.
	built := filepath.Join(t.TempDir(), "snapshot.db")
	snapshotAt(t, built)
	parent := t.TempDir()
	absolute := filepath.Join(parent, awkward, "snapshot.db")
	if err := os.Mkdir(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(built, absolute); err != nil {
		t.Fatal(err)
	}

	if got := accountIDs(t, absolute); !slices.Equal(got, []string{"acc-1"}) {
		t.Errorf("the absolute path gave %v", got)
	}
	t.Chdir(parent)
	if got := accountIDs(t, filepath.Join(awkward, "snapshot.db")); !slices.Equal(got, []string{"acc-1"}) {
		t.Errorf("the relative path gave %v", got)
	}
}
