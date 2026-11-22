package authored_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

var transactionTables = []string{
	"establishment_tags", "establishments", "history_step_reasons", "history_steps",
	"tx_ids", "tx_index", "tx_split_parts", "tx_state", "tx_tags",
}

// withoutMigration is the embedded set less one file, as a binary built before
// it carried the set.
func withoutMigration(t testing.TB, name string) fstest.MapFS {
	t.Helper()
	set := withMigrations(t, nil)
	if _, found := set[name+".sql"]; !found {
		t.Fatalf("the embedded set has no %s", name)
	}
	delete(set, name+".sql")
	return set
}

// registryBeforeTheTransactions is the registry of a binary that did not carry
// the transactions' tables yet.
func registryBeforeTheTransactions(t testing.TB) *authored.Registry {
	t.Helper()
	var tables []authored.Table
	for _, table := range authored.ProductionTables() {
		if !slices.Contains(transactionTables, table.Name) {
			tables = append(tables, table)
		}
	}
	registry, err := authored.NewRegistry(tables)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestADatabaseThatHeldTheBudgetGainsTheTransactionsTablesOnce(t *testing.T) {
	path := databasePath(t)
	earlier, err := authored.Open(t.Context(), path, authored.Options{
		Now: clockAt(t, creationInstant), Migrations: withoutMigration(t, "0003_transactions"), Journal: registryBeforeTheTransactions(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := earlier.Close(); err != nil {
		t.Fatal(err)
	}
	before := tableNames(t, inspect(t, path))
	if !slices.Contains(before, "budget_versions") || slices.Contains(before, "tx_index") {
		t.Fatalf("the database before the migration holds %v", before)
	}

	openAndClose(t, path, laterInstant, nil)
	after := tableNames(t, inspect(t, path))
	for _, name := range transactionTables {
		if !slices.Contains(after, name) {
			t.Errorf("%s is missing after the migration: %v", name, after)
		}
	}
	if gained := len(after) - len(before); gained != len(transactionTables) {
		t.Errorf("the migration added %d tables, want %d", gained, len(transactionTables))
	}
	recorded := namesOf(t, path)
	if count := strings.Count(strings.Join(recorded, " "), "0003_transactions"); count != 1 {
		t.Errorf("0003_transactions is recorded %d times in %v", count, recorded)
	}
	if copies := copiesBeside(t, path); !equalStrings(copies, []string{copyName}) {
		t.Errorf("copies = %v, want the one taken before migrating", copies)
	}

	migrated := recordedOf(t, path)
	openAndClose(t, path, "2027-03-01T12:00:00Z", nil)
	if again := recordedOf(t, path); !equalStrings(again, migrated) {
		t.Errorf("a second start changed schema_migrations: %v -> %v", migrated, again)
	}
	if copies := copiesBeside(t, path); len(copies) != 1 {
		t.Errorf("a second start took a copy: %v", copies)
	}
}

// The valid rows every refusal below starts from: a change, three indexed
// transactions, and one row of each authored table. Line 1 is the fees line,
// which every database has.
var transactionSeed = []string{
	insertChange,
	`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc', '2026-11-30', -1200, 'name:a', 0)`,
	`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc', '2026-11-30', 500, 'doc:123', 1)`,
	`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc', '2026-11-30', 700, 'text:b', 0)`,
	`INSERT INTO tx_ids (provider_id, tx_key) VALUES ('p1', 1)`,
	`INSERT INTO tx_state (tx_key, line_id, classified_by, credit_tx_key, credit_cents) VALUES (1, 1, 'person', 2, 500)`,
	`INSERT INTO tx_tags (tx_key, tag, position) VALUES (1, '#a', 0)`,
	`INSERT INTO tx_split_parts (tx_key, position, line_id, cents) VALUES (1, 0, 1, 100)`,
	`INSERT INTO establishments (key, nickname, default_line_id) VALUES ('name:a', 'A', 1)`,
	`INSERT INTO establishment_tags (key, tag, position) VALUES ('name:a', '#a', 0)`,
	`INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (1, 1, 'step', 'Você colocou em Tarifas', '2026-11-30')`,
	`INSERT INTO history_step_reasons (step_id, reason) VALUES (1, 'porque')`,
}

// Each refusal comes from the database itself, with no Go validation in the
// way. Each is paired with the valid row it differs from, which the database
// accepts on the same seed, so a refusal can only come from the rule under test.
func TestTheTransactionsSchemaRefusesWhatIsNotAValidRow(t *testing.T) {
	cases := []struct{ name, valid, invalid string }{
		{"a second row for one provider id",
			`INSERT INTO tx_ids (provider_id, tx_key) VALUES ('p2', 2)`, `INSERT INTO tx_ids (provider_id, tx_key) VALUES ('p1', 2)`},
		{"an empty provider id",
			`INSERT INTO tx_ids (provider_id, tx_key) VALUES ('p2', 2)`, `INSERT INTO tx_ids (provider_id, tx_key) VALUES ('', 2)`},
		{"a provider id mapped to no transaction",
			`INSERT INTO tx_ids (provider_id, tx_key) VALUES ('p2', 3)`, `INSERT INTO tx_ids (provider_id, tx_key) VALUES ('p2', 99)`},
		{"two purchases linking one credit",
			`INSERT INTO tx_state (tx_key, credit_tx_key) VALUES (3, 1)`, `INSERT INTO tx_state (tx_key, credit_tx_key) VALUES (3, 2)`},
		{"a credit that is no transaction",
			`INSERT INTO tx_state (tx_key, credit_tx_key) VALUES (3, 1)`, `INSERT INTO tx_state (tx_key, credit_tx_key) VALUES (3, 99)`},
		{"the same tag twice on one transaction",
			`INSERT INTO tx_tags (tx_key, tag, position) VALUES (1, '#b', 1)`, `INSERT INTO tx_tags (tx_key, tag, position) VALUES (1, '#a', 1)`},
		{"the same tag twice on one establishment",
			`INSERT INTO establishment_tags (key, tag, position) VALUES ('name:a', '#b', 1)`,
			`INSERT INTO establishment_tags (key, tag, position) VALUES ('name:a', '#a', 1)`},
		{"an empty tag",
			`INSERT INTO tx_tags (tx_key, tag, position) VALUES (2, '#b', 0)`, `INSERT INTO tx_tags (tx_key, tag, position) VALUES (2, '', 0)`},
		{"a tag at a negative position",
			`INSERT INTO tx_tags (tx_key, tag, position) VALUES (2, '#b', 0)`, `INSERT INTO tx_tags (tx_key, tag, position) VALUES (2, '#b', -1)`},
		{"the same place twice in one split",
			`INSERT INTO tx_split_parts (tx_key, position, line_id, cents) VALUES (1, 1, NULL, 0)`,
			`INSERT INTO tx_split_parts (tx_key, position, line_id, cents) VALUES (1, 0, NULL, 0)`},
		{"a split part of -1 cents",
			`INSERT INTO tx_split_parts (tx_key, position, line_id, cents) VALUES (1, 1, NULL, 0)`,
			`INSERT INTO tx_split_parts (tx_key, position, line_id, cents) VALUES (1, 1, NULL, -1)`},
		{"an expected amount of -1 cents",
			`INSERT INTO tx_state (tx_key, expected_cents) VALUES (3, 0)`, `INSERT INTO tx_state (tx_key, expected_cents) VALUES (3, -1)`},
		{"a credit of -1 cents",
			`INSERT INTO tx_state (tx_key, credit_cents) VALUES (3, 0)`, `INSERT INTO tx_state (tx_key, credit_cents) VALUES (3, -1)`},
		{"a classifier outside the three",
			`INSERT INTO tx_state (tx_key, classified_by) VALUES (3, 'model')`, `INSERT INTO tx_state (tx_key, classified_by) VALUES (3, 'Você')`},
		{"a confidence over a hundred",
			`INSERT INTO tx_state (tx_key, confidence) VALUES (3, 100)`, `INSERT INTO tx_state (tx_key, confidence) VALUES (3, 101)`},
		{"a suggestion's confidence under zero",
			`INSERT INTO tx_state (tx_key, pending_confidence) VALUES (3, 0)`, `INSERT INTO tx_state (tx_key, pending_confidence) VALUES (3, -1)`},
		{"an empty pending reason",
			`INSERT INTO tx_state (tx_key, pending_reason) VALUES (3, 'r')`, `INSERT INTO tx_state (tx_key, pending_reason) VALUES (3, '')`},
		{"a refund of 2", `INSERT INTO tx_state (tx_key, refund) VALUES (3, 0)`, `INSERT INTO tx_state (tx_key, refund) VALUES (3, 2)`},
		{"an ask_reason of 2", `INSERT INTO tx_state (tx_key, ask_reason) VALUES (3, 1)`, `INSERT INTO tx_state (tx_key, ask_reason) VALUES (3, 2)`},
		{"a reimbursable of 2", `INSERT INTO tx_state (tx_key, reimbursable) VALUES (3, 1)`, `INSERT INTO tx_state (tx_key, reimbursable) VALUES (3, 2)`},
		{"a bank_pending of 2",
			`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc', '2026-11-30', 1, 'name:c', 1)`,
			`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc', '2026-11-30', 1, 'name:c', 2)`},
		{"an evaluated of 2",
			`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending, evaluated) VALUES ('acc', '2026-11-30', 1, 'name:c', 0, 1)`,
			`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending, evaluated) VALUES ('acc', '2026-11-30', 1, 'name:c', 0, 2)`},
		{"an index day that is not YYYY-MM-DD",
			`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc', '2026-11-30', 1, 'name:c', 0)`,
			`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc', '30/11/2026', 1, 'name:c', 0)`},
		{"an establishment key without its prefix",
			`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc', '2026-11-30', 1, 'name:c', 0)`,
			`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc', '2026-11-30', 1, 'c', 0)`},
		{"an empty account",
			`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc', '2026-11-30', 1, 'name:c', 0)`,
			`INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('', '2026-11-30', 1, 'name:c', 0)`},
		{"a line that is not in lines",
			`INSERT INTO tx_state (tx_key, line_id) VALUES (3, 1)`, `INSERT INTO tx_state (tx_key, line_id) VALUES (3, 99)`},
		{"a suggested line that is not in lines",
			`INSERT INTO tx_state (tx_key, pending_line_id) VALUES (3, 1)`, `INSERT INTO tx_state (tx_key, pending_line_id) VALUES (3, 99)`},
		{"a refund line that is not in lines",
			`INSERT INTO tx_state (tx_key, refund_line_id) VALUES (3, 1)`, `INSERT INTO tx_state (tx_key, refund_line_id) VALUES (3, 99)`},
		{"a split part's line that is not in lines",
			`INSERT INTO tx_split_parts (tx_key, position, line_id, cents) VALUES (1, 1, 1, 0)`,
			`INSERT INTO tx_split_parts (tx_key, position, line_id, cents) VALUES (1, 1, 99, 0)`},
		{"a default line that is not in lines",
			`INSERT INTO establishments (key, default_line_id) VALUES ('name:b', 1)`, `INSERT INTO establishments (key, default_line_id) VALUES ('name:b', 99)`},
		{"an empty nickname",
			`INSERT INTO establishments (key, nickname) VALUES ('name:b', 'B')`, `INSERT INTO establishments (key, nickname) VALUES ('name:b', '')`},
		{"an establishment without its prefix",
			`INSERT INTO establishments (key, nickname) VALUES ('doc:9', 'B')`, `INSERT INTO establishments (key, nickname) VALUES ('9', 'B')`},
		{"a fixed tag of an establishment without its prefix",
			`INSERT INTO establishment_tags (key, tag, position) VALUES ('text:b', '#a', 0)`,
			`INSERT INTO establishment_tags (key, tag, position) VALUES ('b', '#a', 0)`},
		{"a state of no transaction", `INSERT INTO tx_state (tx_key, note) VALUES (3, 'n')`, `INSERT INTO tx_state (tx_key, note) VALUES (99, 'n')`},
		{"a tag of no transaction",
			`INSERT INTO tx_tags (tx_key, tag, position) VALUES (3, '#a', 0)`, `INSERT INTO tx_tags (tx_key, tag, position) VALUES (99, '#a', 0)`},
		{"a split part of no transaction",
			`INSERT INTO tx_split_parts (tx_key, position, cents) VALUES (3, 0, 1)`, `INSERT INTO tx_split_parts (tx_key, position, cents) VALUES (99, 0, 1)`},
		{"a step of no transaction",
			`INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (3, 1, 'note', 'x', '2026-11-30')`,
			`INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (99, 1, 'note', 'x', '2026-11-30')`},
		{"a step of no change",
			`INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (1, 1, 'note', 'x', '2026-11-30')`,
			`INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (1, 99, 'note', 'x', '2026-11-30')`},
		{"a step of a kind outside the two",
			`INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (1, 1, 'note', 'x', '2026-11-30')`,
			`INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (1, 1, 'undo', 'x', '2026-11-30')`},
		{"a step without a sentence",
			`INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (1, 1, 'note', 'x', '2026-11-30')`,
			`INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (1, 1, 'note', '', '2026-11-30')`},
		{"a step on a day that is not YYYY-MM-DD",
			`INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (1, 1, 'note', 'x', '2026-11-30')`,
			`INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (1, 1, 'note', 'x', '2026-11')`},
		{"a second reason for one step",
			`UPDATE history_step_reasons SET reason = 'outra' WHERE step_id = 1`, `INSERT INTO history_step_reasons (step_id, reason) VALUES (1, 'outra')`},
		{"a reason whose step does not exist",
			`INSERT INTO history_steps (id, tx_key, change_id, kind, text, day) VALUES (2, 1, 1, 'note', 'x', '2026-11-30');
INSERT INTO history_step_reasons (step_id, reason) VALUES (2, 'porque')`,
			`INSERT INTO history_step_reasons (step_id, reason) VALUES (2, 'porque')`},
		{"an empty reason",
			`UPDATE history_step_reasons SET reason = 'outra' WHERE step_id = 1`, `UPDATE history_step_reasons SET reason = '' WHERE step_id = 1`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := migratedPool(t)
			for _, valid := range transactionSeed {
				mustExec(t, database, valid)
			}
			tx, err := database.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(c.valid); err != nil {
				t.Errorf("the valid twin %s was refused: %v", c.valid, err)
			}
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if _, err := database.Exec(c.invalid); err == nil {
				t.Errorf("the database accepted %s", c.invalid)
			}
		})
	}
}

func TestATransactionKeyAndAStepIDAreNeverReused(t *testing.T) {
	database := migratedPool(t)
	for _, valid := range transactionSeed {
		mustExec(t, database, valid)
	}
	mustExec(t, database, "DELETE FROM history_step_reasons")
	mustExec(t, database, "DELETE FROM history_steps")
	mustExec(t, database, `INSERT INTO history_steps (tx_key, change_id, kind, text, day) VALUES (1, 1, 'note', 'x', '2026-11-30')`)
	mustExec(t, database, `DELETE FROM tx_index WHERE tx_key = 3`)
	mustExec(t, database, `INSERT INTO tx_index (account_id, day, cents, establishment_key, bank_pending) VALUES ('acc', '2026-11-30', 1, 'name:c', 0)`)
	var step, key int
	if err := database.QueryRow("SELECT max(id) FROM history_steps").Scan(&step); err != nil || step != 2 {
		t.Errorf("the step after deleting id 1 got id %d (%v), want 2", step, err)
	}
	if err := database.QueryRow("SELECT max(tx_key) FROM tx_index").Scan(&key); err != nil || key != 4 {
		t.Errorf("the transaction after deleting key 3 got key %d (%v), want 4", key, err)
	}
}

func TestTheTransactionsTablesAreJournaledWithTheirShapes(t *testing.T) {
	type shape struct {
		shape authored.Shape
		kind  authored.Kind
		key   string
		owner string
	}
	want := map[string]shape{
		"tx_state":             {authored.ShapeAttribute, authored.KindOptionalFacts, "tx_key", ""},
		"establishments":       {authored.ShapeAttribute, authored.KindOptionalFacts, "key", ""},
		"tx_tags":              {authored.ShapeSet, 0, "tx_key tag", ""},
		"tx_split_parts":       {authored.ShapeSet, 0, "tx_key position", ""},
		"establishment_tags":   {authored.ShapeSet, 0, "key tag", ""},
		"history_steps":        {authored.ShapeSet, 0, "id", ""},
		"history_step_reasons": {authored.ShapeSet, 0, "step_id", "history_steps step_id"},
	}
	got := map[string]shape{}
	for _, table := range authored.ProductionTables() {
		if _, ours := want[table.Name]; !ours {
			continue
		}
		entry := shape{shape: table.Shape, kind: table.Kind, key: strings.Join(table.Key, " ")}
		if table.OwnedBy != nil {
			entry.owner = table.OwnedBy.Owner + " " + strings.Join(table.OwnedBy.Columns, " ")
		}
		got[table.Name] = entry
	}
	for name, entry := range want {
		if got[name] != entry {
			t.Errorf("%s is registered as %+v, want %+v", name, got[name], entry)
		}
	}
	for _, name := range []string{"tx_index", "tx_ids"} {
		if _, excluded := authored.NotJournaledTables()[name]; !excluded {
			t.Errorf("%s is not on the list of tables deliberately not journaled", name)
		}
	}
}

// A row that holds the schema's defaults must be what the registry reads as no
// row, or a transaction nobody touched would show in the dump and an undo that
// cleared every fact would leave a row behind.
func TestARowAtTheSchemasDefaultsIsNoRowToTheJournal(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	empty := dump(t, database)
	around(t, database, transactionSeed[1])
	around(t, database, `INSERT INTO tx_state (tx_key) VALUES (1)`)
	around(t, database, `INSERT INTO establishments (key) VALUES ('name:a')`)
	if got := dump(t, database); got != empty {
		t.Errorf("rows at the schema's defaults are in the dump:\n%s", got)
	}
}
