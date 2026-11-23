package authored

// The review's tables, each a set: a step ticked, a transfer made and a warning
// dismissed are facts by existing, so undoing the change that marked one takes
// the row away. The functions that write them belong to the screens that mark
// them.

const (
	closingStepsTable      = "closing_steps"
	transferMarksTable     = "transfer_marks"
	dismissedWarningsTable = "dismissed_warnings"
)

const (
	statementClosingStepRow    = `SELECT month, step FROM closing_steps WHERE month = ? AND step = ?`
	statementClosingStepAll    = `SELECT month, step FROM closing_steps`
	statementClosingStepInsert = `INSERT INTO closing_steps (month, step) VALUES (?, ?)`
	statementClosingStepDelete = `DELETE FROM closing_steps WHERE month = ? AND step = ?`

	statementTransferMarkRow    = `SELECT month, transfer_key, done_on FROM transfer_marks WHERE month = ? AND transfer_key = ?`
	statementTransferMarkAll    = `SELECT month, transfer_key, done_on FROM transfer_marks`
	statementTransferMarkInsert = `INSERT INTO transfer_marks (month, transfer_key, done_on) VALUES (?, ?, ?)`
	statementTransferMarkUpdate = `UPDATE transfer_marks SET done_on = ? WHERE month = ? AND transfer_key = ?`
	statementTransferMarkDelete = `DELETE FROM transfer_marks WHERE month = ? AND transfer_key = ?`

	statementDismissedWarningRow    = `SELECT id FROM dismissed_warnings WHERE id = ?`
	statementDismissedWarningAll    = `SELECT id FROM dismissed_warnings`
	statementDismissedWarningInsert = `INSERT INTO dismissed_warnings (id) VALUES (?)`
	statementDismissedWarningDelete = `DELETE FROM dismissed_warnings WHERE id = ?`
)

var closingStepsEntry = Table{
	Name: closingStepsTable, Shape: ShapeSet, Key: []string{"month", "step"},
	Columns: []Column{{Name: "month"}, {Name: "step"}},
	Statements: TableStatements{
		SelectRow: statementClosingStepRow, SelectAll: statementClosingStepAll,
		Insert: statementClosingStepInsert, Delete: statementClosingStepDelete,
	},
}

// transferMarksEntry keeps the day a transfer was marked beside its key, so
// marking it again on another day replaces the row at its key.
var transferMarksEntry = Table{
	Name: transferMarksTable, Shape: ShapeSet, Key: []string{"month", "transfer_key"},
	Columns: []Column{{Name: "month"}, {Name: "transfer_key"}, {Name: "done_on"}},
	Statements: TableStatements{
		SelectRow: statementTransferMarkRow, SelectAll: statementTransferMarkAll,
		Insert: statementTransferMarkInsert, Update: statementTransferMarkUpdate, Delete: statementTransferMarkDelete,
	},
}

var dismissedWarningsEntry = Table{
	Name: dismissedWarningsTable, Shape: ShapeSet, Key: []string{"id"},
	Columns: []Column{{Name: "id"}},
	Statements: TableStatements{
		SelectRow: statementDismissedWarningRow, SelectAll: statementDismissedWarningAll,
		Insert: statementDismissedWarningInsert, Delete: statementDismissedWarningDelete,
	},
}
