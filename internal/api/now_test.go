package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/contract"
)

func TestNewHandlerNeedsAClock(t *testing.T) {
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewHandler(Config{Methods: methods, Reads: NewRegistry(), VerifyPIN: func(string) bool { return false }})
	if err == nil {
		t.Error("a handler without a clock was built")
	}
}

func TestNowOfWithoutAClockPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("nowOf answered on a context with no clock")
		}
	}()
	nowOf(context.Background())
}

// A request reads the clock once: two calls of nowOf in one handler see one
// instant even when the clock moves between them.
func TestEveryCallOfNowOfInOneRequestSeesOneInstant(t *testing.T) {
	var seen []time.Time
	twice := func(ctx context.Context, _ []json.RawMessage, _ Views) (any, error) {
		seen = append(seen, nowOf(ctx), nowOf(ctx))
		return nil, nil
	}
	ticks := 0
	moving := func(config *Config) {
		config.Now = func() time.Time {
			ticks++
			return time.Date(2026, 3, 1, 2, 59, 0, 0, time.UTC).Add(time.Duration(ticks) * time.Hour)
		}
	}
	env := newTestEnv(t, withRead("shell.overview", twice), moving)
	env.call("/api/shell/overview", `[]`, env.unlock())
	if len(seen) != 2 || !seen[0].Equal(seen[1]) {
		t.Errorf("one request saw the instants %v", seen)
	}
}
