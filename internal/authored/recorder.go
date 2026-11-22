package authored

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// ChangeFacts is what a change is recorded with. At comes from the caller; the
// package never reads the host's clock.
type ChangeFacts struct {
	Author Author
	Method string
	At     time.Time
}

// Recorder holds one immediate transaction and one open change, and is the
// only way a journaled table is written. Its change's row is written with the
// first effect or when the id is first asked for, so a write that records
// nothing and answers no id leaves no trace in the journal.
type Recorder struct {
	tx       *sql.Tx
	registry *Registry
	facts    ChangeFacts
	id       ChangeID
	written  bool
	seq      int64
	// failed is the first failure of a recorded write. The transaction is then
	// in a state no effect describes, so nothing more is written and it is
	// rolled back whatever the handler returns.
	failed error
	closed bool
}

// ErrInvalidAuthor is a change opened with an author outside the three.
var ErrInvalidAuthor = errors.New("authored: a change's author is person, rule or model")

// ErrNoMethod is a change opened without the API method that makes it.
var ErrNoMethod = errors.New("authored: a change names the method that makes it")

// ErrRecorderClosed is a recorder used after its transaction ended.
var ErrRecorderClosed = errors.New("authored: the change's transaction has ended")

// Change runs write with a recorder over one immediate transaction and
// commits what it recorded. Any failure, write's own included, rolls the whole
// of it back; write's error is returned as it is, so a refusal keeps its type.
func (database *Database) Change(ctx context.Context, facts ChangeFacts, write func(*Recorder) error) error {
	if !facts.Author.valid() {
		return ErrInvalidAuthor
	}
	if facts.Method == "" {
		return ErrNoMethod
	}
	return database.journalTransaction(ctx, func(tx *sql.Tx) error {
		recorder := &Recorder{tx: tx, registry: database.journal, facts: facts}
		return recorder.run(write)
	})
}

func (recorder *Recorder) run(write func(*Recorder) error) error {
	err := write(recorder)
	recorder.closed = true
	if err != nil {
		return err
	}
	return recorder.failed
}

// journalTransaction is one immediate transaction with foreign keys checked
// at its commit. fn's error comes back unwrapped; a failed begin or commit is
// a TransactionError. However fn ends, a panic included, the transaction is
// rolled back unless it committed, so the connection never keeps the write
// lock; the panic itself goes on unchanged.
func (database *Database) journalTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := database.beginWrite(ctx)
	if err != nil {
		return &TransactionError{cause: err}
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, statementDeferForeignKeys); err != nil {
		return &TransactionError{cause: err}
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return &TransactionError{cause: err}
	}
	return nil
}

// ID is the change's id, writing its row if no effect has yet. A handler whose
// result may not be null asks for it even when it changed nothing, and leaves
// an empty change whose undo succeeds and restores nothing.
func (recorder *Recorder) ID(ctx context.Context) (ChangeID, error) {
	if err := recorder.usable(); err != nil {
		return 0, err
	}
	if err := recorder.ensureRow(ctx); err != nil {
		return 0, recorder.fail(err)
	}
	return recorder.id, nil
}

func (recorder *Recorder) ensureRow(ctx context.Context) error {
	if recorder.written {
		return nil
	}
	facts := recorder.facts
	result, err := recorder.tx.ExecContext(ctx, statementInsertChange,
		formatInstant(facts.At), string(brday.DayOf(facts.At)), string(facts.Author), facts.Method)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	recorder.id, recorder.written = ChangeID(id), true
	return nil
}

func (recorder *Recorder) usable() error {
	if recorder.closed {
		return ErrRecorderClosed
	}
	return recorder.failed
}

func (recorder *Recorder) fail(err error) error {
	if recorder.failed != nil {
		return recorder.failed
	}
	var failed *TransactionError
	if !errors.As(err, &failed) {
		failed = &TransactionError{cause: err}
	}
	recorder.failed = failed
	return failed
}

func (recorder *Recorder) record(ctx context.Context, unit Unit, before, after sql.NullString) error {
	if err := recorder.ensureRow(ctx); err != nil {
		return err
	}
	recorder.seq++
	column := sql.NullString{String: unit.Column, Valid: unit.Column != ""}
	_, err := recorder.tx.ExecContext(ctx, statementInsertEffect,
		int64(recorder.id), recorder.seq, unit.Table, unit.Key, column, before, after)
	return err
}

func present(text string) sql.NullString { return sql.NullString{String: text, Valid: true} }

var absent = sql.NullString{}

// Set changes columns of an attribute table's row and records one effect per
// column whose content changes, in column order. In a table of optional facts
// a missing row is its defaults, so the first fact creates the row without
// owning it; in a table of things created whole the row must exist.
func (recorder *Recorder) Set(ctx context.Context, tableName string, key []Value, values Row) error {
	table, err := recorder.writable(tableName, ShapeAttribute)
	if err != nil {
		return err
	}
	if err := checkKey(table, key); err != nil {
		return err
	}
	for name, value := range values {
		if table.column(name) == nil || table.isKey(name) {
			return fmt.Errorf("authored: %s has no column %s that a write may set", table.Name, name)
		}
		if err := checkText(value); err != nil {
			return err
		}
	}
	return recorder.guard(recorder.set(ctx, table, key, values))
}

func (recorder *Recorder) set(ctx context.Context, table *Table, key []Value, values Row) error {
	current, found, err := readContent(ctx, recorder.tx, table, key)
	if err != nil {
		return err
	}
	if !found && table.createdWhole() {
		return fmt.Errorf("authored: %s has no row at %s to set; a thing created whole is created with Create", table.Name, EncodeKey(key...))
	}
	next := Row{}
	for name, value := range current {
		next[name] = value
	}
	var changed []Column
	for _, column := range table.Columns {
		value, given := values[column.Name]
		if given && value != current[column.Name] {
			next[column.Name] = value
			changed = append(changed, column)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	if err := putRow(ctx, recorder.tx, table, next, found); err != nil {
		return err
	}
	keyText := EncodeKey(key...)
	for _, column := range changed {
		unit := Unit{Table: table.Name, Key: keyText, Column: column.Name}
		if err := recorder.record(ctx, unit, present(current[column.Name].canonical()), present(next[column.Name].canonical())); err != nil {
			return err
		}
	}
	return nil
}

// Create writes a new row of a table of things created whole, and records its
// creation as one effect on the row, which is the unit that a later change to
// any of its columns makes stale.
func (recorder *Recorder) Create(ctx context.Context, tableName string, row Row) error {
	table, err := recorder.writable(tableName, ShapeAttribute)
	if err != nil {
		return err
	}
	if !table.createdWhole() {
		return fmt.Errorf("authored: %s is a table of optional facts, written with Set", table.Name)
	}
	if err := checkRow(table, row); err != nil {
		return err
	}
	return recorder.guard(recorder.create(ctx, table, row))
}

func (recorder *Recorder) create(ctx context.Context, table *Table, row Row) error {
	key := table.keyOf(row)
	if _, found, err := readRow(ctx, recorder.tx, table, key); err != nil || found {
		if err == nil {
			err = fmt.Errorf("authored: %s already has a row at %s", table.Name, EncodeKey(key...))
		}
		return err
	}
	if err := insertRow(ctx, recorder.tx, table, row); err != nil {
		return err
	}
	return recorder.record(ctx, Unit{Table: table.Name, Key: EncodeKey(key...)}, absent, present(encodeRow(table, row)))
}

// Put makes a set table hold row at its key: one effect for a row appearing,
// one for a row whose other columns change, none for the row already there.
func (recorder *Recorder) Put(ctx context.Context, tableName string, row Row) error {
	table, err := recorder.writable(tableName, ShapeSet)
	if err != nil {
		return err
	}
	if err := checkRow(table, row); err != nil {
		return err
	}
	return recorder.guard(recorder.put(ctx, table, row))
}

func (recorder *Recorder) put(ctx context.Context, table *Table, row Row) error {
	key := table.keyOf(row)
	current, found, err := readRow(ctx, recorder.tx, table, key)
	if err != nil {
		return err
	}
	before, after := absent, present(encodeRow(table, row))
	if found {
		before = present(encodeRow(table, current))
	}
	if before == after {
		return nil
	}
	if err := putRow(ctx, recorder.tx, table, row, found); err != nil {
		return err
	}
	return recorder.record(ctx, Unit{Table: table.Name, Key: EncodeKey(key...)}, before, after)
}

// Delete removes the row at key of a set table or of a table of things created
// whole. The rows it owns go first, each recorded as an effect of this change,
// so that undoing it brings owner and owned rows back together. A missing row
// records nothing.
func (recorder *Recorder) Delete(ctx context.Context, tableName string, key []Value) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	table, err := recorder.registry.table(tableName)
	if err != nil {
		return err
	}
	if table.optionalFacts() {
		return fmt.Errorf("authored: %s is a table of optional facts; a row there is cleared by setting its defaults", table.Name)
	}
	if err := checkKey(table, key); err != nil {
		return err
	}
	return recorder.guard(recorder.delete(ctx, table, key))
}

func (recorder *Recorder) delete(ctx context.Context, table *Table, key []Value) error {
	current, found, err := readRow(ctx, recorder.tx, table, key)
	if err != nil || !found {
		return err
	}
	for _, owned := range recorder.registry.owned[table.Name] {
		rows, err := readOwned(ctx, recorder.tx, owned, key)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := recorder.delete(ctx, owned, owned.keyOf(row)); err != nil {
				return err
			}
		}
	}
	if err := deleteRow(ctx, recorder.tx, table, key); err != nil {
		return err
	}
	return recorder.record(ctx, Unit{Table: table.Name, Key: EncodeKey(key...)}, present(encodeRow(table, current)), absent)
}

func (recorder *Recorder) writable(tableName string, shape Shape) (*Table, error) {
	if err := recorder.usable(); err != nil {
		return nil, err
	}
	table, err := recorder.registry.table(tableName)
	if err != nil {
		return nil, err
	}
	if table.Shape != shape {
		return nil, fmt.Errorf("authored: %s does not have the shape this write is for", table.Name)
	}
	return table, nil
}

// guard turns a failure halfway through a recorded write into the recorder's
// failure, after which nothing more is written.
func (recorder *Recorder) guard(err error) error {
	if err == nil {
		return nil
	}
	return recorder.fail(err)
}

func checkKey(table *Table, key []Value) error {
	if len(key) != len(table.Key) {
		return fmt.Errorf("authored: a key of %s has %d values, not %d", table.Name, len(key), len(table.Key))
	}
	for _, value := range key {
		if err := checkText(value); err != nil {
			return err
		}
	}
	return nil
}

func checkRow(table *Table, row Row) error {
	if len(row) != len(table.Columns) {
		return fmt.Errorf("authored: a row of %s names %d columns, not its %d", table.Name, len(row), len(table.Columns))
	}
	for name, value := range row {
		if table.column(name) == nil {
			return fmt.Errorf("authored: %s has no column %s", table.Name, name)
		}
		if err := checkText(value); err != nil {
			return err
		}
	}
	return nil
}

// checkText refuses a text the canonical form could not carry back exactly.
func checkText(value Value) error {
	if text, ok := value.Str(); ok && !utf8.ValidString(text) {
		return errors.New("authored: a text to journal is not valid UTF-8")
	}
	return nil
}

// Extend reopens a committed change made by method so that write appends its
// effects to it and one undo takes back both. The origin must exist, not be
// undone, have been made by method, not be extended yet, and its effects must
// still hold by the test of an undo's step 2; anything else is stale.
func (database *Database) Extend(ctx context.Context, origin ChangeID, method string, write func(*Recorder) error) error {
	return database.journalTransaction(ctx, func(tx *sql.Tx) error {
		state, err := checkOrigin(ctx, tx, database.journal, origin, method)
		if err != nil {
			return err
		}
		var seq int64
		if err := tx.QueryRowContext(ctx, statementLastSeq, int64(origin)).Scan(&seq); err != nil {
			return &TransactionError{cause: err}
		}
		if _, err := tx.ExecContext(ctx, statementMarkExtended, int64(origin)); err != nil {
			return &TransactionError{cause: err}
		}
		recorder := &Recorder{
			tx: tx, registry: database.journal, id: origin, written: true, seq: seq,
			facts: ChangeFacts{Method: state.method},
		}
		if err := recorder.run(write); err != nil {
			return err
		}
		return extensionStaysLast(ctx, tx, database.journal, origin, seq)
	})
}

func checkOrigin(ctx context.Context, tx *sql.Tx, registry *Registry, origin ChangeID, method string) (changeState, error) {
	state, found, err := loadChange(ctx, tx, origin)
	switch {
	case err != nil:
		return changeState{}, &TransactionError{cause: err}
	case !found:
		return changeState{}, stale(origin, CauseUnknown)
	case state.undone:
		return changeState{}, stale(origin, CauseUndone)
	case state.method != method:
		return changeState{}, stale(origin, CauseOtherMethod)
	case state.extended:
		return changeState{}, stale(origin, CauseExtended)
	}
	effects, err := loadEffects(ctx, tx, origin)
	if err != nil {
		return changeState{}, &TransactionError{cause: err}
	}
	if err := stillInForce(ctx, tx, registry, origin, collapse(effects)); err != nil {
		return changeState{}, err
	}
	return state, nil
}

// extensionStaysLast refuses an extension that wrote a unit a later change in
// force also has an effect on. Its effects carry the origin's id, so they would
// sort before that later change although they came after it, and the journal's
// order, which every undo decides by, would then be wrong about that unit.
func extensionStaysLast(ctx context.Context, tx *sql.Tx, registry *Registry, origin ChangeID, previousSeq int64) error {
	effects, err := loadEffects(ctx, tx, origin)
	if err != nil {
		return &TransactionError{cause: err}
	}
	appended := slices.DeleteFunc(effects, func(found effect) bool { return found.seq <= previousSeq })
	overlap, err := laterChangeOverlaps(ctx, tx, registry, origin, collapse(appended))
	if err != nil {
		return &TransactionError{cause: err}
	}
	if overlap {
		return stale(origin, CauseLaterChange)
	}
	return nil
}
