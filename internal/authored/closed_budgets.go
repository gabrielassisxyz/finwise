package authored

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// A closed month's budget is kept exactly as it stood: a save for it is
// refused and a save across it leaves its version alone. An undo is the one
// write left that could move it, because the journal decides an undo by units
// and a closed month's budget can come from a version the undo restores without
// sharing a unit with the close: the undo of a save made before the close, say.
// closedMonthsKeepTheirBudget refuses those.

// statementBudgetEffectsInForce is every effect on the budget's versions of a
// change not undone, in the journal's order. An undo marks its change undone
// only after the checks, so during an undo the change being undone is among
// them.
const statementBudgetEffectsInForce = `
SELECT e.change_id, e.tbl, e.key, e.before, e.after
FROM change_effects AS e JOIN changes AS c ON c.id = e.change_id
WHERE e.tbl IN ('budget_versions', 'budget_version_lines') AND c.undone_at IS NULL
ORDER BY e.change_id, e.seq`

// closedMonthsKeepTheirBudget is the check every undo of the app runs, a whole
// one and the restore of one part alike: every month that is closed once the
// restore is done has the budget it had before the restore, line by line, in
// the same groups and order, with the same ids, names, amounts and flags. A
// month the undo reopens is not closed once it is done, so its budget may fall
// back. It refuses and never repairs.
func closedMonthsKeepTheirBudget(ctx context.Context, tx *sql.Tx, restored []Unit) (bool, error) {
	touched := map[Unit]bool{}
	for _, unit := range restored {
		if unit.Table == budgetVersionsTable.Name || unit.Table == budgetVersionLinesTable.Name {
			touched[unit] = true
		}
	}
	if len(touched) == 0 {
		return true, nil
	}
	after, err := readVersionRows(ctx, tx)
	if err != nil {
		return false, err
	}
	journal, err := budgetEffectsInForce(ctx, tx)
	if err != nil {
		return false, err
	}
	before, err := after.beforeRestore(touched, journal)
	if err != nil {
		return false, err
	}
	closed, err := closedMonths(ctx, tx)
	if err != nil {
		return false, err
	}
	for _, month := range closed {
		if !slices.EqualFunc(before.inForce(month), after.inForce(month), sameBudgetLine) {
			return false, nil
		}
	}
	return true, nil
}

// sameBudgetLine compares two lines as budget.document answers them; their
// order within the group is the order the two lists already share.
func sameBudgetLine(a, b BudgetLine) bool {
	return a.Group == b.Group && a.ID == b.ID && a.Name == b.Name && a.Cents == b.Cents && a.Flag == b.Flag
}

// versionRows is the budget's two journaled tables held in memory, each row
// under the unit the journal writes for it.
type versionRows map[Unit]Row

func readVersionRows(ctx context.Context, tx *sql.Tx) (versionRows, error) {
	rows := versionRows{}
	for _, table := range []*Table{&budgetVersionsTable, &budgetVersionLinesTable} {
		found, err := readAll(ctx, tx, table)
		if err != nil {
			return nil, err
		}
		for _, row := range found {
			rows[Unit{Table: table.Name, Key: EncodeKey(table.keyOf(row)...)}] = row
		}
	}
	return rows, nil
}

// encoded is the unit's row as the journal writes it, or absent.
func (rows versionRows) encoded(unit Unit) sql.NullString {
	row, found := rows[unit]
	if !found {
		return absent
	}
	return present(encodeRow(versionTableOf(unit), row))
}

func (rows versionRows) put(unit Unit, encoded sql.NullString) error {
	if !encoded.Valid {
		delete(rows, unit)
		return nil
	}
	row, err := decodeRow(versionTableOf(unit), encoded.String)
	if err != nil {
		return err
	}
	rows[unit] = row
	return nil
}

func versionTableOf(unit Unit) *Table {
	if unit.Table == budgetVersionsTable.Name {
		return &budgetVersionsTable
	}
	return &budgetVersionLinesTable
}

// lastChange is what the latest change in force did to one unit: its first
// before and its last after.
type lastChange struct {
	id            int64
	before, after sql.NullString
}

func budgetEffectsInForce(ctx context.Context, tx *sql.Tx) (map[Unit]*lastChange, error) {
	rows, err := tx.QueryContext(ctx, statementBudgetEffectsInForce)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	latest := map[Unit]*lastChange{}
	for rows.Next() {
		var id int64
		var unit Unit
		var before, after sql.NullString
		if err := rows.Scan(&id, &unit.Table, &unit.Key, &before, &after); err != nil {
			return nil, err
		}
		if found, seen := latest[unit]; seen && found.id == id {
			found.after = after
			continue
		}
		latest[unit] = &lastChange{id: id, before: before, after: after}
	}
	return latest, rows.Err()
}

// beforeRestore is what the two tables held before the restore. A restored
// unit was moved between the two ends of the latest change in force on it: an
// undo moves it from that change's last after back to its first before, the
// restore of a part records its own write and so moves it forward through a
// change of its own. Either way it held the end it does not hold now.
//
// Removing a version takes every line it owns with it, the ones no unit of the
// restore names included, so the journal is asked what those held.
func (rows versionRows) beforeRestore(touched map[Unit]bool, journal map[Unit]*lastChange) (versionRows, error) {
	before := versionRows{}
	for unit, row := range rows {
		before[unit] = row
	}
	removed := map[Value]bool{}
	for unit := range touched {
		end, found := journal[unit]
		if !found {
			return nil, errors.New("authored: a restored unit of the budget has no effect in force")
		}
		now, held := rows.encoded(unit), end.after
		if now == end.after {
			held = end.before
		}
		if err := before.put(unit, held); err != nil {
			return nil, err
		}
		if unit.Table == budgetVersionsTable.Name && held.Valid && !now.Valid {
			from, err := decodeKey(unit.Key)
			if err != nil {
				return nil, err
			}
			removed[from[0]] = true
		}
	}
	return before, before.addOwnedLinesOf(removed, touched, journal, rows)
}

func (before versionRows) addOwnedLinesOf(removed map[Value]bool, touched map[Unit]bool, journal map[Unit]*lastChange, after versionRows) error {
	if len(removed) == 0 {
		return nil
	}
	for unit, end := range journal {
		if unit.Table != budgetVersionLinesTable.Name || touched[unit] || !end.after.Valid {
			continue
		}
		if _, stays := after[unit]; stays {
			continue
		}
		key, err := decodeKey(unit.Key)
		if err != nil {
			return err
		}
		if removed[key[0]] {
			if err := before.put(unit, end.after); err != nil {
				return err
			}
		}
	}
	return nil
}

// inForce is the lines of the version in force for month by the rule of
// statementBudgetVersionInForce: the latest that starts at or before it, else
// the first, and none without a version.
func (rows versionRows) inForce(month brday.Month) []BudgetLine {
	var latest, first string
	for unit, row := range rows {
		if unit.Table != budgetVersionsTable.Name {
			continue
		}
		from, _ := row["from_month"].Str()
		if from <= string(month) && from > latest {
			latest = from
		}
		if first == "" || from < first {
			first = from
		}
	}
	if latest == "" {
		latest = first
	}
	var lines []BudgetLine
	for unit, row := range rows {
		if from, _ := row["from_month"].Str(); unit.Table == budgetVersionLinesTable.Name && from == latest {
			lines = append(lines, budgetLineOf(row))
		}
	}
	slices.SortFunc(lines, compareBudgetLines)
	return lines
}

func budgetLineOf(row Row) BudgetLine {
	group, _ := row["group_key"].Str()
	position, _ := row["position"].Int64()
	id, _ := row["line_id"].Int64()
	name, _ := row["name"].Str()
	cents, _ := row["cents"].Int64()
	flag, _ := row["flag"].Str()
	return BudgetLine{Group: group, Position: int(position), ID: LineID(id), Name: name, Cents: money.Cents(cents), Flag: flag}
}
