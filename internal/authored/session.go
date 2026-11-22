package authored

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const statementStoredPIN = `SELECT hash FROM pin WHERE id = 1`

const statementStorePIN = `
INSERT INTO pin (id, hash, changed_at) VALUES (1, ?, ?)
ON CONFLICT (id) DO UPDATE SET hash = excluded.hash, changed_at = excluded.changed_at`

// StoredPINHash is the hash of the PIN the person set in the app, and false
// until they have. It is the stored form, never a PIN.
func (database *Database) StoredPINHash(ctx context.Context) (hash string, found bool, err error) {
	err = database.read(ctx, func(tx *sql.Tx) error {
		scanned := tx.QueryRowContext(ctx, statementStoredPIN).Scan(&hash)
		if errors.Is(scanned, sql.ErrNoRows) {
			return nil
		}
		found = scanned == nil
		return scanned
	})
	return hash, found && err == nil, err
}

// StorePINHash replaces the stored hash, with the instant of the change, in one
// transaction. The change is not a recorded one: nothing is written to the
// journal, and nothing can undo it.
func (database *Database) StorePINHash(ctx context.Context, hash string, at time.Time) error {
	return database.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, statementStorePIN, hash, formatInstant(at))
		return err
	})
}
