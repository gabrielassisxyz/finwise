package authored

import (
	"context"
	"strings"
)

// What the person says about one transaction (its own tags and its note) and
// about the establishment or person behind it (a nickname, a default line and
// the tags every transaction of it carries). Each function is one write into
// the recorder's change; one that finds the state already as asked records
// nothing.

const (
	// The place a new tag takes: one past the highest the owner holds, so a tag
	// added later goes after every other and no tag is ever renumbered.
	statementNextTagPosition      = `SELECT coalesce(max(position) + 1, 0) FROM tx_tags WHERE tx_key = ?`
	statementNextFixedTagPosition = `SELECT coalesce(max(position) + 1, 0) FROM establishment_tags WHERE key = ?`
)

// nextPlace answers the place a new tag takes among those its owner holds.
type nextPlace func(ctx context.Context) (int64, error)

// NormalizeTag is the one spelling a tag is kept in: the text trimmed, the
// leading # characters dropped, trimmed again, each run of white space turned
// into one hyphen, and one # put in front. It answers "" for a text that holds
// nothing to tag with.
func NormalizeTag(text string) string {
	body := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(text), "#"))
	if body == "" {
		return ""
	}
	return "#" + strings.Join(strings.Fields(body), "-")
}

// AddTag gives the transaction a tag, kept as NormalizeTag spells it, at the
// place after its last. A tag it has already, and a text that normalises to
// nothing, record nothing.
func (recorder *Recorder) AddTag(ctx context.Context, key TxKey, tag string) error {
	next := func(ctx context.Context) (position int64, err error) {
		err = recorder.tx.QueryRowContext(ctx, statementNextTagPosition, int64(key)).Scan(&position)
		return position, err
	}
	return recorder.addTag(ctx, txTagsTable, "tx_key", Integer(int64(key)), next, tag)
}

// RemoveTag takes a tag off the transaction by the text it is kept in, and
// moves no other. A tag the transaction does not have records nothing.
func (recorder *Recorder) RemoveTag(ctx context.Context, key TxKey, tag string) error {
	return recorder.Delete(ctx, txTagsTable, []Value{Integer(int64(key)), Text(tag)})
}

// AddFixedTag gives an establishment a tag that every transaction of it
// carries, with the rules of AddTag.
func (recorder *Recorder) AddFixedTag(ctx context.Context, establishment, tag string) error {
	next := func(ctx context.Context) (position int64, err error) {
		err = recorder.tx.QueryRowContext(ctx, statementNextFixedTagPosition, establishment).Scan(&position)
		return position, err
	}
	return recorder.addTag(ctx, establishmentTagsTable, "key", Text(establishment), next, tag)
}

// RemoveFixedTag takes a fixed tag off an establishment, with the rules of
// RemoveTag.
func (recorder *Recorder) RemoveFixedTag(ctx context.Context, establishment, tag string) error {
	return recorder.Delete(ctx, establishmentTagsTable, []Value{Text(establishment), Text(tag)})
}

func (recorder *Recorder) addTag(ctx context.Context, table, ownerColumn string, owner Value, next nextPlace, tag string) error {
	name := NormalizeTag(tag)
	if name == "" {
		return nil
	}
	if err := recorder.usable(); err != nil {
		return err
	}
	view := recorder.View()
	defer view.Close()
	_, held, err := view.JournaledRow(ctx, table, []Value{owner, Text(name)})
	if err != nil {
		return recorder.fail(err)
	}
	if held {
		return nil
	}
	position, err := next(ctx)
	if err != nil {
		return recorder.fail(err)
	}
	return recorder.Put(ctx, table, Row{ownerColumn: owner, "tag": Text(name), "position": Integer(position)})
}

// SetNote keeps the note as it was typed, spaces included. An empty note is
// the default: a transaction with nothing else said about it has no row left.
func (recorder *Recorder) SetNote(ctx context.Context, key TxKey, note string) error {
	return recorder.Set(ctx, txStateTable, []Value{Integer(int64(key))}, Row{"note": Text(note)})
}

// SetNickname keeps the name the person gives an establishment or a person,
// trimmed; a blank one takes it away. It is stored by the establishment's key,
// so every transaction of it reads it.
func (recorder *Recorder) SetNickname(ctx context.Context, establishment, nickname string) error {
	value := Null
	if trimmed := strings.TrimSpace(nickname); trimmed != "" {
		value = Text(trimmed)
	}
	return recorder.Set(ctx, establishmentsTable, []Value{Text(establishment)}, Row{"nickname": value})
}

// SetDefaultLine keeps the budget line an establishment's transactions are put
// on from now on, by the line's id. The store checks no line: the table refuses
// an id the budget never minted, and the write then fails when the change
// commits. It touches no transaction.
func (recorder *Recorder) SetDefaultLine(ctx context.Context, establishment string, line LineID) error {
	return recorder.Set(ctx, establishmentsTable, []Value{Text(establishment)}, Row{"default_line_id": Integer(int64(line))})
}
