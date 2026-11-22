package authored_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const (
	createX2 = "CREATE TABLE x2 (id INTEGER PRIMARY KEY) STRICT;"
	createX3 = "CREATE TABLE x3 (id INTEGER PRIMARY KEY) STRICT;"
	// copyName is the copy a start at laterInstant takes of finwise.db.
	copyName = "finwise.db.before-20270210T120000Z"
)

func copiesBeside(t testing.TB, path string) []string {
	t.Helper()
	var copies []string
	for _, name := range listDirectory(t, filepath.Dir(path)) {
		if strings.Contains(name, ".before-") {
			copies = append(copies, name)
		}
	}
	return copies
}

func recordedOf(t testing.TB, path string) []string {
	t.Helper()
	return recordedMigrations(t, inspect(t, path))
}

func namesOf(t testing.TB, path string) []string {
	t.Helper()
	return columnValues(t, inspect(t, path), "SELECT name FROM schema_migrations ORDER BY name")
}

func TestASecondStartWithNothingPendingAppliesNothing(t *testing.T) {
	path := databasePath(t)
	openAndClose(t, path, creationInstant, nil)
	before := recordedOf(t, path)

	openAndClose(t, path, laterInstant, nil)
	if after := recordedOf(t, path); !equalStrings(before, after) {
		t.Errorf("schema_migrations changed: %v -> %v", before, after)
	}
	if copies := copiesBeside(t, path); len(copies) != 0 {
		t.Errorf("a copy was made: %v", copies)
	}
}

func TestAMissingMigrationIsAppliedWhateverItsPosition(t *testing.T) {
	path := databasePath(t)
	openAndClose(t, path, creationInstant, withMigrations(t, map[string]string{"0003_x": createX3}))

	openAndClose(t, path, laterInstant, withMigrations(t, map[string]string{"0002_x": createX2, "0003_x": createX3}))
	if got, want := namesOf(t, path), []string{"0001_journal", "0002_budget", "0002_x", "0003_x", "0004_settings", "0008_session"}; !equalStrings(got, want) {
		t.Errorf("recorded = %v, want %v", got, want)
	}
	if tables := tableNames(t, inspect(t, path)); !slices.Contains(tables, "x2") {
		t.Errorf("0002_x was not applied: tables %v", tables)
	}
}

func TestARecordedMigrationTheSetCannotVouchForStopsTheStart(t *testing.T) {
	changed := withMigrations(t, map[string]string{"0002_x": createX2})
	journal := changed["0001_journal.sql"].Data
	altered := append([]byte{}, journal...)
	altered[len(altered)-2] ^= 0x01
	changed["0001_journal.sql"] = &fstest.MapFile{Data: altered}

	cases := map[string]struct {
		created fstest.MapFS
		started fstest.MapFS
		failure authored.Failure
	}{
		"one byte of an applied file differs": {
			created: withMigrations(t, nil), started: changed, failure: authored.FailureChangedMigration,
		},
		"the database records a name the set lacks": {
			created: withMigrations(t, map[string]string{"0003_x": createX3}),
			started: withMigrations(t, map[string]string{"0002_x": createX2}),
			failure: authored.FailureUnknownMigration,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			path := databasePath(t)
			openAndClose(t, path, creationInstant, c.created)
			before := recordedOf(t, path)

			database, err := authored.Open(t.Context(), path, authored.Options{Now: clockAt(t, laterInstant), Migrations: c.started})
			if err == nil {
				database.Close()
				t.Fatal("the start was not refused")
			}
			if failure := asOpenError(t, err).Failure; failure != c.failure {
				t.Errorf("failure = %d, want %d", failure, c.failure)
			}
			if after := recordedOf(t, path); !equalStrings(before, after) {
				t.Errorf("schema_migrations changed: %v -> %v", before, after)
			}
			if tables := tableNames(t, inspect(t, path)); slices.Contains(tables, "x2") {
				t.Error("the pending migration was applied")
			}
			if copies := copiesBeside(t, path); len(copies) != 0 {
				t.Errorf("a copy was made: %v", copies)
			}
		})
	}
}

func TestACopyIsTakenBeforeMigratingADatabaseThatRecordsOne(t *testing.T) {
	path := databasePath(t)
	openAndClose(t, path, creationInstant, nil)

	openAndClose(t, path, laterInstant, withMigrations(t, map[string]string{"0002_x": createX2}))
	if copies := copiesBeside(t, path); !equalStrings(copies, []string{copyName}) {
		t.Fatalf("copies = %v, want [%s]", copies, copyName)
	}
	copyPath := filepath.Join(filepath.Dir(path), copyName)
	if got := headerID(t, copyPath); got != ourID {
		t.Errorf("the copy's id bytes are %s", got)
	}
	copyDatabase := inspect(t, copyPath)
	var integrity string
	if err := copyDatabase.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Errorf("integrity_check = %q (%v)", integrity, err)
	}
	if names := columnValues(t, copyDatabase, "SELECT name FROM schema_migrations"); !equalStrings(names, []string{"0001_journal", "0002_budget", "0004_settings", "0008_session"}) {
		t.Errorf("the copy records %v: it was not taken before the migration", names)
	}
	if names := namesOf(t, path); !equalStrings(names, []string{"0001_journal", "0002_budget", "0002_x", "0004_settings", "0008_session"}) {
		t.Errorf("the database records %v", names)
	}
}

func TestACopyNameAlreadyTakenStopsTheStartAndIsNotOverwritten(t *testing.T) {
	for name, content := range map[string]string{"a file with content": "occupied", "an empty file": ""} {
		t.Run(name, func(t *testing.T) {
			path := databasePath(t)
			openAndClose(t, path, creationInstant, nil)
			occupied := filepath.Join(filepath.Dir(path), copyName)
			writeFile(t, occupied, content)
			hashBefore := fileHash(t, occupied)

			database, err := authored.Open(t.Context(), path, authored.Options{
				Now: clockAt(t, laterInstant), Migrations: withMigrations(t, map[string]string{"0002_x": createX2}),
			})
			if err == nil {
				database.Close()
				t.Fatal("the start was not refused")
			}
			if failure := asOpenError(t, err).Failure; failure != authored.FailureCopy {
				t.Errorf("failure = %d, want FailureCopy", failure)
			}
			if hashAfter := fileHash(t, occupied); hashAfter != hashBefore {
				t.Error("the file at the copy's name was overwritten")
			}
			if names := namesOf(t, path); !equalStrings(names, []string{"0001_journal", "0002_budget", "0004_settings", "0008_session"}) {
				t.Errorf("the migration ran without its copy: %v", names)
			}
		})
	}
}

func TestAFailingMigrationRollsBackAndKeepsWhatCameBefore(t *testing.T) {
	path := databasePath(t)
	openAndClose(t, path, creationInstant, nil)

	set := withMigrations(t, map[string]string{
		"0002_x":    createX2,
		"0003_fail": "CREATE TABLE x3 (id INTEGER PRIMARY KEY) STRICT; INSERT INTO nowhere VALUES (1);",
	})
	_, err := authored.Open(t.Context(), path, authored.Options{Now: clockAt(t, laterInstant), Migrations: set})
	openErr := asOpenError(t, err)
	if openErr.Failure != authored.FailureMigration || openErr.Migration != "0003_fail" {
		t.Errorf("failure = %d for %q, want FailureMigration for 0003_fail", openErr.Failure, openErr.Migration)
	}
	if !strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "nowhere") {
		t.Errorf("message %q must name the path and not quote SQLite", err)
	}
	if names := namesOf(t, path); !equalStrings(names, []string{"0001_journal", "0002_budget", "0002_x", "0004_settings", "0008_session"}) {
		t.Errorf("recorded = %v, want 0001_journal, 0002_budget, 0002_x, 0004_settings and 0008_session", names)
	}
	tables := tableNames(t, inspect(t, path))
	if slices.Contains(tables, "x3") || !slices.Contains(tables, "x2") {
		t.Errorf("tables = %v: x3 must be absent and x2 present", tables)
	}
	if copies := copiesBeside(t, path); !equalStrings(copies, []string{copyName}) {
		t.Errorf("copies = %v, want the one taken before migrating", copies)
	}
}

func TestAFailedFirstMigrationLeavesAFileTheNextStartFinishes(t *testing.T) {
	path := databasePath(t)
	broken := fstest.MapFS{"0001_journal.sql": &fstest.MapFile{Data: []byte("CREATE TABLE changes (id INTEGER PRIMARY KEY); SELECT * FROM nowhere;")}}
	if _, err := authored.Open(t.Context(), path, authored.Options{Now: clockAt(t, creationInstant), Migrations: broken}); err == nil {
		t.Fatal("the broken set was applied")
	}

	database := openDatabase(t, path, laterInstant, nil)
	if got := trackingStart(t, database); got != "2027-02" {
		t.Errorf("tracking start month = %s, want the month of the start that created the tables", got)
	}
	database.Close()
	requireFreshSchema(t, path)
	if copies := copiesBeside(t, path); len(copies) != 0 {
		t.Errorf("a copy was made of a file that recorded nothing: %v", copies)
	}
}

func TestTheTrackingStartMonthIsTheBrazilMonthOfCreationAndNeverChanges(t *testing.T) {
	for instant, want := range map[string]string{
		"2026-12-01T01:30:00Z": "2026-11", // 22:30 on 30 November in Brazil
		"2026-12-01T03:00:00Z": "2026-12", // midnight on 1 December in Brazil
	} {
		database := openDatabase(t, databasePath(t), instant, nil)
		if got := trackingStart(t, database); got != want {
			t.Errorf("created at %s: %s, want %s", instant, got, want)
		}
	}

	path := databasePath(t)
	openAndClose(t, path, creationInstant, nil)
	reopened := openDatabase(t, path, laterInstant, nil)
	if got := trackingStart(t, reopened); got != "2026-11" {
		t.Errorf("after a reopen: %s, want 2026-11", got)
	}
	reopened.Close()
	migrated := openDatabase(t, path, laterInstant, withMigrations(t, map[string]string{"0002_x": createX2}))
	if got := trackingStart(t, migrated); got != "2026-11" {
		t.Errorf("after a further migration: %s, want 2026-11", got)
	}
}
