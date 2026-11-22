package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// pendingReviewsOver is the authored state of the third read case with three
// more reviews on rows it leaves alone: one with no line to propose, one whose
// line the month's budget lacks, and one with a line and no confidence.
func pendingReviewsOver(t *testing.T) *readEnv {
	t.Helper()
	scratch, full := newAuthoredStores(t)
	lines := lineIDsByName(t, scratch.Database)
	authorOver(t, scratch, viewOf(t, full.Path), lines, laterCorrection.Add(24*time.Hour), "scratch.pendingWithoutSuggestion", func(a authoring) error {
		return each(
			func() error {
				return a.state(txUnconverted, authored.TransactionState{PendingReason: "Estabelecimento novo"})
			},
			func() error {
				return a.state(txBankBill, authored.TransactionState{
					PendingReason: "Baixa confiança", PendingLine: a.line(plannedLine), PendingConfidence: authoredtest.Some[int64](70),
				})
			},
			func() error {
				return a.state(txOwnTransfer, authored.TransactionState{PendingReason: "Baixa confiança", PendingLine: a.line("Mercado")})
			},
		)
	})
	return newReadEnvOver(t, full, scratch.Database)
}

var pendingReviewsExpected = map[string]map[string]any{
	txUnconverted: {"reason": "Estabelecimento novo", "suggestion": nil, "confidence": nil, "justification": nil},
	// The line exists, but not in the month of the transaction: there is no
	// name to propose, so the confidence has nothing to be about either.
	txBankBill: {"reason": "Baixa confiança", "suggestion": nil, "confidence": nil, "justification": nil},
	// The line is kept by its id, so it is named as the month's budget names it.
	txOwnTransfer: {"reason": "Baixa confiança", "suggestion": "Feira", "confidence": nil, "justification": nil},
	txBoleto:      {"reason": "Confiança abaixo do limite", "suggestion": "Contas da casa", "confidence": float64(64), "justification": nil},
}

func TestAPendingReviewAnswersNullForWhatTheAppDoesNotKnowInTheListAndTheDetail(t *testing.T) {
	env := pendingReviewsOver(t)

	status, body := env.answer("transactions.list", listRaw("2026-03", "month"))
	if status != http.StatusOK {
		t.Fatalf("transactions.list answered %d", status)
	}
	if problem := conformanceProblem("transactions.list", body); problem != "" {
		t.Errorf("transactions.list does not conform: %s", problem)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatal(err)
	}
	for id, want := range pendingReviewsExpected {
		if got := rowWithIDIn(t, rows, id)["pending"]; !reflect.DeepEqual(got, any(want)) {
			t.Errorf("transactions.list %s: pending = %v, want %v", id, got, want)
		}
		status, detail := env.answer("transactions.detail", `["`+id+`"]`)
		if status != http.StatusOK {
			t.Fatalf("transactions.detail %s answered %d", id, status)
		}
		if problem := conformanceProblem("transactions.detail", detail); problem != "" {
			t.Errorf("transactions.detail %s does not conform: %s", id, problem)
		}
		if got := decodeAnswer(t, detail)["pending"]; !reflect.DeepEqual(got, any(want)) {
			t.Errorf("transactions.detail %s: pending = %v, want %v", id, got, want)
		}
	}
}

func TestAPendingReviewWithNoSuggestionWritesNullAndNotAnEmptyName(t *testing.T) {
	env := pendingReviewsOver(t)
	_, body := env.answer("transactions.detail", `["`+txUnconverted+`"]`)
	want := `"pending":{"reason":"Estabelecimento novo","suggestion":null,"confidence":null,"justification":null}`
	if !strings.Contains(string(body), want) {
		t.Errorf("the answer does not hold %s:\n%s", want, body)
	}
}
