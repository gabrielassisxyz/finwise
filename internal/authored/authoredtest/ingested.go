package authoredtest

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// TakeRunIn records run as the collector run whose snapshot the database took
// in last, as a run of the ingest records it, and writes nothing else: no row
// of the index and no change, since recording a run is no effect. It is for a
// test whose database stands for one the ingest keeps current over an invented
// ledger it never runs the ingest on, so that a write's lookup finds that
// snapshot taken in. A run earlier than the one recorded moves nothing.
func TakeRunIn(t testing.TB, database *authored.Database, run int64) {
	t.Helper()
	// The change writes no row, so its instant is never kept.
	facts := authored.ChangeFacts{Author: authored.AuthorRule, Method: "ingest.run", At: time.Time{}}
	err := database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		return recorder.RecordIngestedRun(t.Context(), run)
	})
	if err != nil {
		t.Fatalf("authoredtest: take run %d in: %v", run, err)
	}
}
