package authored

// The budget's journaled tables and every statement over its tables. A version
// and its lines are a parent and its children; a line's key is its whole
// identity (version, id, name), so a rename is one row removed and one written
// and the uniqueness of names is never met halfway by an update.

const (
	statementBudgetVersionRow    = `SELECT from_month FROM budget_versions WHERE from_month = ?`
	statementBudgetVersionAll    = `SELECT from_month FROM budget_versions`
	statementBudgetVersionInsert = `INSERT INTO budget_versions (from_month) VALUES (?)`
	statementBudgetVersionDelete = `DELETE FROM budget_versions WHERE from_month = ?`

	budgetLineColumns = `from_month, group_key, position, line_id, name, cents, flag`

	statementBudgetLineRow = `SELECT ` + budgetLineColumns + `
FROM budget_version_lines WHERE from_month = ? AND line_id = ? AND name = ?`
	statementBudgetLineAll    = `SELECT ` + budgetLineColumns + ` FROM budget_version_lines`
	statementBudgetLineOwned  = `SELECT ` + budgetLineColumns + ` FROM budget_version_lines WHERE from_month = ?`
	statementBudgetLineInsert = `
INSERT INTO budget_version_lines (` + budgetLineColumns + `) VALUES (?, ?, ?, ?, ?, ?, ?)`
	statementBudgetLineUpdate = `
UPDATE budget_version_lines SET group_key = ?, position = ?, cents = ?, flag = ?
WHERE from_month = ? AND line_id = ? AND name = ?`
	statementBudgetLineDelete = `DELETE FROM budget_version_lines WHERE from_month = ? AND line_id = ? AND name = ?`

	statementScenarioRow    = `SELECT name, position, doc FROM scenarios WHERE name = ?`
	statementScenarioAll    = `SELECT name, position, doc FROM scenarios`
	statementScenarioInsert = `INSERT INTO scenarios (name, position, doc) VALUES (?, ?, ?)`
	statementScenarioUpdate = `UPDATE scenarios SET position = ?, doc = ? WHERE name = ?`
	statementScenarioDelete = `DELETE FROM scenarios WHERE name = ?`

	statementClosedMonthRow    = `SELECT month, closed_on FROM closed_months WHERE month = ?`
	statementClosedMonthAll    = `SELECT month, closed_on FROM closed_months`
	statementClosedMonthInsert = `INSERT INTO closed_months (month, closed_on) VALUES (?, ?)`
	statementClosedMonthUpdate = `UPDATE closed_months SET closed_on = ? WHERE month = ?`
	statementClosedMonthDelete = `DELETE FROM closed_months WHERE month = ?`
)

var budgetVersionsTable = Table{
	Name:    "budget_versions",
	Shape:   ShapeSet,
	Columns: []Column{{Name: "from_month"}},
	Key:     []string{"from_month"},
	Statements: TableStatements{
		SelectRow: statementBudgetVersionRow, SelectAll: statementBudgetVersionAll,
		Insert: statementBudgetVersionInsert, Delete: statementBudgetVersionDelete,
	},
}

var budgetVersionLinesTable = Table{
	Name:  "budget_version_lines",
	Shape: ShapeSet,
	Columns: []Column{
		{Name: "from_month"}, {Name: "group_key"}, {Name: "position"}, {Name: "line_id"},
		{Name: "name"}, {Name: "cents"}, {Name: "flag"},
	},
	Key: []string{"from_month", "line_id", "name"},
	// A line is the person's budget, so a later save that only added one to a
	// version keeps the save that created the version from being undone.
	OwnedBy: &Ownership{Owner: "budget_versions", Columns: []string{"from_month"}, OnUndo: RowsHoldTheirOwner},
	Statements: TableStatements{
		SelectRow: statementBudgetLineRow, SelectAll: statementBudgetLineAll, SelectOwned: statementBudgetLineOwned,
		Insert: statementBudgetLineInsert, Update: statementBudgetLineUpdate, Delete: statementBudgetLineDelete,
	},
}

var scenariosTable = Table{
	Name:    "scenarios",
	Shape:   ShapeSet,
	Columns: []Column{{Name: "name"}, {Name: "position"}, {Name: "doc"}},
	Key:     []string{"name"},
	Statements: TableStatements{
		SelectRow: statementScenarioRow, SelectAll: statementScenarioAll,
		Insert: statementScenarioInsert, Update: statementScenarioUpdate, Delete: statementScenarioDelete,
	},
}

var closedMonthsTable = Table{
	Name:    "closed_months",
	Shape:   ShapeSet,
	Columns: []Column{{Name: "month"}, {Name: "closed_on"}},
	Key:     []string{"month"},
	Statements: TableStatements{
		SelectRow: statementClosedMonthRow, SelectAll: statementClosedMonthAll,
		Insert: statementClosedMonthInsert, Update: statementClosedMonthUpdate, Delete: statementClosedMonthDelete,
	},
}

// budgetTables are the budget's entries of the registry.
var budgetTables = []Table{budgetVersionsTable, budgetVersionLinesTable, scenariosTable, closedMonthsTable}

// The budget's reads and the one statement that mints.
const (
	// statementBudgetVersionInForce is the version a month uses: the latest
	// that starts at or before it, else the first. Null when there is none.
	statementBudgetVersionInForce = `
SELECT coalesce(
  (SELECT max(from_month) FROM budget_versions WHERE from_month <= ?),
  (SELECT min(from_month) FROM budget_versions))`
	statementBudgetLinesOfVersion = `
SELECT group_key, position, line_id, name, cents, flag FROM budget_version_lines WHERE from_month = ?`
	statementBudgetVersionsAfter = `SELECT from_month FROM budget_versions WHERE from_month > ? ORDER BY from_month`
	statementClosedMonthsInOrder = `SELECT month FROM closed_months ORDER BY month`
	statementLineBuiltin         = `SELECT builtin FROM lines WHERE id = ?`
	statementMintLine            = `INSERT INTO lines (builtin) VALUES (0)`
	statementScenariosInOrder    = `SELECT name, doc FROM scenarios ORDER BY position`
	statementLastScenarioPlace   = `SELECT coalesce(max(position), -1) FROM scenarios`
)
