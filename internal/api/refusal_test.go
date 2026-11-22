package api

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/contract"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

var errorSchema struct {
	once   sync.Once
	schema *jsonschema.Schema
	err    error
}

// errorEnvelopeProblem is empty when the body is a valid error envelope of the
// manifest: the union of a kind that carries nothing and a refusal with a
// reason of the list.
func errorEnvelopeProblem(body []byte) string {
	errorSchema.once.Do(func() {
		errorSchema.schema, errorSchema.err = compileErrorEnvelope()
	})
	if errorSchema.err != nil {
		return errorSchema.err.Error()
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return "the body is not JSON: " + err.Error()
	}
	if err := errorSchema.schema.Validate(instance); err != nil {
		return err.Error()
	}
	return ""
}

func compileErrorEnvelope() (*jsonschema.Schema, error) {
	raw, err := os.ReadFile("../contract/contract.json")
	if err != nil {
		return nil, err
	}
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	manifest, ok := parsed.(map[string]any)
	if !ok {
		return nil, errors.New("the manifest is not an object")
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft7)
	document := map[string]any{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"definitions": manifest["definitions"],
		"allOf":       []any{manifest["error"]},
	}
	if err := compiler.AddResource("urn:finwise:error", document); err != nil {
		return nil, err
	}
	return compiler.Compile("urn:finwise:error")
}

func answerTo(write func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	write(recorder, httptest.NewRequest(http.MethodPost, "/api/changes/undo", nil))
	return recorder
}

func TestARefusalIsA409WithItsReasonAndNothingElse(t *testing.T) {
	for _, reason := range refusalReasons {
		response := answerTo(func(w http.ResponseWriter, r *http.Request) { writeRefusal(w, r, reason) })
		want := `{"error":{"kind":"refused","reason":"` + string(reason) + `"}}`
		if response.Code != http.StatusConflict || response.Body.String() != want {
			t.Errorf("%s: status %d body %s, want 409 %s", reason, response.Code, response.Body.String(), want)
		}
		if problem := errorEnvelopeProblem(response.Body.Bytes()); problem != "" {
			t.Errorf("%s does not conform to the manifest's error envelope: %s", reason, problem)
		}
	}
}

func TestEveryOtherKindsBodyHasNoReason(t *testing.T) {
	plain := []errorKind{kindLocked, kindReadOnly, kindLedgerUnavailable, kindBadRequest, kindUnknownMethod, kindInternal}
	for _, kind := range plain {
		response := answerTo(func(w http.ResponseWriter, r *http.Request) { writeError(w, r, kind) })
		want := `{"error":{"kind":"` + string(kind) + `"}}`
		if response.Body.String() != want {
			t.Errorf("%s: body %s, want %s", kind, response.Body.String(), want)
		}
		if strings.Contains(response.Body.String(), "reason") {
			t.Errorf("%s carries a reason: %s", kind, response.Body.String())
		}
		if status := statusOfKind[kind]; response.Code != status || status == http.StatusConflict {
			t.Errorf("%s: status %d, want %d and not the refusal's 409", kind, response.Code, status)
		}
		if problem := errorEnvelopeProblem(response.Body.Bytes()); problem != "" {
			t.Errorf("%s does not conform to the manifest's error envelope: %s", kind, problem)
		}
	}
}

func TestTheEnvelopeIsAClosedUnionARefusalWithoutAReasonIsNotSent(t *testing.T) {
	for _, reason := range []refusalReason{"", "made_up", "STALE_CHANGE", "stale_change "} {
		response := answerTo(func(w http.ResponseWriter, r *http.Request) { writeRefusal(w, r, reason) })
		if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error":{"kind":"internal"}}` {
			t.Errorf("reason %q: status %d body %s, want the internal answer", reason, response.Code, response.Body.String())
		}
	}
	if problem := errorEnvelopeProblem([]byte(`{"error":{"kind":"refused"}}`)); problem == "" {
		t.Error("the manifest accepts a refusal without a reason, so the check proves nothing")
	}
	if problem := errorEnvelopeProblem([]byte(`{"error":{"kind":"locked","reason":"stale_change"}}`)); problem == "" {
		t.Error("the manifest accepts a reason on another kind, so the check proves nothing")
	}
}

func TestGoRefusalReasonsAreTheManifestsList(t *testing.T) {
	manifest, err := contract.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	declared := make([]string, len(refusalReasons))
	for i, reason := range refusalReasons {
		declared[i] = string(reason)
	}
	want := append([]string(nil), manifest.RefusalReasons...)
	sort.Strings(declared)
	sort.Strings(want)
	if fmt.Sprint(declared) != fmt.Sprint(want) {
		t.Errorf("Go's reasons are %v, the manifest's are %v", declared, want)
	}
	if len(refusalReasons) != 9 {
		t.Errorf("%d reasons, want the nine of the contract", len(refusalReasons))
	}
}

func TestHandlerErrorsAreMappedByClassWhateverTheMethod(t *testing.T) {
	stale := func(cause authored.StaleCause) error { return &authored.StaleChangeError{Change: 7, Cause: cause} }
	cases := map[string]struct {
		err    error
		kind   errorKind
		reason refusalReason
	}{
		"a refusal":                 {refuse(reasonMonthClosed), kindRefused, reasonMonthClosed},
		"every reason is carried":   {refuse(reasonSplitWithReimbursement), kindRefused, reasonSplitWithReimbursement},
		"a wrapped refusal":         {fmt.Errorf("closing: %w", refuse(reasonPendingReviews)), kindRefused, reasonPendingReviews},
		"an unknown change":         {stale(authored.CauseUnknown), kindRefused, reasonStaleChange},
		"an undone change":          {stale(authored.CauseUndone), kindRefused, reasonStaleChange},
		"a later change":            {stale(authored.CauseLaterChange), kindRefused, reasonStaleChange},
		"an overwritten unit":       {stale(authored.CauseOverwritten), kindRefused, reasonStaleChange},
		"a database that refuses":   {stale(authored.CauseConstraint), kindRefused, reasonStaleChange},
		"a check that refuses":      {stale(authored.CauseCheck), kindRefused, reasonStaleChange},
		"another method's change":   {stale(authored.CauseOtherMethod), kindRefused, reasonStaleChange},
		"an extended change":        {stale(authored.CauseExtended), kindRefused, reasonStaleChange},
		"a wrapped stale change":    {fmt.Errorf("undo: %w", stale(authored.CauseLaterChange)), kindRefused, reasonStaleChange},
		"arguments":                 {&badArgumentsError{}, kindBadRequest, ""},
		"a read-only input":         {&readOnlyError{}, kindReadOnly, ""},
		"a missing snapshot":        {fmt.Errorf("ledger: %w", fs.ErrNotExist), kindLedgerUnavailable, ""},
		"an unreadable snapshot":    {fmt.Errorf("ledger: %w", fs.ErrPermission), kindLedgerUnavailable, ""},
		"an unsupported snapshot":   {&ledger.UnsupportedVersionError{Found: 9}, kindLedgerUnavailable, ""},
		"a transaction that failed": {&authored.TransactionError{}, kindInternal, ""},
		"a handler that panicked":   {errHandlerPanicked, kindInternal, ""},
		"anything else":             {errors.New("boom"), kindInternal, ""},
	}
	for label, tc := range cases {
		kind, reason := classify(tc.err)
		if kind != tc.kind || reason != tc.reason {
			t.Errorf("%s: classified as %s/%q, want %s/%q", label, kind, reason, tc.kind, tc.reason)
		}
	}
}
