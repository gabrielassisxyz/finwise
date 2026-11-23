package authored

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// A restore of one part of a change runs inside a savepoint, so a refusal
// takes back the restore alone and leaves the open change to its caller.
const (
	statementSavepointRestore = `SAVEPOINT restore_part`
	statementReleaseRestore   = `RELEASE restore_part`
	statementRollbackRestore  = `ROLLBACK TO restore_part`
)

// StaleCause says why a change could not be taken back or extended. The API
// answers every one of them as refused with the reason stale_change.
type StaleCause int

const (
	// CauseUnknown: no change has that id.
	CauseUnknown StaleCause = iota + 1
	// CauseUndone: the change was already undone.
	CauseUndone
	// CauseLaterChange: a later change still in force has an effect on what it
	// would restore, even if that change put the old value back.
	CauseLaterChange
	// CauseOverwritten: what it would restore no longer holds what it left,
	// because a write went around the journal.
	CauseOverwritten
	// CauseConstraint: the database refuses the state it would bring back.
	CauseConstraint
	// CauseCheck: a registered check refuses the state it would bring back.
	CauseCheck
	// CauseOtherMethod: an extension of a change made by another method.
	CauseOtherMethod
	// CauseExtended: an extension of a change already extended.
	CauseExtended
	// CauseOwnedRowHeld: a row it would remove owns a row that it does not
	// restore, in a table whose rows hold their owner.
	CauseOwnedRowHeld
)

// StaleChangeError is a refusal: nothing was changed.
type StaleChangeError struct {
	Change ChangeID
	Cause  StaleCause
	cause  error
}

func (err *StaleChangeError) Error() string {
	return fmt.Sprintf("authored: change %s was changed again since and cannot be taken back (cause %d)", err.Change, err.Cause)
}

func (err *StaleChangeError) Unwrap() error { return err.cause }

func stale(id ChangeID, cause StaleCause) *StaleChangeError {
	return &StaleChangeError{Change: id, Cause: cause}
}

// Undo takes back every effect of a change, the ones an extension appended
// included, by restoring each unit's first before, and marks it undone at the
// given instant. It refuses, changing nothing, when the change is unknown or
// undone, when a later change in force has an effect on any unit it would
// restore, when a unit no longer holds what the change left, when removing a
// row would take along an owned row it does not restore whose table holds its
// owner, or when the database or a registered check refuses the restored
// state. It records no change and cannot itself be undone.
func (database *Database) Undo(ctx context.Context, id ChangeID, at time.Time) error {
	err := database.journalTransaction(ctx, func(tx *sql.Tx) error {
		state, found, err := loadChange(ctx, tx, id)
		switch {
		case err != nil:
			return &TransactionError{cause: err}
		case !found:
			return stale(id, CauseUnknown)
		case state.undone:
			return stale(id, CauseUndone)
		}
		effects, err := loadEffects(ctx, tx, id)
		if err != nil {
			return &TransactionError{cause: err}
		}
		units := collapse(effects)
		if err := restorable(ctx, tx, database.journal, id, units); err != nil {
			return err
		}
		if err := restoreUnrecorded(ctx, tx, database.journal, units); err != nil {
			return refusedOrFailed(id, err)
		}
		if err := runChecks(ctx, tx, database.journal, id, units); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, statementMarkUndone, formatInstant(at), int64(id)); err != nil {
			return &TransactionError{cause: err}
		}
		return nil
	})
	var failed *TransactionError
	if errors.As(err, &failed) && isConstraint(failed.cause) {
		// A foreign key the restore left unsatisfied is found at the commit.
		return &StaleChangeError{Change: id, Cause: CauseConstraint, cause: failed.cause}
	}
	return err
}

// stillInForce is step 2: no later change in force has an effect on any of
// units, and every unit holds what the change left in it.
func stillInForce(ctx context.Context, tx *sql.Tx, registry *Registry, id ChangeID, units []collapsed) error {
	overlap, err := laterChangeOverlaps(ctx, tx, registry, id, units)
	if err != nil {
		return &TransactionError{cause: err}
	}
	if overlap {
		return stale(id, CauseLaterChange)
	}
	plans, err := planRows(registry, units)
	if err != nil {
		return &TransactionError{cause: err}
	}
	for _, plan := range plans {
		holds, err := plan.holds(ctx, tx)
		if err != nil {
			return &TransactionError{cause: err}
		}
		if !holds {
			return stale(id, CauseOverwritten)
		}
	}
	return nil
}

// restorable is step 2 for an undo and for the restore of a part. An extension
// removes nothing, so its origin is judged by stillInForce alone.
func restorable(ctx context.Context, tx *sql.Tx, registry *Registry, id ChangeID, units []collapsed) error {
	if err := stillInForce(ctx, tx, registry, id, units); err != nil {
		return err
	}
	return removesNoHeldRow(ctx, tx, registry, id, units)
}

// removesNoHeldRow is the rest of step 2: removing an owner's row takes along
// every row it owns, and one that the undo does not itself restore may only be
// taken from a table whose rows follow their owner. Such a row in a table whose
// rows hold their owner was written by another change or around the journal,
// so the undo is refused here, before anything is written.
func removesNoHeldRow(ctx context.Context, tx *sql.Tx, registry *Registry, id ChangeID, units []collapsed) error {
	restored := make(map[Unit]bool, len(units))
	for _, unit := range units {
		restored[unit.unit] = true
	}
	plans, err := planRows(registry, units)
	if err != nil {
		return &TransactionError{cause: err}
	}
	for _, plan := range plans {
		if plan.row == nil || plan.row.before.Valid {
			continue
		}
		held, err := leavesAHeldRow(ctx, tx, registry, plan, restored)
		if err != nil {
			return &TransactionError{cause: err}
		}
		if held {
			return stale(id, CauseOwnedRowHeld)
		}
	}
	return nil
}

// leavesAHeldRow says whether the row that plan removes owns a row that is not
// among restored, in a table whose rows hold their owner.
func leavesAHeldRow(ctx context.Context, tx *sql.Tx, registry *Registry, plan *rowPlan, restored map[Unit]bool) (bool, error) {
	for _, owned := range registry.owned[plan.table.Name] {
		if owned.OwnedBy.OnUndo == RowsFollowTheirOwner {
			continue
		}
		rows, err := readOwned(ctx, tx, owned, plan.key)
		if err != nil {
			return false, err
		}
		for _, row := range rows {
			if !restored[Unit{Table: owned.Name, Key: EncodeKey(owned.keyOf(row)...)}] {
				return true, nil
			}
		}
	}
	return false, nil
}

// restoreUnrecorded is step 3 for an undo: each row is written once with its
// target, or removed once, rows first touched last first. Removing an owner
// removes the rows it owns, which step 2 left only where they are the undo's
// own or follow their owner; none of it is an effect.
func restoreUnrecorded(ctx context.Context, tx *sql.Tx, registry *Registry, units []collapsed) error {
	plans, err := planRows(registry, units)
	if err != nil {
		return err
	}
	for _, plan := range plans {
		restored, keep, err := plan.target(ctx, tx)
		if err != nil {
			return err
		}
		_, exists, err := readRow(ctx, tx, plan.table, plan.key)
		if err != nil {
			return err
		}
		if keep {
			err = putRow(ctx, tx, plan.table, restored, exists)
		} else if exists {
			err = removeWithOwned(ctx, tx, registry, plan.table, plan.key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func removeWithOwned(ctx context.Context, tx *sql.Tx, registry *Registry, table *Table, key []Value) error {
	for _, owned := range registry.owned[table.Name] {
		rows, err := readOwned(ctx, tx, owned, key)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := deleteRow(ctx, tx, owned, owned.keyOf(row)); err != nil {
				return err
			}
		}
	}
	return deleteRow(ctx, tx, table, key)
}

// runChecks is step 4: an undo never repairs by itself, so a check that
// refuses the restored state refuses the undo.
func runChecks(ctx context.Context, tx *sql.Tx, registry *Registry, id ChangeID, units []collapsed) error {
	restored := unitsOf(units)
	for _, check := range registry.checks {
		allowed, err := check(ctx, tx, restored)
		if err != nil {
			return refusedOrFailed(id, err)
		}
		if !allowed {
			return stale(id, CauseCheck)
		}
	}
	return nil
}

// refusedOrFailed tells the database refusing a restored state, which makes the
// change stale, from a failure of the database itself.
func refusedOrFailed(id ChangeID, err error) error {
	if isConstraint(err) {
		return &StaleChangeError{Change: id, Cause: CauseConstraint, cause: err}
	}
	return &TransactionError{cause: err}
}

func isConstraint(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == sqlite3.SQLITE_CONSTRAINT
}

// RestorePart takes back the units of source that selected picks, inside the
// recorder's own change: each restore is an effect of that change, source is
// not marked undone, and the recorder stays open for the caller's further
// writes. It refuses by the same rules as Undo, over the selected units only,
// and a refusal changes nothing.
func (recorder *Recorder) RestorePart(ctx context.Context, source ChangeID, selected func(Unit) bool) error {
	if err := recorder.usable(); err != nil {
		return err
	}
	if _, err := recorder.tx.ExecContext(ctx, statementSavepointRestore); err != nil {
		return recorder.fail(err)
	}
	written, seq := recorder.written, recorder.seq
	err := recorder.restorePart(ctx, source, selected)
	var refused *StaleChangeError
	if err != nil && errors.As(err, &refused) {
		if _, rollbackErr := recorder.tx.ExecContext(ctx, statementRollbackRestore); rollbackErr != nil {
			return recorder.fail(rollbackErr)
		}
		recorder.written, recorder.seq, recorder.failed = written, seq, nil
	}
	if _, releaseErr := recorder.tx.ExecContext(ctx, statementReleaseRestore); releaseErr != nil {
		return recorder.fail(releaseErr)
	}
	if err != nil && refused == nil {
		return recorder.fail(err)
	}
	return err
}

func (recorder *Recorder) restorePart(ctx context.Context, source ChangeID, selected func(Unit) bool) error {
	units, err := selectedUnits(ctx, recorder.tx, recorder.registry, source, selected)
	if err != nil {
		return err
	}
	plans, err := planRows(recorder.registry, units)
	if err != nil {
		return err
	}
	for _, plan := range plans {
		if err := recorder.restoreRecorded(ctx, plan); err != nil {
			return refusedOrFailed(source, err)
		}
	}
	return runChecks(ctx, recorder.tx, recorder.registry, source, units)
}

// selectedUnits is steps 1 and 2 over the units of source that selected picks.
func selectedUnits(ctx context.Context, tx *sql.Tx, registry *Registry, source ChangeID, selected func(Unit) bool) ([]collapsed, error) {
	state, found, err := loadChange(ctx, tx, source)
	switch {
	case err != nil:
		return nil, &TransactionError{cause: err}
	case !found:
		return nil, stale(source, CauseUnknown)
	case state.undone:
		return nil, stale(source, CauseUndone)
	}
	effects, err := loadEffects(ctx, tx, source)
	if err != nil {
		return nil, &TransactionError{cause: err}
	}
	var units []collapsed
	for _, unit := range collapse(effects) {
		if selected(unit.unit) {
			units = append(units, unit)
		}
	}
	if err := restorable(ctx, tx, registry, source, units); err != nil {
		return nil, err
	}
	return units, nil
}

// restoreRecorded is step 3 for one row as a recorded write, so its effects
// fall to the recorder's change and can be undone in turn.
func (recorder *Recorder) restoreRecorded(ctx context.Context, plan *rowPlan) error {
	restored, keep, err := plan.target(ctx, recorder.tx)
	if err != nil {
		return err
	}
	if !keep {
		return recorder.delete(ctx, plan.table, plan.key)
	}
	if plan.table.Shape == ShapeSet {
		return recorder.put(ctx, plan.table, restored)
	}
	if _, found, err := readRow(ctx, recorder.tx, plan.table, plan.key); err != nil || (!found && plan.table.createdWhole()) {
		if err != nil {
			return err
		}
		return recorder.create(ctx, plan.table, restored)
	}
	columns := Row{}
	for _, column := range plan.table.Columns {
		if !plan.table.isKey(column.Name) {
			columns[column.Name] = restored[column.Name]
		}
	}
	return recorder.set(ctx, plan.table, plan.key, columns)
}

// CanRestorePart says, without writing, whether RestorePart of the same units
// would get past steps 1 and 2. A restore the database's constraints or a
// registered check would refuse is beyond what a read can know.
func (database *Database) CanRestorePart(ctx context.Context, source ChangeID, selected func(Unit) bool) (bool, error) {
	var can bool
	err := database.read(ctx, func(tx *sql.Tx) error {
		var err error
		can, err = canRestorePart(ctx, tx, database.journal, source, selected)
		return err
	})
	return can, err
}

// canRestorePart is CanRestorePart inside a transaction the caller holds,
// read-only or not.
func canRestorePart(ctx context.Context, tx *sql.Tx, registry *Registry, source ChangeID, selected func(Unit) bool) (bool, error) {
	_, err := selectedUnits(ctx, tx, registry, source, selected)
	var refused *StaleChangeError
	if errors.As(err, &refused) {
		return false, nil
	}
	return err == nil, err
}
