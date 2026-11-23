package suggest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/api"
	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

const (
	// DefaultTimeout is the per-request HTTP timeout for a model call.
	DefaultTimeout   = 120 * time.Second
	maxResponseBytes = 4 << 20
)

// FailureKind classifies why a suggest run failed without naming any value.
type FailureKind int

const (
	FailureUnreachable FailureKind = iota + 1
	FailureTimeout
	FailureStatus
	FailureMalformed
	FailureDatabase
)

func (kind FailureKind) String() string {
	switch kind {
	case FailureUnreachable:
		return "unreachable"
	case FailureTimeout:
		return "timeout"
	case FailureStatus:
		return "status"
	case FailureMalformed:
		return "malformed"
	case FailureDatabase:
		return "database"
	default:
		return "unknown"
	}
}

// Error is a classified failure of Run. Its message names only the failure
// kind so logs never carry ledger values, URLs, keys or model answers.
type Error struct {
	Failure FailureKind
	cause   error
}

func (err *Error) Error() string {
	return "suggest: " + err.Failure.String()
}

func (err *Error) Unwrap() error { return err.cause }

// FailureOf extracts the classified kind from err, if any.
func FailureOf(err error) (FailureKind, bool) {
	var classified *Error
	if errors.As(err, &classified) {
		return classified.Failure, true
	}
	return 0, false
}

// Sources are the ledger snapshot and the app's database that Run reads and updates.
type Sources struct {
	Ledger   *ledger.Reader
	Database *authored.Database
}

// Config is the endpoint and model configuration for a suggest run.
type Config struct {
	URL        string
	Model      string
	Key        string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// Counts summarizes one suggest run: how many requests were answered and how
// many purchases were asked, suggested or left unusable.
type Counts struct {
	Requests  int
	Asked     int
	Suggested int
	Unusable  int
}

// Run reads every unasked pending purchase at an establishment, sends one
// request per month (at most 40 purchases each) sequentially to the configured
// model, stores each answered request in a transaction of its own, and records
// the run's outcome in meta whenever at least one request was attempted.
func Run(ctx context.Context, sources Sources, config Config, now func() time.Time) (Counts, error) {
	candidates, err := readCandidates(ctx, sources)
	if err != nil {
		return Counts{}, &Error{Failure: FailureDatabase, cause: err}
	}
	requests, err := BuildRequests(config.Model, candidates)
	if err != nil {
		return Counts{}, &Error{Failure: FailureDatabase, cause: err}
	}
	if len(requests) == 0 {
		return Counts{}, nil
	}

	endpoint := strings.TrimRight(config.URL, "/") + "/chat/completions"
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	var counts Counts
	for _, req := range requests {
		if err := ctx.Err(); err != nil {
			return counts, err
		}
		body, kind, err := sendRequest(ctx, client, endpoint, config.Key, timeout, req.Body)
		if err != nil {
			if ctx.Err() != nil {
				return counts, ctx.Err()
			}
			_ = sources.Database.RecordModelRun(ctx, now(), false)
			return counts, &Error{Failure: kind, cause: err}
		}
		outcome, err := ParseResponse(body, req)
		if err != nil {
			_ = sources.Database.RecordModelRun(ctx, now(), false)
			return counts, &Error{Failure: FailureMalformed, cause: err}
		}
		if err := sources.Database.RecordModelSuggestions(ctx, config.Model, now(), outcome.Rows); err != nil {
			_ = sources.Database.RecordModelRun(ctx, now(), false)
			return counts, &Error{Failure: FailureDatabase, cause: err}
		}
		counts.Requests++
		counts.Asked += len(outcome.Rows)
		counts.Suggested += outcome.Suggested
		counts.Unusable += outcome.Unusable
	}
	if err := sources.Database.RecordModelRun(ctx, now(), true); err != nil {
		return counts, &Error{Failure: FailureDatabase, cause: err}
	}
	return counts, nil
}

// readCandidates opens both views, reads all candidates, and closes both views
// before any HTTP request is made so no database transaction is held across
// network I/O.
func readCandidates(ctx context.Context, sources Sources) (api.SuggestionCandidates, error) {
	ledgerView := sources.Ledger.View()
	defer ledgerView.Close()
	authoredView := sources.Database.View()
	defer authoredView.Close()
	return api.ReadSuggestionCandidates(ctx, api.Views{Ledger: ledgerView, Authored: authoredView})
}

func sendRequest(
	ctx context.Context,
	client *http.Client,
	endpoint, apiKey string,
	timeout time.Duration,
	body []byte,
) ([]byte, FailureKind, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, FailureUnreachable, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		if isTimeout(reqCtx, err) {
			return nil, FailureTimeout, err
		}
		return nil, FailureUnreachable, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil, FailureStatus, errors.New("non-2xx status")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		if isTimeout(reqCtx, err) {
			return nil, FailureTimeout, err
		}
		return nil, FailureUnreachable, err
	}
	if len(raw) > maxResponseBytes {
		return nil, FailureMalformed, errMalformedResponse
	}
	return raw, 0, nil
}

func isTimeout(reqCtx context.Context, err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
