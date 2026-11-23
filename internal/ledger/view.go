package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

// ErrViewClosed is what a query on a closed view returns. A closed view never
// opens the file again: a second open could see another snapshot.
var ErrViewClosed = errors.New("ledger: view is closed")

// View is one state of the snapshot: one handle and one read-only transaction
// that every query made through it runs on, so that all of them read the file
// that was at the path when the view first touched it, even if the collector
// renames a new one over it meanwhile. A view opened later sees the new file.
//
// The file is opened by the first query, not when the view is made, so a view
// nobody asks anything of never fails on a missing snapshot. Whatever that
// first open returns is what every later query gets: a view answers from one
// snapshot or from none. Close releases the handle; a view is meant to live for
// one unit of work, such as a request, and not beyond it.
type View struct {
	path                string
	observeRowReading   func()
	observeYieldReading func()

	mu       sync.Mutex
	database *sql.DB
	tx       *sql.Tx
	openErr  error
	closed   bool

	// rowsMu is held across the one reading of the clean list, so that a
	// second caller waits for the first one's rows instead of reading again.
	// It is taken before mu.
	rowsMu    sync.Mutex
	cleanRows []Transaction
	rowsRead  bool
}

// View returns a view of the snapshot at the reader's path. It does not touch
// the file.
func (reader *Reader) View() *View {
	return &View{path: reader.path, observeRowReading: reader.observeRowReading,
		observeYieldReading: reader.observeYieldReading}
}

// Close releases the view's handle and drops the rows it read. It may be
// called more than once, and on a view that never opened the file.
func (view *View) Close() error {
	view.rowsMu.Lock()
	defer view.rowsMu.Unlock()
	view.cleanRows, view.rowsRead = nil, false
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.closed {
		return nil
	}
	view.closed = true
	if view.tx == nil {
		return nil
	}
	rollbackErr, closeErr := view.tx.Rollback(), view.database.Close()
	view.tx, view.database = nil, nil
	return errors.Join(rollbackErr, closeErr)
}

// read runs fn on the view's transaction, opening it first if no query has.
func (view *View) read(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := view.transaction(ctx)
	if err != nil {
		return err
	}
	return asUnreadable(fn(tx))
}

func (view *View) transaction(ctx context.Context) (*sql.Tx, error) {
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.closed {
		return nil, ErrViewClosed
	}
	if view.tx == nil && view.openErr == nil {
		view.database, view.tx, view.openErr = openView(ctx, view.path)
	}
	return view.tx, view.openErr
}

// openView begins the one read transaction after the version check. The
// transaction outlives the query that opened it, so it is not tied to that
// query's context: a deadline on the first query must not end the view.
func openView(ctx context.Context, path string) (*sql.DB, *sql.Tx, error) {
	database, err := openReadOnly(path)
	if err != nil {
		return nil, nil, err
	}
	tx, err := database.BeginTx(context.WithoutCancel(ctx), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("ledger: begin read: %w", err)
	}
	if err := requireSupportedVersion(ctx, tx); err != nil {
		tx.Rollback()
		database.Close()
		return nil, nil, asUnreadable(err)
	}
	return database, tx, nil
}
