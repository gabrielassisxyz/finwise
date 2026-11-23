package authored_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	_ "modernc.org/sqlite"
)

const (
	creationInstant = "2026-12-01T01:30:00Z"
	laterInstant    = "2027-02-10T12:00:00Z"
	ourID           = "46494e57"
)

func clockAt(t testing.TB, text string) func() time.Time {
	t.Helper()
	instant, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatal(err)
	}
	return func() time.Time { return instant }
}

func databasePath(t testing.TB) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "finwise.db")
}

// openDatabase opens with the embedded set unless one is given, and closes the
// handle when the test ends.
func openDatabase(t testing.TB, path, instant string, set fs.FS) *authored.Database {
	t.Helper()
	database, err := authored.Open(t.Context(), path, authored.Options{Now: clockAt(t, instant), Migrations: set})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// openAndClose is a whole start: open, then close, so the next one finds the
// file as a stopped server leaves it.
func openAndClose(t testing.TB, path, instant string, set fs.FS) {
	t.Helper()
	database, err := authored.Open(t.Context(), path, authored.Options{Now: clockAt(t, instant), Migrations: set})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}

// withMigrations is the embedded set plus test files, each name without .sql.
func withMigrations(t testing.TB, extra map[string]string) fstest.MapFS {
	t.Helper()
	set := fstest.MapFS{}
	embedded := authored.EmbeddedMigrations()
	paths, err := fs.Glob(embedded, "*.sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("embedded set: %v %v", paths, err)
	}
	for _, path := range paths {
		content, err := fs.ReadFile(embedded, path)
		if err != nil {
			t.Fatal(err)
		}
		set[path] = &fstest.MapFile{Data: content}
	}
	for name, body := range extra {
		set[name+".sql"] = &fstest.MapFile{Data: []byte(body)}
	}
	return set
}

// embeddedMigrationNames is the sorted names of the embedded migrations plus
// any extras a test adds.
func embeddedMigrationNames(t testing.TB, extra ...string) []string {
	t.Helper()
	paths, err := fs.Glob(authored.EmbeddedMigrations(), "*.sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("embedded set: %v %v", paths, err)
	}
	names := make([]string, 0, len(paths)+len(extra))
	for _, path := range paths {
		names = append(names, strings.TrimSuffix(path, ".sql"))
	}
	names = append(names, extra...)
	sort.Strings(names)
	return names
}

func asOpenError(t testing.TB, err error) *authored.OpenError {
	t.Helper()
	var openErr *authored.OpenError
	if !errors.As(err, &openErr) {
		t.Fatalf("err = %v (%T), want an OpenError", err, err)
	}
	return openErr
}

// inspect opens a closed database file directly, for what the test reads from it.
func inspect(t testing.TB, path string) *sql.DB {
	t.Helper()
	query := url.Values{}
	query.Set("mode", "ro")
	database, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String())
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { database.Close() })
	return database
}

type querier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

func columnValues(t testing.TB, database querier, query string) []string {
	t.Helper()
	rows, err := database.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}

func tableNames(t testing.TB, database querier) []string {
	t.Helper()
	return columnValues(t, database, "SELECT name FROM sqlite_schema WHERE type = 'table' ORDER BY name")
}

// recordedMigrations is every row of schema_migrations as name=sha256.
func recordedMigrations(t testing.TB, database querier) []string {
	t.Helper()
	return columnValues(t, database, "SELECT name || '=' || sha256 FROM schema_migrations ORDER BY name")
}

func trackingStart(t testing.TB, database *authored.Database) string {
	t.Helper()
	month, err := database.TrackingStartMonth(t.Context())
	if err != nil {
		t.Fatalf("tracking start month: %v", err)
	}
	return string(month)
}

func fileHash(t testing.TB, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func shaOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
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

// headerID reads bytes 68 to 71 of the file from disk, without SQLite.
func headerID(t testing.TB, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) < 72 {
		t.Fatalf("%s holds %d bytes, no header", path, len(content))
	}
	return hex.EncodeToString(content[68:72])
}

// rawDatabase builds a test's own SQLite file with plain settings.
func rawDatabase(t testing.TB, path string, statements ...string) {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}
