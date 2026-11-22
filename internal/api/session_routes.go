package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/contract"
)

// PINStore keeps the hash of a PIN the person set in the app, where the next
// start finds it.
type PINStore interface {
	StorePINHash(ctx context.Context, hash string, at time.Time) error
}

// sessionService answers the contract's session methods, and changePin, which
// is a write that only it can answer: it needs the caller's token, the lockout
// and the sessions.
type sessionService struct {
	store   *sessionStore
	lockout *Lockout
	cookie  sessionCookie
	pins    PINStore

	// changing serialises a change from its verification to the swap of the
	// verifier. The lockout's mutex is not held across the database write.
	changing sync.Mutex
	verifier sync.RWMutex
	verify   func(attempt string) bool
}

func newSessionService(config Config) *sessionService {
	return &sessionService{
		store:   newSessionStore(config.Now, config.SessionIdle),
		lockout: newLockout(config.Now),
		cookie:  newSessionCookie(config.InsecureCookie, config.SessionIdle),
		pins:    config.PINStore,
		verify:  config.VerifyPIN,
	}
}

func (service *sessionService) verifyPIN(attempt string) bool {
	service.verifier.RLock()
	verify := service.verify
	service.verifier.RUnlock()
	return verify(attempt)
}

func (service *sessionService) replaceVerifier(verify func(attempt string) bool) {
	service.verifier.Lock()
	service.verify = verify
	service.verifier.Unlock()
}

// recognise reports whether the request carries a live session and, if so,
// sends the cookie again so its lifetime follows the server's sliding expiry.
func (service *sessionService) recognise(w http.ResponseWriter, r *http.Request) bool {
	token := service.cookie.tokenOf(r)
	if !service.store.Touch(token) {
		return false
	}
	service.cookie.issue(w, token)
	return true
}

func (service *sessionService) dispatch(w http.ResponseWriter, r *http.Request, method contract.Method, arguments []json.RawMessage, live bool) {
	switch method.Name {
	case "status":
		writeJSON(w, map[string]any{"locked": !live, "retryAfterSeconds": service.lockout.RetryAfterSeconds()})
	case "unlock":
		service.unlock(w, r, arguments)
	case "lock":
		service.lock(w, r)
	default:
		writeError(w, r, kindInternal)
	}
}

func (service *sessionService) unlock(w http.ResponseWriter, r *http.Request, arguments []json.RawMessage) {
	var attempt string
	if err := json.Unmarshal(arguments[0], &attempt); err != nil {
		writeError(w, r, kindBadRequest)
		return
	}
	outcome, wait := service.lockout.Attempt(func() bool { return service.verifyPIN(attempt) })
	switch outcome {
	case OutcomeUnlocked:
		service.openSession(w, r)
	case OutcomeHeld:
		writeJSON(w, map[string]any{"ok": false, "reason": "locked-out", "retryAfterSeconds": wait})
	default:
		writeJSON(w, map[string]any{"ok": false, "reason": "wrong-pin"})
	}
}

func (service *sessionService) openSession(w http.ResponseWriter, r *http.Request) {
	token, err := service.store.Create()
	if err != nil {
		writeError(w, r, kindInternal)
		return
	}
	service.cookie.issue(w, token)
	writeJSON(w, map[string]any{"ok": true})
}

func (service *sessionService) lock(w http.ResponseWriter, r *http.Request) {
	// recognise already renewed the cookie; two Set-Cookie lines for one name
	// leave the outcome to whichever a client reads.
	service.endCallersSession(w, r)
	writeJSON(w, nil)
}

// changePin verifies the current PIN under the lockout the PIN screen shares,
// checks the new one, stores its hash, makes the server verify against it and
// ends every other session. The store comes before the swap, so a write that
// fails leaves the old PIN as the one that opens the app.
func (service *sessionService) changePin(ctx context.Context, w http.ResponseWriter, r *http.Request, arguments []json.RawMessage, at time.Time) {
	var current, next, repeat string
	for index, target := range []*string{&current, &next, &repeat} {
		if json.Unmarshal(arguments[index], target) != nil {
			writeError(w, r, kindBadRequest)
			return
		}
	}
	service.changing.Lock()
	defer service.changing.Unlock()
	outcome, _ := service.lockout.Attempt(func() bool { return service.verifyPIN(current) })
	switch outcome {
	case OutcomeHeld:
		service.endCallersSession(w, r)
		writeError(w, r, kindLocked)
		return
	case OutcomeWrongPIN:
		writeJSON(w, map[string]any{"ok": false, "reason": "wrong-current"})
		return
	}
	if !ValidPIN(next) {
		writeJSON(w, map[string]any{"ok": false, "reason": "invalid-new"})
		return
	}
	if repeat != next {
		writeJSON(w, map[string]any{"ok": false, "reason": "mismatch"})
		return
	}
	if !service.replacePIN(ctx, next, at) {
		writeError(w, r, kindInternal)
		return
	}
	service.store.KeepOnly(service.cookie.tokenOf(r))
	writeJSON(w, map[string]any{"ok": true})
}

// replacePIN stores the hash of pin and only then makes the server verify
// against it. The write is not cancelled with the request: once the row is
// committed the verifier has to follow it.
func (service *sessionService) replacePIN(ctx context.Context, pin string, at time.Time) bool {
	stored, err := NewPINHash(pin)
	if err != nil {
		return false
	}
	parsed, err := ParsePINHash(stored)
	if err != nil {
		return false
	}
	if err := service.pins.StorePINHash(context.WithoutCancel(ctx), stored, at); err != nil {
		return false
	}
	service.replaceVerifier(parsed.Verify)
	return true
}

// endCallersSession removes the caller's session and the cookie that
// recognise renewed, leaving the clearing one as the only Set-Cookie line.
func (service *sessionService) endCallersSession(w http.ResponseWriter, r *http.Request) {
	service.store.Remove(service.cookie.tokenOf(r))
	w.Header().Del("Set-Cookie")
	service.cookie.clear(w)
}
