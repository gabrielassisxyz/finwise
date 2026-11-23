package suggest_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/classrules"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	"github.com/gabrielassisxyz/finwise/internal/suggest"
)

var scopeNow = time.Date(2026, time.March, 20, 15, 0, 0, 0, time.UTC)

// modelAnswering is a fake model that records every request body it receives
// and answers each purchase with the line Mercado.
type modelAnswering struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newModelAnswering(t *testing.T) *modelAnswering {
	t.Helper()
	model := &modelAnswering{}
	model.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		model.mu.Lock()
		model.bodies = append(model.bodies, string(body))
		model.mu.Unlock()
		_, _ = w.Write(chatReply(t, `{"suggestions":[{"n":1,"line":"Mercado","confidence":80,"justification":"Compra de loja."}]}`))
	}))
	t.Cleanup(model.Close)
	return model
}

func (model *modelAnswering) requests() []string {
	model.mu.Lock()
	defer model.mu.Unlock()
	return append([]string(nil), model.bodies...)
}

// scopeWorld is an invented ledger, one card purchase at a new establishment
// and whatever else the test adds, taken in by the ingest over the invented
// budget of March.
func scopeWorld(t *testing.T, add func(*ledgertest.Fixture)) suggest.Sources {
	t.Helper()
	fixture := ledgertest.New(t)
	fixture.AddItem(ledgertest.Item{ID: "item-1", Connector: "Banco", Status: "UPDATED", ExecutionStatus: "SUCCESS"})
	fixture.AddAccount(ledgertest.Account{ID: "acc-bank", ItemID: "item-1", Type: "BANK", Subtype: "CHECKING_ACCOUNT", Name: "Conta", Currency: "BRL", Balance: 900})
	fixture.AddAccount(ledgertest.Account{ID: "acc-card", ItemID: "item-1", Type: "CREDIT", Subtype: "CREDIT_CARD", Name: "Cartão", Currency: "BRL", Balance: 400})
	if add != nil {
		add(fixture)
	}
	db := authoredtest.Fresh(t, func() time.Time { return scopeNow })
	authoredtest.SaveBudget(t, db, scopeNow, "2026-03", authoredtest.InventedBudget())
	sources := suggest.Sources{Ledger: ledger.New(fixture.Path), Database: db}
	if _, err := ingest.Run(t.Context(), ingest.Sources{Ledger: sources.Ledger, Database: db}, ingest.TriggerStart, scopeNow); err != nil {
		t.Fatal(err)
	}
	return sources
}

func newEstablishmentPurchase(fixture *ledgertest.Fixture) {
	fixture.AddTransaction(ledgertest.Transaction{
		ID: "tx-purchase", AccountID: "acc-card", Date: time.Date(2026, time.March, 12, 15, 0, 0, 0, time.UTC),
		Description: "LIVRARIA NOVA", Amount: 40, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO"},
	})
}

func TestACreditAtANewEstablishmentIsQueuedByTheIngestAndNeverReachesTheModel(t *testing.T) {
	sources := scopeWorld(t, func(fixture *ledgertest.Fixture) {
		newEstablishmentPurchase(fixture)
		fixture.AddTransaction(ledgertest.Transaction{
			ID: "tx-credit", AccountID: "acc-bank", Date: time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC),
			Description: "CREDITO ESTUDIO SOL", Amount: 250, Currency: "BRL", Type: "CREDIT",
			Raw: map[string]any{"operationType": "TRANSFERENCIA", "paymentData": map[string]any{
				"paymentMethod": "TED",
				"payer":         map[string]any{"name": "Estudio Sol Ltda", "documentNumber": map[string]any{"type": "CNPJ", "value": "55566677000188"}},
			}},
		})
	})

	view := sources.Database.View()
	keys, err := view.TransactionKeys(t.Context(), []string{"tx-purchase", "tx-credit"})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := view.Transactions(t.Context(), []authored.TxKey{keys["tx-purchase"], keys["tx-credit"]})
	view.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"tx-purchase", "tx-credit"} {
		if got := kept[keys[id]].State.PendingReason; got != string(classrules.ReasonNewEstablishment) {
			t.Fatalf("%s waits as %q after the ingest, want %q", id, got, classrules.ReasonNewEstablishment)
		}
	}

	model := newModelAnswering(t)
	counts, err := suggest.Run(t.Context(), sources, suggest.Config{URL: model.URL, Model: "model-x"}, func() time.Time { return scopeNow })
	if err != nil {
		t.Fatal(err)
	}
	if counts.Asked != 1 {
		t.Errorf("counts = %+v, want only the purchase asked", counts)
	}
	requests := model.requests()
	if len(requests) != 1 || !strings.Contains(requests[0], "LIVRARIA NOVA") {
		t.Fatalf("requests = %q, want one carrying the purchase", requests)
	}
	if strings.Contains(requests[0], "ESTUDIO SOL") {
		t.Errorf("the credit reached the model: %s", requests[0])
	}
}
