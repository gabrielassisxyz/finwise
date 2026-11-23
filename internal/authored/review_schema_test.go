package authored_test

import (
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

var reviewTables = []string{"closing_steps", "dismissed_warnings", "transfer_marks"}

const (
	markAStep      = `INSERT INTO closing_steps (month, step) VALUES ('2026-03', 'leftover')`
	markATransfer  = `INSERT INTO transfer_marks (month, transfer_key, done_on) VALUES ('2026-03', 'prov:remedios', '2026-03-06')`
	dismissWarning = `INSERT INTO dismissed_warnings (id) VALUES ('price-rose:streamsvc')`
)

// The database the earlier releases left holds 0008_session, numbered above
// this one, so the migration has to be applied whatever its place.
func TestADatabaseThatHeldTheEarlierMigrationsGainsTheReviewTablesWhenOpened(t *testing.T) {
	path := databasePath(t)
	earlier := fstest.MapFS{}
	embedded := withMigrations(t, nil)
	for name, file := range embedded {
		if name != "0007_review.sql" {
			earlier[name] = file
		}
	}
	if _, found := embedded["0007_review.sql"]; !found || len(earlier) != len(embedded)-1 || earlier["0008_session.sql"] == nil {
		t.Fatal("the embedded set does not carry 0007_review below 0008_session, so the test would prove nothing")
	}
	openWithoutReviewTables(t, path, earlier)
	for _, table := range reviewTables {
		if slices.Contains(tableNames(t, inspect(t, path)), table) {
			t.Fatalf("the earlier database already holds %s", table)
		}
	}

	openAndClose(t, path, laterInstant, nil)
	tables := tableNames(t, inspect(t, path))
	for _, table := range reviewTables {
		if !slices.Contains(tables, table) {
			t.Errorf("tables after the open = %v, want %s among them", tables, table)
		}
	}
	if got := columnValues(t, inspect(t, path), "SELECT name FROM schema_migrations WHERE name = '0007_review'"); !equalStrings(got, []string{"0007_review"}) {
		t.Errorf("recorded %v, want 0007_review once", got)
	}
	if copies := copiesBeside(t, path); len(copies) != 1 {
		t.Errorf("copies = %v, want the one taken before migrating a database that holds data", copies)
	}

	openAndClose(t, path, laterInstant, nil)
	if got := columnValues(t, inspect(t, path), "SELECT name FROM schema_migrations WHERE name = '0007_review'"); !equalStrings(got, []string{"0007_review"}) {
		t.Errorf("after a second open recorded %v, want 0007_review once", got)
	}
	if copies := copiesBeside(t, path); len(copies) != 1 {
		t.Errorf("the second open took a copy, so it migrated: %v", copies)
	}
}

func openWithoutReviewTables(t *testing.T, path string, set fs.FS) {
	t.Helper()
	var tables []authored.Table
	for _, table := range authored.AppTables() {
		if !slices.Contains(reviewTables, table.Name) {
			tables = append(tables, table)
		}
	}
	registry, err := authored.NewRegistry(tables)
	if err != nil {
		t.Fatal(err)
	}
	database, err := authored.Open(t.Context(), path, authored.Options{Now: clockAt(t, creationInstant), Migrations: set, Journal: registry})
	if err != nil {
		t.Fatalf("open the earlier release: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTheReviewMigrationIsInTheEmbeddedSet(t *testing.T) {
	names, err := fs.Glob(withMigrations(t, nil), "0007_*.sql")
	if err != nil || !equalStrings(names, []string{"0007_review.sql"}) {
		t.Errorf("0007_* = %v (%v), want 0007_review.sql", names, err)
	}
}

// Each refusal comes from the database itself, with no Go in the way, after the
// valid rows it differs from: the checks a store function written later would
// have to get past.
func TestTheReviewTablesRefuseWhatIsNotAValidRow(t *testing.T) {
	step := func(values string) string { return `INSERT INTO closing_steps (month, step) VALUES ` + values }
	transfer := func(values string) string {
		return `INSERT INTO transfer_marks (month, transfer_key, done_on) VALUES ` + values
	}
	cases := []struct{ name, invalid string }{
		{"the same step of a month twice", step(`('2026-03', 'leftover')`)},
		{"the step pending, which follows the queue", step(`('2026-03', 'pending')`)},
		{"the step close, which is the close itself", step(`('2026-03', 'close')`)},
		{"a step of no known name", step(`('2026-03', 'xyz')`)},
		{"a step in upper case", step(`('2026-03', 'LEFTOVER')`)},
		{"a step of no month", step(`(NULL, 'report')`)},
		{"a step of a month that is not one", step(`('março', 'report')`)},
		{"no step", step(`('2026-03', NULL)`)},
		{"the same transfer of a month twice", transfer(`('2026-03', 'prov:remedios', '2026-03-07')`)},
		{"a transfer of no month", transfer(`(NULL, 'res:viagens', '2026-03-06')`)},
		{"a transfer of a month that is not one", transfer(`('2026-3', 'res:viagens', '2026-03-06')`)},
		{"a transfer of no key", transfer(`('2026-03', NULL, '2026-03-06')`)},
		{"a transfer of an empty key", transfer(`('2026-03', '', '2026-03-06')`)},
		{"a transfer marked on no day", transfer(`('2026-03', 'res:viagens', NULL)`)},
		{"a transfer marked on a day that is not one", transfer(`('2026-03', 'res:viagens', 'ontem')`)},
		{"the same warning dismissed twice", `INSERT INTO dismissed_warnings (id) VALUES ('price-rose:streamsvc')`},
		{"a warning of no id", `INSERT INTO dismissed_warnings (id) VALUES (NULL)`},
		{"a warning of an empty id", `INSERT INTO dismissed_warnings (id) VALUES ('')`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := migratedPool(t)
			for _, valid := range []string{markAStep, markATransfer, dismissWarning} {
				mustExec(t, database, valid)
			}
			if _, err := database.Exec(c.invalid); err == nil {
				t.Errorf("the database accepted %s", c.invalid)
			}
		})
	}
}

// Every step a person ticks by hand, another month's mark of the same step and
// transfer, and a second warning are all rows of their own.
func TestTheReviewTablesAcceptEveryMarkableStepAndOneRowPerMonthAndKey(t *testing.T) {
	database := migratedPool(t)
	for _, valid := range []string{markAStep, markATransfer, dismissWarning} {
		mustExec(t, database, valid)
	}
	for _, step := range []string{"planned-actual", "contributions", "card-bill", "subscriptions", "report"} {
		mustExec(t, database, `INSERT INTO closing_steps (month, step) VALUES ('2026-03', '`+step+`')`)
	}
	mustExec(t, database, `INSERT INTO closing_steps (month, step) VALUES ('2026-04', 'leftover')`)
	mustExec(t, database, `INSERT INTO transfer_marks (month, transfer_key, done_on) VALUES ('2026-04', 'prov:remedios', '2026-04-06')`)
	mustExec(t, database, `INSERT INTO dismissed_warnings (id) VALUES ('charged-after-cancel:streamsvc')`)
	for table, want := range map[string]string{"closing_steps": "7", "transfer_marks": "2", "dismissed_warnings": "2"} {
		if got := columnValues(t, database, "SELECT count(*) FROM "+table); !equalStrings(got, []string{want}) {
			t.Errorf("%s holds %v rows, want %s", table, got, want)
		}
	}
}

// The three are set tables of the app's registry: a mark is a fact by
// existing, never a column of a row that changes.
func TestTheReviewTablesAreSetTablesOfTheRegistry(t *testing.T) {
	found := map[string]bool{}
	for _, table := range authored.AppTables() {
		if !slices.Contains(reviewTables, table.Name) {
			continue
		}
		found[table.Name] = true
		if table.Shape != authored.ShapeSet || table.OwnedBy != nil {
			t.Errorf("%s is entered with shape %d and owner %v, want an unowned set table", table.Name, table.Shape, table.OwnedBy)
		}
	}
	for _, table := range reviewTables {
		if !found[table] {
			t.Errorf("%s is not in the registry", table)
		}
	}
}
