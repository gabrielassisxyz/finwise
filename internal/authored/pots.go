package authored

import (
	"context"
	"database/sql"
)

// PotBucket is where a pot's money counts in the net worth. The three values
// are the words the contract uses; the table refuses any other.
type PotBucket string

const (
	BucketAvailable PotBucket = "Disponível"
	BucketReserved  PotBucket = "Reservado"
	BucketInvested  PotBucket = "Investido"
)

const (
	potSettingsTable = "pot_settings"
	potLinesTable    = "pot_lines"
)

const (
	statementPotSettingRow    = `SELECT pot_id, bucket FROM pot_settings WHERE pot_id = ?`
	statementPotSettingAll    = `SELECT pot_id, bucket FROM pot_settings`
	statementPotSettingInsert = `INSERT INTO pot_settings (pot_id, bucket) VALUES (?, ?)`
	statementPotSettingUpdate = `UPDATE pot_settings SET bucket = ? WHERE pot_id = ?`
	statementPotSettingDelete = `DELETE FROM pot_settings WHERE pot_id = ?`

	statementPotLineRow    = `SELECT line_id, pot_id FROM pot_lines WHERE line_id = ?`
	statementPotLineAll    = `SELECT line_id, pot_id FROM pot_lines`
	statementPotLineInsert = `INSERT INTO pot_lines (line_id, pot_id) VALUES (?, ?)`
	statementPotLineUpdate = `UPDATE pot_lines SET pot_id = ? WHERE line_id = ?`
	statementPotLineDelete = `DELETE FROM pot_lines WHERE line_id = ?`

	// statementLinesOfPot orders by line id so that a pot's lines read the same
	// every time; the order the links were made in is not kept.
	statementLinesOfPot = `SELECT line_id FROM pot_lines WHERE pot_id = ? ORDER BY line_id`
)

// potSettingsEntry is an attribute table of optional facts: a pot with no row
// has no bucket, which is also what its one column holds at its default, so
// clearing a bucket removes the row.
func potSettingsEntry() Table {
	return Table{
		Name: potSettingsTable, Shape: ShapeAttribute, Kind: KindOptionalFacts, Key: []string{"pot_id"},
		Columns: []Column{{Name: "pot_id"}, {Name: "bucket", Default: Null}},
		Statements: TableStatements{
			SelectRow: statementPotSettingRow, SelectAll: statementPotSettingAll,
			Insert: statementPotSettingInsert, Update: statementPotSettingUpdate, Delete: statementPotSettingDelete,
		},
	}
}

// potLinesEntry is keyed by the line and its one fact is the pot, so a line has
// at most one pot in the journal as it does in the table, and moving a line is
// one column changing: undoing it is one write that cannot meet the uniqueness
// of the line halfway. It is a table of things created whole because a link
// has no default pot and creating it is a unit of its own.
func potLinesEntry() Table {
	return Table{
		Name: potLinesTable, Shape: ShapeAttribute, Kind: KindCreatedWhole, Key: []string{"line_id"},
		Columns: []Column{{Name: "line_id"}, {Name: "pot_id"}},
		Statements: TableStatements{
			SelectRow: statementPotLineRow, SelectAll: statementPotLineAll,
			Insert: statementPotLineInsert, Update: statementPotLineUpdate, Delete: statementPotLineDelete,
		},
	}
}

// PotBucket is the bucket the person chose for a pot, and false when they
// chose none, on the empty view too.
func (view *View) PotBucket(ctx context.Context, potID string) (PotBucket, bool, error) {
	row, found, err := view.JournaledRow(ctx, potSettingsTable, []Value{Text(potID)})
	if err != nil || !found {
		return "", false, err
	}
	bucket, chosen := row["bucket"].Str()
	return PotBucket(bucket), chosen, nil
}

// PotOfLine is the pot a budget line is linked to, and false when it is linked
// to none, on the empty view too.
func (view *View) PotOfLine(ctx context.Context, line LineID) (potID string, linked bool, err error) {
	row, found, err := view.JournaledRow(ctx, potLinesTable, []Value{Integer(int64(line))})
	if err != nil || !found {
		return "", false, err
	}
	potID, linked = row["pot_id"].Str()
	return potID, linked, nil
}

// LinesOfPot is the budget lines linked to a pot, by ascending id; none on the
// empty view.
func (view *View) LinesOfPot(ctx context.Context, potID string) ([]LineID, error) {
	var lines []LineID
	_, err := view.read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, statementLinesOfPot, potID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			lines = append(lines, LineID(id))
		}
		return rows.Err()
	})
	return lines, err
}

// SetPotBucket records the bucket of a pot. The bucket already stored records
// nothing. The store checks no value: the table refuses a bucket that is not
// one of the three, and an id that is empty or a slot's, and the write then
// fails, so a handler validates before it writes.
func (recorder *Recorder) SetPotBucket(ctx context.Context, potID string, bucket PotBucket) error {
	return recorder.Set(ctx, potSettingsTable, []Value{Text(potID)}, Row{"bucket": Text(string(bucket))})
}

// ClearPotBucket takes back the person's choice, leaving the pot as if none
// had been made. It records nothing for a pot with no bucket stored.
func (recorder *Recorder) ClearPotBucket(ctx context.Context, potID string) error {
	return recorder.Set(ctx, potSettingsTable, []Value{Text(potID)}, Row{"bucket": Null})
}

// ToggleLink links the line to the pot, or unlinks it when it is linked to
// that pot already; a line linked to another pot is moved, in the same change,
// because a line paying into two pots would count its planned amount twice.
// A line that is not in the table of lines is refused when the change commits.
func (recorder *Recorder) ToggleLink(ctx context.Context, line LineID, potID string) error {
	key := []Value{Integer(int64(line))}
	row, linked, err := recorder.View().JournaledRow(ctx, potLinesTable, key)
	switch {
	case err != nil:
		return recorder.fail(err)
	case !linked:
		return recorder.Create(ctx, potLinesTable, Row{"line_id": key[0], "pot_id": Text(potID)})
	case row["pot_id"] == Text(potID):
		return recorder.Delete(ctx, potLinesTable, key)
	}
	return recorder.Set(ctx, potLinesTable, key, Row{"pot_id": Text(potID)})
}
