// Package api serves the interface's API contract over HTTP: a router built
// from the contract manifest, the PIN, the session and the refusal of writes.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/contract"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

const maxBodyBytes = 1 << 20

// Config is everything the HTTP layer needs from outside. Methods and Reads
// default to the embedded manifest and the registered domains.
type Config struct {
	Methods        []contract.Method
	Reads          *Registry
	Ledger         *ledger.Reader
	VerifyPIN      func(attempt string) bool
	Now            func() time.Time
	SessionIdle    time.Duration
	InsecureCookie bool
	Logger         *slog.Logger
}

// router carries what the per-route functions share.
type router struct {
	reads    *Registry
	reader   *ledger.Reader
	sessions *sessionService
}

// NewHandler builds the router from the manifest. A write or an evaluation has
// no handler anywhere: the route exists only to refuse.
func NewHandler(config Config) (http.Handler, error) {
	config, err := withDefaults(config)
	if err != nil {
		return nil, err
	}
	if err := checkRegistered(config.Reads, config.Methods); err != nil {
		return nil, err
	}
	found := &router{
		reads:    config.Reads,
		reader:   config.Ledger,
		sessions: newSessionService(config),
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
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.SessionIdle <= 0 {
		config.SessionIdle = defaultSessionIdle
	}
	if config.Logger == nil {
		config.Logger = slog.New(slog.DiscardHandler)
	}
	return config, nil
}

// checkRegistered refuses a handler for anything that is not a `read` method,
// so that a write cannot be given a handler by mistake.
func checkRegistered(reads *Registry, methods []contract.Method) error {
	readMethods := map[string]bool{}
	for _, method := range methods {
		if method.Class == contract.ClassRead {
			readMethods[method.Qualified()] = true
		}
	}
	for _, name := range reads.Names() {
		if !readMethods[name] {
			return fmt.Errorf("api: handler registered for %s, which is not a read method of the contract", name)
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
// nothing but `locked`; the class comes before the body so that a write is
// refused without its body being read.
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
		if method.Class == contract.ClassWrite || method.Class == contract.ClassEvaluate {
			writeError(w, r, kindReadOnly)
			return
		}
		arguments, ok := readArguments(w, r, method)
		if !ok {
			return
		}
		if method.Class == contract.ClassSession {
			found.sessions.dispatch(w, r, method, arguments, live)
			return
		}
		found.serveRead(w, r, method, arguments)
	}
}

func opensWithoutSession(method contract.Method) bool {
	return method.Qualified() == "session.status" || method.Qualified() == "session.unlock"
}

// readArguments checks the content type and the body, and answers the error
// itself when either is wrong.
func readArguments(w http.ResponseWriter, r *http.Request, method contract.Method) ([]json.RawMessage, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, r, kindBadRequest)
		return nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, r, kindBadRequest)
		return nil, false
	}
	var arguments []json.RawMessage
	if err := json.Unmarshal(body, &arguments); err != nil || arguments == nil ||
		len(arguments) < method.MinArguments || len(arguments) > method.MaxArguments {
		writeError(w, r, kindBadRequest)
		return nil, false
	}
	return arguments, true
}

func (found *router) serveRead(w http.ResponseWriter, r *http.Request, method contract.Method, arguments []json.RawMessage) {
	handler, registered := found.reads.lookup(method.Qualified())
	if !registered {
		writeError(w, r, kindInternal)
		return
	}
	result, err := handler(r.Context(), arguments, found.reader)
	if err != nil {
		writeError(w, r, kindOfReadError(err))
		return
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		writeError(w, r, kindInternal)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encoded)
}
