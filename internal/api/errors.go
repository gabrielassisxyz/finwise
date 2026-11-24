package api

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/authored"
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
	kindRefused           errorKind = "refused"
)

var statusOfKind = map[errorKind]int{
	kindLocked:            http.StatusUnauthorized,
	kindReadOnly:          http.StatusForbidden,
	kindLedgerUnavailable: http.StatusServiceUnavailable,
	kindBadRequest:        http.StatusBadRequest,
	kindUnknownMethod:     http.StatusNotFound,
	kindInternal:          http.StatusInternalServerError,
	kindRefused:           http.StatusConflict,
}

// refusalReason is why the app declined a request that was well formed: the
// contract's RefusalReason. The interface has a sentence for each, so the
// server sends none, and a reason outside the list is never sent.
type refusalReason string

const (
	reasonMonthClosed            refusalReason = "month_closed"
	reasonFutureMonth            refusalReason = "future_month"
	reasonPendingReviews         refusalReason = "pending_reviews"
	reasonStaleChange            refusalReason = "stale_change"
	reasonNotFound               refusalReason = "not_found"
	reasonUnknownLine            refusalReason = "unknown_line"
	reasonInvalidBudget          refusalReason = "invalid_budget"
	reasonUnstablePot            refusalReason = "unstable_pot"
	reasonSplitWithReimbursement refusalReason = "split_with_reimbursement"
	reasonProposalStale          refusalReason = "proposal_stale"
)

// refusalReasons is the closed list, in the contract's order.
var refusalReasons = []refusalReason{
	reasonMonthClosed,
	reasonFutureMonth,
	reasonPendingReviews,
	reasonStaleChange,
	reasonNotFound,
	reasonUnknownLine,
	reasonInvalidBudget,
	reasonUnstablePot,
	reasonSplitWithReimbursement,
	reasonProposalStale,
}

// errorEnvelope is a closed union: every kind but refused carries nothing else,
// and refused always carries a reason.
type errorEnvelope struct {
	Error struct {
		Kind   errorKind     `json:"kind"`
		Reason refusalReason `json:"reason,omitempty"`
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

// writeRefusal answers a request the app declined. The log learns only that
// the kind was refused: the reason is not a field of the request log.
func writeRefusal(w http.ResponseWriter, r *http.Request, reason refusalReason) {
	if !slices.Contains(refusalReasons, reason) {
		writeError(w, r, kindInternal)
		return
	}
	noteErrorKind(r.Context(), kindRefused)
	var envelope errorEnvelope
	envelope.Error.Kind, envelope.Error.Reason = kindRefused, reason
	writeJSONStatus(w, statusOfKind[kindRefused], envelope)
}

// writeFailure answers what a handler returned, whatever the method's class.
func writeFailure(w http.ResponseWriter, r *http.Request, err error) {
	kind, reason := classify(err)
	if kind == kindRefused {
		writeRefusal(w, r, reason)
		return
	}
	writeError(w, r, kind)
}

// classify maps what a handler returned. Only the error's class is looked at,
// never its text: that text may carry a row id. The reason is meaningful only
// for kindRefused.
func classify(err error) (errorKind, refusalReason) {
	var refusal *refusalError
	var stale *authored.StaleChangeError
	var bad *badArgumentsError
	var readOnly *readOnlyError
	switch {
	case errors.As(err, &refusal):
		return kindRefused, refusal.reason
	case errors.As(err, &readOnly):
		return kindReadOnly, ""
	case errors.As(err, &stale):
		return kindRefused, reasonStaleChange
	case errors.As(err, &bad):
		return kindBadRequest, ""
	case ledger.IsUnsupportedVersion(err),
		ledger.IsUnreadableSnapshot(err),
		ledger.IsNotProjected(err),
		errors.Is(err, fs.ErrNotExist),
		errors.Is(err, fs.ErrPermission):
		return kindLedgerUnavailable, ""
	default:
		return kindInternal, ""
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

// refusalError is a handler declining a request that was well formed, for one
// of the closed list's reasons. It carries the reason and no text: an argument
// may be a row id.
type refusalError struct{ reason refusalReason }

func (err *refusalError) Error() string { return "api: the request was refused: " + string(err.reason) }

// readOnlyError is a handler answering that what the request would act on is not
// something the app changes, as a method with no handler is answered. It carries
// no text: an argument may be a row id.
type readOnlyError struct{}

func (*readOnlyError) Error() string { return "api: the request is not one the app changes" }

// refuse is how a handler declines a request for a reason of the list.
func refuse(reason refusalReason) error { return &refusalError{reason: reason} }

// errHandlerPanicked is what a write handler's panic becomes, so that the
// transaction it ran in ends. The panic's value is dropped: it may hold data.
var errHandlerPanicked = errors.New("api: a handler panicked")
