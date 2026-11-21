package api

import (
	"sync"
	"time"
)

// fakeClock is the injected clock: tests move it, nothing sleeps.
type fakeClock struct {
	mutex   sync.Mutex
	current time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{current: time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)}
}

func (clock *fakeClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.current
}

func (clock *fakeClock) Advance(by time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.current = clock.current.Add(by)
}
