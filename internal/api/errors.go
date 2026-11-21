package api

import (
	"context"
	"errors"
	"io/fs"
	"net/http"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// errorKind is the kind a client sees in {"error":{"kind":...}}; the set is the
// contract's WireErrorKind.
type errorKind string

const (
	kindLocked            errorKind = "locked"
	kindReadOnly          errorKind = "read_only"
	kindLedgerUnavailable errorKind = "ledger_unavailable"
	kindBadRequest        errorKind = "bad_request"
	kindUnknownMethod     errorKind = "unknown_method"
	kindInternal          errorKind = "internal"
)

var statusOfKind = map[errorKind]int{
	kindLocked:            http.StatusUnauthorized,
	kindReadOnly:          http.StatusForbidden,
	kindLedgerUnavailable: http.StatusServiceUnavailable,
	kindBadRequest:        http.StatusBadRequest,
	kindUnknownMethod:     http.StatusNotFound,
	kindInternal:          http.StatusInternalServerError,
}

type errorEnvelope struct {
	Error struct {
		Kind errorKind `json:"kind"`
	} `json:"error"`
}

// writeError answers with the kind's own status.
func writeError(w http.ResponseWriter, r *http.Request, kind errorKind) {
	writeErrorWithStatus(w, r, statusOfKind[kind], kind)
}

// writeErrorWithStatus is for the one answer whose status is not its kind's:
// the cross-origin refusal, which the contract has no kind of its own for.
func writeErrorWithStatus(w http.ResponseWriter, r *http.Request, status int, kind errorKind) {
	noteErrorKind(r.Context(), kind)
	var envelope errorEnvelope
	envelope.Error.Kind = kind
	writeJSONStatus(w, status, envelope)
}

// kindOfReadError maps what a read handler returned. Only the error's class is
// looked at, never its text: that text may carry a row id.
func kindOfReadError(err error) errorKind {
	var refused *badArgumentsError
	switch {
	case errors.As(err, &refused):
		return kindBadRequest
	case ledger.IsUnsupportedVersion(err),
		ledger.IsUnreadableSnapshot(err),
		errors.Is(err, fs.ErrNotExist),
		errors.Is(err, fs.ErrPermission):
		return kindLedgerUnavailable
	default:
		return kindInternal
	}
}

type errorKindNote struct{ kind errorKind }

type errorKindNoteKey struct{}

// withErrorKindNote lets the logging layer learn which kind an answer carried
// without anyone passing the error itself up to it.
func withErrorKindNote(ctx context.Context) (context.Context, *errorKindNote) {
	note := &errorKindNote{}
	return context.WithValue(ctx, errorKindNoteKey{}, note), note
}

func noteErrorKind(ctx context.Context, kind errorKind) {
	if note, ok := ctx.Value(errorKindNoteKey{}).(*errorKindNote); ok {
		note.kind = kind
	}
}

// badArgumentsError is a handler refusing arguments that passed the router's
// count but are not what the method takes. It carries no text: an argument may
// be a row id.
type badArgumentsError struct{}

func (*badArgumentsError) Error() string { return "api: the arguments are not what the method takes" }
