package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/suggest"
)

// What serve relies on to close the database after the suggester: when stop
// returns, the step it cancelled has already returned. The step here winds down
// only when the test lets it, so the order of the two returns is the test's to
// observe: "step returned" is recorded before the step returns and "stop
// returned" right after stop does.
//
// The step is let go as soon as stop is seen to return, which is what a stop
// that does not wait does at once, and otherwise once stop has had a quarter of
// a second to return and has not, which is what a stop that waits never does.
// Either way the order recorded decides: the wait only gives a stop that does
// not wait the chance to show it, and cannot make one that waits fail.
func TestStopReturnsOnlyAfterTheStepInFlightHasReturned(t *testing.T) {
	var (
		mu    sync.Mutex
		order []string
	)
	note := func(event string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, event)
	}
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	step := func(ctx context.Context, _ suggest.Sources, _ suggest.Config, _ func() time.Time) (suggest.Counts, error) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-release
		note("step returned")
		return suggest.Counts{}, ctx.Err()
	}
	settings := serveSettings{
		ledgerPath: filepath.Join(t.TempDir(), "absent-snapshot.db"),
		model:      modelSettings{configured: true, url: "http://127.0.0.1:1", name: "model-x"},
	}
	env := environment{runSuggest: step, now: func() time.Time { return testNow }}
	suggester := newServerSuggester(context.Background(), settings, nil, env, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		suggester.stop()
	})

	suggester.trigger()
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("the step never started")
	}

	stopped := make(chan struct{})
	go func() {
		suggester.stop()
		note("stop returned")
		close(stopped)
	}()
	select {
	case <-cancelled:
	case <-time.After(30 * time.Second):
		t.Fatal("stop did not cancel the step")
	}
	select {
	case <-stopped:
	case <-time.After(250 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(30 * time.Second):
		t.Fatal("stop did not return once the step had")
	}

	mu.Lock()
	defer mu.Unlock()
	if want := []string{"step returned", "stop returned"}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v: stop returned while the step was still in flight", order, want)
	}
}
