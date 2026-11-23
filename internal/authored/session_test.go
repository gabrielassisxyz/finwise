package authored_test

import (
	"context"
	"strings"
	"testing"
	"time"
)

const (
	hashOne = "pbkdf2-sha256$600000$c2FsdHNhbHRzYWx0$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5"
	hashTwo = "pbkdf2-sha256$600000$b3RoZXJzYWx0$b3RoZXJrZXlvdGhlcmtleW90aGVya2V5"
)

func TestTheSessionMigrationIsRecorded(t *testing.T) {
	path := databasePath(t)
	openAndClose(t, path, creationInstant, nil)
	if got := namesOf(t, path); !equalStrings(got, []string{"0001_journal", "0002_budget", "0003_transactions", "0004_settings", "0005_pots", "0006_subscriptions", "0008_session"}) {
		t.Errorf("recorded = %v, want 0001_journal, 0002_budget, 0003_transactions, 0004_settings, 0005_pots, 0006_subscriptions and 0008_session", got)
	}
}

func TestThePinTableRefusesWhatIsNotOneRowWithBothValues(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	pool := database.Pool()
	if _, err := pool.ExecContext(t.Context(), "INSERT INTO pin (id, hash, changed_at) VALUES (1, 'h', 't')"); err != nil {
		t.Fatalf("a valid row was refused: %v", err)
	}
	refused := map[string]string{
		"id 2":           "INSERT INTO pin (id, hash, changed_at) VALUES (2, 'h', 't')",
		"a second id 1":  "INSERT INTO pin (id, hash, changed_at) VALUES (1, 'h', 't')",
		"a null hash":    "UPDATE pin SET hash = NULL",
		"a null instant": "UPDATE pin SET changed_at = NULL",
		"a changed id":   "UPDATE pin SET id = 2",
	}
	for name, statement := range refused {
		if _, err := pool.ExecContext(t.Context(), statement); err == nil {
			t.Errorf("the database accepted %s", name)
		}
	}
}

func TestNoPINIsStoredUntilOneIsWritten(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	hash, found, err := database.StoredPINHash(t.Context())
	if err != nil || found || hash != "" {
		t.Errorf("a fresh database answered %q %v %v, want nothing", hash, found, err)
	}
}

func TestStoringAPINHashReplacesTheOneRowAndWritesNoJournal(t *testing.T) {
	path := databasePath(t)
	database := openDatabase(t, path, creationInstant, nil)
	first := time.Date(2026, 12, 1, 10, 0, 0, 123_000_000, time.UTC)
	second := first.Add(time.Hour)
	for _, step := range []struct {
		hash string
		at   time.Time
	}{{hashOne, first}, {hashTwo, second}} {
		if err := database.StorePINHash(t.Context(), step.hash, step.at); err != nil {
			t.Fatal(err)
		}
		got, found, err := database.StoredPINHash(t.Context())
		if err != nil || !found || got != step.hash {
			t.Fatalf("stored %v, found %v, err %v", got == step.hash, found, err)
		}
	}
	pool := database.Pool()
	var rows int
	var changedAt string
	if err := pool.QueryRowContext(t.Context(), "SELECT count(*), max(changed_at) FROM pin").Scan(&rows, &changedAt); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || changedAt != "2026-12-01T11:00:00.123Z" {
		t.Errorf("pin holds %d rows, changed_at %s; want one row at 2026-12-01T11:00:00.123Z", rows, changedAt)
	}
	for _, table := range []string{"changes", "change_effects"} {
		var count int
		if err := pool.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Errorf("%s holds %d rows (%v), want none", table, count, err)
		}
	}
}

func TestAStoredPINHashSurvivesReopeningTheFile(t *testing.T) {
	path := databasePath(t)
	database := openDatabase(t, path, creationInstant, nil)
	if err := database.StorePINHash(t.Context(), hashOne, time.Now()); err != nil {
		t.Fatal(err)
	}
	database.Close()
	reopened := openDatabase(t, path, laterInstant, nil)
	if got, found, err := reopened.StoredPINHash(t.Context()); err != nil || !found || got != hashOne {
		t.Errorf("after reopening: found %v, err %v", found, err)
	}
}

func TestAFailedStoreLeavesTheOldHash(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	if err := database.StorePINHash(t.Context(), hashOne, time.Now()); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := database.StorePINHash(cancelled, hashTwo, time.Now()); err == nil || strings.Contains(err.Error(), hashTwo) {
		t.Errorf("a store under a cancelled context: %v", err)
	}
	if got, _, _ := database.StoredPINHash(t.Context()); got != hashOne {
		t.Error("a failed store changed the hash")
	}
}
