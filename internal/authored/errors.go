package authored

import (
	"errors"
	"fmt"
)

// ErrNoClock is returned by Open when the options carry no clock: the month a
// database was created in must come from the caller, never from the host.
var ErrNoClock = errors.New("authored: no clock was given")

// Failure says which step of opening the database refused or failed.
type Failure int

const (
	// FailureMissingDirectory: the directory the database would live in does not exist.
	FailureMissingDirectory Failure = iota + 1
	// FailureNotOurs: the file is not finwise's database; it was not opened.
	FailureNotOurs
	// FailureChangedMigration: a recorded migration differs from the binary's file of that name.
	FailureChangedMigration
	// FailureUnknownMigration: the database records a migration the binary does not have.
	FailureUnknownMigration
	// FailureCopy: the copy taken before migrating could not be written, or its name is taken.
	FailureCopy
	// FailureMigration: a migration did not apply; its transaction was rolled back.
	FailureMigration
	// FailureOpen: the file could not be read, stamped or opened.
	FailureOpen
)

// OpenError is every failure of Open. Its text is a fixed sentence about the
// path, and the name of a migration only when the binary has a file of that
// name: a raw SQLite error can quote a value from the file, so the cause is kept
// for errors.As and never printed.
type OpenError struct {
	Path      string
	Failure   Failure
	Migration string
	cause     error
}

func (err *OpenError) Error() string {
	switch err.Failure {
	case FailureMissingDirectory:
		return fmt.Sprintf("authored: %s: its directory does not exist", err.Path)
	case FailureNotOurs:
		return fmt.Sprintf("authored: %s is not finwise's database; it was left untouched", err.Path)
	case FailureChangedMigration:
		return fmt.Sprintf("authored: %s records migration %s with a checksum this binary does not have", err.Path, err.Migration)
	case FailureUnknownMigration:
		return fmt.Sprintf("authored: %s records a migration this binary does not have", err.Path)
	case FailureCopy:
		return fmt.Sprintf("authored: %s: the copy before migrating could not be written, so nothing was migrated", err.Path)
	case FailureMigration:
		return fmt.Sprintf("authored: %s: migration %s failed and was rolled back", err.Path, err.Migration)
	default:
		return fmt.Sprintf("authored: %s could not be opened", err.Path)
	}
}

func (err *OpenError) Unwrap() error { return err.cause }

// TransactionError is a read or a write that did not complete; a write that
// returns it kept nothing.
type TransactionError struct {
	cause error
}

func (err *TransactionError) Error() string {
	return "authored: the transaction did not complete and was rolled back"
}

func (err *TransactionError) Unwrap() error { return err.cause }
