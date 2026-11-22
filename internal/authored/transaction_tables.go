package authored

// The transactions' journaled tables and the statements their entries name.
// tx_index and tx_ids, the index of what the database has seen, are not
// journaled; their statements are in transaction_index.go.

const (
	txStateTable           = "tx_state"
	txTagsTable            = "tx_tags"
	txSplitPartsTable      = "tx_split_parts"
	establishmentsTable    = "establishments"
	establishmentTagsTable = "establishment_tags"
	historyStepsTable      = "history_steps"
	historyReasonsTable    = "history_step_reasons"
)

const (
	txStateColumns = `tx_key, line_id, classified_by, confidence, pending_reason, pending_line_id, pending_confidence,
ask_reason, note, exclusion, refund, refund_line_id, reimbursable, expected_cents, credit_tx_key, credit_cents`

	statementTxStateRow    = `SELECT ` + txStateColumns + ` FROM tx_state WHERE tx_key = ?`
	statementTxStateAll    = `SELECT ` + txStateColumns + ` FROM tx_state`
	statementTxStateInsert = `INSERT INTO tx_state (` + txStateColumns + `)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	statementTxStateUpdate = `
UPDATE tx_state SET line_id = ?, classified_by = ?, confidence = ?, pending_reason = ?, pending_line_id = ?,
  pending_confidence = ?, ask_reason = ?, note = ?, exclusion = ?, refund = ?, refund_line_id = ?, reimbursable = ?,
  expected_cents = ?, credit_tx_key = ?, credit_cents = ?
WHERE tx_key = ?`
	statementTxStateDelete = `DELETE FROM tx_state WHERE tx_key = ?`

	statementTxTagRow    = `SELECT tx_key, tag, position FROM tx_tags WHERE tx_key = ? AND tag = ?`
	statementTxTagAll    = `SELECT tx_key, tag, position FROM tx_tags`
	statementTxTagInsert = `INSERT INTO tx_tags (tx_key, tag, position) VALUES (?, ?, ?)`
	statementTxTagUpdate = `UPDATE tx_tags SET position = ? WHERE tx_key = ? AND tag = ?`
	statementTxTagDelete = `DELETE FROM tx_tags WHERE tx_key = ? AND tag = ?`

	statementSplitPartRow    = `SELECT tx_key, position, line_id, cents FROM tx_split_parts WHERE tx_key = ? AND position = ?`
	statementSplitPartAll    = `SELECT tx_key, position, line_id, cents FROM tx_split_parts`
	statementSplitPartInsert = `INSERT INTO tx_split_parts (tx_key, position, line_id, cents) VALUES (?, ?, ?, ?)`
	statementSplitPartUpdate = `UPDATE tx_split_parts SET line_id = ?, cents = ? WHERE tx_key = ? AND position = ?`
	statementSplitPartDelete = `DELETE FROM tx_split_parts WHERE tx_key = ? AND position = ?`

	statementEstablishmentRow    = `SELECT key, nickname, default_line_id FROM establishments WHERE key = ?`
	statementEstablishmentAll    = `SELECT key, nickname, default_line_id FROM establishments`
	statementEstablishmentInsert = `INSERT INTO establishments (key, nickname, default_line_id) VALUES (?, ?, ?)`
	statementEstablishmentUpdate = `UPDATE establishments SET nickname = ?, default_line_id = ? WHERE key = ?`
	statementEstablishmentDelete = `DELETE FROM establishments WHERE key = ?`

	statementFixedTagRow    = `SELECT key, tag, position FROM establishment_tags WHERE key = ? AND tag = ?`
	statementFixedTagAll    = `SELECT key, tag, position FROM establishment_tags`
	statementFixedTagInsert = `INSERT INTO establishment_tags (key, tag, position) VALUES (?, ?, ?)`
	statementFixedTagUpdate = `UPDATE establishment_tags SET position = ? WHERE key = ? AND tag = ?`
	statementFixedTagDelete = `DELETE FROM establishment_tags WHERE key = ? AND tag = ?`

	historyStepColumns = `id, tx_key, change_id, kind, text, day`

	statementHistoryStepRow    = `SELECT ` + historyStepColumns + ` FROM history_steps WHERE id = ?`
	statementHistoryStepAll    = `SELECT ` + historyStepColumns + ` FROM history_steps`
	statementHistoryStepInsert = `INSERT INTO history_steps (` + historyStepColumns + `) VALUES (?, ?, ?, ?, ?, ?)`
	statementHistoryStepUpdate = `UPDATE history_steps SET tx_key = ?, change_id = ?, kind = ?, text = ?, day = ? WHERE id = ?`
	statementHistoryStepDelete = `DELETE FROM history_steps WHERE id = ?`

	statementStepReasonRow    = `SELECT step_id, reason FROM history_step_reasons WHERE step_id = ?`
	statementStepReasonAll    = `SELECT step_id, reason FROM history_step_reasons`
	statementStepReasonInsert = `INSERT INTO history_step_reasons (step_id, reason) VALUES (?, ?)`
	statementStepReasonUpdate = `UPDATE history_step_reasons SET reason = ? WHERE step_id = ?`
	statementStepReasonDelete = `DELETE FROM history_step_reasons WHERE step_id = ?`
)

// The defaults are what a missing row of tx_state reads as, and they are the
// schema's: every column null but ask_reason and reimbursable, at 0, and note,
// empty.
var txStateEntry = Table{
	Name: txStateTable, Shape: ShapeAttribute, Kind: KindOptionalFacts, Key: []string{"tx_key"},
	Columns: []Column{
		{Name: "tx_key"}, {Name: "line_id"}, {Name: "classified_by"}, {Name: "confidence"},
		{Name: "pending_reason"}, {Name: "pending_line_id"}, {Name: "pending_confidence"},
		{Name: "ask_reason", Default: Integer(0)}, {Name: "note", Default: Text("")}, {Name: "exclusion"},
		{Name: "refund"}, {Name: "refund_line_id"}, {Name: "reimbursable", Default: Integer(0)},
		{Name: "expected_cents"}, {Name: "credit_tx_key"}, {Name: "credit_cents"},
	},
	Statements: TableStatements{
		SelectRow: statementTxStateRow, SelectAll: statementTxStateAll,
		Insert: statementTxStateInsert, Update: statementTxStateUpdate, Delete: statementTxStateDelete,
	},
}

var txTagsEntry = Table{
	Name: txTagsTable, Shape: ShapeSet, Key: []string{"tx_key", "tag"},
	Columns: []Column{{Name: "tx_key"}, {Name: "tag"}, {Name: "position"}},
	Statements: TableStatements{
		SelectRow: statementTxTagRow, SelectAll: statementTxTagAll,
		Insert: statementTxTagInsert, Update: statementTxTagUpdate, Delete: statementTxTagDelete,
	},
}

var txSplitPartsEntry = Table{
	Name: txSplitPartsTable, Shape: ShapeSet, Key: []string{"tx_key", "position"},
	Columns: []Column{{Name: "tx_key"}, {Name: "position"}, {Name: "line_id"}, {Name: "cents"}},
	Statements: TableStatements{
		SelectRow: statementSplitPartRow, SelectAll: statementSplitPartAll,
		Insert: statementSplitPartInsert, Update: statementSplitPartUpdate, Delete: statementSplitPartDelete,
	},
}

var establishmentsEntry = Table{
	Name: establishmentsTable, Shape: ShapeAttribute, Kind: KindOptionalFacts, Key: []string{"key"},
	Columns: []Column{{Name: "key"}, {Name: "nickname"}, {Name: "default_line_id"}},
	Statements: TableStatements{
		SelectRow: statementEstablishmentRow, SelectAll: statementEstablishmentAll,
		Insert: statementEstablishmentInsert, Update: statementEstablishmentUpdate, Delete: statementEstablishmentDelete,
	},
}

var establishmentTagsEntry = Table{
	Name: establishmentTagsTable, Shape: ShapeSet, Key: []string{"key", "tag"},
	Columns: []Column{{Name: "key"}, {Name: "tag"}, {Name: "position"}},
	Statements: TableStatements{
		SelectRow: statementFixedTagRow, SelectAll: statementFixedTagAll,
		Insert: statementFixedTagInsert, Update: statementFixedTagUpdate, Delete: statementFixedTagDelete,
	},
}

var historyStepsEntry = Table{
	Name: historyStepsTable, Shape: ShapeSet, Key: []string{"id"},
	Columns: []Column{{Name: "id"}, {Name: "tx_key"}, {Name: "change_id"}, {Name: "kind"}, {Name: "text"}, {Name: "day"}},
	Statements: TableStatements{
		SelectRow: statementHistoryStepRow, SelectAll: statementHistoryStepAll,
		Insert: statementHistoryStepInsert, Update: statementHistoryStepUpdate, Delete: statementHistoryStepDelete,
	},
}

// A reason is owned by its step: writing one is an effect on the reason alone,
// so the step's change can still be undone or extended, and removing the step
// removes its reason as an effect of the same change.
var historyReasonsEntry = Table{
	Name: historyReasonsTable, Shape: ShapeSet, Key: []string{"step_id"},
	Columns: []Column{{Name: "step_id"}, {Name: "reason"}},
	OwnedBy: &Ownership{Owner: historyStepsTable, Columns: []string{"step_id"}},
	Statements: TableStatements{
		// A step owns at most one reason, which is the row at the step's id.
		SelectRow: statementStepReasonRow, SelectAll: statementStepReasonAll, SelectOwned: statementStepReasonRow,
		Insert: statementStepReasonInsert, Update: statementStepReasonUpdate, Delete: statementStepReasonDelete,
	},
}
