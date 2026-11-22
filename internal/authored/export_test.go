package authored

import (
	"context"
	"database/sql"
	"io/fs"
)

// EmbeddedMigrations exposes the set the binary carries, for tests that add to it.
func EmbeddedMigrations() fs.FS { return embeddedMigrations() }

// AllStatements exposes the registry of statements the package executes.
func AllStatements() map[string]string { return allStatements }

// Pool exposes the handle's connections, for tests that hold several at once.
func (database *Database) Pool() *sql.DB { return database.pool }

// BeginRead and BeginWrite expose the two kinds of transaction the handle offers.
func (database *Database) BeginRead(ctx context.Context) (*sql.Tx, error) {
	return database.beginRead(ctx)
}

func (database *Database) BeginWrite(ctx context.Context) (*sql.Tx, error) {
	return database.beginWrite(ctx)
}

// Write exposes the write transaction that keeps all of fn or nothing.
func (database *Database) Write(ctx context.Context, fn func(*sql.Tx) error) error {
	return database.write(ctx, fn)
}
