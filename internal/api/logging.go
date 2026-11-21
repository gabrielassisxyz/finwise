package api

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder remembers the status for the log line. It also remembers
// whether anything was written, so a panic can still be answered.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (recorder *statusRecorder) WriteHeader(status int) {
	if !recorder.wrote {
		recorder.status = status
		recorder.wrote = true
	}
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(body []byte) (int, error) {
	if !recorder.wrote {
		recorder.WriteHeader(http.StatusOK)
	}
	return recorder.ResponseWriter.Write(body)
}

func (recorder *statusRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}

// logRequests writes one line per request: the verb, the route, the status,
// the duration and, for a refusal, its kind. Nothing else about a request is
// read here: a body, an argument, a cookie, a PIN or a value from the ledger
// would put a person's finances in the log. The route is the manifest's, never
// the raw path, which a client controls. A handler that panics is answered as
// `internal`, and the panic value, which may hold data, is dropped.
func logRequests(logger *slog.Logger, now func() time.Time, knownRoutes map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := now()
		ctx, note := withErrorKindNote(r.Context())
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		func() {
			defer func() {
				if recover() == nil {
					return
				}
				if recorder.wrote {
					return
				}
				writeError(recorder, r.WithContext(ctx), kindInternal)
			}()
			next.ServeHTTP(recorder, r.WithContext(ctx))
		}()
		route := "unmatched"
		if knownRoutes[r.URL.Path] {
			route = r.URL.Path
		}
		attrs := []slog.Attr{
			slog.String("verb", r.Method),
			slog.String("route", route),
			slog.Int("status", recorder.status),
			slog.Int64("duration_ms", now().Sub(started).Milliseconds()),
		}
		if note.kind != "" {
			attrs = append(attrs, slog.String("error_kind", string(note.kind)))
		}
		logger.LogAttrs(r.Context(), slog.LevelInfo, "request", attrs...)
	})
}
