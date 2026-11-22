package main

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// createdSource is a database created by the package, holding an invented
// change whose method is the sentinel, closed as a stopped server leaves it.
func createdSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finwise.db")
	database, err := authored.Open(t.Context(), path, authored.Options{Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	query := url.Values{}
	query.Add("_pragma", "busy_timeout(5000)")
	raw, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`INSERT INTO changes (at, day, author, method) VALUES ('2026-12-01T01:30:00.000Z', '2026-11-30', 'person', ?)`, sentinel); err != nil {
		t.Fatal(err)
	}
	return path
}

func backupCommand(source, destination string) []string {
	return []string{"backup", "--db", source, "--to", destination}
}

// recordedInstant is read last in a test: an open of the source as a database
// may leave files beside it.
func recordedInstant(t *testing.T, source string) []string {
	t.Helper()
	return columnValues(t, inspectDatabase(t, source), "SELECT value FROM meta WHERE key = 'last_backup_at'")
}

func requireNoSentinel(t *testing.T, result outcome) {
	t.Helper()
	if strings.Contains(result.stdout+result.stderr, sentinel) || strings.Contains(result.stdout+result.stderr, "SENTINEL") {
		t.Errorf("the output quotes the file: stdout %q stderr %q", result.stdout, result.stderr)
	}
}

func TestBackupWritesACheckedCopyAndNothingElse(t *testing.T) {
	source := createdSource(t)
	directory := t.TempDir()
	destination := filepath.Join(directory, "finwise-backup.db")

	result := runCommand(context.Background(), backupCommand(source, destination), "", nil, nil)
	if result.code != 0 {
		t.Fatalf("backup exited %d: %s", result.code, result.stderr)
	}
	requireNoSentinel(t, result)
	if files := listDirectory(t, directory); !slices.Equal(files, []string{"finwise-backup.db"}) {
		t.Fatalf("the destination's directory holds %v, want only the backup", files)
	}
	if files := listDirectory(t, filepath.Dir(source)); !slices.Equal(files, []string{"finwise.db"}) {
		t.Errorf("the source's directory holds %v", files)
	}
	copied, original := inspectDatabase(t, destination), inspectDatabase(t, source)
	var id int64
	var integrity string
	if err := copied.QueryRow("PRAGMA application_id").Scan(&id); err != nil || id != 1179209303 {
		t.Errorf("the copy's application_id = %d (%v)", id, err)
	}
	if err := copied.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Errorf("the copy's integrity_check = %q (%v)", integrity, err)
	}
	for _, query := range []string{
		"SELECT name || '=' || sha256 FROM schema_migrations ORDER BY name",
		"SELECT value FROM meta WHERE key = 'tracking_start_month'",
	} {
		got, want := columnValues(t, copied, query), columnValues(t, original, query)
		if len(want) == 0 || !slices.Equal(got, want) {
			t.Errorf("%s: the copy has %v, the source %v", query, got, want)
		}
	}
	if got := recordedInstant(t, source); !slices.Equal(got, []string{"2026-12-01T01:30:00Z"}) {
		t.Errorf("the source records %v, want the test clock in RFC 3339", got)
	}
}

func TestBackupAcceptsPathsRelativeToTheWorkingDirectory(t *testing.T) {
	source := createdSource(t)
	t.Chdir(filepath.Dir(source))
	if err := os.Mkdir("copies", 0o700); err != nil {
		t.Fatal(err)
	}
	result := runCommand(context.Background(), backupCommand("finwise.db", filepath.Join("copies", "finwise-backup.db")), "", nil, nil)
	if result.code != 0 {
		t.Fatalf("backup exited %d: %s", result.code, result.stderr)
	}
	if files := listDirectory(t, "copies"); !slices.Equal(files, []string{"finwise-backup.db"}) {
		t.Errorf("the destination's directory holds %v", files)
	}
}

func TestBackupTakesTheDatabaseFromTheEnvironmentAndTheFlagWins(t *testing.T) {
	fromVariable := createdSource(t)
	vars := map[string]string{"FINWISE_DB": fromVariable}
	if result := runCommand(context.Background(), []string{"backup", "--to", filepath.Join(t.TempDir(), "copy.db")}, "", vars, nil); result.code != 0 {
		t.Fatalf("backup from FINWISE_DB exited %d: %s", result.code, result.stderr)
	}
	if got := recordedInstant(t, fromVariable); len(got) != 1 {
		t.Errorf("the variable's database records %v", got)
	}

	ignoredDirectory, fromFlag := t.TempDir(), createdSource(t)
	vars["FINWISE_DB"] = filepath.Join(ignoredDirectory, "finwise.db")
	if result := runCommand(context.Background(), backupCommand(fromFlag, filepath.Join(t.TempDir(), "copy.db")), "", vars, nil); result.code != 0 {
		t.Fatalf("backup with both exited %d: %s", result.code, result.stderr)
	}
	if got := recordedInstant(t, fromFlag); len(got) != 1 {
		t.Errorf("the flag's database records %v", got)
	}
	if files := listDirectory(t, ignoredDirectory); len(files) != 0 {
		t.Errorf("the variable's path was used although the flag was given: %v", files)
	}
}

func TestBackupWithoutASourceOrADestinationIsAUsageError(t *testing.T) {
	source := createdSource(t)
	directory := t.TempDir()
	destination := filepath.Join(directory, "copy.db")
	cases := map[string]struct {
		args []string
		vars map[string]string
	}{
		"neither":                      {[]string{"backup"}, nil},
		"no destination":               {[]string{"backup", "--db", source}, nil},
		"no destination, FINWISE_DB":   {[]string{"backup"}, map[string]string{"FINWISE_DB": source}},
		"no source":                    {[]string{"backup", "--to", destination}, nil},
		"a variable for --to":          {[]string{"backup", "--db", source}, map[string]string{"FINWISE_TO": destination, "FINWISE_BACKUP_TO": destination, "FINWISE_BACKUP": destination}},
		"an unknown flag":              {[]string{"backup", "--db", source, "--to", destination, "--force"}, nil},
		"an argument after the flags":  {[]string{"backup", "--db", source, "--to", destination, "extra"}, nil},
		"the destination as its value": {[]string{"backup", "--db", source, destination}, nil},
	}
	hashBefore := fileHash(t, source)
	for name, c := range cases {
		result := runCommand(context.Background(), c.args, "", c.vars, nil)
		if result.code != exitUsage {
			t.Errorf("%s: exited %d, want %d", name, result.code, exitUsage)
		}
		requireNoSentinel(t, result)
		if files := listDirectory(t, directory); len(files) != 0 {
			t.Errorf("%s: the destination's directory holds %v", name, files)
		}
	}
	if fileHash(t, source) != hashBefore || !slices.Equal(listDirectory(t, filepath.Dir(source)), []string{"finwise.db"}) {
		t.Error("a usage error touched the source")
	}
	if got := recordedInstant(t, source); len(got) != 0 {
		t.Errorf("a usage error recorded %v", got)
	}
	if result := runCommand(context.Background(), nil, "", nil, nil); !strings.Contains(result.stderr, "backup") {
		t.Errorf("the usage line %q does not name backup", result.stderr)
	}
}

func TestBackupRefusesADatabaseThatIsNotOurs(t *testing.T) {
	for _, kind := range []string{"write-ahead", "collector snapshot", "application id 1", "text"} {
		t.Run(kind, func(t *testing.T) {
			source := foreignFile(t, kind)
			sourceDirectory, directory := filepath.Dir(source), t.TempDir()
			hashBefore, filesBefore := fileHash(t, source), listDirectory(t, sourceDirectory)

			result := runCommand(context.Background(), backupCommand(source, filepath.Join(directory, "copy.db")), "", nil, nil)
			if result.code == 0 || !strings.Contains(result.stderr, source) || !strings.Contains(result.stderr, "is not finwise's database") {
				t.Errorf("exit %d, stderr %q: want a refusal that names the path", result.code, result.stderr)
			}
			requireNoSentinel(t, result)
			if fileHash(t, source) != hashBefore {
				t.Error("the file changed")
			}
			if got := listDirectory(t, sourceDirectory); !slices.Equal(got, filesBefore) {
				t.Errorf("the file's directory went from %v to %v", filesBefore, got)
			}
			if got := listDirectory(t, directory); len(got) != 0 {
				t.Errorf("the destination's directory holds %v", got)
			}
		})
	}
}

func TestBackupOfAMissingOrEmptyDatabaseCreatesNothing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "finwise.db")
	directory := t.TempDir()
	result := runCommand(context.Background(), backupCommand(missing, filepath.Join(directory, "copy.db")), "", nil, nil)
	if result.code == 0 || !strings.Contains(result.stderr, missing) {
		t.Errorf("a missing database: exit %d, stderr %q", result.code, result.stderr)
	}
	if _, err := os.Lstat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the missing database was created: %v", err)
	}
	if got := listDirectory(t, filepath.Dir(missing)); len(got) != 0 {
		t.Errorf("the missing database's directory holds %v", got)
	}

	empty := filepath.Join(t.TempDir(), "finwise.db")
	writeTestFile(t, empty, "")
	result = runCommand(context.Background(), backupCommand(empty, filepath.Join(directory, "copy.db")), "", nil, nil)
	if result.code == 0 || !strings.Contains(result.stderr, empty) {
		t.Errorf("an empty database: exit %d, stderr %q", result.code, result.stderr)
	}
	if info, err := os.Stat(empty); err != nil || info.Size() != 0 {
		t.Errorf("the empty database is no longer empty: %v %v", info, err)
	}
	if got := listDirectory(t, filepath.Dir(empty)); !slices.Equal(got, []string{"finwise.db"}) {
		t.Errorf("the empty database's directory holds %v", got)
	}
	if got := listDirectory(t, directory); len(got) != 0 {
		t.Errorf("the destination's directory holds %v", got)
	}
}

func TestBackupNeverReplacesADestinationThatExists(t *testing.T) {
	source := createdSource(t)
	directory := t.TempDir()
	destination := filepath.Join(directory, "finwise-backup.db")
	writeTestFile(t, destination, "an earlier backup")
	destinationBefore, sourceBefore := fileHash(t, destination), fileHash(t, source)
	filesBefore := listDirectory(t, directory)

	result := runCommand(context.Background(), backupCommand(source, destination), "", nil, nil)
	if result.code == 0 || !strings.Contains(result.stderr, destination) {
		t.Errorf("exit %d, stderr %q: want a refusal that names the destination", result.code, result.stderr)
	}
	requireNoSentinel(t, result)
	if fileHash(t, destination) != destinationBefore {
		t.Error("the destination was replaced")
	}
	if fileHash(t, source) != sourceBefore {
		t.Error("the source's bytes changed")
	}
	if got := listDirectory(t, directory); !slices.Equal(got, filesBefore) {
		t.Errorf("the destination's directory went from %v to %v", filesBefore, got)
	}
	if got := recordedInstant(t, source); len(got) != 0 {
		t.Errorf("a refused backup recorded %v", got)
	}
}
