package authored_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

var errPanicValue = errors.New("the panic's own value")

// panicsWith runs fn and reports what it panicked with, nil if it did not.
func panicsWith(fn func()) (value any) {
	defer func() { value = recover() }()
	fn()
	return nil
}

// afterAPanicTheLockIsFree is what a panic inside a journal transaction must
// leave: the content and the journal as they were before the panicking call,
// and the write lock free, so that the next write gets it at once instead of
// waiting out the busy timeout.
func afterAPanicTheLockIsFree(t *testing.T, database *authored.Database, before string) {
	t.Helper()
	if after := everything(t, database); after != before {
		t.Errorf("the panic left something behind:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	err := database.Change(ctx, facts("test.write"), setFacts("after", authored.Row{"refund": integer(1)}))
	if elapsed := time.Since(started); err != nil || elapsed > 2*time.Second {
		t.Fatalf("the write after the panic: err %v after %v", err, elapsed)
	}
}

func TestAPanicInsideTheFunctionGivenToChangeReleasesTheLock(t *testing.T) {
	database := openScratch(t, "")
	change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	before := everything(t, database)
	value := panicsWith(func() {
		_ = database.Change(t.Context(), facts("test.write"), func(recorder *authored.Recorder) error {
			if err := recorder.Set(t.Context(), "facts", key(text("t1")), authored.Row{"amount": integer(9)}); err != nil {
				return err
			}
			panic(errPanicValue)
		})
	})
	if value != errPanicValue {
		t.Fatalf("the panic reached the caller as %v, want its own value", value)
	}
	afterAPanicTheLockIsFree(t, database, before)
}

func TestAPanicInsideTheFunctionGivenToExtendReleasesTheLock(t *testing.T) {
	database := openScratch(t, "")
	origin := changeBy(t, database, setLine, setFacts("t1", authored.Row{"refund": integer(1)}))
	before := everything(t, database)
	value := panicsWith(func() {
		_ = database.Extend(t.Context(), origin, setLine, func(recorder *authored.Recorder) error {
			if err := recorder.Set(t.Context(), "facts", key(text("t2")), authored.Row{"amount": integer(9)}); err != nil {
				return err
			}
			panic(errPanicValue)
		})
	})
	if value != errPanicValue {
		t.Fatalf("the panic reached the caller as %v, want its own value", value)
	}
	afterAPanicTheLockIsFree(t, database, before)
}

func panickingCheck(armed *bool) authored.RestoreCheck {
	return func(context.Context, *sql.Tx, []authored.Unit) (bool, error) {
		if *armed {
			panic(errPanicValue)
		}
		return true, nil
	}
}

func TestAPanicInsideARegisteredCheckDuringAnUndoReleasesTheLock(t *testing.T) {
	armed := false
	database := openScratch(t, "", panickingCheck(&armed))
	id := change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	before := everything(t, database)
	armed = true
	value := panicsWith(func() { _ = database.Undo(t.Context(), id, undoInstant) })
	armed = false
	if value != errPanicValue {
		t.Fatalf("the panic reached the caller as %v, want its own value", value)
	}
	afterAPanicTheLockIsFree(t, database, before)
	undo(t, database, id)
}

func TestAPanicInsideARegisteredCheckDuringARestorePartReleasesTheLock(t *testing.T) {
	armed := false
	database := openScratch(t, "", panickingCheck(&armed))
	source := change(t, database, setFacts("t1", authored.Row{"refund": integer(1)}))
	before := everything(t, database)
	armed = true
	value := panicsWith(func() {
		_ = database.Change(t.Context(), facts("test.write"), func(recorder *authored.Recorder) error {
			return recorder.RestorePart(t.Context(), source, selectKey("facts", `["t1"]`))
		})
	})
	armed = false
	if value != errPanicValue {
		t.Fatalf("the panic reached the caller as %v, want its own value", value)
	}
	afterAPanicTheLockIsFree(t, database, before)
}
