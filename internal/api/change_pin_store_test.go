package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// gatedPINStore holds the first write on a channel and counts how many calls
// are inside it at once.
type gatedPINStore struct {
	inner   PINStore
	entered chan struct{}
	release chan struct{}
	inside  atomic.Int32
	widest  atomic.Int32
	first   sync.Once
}

func (store *gatedPINStore) StorePINHash(ctx context.Context, hash string, at time.Time) error {
	now := store.inside.Add(1)
	defer store.inside.Add(-1)
	for {
		widest := store.widest.Load()
		if now <= widest || store.widest.CompareAndSwap(widest, now) {
			break
		}
	}
	store.first.Do(func() {
		close(store.entered)
		<-store.release
	})
	return store.inner.StorePINHash(ctx, hash, at)
}

// cancellingPINStore ends the request's context before it writes, as a client
// that hangs up at the wrong moment would.
type cancellingPINStore struct {
	inner  PINStore
	cancel context.CancelFunc
}

func (store *cancellingPINStore) StorePINHash(ctx context.Context, hash string, at time.Time) error {
	store.cancel()
	return store.inner.StorePINHash(ctx, hash, at)
}

func TestTwoChangesAtOnceNeverWriteTheHashTogetherAndTheSecondSeesTheFirst(t *testing.T) {
	scratch := authoredtest.Open(t, newFakeClock().Now)
	gate := &gatedPINStore{inner: scratch.Database, entered: make(chan struct{}), release: make(chan struct{})}
	env := newTestEnv(t, withDatabase(scratch.Database), func(config *Config) { config.PINStore = gate })
	first, second := env.unlock(), env.unlock()

	answers := make(chan *httptest.ResponseRecorder, 2)
	go func() { answers <- env.change(first, testPIN, newTestPIN, newTestPIN) }()
	select {
	case <-gate.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the first change never reached the store")
	}
	secondAnswer := make(chan *httptest.ResponseRecorder, 1)
	go func() { secondAnswer <- env.change(second, testPIN, thirdTestPIN, thirdTestPIN) }()
	// Long enough for the second change to verify a PIN and reach the store, if
	// nothing kept it out.
	time.Sleep(time.Second)
	close(gate.release)

	firstAnswer := <-answers
	if firstAnswer.Body.String() != `{"ok":true}` {
		t.Errorf("the first change answered %s", firstAnswer.Body.String())
	}
	env.expectFault("the second change", <-secondAnswer, "wrong-current")
	if widest := gate.widest.Load(); widest != 1 {
		t.Errorf("%d calls were inside the store at once, want 1", widest)
	}
	if !env.stillUnlocksWith(newTestPIN) || env.stillUnlocksWith(thirdTestPIN) {
		t.Error("the PIN that opens the app is not the first change's")
	}
	expectStoredHashVerifies(t, scratch, newTestPIN)
}

func TestAChangeCompletesWhenTheRequestIsCancelledBeforeTheWrite(t *testing.T) {
	scratch := authoredtest.Open(t, newFakeClock().Now)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	store := &cancellingPINStore{inner: scratch.Database, cancel: cancel}
	env := newTestEnv(t, withDatabase(scratch.Database), func(config *Config) { config.PINStore = store })
	cookie := env.unlock()

	response := env.do(request{
		path: changePinRoute, body: `["` + testPIN + `","` + newTestPIN + `","` + newTestPIN + `"]`, cookie: cookie, ctx: ctx,
	})
	if response.Code != http.StatusOK || response.Body.String() != `{"ok":true}` {
		t.Fatalf("answered %d %s", response.Code, response.Body.String())
	}
	if !env.stillUnlocksWith(newTestPIN) {
		t.Error("the new PIN does not open the app")
	}
	expectStoredHashVerifies(t, scratch, newTestPIN)
}

func expectStoredHashVerifies(t *testing.T, scratch *authoredtest.Scratch, pin string) {
	t.Helper()
	var stored string
	if err := scratch.Inspect(t).QueryRowContext(t.Context(), "SELECT hash FROM pin").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	hash, err := ParsePINHash(stored)
	if err != nil || !hash.Verify(pin) {
		t.Error("the stored hash does not verify the PIN that opens the app")
	}
}
