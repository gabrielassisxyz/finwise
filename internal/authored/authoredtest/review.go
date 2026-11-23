package authoredtest

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// SeedReviewMarks writes into recorder's change one row of each of the review's
// tables: a closing step ticked for 2026-02, a payday transfer marked done and a
// warning dismissed. Nothing writes them yet but a fixture, so the rows go
// straight through the recorder.
func SeedReviewMarks(recorder *authored.Recorder) error {
	ctx := context.Background()
	if err := recorder.Put(ctx, "closing_steps", authored.Row{"month": authored.Text("2026-02"), "step": authored.Text("leftover")}); err != nil {
		return err
	}
	err := recorder.Put(ctx, "transfer_marks", authored.Row{
		"month": authored.Text("2026-03"), "transfer_key": authored.Text("prov:remedios"), "done_on": authored.Text("2026-03-06"),
	})
	if err != nil {
		return err
	}
	return recorder.Put(ctx, "dismissed_warnings", authored.Row{"id": authored.Text("price-rose:streamsvc")})
}
