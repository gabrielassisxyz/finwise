package api

import (
	"encoding/json"
	"net/http"

	"github.com/gabrielassisxyz/finwise/internal/contract"
)

// sessionService answers the contract's session methods.
type sessionService struct {
	store     *sessionStore
	lockout   *Lockout
	verifyPIN func(attempt string) bool
	cookie    sessionCookie
}

func newSessionService(config Config) *sessionService {
	return &sessionService{
		store:     newSessionStore(config.Now, config.SessionIdle),
		lockout:   newLockout(config.Now),
		verifyPIN: config.VerifyPIN,
		cookie:    newSessionCookie(config.InsecureCookie, config.SessionIdle),
	}
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
	service.store.Remove(service.cookie.tokenOf(r))
	// recognise already renewed the cookie; two Set-Cookie lines for one name
	// leave the outcome to whichever a client reads.
	w.Header().Del("Set-Cookie")
	service.cookie.clear(w)
	writeJSON(w, nil)
}
