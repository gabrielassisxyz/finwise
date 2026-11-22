package authored_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const insertChange = `INSERT INTO changes (at, day, author, method)
VALUES ('2026-12-01T01:30:00.000Z', '2026-11-30', 'person', 'budget.setLine')`

// twoHandles is two opens of one file, as two parts of a process or two
// processes would hold it.
func twoHandles(t *testing.T) (*authored.Database, *authored.Database) {
	t.Helper()
	path := databasePath(t)
	return openDatabase(t, path, creationInstant, nil), openDatabase(t, path, creationInstant, nil)
}

func countChanges(t testing.TB, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) int {
	t.Helper()
	var count int
	if err := query.QueryRowContext(t.Context(), "SELECT count(*) FROM changes").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestEveryPooledConnectionOfEveryHandleHasTheSettings(t *testing.T) {
	first, second := twoHandles(t)
	want := map[string]string{"foreign_keys": "1", "busy_timeout": "5000", "synchronous": "2", "journal_mode": "wal"}
	for index, database := range []*authored.Database{first, second} {
		var held []*sql.Conn
		for range 3 {
			connection, err := database.Pool().Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			held = append(held, connection)
		}
		for number, connection := range held {
			for pragma, expected := range want {
				var got string
				if err := connection.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if got != expected {
					t.Errorf("handle %d, connection %d: %s = %s, want %s", index, number, pragma, got, expected)
				}
			}
		}
		for _, connection := range held {
			connection.Close()
		}
	}
}

func TestADanglingEffectIsRefusedThroughEitherHandle(t *testing.T) {
	first, second := twoHandles(t)
	for index, database := range []*authored.Database{first, second} {
		err := database.Write(t.Context(), func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO change_effects (change_id, seq, tbl, key, after) VALUES (999, 1, 'lines', '["a"]', '1')`)
			return err
		})
		if err == nil {
			t.Errorf("handle %d accepted an effect of a change that does not exist", index)
		}
	}
}

func TestASecondWriterWaitsAtItsBeginUntilTheFirstCommits(t *testing.T) {
	first, second := twoHandles(t)
	writing, err := first.BeginWrite(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writing.Exec(insertChange); err != nil {
		t.Fatal(err)
	}

	begun := make(chan error, 1)
	var waiting *sql.Tx
	go func() {
		tx, err := second.BeginWrite(t.Context())
		waiting = tx
		begun <- err
	}()
	select {
	case err := <-begun:
		t.Fatalf("the second write began while the first was open (err %v)", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := writing.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-begun:
		if err != nil {
			t.Fatalf("the second write failed instead of waiting: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("the second write did not begin after the first committed")
	}
	if got := countChanges(t, waiting); got != 1 {
		t.Errorf("the second writer sees %d changes, want the first's commit", got)
	}
	waiting.Rollback()
}

func TestAReaderDoesNotWaitForAWriterAndSeesTheStateBeforeIt(t *testing.T) {
	first, second := twoHandles(t)
	writing, err := first.BeginWrite(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer writing.Rollback()
	if _, err := writing.Exec(insertChange); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	reading, err := second.BeginRead(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer reading.Rollback()
	if got := countChanges(t, reading); got != 0 {
		t.Errorf("the reader sees %d changes from a write not yet committed", got)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("the reader waited %v", elapsed)
	}
}

func TestAWriteThatFailsHalfwayKeepsNothing(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	failure := errors.New("the second step failed")
	err := database.Write(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(insertChange); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v", err)
	}
	if got := countChanges(t, database.Pool()); got != 0 {
		t.Errorf("%d changes were kept", got)
	}
}

// Waits out busy_timeout, five seconds, on purpose.
func TestAWriteThatWaitsOutTheLockFailsAndKeepsNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("waits five seconds")
	}
	first, second := twoHandles(t)
	writing, err := first.BeginWrite(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer writing.Rollback()

	ran := false
	err = second.Write(t.Context(), func(tx *sql.Tx) error {
		ran = true
		_, err := tx.Exec(insertChange)
		return err
	})
	var transactionErr *authored.TransactionError
	if !errors.As(err, &transactionErr) || ran {
		t.Fatalf("err = %v, ran = %v: want a failed begin and nothing run", err, ran)
	}
	writing.Rollback()
	if got := countChanges(t, second.Pool()); got != 0 {
		t.Errorf("%d changes were kept", got)
	}
}
