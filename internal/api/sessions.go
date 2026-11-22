package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"
)

const (
	sessionTokenBytes  = 32
	defaultSessionIdle = 12 * time.Hour
)

// sessionStore keeps sessions in memory. A restart loses them, which only
// means typing the PIN again.
type sessionStore struct {
	mutex    sync.Mutex
	now      func() time.Time
	idleTTL  time.Duration
	lastSeen map[[sha256.Size]byte]time.Time
}

func newSessionStore(now func() time.Time, idleTTL time.Duration) *sessionStore {
	return &sessionStore{now: now, idleTTL: idleTTL, lastSeen: map[[sha256.Size]byte]time.Time{}}
}

// digest keys the map by a hash of the token, so that a lookup never compares
// the secret itself and the map holds nothing that works as a cookie.
func digest(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

// Create returns the token of a new session.
func (store *sessionStore) Create() (string, error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.dropExpiredLocked()
	store.lastSeen[digest(token)] = store.now()
	return token, nil
}

// Touch reports whether token names a live session and, if so, counts the
// request as use, so a session only expires after idleTTL without one.
func (store *sessionStore) Touch(token string) bool {
	if token == "" {
		return false
	}
	key := digest(token)
	store.mutex.Lock()
	defer store.mutex.Unlock()
	seen, found := store.lastSeen[key]
	if !found {
		return false
	}
	now := store.now()
	if now.Sub(seen) >= store.idleTTL {
		delete(store.lastSeen, key)
		return false
	}
	store.lastSeen[key] = now
	return true
}

// Remove ends the session, if there is one.
func (store *sessionStore) Remove(token string) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.lastSeen, digest(token))
}

// KeepOnly ends every session but the one token names. A token that names none
// ends them all.
func (store *sessionStore) KeepOnly(token string) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	kept := digest(token)
	for key := range store.lastSeen {
		if key != kept {
			delete(store.lastSeen, key)
		}
	}
}

func (store *sessionStore) dropExpiredLocked() {
	now := store.now()
	for key, seen := range store.lastSeen {
		if now.Sub(seen) >= store.idleTTL {
			delete(store.lastSeen, key)
		}
	}
}
