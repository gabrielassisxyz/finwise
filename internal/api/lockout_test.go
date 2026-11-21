package api

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func attemptWith(lockout *Lockout, correct bool) (UnlockOutcome, int) {
	return lockout.Attempt(func() bool { return correct })
}

func tripLockout(t *testing.T, lockout *Lockout) {
	t.Helper()
	for range maxWrongPINs - 1 {
		if outcome, _ := attemptWith(lockout, false); outcome != OutcomeWrongPIN {
			t.Fatalf("a wrong PIN before the limit gave outcome %d", outcome)
		}
	}
	outcome, wait := attemptWith(lockout, false)
	if outcome != OutcomeHeld || wait != 300 {
		t.Fatalf("the third wrong PIN gave outcome %d, wait %d; want held for 300", outcome, wait)
	}
}

func TestThreeWrongPINsHoldEvenTheRightOne(t *testing.T) {
	clock := newFakeClock()
	lockout := newLockout(clock.Now)
	tripLockout(t, lockout)

	clock.Advance(100 * time.Second)
	outcome, wait := attemptWith(lockout, true)
	if outcome != OutcomeHeld || wait < 1 || wait > 300 {
		t.Fatalf("the right PIN during the hold gave outcome %d, wait %d", outcome, wait)
	}
	if wait != 200 {
		t.Errorf("wait after 100s = %d, want 200", wait)
	}
}

func TestTheHoldEndsAfterThreeHundredSeconds(t *testing.T) {
	clock := newFakeClock()
	lockout := newLockout(clock.Now)
	tripLockout(t, lockout)

	clock.Advance(299 * time.Second)
	if lockout.RetryAfterSeconds() != 1 {
		t.Errorf("one second before the end, wait = %d, want 1", lockout.RetryAfterSeconds())
	}
	clock.Advance(1 * time.Second)
	if outcome, _ := attemptWith(lockout, true); outcome != OutcomeUnlocked {
		t.Errorf("the right PIN after 300s gave outcome %d", outcome)
	}
}

func TestACorrectAttemptResetsTheCount(t *testing.T) {
	lockout := newLockout(newFakeClock().Now)
	attemptWith(lockout, false)
	attemptWith(lockout, false)
	if outcome, _ := attemptWith(lockout, true); outcome != OutcomeUnlocked {
		t.Fatal("the right PIN was refused")
	}
	for range 2 {
		if outcome, _ := attemptWith(lockout, false); outcome != OutcomeWrongPIN {
			t.Fatalf("a wrong PIN after a reset gave outcome %d: the count was not reset", outcome)
		}
	}
}

func TestAHeldKeypadDoesNotDeriveAKey(t *testing.T) {
	lockout := newLockout(newFakeClock().Now)
	tripLockout(t, lockout)
	called := false
	lockout.Attempt(func() bool { called = true; return true })
	if called {
		t.Error("the PIN was compared while the keypad was on hold")
	}
}

func TestParallelAttemptsAreComparedOneAtATime(t *testing.T) {
	lockout := newLockout(newFakeClock().Now)
	var guesses, inFlight, mostInFlight atomic.Int32
	var group sync.WaitGroup
	for range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			lockout.Attempt(func() bool {
				guesses.Add(1)
				now := inFlight.Add(1)
				for {
					seen := mostInFlight.Load()
					if now <= seen || mostInFlight.CompareAndSwap(seen, now) {
						break
					}
				}
				for range 100 {
					runtime.Gosched()
				}
				inFlight.Add(-1)
				return false
			})
		}()
	}
	group.Wait()
	if guesses.Load() != maxWrongPINs {
		t.Errorf("%d guesses were compared before the hold, want %d", guesses.Load(), maxWrongPINs)
	}
	if mostInFlight.Load() != 1 {
		t.Errorf("%d comparisons ran at once, want 1", mostInFlight.Load())
	}
}
