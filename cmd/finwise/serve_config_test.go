package main

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

// The API is given the process's one clock and the app's database serve opened.
func TestTheAPIGetsTheProcesssClockAndTheAppDatabase(t *testing.T) {
	instant := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	env := environment{now: func() time.Time { return instant }}
	database, err := authored.Open(t.Context(), filepath.Join(t.TempDir(), "finwise.db"), authored.Options{Now: env.now})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	logger := slog.New(slog.DiscardHandler)

	config := apiConfig(serveSettings{ledgerPath: "unused"}, database, env, logger)
	if config.Database != database {
		t.Error("the API was not given the app's database")
	}
	if config.Now == nil || !config.Now().Equal(instant) {
		t.Error("the API was not given the process's clock")
	}
	if without := apiConfig(serveSettings{ledgerPath: "unused"}, nil, env, logger); without.Database != nil {
		t.Error("without --db the API was given a database")
	}
}
