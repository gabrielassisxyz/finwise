package authored

import (
	"context"
	"database/sql"
	"errors"
	"slices"
)

// Reads and writes of a journaled table's rows, through the statements its
// registry entry names. Nothing here records an effect: the recorder does,
// around these, and an undo's restore is the one caller that must not.

func readRow(ctx context.Context, tx *sql.Tx, table *Table, key []Value) (Row, bool, error) {
	statements := table.Statements
	rows, err := tx.QueryContext(ctx, statements.SelectRow, arguments(key)...)
	if err != nil {
		return nil, false, err
	}
	found, err := scanRows(rows, table)
	if err != nil {
		return nil, false, err
	}
	switch len(found) {
	case 0:
		return nil, false, nil
	case 1:
		return found[0], true, nil
	}
	return nil, false, errors.New("authored: a key of a journaled table selected more than one row")
}

// readContent is a row as every reader sees it: in a table of optional facts a
// missing row is the row at its defaults.
func readContent(ctx context.Context, tx *sql.Tx, table *Table, key []Value) (Row, bool, error) {
	row, found, err := readRow(ctx, tx, table, key)
	if err != nil || found || !table.optionalFacts() {
		return row, found, err
	}
	return table.defaults(key), false, nil
}

func readAll(ctx context.Context, tx *sql.Tx, table *Table) ([]Row, error) {
	statements := table.Statements
	rows, err := tx.QueryContext(ctx, statements.SelectAll)
	if err != nil {
		return nil, err
	}
	return scanRows(rows, table)
}

// readOwned is every row of table that the owner's row of ownerKey owns, in
// the order of their canonical keys, so that the effects recording their
// removal come in the same order whatever the statement returns.
func readOwned(ctx context.Context, tx *sql.Tx, table *Table, ownerKey []Value) ([]Row, error) {
	statements := table.Statements
	rows, err := tx.QueryContext(ctx, statements.SelectOwned, arguments(ownerKey)...)
	if err != nil {
		return nil, err
	}
	found, err := scanRows(rows, table)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(found, func(a, b Row) int {
		return compareText(EncodeKey(table.keyOf(a)...), EncodeKey(table.keyOf(b)...))
	})
	return found, nil
}

func scanRows(rows *sql.Rows, table *Table) ([]Row, error) {
	defer rows.Close()
	var found []Row
	for rows.Next() {
		scanned := make([]any, len(table.Columns))
		targets := make([]any, len(table.Columns))
		for i := range scanned {
			targets[i] = &scanned[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		row := Row{}
		for i, column := range table.Columns {
			value, err := scannedValue(scanned[i])
			if err != nil {
				return nil, err
			}
			row[column.Name] = value
		}
		found = append(found, row)
	}
	return found, rows.Err()
}

func insertRow(ctx context.Context, tx *sql.Tx, table *Table, row Row) error {
	values := make([]Value, len(table.Columns))
	for i, column := range table.Columns {
		values[i] = row[column.Name]
	}
	statements := table.Statements
	_, err := tx.ExecContext(ctx, statements.Insert, arguments(values)...)
	return err
}

func updateRow(ctx context.Context, tx *sql.Tx, table *Table, row Row) error {
	var values []Value
	for _, column := range table.Columns {
		if !table.isKey(column.Name) {
			values = append(values, row[column.Name])
		}
	}
	values = append(values, table.keyOf(row)...)
	statements := table.Statements
	_, err := tx.ExecContext(ctx, statements.Update, arguments(values)...)
	return err
}

func deleteRow(ctx context.Context, tx *sql.Tx, table *Table, key []Value) error {
	statements := table.Statements
	_, err := tx.ExecContext(ctx, statements.Delete, arguments(key)...)
	return err
}

// putRow writes a whole row in one statement: an insert where there is none,
// an update where there is one. A table of optional facts loses the row
// instead when it is back at its defaults, which every reader and the dump
// take for no row.
func putRow(ctx context.Context, tx *sql.Tx, table *Table, row Row, exists bool) error {
	switch {
	case table.optionalFacts() && table.atDefaults(row):
		if !exists {
			return nil
		}
		return deleteRow(ctx, tx, table, table.keyOf(row))
	case exists:
		if len(table.Key) == len(table.Columns) {
			return nil
		}
		return updateRow(ctx, tx, table, row)
	}
	return insertRow(ctx, tx, table, row)
}

func arguments(values []Value) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value.argument()
	}
	return out
}
