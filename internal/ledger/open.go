// Package ledger reads pluggy-sync's snapshot through typed, read-only queries.
//
// The snapshot is replaced by an atomic rename and nothing promises what an open
// handle sees across the swap, so every call opens the file, runs and closes.
package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"

	_ "modernc.org/sqlite"
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

// read runs fn inside one read transaction on a fresh handle, after the
// version check, so that a call that issues several statements sees one state.
func (reader *Reader) read(ctx context.Context, fn func(*sql.Tx) error) error {
	database, err := openReadOnly(reader.path)
	if err != nil {
		return err
	}
	defer database.Close()

	tx, err := database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("ledger: begin read: %w", err)
	}
	defer tx.Rollback()

	if err := requireSupportedVersion(ctx, tx); err != nil {
		return err
	}
	return fn(tx)
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
