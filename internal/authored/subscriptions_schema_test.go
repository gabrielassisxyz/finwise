package authored_test

import (
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const (
	mintLineTwo     = `INSERT INTO lines (builtin) VALUES (0)`
	indexAPurchase  = `INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc-1', '2026-11-20', -6400, 'text:STREAMSVC', 0)`
	keepASubscriber = `
INSERT INTO subscriptions (establishment_key, created_from_tx_key, created_day, created_cents, name, line_id, status, status_day, review_on)
VALUES ('text:STREAMSVC', 1, '2026-11-20', 6400, 'STREAMSVC', 2, 'Analisar', '2026-11-22', '2027-02-22')`
)

func TestTheSubscriptionsMigrationIsRecordedOnceAndAppliedNothingTheSecondTime(t *testing.T) {
	path := databasePath(t)
	openAndClose(t, path, creationInstant, nil)
	openAndClose(t, path, laterInstant, nil)
	got := columnValues(t, inspect(t, path), "SELECT name FROM schema_migrations WHERE name = '0006_subscriptions'")
	if !equalStrings(got, []string{"0006_subscriptions"}) {
		t.Errorf("recorded %v, want 0006_subscriptions once", got)
	}
	if copies := copiesBeside(t, path); len(copies) != 0 {
		t.Errorf("the second open took a copy, so it migrated: %v", copies)
	}
}

// The database the earlier releases left holds 0008_session, numbered above
// this one, so the migration has to be applied whatever its place.
func TestADatabaseThatHeldTheEarlierMigrationsGainsTheSubscriptionsTableWhenOpened(t *testing.T) {
	path := databasePath(t)
	earlier := fstest.MapFS{}
	embedded := withMigrations(t, nil)
	for name, file := range embedded {
		if name != "0006_subscriptions.sql" {
			earlier[name] = file
		}
	}
	if _, found := embedded["0006_subscriptions.sql"]; !found || len(earlier) != len(embedded)-1 || earlier["0008_session.sql"] == nil {
		t.Fatal("the embedded set does not carry 0006_subscriptions below 0008_session, so the test would prove nothing")
	}
	openWithoutSubscriptions(t, path, earlier)
	if tables := tableNames(t, inspect(t, path)); slices.Contains(tables, "subscriptions") {
		t.Fatalf("the earlier database already holds the subscriptions: %v", tables)
	}

	openAndClose(t, path, laterInstant, nil)
	if tables := tableNames(t, inspect(t, path)); !slices.Contains(tables, "subscriptions") {
		t.Errorf("tables after the open = %v, want subscriptions", tables)
	}
	if got := columnValues(t, inspect(t, path), "SELECT name FROM schema_migrations WHERE name = '0006_subscriptions'"); !equalStrings(got, []string{"0006_subscriptions"}) {
		t.Errorf("recorded %v, want 0006_subscriptions once", got)
	}
	if copies := copiesBeside(t, path); len(copies) != 1 {
		t.Errorf("copies = %v, want the one taken before migrating a database that holds data", copies)
	}
}

func openWithoutSubscriptions(t *testing.T, path string, set fs.FS) {
	t.Helper()
	var tables []authored.Table
	for _, table := range authored.AppTables() {
		if table.Name != "subscriptions" {
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

func TestTheSubscriptionsMigrationIsInTheEmbeddedSet(t *testing.T) {
	names, err := fs.Glob(withMigrations(t, nil), "0006_*.sql")
	if err != nil || !equalStrings(names, []string{"0006_subscriptions.sql"}) {
		t.Errorf("0006_* = %v (%v), want 0006_subscriptions.sql", names, err)
	}
}

// Each refusal comes from the database itself, with no Go in the way, after the
// valid rows it differs from: the checks an error in a handler would have to
// get past.
func TestTheSubscriptionsTableRefusesWhatIsNotAValidRow(t *testing.T) {
	insert := func(values string) string {
		return `INSERT INTO subscriptions (establishment_key, created_from_tx_key, created_day, created_cents, name, line_id, status, status_day, review_on) VALUES ` + values
	}
	cases := []struct{ name, invalid string }{
		{"a second subscription of one establishment", insert(`('text:STREAMSVC', 1, '2026-11-20', 6400, 'Outra', NULL, 'Manter', '2026-11-22', '2027-02-22')`)},
		{"the status Pausar", insert(`('text:OUTRA', 1, '2026-11-20', 6400, 'Outra', NULL, 'Pausar', '2026-11-22', '2027-02-22')`)},
		{"a status in lower case", insert(`('text:OUTRA', 1, '2026-11-20', 6400, 'Outra', NULL, 'manter', '2026-11-22', '2027-02-22')`)},
		{"no status", insert(`('text:OUTRA', 1, '2026-11-20', 6400, 'Outra', NULL, NULL, '2026-11-22', '2027-02-22')`)},
		{"a negative amount", insert(`('text:OUTRA', 1, '2026-11-20', -1, 'Outra', NULL, 'Manter', '2026-11-22', '2027-02-22')`)},
		{"a line that is in no lines row", insert(`('text:OUTRA', 1, '2026-11-20', 6400, 'Outra', 99, 'Manter', '2026-11-22', '2027-02-22')`)},
		{"a purchase that is in no index row", insert(`('text:OUTRA', 99, '2026-11-20', 6400, 'Outra', NULL, 'Manter', '2026-11-22', '2027-02-22')`)},
		{"no purchase", insert(`('text:OUTRA', NULL, '2026-11-20', 6400, 'Outra', NULL, 'Manter', '2026-11-22', '2027-02-22')`)},
		{"no establishment", insert(`(NULL, 1, '2026-11-20', 6400, 'Outra', NULL, 'Manter', '2026-11-22', '2027-02-22')`)},
		{"an establishment key of no known form", insert(`('OUTRA', 1, '2026-11-20', 6400, 'Outra', NULL, 'Manter', '2026-11-22', '2027-02-22')`)},
		{"a creation day that is not a day", insert(`('text:OUTRA', 1, '20/11/2026', 6400, 'Outra', NULL, 'Manter', '2026-11-22', '2027-02-22')`)},
		{"a review day that is not a day", insert(`('text:OUTRA', 1, '2026-11-20', 6400, 'Outra', NULL, 'Manter', '2026-11-22', 'amanhã')`)},
		{"an amount written as text", insert(`('text:OUTRA', 1, '2026-11-20', '64,00', 'Outra', NULL, 'Manter', '2026-11-22', '2027-02-22')`)},
		{"no note", `UPDATE subscriptions SET note = NULL`},
		{"the purchase removed from the index", `DELETE FROM tx_index WHERE tx_key = 1`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := migratedPool(t)
			for _, valid := range []string{mintLineTwo, indexAPurchase, keepASubscriber} {
				mustExec(t, database, valid)
			}
			if _, err := database.Exec(c.invalid); err == nil {
				t.Errorf("the database accepted %s", c.invalid)
			}
		})
	}
}

func TestTheSubscriptionsTableAcceptsEachStatusAndASubscriptionWithNoLine(t *testing.T) {
	for index, status := range authored.SubscriptionStatuses {
		database := migratedPool(t)
		mustExec(t, database, indexAPurchase)
		if _, err := database.Exec(`
INSERT INTO subscriptions (establishment_key, created_from_tx_key, created_day, created_cents, name, status, status_day, review_on)
VALUES (?, 1, '2026-11-20', 0, 'STREAMSVC', ?, '2026-11-22', '2027-02-22')`, []string{"text:A", "doc:12345678000199", "name:b", "text:C"}[index], string(status)); err != nil {
			t.Errorf("the database refused the status %s: %v", status, err)
		}
		if got := columnValues(t, database, `SELECT note FROM subscriptions`); !equalStrings(got, []string{""}) {
			t.Errorf("a note left out reads %q, want the empty note", got)
		}
	}
}
