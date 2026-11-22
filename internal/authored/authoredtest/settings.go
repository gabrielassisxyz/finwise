package authoredtest

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// Settings names the settings an invented authored state sets; a nil field is
// left unset, so the state answers its default.
type Settings struct {
	ConfidenceThreshold *int64
	MinExamples         *int64
	CardLabel           *string
	ClosingMode         *authored.ClosingMode
	ManualClosingDay    *int64
}

// Some points at a copy of value, for a field of Settings.
func Some[T any](value T) *T { return &value }

// WithSettings records the given settings in one change made by the person at
// at, and returns the change's id. The database is one made by Fresh: a
// Scratch database journals only scratch tables.
func WithSettings(t testing.TB, database *authored.Database, at time.Time, settings Settings) authored.ChangeID {
	t.Helper()
	var id authored.ChangeID
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: "authoredtest.settings", At: at}
	err := database.Change(t.Context(), facts, func(recorder *authored.Recorder) error {
		ctx := t.Context()
		var err error
		if id, err = recorder.ID(ctx); err != nil {
			return err
		}
		if settings.ConfidenceThreshold != nil {
			if err := recorder.SetConfidenceThreshold(ctx, *settings.ConfidenceThreshold); err != nil {
				return err
			}
		}
		if settings.MinExamples != nil {
			if err := recorder.SetMinExamples(ctx, *settings.MinExamples); err != nil {
				return err
			}
		}
		if settings.CardLabel != nil {
			if err := recorder.SetCardLabel(ctx, *settings.CardLabel); err != nil {
				return err
			}
		}
		if settings.ClosingMode != nil {
			if err := recorder.SetClosingMode(ctx, *settings.ClosingMode); err != nil {
				return err
			}
		}
		if settings.ManualClosingDay != nil {
			return recorder.SetManualClosingDay(ctx, *settings.ManualClosingDay)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("authoredtest: settings: %v", err)
	}
	return id
}
