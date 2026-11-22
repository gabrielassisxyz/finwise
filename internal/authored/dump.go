package authored

import (
	"context"
	"database/sql"
	"slices"
	"strings"
)

// Dump is every journaled table's content in canonical form, one line per
// row, "table row", tables in name order and rows in the order of their keys.
// A row of a table of optional facts at its defaults is left out, since it is
// the same as no row. It is what the proofs compare: an undo returns the dump
// to what it was before the change.
func (database *Database) Dump(ctx context.Context) (string, error) {
	var dump string
	err := database.read(ctx, func(tx *sql.Tx) error {
		var err error
		dump, err = dumpTables(ctx, tx, database.journal)
		return err
	})
	return dump, err
}

func dumpTables(ctx context.Context, tx *sql.Tx, registry *Registry) (string, error) {
	var out strings.Builder
	for _, name := range registry.names() {
		table := registry.tables[name]
		rows, err := readAll(ctx, tx, table)
		if err != nil {
			return "", err
		}
		type keyed struct{ key, line string }
		lines := make([]keyed, 0, len(rows))
		for _, row := range rows {
			if table.optionalFacts() && table.atDefaults(row) {
				continue
			}
			lines = append(lines, keyed{key: EncodeKey(table.keyOf(row)...), line: encodeRow(table, row)})
		}
		slices.SortFunc(lines, func(a, b keyed) int { return compareText(a.key, b.key) })
		for _, line := range lines {
			out.WriteString(name + " " + line.line + "\n")
		}
	}
	return out.String(), nil
}
