package authored_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const moveTrackingStart = `UPDATE meta SET value = '2026-07' WHERE key = 'tracking_start_month'`

func viewMonth(t *testing.T, view *authored.View) string {
	t.Helper()
	month, found, err := view.TrackingStartMonth(t.Context())
	if err != nil || !found {
		t.Fatalf("tracking start month: %q found %v err %v", month, found, err)
	}
	return string(month)
}

func TestAViewKeepsTheStateOfItsFirstReadWhileAnotherConnectionCommits(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	view := database.View()
	defer view.Close()
	before := viewMonth(t, view)

	err := database.Write(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(moveTrackingStart)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if again := viewMonth(t, view); again != before {
		t.Errorf("the second read of one view saw %s, the first %s", again, before)
	}
	later := database.View()
	defer later.Close()
	if after := viewMonth(t, later); after != "2026-07" {
		t.Errorf("a view made after the commit saw %s, want 2026-07", after)
	}
}

func TestTheEmptyViewAnswersNothingAuthored(t *testing.T) {
	view := authored.EmptyView()
	month, found, err := view.TrackingStartMonth(t.Context())
	if err != nil || found || month != "" {
		t.Errorf("the empty view answered %q found %v err %v", month, found, err)
	}
	if err := view.Close(); err != nil {
		t.Errorf("closing the empty view: %v", err)
	}
}

func TestClosingAViewHandsItsConnectionBackAndEndsIt(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	idle := database.View()
	if inUse := database.ConnectionsInUse(); inUse != 0 {
		t.Errorf("a view that never read holds %d connections", inUse)
	}
	if err := idle.Close(); err != nil {
		t.Errorf("closing a view that never read: %v", err)
	}

	view := database.View()
	viewMonth(t, view)
	if inUse := database.ConnectionsInUse(); inUse != 1 {
		t.Errorf("an open view holds %d connections, want 1", inUse)
	}
	if err := view.Close(); err != nil {
		t.Fatal(err)
	}
	if inUse := database.ConnectionsInUse(); inUse != 0 {
		t.Errorf("a closed view still holds %d connections", inUse)
	}
	if err := view.Close(); err != nil {
		t.Errorf("a second close: %v", err)
	}
	if _, _, err := view.TrackingStartMonth(t.Context()); !errors.Is(err, authored.ErrViewClosed) {
		t.Errorf("a read on a closed view: %v, want ErrViewClosed", err)
	}
}
