package authored

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// ErrViewClosed is what a read on a closed view returns. A closed view never
// begins a second transaction: it could see another state of the file.
var ErrViewClosed = errors.New("authored: view is closed")

// View is one state of the app's database: one read-only transaction that every
// read made through it runs on, so that all of them see the file as it was at
// the first read, whatever another connection commits meanwhile. A view made
// later sees those commits.
//
// The transaction begins at the first read, not when the view is made, so a
// view nobody asks anything of holds no connection. Close hands the connection
// back to the pool; a view is meant to live for one unit of work, such as a
// request, and not beyond it.
//
// The empty view, for a server that runs without the app's database, answers
// every read with nothing authored, so the code that reads has one path.
type View struct {
	database *Database

	mu       sync.Mutex
	tx       *sql.Tx
	beginErr error
	closed   bool
}

// View returns a view of the database. It takes no connection.
func (database *Database) View() *View {
	return &View{database: database}
}

// EmptyView returns a view that answers every read with nothing authored.
func EmptyView() *View {
	return &View{}
}

// ConnectionsInUse is how many connections of the pool are lent out now: to an
// open view, or to a read or a write in progress.
func (database *Database) ConnectionsInUse() int {
	return database.pool.Stats().InUse
}

// Close ends the view's transaction and returns its connection to the pool. It
// may be called more than once, and on a view that never read.
func (view *View) Close() error {
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.closed {
		return nil
	}
	view.closed = true
	if view.tx == nil {
		return nil
	}
	err := view.tx.Rollback()
	view.tx = nil
	return err
}

// read runs fn on the view's transaction, beginning it first if no read has.
// It reports false, without calling fn, for the empty view.
func (view *View) read(ctx context.Context, fn func(*sql.Tx) error) (bool, error) {
	tx, err := view.transaction(ctx)
	if err != nil || tx == nil {
		return false, err
	}
	if err := fn(tx); err != nil {
		return true, &TransactionError{cause: err}
	}
	return true, nil
}

func (view *View) transaction(ctx context.Context) (*sql.Tx, error) {
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.closed {
		return nil, ErrViewClosed
	}
	if view.database == nil {
		return nil, nil
	}
	if view.tx == nil && view.beginErr == nil {
		// The transaction outlives the read that began it, so a deadline on that
		// read must not end the view.
		view.tx, view.beginErr = view.database.beginRead(context.WithoutCancel(ctx))
		if view.beginErr != nil {
			view.beginErr = &TransactionError{cause: view.beginErr}
		}
	}
	return view.tx, view.beginErr
}

// TrackingStartMonth is the Brazil month in which the database was created.
// found is false on the empty view.
func (view *View) TrackingStartMonth(ctx context.Context) (month brday.Month, found bool, err error) {
	var stored string
	found, err = view.read(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, statementTrackingStart).Scan(&stored)
	})
	if err != nil || !found {
		return "", false, err
	}
	month, err = brday.ParseMonth(stored)
	if err != nil {
		return "", false, &TransactionError{cause: err}
	}
	return month, true, nil
}
