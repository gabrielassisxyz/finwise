package authored_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const sentinel = "sentinel-7c1e-never-printed"

// foreignFiles builds, each in a directory of its own, every kind of file that
// is not finwise's database. Each returns the path of the file.
var foreignFiles = map[string]func(t *testing.T) string{
	"foreign database in write-ahead mode": func(t *testing.T) string {
		path := filepath.Join(t.TempDir(), "finwise.db")
		rawDatabase(t, path, "PRAGMA journal_mode = WAL", "CREATE TABLE notes (body TEXT)",
			"INSERT INTO notes VALUES ('"+sentinel+"')")
		return path
	},
	"foreign database in write-ahead mode with a -wal never checkpointed": foreignWithPendingWAL,
	"database with application id 1": func(t *testing.T) string {
		path := filepath.Join(t.TempDir(), "finwise.db")
		rawDatabase(t, path, "PRAGMA application_id = 1", "CREATE TABLE notes (body TEXT)",
			"INSERT INTO notes VALUES ('"+sentinel+"')")
		return path
	},
	"SQLite database with no table and no application id": func(t *testing.T) string {
		path := filepath.Join(t.TempDir(), "finwise.db")
		rawDatabase(t, path, "PRAGMA user_version = 3")
		return path
	},
	"text file longer than a header": func(t *testing.T) string {
		path := filepath.Join(t.TempDir(), "finwise.db")
		writeFile(t, path, strings.Repeat("this is a note and not a database, "+sentinel+"\n", 4))
		return path
	},
	// Recognition must not rest on the id alone: a file needs SQLite's magic too.
	"file that is not SQLite with our id at bytes 68 to 71": func(t *testing.T) string {
		path := filepath.Join(t.TempDir(), "finwise.db")
		writeFile(t, path, string(idWithoutMagic("plain text, not a database, "+sentinel)))
		return path
	},
	"file of ten bytes": func(t *testing.T) string {
		path := filepath.Join(t.TempDir(), "finwise.db")
		writeFile(t, path, "SENTINEL!!")
		return path
	},
}

// foreignWithPendingWAL is a database whose last writes are still only in its
// -wal file, as a crash leaves it: any open of it as a database recovers the
// -wal and, on close, checkpoints it into the main file.
func foreignWithPendingWAL(t *testing.T) string {
	source := filepath.Join(t.TempDir(), "source.db")
	database, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	for _, statement := range []string{
		"PRAGMA journal_mode = WAL", "PRAGMA wal_autocheckpoint = 0",
		"CREATE TABLE notes (body TEXT)", "INSERT INTO notes VALUES ('" + sentinel + "')",
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "finwise.db")
	copyFile(t, source, path)
	copyFile(t, source+"-wal", path+"-wal")
	return path
}

// idWithoutMagic is 128 bytes that begin with text instead of SQLite's magic
// and carry finwise's application id, "FINW", at bytes 68 to 71.
func idWithoutMagic(text string) []byte {
	content := []byte(strings.Repeat(".", 128))
	copy(content, text)
	copy(content[68:], "FINW")
	return content
}

func writeFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func copyFile(t testing.TB, from, to string) {
	t.Helper()
	content, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, to, string(content))
}

func TestEveryFileThatIsNotOursIsRefusedAndLeftByteForByteUntouched(t *testing.T) {
	for name, build := range foreignFiles {
		t.Run(name, func(t *testing.T) {
			path := build(t)
			directory := filepath.Dir(path)
			hashBefore, filesBefore := fileHash(t, path), listDirectory(t, directory)

			database, err := authored.Open(t.Context(), path, authored.Options{Now: clockAt(t, creationInstant)})
			if err == nil {
				database.Close()
				t.Fatal("the file was accepted")
			}
			if failure := asOpenError(t, err).Failure; failure != authored.FailureNotOurs {
				t.Errorf("failure = %d, want FailureNotOurs", failure)
			}
			if !strings.Contains(err.Error(), path) || strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), "SENTINEL") {
				t.Errorf("message %q must name the path and nothing from the file", err)
			}
			if hashAfter := fileHash(t, path); hashAfter != hashBefore {
				t.Errorf("the file changed: %s -> %s", hashBefore, hashAfter)
			}
			if filesAfter := listDirectory(t, directory); !equalStrings(filesBefore, filesAfter) {
				t.Errorf("the directory changed: %v -> %v", filesBefore, filesAfter)
			}
		})
	}
}

func TestAMissingOrEmptyFileOrABareStampBecomesTheDatabase(t *testing.T) {
	cases := map[string]func(t *testing.T, path string){
		"missing": func(t *testing.T, path string) {},
		"empty":   func(t *testing.T, path string) { writeFile(t, path, "") },
		// A creation that stopped right after the stamp: the header and no table.
		"stamped, no table": func(t *testing.T, path string) {
			rawDatabase(t, path, "PRAGMA application_id = 1179209303")
		},
	}
	for name, prepare := range cases {
		t.Run(name, func(t *testing.T) {
			path := databasePath(t)
			prepare(t, path)
			database := openDatabase(t, path, creationInstant, nil)
			if got := trackingStart(t, database); got != "2026-11" {
				t.Errorf("tracking start month = %s, want 2026-11", got)
			}
			database.Close()
			requireFreshSchema(t, path)
		})
	}
}

// A bare stamp is a creation another start began; the month is the one of the
// start that finishes it, not of the one that stamped.
func TestABareStampTakesTheMonthOfTheStartThatFinishesIt(t *testing.T) {
	path := databasePath(t)
	rawDatabase(t, path, "PRAGMA application_id = 1179209303")
	database := openDatabase(t, path, laterInstant, nil)
	if got := trackingStart(t, database); got != "2027-02" {
		t.Errorf("tracking start month = %s, want 2027-02", got)
	}
}

func TestADirectoryThatDoesNotExistIsRefusedAndNotCreated(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "absent", "finwise.db")
	_, err := authored.Open(t.Context(), path, authored.Options{Now: clockAt(t, creationInstant)})
	if failure := asOpenError(t, err).Failure; failure != authored.FailureMissingDirectory {
		t.Errorf("failure = %d, want FailureMissingDirectory", failure)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("message %q does not name the path", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the directory exists now: %v", err)
	}
}

// The id has to be in the main file itself, not only in the -wal, while the
// database is open: that is what a crash before the first checkpoint leaves.
func TestTheMainFileCarriesTheIdBeforeAnyCheckpoint(t *testing.T) {
	path := databasePath(t)
	openDatabase(t, path, creationInstant, nil)

	wal, err := os.Stat(path + "-wal")
	if err != nil || wal.Size() == 0 {
		t.Fatalf("the creation is not waiting in the -wal (%v): the check would prove nothing", err)
	}
	if got := headerID(t, path); got != ourID {
		t.Errorf("bytes 68 to 71 of the main file are %s, want %s", got, ourID)
	}
}

func TestOpeningWithoutAClockFailsBeforeTouchingTheFile(t *testing.T) {
	path := databasePath(t)
	_, err := authored.Open(t.Context(), path, authored.Options{})
	if !errors.Is(err, authored.ErrNoClock) {
		t.Fatalf("err = %v, want ErrNoClock", err)
	}
	if files := listDirectory(t, filepath.Dir(path)); len(files) != 0 {
		t.Errorf("the directory holds %v", files)
	}
}

// requireFreshSchema checks a closed database is exactly what the embedded set
// creates: our id, write-ahead mode, the tables and the recorded migrations.
func requireFreshSchema(t *testing.T, path string) {
	t.Helper()
	if got := headerID(t, path); got != ourID {
		t.Errorf("application id bytes = %s, want %s", got, ourID)
	}
	database := inspect(t, path)
	var id int64
	var mode string
	if err := database.QueryRow("PRAGMA application_id").Scan(&id); err != nil || id != 1179209303 {
		t.Errorf("PRAGMA application_id = %d (%v)", id, err)
	}
	if err := database.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("PRAGMA journal_mode = %q (%v)", mode, err)
	}
	want := []string{"change_effects", "changes", "meta", "pin", "schema_migrations", "settings", "sqlite_sequence"}
	if got := tableNames(t, database); !equalStrings(got, want) {
		t.Errorf("tables = %v, want %v", got, want)
	}
	if got, want := recordedMigrations(t, database), []string{"0001_journal=" + embeddedSum(t, "0001_journal"), "0004_settings=" + embeddedSum(t, "0004_settings"), "0008_session=" + embeddedSum(t, "0008_session")}; !equalStrings(got, want) {
		t.Errorf("schema_migrations = %v, want %v", got, want)
	}
}

func embeddedSum(t testing.TB, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("migrations", name+".sql"))
	if err != nil {
		t.Fatal(err)
	}
	return shaOf(content)
}
