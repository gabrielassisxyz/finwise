package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
)

// What the router does around the take-in: when it calls it, what it keeps of
// a first attempt, and what a write answers when the snapshot it reads cannot
// be taken in.

func TestAWriteOverTheSnapshotTakenInCallsNoTakeIn(t *testing.T) {
	w := pendingPurchaseSaidAbout(t, theRealIngest)
	w.write("transactions.addTag", `["`+bakeryPending+`","padaria"]`)

	if w.takeIns != 0 {
		t.Errorf("four writes over the snapshot taken in called the take-in %d times, want none", w.takeIns)
	}
	if runs := w.runs["transactions.addTag"]; runs != 2 {
		t.Errorf("two tags ran their handler %d times, want once each", runs)
	}
}

// A setting names no transaction: it never reads the ledger, so neither a
// missing snapshot nor one not taken in changes what it answers.
func TestASettingIsSavedWithoutTheTakeInWhateverTheSnapshot(t *testing.T) {
	for name, prepare := range map[string]func(*nightsWorld){
		"the snapshot missing":    func(*nightsWorld) {},
		"a snapshot not taken in": func(w *nightsWorld) { w.night(nightsPurchase(newsstand, onDay(time.March, 12), "BANCA EXEMPLO", 12)) },
	} {
		t.Run(name, func(t *testing.T) {
			w := newNightsWorld(t, theRealIngest)
			prepare(w)
			w.write("settings.setThreshold", `[80]`)
			if w.takeIns != 0 {
				t.Errorf("the setting called the take-in %d times, want none", w.takeIns)
			}
		})
	}
}

func TestAWriteWhoseSnapshotCannotBeTakenInAnswersInternalAndRecordsNothing(t *testing.T) {
	cases := []struct {
		name                 string
		kind                 takeIn
		takeIns, handlerRuns int
	}{
		{"a take-in that does nothing", anIdleTakeIn, 1, 2},
		{"a take-in that fails", aFailingTakeIn, 1, 1},
		{"no take-in", noTakeIn, 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := pendingPurchaseSaidAbout(t, c.kind)
			w.bakeryPostedNight()
			before, runs := w.scratch.Everything(t), w.runs["transactions.addTag"]

			expectError(t, c.name, w.post("transactions.addTag", `["`+bakeryPosted+`","lanche"]`), http.StatusInternalServerError, "internal")

			if handlerRuns := w.runs["transactions.addTag"] - runs; w.takeIns != c.takeIns || handlerRuns != c.handlerRuns {
				t.Errorf("the take-in ran %d times and the handler %d, want %d and %d", w.takeIns, handlerRuns, c.takeIns, c.handlerRuns)
			}
			if after := w.scratch.Everything(t); after != before {
				t.Errorf("the write kept %v", linesAdded(before, after))
			}
		})
	}
}

// Such a snapshot lists nothing, so the write finds no transaction before it
// could compare runs.
func TestAWriteOverASnapshotThatRecordsNoRunRecordsNothingAndTakesNothingIn(t *testing.T) {
	w := pendingPurchaseSaidAbout(t, theRealIngest)
	w.bakeryPostedNight()
	unproject(t, w.ledgerPath)
	before := w.scratch.Everything(t)

	expectRefused(t, "a tag over a snapshot that records no run", w.post("transactions.addTag", `["`+bakeryPosted+`","lanche"]`), string(reasonNotFound))

	if after := w.scratch.Everything(t); after != before || w.takeIns != 0 {
		t.Errorf("after %d take-ins the write kept %v", w.takeIns, linesAdded(before, after))
	}
}

// The collector may replace the snapshot again while the take-in runs; the
// second attempt reads the one taken in, not the one the first attempt read.
func TestTheSecondAttemptReadsTheSnapshotTheTakeInLeft(t *testing.T) {
	w := pendingPurchaseSaidAbout(t, aNewerNightThenTheIngest)
	pendingKey, _ := w.key(bakeryPending)
	w.bakeryPostedNight()

	w.write("transactions.addTag", `["`+bakeryPosted+`","lanche"]`)

	w.requireThePostedRowKeepsWhatWasSaid(pendingKey, 2, "#feira", "#lanche")
	if w.takeIns != 1 || w.nights != 3 {
		t.Errorf("the take-in ran %d times over %d nights, want once, over the third", w.takeIns, w.nights)
	}
}

// attemptRecordingThenTagging is a write handler that records which attempt it
// is in a scratch fact and then tags the posted row of the pending purchase.
func attemptRecordingThenTagging(attempts *int) WriteHandler {
	return func(ctx context.Context, _ []json.RawMessage, writes Writes) (any, error) {
		*attempts++
		if err := writes.Recorder.Set(ctx, authoredtest.Facts, key(text("attempt")), authored.Row{"amount": integer(int64(*attempts))}); err != nil {
			return nil, err
		}
		target, err := targetNamedBy(ctx, writes, bakeryPosted)
		if err != nil {
			return nil, err
		}
		if err := writes.Recorder.AddTag(ctx, target.key, "lanche"); err != nil {
			return nil, err
		}
		return changeResultOf(ctx, writes.Recorder)
	}
}

func TestTheSecondAttemptKeepsNothingOfTheFirst(t *testing.T) {
	var attempts int
	w := pendingPurchaseSaidAbout(t, theRealIngest, replacingWrite(unhandledMethod, attemptRecordingThenTagging(&attempts)))
	w.bakeryPostedNight()
	changes := len(w.scratch.Changes(t))

	id := w.write(unhandledMethod, unhandledArguments)

	w.requireTheRulesChangeBefore(changes, id)
	if attempts != 2 {
		t.Fatalf("the handler ran %d times, want twice", attempts)
	}
	var shown []string
	for _, effect := range w.scratch.EffectsOf(t, changeNumber(t, id)) {
		shown = append(shown, effect.Table+" "+effect.Column+" "+effect.After)
	}
	if len(shown) != 2 || !strings.HasPrefix(shown[0], authoredtest.Facts+" amount 2") || !strings.HasPrefix(shown[1], "tx_tags") {
		t.Errorf("the change holds %q, want the second attempt's fact and its tag alone", shown)
	}
}

// holdTheWriteLock is a write transaction of the test's own on the database at
// path, which a change waits for at its begin; release lets the change go on.
func holdTheWriteLock(t *testing.T, path string) (release func()) {
	t.Helper()
	pool, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	connection, err := pool.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.ExecContext(t.Context(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	released := false
	release = func() {
		if released {
			return
		}
		released = true
		_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		connection.Close()
		pool.Close()
	}
	t.Cleanup(release)
	return release
}

// The write waits for the lock with the first snapshot in place; the second
// one replaces it before the lock is freed. Compared before the change is
// opened, the runs would be equal and the write would go on over the first
// snapshot, which does not list the posted row.
func TestTheRunsAreComparedUnderTheLockTheWriteHolds(t *testing.T) {
	w := pendingPurchaseSaidAbout(t, theRealIngest)
	pendingKey, _ := w.key(bakeryPending)
	second, _ := w.nightBeside(nightsPurchase(bakeryPosted, onDay(time.March, 11), "PADARIA EXEMPLO", 45),
		nightsPurchase(newsstand, onDay(time.March, 12), "BANCA EXEMPLO", 12))
	release := holdTheWriteLock(t, w.scratch.Path)
	inUse := w.scratch.Database.ConnectionsInUse()

	answered := make(chan *httptest.ResponseRecorder, 1)
	go func() { answered <- w.post("transactions.addTag", `["`+bakeryPosted+`","lanche"]`) }()
	for deadline := time.Now().Add(10 * time.Second); w.scratch.Database.ConnectionsInUse() <= inUse; {
		if time.Now().After(deadline) {
			t.Fatal("the write never waited for the lock")
		}
		time.Sleep(time.Millisecond)
	}
	// Whatever the write did before its change, it has done by now.
	time.Sleep(100 * time.Millisecond)
	w.replaceTheSnapshotWith(second)
	release()
	response := <-answered

	if response.Code != http.StatusOK {
		t.Fatalf("the write answered %d %s, want 200", response.Code, response.Body)
	}
	if key, found := w.key(bakeryPosted); !found || key != pendingKey || w.takeIns != 1 {
		t.Errorf("after %d take-ins the posted row is under key %d (indexed %t), want the pending row's %d", w.takeIns, key, found, pendingKey)
	}
}

// indexOf is the two tables of the index in a dump of every table.
func indexOf(everything string) string {
	var kept []string
	keeping := false
	for _, line := range strings.Split(everything, "\n") {
		if table, found := strings.CutPrefix(line, "== "); found {
			keeping = table == "tx_index" || table == "tx_ids"
		}
		if keeping {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func TestAWriteOverTheSnapshotTakenInLeavesTheIndexAsTheIngestLeftIt(t *testing.T) {
	w := pendingPurchaseSaidAbout(t, theRealIngest)
	w.bakeryPostedNight()
	w.ingest(ingest.TriggerTick)
	// The bank lists the pending row again beside the posted one: the ingest
	// keeps the posted row's facts for the key both ids name.
	w.night(nightsPendingPurchase(bakeryPending, onDay(time.March, 10), "PADARIA EXEMPLO", 45),
		nightsPurchase(bakeryPosted, onDay(time.March, 11), "PADARIA EXEMPLO", 45),
		nightsPurchase(newsstand, onDay(time.March, 12), "BANCA EXEMPLO", 12))
	w.ingest(ingest.TriggerTick)
	takenIn := indexOf(w.scratch.Everything(t))

	for _, id := range []string{bakeryPosted, bakeryPending, newsstand} {
		w.write("transactions.setNote", `["`+id+`","nota"]`)
		if index := indexOf(w.scratch.Everything(t)); index != takenIn {
			t.Errorf("a note on %s moved the index:\n%s\nwas\n%s", id, index, takenIn)
		}
	}
	if w.takeIns != 0 {
		t.Errorf("writes over the snapshot taken in called the take-in %d times", w.takeIns)
	}
}
