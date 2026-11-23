package suggest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/api"
	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/suggest"
)

func TestScrubTextReplacesDocumentsAndLongDigitRuns(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"PADARIA 111.222.333-44 CENTRO", "PADARIA [número] CENTRO"},
		{"PIX 11122233344 LOJA", "PIX [número] LOJA"},
		{"ALVORADA 12.345.678/0001-95.", "ALVORADA [número]."},
		{"FORNECEDOR 12345678000195", "FORNECEDOR [número]"},
		{"CARTAO 5555 6666 7777 8888", "CARTAO [número]"},
		{"CONTA 0001/12345-6", "CONTA [número]"},
		{"PEDIDO 12345678", "PEDIDO [número]"},
		{"PEDIDO 1234-5678", "PEDIDO [número]"},
		{"PEDIDO 12.34.56.78", "PEDIDO [número]"},
		{"PEDIDO 1234567", "PEDIDO 1234567"},
		{"LIVRARIA EXEMPLO 1/3", "LIVRARIA EXEMPLO 1/3"},
		{"FARMACIA 24H", "FARMACIA 24H"},
		{"12345678901", "[número]"},
	}
	for _, c := range cases {
		if got := suggest.ScrubText(c.in); got != c.want {
			t.Errorf("ScrubText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	for _, empty := range []string{"[número]", " [número] - [número] ", "   "} {
		if !suggest.IsEmptyExampleName(empty) {
			t.Errorf("IsEmptyExampleName(%q) = false, want true", empty)
		}
	}
	for _, kept := range []string{"PADARIA [número]", "99", "7-ELEVEN"} {
		if suggest.IsEmptyExampleName(kept) {
			t.Errorf("IsEmptyExampleName(%q) = true, want false", kept)
		}
	}
}

func TestBuildRequestsMatchesGoldenByteForByte(t *testing.T) {
	candidates := api.SuggestionCandidates{
		Months: []api.SuggestionMonth{
			{
				Month: "2026-03",
				Lines: []api.SuggestionLine{
					{ID: 2, Name: "Mercado"},
					{ID: 3, Name: "Delivery"},
					{ID: 1, Name: "Tarifas e juros"},
				},
				Purchases: []api.SuggestionPurchase{
					{Key: 10, Month: "2026-03", Description: "PADARIA SOL 12.345.678/0001-95", Cents: -1850, Day: "2026-03-14"},
					{Key: 11, Month: "2026-03", Description: "IFOOD PEDIDO 98765432", Cents: -6490, Day: "2026-03-15"},
				},
			},
		},
		Examples: []api.SuggestionExample{
			{Establishment: "12345678901234", LineID: 2, Line: "Mercado"},
			{Establishment: "Supermercado Bairro 12345678", LineID: 2, Line: "Mercado"},
			{Establishment: "Hamburgueria Exemplo", LineID: 3, Line: "Delivery"},
		},
	}
	requests, err := suggest.BuildRequests("test-model-v1", candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(requests))
	}
	goldenPath := filepath.Join("testdata", "request.golden.json")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	got := append(bytes.Clone(requests[0].Body), '\n')
	if !bytes.Equal(got, want) {
		t.Fatalf("request body differs from %s:\ngot:\n%s\nwant:\n%s", goldenPath, got, want)
	}
}

func TestBuildRequestsSplitsAtFortyPurchasesPerRequest(t *testing.T) {
	purchases := make([]api.SuggestionPurchase, 85)
	for i := range purchases {
		purchases[i] = api.SuggestionPurchase{
			Key:         authored.TxKey(i + 1),
			Month:       "2026-03",
			Description: fmt.Sprintf("LOJA %d", i+1),
			Cents:       -1000,
			Day:         "2026-03-10",
		}
	}
	requests, err := suggest.BuildRequests("test-model", api.SuggestionCandidates{
		Months: []api.SuggestionMonth{{
			Month:     "2026-03",
			Lines:     []api.SuggestionLine{{ID: 2, Name: "Mercado"}},
			Purchases: purchases,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("85 purchases produced %d requests, want 3", len(requests))
	}
	wantSizes := []int{40, 40, 5}
	for i, req := range requests {
		if len(req.Purchases) != wantSizes[i] {
			t.Errorf("batch %d has %d purchases, want %d", i, len(req.Purchases), wantSizes[i])
		}
		for j, p := range req.Purchases {
			if p.N != j+1 {
				t.Errorf("batch %d item %d has n=%d, want %d", i, j, p.N, j+1)
			}
		}
	}
}

func TestParseResponseValidatesEveryFieldIndependently(t *testing.T) {
	req := suggest.Request{
		Month: "2026-03",
		Purchases: []suggest.RequestPurchase{
			{N: 1, Key: 101}, // valid
			{N: 2, Key: 102}, // line the month lacks
			{N: 3, Key: 103}, // confidence 101
			{N: 4, Key: 104}, // confidence -1
			{N: 5, Key: 105}, // confidence 82.5
			{N: 6, Key: 106}, // blank justification
			{N: 7, Key: 107}, // 201-rune justification
			{N: 8, Key: 108}, // repeated n
			{N: 9, Key: 109}, // omitted
			{N: 10, Key: 110}, // valid with exact 200-rune Portuguese justification and confidence 0
		},
		LineIDs: map[string]authored.LineID{
			"Mercado":  2,
			"Delivery": 3,
		},
	}
	just200 := strings.Repeat("á", 200)
	just201 := strings.Repeat("é", 201)
	content := fmt.Sprintf("```json\n"+`{
		"suggestions": [
			{"n": 1, "line": "Mercado", "confidence": 82, "justification": "  Padaria de bairro  "},
			{"n": 2, "line": "Inexistente", "confidence": 80, "justification": "Linha fora do orçamento"},
			{"n": 3, "line": "Mercado", "confidence": 101, "justification": "Acima do limite"},
			{"n": 4, "line": "Mercado", "confidence": -1, "justification": "Negativo"},
			{"n": 5, "line": "Mercado", "confidence": 82.5, "justification": "Decimal"},
			{"n": 6, "line": "Mercado", "confidence": 75, "justification": "   "},
			{"n": 7, "line": "Mercado", "confidence": 75, "justification": %q},
			{"n": 8, "line": "Mercado", "confidence": 80, "justification": "Primeira"},
			{"n": 8, "line": "Delivery", "confidence": 90, "justification": "Segunda repetida"},
			{"n": 99, "line": "Mercado", "confidence": 90, "justification": "Desconhecido"},
			{"n": 10, "line": "Delivery", "confidence": 0, "justification": %q}
		]
	}`+"\n```", just201, just200)

	raw := chatReply(t, content)
	outcome, err := suggest.ParseResponse(raw, req)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}
	if outcome.Suggested != 2 || outcome.Unusable != 8 || len(outcome.Rows) != 10 {
		t.Fatalf("outcome counts = suggested:%d unusable:%d rows:%d, want 2/8/10", outcome.Suggested, outcome.Unusable, len(outcome.Rows))
	}
	if row := outcome.Rows[0]; row.Line == nil || *row.Line != 2 || *row.Confidence != 82 || *row.Justification != "Padaria de bairro" {
		t.Errorf("row 1 = %+v", row)
	}
	for idx := 1; idx <= 8; idx++ {
		if row := outcome.Rows[idx]; row.Line != nil || row.Confidence != nil || row.Justification != nil {
			t.Errorf("row %d should be unusable, got %+v", idx+1, row)
		}
	}
	if row := outcome.Rows[9]; row.Line == nil || *row.Line != 3 || *row.Confidence != 0 || *row.Justification != just200 {
		t.Errorf("row 10 = %+v", row)
	}
}

func TestParseResponseRefusesMalformedReplies(t *testing.T) {
	req := suggest.Request{
		Month:     "2026-03",
		Purchases: []suggest.RequestPurchase{{N: 1, Key: 1}},
		LineIDs:   map[string]authored.LineID{"Mercado": 2},
	}
	badEnvelopes := map[string][]byte{
		"not json":            []byte("not json"),
		"no choices":          []byte(`{"choices":[]}`),
		"null content":        []byte(`{"choices":[{"message":{"content":null}}]}`),
		"plain text content":  chatReply(t, "Desculpe, não consegui classificar."),
		"array not object":    chatReply(t, `[{"n":1}]`),
		"missing suggestions": chatReply(t, `{"other":[]}`),
		"null suggestions":    chatReply(t, `{"suggestions":null}`),
		"trailing json":       chatReply(t, `{"suggestions":[]}{}`),
		"unclosed fence":      chatReply(t, "```json\n{\"suggestions\":[]}"),
	}
	for name, raw := range badEnvelopes {
		if _, err := suggest.ParseResponse(raw, req); err == nil {
			t.Errorf("%s: expected malformed error, got nil", name)
		}
	}
}

func TestExamplesCappedAt100NewestPersonDecisionsWithoutPersons(t *testing.T) {
	now := time.Date(2026, time.May, 1, 15, 0, 0, 0, time.UTC)
	fixture := ledgertest.New(t)
	fixture.AddItem(ledgertest.Item{ID: "item-1", Connector: "Banco", Status: "UPDATED", ExecutionStatus: "SUCCESS"})
	fixture.AddAccount(ledgertest.Account{ID: "card-1", ItemID: "item-1", Type: "CREDIT", Subtype: "CREDIT_CARD", Name: "Cartão", Currency: "BRL", Balance: 100})
	fixture.AddAccount(ledgertest.Account{ID: "bank-1", ItemID: "item-1", Type: "BANK", Subtype: "CHECKING_ACCOUNT", Name: "Conta", Currency: "BRL", Balance: 500})

	// 120 establishments + 1 person transfer + 1 rule-classified establishment + 1 pending purchase.
	baseDay := time.Date(2026, time.January, 1, 15, 0, 0, 0, time.UTC)
	for i := 1; i <= 120; i++ {
		fixture.AddTransaction(ledgertest.Transaction{
			ID: fmt.Sprintf("tx-est-%03d", i), AccountID: "card-1",
			Date:        baseDay.AddDate(0, 0, i-1),
			Description: fmt.Sprintf("LOJA %03d", i),
			Amount:      10, Currency: "BRL", Type: "DEBIT",
			Raw: map[string]any{"operationType": "PAGAMENTO"},
		})
	}
	fixture.AddTransaction(ledgertest.Transaction{
		ID: "tx-person-1", AccountID: "bank-1",
		Date:        time.Date(2026, time.May, 1, 12, 0, 0, 0, time.UTC),
		Description: "PIX JOAO EXEMPLO", Amount: -50, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PIX", "paymentData": map[string]any{
			"paymentMethod": "PIX",
			"receiver":      map[string]any{"name": "João Exemplo", "documentNumber": map[string]any{"type": "CPF", "value": "111.222.333-44"}},
		}},
	})
	fixture.AddTransaction(ledgertest.Transaction{
		ID: "tx-rule-1", AccountID: "card-1",
		Date:        time.Date(2026, time.May, 1, 12, 0, 0, 0, time.UTC),
		Description: "ESTABELECIMENTO REGRA", Amount: 20, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO"},
	})
	fixture.AddTransaction(ledgertest.Transaction{
		ID: "tx-pending-1", AccountID: "card-1",
		Date:        time.Date(2026, time.May, 1, 13, 0, 0, 0, time.UTC),
		Description: "NOVA PADARIA", Amount: 15, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO"},
	})

	db := authoredtest.Fresh(t, func() time.Time { return time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC) })
	authoredtest.SaveBudget(t, db, now, "2026-01", authoredtest.InventedBudget())
	view := db.View()
	budget, err := view.BudgetOf(t.Context(), "2026-01")
	view.Close()
	if err != nil {
		t.Fatal(err)
	}
	lineID := budget.Lines[0].ID

	if err := db.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorPerson, Method: "test.seed", At: now}, func(recorder *authored.Recorder) error {
		for i := 1; i <= 120; i++ {
			day := brday.DayOf(baseDay.AddDate(0, 0, i-1))
			key, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen(fmt.Sprintf("tx-est-%03d", i), day, -1000, fmt.Sprintf("text:LOJA %03d", i)))
			if err != nil {
				return err
			}
			if err := recorder.SettleLine(t.Context(), key, lineID, authored.AuthorPerson); err != nil {
				return err
			}
		}
		personKey, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen("tx-person-1", "2026-05-01", -5000, "doc:11122233344"))
		if err != nil {
			return err
		}
		if err := recorder.SettleLine(t.Context(), personKey, lineID, authored.AuthorPerson); err != nil {
			return err
		}
		ruleKey, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen("tx-rule-1", "2026-05-01", -2000, "text:ESTABELECIMENTO REGRA"))
		if err != nil {
			return err
		}
		if err := recorder.SettleLine(t.Context(), ruleKey, lineID, authored.AuthorRule); err != nil {
			return err
		}
		pendingKey, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen("tx-pending-1", "2026-05-01", -1500, "text:NOVA PADARIA"))
		if err != nil {
			return err
		}
		return recorder.SetPendingReview(t.Context(), pendingKey, "Estabelecimento novo", nil, nil)
	}); err != nil {
		t.Fatal(err)
	}

	lv := ledger.New(fixture.Path).View()
	defer lv.Close()
	av := db.View()
	defer av.Close()
	candidates, err := api.ReadSuggestionCandidates(t.Context(), api.Views{Ledger: lv, Authored: av})
	if err != nil {
		t.Fatal(err)
	}
	requests, err := suggest.BuildRequests("test-model", candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(requests))
	}
	payload := decodeUserPayload(t, requests[0].Body)
	if len(payload.Examples) != 100 {
		t.Fatalf("got %d examples, want 100", len(payload.Examples))
	}
	if payload.Examples[0].Establishment != "LOJA 120" || payload.Examples[99].Establishment != "LOJA 021" {
		t.Errorf("first=%q last=%q, want LOJA 120 .. LOJA 021", payload.Examples[0].Establishment, payload.Examples[99].Establishment)
	}
	for _, ex := range payload.Examples {
		if strings.Contains(ex.Establishment, "João") || strings.Contains(ex.Establishment, "REGRA") {
			t.Errorf("disallowed example leaked: %+v", ex)
		}
	}
}

func TestRunOverFakeServerStoresRowsScrubsAndHandlesFailures(t *testing.T) {
	now := time.Date(2026, time.March, 20, 15, 0, 0, 0, time.UTC)
	fixedNow := func() time.Time { return now }

	// Build an invented ledger with two months of pending purchases, planted
	// CPF/CNPJ/digit runs, a note, a tag, a balance, an account number, and a
	// pending transfer to a person.
	newWorld := func(t *testing.T) (suggest.Sources, string) {
		t.Helper()
		fixture := ledgertest.New(t)
		fixture.AddItem(ledgertest.Item{ID: "item-1", Connector: "Banco", Status: "UPDATED", ExecutionStatus: "SUCCESS"})
		fixture.AddAccount(ledgertest.Account{ID: "acc-bank", ItemID: "item-1", Type: "BANK", Subtype: "CHECKING_ACCOUNT", Name: "Conta", Currency: "BRL", Balance: 9876.54, TransferNumber: "341/0001/99988-7"})
		fixture.AddAccount(ledgertest.Account{ID: "acc-card", ItemID: "item-1", Type: "CREDIT", Subtype: "CREDIT_CARD", Name: "Cartão", Currency: "BRL", Balance: 4321.10})
		fixture.AddTransaction(ledgertest.Transaction{
			ID: "tx-feb-1", AccountID: "acc-card", Date: time.Date(2026, time.February, 10, 15, 0, 0, 0, time.UTC),
			Description: "PADARIA 111.222.333-44 PED 87654321", Amount: 25.50, Currency: "BRL", Type: "DEBIT",
			Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "5555666677778888"}},
		})
		fixture.AddTransaction(ledgertest.Transaction{
			ID: "tx-mar-1", AccountID: "acc-card", Date: time.Date(2026, time.March, 12, 15, 0, 0, 0, time.UTC),
			Description: "MERCADO 12.345.678/0001-95", Amount: 140.00, Currency: "BRL", Type: "DEBIT",
			Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "5555666677778888"}},
		})
		fixture.AddTransaction(ledgertest.Transaction{
			ID: "tx-mar-person", AccountID: "acc-bank", Date: time.Date(2026, time.March, 14, 15, 0, 0, 0, time.UTC),
			Description: "PIX FULANA SECRETA", Amount: -80.00, Currency: "BRL", Type: "DEBIT",
			Raw: map[string]any{"operationType": "PIX", "paymentData": map[string]any{
				"paymentMethod": "PIX",
				"receiver":      map[string]any{"name": "Fulana Secreta", "documentNumber": map[string]any{"type": "CPF", "value": "999.888.777-66"}},
			}},
		})
		fixture.AddTransaction(ledgertest.Transaction{
			ID: "tx-ex-1", AccountID: "acc-card", Date: time.Date(2026, time.February, 5, 15, 0, 0, 0, time.UTC),
			Description: "FARMACIA 12.345.678/0001-95 CENTRO", Amount: 40.00, Currency: "BRL", Type: "DEBIT",
			Raw: map[string]any{"operationType": "PAGAMENTO"},
		})

		db := authoredtest.Fresh(t, func() time.Time { return time.Date(2026, time.February, 1, 12, 0, 0, 0, time.UTC) })
		authoredtest.SaveBudget(t, db, now, "2026-02", authoredtest.InventedBudget())
		view := db.View()
		budget, _ := view.BudgetOf(t.Context(), "2026-02")
		view.Close()
		var mercadoID authored.LineID
		for _, line := range budget.Lines {
			if line.Name == "Mercado" {
				mercadoID = line.ID
			}
		}
		if err := db.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorPerson, Method: "test.seed", At: now}, func(recorder *authored.Recorder) error {
			exKey, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen("tx-ex-1", "2026-02-05", -4000, "text:FARMACIA 12.345.678/0001-95 CENTRO"))
			if err != nil {
				return err
			}
			if err := recorder.SettleLine(t.Context(), exKey, mercadoID, authored.AuthorPerson); err != nil {
				return err
			}
			if err := recorder.SetNote(t.Context(), exKey, "NOTA-PRIVADA-SECRETA"); err != nil {
				return err
			}
			if err := recorder.AddTag(t.Context(), exKey, "tag-privada-secreta"); err != nil {
				return err
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		sources := suggest.Sources{Ledger: ledger.New(fixture.Path), Database: db}
		if _, err := ingest.Run(t.Context(), ingest.Sources{Ledger: sources.Ledger, Database: db}, ingest.TriggerStart, now); err != nil {
			t.Fatal(err)
		}
		return sources, fixture.Path
	}

	t.Run("good answers per month, scrubbed, and asked once", func(t *testing.T) {
		sources, _ := newWorld(t)
		var seenAuth []string
		var seenBodies [][]byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}
			seenAuth = append(seenAuth, r.Header.Get("Authorization"))
			body, _ := io.ReadAll(r.Body)
			seenBodies = append(seenBodies, body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(chatReply(t, `{"suggestions":[{"n":1,"line":"Mercado","confidence":84,"justification":"Compra de supermercado ou padaria."}]}`))
		}))
		defer server.Close()

		counts, err := suggest.Run(t.Context(), sources, suggest.Config{
			URL: server.URL + "/v1", Model: "model-x", Key: "secret-key",
		}, fixedNow)
		if err != nil {
			t.Fatal(err)
		}
		if counts.Requests != 2 || counts.Asked != 2 || counts.Suggested != 2 || counts.Unusable != 0 {
			t.Errorf("counts = %+v, want 2/2/2/0", counts)
		}
		for _, auth := range seenAuth {
			if auth != "Bearer secret-key" {
				t.Errorf("Authorization = %q, want Bearer secret-key", auth)
			}
		}
		digits8 := regexp.MustCompile(`\d{8,}`)
		for i, body := range seenBodies {
			text := string(body)
			for _, forbidden := range []string{
				"111.222.333-44", "12.345.678/0001-95", "87654321", "999.888.777-66",
				"5555666677778888", "99988-7", "Fulana", "tx-feb-1", "tx-mar-1", "tx-ex-1",
				"NOTA-PRIVADA-SECRETA", "tag-privada-secreta", "9876.54", "4321.10",
			} {
				if strings.Contains(text, forbidden) {
					t.Errorf("request %d leaked %q", i, forbidden)
				}
			}
			if match := digits8.FindString(text); match != "" {
				t.Errorf("request %d holds 8+ consecutive digits %q", i, match)
			}
			payload := decodeUserPayload(t, body)
			for _, p := range payload.Purchases {
				if suggest.ContainsSensitiveRun(p.Description) {
					t.Errorf("purchase description still contains sensitive run: %q", p.Description)
				}
			}
			for _, ex := range payload.Examples {
				if suggest.ContainsSensitiveRun(ex.Establishment) {
					t.Errorf("example establishment still contains sensitive run: %q", ex.Establishment)
				}
			}
		}

		// Meta records successful run.
		view := sources.Database.View()
		run, found, err := view.LastModelRun(t.Context())
		view.Close()
		if err != nil || !found || !run.OK || !run.At.Equal(now) {
			t.Errorf("last model run = %+v found=%v err=%v", run, found, err)
		}

		// A second run asks nothing and sends 0 requests.
		beforeCalls := len(seenBodies)
		secondCounts, err := suggest.Run(t.Context(), sources, suggest.Config{
			URL: server.URL + "/v1", Model: "model-x",
		}, func() time.Time { return now.Add(time.Hour) })
		if err != nil || secondCounts != (suggest.Counts{}) || len(seenBodies) != beforeCalls {
			t.Errorf("second run: counts=%+v err=%v calls=%d", secondCounts, err, len(seenBodies)-beforeCalls)
		}
	})

	t.Run("malformed on second request keeps first request stored and marks meta failed", func(t *testing.T) {
		sources, _ := newWorld(t)
		var call atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if call.Add(1) == 1 {
				_, _ = w.Write(chatReply(t, `{"suggestions":[{"n":1,"line":"Mercado","confidence":80,"justification":"Padaria."}]}`))
				return
			}
			_, _ = w.Write(chatReply(t, `not valid json`))
		}))
		defer server.Close()

		counts, err := suggest.Run(t.Context(), sources, suggest.Config{URL: server.URL, Model: "model-x"}, fixedNow)
		if kind, ok := suggest.FailureOf(err); !ok || kind != suggest.FailureMalformed {
			t.Fatalf("err = %v, want FailureMalformed", err)
		}
		if counts.Requests != 1 || counts.Suggested != 1 {
			t.Errorf("counts = %+v, want 1 request stored before failure", counts)
		}
		view := sources.Database.View()
		keys, _ := view.TransactionKeys(t.Context(), []string{"tx-feb-1", "tx-mar-1"})
		stored, _ := view.ModelSuggestions(t.Context(), []authored.TxKey{keys["tx-feb-1"], keys["tx-mar-1"]})
		run, found, _ := view.LastModelRun(t.Context())
		view.Close()
		if _, ok := stored[keys["tx-feb-1"]]; !ok {
			t.Error("first month suggestion was not kept")
		}
		if _, ok := stored[keys["tx-mar-1"]]; ok {
			t.Error("second month failed request should have no stored row")
		}
		if !found || run.OK {
			t.Errorf("meta run = %+v found=%v, want failed run recorded", run, found)
		}
	})

	t.Run("status 500, timeout and unreachable leave no rows and mark meta failed", func(t *testing.T) {
		statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer statusServer.Close()

		slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
		}))
		defer slowServer.Close()

		closedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		closedURL := closedServer.URL
		closedServer.Close()

		cases := []struct {
			name    string
			url     string
			timeout time.Duration
			want    suggest.FailureKind
		}{
			{"500", statusServer.URL, 5 * time.Second, suggest.FailureStatus},
			{"timeout", slowServer.URL, 40 * time.Millisecond, suggest.FailureTimeout},
			{"unreachable", closedURL, 5 * time.Second, suggest.FailureUnreachable},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				sources, _ := newWorld(t)
				_, err := suggest.Run(t.Context(), sources, suggest.Config{URL: c.url, Model: "model-x", Timeout: c.timeout}, fixedNow)
				if kind, ok := suggest.FailureOf(err); !ok || kind != c.want {
					t.Fatalf("err = %v, want %v", err, c.want)
				}
				view := sources.Database.View()
				keys, _ := view.TransactionKeys(t.Context(), []string{"tx-feb-1", "tx-mar-1"})
				stored, _ := view.ModelSuggestions(t.Context(), []authored.TxKey{keys["tx-feb-1"], keys["tx-mar-1"]})
				run, found, _ := view.LastModelRun(t.Context())
				view.Close()
				if len(stored) != 0 {
					t.Errorf("failed run left %d rows", len(stored))
				}
				if !found || run.OK {
					t.Errorf("meta run = %+v found=%v, want ok=false", run, found)
				}
			})
		}
	})
}

func chatReply(t *testing.T, content string) []byte {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func decodeUserPayload(t *testing.T, body []byte) suggest.UserPayload {
	t.Helper()
	var req suggest.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(req.Messages))
	}
	var payload suggest.UserPayload
	if err := json.Unmarshal([]byte(req.Messages[1].Content), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

// Verify error wrapping does not expose underlying values.
func TestFailureErrorTextNamesOnlyKind(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := closed.URL
	closed.Close()

	fixture := ledgertest.New(t)
	fixture.AddItem(ledgertest.Item{ID: "item-1", Connector: "Banco", Status: "UPDATED", ExecutionStatus: "SUCCESS"})
	fixture.AddAccount(ledgertest.Account{ID: "card-1", ItemID: "item-1", Type: "CREDIT", Subtype: "CREDIT_CARD", Name: "Cartão", Currency: "BRL"})
	fixture.AddTransaction(ledgertest.Transaction{
		ID: "tx-1", AccountID: "card-1", Date: time.Date(2026, time.March, 10, 15, 0, 0, 0, time.UTC),
		Description: "SENTINEL-DESC", Amount: 10, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO"},
	})
	now := time.Date(2026, time.March, 20, 15, 0, 0, 0, time.UTC)
	db := authoredtest.Fresh(t, func() time.Time { return now })
	authoredtest.SaveBudget(t, db, now, "2026-03", authoredtest.InventedBudget())
	if _, err := ingest.Run(t.Context(), ingest.Sources{Ledger: ledger.New(fixture.Path), Database: db}, ingest.TriggerStart, now); err != nil {
		t.Fatal(err)
	}
	_, err := suggest.Run(t.Context(), suggest.Sources{Ledger: ledger.New(fixture.Path), Database: db}, suggest.Config{
		URL: url, Model: "model-x", Key: "SECRET-API-KEY",
	}, func() time.Time { return now })
	if err == nil || !errors.Is(err, err) {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "SENTINEL") || strings.Contains(err.Error(), "SECRET-API-KEY") || strings.Contains(err.Error(), url) {
		t.Errorf("error text leaked sensitive details: %q", err.Error())
	}
}
