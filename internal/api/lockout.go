package api

import (
	"math"
	"sync"
	"time"
)

const (
	maxWrongPINs    = 3
	lockoutDuration = 300 * time.Second
)

// Lockout holds the keypad after too many wrong PINs. It is global on purpose:
// the app has one user, so a per-client count would only help a guesser who
// has several addresses.
type Lockout struct {
	mutex     sync.Mutex
	now       func() time.Time
	wrongPINs int
	heldUntil time.Time
}

// UnlockOutcome is what one attempt came to.
type UnlockOutcome int

const (
	OutcomeUnlocked UnlockOutcome = iota
	OutcomeWrongPIN
	OutcomeHeld
)

func newLockout(now func() time.Time) *Lockout {
	return &Lockout{now: now}
}

// RetryAfterSeconds is zero unless the keypad is on hold.
func (lockout *Lockout) RetryAfterSeconds() int {
	lockout.mutex.Lock()
	defer lockout.mutex.Unlock()
	return lockout.retryAfterLocked()
}

func (lockout *Lockout) retryAfterLocked() int {
	remaining := lockout.heldUntil.Sub(lockout.now())
	if remaining <= 0 {
		return 0
	}
	return int(math.Ceil(remaining.Seconds()))
}

// Attempt runs one PIN attempt. verify is not called while the keypad is on
// hold, so a held lock costs no key derivation and cannot be probed. The mutex
// stays held across verify: attempts that ran in parallel could all be counted
// before any of them tripped the hold, and each would then get a guess.
func (lockout *Lockout) Attempt(verify func() bool) (UnlockOutcome, int) {
	lockout.mutex.Lock()
	defer lockout.mutex.Unlock()
	if wait := lockout.retryAfterLocked(); wait > 0 {
		return OutcomeHeld, wait
	}
	if verify() {
		lockout.wrongPINs = 0
		return OutcomeUnlocked, 0
	}
	lockout.wrongPINs++
	if lockout.wrongPINs < maxWrongPINs {
		return OutcomeWrongPIN, 0
	}
	lockout.wrongPINs = 0
	lockout.heldUntil = lockout.now().Add(lockoutDuration)
	return OutcomeHeld, int(lockoutDuration / time.Second)
}
