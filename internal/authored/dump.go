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

// keyedLine is one dumped row with the encoded key the rows are ordered by.
type keyedLine struct{ key, line string }

// dumpedRows is a table's rows in canonical form, in the order of their keys. A
// row of a table of optional facts at its defaults is left out.
func dumpedRows(ctx context.Context, tx *sql.Tx, table *Table) ([]keyedLine, error) {
	rows, err := readAll(ctx, tx, table)
	if err != nil {
		return nil, err
	}
	lines := make([]keyedLine, 0, len(rows))
	for _, row := range rows {
		if table.optionalFacts() && table.atDefaults(row) {
			continue
		}
		lines = append(lines, keyedLine{key: EncodeKey(table.keyOf(row)...), line: encodeRow(table, row)})
	}
	slices.SortFunc(lines, func(a, b keyedLine) int { return compareText(a.key, b.key) })
	return lines, nil
}

func dumpTables(ctx context.Context, tx *sql.Tx, registry *Registry) (string, error) {
	var out strings.Builder
	for _, name := range registry.names() {
		lines, err := dumpedRows(ctx, tx, registry.tables[name])
		if err != nil {
			return "", err
		}
		for _, line := range lines {
			out.WriteString(name + " " + line.line + "\n")
		}
	}
	return out.String(), nil
}

// CanonicalDump is the same content as Dump as one JSON document: an object
// with a member for every journaled table in name order, an empty table
// included, each holding its rows in the order of their keys, one row to a
// line. It is what a harness compares and what a person reads in a diff.
func (database *Database) CanonicalDump(ctx context.Context) (string, error) {
	var dump string
	err := database.read(ctx, func(tx *sql.Tx) error {
		var err error
		dump, err = canonicalDocument(ctx, tx, database.journal)
		return err
	})
	return dump, err
}

func canonicalDocument(ctx context.Context, tx *sql.Tx, registry *Registry) (string, error) {
	names := registry.names()
	if len(names) == 0 {
		return "{}\n", nil
	}
	var out strings.Builder
	out.WriteString("{\n")
	for i, name := range names {
		lines, err := dumpedRows(ctx, tx, registry.tables[name])
		if err != nil {
			return "", err
		}
		out.WriteString("  " + quoteJSON(name) + ": [")
		for j, line := range lines {
			out.WriteString("\n    " + line.line)
			if j < len(lines)-1 {
				out.WriteString(",")
			}
		}
		if len(lines) > 0 {
			out.WriteString("\n  ")
		}
		out.WriteString("]")
		if i < len(names)-1 {
			out.WriteString(",")
		}
		out.WriteString("\n")
	}
	out.WriteString("}\n")
	return out.String(), nil
}
