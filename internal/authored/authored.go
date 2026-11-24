// Package authored owns finwise.db, the app's own database, where what the
// person authors is kept. It is the only package that executes SQL against that
// file; it never opens the ledger snapshot, and the two stores are joined in Go,
// never in SQL. It logs nothing: every failure is returned.
package authored

import (
	"context"
	"database/sql"
	"io/fs"
	"net/url"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Options is what Open needs besides the path.
type Options struct {
	// Now is the clock. It is required: the month the database was created in
	// and the name of the copy taken before a migration both come from it.
	Now func() time.Time
	// Migrations is the set of migration files; nil means the set embedded in
	// the binary. A test hands it a changed, unknown, failing or reordered set.
	Migrations fs.FS
	// Journal is the registry of journaled tables; nil means the package's
	// own. A test hands it one naming the scratch tables its migrations add.
	Journal *Registry
	// MustExist refuses a missing or empty file instead of creating a new
	// database, so a command that only reads and updates an existing one never
	// creates a file at a mistyped path.
	MustExist bool
}

// Database is the app's database, open for the life of the process. Unlike the
// ledger snapshot, nobody swaps this file, so it is not reopened per call.
type Database struct {
	path    string
	pool    *sql.DB
	now     func() time.Time
	journal *Registry
}

// Open recognises the file at path, creates it if it is missing or empty,
// brings it up to date with the migration set and keeps it open. A file that is
// not finwise's is refused before a byte of it is written.
func Open(ctx context.Context, path string, options Options) (*Database, error) {
	if options.Now == nil {
		return nil, ErrNoClock
	}
	set, err := loadMigrations(options.Migrations)
	if err != nil {
		return nil, &OpenError{Path: path, Failure: FailureOpen, cause: err}
	}
	journal := options.Journal
	if journal == nil {
		if journal, err = productionRegistry(); err != nil {
			return nil, &OpenError{Path: path, Failure: FailureOpen, cause: err}
		}
	}
	kind, err := recognize(path)
	if err != nil {
		return nil, err
	}
	if kind == fileNew {
		if options.MustExist {
			return nil, &OpenError{Path: path, Failure: FailureMissing}
		}
		if err := stamp(ctx, path); err != nil {
			return nil, err
		}
	}
	database, err := openPool(ctx, path, options.Now)
	if err != nil {
		return nil, err
	}
	if err := database.migrate(ctx, set); err != nil {
		database.pool.Close()
		return nil, err
	}
	database.journal = journal
	if err := database.read(ctx, func(tx *sql.Tx) error { return journal.verify(ctx, tx) }); err != nil {
		database.pool.Close()
		return nil, &OpenError{Path: path, Failure: FailureOpen, cause: err}
	}
	return database, nil
}

// openPool carries every connection setting in the DSN, so each connection of
// the pool has them. Write transactions begin immediate, so a writer waits for
// the lock at its begin rather than failing at its first write; read-only
// transactions begin deferred, so under write-ahead mode a reader never waits
// for a writer. The pool has no connection limit for the same reason.
func openPool(ctx context.Context, path string, now func() time.Time) (*Database, error) {
	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "synchronous(FULL)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Set("_txlock", "immediate")
	address, err := fileAddress(path, query)
	if err != nil {
		return nil, &OpenError{Path: path, Failure: FailureOpen, cause: err}
	}
	pool, err := sql.Open("sqlite", address)
	if err != nil {
		return nil, &OpenError{Path: path, Failure: FailureOpen, cause: err}
	}
	if err := pool.PingContext(ctx); err != nil {
		pool.Close()
		return nil, &OpenError{Path: path, Failure: FailureOpen, cause: err}
	}
	return &Database{path: path, pool: pool, now: now}, nil
}

// fileAddress is the file: address SQLite opens path through. A relative path is
// made absolute first: printed as it is, the address reads its first segment as
// a host name, which SQLite refuses. Symbolic links are left alone, because the
// name is what has to be opened.
func fileAddress(path string, query url.Values) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return (&url.URL{Scheme: "file", Path: absolute, RawQuery: query.Encode()}).String(), nil
}

// Close closes every connection; the last one checkpoints the -wal file into
// the database and removes it.
func (database *Database) Close() error {
	return database.pool.Close()
}

func (database *Database) beginRead(ctx context.Context) (*sql.Tx, error) {
	return database.pool.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
}

func (database *Database) beginWrite(ctx context.Context) (*sql.Tx, error) {
	return database.pool.BeginTx(ctx, nil)
}

// read runs fn in a read-only transaction, which sees one state of the file.
func (database *Database) read(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := database.beginRead(ctx)
	if err != nil {
		return &TransactionError{cause: err}
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return &TransactionError{cause: err}
	}
	return nil
}

// write runs fn in one immediate transaction: either all of it is kept or,
// when fn, the begin (a lock not freed within busy_timeout) or the commit
// fails, none of it.
func (database *Database) write(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := database.beginWrite(ctx)
	if err != nil {
		return &TransactionError{cause: err}
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return &TransactionError{cause: err}
	}
	if err := tx.Commit(); err != nil {
		return &TransactionError{cause: err}
	}
	return nil
}
