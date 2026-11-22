// Package ledger reads pluggy-sync's snapshot through typed, read-only queries.
//
// The snapshot is replaced by an atomic rename. A View holds one handle and one
// read transaction, so every query made through it sees the file that was there
// when it opened; it is closed when its work is done, and the next view opens
// the file that is there then.
package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"

	"modernc.org/sqlite"
)

// supportedUserVersion is the only schema version the queries were written
// against. The collector promises readers nothing about it, so anything else is
// refused before a query runs.
const supportedUserVersion = 1

// UnsupportedVersionError means the snapshot's PRAGMA user_version is not one
// this reader knows.
type UnsupportedVersionError struct {
	Found int
}

func (err *UnsupportedVersionError) Error() string {
	return fmt.Sprintf("ledger: snapshot has user_version %d, this reader supports only %d",
		err.Found, supportedUserVersion)
}

// UnreadableSnapshotError means the snapshot file is there but SQLite cannot
// read it as a database: it is not one, or it is damaged. It is not an absent
// file and not another schema version, which have their own errors, and its text
// says nothing about the file's content.
type UnreadableSnapshotError struct {
	cause error
}

func (err *UnreadableSnapshotError) Error() string {
	return "ledger: snapshot is not a readable SQLite database"
}

func (err *UnreadableSnapshotError) Unwrap() error { return err.cause }

// The primary result codes of SQLite that mean a file is not usable as a
// database: SQLITE_CORRUPT and SQLITE_NOTADB.
const (
	sqliteCorrupt = 11
	sqliteNotADB  = 26
)

// asUnreadable turns SQLite's refusal of a file into the typed error and leaves
// every other error as it was.
func asUnreadable(err error) error {
	var refusal *sqlite.Error
	if !errors.As(err, &refusal) {
		return err
	}
	if code := refusal.Code() & 0xff; code == sqliteCorrupt || code == sqliteNotADB {
		return &UnreadableSnapshotError{cause: err}
	}
	return err
}

// IsUnreadableSnapshot reports whether err is an UnreadableSnapshotError.
func IsUnreadableSnapshot(err error) bool {
	var target *UnreadableSnapshotError
	return errors.As(err, &target)
}

// Reader answers queries over one snapshot file.
type Reader struct {
	path string
}

// New returns a reader of the snapshot at path. It does not touch the file.
func New(path string) *Reader {
	return &Reader{path: path}
}

// openReadOnly refuses writes twice, in the connection mode and in the session:
// either alone is enough, and neither depends on the other.
func openReadOnly(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("ledger: snapshot: %w", err)
	}
	query := url.Values{}
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String()

	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("ledger: open snapshot: %w", err)
	}
	database.SetMaxOpenConns(1)
	return database, nil
}

func requireSupportedVersion(ctx context.Context, tx *sql.Tx) error {
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("ledger: read user_version: %w", err)
	}
	if version != supportedUserVersion {
		return &UnsupportedVersionError{Found: version}
	}
	return nil
}

// IsUnsupportedVersion reports whether err is an UnsupportedVersionError.
func IsUnsupportedVersion(err error) bool {
	var target *UnsupportedVersionError
	return errors.As(err, &target)
}
