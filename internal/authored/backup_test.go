package authored_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const (
	firstBackupInstant  = "2026-12-01T01:30:00Z"
	secondBackupInstant = "2026-12-02T05:15:00Z"
	insertSentinel      = `INSERT INTO changes (at, day, author, method)
VALUES ('2026-12-01T01:30:00.000Z', '2026-11-30', 'person', '` + sentinel + `')`
)

// stoppedSource is a database created by the package, holding one invented
// change, as a stopped server leaves it: nobody has it open.
func stoppedSource(t *testing.T) string {
	t.Helper()
	path := databasePath(t)
	database, err := authored.Open(t.Context(), path, authored.Options{Now: clockAt(t, creationInstant)})
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, database.Pool(), insertSentinel)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func backupAt(ctx context.Context, t *testing.T, source, destination, instant string) error {
	t.Helper()
	return authored.Backup(ctx, source, destination, authored.BackupOptions{Now: clockAt(t, instant)})
}

func backupWithHooks(ctx context.Context, t *testing.T, source, destination string, afterCopy func(string), beforePublish, afterPublish func()) error {
	t.Helper()
	options := authored.WithBackupHooks(authored.BackupOptions{Now: clockAt(t, firstBackupInstant)}, afterCopy, beforePublish, afterPublish)
	return authored.Backup(ctx, source, destination, options)
}

func asBackupError(t *testing.T, err error, want authored.BackupFailure) {
	t.Helper()
	var backupErr *authored.BackupError
	if !errors.As(err, &backupErr) {
		t.Fatalf("err = %v (%T), want a BackupError", err, err)
	}
	if backupErr.Failure != want {
		t.Fatalf("failure %d (%v), want %d", backupErr.Failure, err, want)
	}
}

// recordedBackup is the instant stored in a stopped source, read through a
// view of the package as the settings read does; "" when none is recorded.
func recordedBackup(t *testing.T, source string) string {
	t.Helper()
	database := openDatabase(t, source, laterInstant, nil)
	view := database.View()
	defer view.Close()
	instant, found, err := view.LastBackup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		return ""
	}
	return instant.Format("2006-01-02T15:04:05Z07:00")
}

func storedBackupText(t *testing.T, database querier) string {
	t.Helper()
	values := columnValues(t, database, "SELECT value FROM meta WHERE key = 'last_backup_at'")
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func integrityOf(t *testing.T, path string) string {
	t.Helper()
	var answer string
	if err := inspect(t, path).QueryRow("PRAGMA integrity_check").Scan(&answer); err != nil {
		t.Fatal(err)
	}
	return answer
}

// The server holds the source open while the copy is taken, as the nightly
// job finds it.
func TestABackupIsACheckedCopyOfTheSourceAndRecordsItsInstant(t *testing.T) {
	source := databasePath(t)
	database := openDatabase(t, source, creationInstant, nil)
	mustExec(t, database.Pool(), insertSentinel)
	directory := t.TempDir()
	destination := filepath.Join(directory, "finwise-backup.db")

	if err := backupAt(t.Context(), t, source, destination, firstBackupInstant); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if files := listDirectory(t, directory); !equalStrings(files, []string{"finwise-backup.db"}) {
		t.Fatalf("the destination's directory holds %v, want only the backup", files)
	}
	if got := headerID(t, destination); got != ourID {
		t.Errorf("the copy's application id is %s, want %s", got, ourID)
	}
	if got := integrityOf(t, destination); got != "ok" {
		t.Errorf("integrity_check answered %q", got)
	}
	copied := inspect(t, destination)
	if got, want := recordedMigrations(t, copied), recordedMigrations(t, database.Pool()); !equalStrings(got, want) {
		t.Errorf("the copy records %v, the source %v", got, want)
	}
	if got := columnValues(t, copied, "SELECT value FROM meta WHERE key = 'tracking_start_month'"); !equalStrings(got, []string{trackingStart(t, database)}) {
		t.Errorf("the copy's creation month is %v, want %s", got, trackingStart(t, database))
	}
	if got := columnValues(t, copied, "SELECT method FROM changes"); !equalStrings(got, []string{sentinel}) {
		t.Errorf("the copy holds the changes %d", len(got))
	}
	if got := storedBackupText(t, database.Pool()); got != firstBackupInstant {
		t.Errorf("the source records %q, want %q", got, firstBackupInstant)
	}

	second := filepath.Join(directory, "finwise-backup-2.db")
	if err := backupAt(t.Context(), t, source, second, secondBackupInstant); err != nil {
		t.Fatalf("second backup: %v", err)
	}
	if got := storedBackupText(t, database.Pool()); got != secondBackupInstant {
		t.Errorf("after the second backup the source records %q, want %q", got, secondBackupInstant)
	}
}

func TestTheViewAnswersTheLastBackupAndNothingBeforeTheFirst(t *testing.T) {
	source := stoppedSource(t)
	if got := recordedBackup(t, source); got != "" {
		t.Fatalf("a database never backed up answers %q", got)
	}
	if _, found, err := authored.EmptyView().LastBackup(t.Context()); found || err != nil {
		t.Errorf("the empty view answered found %v err %v", found, err)
	}
	if err := backupAt(t.Context(), t, source, filepath.Join(t.TempDir(), "copy.db"), firstBackupInstant); err != nil {
		t.Fatal(err)
	}
	if got := recordedBackup(t, source); got != firstBackupInstant {
		t.Errorf("the view answers %q, want %q", got, firstBackupInstant)
	}
}

// tableDump is every row of every table and the schema, the instant of the
// last backup left out.
func tableDump(t *testing.T, database *sql.DB) []string {
	t.Helper()
	dump := columnValues(t, database, "SELECT type || ' ' || name || ' ' || coalesce(sql, '') FROM sqlite_schema ORDER BY name")
	for _, table := range tableNames(t, database) {
		query := fmt.Sprintf("SELECT * FROM %q", table)
		if table == "meta" {
			query += " WHERE key <> 'last_backup_at'"
		}
		dump = append(dump, rowsAsText(t, database, table, query)...)
	}
	return dump
}

func rowsAsText(t *testing.T, database *sql.DB, table, query string) []string {
	t.Helper()
	rows, err := database.Query(query + " ORDER BY rowid")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, fmt.Sprintf("%s %v", table, values))
	}
	return lines
}

func TestABackupChangesNothingInTheSourceButItsInstant(t *testing.T) {
	source := databasePath(t)
	database := openDatabase(t, source, creationInstant, nil)
	mustExec(t, database.Pool(), insertSentinel)
	mustExec(t, database.Pool(), insertChange)
	before := tableDump(t, database.Pool())

	if err := backupAt(t.Context(), t, source, filepath.Join(t.TempDir(), "copy.db"), firstBackupInstant); err != nil {
		t.Fatal(err)
	}
	if after := tableDump(t, database.Pool()); !equalStrings(before, after) {
		t.Errorf("the source changed:\nbefore %v\nafter  %v", before, after)
	}
	if got := storedBackupText(t, database.Pool()); got != firstBackupInstant {
		t.Errorf("the instant is %q, want %q", got, firstBackupInstant)
	}
}

// takenDestinations are what can hold the destination's name. A link refuses
// each, a dangling link included, where a rename would replace it.
var takenDestinations = map[string]func(t *testing.T, path string){
	"file":             func(t *testing.T, path string) { writeFile(t, path, "an earlier backup, "+sentinel) },
	"empty file":       func(t *testing.T, path string) { writeFile(t, path, "") },
	"dangling symlink": func(t *testing.T, path string) { mustSymlink(t, filepath.Join(filepath.Dir(path), "gone"), path) },
}

func mustSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

// fingerprint tells a file, or the link itself, apart from any other content.
func fingerprint(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			t.Fatal(err)
		}
		return "link to " + target
	}
	return "file " + fileHash(t, path)
}

func TestABackupNeverReplacesADestinationThatExists(t *testing.T) {
	for name, take := range takenDestinations {
		t.Run(name, func(t *testing.T) {
			source := stoppedSource(t)
			directory := t.TempDir()
			destination := filepath.Join(directory, "finwise-backup.db")
			take(t, destination)
			destinationBefore, sourceBefore := fingerprint(t, destination), fileHash(t, source)
			filesBefore, sourceFilesBefore := listDirectory(t, directory), listDirectory(t, filepath.Dir(source))

			asBackupError(t, backupAt(t.Context(), t, source, destination, firstBackupInstant), authored.BackupDestinationTaken)
			if got := fingerprint(t, destination); got != destinationBefore {
				t.Errorf("the destination changed: %s -> %s", destinationBefore, got)
			}
			if got := fileHash(t, source); got != sourceBefore {
				t.Error("the source's bytes changed")
			}
			if got := listDirectory(t, directory); !equalStrings(got, filesBefore) {
				t.Errorf("the destination's directory went from %v to %v", filesBefore, got)
			}
			if got := listDirectory(t, filepath.Dir(source)); !equalStrings(got, sourceFilesBefore) {
				t.Errorf("the source's directory went from %v to %v", sourceFilesBefore, got)
			}
			if got := recordedBackup(t, source); got != "" {
				t.Errorf("a refused backup recorded %q", got)
			}
		})
	}
}

// The hook creates the name with O_EXCL, so the file really appears after the
// backup began and found the name free, and before the publication.
func TestADestinationThatAppearsBeforeThePublicationIsNotReplaced(t *testing.T) {
	for name, take := range takenDestinations {
		t.Run(name, func(t *testing.T) {
			source := stoppedSource(t)
			directory := t.TempDir()
			destination := filepath.Join(directory, "finwise-backup.db")
			var appeared string
			appear := func() {
				if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("the destination existed before the hook: %v", err)
				}
				take(t, destination)
				appeared = fingerprint(t, destination)
			}

			asBackupError(t, backupWithHooks(t.Context(), t, source, destination, nil, appear, nil), authored.BackupDestinationTaken)
			if appeared == "" {
				t.Fatal("the backup never reached its publication")
			}
			if got := fingerprint(t, destination); got != appeared {
				t.Errorf("the file that appeared changed: %s -> %s", appeared, got)
			}
			if got := listDirectory(t, directory); !equalStrings(got, []string{"finwise-backup.db"}) {
				t.Errorf("the directory holds %v, want only the file that appeared", got)
			}
			if got := recordedBackup(t, source); got != "" {
				t.Errorf("a refused backup recorded %q", got)
			}
		})
	}
}

// damages make the copy fail its check after VACUUM INTO wrote it.
var damages = map[string]func(t *testing.T, temporary string){
	"another application id": func(t *testing.T, temporary string) { overwrite(t, temporary, 68, []byte{0, 0, 0, 1}) },
	"a damaged page": func(t *testing.T, temporary string) {
		overwrite(t, temporary, 4096, []byte(strings.Repeat("\xff", 64)))
	},
	"one integrity row other than ok": breakOneCheckConstraint,
}

// breakOneCheckConstraint stores one effect with neither side, which the schema
// forbids, so integrity_check answers a single row that is not "ok": a copy
// that is damaged in one place only.
func breakOneCheckConstraint(t *testing.T, temporary string) {
	t.Helper()
	rawDatabase(t, temporary, "PRAGMA ignore_check_constraints = 1",
		`INSERT INTO change_effects (change_id, seq, tbl, key) VALUES (1, 1, 'lines', '["a"]')`)
	copied, err := sql.Open("sqlite", temporary)
	if err != nil {
		t.Fatal(err)
	}
	defer copied.Close()
	answers := columnValues(t, copied, "PRAGMA integrity_check")
	if len(answers) != 1 || answers[0] == "ok" {
		t.Fatalf("integrity_check answered %d rows (%v), want one row other than ok", len(answers), answers)
	}
}

func overwrite(t *testing.T, path string, offset int64, content []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteAt(content, offset); err != nil {
		t.Fatal(err)
	}
}

func TestACopyThatFailsItsCheckIsNeitherPublishedNorLeftBehind(t *testing.T) {
	for name, damage := range damages {
		t.Run(name, func(t *testing.T) {
			source := stoppedSource(t)
			directory := t.TempDir()
			destination := filepath.Join(directory, "finwise-backup.db")
			reached := false
			damageCopy := func(temporary string) {
				if filepath.Dir(temporary) != directory {
					t.Errorf("the temporary copy %s is not beside the destination", temporary)
				}
				reached = true
				damage(t, temporary)
			}

			asBackupError(t, backupWithHooks(t.Context(), t, source, destination, damageCopy, nil, nil), authored.BackupCheckFailed)
			if !reached {
				t.Fatal("the copy was never written")
			}
			if got := listDirectory(t, directory); len(got) != 0 {
				t.Errorf("the directory holds %v after a failed check", got)
			}
			if got := recordedBackup(t, source); got != "" {
				t.Errorf("a failed backup recorded %q", got)
			}
		})
	}
}

// The context is cancelled once the copy is published, so the write of the
// instant really fails after the publication.
func TestARecordThatFailsAfterThePublicationWithdrawsTheCopy(t *testing.T) {
	source := stoppedSource(t)
	directory := t.TempDir()
	destination := filepath.Join(directory, "finwise-backup.db")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	published := false
	failTheRecord := func() {
		if got := headerID(t, destination); got != ourID {
			t.Fatalf("the published copy has id %s", got)
		}
		published = true
		cancel()
	}

	asBackupError(t, backupWithHooks(ctx, t, source, destination, nil, nil, failTheRecord), authored.BackupNotRecorded)
	if !published {
		t.Fatal("the copy was never published")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the published copy is still there: %v", err)
	}
	if got := listDirectory(t, directory); len(got) != 0 {
		t.Errorf("the directory holds %v, want what it held before: nothing", got)
	}
	if got := recordedBackup(t, source); got != "" {
		t.Errorf("a withdrawn backup recorded %q", got)
	}
}

// Between the publication and the failed record another file takes the
// destination's name: the withdrawal must leave that file, which this backup
// did not publish.
func TestAFailedRecordLeavesAFileThatTookTheNameAfterThePublication(t *testing.T) {
	source := stoppedSource(t)
	directory := t.TempDir()
	destination := filepath.Join(directory, "finwise-backup.db")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var other string
	replaceThenFail := func() {
		if err := os.Remove(destination); err != nil {
			t.Fatalf("the copy was not published: %v", err)
		}
		writeFile(t, destination, "another file, written after the publication")
		other = fingerprint(t, destination)
		cancel()
	}

	asBackupError(t, backupWithHooks(ctx, t, source, destination, nil, nil, replaceThenFail), authored.BackupNotWithdrawn)
	if other == "" {
		t.Fatal("the copy was never published")
	}
	if got := fingerprint(t, destination); got != other {
		t.Errorf("the file that took the name changed: %s -> %s", other, got)
	}
	if got := listDirectory(t, directory); !equalStrings(got, []string{"finwise-backup.db"}) {
		t.Errorf("the directory holds %v, want only the other file", got)
	}
	if got := recordedBackup(t, source); got != "" {
		t.Errorf("a failed backup recorded %q", got)
	}
}

func TestABackupRefusesASourceThatIsNotOurs(t *testing.T) {
	for name, build := range foreignFiles {
		t.Run(name, func(t *testing.T) {
			source := build(t)
			sourceDirectory, directory := filepath.Dir(source), t.TempDir()
			hashBefore, filesBefore := fileHash(t, source), listDirectory(t, sourceDirectory)

			err := backupAt(t.Context(), t, source, filepath.Join(directory, "copy.db"), firstBackupInstant)
			if asOpenError(t, err).Failure != authored.FailureNotOurs || strings.Contains(err.Error(), sentinel) {
				t.Errorf("err = %v, want a refusal as not ours that quotes nothing", err)
			}
			if fileHash(t, source) != hashBefore {
				t.Error("the source's bytes changed")
			}
			if got := listDirectory(t, sourceDirectory); !equalStrings(got, filesBefore) {
				t.Errorf("the source's directory went from %v to %v", filesBefore, got)
			}
			if got := listDirectory(t, directory); len(got) != 0 {
				t.Errorf("the destination's directory holds %v", got)
			}
		})
	}
}

func TestABackupOfAMissingOrEmptySourceCreatesNothing(t *testing.T) {
	for name, prepare := range map[string]func(t *testing.T, path string){
		"missing":    func(*testing.T, string) {},
		"zero bytes": func(t *testing.T, path string) { writeFile(t, path, "") },
	} {
		t.Run(name, func(t *testing.T) {
			source := databasePath(t)
			prepare(t, source)
			filesBefore, directory := listDirectory(t, filepath.Dir(source)), t.TempDir()

			asBackupError(t, backupAt(t.Context(), t, source, filepath.Join(directory, "copy.db"), firstBackupInstant), authored.BackupSourceMissing)
			if got := listDirectory(t, filepath.Dir(source)); !equalStrings(got, filesBefore) {
				t.Errorf("the source's directory went from %v to %v", filesBefore, got)
			}
			if info, err := os.Stat(source); err == nil && info.Size() != 0 {
				t.Errorf("the source now holds %d bytes", info.Size())
			}
			if got := listDirectory(t, directory); len(got) != 0 {
				t.Errorf("the destination's directory holds %v", got)
			}
		})
	}
}

func TestABackupRequiresAClock(t *testing.T) {
	source := stoppedSource(t)
	directory := t.TempDir()
	if err := authored.Backup(t.Context(), source, filepath.Join(directory, "copy.db"), authored.BackupOptions{}); !errors.Is(err, authored.ErrNoClock) {
		t.Errorf("err = %v, want ErrNoClock", err)
	}
	if got := listDirectory(t, directory); len(got) != 0 {
		t.Errorf("the destination's directory holds %v", got)
	}
}
