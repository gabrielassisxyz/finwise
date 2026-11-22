package authored

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"time"
)

// instantLayout is how changes.at and changes.undone_at hold an instant.
const instantLayout = "2006-01-02T15:04:05.000Z"

const statementInsertChange = `
INSERT INTO changes (at, day, author, method) VALUES (?, ?, ?, ?)`

const statementInsertEffect = `
INSERT INTO change_effects (change_id, seq, tbl, key, col, before, after) VALUES (?, ?, ?, ?, ?, ?, ?)`

const statementChangeByID = `
SELECT method, extended, undone_at IS NOT NULL FROM changes WHERE id = ?`

const statementEffectsOfChange = `
SELECT seq, tbl, key, col, before, after FROM change_effects WHERE change_id = ? ORDER BY seq`

const statementLastSeq = `SELECT coalesce(max(seq), 0) FROM change_effects WHERE change_id = ?`

// The range on change_id walks the primary key from the change onwards, so
// the cost is the journal written since, not the whole journal.
const statementLaterEffectsInForce = `
SELECT e.tbl, e.key, e.col
FROM change_effects AS e JOIN changes AS c ON c.id = e.change_id
WHERE e.change_id > ? AND c.undone_at IS NULL`

const statementMarkUndone = `UPDATE changes SET undone_at = ? WHERE id = ? AND undone_at IS NULL`

const statementMarkExtended = `UPDATE changes SET extended = 1 WHERE id = ? AND extended = 0`

// Foreign keys are checked at the commit of every journal transaction, so a
// parent and its children can be written, removed or restored in any order.
const statementDeferForeignKeys = `PRAGMA defer_foreign_keys = 1`

// ChangeID is the handle of a change; the API writes it as a decimal string.
type ChangeID int64

func (id ChangeID) String() string { return strconv.FormatInt(int64(id), 10) }

// Author is who made a change.
type Author string

const (
	// AuthorPerson is a write from an HTTP session.
	AuthorPerson Author = "person"
	// AuthorRule is the step that takes in a new snapshot and classifies new
	// transactions by rule.
	AuthorRule Author = "rule"
	// AuthorModel is reserved for a later nightly model call.
	AuthorModel Author = "model"
)

func (author Author) valid() bool {
	return author == AuthorPerson || author == AuthorRule || author == AuthorModel
}

func formatInstant(instant time.Time) string {
	return instant.UTC().Format(instantLayout)
}

// Unit is what one effect is about: a table, a key and, for an attribute
// table, a column. Column is empty for a set row and for the creation or
// removal of a row of a table of things created whole.
type Unit struct {
	Table  string
	Key    string
	Column string
}

type effect struct {
	seq    int64
	unit   Unit
	before sql.NullString
	after  sql.NullString
}

// collapsed is a unit as one change left it: the first before, the last after.
type collapsed struct {
	unit     Unit
	firstSeq int64
	lastSeq  int64
	before   sql.NullString
	after    sql.NullString
}

type changeState struct {
	method   string
	extended bool
	undone   bool
}

func loadChange(ctx context.Context, tx *sql.Tx, id ChangeID) (changeState, bool, error) {
	var state changeState
	err := tx.QueryRowContext(ctx, statementChangeByID, int64(id)).Scan(&state.method, &state.extended, &state.undone)
	if err == sql.ErrNoRows {
		return changeState{}, false, nil
	}
	return state, err == nil, err
}

func loadEffects(ctx context.Context, tx *sql.Tx, id ChangeID) ([]effect, error) {
	rows, err := tx.QueryContext(ctx, statementEffectsOfChange, int64(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var effects []effect
	for rows.Next() {
		var found effect
		var column sql.NullString
		if err := rows.Scan(&found.seq, &found.unit.Table, &found.unit.Key, &column, &found.before, &found.after); err != nil {
			return nil, err
		}
		found.unit.Column = column.String
		effects = append(effects, found)
	}
	return effects, rows.Err()
}

// collapse is step 1 of an undo: per unit, the first before and the last
// after, in the order of each unit's first effect.
func collapse(effects []effect) []collapsed {
	index := map[Unit]int{}
	var units []collapsed
	for _, found := range effects {
		at, seen := index[found.unit]
		if !seen {
			index[found.unit] = len(units)
			units = append(units, collapsed{
				unit: found.unit, firstSeq: found.seq, lastSeq: found.seq, before: found.before, after: found.after,
			})
			continue
		}
		units[at].lastSeq = found.seq
		units[at].after = found.after
	}
	return units
}

// laterUnitsInForce is every unit that a change after id, not undone, has an
// effect on. The journal's order decides, not the value a unit holds.
func laterUnitsInForce(ctx context.Context, tx *sql.Tx, id ChangeID) ([]Unit, error) {
	rows, err := tx.QueryContext(ctx, statementLaterEffectsInForce, int64(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var units []Unit
	for rows.Next() {
		var unit Unit
		var column sql.NullString
		if err := rows.Scan(&unit.Table, &unit.Key, &column); err != nil {
			return nil, err
		}
		unit.Column = column.String
		units = append(units, unit)
	}
	return units, rows.Err()
}

// overlaps says whether a later effect on other falls on unit. In a table of
// things created whole the row is a unit of its own that every column of it
// meets, so a later change to any column makes the creation stale and the
// creation's undo never leaves half a row; in a table of optional facts no
// change owns a row, so only the same column meets.
func overlaps(table *Table, unit, other Unit) bool {
	if unit.Table != other.Table || unit.Key != other.Key {
		return false
	}
	if table.createdWhole() && (unit.Column == "" || other.Column == "") {
		return true
	}
	return unit.Column == other.Column
}

// laterChangeOverlaps is the first half of step 2: a later change still in
// force has an effect on one of units.
func laterChangeOverlaps(ctx context.Context, tx *sql.Tx, registry *Registry, id ChangeID, units []collapsed) (bool, error) {
	later, err := laterUnitsInForce(ctx, tx, id)
	if err != nil {
		return false, err
	}
	for _, unit := range units {
		table, err := registry.table(unit.unit.Table)
		if err != nil {
			return false, err
		}
		for _, other := range later {
			if overlaps(table, unit.unit, other) {
				return true, nil
			}
		}
	}
	return false, nil
}

// rowPlan is every collapsed unit that falls on one row, which is read once,
// checked once and restored with one statement.
type rowPlan struct {
	table   *Table
	key     []Value
	order   int64
	row     *collapsed
	columns []collapsed
}

// planRows groups units by row and orders the rows for the restore: the row
// first touched last is restored first, so a unique value one row took from
// another within the change is given back before the other row asks for it.
func planRows(registry *Registry, units []collapsed) ([]*rowPlan, error) {
	plans := map[[2]string]*rowPlan{}
	var ordered []*rowPlan
	for _, unit := range units {
		table, err := registry.table(unit.unit.Table)
		if err != nil {
			return nil, err
		}
		id := [2]string{unit.unit.Table, unit.unit.Key}
		plan, seen := plans[id]
		if !seen {
			key, err := decodeKey(unit.unit.Key)
			if err != nil {
				return nil, err
			}
			plan = &rowPlan{table: table, key: key, order: unit.firstSeq}
			plans[id] = plan
			ordered = append(ordered, plan)
		}
		plan.order = min(plan.order, unit.firstSeq)
		if unit.unit.Column == "" {
			plan.row = &unit
		} else {
			plan.columns = append(plan.columns, unit)
		}
	}
	slices.SortFunc(ordered, func(a, b *rowPlan) int { return int(b.order - a.order) })
	return ordered, nil
}

// holds is the second half of step 2: the row holds what the change left in
// it, as a guard against a write that went around the journal.
func (plan *rowPlan) holds(ctx context.Context, tx *sql.Tx) (bool, error) {
	current, found, err := readContent(ctx, tx, plan.table, plan.key)
	if err != nil {
		return false, err
	}
	if plan.row == nil {
		if !found && !plan.table.optionalFacts() {
			return false, nil
		}
		return columnsHold(current, plan.columns, func(unit collapsed) sql.NullString { return unit.after })
	}
	if !plan.row.after.Valid {
		return !found, nil
	}
	if !found {
		return false, nil
	}
	expected, err := decodeRow(plan.table, plan.row.after.String)
	if err != nil {
		return false, err
	}
	for _, unit := range plan.columns {
		if unit.lastSeq < plan.row.lastSeq {
			continue
		}
		if expected[unit.unit.Column], err = decodeValue(unit.after.String); err != nil {
			return false, err
		}
	}
	return encodeRow(plan.table, current) == encodeRow(plan.table, expected), nil
}

func columnsHold(current Row, units []collapsed, side func(collapsed) sql.NullString) (bool, error) {
	for _, unit := range units {
		value, ok := current[unit.unit.Column]
		if !ok {
			return false, nil
		}
		if !side(unit).Valid || value.canonical() != side(unit).String {
			return false, nil
		}
	}
	return true, nil
}

// target is step 3 for one row: the whole row as it was before the change,
// computed from every before that falls on it, or no row.
func (plan *rowPlan) target(ctx context.Context, tx *sql.Tx) (Row, bool, error) {
	if plan.row == nil {
		current, _, err := readContent(ctx, tx, plan.table, plan.key)
		if err != nil {
			return nil, false, err
		}
		return withBefores(current, plan.columns, func(collapsed) bool { return true })
	}
	if !plan.row.before.Valid {
		return nil, false, nil
	}
	restored, err := decodeRow(plan.table, plan.row.before.String)
	if err != nil {
		return nil, false, err
	}
	// A column changed before the row's own first effect was then captured in
	// that effect's before with its new value; its own before is the older one.
	return withBefores(restored, plan.columns, func(unit collapsed) bool { return unit.firstSeq < plan.row.firstSeq })
}

func withBefores(row Row, units []collapsed, applies func(collapsed) bool) (Row, bool, error) {
	restored := Row{}
	for name, value := range row {
		restored[name] = value
	}
	for _, unit := range units {
		if !applies(unit) {
			continue
		}
		value, err := decodeValue(unit.before.String)
		if err != nil {
			return nil, false, err
		}
		restored[unit.unit.Column] = value
	}
	return restored, true, nil
}

// unitsOf is the units a restore put back, for the checks.
func unitsOf(units []collapsed) []Unit {
	out := make([]Unit, len(units))
	for i, unit := range units {
		out[i] = unit.unit
	}
	return out
}
