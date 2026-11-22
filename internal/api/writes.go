package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/contract"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// WriteHandler serves one `write` method that records what it changes. It
// works inside the one transaction the router opened for it, through the
// recorder, and returns; the router commits. A handler that returns an error,
// panics or returns a result that cannot be encoded keeps nothing, and the
// error is answered by its class as a read's is, a refusal included.
//
// The change's row is written with its first effect, or when the handler asks
// the recorder for the change's id, which a handler whose result may not be null
// must do even when it changed nothing. A handler that does neither leaves no
// trace in the journal and answers null.
type WriteHandler func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error)

// Writes is what one write is answered from: the request's view of the ledger
// and the recorder of its change. What a write reads of the app's database it
// reads through the recorder, inside the same transaction.
type Writes struct {
	Ledger   *ledger.View
	Recorder *authored.Recorder
}

// JournalHandler serves a `write` method that acts on changes already
// committed. Each call of the engine is a transaction of its own, so the router
// opens none around it: a change begun inside another change's transaction
// would wait for a lock its own caller holds. For the same reason what the
// handler returns is encoded after the engine's call has committed, so its
// result must not be one that can fail to encode.
type JournalHandler func(ctx context.Context, args []json.RawMessage, journal Journal) (any, error)

// Journal is the engine's calls on committed changes.
type Journal interface {
	Undo(ctx context.Context, id authored.ChangeID, at time.Time) error
}

// serveChange answers a `write` or an `evaluate` method. The app's own database
// and a handler for the method are both needed: without either the answer is
// `read_only`, and it is given before the body is read.
func (found *router) serveChange(w http.ResponseWriter, r *http.Request, method contract.Method) {
	registered, ok := found.handlers.lookup(method.Qualified())
	if found.database == nil || !ok {
		writeError(w, r, kindReadOnly)
		return
	}
	arguments, body, ok := readArguments(w, r, method)
	if !ok {
		return
	}
	if !argumentsMatch(found.schemas[method.Qualified()], body) {
		writeError(w, r, kindBadRequest)
		return
	}
	switch {
	case registered.write != nil:
		found.serveWrite(w, r, method, registered.write, arguments)
	case registered.journal != nil:
		found.serveJournal(w, r, registered.journal, arguments)
	default:
		found.answerRead(w, r, registered.read, arguments)
	}
}

// serveWrite runs the handler inside a change. The result is encoded before the
// transaction commits, so a result that cannot be encoded keeps nothing, and
// the answer is written only after the commit: a request that fails at any
// point answers its failure and nothing it did is kept.
func (found *router) serveWrite(w http.ResponseWriter, r *http.Request, method contract.Method, handler WriteHandler, arguments []json.RawMessage) {
	ctx, instant := found.requestContext(r)
	// The authored half stays empty: a write reads the app's database through
	// its recorder, in its own transaction.
	views := openViews(found.reader, nil)
	defer views.close()
	facts := authored.ChangeFacts{Author: authored.AuthorPerson, Method: method.Qualified(), At: instant}
	var encoded []byte
	err := found.database.Change(ctx, facts, func(recorder *authored.Recorder) error {
		result, err := callWrite(ctx, handler, arguments, Writes{Ledger: views.Ledger, Recorder: recorder})
		if err != nil {
			return err
		}
		encoded, err = json.Marshal(result)
		return err
	})
	if err != nil {
		writeFailure(w, r, err)
		return
	}
	writeEncoded(w, encoded)
}

// callWrite turns a panic into an error. The transaction is open underneath, and
// only a return lets it be rolled back: a panic that went up through it would
// leave the write lock held by a connection nobody ends.
func callWrite(ctx context.Context, handler WriteHandler, arguments []json.RawMessage, writes Writes) (result any, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, errHandlerPanicked
		}
	}()
	return handler(ctx, arguments, writes)
}

func (found *router) serveJournal(w http.ResponseWriter, r *http.Request, handler JournalHandler, arguments []json.RawMessage) {
	ctx, _ := found.requestContext(r)
	result, err := handler(ctx, arguments, found.database)
	if err != nil {
		writeFailure(w, r, err)
		return
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		writeError(w, r, kindInternal)
		return
	}
	writeEncoded(w, encoded)
}

func writeEncoded(w http.ResponseWriter, encoded []byte) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encoded)
}
