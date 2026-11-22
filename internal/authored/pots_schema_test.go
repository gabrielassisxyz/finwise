package authored_test

import (
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const (
	mintTwoLines   = `INSERT INTO lines (builtin) VALUES (0), (0)`
	linkLineTwo    = `INSERT INTO pot_lines (line_id, pot_id) VALUES (2, 'investment:inv-1')`
	bucketForPotOf = `INSERT INTO pot_settings (pot_id, bucket) VALUES ('reserve:acc-pots/pot-1', 'Reservado')`
)

func TestThePotsMigrationIsRecordedOnceAndAppliedNothingTheSecondTime(t *testing.T) {
	path := databasePath(t)
	openAndClose(t, path, creationInstant, nil)
	openAndClose(t, path, laterInstant, nil)
	got := columnValues(t, inspect(t, path), "SELECT name FROM schema_migrations WHERE name = '0005_pots'")
	if !equalStrings(got, []string{"0005_pots"}) {
		t.Errorf("recorded %v, want 0005_pots once", got)
	}
	if copies := copiesBeside(t, path); len(copies) != 0 {
		t.Errorf("the second open took a copy, so it migrated: %v", copies)
	}
}

// The database the earlier releases left holds migrations numbered above 0005,
// so the migration has to be applied to it whatever its place in the order.
func TestADatabaseThatHeldTheEarlierMigrationsGainsThePotTablesWhenOpened(t *testing.T) {
	path := databasePath(t)
	earlier := fstest.MapFS{}
	embedded := withMigrations(t, nil)
	for name, file := range embedded {
		if name != "0005_pots.sql" {
			earlier[name] = file
		}
	}
	if _, found := embedded["0005_pots.sql"]; !found || len(earlier) != len(embedded)-1 {
		t.Fatal("the embedded set does not carry 0005_pots, so the test would prove nothing")
	}
	openEarlier(t, path, earlier)
	if tables := tableNames(t, inspect(t, path)); slices.Contains(tables, "pot_lines") || slices.Contains(tables, "pot_settings") {
		t.Fatalf("the earlier database already holds the pot tables: %v", tables)
	}

	openAndClose(t, path, laterInstant, nil)
	tables := tableNames(t, inspect(t, path))
	if !slices.Contains(tables, "pot_lines") || !slices.Contains(tables, "pot_settings") {
		t.Errorf("tables after the open = %v, want pot_lines and pot_settings", tables)
	}
	if got := columnValues(t, inspect(t, path), "SELECT name FROM schema_migrations WHERE name = '0005_pots'"); !equalStrings(got, []string{"0005_pots"}) {
		t.Errorf("recorded %v, want 0005_pots once", got)
	}
	if copies := copiesBeside(t, path); len(copies) != 1 {
		t.Errorf("copies = %v, want the one taken before migrating a database that holds data", copies)
	}
}

// openEarlier starts the app as the release before the pots was: its migration
// set and a registry that does not name the pot tables, which a database
// without them could not be opened with.
func openEarlier(t *testing.T, path string, set fs.FS) {
	t.Helper()
	var tables []authored.Table
	for _, table := range authored.AppTables() {
		if table.Name != "pot_settings" && table.Name != "pot_lines" {
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

func TestThePotsMigrationIsInTheEmbeddedSet(t *testing.T) {
	names, err := fs.Glob(withMigrations(t, nil), "0005_*.sql")
	if err != nil || !equalStrings(names, []string{"0005_pots.sql"}) {
		t.Errorf("0005_* = %v (%v), want 0005_pots.sql", names, err)
	}
}

// Each refusal comes from the database itself, with no Go validation in the
// way, and is preceded by the valid rows it differs from. The checks here are
// the ones an error in a handler would have to get past.
func TestThePotTablesRefuseWhatIsNotAValidRow(t *testing.T) {
	cases := []struct{ name, invalid string }{
		{"a second link for one line to the same pot", `INSERT INTO pot_lines (line_id, pot_id) VALUES (2, 'investment:inv-1')`},
		{"a second link for one line to another pot", `INSERT INTO pot_lines (line_id, pot_id) VALUES (2, 'reserve:acc-pots/pot-1')`},
		{"a link to a line that is not in lines", `INSERT INTO pot_lines (line_id, pot_id) VALUES (99, 'investment:inv-1')`},
		{"a link with no line", `INSERT INTO pot_lines (line_id, pot_id) VALUES (NULL, 'investment:inv-1')`},
		{"a link with no pot", `INSERT INTO pot_lines (line_id, pot_id) VALUES (3, NULL)`},
		{"a link to a pot with an empty id", `INSERT INTO pot_lines (line_id, pot_id) VALUES (3, '')`},
		{"a link to a slot", `INSERT INTO pot_lines (line_id, pot_id) VALUES (3, 'slot:acc-pots/1')`},
		{"the bucket Outro", `INSERT INTO pot_settings (pot_id, bucket) VALUES ('investment:inv-1', 'Outro')`},
		{"a bucket without its accent", `INSERT INTO pot_settings (pot_id, bucket) VALUES ('investment:inv-1', 'Disponivel')`},
		{"a bucket in lower case", `INSERT INTO pot_settings (pot_id, bucket) VALUES ('investment:inv-1', 'reservado')`},
		{"an empty bucket", `INSERT INTO pot_settings (pot_id, bucket) VALUES ('investment:inv-1', '')`},
		{"a bucket written as a number", `INSERT INTO pot_settings (pot_id, bucket) VALUES ('investment:inv-1', 1)`},
		{"a second row for one pot", `INSERT INTO pot_settings (pot_id, bucket) VALUES ('reserve:acc-pots/pot-1', 'Investido')`},
		{"a bucket for a pot with an empty id", `INSERT INTO pot_settings (pot_id, bucket) VALUES ('', 'Investido')`},
		{"a bucket for a slot", `INSERT INTO pot_settings (pot_id, bucket) VALUES ('slot:acc-pots/1', 'Investido')`},
		{"a bucket for no pot", `INSERT INTO pot_settings (pot_id, bucket) VALUES (NULL, 'Investido')`},
		{"a line deleted while it is linked", `DELETE FROM lines WHERE id = 2`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := migratedPool(t)
			for _, valid := range []string{mintTwoLines, linkLineTwo, bucketForPotOf} {
				mustExec(t, database, valid)
			}
			if _, err := database.Exec(c.invalid); err == nil {
				t.Errorf("the database accepted %s", c.invalid)
			}
		})
	}
}

func TestThePotTablesAcceptEachBucketAndManyLinesForOnePot(t *testing.T) {
	for _, bucket := range []string{"Disponível", "Reservado", "Investido"} {
		database := migratedPool(t)
		if _, err := database.Exec(`INSERT INTO pot_settings (pot_id, bucket) VALUES ('reserve:acc-pots/pot-1', ?)`, bucket); err != nil {
			t.Errorf("the database refused the bucket %s: %v", bucket, err)
		}
	}
	database := migratedPool(t)
	mustExec(t, database, `INSERT INTO lines (builtin) VALUES (0), (0), (0)`)
	for _, line := range []int{2, 3, 4} {
		if _, err := database.Exec(`INSERT INTO pot_lines (line_id, pot_id) VALUES (?, 'investment:inv-1')`, line); err != nil {
			t.Errorf("the database refused line %d on a pot that already has a line: %v", line, err)
		}
	}
	for _, pot := range []string{"reserve:acc-pots/pot-1", "investment:inv-1", "slotted:a"} {
		if _, err := database.Exec(`INSERT INTO pot_settings (pot_id, bucket) VALUES (?, NULL)`, pot); err != nil {
			t.Errorf("the database refused the pot %s with no bucket: %v", pot, err)
		}
	}
}
