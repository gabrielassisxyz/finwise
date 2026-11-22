package authored_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const insertPairRow = `INSERT INTO changes (at, day, author, method)
VALUES ('2026-12-01T01:30:00.000Z', '2026-11-30', 'rule', 'pair')`

// fillerRows make the copy take long enough for the writer to commit during it.
const fillerRows = 20000

func fill(t *testing.T, database *authored.Database) {
	t.Helper()
	err := database.Write(t.Context(), func(tx *sql.Tx) error {
		for range fillerRows {
			if _, err := tx.Exec(insertChange); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// pairWriter commits transactions of two rows each through the server's
// handle until it is stopped, counting the commits that returned.
type pairWriter struct {
	committed atomic.Int64
	stop      context.CancelFunc
	done      sync.WaitGroup
	failure   atomic.Value
}

func startPairWriter(t *testing.T, database *authored.Database) *pairWriter {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	writer := &pairWriter{stop: stop}
	writer.done.Add(1)
	go func() {
		defer writer.done.Done()
		for ctx.Err() == nil {
			err := database.Write(ctx, func(tx *sql.Tx) error {
				for range 2 {
					if _, err := tx.Exec(insertPairRow); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil && ctx.Err() == nil {
				writer.failure.Store(err.Error())
				return
			}
			if err == nil {
				writer.committed.Add(1)
			}
		}
	}()
	return writer
}

func (writer *pairWriter) halt(t *testing.T) {
	t.Helper()
	writer.stop()
	writer.done.Wait()
	if failure := writer.failure.Load(); failure != nil {
		t.Fatalf("the writer failed: %v", failure)
	}
}

// The writer is halted once the copy is published, so the record of the
// instant does not have to win the lock from a loop that never pauses.
func TestACopyTakenWhileAnotherConnectionCommitsHoldsWholeTransactions(t *testing.T) {
	source := databasePath(t)
	database := openDatabase(t, source, creationInstant, nil)
	fill(t, database)
	writer := startPairWriter(t, database)
	defer writer.halt(t)
	for writer.committed.Load() < 5 {
		if failure := writer.failure.Load(); failure != nil {
			t.Fatalf("the writer failed: %v", failure)
		}
	}
	before := writer.committed.Load()
	var atCopy int64
	copied := func(string) { atCopy = writer.committed.Load() }
	destination := filepath.Join(t.TempDir(), "finwise-backup.db")

	if err := backupWithHooks(t.Context(), t, source, destination, copied, nil, func() { writer.halt(t) }); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if atCopy <= before {
		t.Fatalf("no transaction was committed while the copy was taken (%d before, %d after)", before, atCopy)
	}
	if got := headerID(t, destination); got != ourID {
		t.Errorf("the copy's application id is %s", got)
	}
	if got := integrityOf(t, destination); got != "ok" {
		t.Errorf("integrity_check answered %q", got)
	}
	counted := columnValues(t, inspect(t, destination), "SELECT count(*) FROM changes WHERE method = 'pair'")
	pairs, err := strconv.ParseInt(counted[0], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if pairs%2 != 0 || pairs < 2*before || pairs > 2*atCopy {
		t.Errorf("the copy holds %d pair rows; want an even number from %d to %d", pairs, 2*before, 2*atCopy)
	}
	t.Logf("%d transactions before the copy, %d when it was written, %d rows in it", before, atCopy, pairs)
}
