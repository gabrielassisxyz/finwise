package ingest

import (
	"errors"
	"io/fs"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// Failure is the kind of a run that failed. A failed run changed nothing, and
// the next call tries again. A caller logs the kind and never the text of what
// caused it: an error of the ledger's reader can quote a row's id.
type Failure int

const (
	// FailureSnapshotUnavailable: the snapshot is missing, is not a readable
	// database, declares a schema version the reader does not know, or records
	// no projected run.
	FailureSnapshotUnavailable Failure = iota + 1
	// FailureLedger: the snapshot opened, and a read of it failed.
	FailureLedger
	// FailureUnplaced: the rules did not place every row of the ledger, so the
	// list would be silently short of a transaction.
	FailureUnplaced
	// FailureDatabase: the app's database failed, a lock not freed in time
	// included; its transaction was rolled back.
	FailureDatabase
)

func (failure Failure) String() string {
	switch failure {
	case FailureSnapshotUnavailable:
		return "snapshot-unavailable"
	case FailureLedger:
		return "ledger-read"
	case FailureUnplaced:
		return "rows-unplaced"
	case FailureDatabase:
		return "database"
	}
	return "unknown"
}

// Error is every failure of Run. Its text is a fixed sentence per kind; the
// cause is kept for errors.As and never printed.
type Error struct {
	Failure Failure
	cause   error
}

func (err *Error) Error() string {
	switch err.Failure {
	case FailureSnapshotUnavailable:
		return "ingest: the ledger snapshot is unavailable; nothing was taken in"
	case FailureLedger:
		return "ingest: the ledger snapshot could not be read; nothing was taken in"
	case FailureUnplaced:
		return "ingest: the rules did not place every row of the ledger; nothing was taken in"
	}
	return "ingest: the app's database failed; nothing was taken in"
}

func (err *Error) Unwrap() error { return err.cause }

// FailureOf is the kind of a failure Run returned, and false for any other error.
func FailureOf(err error) (Failure, bool) {
	var failed *Error
	if !errors.As(err, &failed) {
		return 0, false
	}
	return failed.Failure, true
}

func ledgerFailure(err error) error {
	var missing *fs.PathError
	if errors.As(err, &missing) || ledger.IsUnreadableSnapshot(err) || ledger.IsUnsupportedVersion(err) || ledger.IsNotProjected(err) {
		return &Error{Failure: FailureSnapshotUnavailable, cause: err}
	}
	return &Error{Failure: FailureLedger, cause: err}
}

func databaseFailure(err error) error {
	if _, classified := FailureOf(err); classified {
		return err
	}
	return &Error{Failure: FailureDatabase, cause: err}
}
