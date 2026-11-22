package authored_test

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

func TestTheBudgetMigrationIsRecordedOnceAndSeedsOneLineOnce(t *testing.T) {
	path := databasePath(t)
	openAndClose(t, path, creationInstant, nil)
	openAndClose(t, path, laterInstant, nil)
	names := namesOf(t, path)
	count := 0
	for _, name := range names {
		if name == "0002_budget" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("0002_budget is recorded %d times in %v", count, names)
	}
	var builtin int
	if err := inspect(t, path).QueryRow("SELECT count(*) FROM lines WHERE builtin = 1").Scan(&builtin); err != nil || builtin != 1 {
		t.Errorf("builtin lines = %d (%v), want 1", builtin, err)
	}
	var total int
	if err := inspect(t, path).QueryRow("SELECT count(*) FROM lines").Scan(&total); err != nil || total != 1 {
		t.Errorf("lines = %d (%v), want only the seeded one after two starts", total, err)
	}
}

func TestTheFeesLineHasTheSameIDInEveryDatabase(t *testing.T) {
	var ids [2]int64
	for i := range ids {
		database := migratedPool(t)
		if err := database.QueryRow("SELECT id FROM lines WHERE builtin = 1").Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	if ids[0] != ids[1] || ids[0] == 0 {
		t.Errorf("the fees line is %d in one database and %d in another", ids[0], ids[1])
	}
}

const (
	seedVersion = `INSERT INTO budget_versions (from_month) VALUES ('2026-02')`
	seedLines   = `INSERT INTO lines (builtin) VALUES (0), (0)`
	seedLine    = `INSERT INTO budget_version_lines (from_month, group_key, position, line_id, name, cents, flag)
VALUES ('2026-02', 'var', 0, 2, 'Mercado', 40000, NULL)`
)

// Each refusal comes from the database itself, with no Go validation in the
// way, and is preceded by the valid row it differs from.
func TestTheBudgetSchemaRefusesWhatIsNotAValidRow(t *testing.T) {
	cases := []struct{ name, invalid string }{
		{"the same name twice in one version", `INSERT INTO budget_version_lines VALUES ('2026-02', 'fix', 0, 3, 'Mercado', 1, NULL)`},
		{"the same line id twice in one version", `INSERT INTO budget_version_lines VALUES ('2026-02', 'fix', 0, 2, 'Outro', 1, NULL)`},
		{"a group key of xyz", `INSERT INTO budget_version_lines VALUES ('2026-02', 'xyz', 0, 3, 'Outro', 1, NULL)`},
		{"a line of -1 cents", `INSERT INTO budget_version_lines VALUES ('2026-02', 'fix', 0, 3, 'Outro', -1, NULL)`},
		{"a line id that is not in lines", `INSERT INTO budget_version_lines VALUES ('2026-02', 'fix', 0, 99, 'Outro', 1, NULL)`},
		{"a version that does not exist", `INSERT INTO budget_version_lines VALUES ('2026-09', 'fix', 0, 3, 'Outro', 1, NULL)`},
		{"a name of blanks", `INSERT INTO budget_version_lines VALUES ('2026-02', 'fix', 0, 3, '  ', 1, NULL)`},
		{"an empty flag", `INSERT INTO budget_version_lines VALUES ('2026-02', 'fix', 0, 3, 'Outro', 1, '')`},
		{"a second built-in line", `INSERT INTO lines (builtin) VALUES (1)`},
		{"a built-in value of 2", `INSERT INTO lines (builtin) VALUES (2)`},
		{"a version that is not a month", `INSERT INTO budget_versions (from_month) VALUES ('2026-2')`},
		{"a scenario with no name", `INSERT INTO scenarios (name, position, doc) VALUES ('', 5, '[]')`},
		{"a scenario at a place that is taken", `INSERT INTO scenarios (name, position, doc) VALUES ('B', 0, '[]')`},
		{"a scenario whose document is not JSON", `INSERT INTO scenarios (name, position, doc) VALUES ('B', 5, 'x')`},
		{"a closed month that is not a month", `INSERT INTO closed_months (month, closed_on) VALUES ('March', '2026-04-01')`},
		{"a closed month closed on no day", `INSERT INTO closed_months (month, closed_on) VALUES ('2026-03', 'April')`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			database := migratedPool(t)
			for _, valid := range []string{seedVersion, seedLines, seedLine,
				`INSERT INTO scenarios (name, position, doc) VALUES ('A', 0, '[]')`,
				`INSERT INTO closed_months (month, closed_on) VALUES ('2026-03', '2026-04-01')`} {
				mustExec(t, database, valid)
			}
			if _, err := database.Exec(c.invalid); err == nil {
				t.Errorf("the database accepted %s", c.invalid)
			}
		})
	}
}

func TestALineIDIsNeverReused(t *testing.T) {
	database := migratedPool(t)
	mustExec(t, database, seedLines)
	mustExec(t, database, "DELETE FROM lines WHERE id = 3")
	mustExec(t, database, "INSERT INTO lines (builtin) VALUES (0)")
	var highest int
	if err := database.QueryRow("SELECT max(id) FROM lines").Scan(&highest); err != nil || highest != 4 {
		t.Errorf("the line after deleting id 3 got id %d (%v), want 4", highest, err)
	}
}

func TestTheBudgetTablesAreJournaledAndLinesAreWrittenOnlyByTheMint(t *testing.T) {
	journaled := map[string]bool{}
	for _, table := range authored.ProductionTables() {
		journaled[table.Name] = true
	}
	for _, name := range []string{"budget_versions", "budget_version_lines", "scenarios", "closed_months"} {
		if !journaled[name] {
			t.Errorf("%s is not in the recorder's registry", name)
		}
	}
	if journaled["lines"] {
		t.Error("lines is journaled; an id once minted exists for good")
	}
	for name, statement := range authored.AllStatements() {
		for _, match := range writeTarget.FindAllStringSubmatch(statement, -1) {
			if match[1] == "lines" && name != "statementMintLine" {
				t.Errorf("%s writes lines; only the statement that mints an id may", name)
			}
		}
	}
}
