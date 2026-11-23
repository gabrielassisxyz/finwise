package api

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// How often one request reads the ledger's transaction rows from the snapshot
// file: once, however many parts of its answer need them, and never from a
// reading another request made.

// countingRowReadings makes the router's ledger reader count every reading of
// the rows from the file. It goes after the option that sets the ledger.
func countingRowReadings(readings *atomic.Int64) envOption {
	return func(config *Config) {
		config.Ledger = config.Ledger.ObservingRowReadings(func() { readings.Add(1) })
	}
}

// readsOfTheTimingTable are the reads whose time was measured, among them the
// ones that read the rows more than once each before they shared a reading.
var readsOfTheTimingTable = []struct{ method, arguments string }{
	{"home.summary", `["2026-03"]`},
	{"review.closing", `["2026-03"]`},
	{"review.weekly", `["2026-03"]`},
	{"settings.overview", `[]`},
	{"budget.view", `["2026-03"]`},
	{"shell.overview", `[]`},
	{"transactions.detail", `["tx-pix-out"]`},
	{"transactions.detail", `["tx-refund"]`},
	{"review.payday", `["2026-03"]`},
	{"transactions.list", `[{"month":"2026-03","period":"month"}]`},
	{"card.overview", `[]`},
	{"savingsPots.list", `[{"includeClosed":true}]`},
	{"netWorth.overview", `[]`},
}

// queueingThirdState is the third state with reviews waiting, so that every
// part of an answer that follows the queue reads the ledger.
func queueingThirdState(t *testing.T, options ...envOption) *readEnv {
	t.Helper()
	scratch, full := newAuthoredStores(t)
	queueOverTheThirdState(t, scratch, full)
	all := append([]envOption{withLedger(full.Path, fixtureNow), withDatabase(scratch.Database)}, options...)
	env := newTestEnv(t, all...)
	return &readEnv{testEnv: env, cookie: env.unlock()}
}

func TestEachRequestReadsTheLedgersRowsOnce(t *testing.T) {
	var readings atomic.Int64
	env := queueingThirdState(t, countingRowReadings(&readings))
	requireAReviewWaits(t, env)

	for _, c := range readsOfTheTimingTable {
		for request := 1; request <= 2; request++ {
			readings.Store(0)
			if status, body := env.answer(c.method, c.arguments); status != http.StatusOK {
				t.Fatalf("%s %s answered %d %s", c.method, c.arguments, status, body)
			}
			if got := readings.Load(); got != 1 {
				t.Errorf("request %d of %s %s read the rows %d times, want once", request, c.method, c.arguments, got)
			}
		}
	}
}

func requireAReviewWaits(t *testing.T, env *readEnv) {
	t.Helper()
	status, body := env.answer("shell.overview", `[]`)
	var frame struct {
		PendingCount int `json:"pendingCount"`
	}
	if status != http.StatusOK || json.Unmarshal(body, &frame) != nil || frame.PendingCount == 0 {
		t.Fatalf("shell.overview answered %d %s, want reviews waiting", status, body)
	}
}

func TestEachWriteThatLooksATransactionUpReadsTheRowsOnce(t *testing.T) {
	var readings atomic.Int64
	w := pendingPurchaseSaidAbout(t, theRealIngest, countingRowReadings(&readings))
	w.bakeryPostedNight()
	w.ingest(ingest.TriggerTick)

	for _, c := range []struct{ method, arguments string }{
		{"transactions.addTag", `["` + bakeryPosted + `","lanche"]`},
		{"transactions.setLine", `["` + newsstand + `","Mercado"]`},
		{"transactions.addTagToMany", `[["` + bakeryPosted + `","` + newsstand + `"],"mercado"]`},
		{"transactions.setNote", `["` + newsstand + `","revista"]`},
	} {
		readings.Store(0)
		w.write(c.method, c.arguments)
		if got := readings.Load(); got != 1 {
			t.Errorf("%s read the rows %d times, want once", c.method, got)
		}
	}
}

// The collector renames a new snapshot over the ledger between two requests:
// the second one reads it, and nothing of the first request's reading.
func TestTheRequestAfterTheSnapshotWasReplacedAnswersFromTheNewOne(t *testing.T) {
	var readings atomic.Int64
	w := newNightsWorld(t, theRealIngest, countingRowReadings(&readings))
	w.night(nightsPurchase(bakeryPosted, onDay(time.March, 11), "PADARIA EXEMPLO", 45))
	before := w.listedIn("2026-03")

	w.night(nightsPurchase(bakeryPosted, onDay(time.March, 11), "PADARIA EXEMPLO", 45),
		nightsPurchase(bookshop, onDay(time.March, 13), "LIVRARIA EXEMPLO", 30))
	after := w.listedIn("2026-03")

	if _, listed := before[bookshop]; listed || len(before) != 1 {
		t.Errorf("the first snapshot listed %v, want the bakery alone", slices.Sorted(maps.Keys(before)))
	}
	if _, listed := after[bookshop]; !listed || len(after) != 2 {
		t.Errorf("the request after the rename listed %v, want the bakery and the bookshop", slices.Sorted(maps.Keys(after)))
	}
	if got := readings.Load(); got != 2 {
		t.Errorf("two requests read the rows %d times, want once each", got)
	}
}

// rowsCountingThenTagging is a write handler that notes how many rows its
// attempt read and then tags the posted row of the pending purchase, whose
// lookup sends the first attempt back for a take-in.
func rowsCountingThenTagging(seen *[]int) WriteHandler {
	return func(ctx context.Context, _ []json.RawMessage, writes Writes) (any, error) {
		rows, err := writes.Ledger.CleanTransactions(ctx, wholeLedger)
		if err != nil {
			return nil, err
		}
		*seen = append(*seen, len(rows))
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

// The night the take-in writes lists one row more than the night the first
// attempt read, so what the second attempt read says which file it was.
func TestTheSecondAttemptOfAWriteReadsTheRowsOfTheSnapshotTheTakeInLeft(t *testing.T) {
	var readings atomic.Int64
	var seen []int
	w := pendingPurchaseSaidAbout(t, aNewerNightThenTheIngest, countingRowReadings(&readings),
		replacingWrite(unhandledMethod, rowsCountingThenTagging(&seen)))
	w.bakeryPostedNight()
	w.lastRows = append(slices.Clone(w.lastRows), nightsPurchase(bookshop, onDay(time.March, 13), "LIVRARIA EXEMPLO", 30))
	readings.Store(0)

	w.write(unhandledMethod, unhandledArguments)

	if !slices.Equal(seen, []int{2, 3}) || w.takeIns != 1 {
		t.Errorf("after %d take-ins the attempts read %v rows, want 2 then the 3 of the night the take-in left", w.takeIns, seen)
	}
	if got := readings.Load(); got != 2 {
		t.Errorf("the two attempts read the rows %d times, want once each", got)
	}
}

// One part of an answer reads the rows first and changes all of them, through
// every pointer too; the answer the request gives is still the one it gives
// when nothing changed them.
func TestWhatOnePartOfAnAnswerDoesToItsRowsReachesNoOtherPart(t *testing.T) {
	scratch, full := newAuthoredStores(t)
	queueOverTheThirdState(t, scratch, full)
	reader, registry := ledger.New(full.Path), NewReadRegistry()
	ctx := withNow(t.Context(), func() time.Time { return fixtureNow })

	for _, c := range readsOfTheTimingTable {
		var arguments []json.RawMessage
		if err := json.Unmarshal([]byte(c.arguments), &arguments); err != nil {
			t.Fatal(err)
		}
		registered, found := registry.lookup(c.method)
		if !found || registered.read == nil {
			t.Fatalf("%s has no read handler", c.method)
		}
		answer := func(changeTheRowsFirst bool) string {
			views := openViews(reader, scratch.Database)
			defer views.close()
			if changeTheRowsFirst {
				changeEveryRow(t, ctx, views.Ledger)
			}
			result, err := registered.read(ctx, arguments, views)
			if err != nil {
				t.Fatalf("%s %s: %v", c.method, c.arguments, err)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			return string(encoded)
		}
		if changed, unchanged := answer(true), answer(false); changed != unchanged {
			t.Errorf("%s %s answered\n%s\nafter a part changed its rows, want\n%s", c.method, c.arguments, changed, unchanged)
		}
	}
}

func changeEveryRow(t *testing.T, ctx context.Context, snapshot *ledger.View) {
	t.Helper()
	rows, err := snapshot.CleanTransactions(ctx, wholeLedger)
	if err != nil || len(rows) == 0 {
		t.Fatalf("the rows: %d, %v", len(rows), err)
	}
	for index := range rows {
		changeEveryField(reflect.ValueOf(&rows[index]).Elem())
	}
	slices.Reverse(rows)
}

var aTime = reflect.TypeFor[time.Time]()

// changeEveryField changes every value reachable from value, through pointers.
func changeEveryField(value reflect.Value) {
	switch {
	case value.Type() == aTime:
		value.Set(reflect.ValueOf(value.Interface().(time.Time).AddDate(0, 1, 0)))
	case value.Kind() == reflect.String:
		value.SetString(value.String() + "-changed")
	case value.CanInt():
		value.SetInt(value.Int() + 1)
	case value.Kind() == reflect.Bool:
		value.SetBool(!value.Bool())
	case value.Kind() == reflect.Pointer:
		if !value.IsNil() {
			changeEveryField(value.Elem())
		}
	case value.Kind() == reflect.Struct:
		for index := range value.NumField() {
			changeEveryField(value.Field(index))
		}
	}
}
