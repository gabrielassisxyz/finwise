package authored_test

import (
	"database/sql"
	"testing"
)

func migratedPool(t *testing.T) *sql.DB {
	t.Helper()
	return openDatabase(t, databasePath(t), creationInstant, nil).Pool()
}

func mustExec(t *testing.T, database *sql.DB, statement string) {
	t.Helper()
	if _, err := database.Exec(statement); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

// Each refusal is preceded by the valid row it differs from, so a refusal can
// only come from the constraint under test.
func TestTheJournalSchemaRefusesWhatIsNotAValidRow(t *testing.T) {
	cases := []struct {
		name, valid, invalid string
	}{
		{"a duplicate effect", `INSERT INTO change_effects (change_id, seq, tbl, key, after) VALUES (1, 1, 'lines', '["a"]', '1')`,
			`INSERT INTO change_effects (change_id, seq, tbl, key, after) VALUES (1, 1, 'lines', '["b"]', '2')`},
		{"a duplicate meta key", `INSERT INTO meta (key, value) VALUES ('last_backup_at', 'x')`,
			`INSERT INTO meta (key, value) VALUES ('last_backup_at', 'y')`},
		{"an author outside the three", insertChange,
			`INSERT INTO changes (at, day, author, method) VALUES ('2026-12-01T01:30:00.000Z', '2026-11-30', 'admin', 'm')`},
		{"a change without at", insertChange,
			`INSERT INTO changes (day, author, method) VALUES ('2026-11-30', 'person', 'm')`},
		{"a change without day", insertChange,
			`INSERT INTO changes (at, author, method) VALUES ('2026-12-01T01:30:00.000Z', 'person', 'm')`},
		{"a change without author", insertChange,
			`INSERT INTO changes (at, day, method) VALUES ('2026-12-01T01:30:00.000Z', '2026-11-30', 'm')`},
		{"a change without method", insertChange,
			`INSERT INTO changes (at, day, author) VALUES ('2026-12-01T01:30:00.000Z', '2026-11-30', 'person')`},
		{"extended outside its two values",
			`INSERT INTO changes (at, day, author, method, extended) VALUES ('2026-12-01T01:30:00.000Z', '2026-11-30', 'rule', 'm', 1)`,
			`INSERT INTO changes (at, day, author, method, extended) VALUES ('2026-12-01T01:30:00.000Z', '2026-11-30', 'rule', 'm', 2)`},
		{"an instant that is not UTC with milliseconds", insertChange,
			`INSERT INTO changes (at, day, author, method) VALUES ('2026-12-01 01:30:00', '2026-11-30', 'person', 'm')`},
		{"a day that is not YYYY-MM-DD", insertChange,
			`INSERT INTO changes (at, day, author, method) VALUES ('2026-12-01T01:30:00.000Z', '30/11/2026', 'person', 'm')`},
		{"an effect of a change that does not exist",
			`INSERT INTO change_effects (change_id, seq, tbl, key, after) VALUES (1, 1, 'lines', '["a"]', '1')`,
			`INSERT INTO change_effects (change_id, seq, tbl, key, after) VALUES (2, 1, 'lines', '["a"]', '1')`},
		{"an effect whose key is not a JSON array",
			`INSERT INTO change_effects (change_id, seq, tbl, key, after) VALUES (1, 1, 'lines', '["a"]', '1')`,
			`INSERT INTO change_effects (change_id, seq, tbl, key, after) VALUES (1, 2, 'lines', '"a"', '1')`},
		{"an effect with neither side",
			`INSERT INTO change_effects (change_id, seq, tbl, key, before) VALUES (1, 1, 'lines', '["a"]', 'null')`,
			`INSERT INTO change_effects (change_id, seq, tbl, key) VALUES (1, 2, 'lines', '["a"]')`},
		{"a tracking start month that is not YYYY-MM", `INSERT INTO meta (key, value) VALUES ('other', 'x')`,
			`UPDATE meta SET value = 'November' WHERE key = 'tracking_start_month'`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := migratedPool(t)
			mustExec(t, database, insertChange)
			mustExec(t, database, c.valid)
			if _, err := database.Exec(c.invalid); err == nil {
				t.Errorf("the database accepted %s", c.invalid)
			}
		})
	}
}

func TestAChangeIdIsNeverReused(t *testing.T) {
	database := migratedPool(t)
	mustExec(t, database, insertChange)
	mustExec(t, database, insertChange)
	mustExec(t, database, "DELETE FROM changes WHERE id = 2")
	mustExec(t, database, insertChange)
	var highest int
	if err := database.QueryRow("SELECT max(id) FROM changes").Scan(&highest); err != nil {
		t.Fatal(err)
	}
	if highest != 3 {
		t.Errorf("the row after deleting id 2 got id %d, want 3", highest)
	}
}
