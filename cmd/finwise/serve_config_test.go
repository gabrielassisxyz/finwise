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

// The calibration counts the suggestions of the model configured now, so the
// API is given its name as FINWISE_MODEL_NAME set it, and none without a model.
func TestTheAPIGetsTheNameOfTheConfiguredModel(t *testing.T) {
	env := environment{now: time.Now}
	logger := slog.New(slog.DiscardHandler)
	settings, err := readModelSettings(func(name string) string {
		return map[string]string{"FINWISE_MODEL_URL": "http://127.0.0.1:9", "FINWISE_MODEL_NAME": "modelo-teste"}[name]
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := apiConfig(serveSettings{ledgerPath: "unused", model: settings}, nil, env, logger).ModelName; got != "modelo-teste" {
		t.Errorf("the API was given the model name %q, want modelo-teste", got)
	}
	if got := apiConfig(serveSettings{ledgerPath: "unused"}, nil, env, logger).ModelName; got != "" {
		t.Errorf("with no model configured the API was given the name %q", got)
	}
}
