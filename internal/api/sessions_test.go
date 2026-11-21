package api

import (
	"testing"
	"time"
)

func TestASessionExpiresAfterTheIdleLimit(t *testing.T) {
	clock := newFakeClock()
	store := newSessionStore(clock.Now, defaultSessionIdle)
	token, err := store.Create()
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(defaultSessionIdle - time.Second)
	if !store.Touch(token) {
		t.Fatal("a session used within 12 hours was refused")
	}
	clock.Advance(defaultSessionIdle)
	if store.Touch(token) {
		t.Error("a session unused for 12 hours was accepted")
	}
}

func TestUseKeepsASessionAliveBeyondTheIdleLimit(t *testing.T) {
	clock := newFakeClock()
	store := newSessionStore(clock.Now, defaultSessionIdle)
	token, _ := store.Create()
	clock.Advance(11 * time.Hour)
	if !store.Touch(token) {
		t.Fatal("refused at 11 hours")
	}
	clock.Advance(9 * time.Hour)
	if !store.Touch(token) {
		t.Error("refused at 20 hours after a request at 11")
	}
}

func TestAnUnknownOrRemovedTokenIsRefused(t *testing.T) {
	store := newSessionStore(newFakeClock().Now, defaultSessionIdle)
	if store.Touch("") || store.Touch("made-up") {
		t.Error("a token that was never issued was accepted")
	}
	token, _ := store.Create()
	store.Remove(token)
	if store.Touch(token) {
		t.Error("a removed session was accepted")
	}
}

func TestTokensAreLongAndDistinct(t *testing.T) {
	store := newSessionStore(newFakeClock().Now, defaultSessionIdle)
	first, _ := store.Create()
	second, _ := store.Create()
	if first == second {
		t.Error("two sessions share a token")
	}
	if len(first) < 43 { // 32 bytes in unpadded base64
		t.Errorf("token is %d characters, want 256 bits", len(first))
	}
}

func TestExpiredSessionsAreDroppedWhenANewOneIsMade(t *testing.T) {
	clock := newFakeClock()
	store := newSessionStore(clock.Now, defaultSessionIdle)
	store.Create()
	clock.Advance(defaultSessionIdle)
	store.Create()
	if len(store.lastSeen) != 1 {
		t.Errorf("%d sessions held, want 1", len(store.lastSeen))
	}
}
