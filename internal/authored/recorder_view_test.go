package authored_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// kindOfChange runs a body inside each way a recorder is made: the function
// given to Change and the one given to Extend, which must behave alike.
type kindOfChange struct {
	name string
	run  func(t *testing.T, database *authored.Database, write func(*authored.Recorder) error) error
}

func kindsOfChange() []kindOfChange {
	opened := 0
	return []kindOfChange{
		{"change", func(t *testing.T, database *authored.Database, write func(*authored.Recorder) error) error {
			return database.Change(t.Context(), facts("test.write"), write)
		}},
		{"extension", func(t *testing.T, database *authored.Database, write func(*authored.Recorder) error) error {
			opened++
			origin := changeBy(t, database, setLine, setFacts(fmt.Sprintf("origin-%d", opened), authored.Row{"refund": integer(1)}))
			return database.Extend(t.Context(), origin, setLine, write)
		}},
	}
}

func factRow(t *testing.T, view *authored.View, id string) (authored.Row, bool) {
	t.Helper()
	row, found, err := view.JournaledRow(t.Context(), "facts", key(text(id)))
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	return row, found
}

func TestTheRecordersViewSeesTheChangesOwnWritesAndAViewOpenedBeforeDoesNot(t *testing.T) {
	for _, kind := range kindsOfChange() {
		t.Run(kind.name, func(t *testing.T) {
			database := openScratch(t, "")
			earlier := database.View()
			defer earlier.Close()
			if _, found := factRow(t, earlier, "t1"); found {
				t.Fatal("the fact exists before the change")
			}
			err := kind.run(t, database, func(recorder *authored.Recorder) error {
				if err := recorder.Set(t.Context(), "facts", key(text("t1")), authored.Row{"amount": integer(700)}); err != nil {
					return err
				}
				row, found := factRow(t, recorder.View(), "t1")
				if !found || row["amount"] != integer(700) {
					t.Errorf("the change's view read %v found %v after the write", row, found)
				}
				if _, found := factRow(t, earlier, "t1"); found {
					t.Error("a view opened before the change saw its write")
				}
				return errors.New("rolled back on purpose")
			})
			if err == nil {
				t.Fatal("the change was not refused")
			}
			if _, found := factRow(t, earlier, "t1"); found {
				t.Error("the view opened before saw a rolled back write")
			}
			later := database.View()
			defer later.Close()
			if _, found := factRow(t, later, "t1"); found {
				t.Error("a view opened after saw a rolled back write")
			}
		})
	}
}

func TestClosingTheRecordersViewLeavesTheChangeOpen(t *testing.T) {
	for _, kind := range kindsOfChange() {
		t.Run(kind.name, func(t *testing.T) {
			database := openScratch(t, "")
			var closed *authored.View
			err := kind.run(t, database, func(recorder *authored.Recorder) error {
				closed = recorder.View()
				if err := closed.Close(); err != nil {
					t.Errorf("close: %v", err)
				}
				if _, _, err := closed.JournaledRow(t.Context(), "facts", key(text("t1"))); !errors.Is(err, authored.ErrViewClosed) {
					t.Errorf("a read through the closed view: %v, want ErrViewClosed", err)
				}
				if again, found := factRow(t, recorder.View(), "t1"); found {
					t.Errorf("a second view of the change read %v", again)
				}
				return recorder.Set(t.Context(), "facts", key(text("t1")), authored.Row{"amount": integer(5)})
			})
			if err != nil {
				t.Fatalf("a write after the view's close: %v", err)
			}
			view := database.View()
			defer view.Close()
			if row, found := factRow(t, view, "t1"); !found || row["amount"] != integer(5) {
				t.Errorf("the write after the close was not committed: %v found %v", row, found)
			}
		})
	}
}

func TestTheRecordersViewRefusesAfterTheChangeEndedAndBeginsNothing(t *testing.T) {
	for _, kind := range kindsOfChange() {
		t.Run(kind.name, func(t *testing.T) {
			database := openScratch(t, "")
			var kept *authored.View
			err := kind.run(t, database, func(recorder *authored.Recorder) error {
				kept = recorder.View()
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			inUse := database.ConnectionsInUse()
			if _, _, err := kept.JournaledRow(t.Context(), "facts", key(text("t1"))); !errors.Is(err, authored.ErrViewClosed) {
				t.Errorf("a read after the change ended: %v, want ErrViewClosed", err)
			}
			if _, _, err := kept.LastBackup(t.Context()); !errors.Is(err, authored.ErrViewClosed) {
				t.Errorf("LastBackup after the change ended: %v, want ErrViewClosed", err)
			}
			if got := database.ConnectionsInUse(); got != inUse {
				t.Errorf("connections in use went from %d to %d", inUse, got)
			}
		})
	}
}

func TestTheRecordersViewAnswersTheFailureOfARecordedWriteAndBeginsNothing(t *testing.T) {
	for _, kind := range kindsOfChange() {
		t.Run(kind.name, func(t *testing.T) {
			database := openScratch(t, "")
			change(t, database, createThing(thing(1, "rent", 500, null)))
			_ = kind.run(t, database, func(recorder *authored.Recorder) error {
				failure := recorder.Create(t.Context(), "things", thing(1, "rent", 500, null))
				if failure == nil {
					t.Fatal("creating a thing twice succeeded")
				}
				inUse := database.ConnectionsInUse()
				_, _, err := recorder.View().JournaledRow(t.Context(), "things", key(integer(1)))
				if err == nil || err != failure {
					t.Errorf("a read after the failed write: %v, want the failure %v", err, failure)
				}
				if got := database.ConnectionsInUse(); got != inUse {
					t.Errorf("connections in use went from %d to %d", inUse, got)
				}
				return nil
			})
		})
	}
}
