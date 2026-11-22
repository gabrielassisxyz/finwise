package authoredtest

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// SetPotBucket records the bucket the person chose for a pot, in one change made
// at at, and returns the change's id. The database is one made by Fresh or Open.
func SetPotBucket(t testing.TB, database *authored.Database, at time.Time, potID string, bucket authored.PotBucket) authored.ChangeID {
	t.Helper()
	return recordBudgetChange(t, database, at, "authoredtest.potBucket", func(recorder *authored.Recorder) error {
		return recorder.SetPotBucket(t.Context(), potID, bucket)
	})
}

// ToggleLink toggles the link of a budget line to a pot as the person would, in
// one change made at at, and returns the change's id: it links a line that is
// linked to no pot, unlinks one that is linked to this pot and moves one that
// is linked to another. The line must exist, as a saved budget makes it.
func ToggleLink(t testing.TB, database *authored.Database, at time.Time, line authored.LineID, potID string) authored.ChangeID {
	t.Helper()
	return recordBudgetChange(t, database, at, "authoredtest.potLink", func(recorder *authored.Recorder) error {
		return recorder.ToggleLink(t.Context(), line, potID)
	})
}
