// Package api serves the interface's API contract over HTTP: a router built
// from the contract manifest, the PIN, the session, the handlers of reads and
// the dispatch of writes through the recorder.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/contract"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

const maxBodyBytes = 1 << 20

// Config is everything the HTTP layer needs from outside. Methods and Reads
// default to the embedded manifest and the registered domains; Reads holds the
// handlers of every class. Now is required: "today" comes from the caller's
// clock, never from the host's. Database is the app's own database; without it
// every read is answered as if the person had authored nothing and every write
// is refused as `read_only`.
type Config struct {
	Methods        []contract.Method
	Reads          *Registry
	Ledger         *ledger.Reader
	Database       *authored.Database
	VerifyPIN      func(attempt string) bool
	Now            func() time.Time
	SessionIdle    time.Duration
	InsecureCookie bool
	Logger         *slog.Logger
	// PINStore keeps a PIN changed in the app; it defaults to Database.
	PINStore PINStore
}

// router carries what the per-route functions share.
type router struct {
	handlers *Registry
	schemas  map[string]*jsonschema.Schema
	reader   *ledger.Reader
	database *authored.Database
	sessions *sessionService
	now      func() time.Time
}

// NewHandler builds the router from the manifest. A write or an evaluation is
// refused as `read_only` unless the server has its own database and the method
// has a handler.
func NewHandler(config Config) (http.Handler, error) {
	config, err := withDefaults(config)
	if err != nil {
		return nil, err
	}
	if err := checkRegistered(config.Reads, config.Methods); err != nil {
		return nil, err
	}
	schemas, err := compileArgumentSchemas(config.Methods)
	if err != nil {
		return nil, err
	}
	found := &router{
		handlers: config.Reads,
		schemas:  schemas,
		reader:   config.Ledger,
		database: config.Database,
		sessions: newSessionService(config),
		now:      config.Now,
	}
	mux := http.NewServeMux()
	known := map[string]bool{"/healthz": true}
	for _, method := range config.Methods {
		mux.HandleFunc(method.Route, found.serve(method))
		known[method.Route] = true
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, kindUnknownMethod)
	})
	guarded := crossOriginProtection().Handler(mux)
	return logRequests(config.Logger, config.Now, known, standardHeaders(guarded)), nil
}

func withDefaults(config Config) (Config, error) {
	if config.VerifyPIN == nil {
		return config, errors.New("api: no PIN verifier configured")
	}
	if config.Now == nil {
		return config, errors.New("api: no clock configured")
	}
	if config.Methods == nil {
		methods, err := contract.Load()
		if err != nil {
			return config, err
		}
		config.Methods = methods
	}
	if config.Reads == nil {
		config.Reads = NewReadRegistry()
	}
	if config.PINStore == nil && config.Database != nil {
		config.PINStore = config.Database
	}
	if config.SessionIdle <= 0 {
		config.SessionIdle = defaultSessionIdle
	}
	if config.Logger == nil {
		config.Logger = slog.New(slog.DiscardHandler)
	}
	return config, nil
}

// checkRegistered refuses a handler for a name the manifest does not have, for a
// `session` method, which the session service answers, and one whose shape
// cannot serve its method's class: a read's handler has no recorder to write
// through, and a write's has no business answering a read.
func checkRegistered(registry *Registry, methods []contract.Method) error {
	classes := map[string]contract.Class{}
	for _, method := range methods {
		classes[method.Qualified()] = method.Class
	}
	for _, name := range registry.Names() {
		class, known := classes[name]
		if !known {
			return fmt.Errorf("api: handler registered for %s, which is not a method of the contract", name)
		}
		if class == contract.ClassSession {
			return fmt.Errorf("api: handler registered for %s, which is a session method that the session service answers", name)
		}
		if registered, _ := registry.lookup(name); !registered.serves(class) {
			return fmt.Errorf("api: the handler registered for %s is not of a shape that can serve a method of class %s", name, class)
		}
	}
	return nil
}

func crossOriginProtection() *http.CrossOriginProtection {
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErrorWithStatus(w, r, http.StatusForbidden, kindBadRequest)
	}))
	return protection
}

func standardHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

// serve runs the checks in a fixed order, each of which can end the request:
// the verb, the session, the method's class, the content type, the body.
// The session comes before the class so that a person without one learns
// nothing but `locked`; the class comes before the body so that a write with
// no store to keep it in, or no handler, is refused without its body being read.
func (found *router) serve(method contract.Method) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, r, kindBadRequest)
			return
		}
		live := found.sessions.recognise(w, r)
		if !live && !opensWithoutSession(method) {
			writeError(w, r, kindLocked)
			return
		}
		if method.Qualified() == sessionChangePin {
			found.serveChangePin(w, r, method)
			return
		}
		if method.Class == contract.ClassWrite || method.Class == contract.ClassEvaluate {
			found.serveChange(w, r, method)
			return
		}
		arguments, body, ok := readArguments(w, r, method)
		if !ok {
			return
		}
		if method.Class == contract.ClassSession {
			found.sessions.dispatch(w, r, method, arguments, live)
			return
		}
		if validatedBySchema(method) && !argumentsMatch(found.schemas[method.Qualified()], body) {
			writeError(w, r, kindBadRequest)
			return
		}
		found.serveRead(w, r, method, arguments)
	}
}

// schemaValidatedRead is the one read whose arguments the manifest's schema
// checks before its handler runs, as a write's are: its draft is a whole budget
// document, which the schema checks from the bytes that arrived, so no amount
// passes through a float. Every other read keeps its own strict decoding.
const schemaValidatedRead = "budget.view"

func validatedBySchema(method contract.Method) bool {
	return method.Class == contract.ClassRead && method.Qualified() == schemaValidatedRead
}

// sessionChangePin is handed to the session service rather than to a handler:
// it needs the caller's token, the lockout and the sessions, and it is not a
// recorded change.
const sessionChangePin = "session.changePin"

func (found *router) serveChangePin(w http.ResponseWriter, r *http.Request, method contract.Method) {
	if found.database == nil || found.sessions.pins == nil {
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
	ctx, instant := found.requestContext(r)
	found.sessions.changePin(ctx, w, r, arguments, instant)
}

func opensWithoutSession(method contract.Method) bool {
	return method.Qualified() == "session.status" || method.Qualified() == "session.unlock"
}

// readArguments checks the content type and the body, and answers the error
// itself when either is wrong. It returns the arguments and the bytes they
// were read from, which a schema is applied to.
func readArguments(w http.ResponseWriter, r *http.Request, method contract.Method) ([]json.RawMessage, []byte, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, r, kindBadRequest)
		return nil, nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, r, kindBadRequest)
		return nil, nil, false
	}
	var arguments []json.RawMessage
	if err := json.Unmarshal(body, &arguments); err != nil || arguments == nil ||
		len(arguments) < method.MinArguments || len(arguments) > method.MaxArguments {
		writeError(w, r, kindBadRequest)
		return nil, nil, false
	}
	return arguments, body, true
}

func (found *router) serveRead(w http.ResponseWriter, r *http.Request, method contract.Method, arguments []json.RawMessage) {
	registered, ok := found.handlers.lookup(method.Qualified())
	if !ok || registered.read == nil {
		writeError(w, r, kindInternal)
		return
	}
	found.answerRead(w, r, registered.read, arguments)
}

// answerRead runs a handler over the request's views, for a read and for an
// evaluation, which stores nothing.
func (found *router) answerRead(w http.ResponseWriter, r *http.Request, handler ReadHandler, arguments []json.RawMessage) {
	views := openViews(found.reader, found.database)
	defer views.close()
	ctx, _ := found.requestContext(r)
	result, err := handler(ctx, arguments, views)
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

// requestContext reads the clock once per request, so that an answer computed
// around midnight cannot place one figure on one day and the next on another.
func (found *router) requestContext(r *http.Request) (context.Context, time.Time) {
	instant := found.now()
	return withNow(r.Context(), func() time.Time { return instant }), instant
}
