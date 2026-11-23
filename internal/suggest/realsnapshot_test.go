package suggest_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/api"
	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/suggest"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// Opt-in verification over a real ledger snapshot: takes the snapshot into a
// scratch database in a temporary directory, builds every model request without
// sending anything anywhere, verifies no document-shaped value and no transfer
// to a person enters any request, and logs counts only.
func TestRealSnapshotRequestsContainNoDocumentsOrPersons(t *testing.T) {
	path := os.Getenv("FINWISE_LEDGER_SNAPSHOT")
	if path == "" {
		t.Skip("FINWISE_LEDGER_SNAPSHOT is not set")
	}
	ctx := t.Context()
	reader := ledger.New(path)

	// Read the snapshot once to locate the latest listed day and identify all
	// rows whose other side is a person.
	view := reader.View()
	accounts, err := view.Accounts(ctx)
	failReal(t, "accounts", err)
	identities, err := view.Identities(ctx)
	failReal(t, "identities", err)
	rows, err := view.CleanTransactions(ctx, brday.Period{First: "1970-01-01", Last: "2999-12-31"})
	failReal(t, "transactions", err)
	view.Close()

	interpreted := txrules.Interpret(rows, txrules.Reference{Accounts: accounts, Identities: identities})
	if len(interpreted.Listed) == 0 {
		t.Fatal("the snapshot lists no transactions")
	}
	var latest brday.Day
	personKeys := map[string]bool{}
	personProviderIDs := map[string]bool{}
	for _, listed := range interpreted.Listed {
		if listed.Day > latest {
			latest = listed.Day
		}
		if listed.Person {
			personKeys[listed.EstablishmentKey] = true
			personProviderIDs[listed.ID] = true
		}
	}
	parsedDay, err := time.Parse(time.DateOnly, string(latest))
	failReal(t, "parse latest day", err)
	now := parsedDay.Add(15 * time.Hour)
	month := brday.MonthOf(now)

	dbPath := filepath.Join(t.TempDir(), "finwise.db")
	db, err := authored.Open(ctx, dbPath, authored.Options{Now: func() time.Time { return now }})
	failReal(t, "open scratch database", err)
	defer db.Close()

	err = db.Change(ctx, authored.ChangeFacts{Author: authored.AuthorPerson, Method: "budget.save", At: now}, func(recorder *authored.Recorder) error {
		return recorder.SaveBudget(ctx, month, authoredtest.InventedBudget())
	})
	failReal(t, "save scratch budget", err)

	ingestCounts, err := ingest.Run(ctx, ingest.Sources{Ledger: reader, Database: db}, ingest.TriggerStart, now)
	failReal(t, "ingest", err)

	ledgerView := reader.View()
	defer ledgerView.Close()
	authoredView := db.View()
	defer authoredView.Close()

	candidates, err := api.ReadSuggestionCandidates(ctx, api.Views{Ledger: ledgerView, Authored: authoredView})
	failReal(t, "read candidates", err)

	// Map tx_keys that belong to person rows so we can assert none were picked.
	indexed, err := authoredView.IndexedTransactions(ctx)
	failReal(t, "read index", err)
	personTxKeys := map[authored.TxKey]bool{}
	for _, row := range indexed {
		if personKeys[row.EstablishmentKey] {
			personTxKeys[row.Key] = true
		}
		for _, pid := range row.ProviderIDs {
			if personProviderIDs[pid] {
				personTxKeys[row.Key] = true
			}
		}
	}

	requests, err := suggest.BuildRequests("dry-run", candidates)
	failReal(t, "build requests", err)

	rawDigits8 := regexp.MustCompile(`\d{8,}`)
	cpfPattern := regexp.MustCompile(`\d{3}\.\d{3}\.\d{3}-\d{2}`)
	cnpjPattern := regexp.MustCompile(`\d{2}\.\d{3}\.\d{3}/\d{4}-\d{2}`)

	totalPurchases := 0
	totalExamples := 0
	for _, req := range requests {
		for _, p := range req.Purchases {
			if personTxKeys[p.Key] {
				t.Error("a purchase whose other side is a person entered a request")
			}
		}
		if rawDigits8.Match(req.Body) || cpfPattern.Match(req.Body) || cnpjPattern.Match(req.Body) {
			t.Error("a request body contains a document-shaped value or 8+ consecutive digits")
		}
		var chat suggest.ChatRequest
		failReal(t, "decode chat request", json.Unmarshal(req.Body, &chat))
		var payload suggest.UserPayload
		failReal(t, "decode user payload", json.Unmarshal([]byte(chat.Messages[1].Content), &payload))
		totalPurchases += len(payload.Purchases)
		totalExamples += len(payload.Examples)
		for _, p := range payload.Purchases {
			if suggest.ContainsSensitiveRun(p.Description) {
				t.Error("a purchase description contains a sensitive digit run")
			}
		}
		for _, ex := range payload.Examples {
			if suggest.ContainsSensitiveRun(ex.Establishment) {
				t.Error("an example establishment contains a sensitive digit run")
			}
		}
	}

	t.Logf("indexed=%d pending_new=%d pending_person=%d months=%d requests=%d purchases=%d examples=%d",
		ingestCounts.Indexed, ingestCounts.PendingNewEstablishment, ingestCounts.PendingUnnamedPerson,
		len(candidates.Months), len(requests), totalPurchases, totalExamples)
}

func failReal(t *testing.T, step string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s failed (%T); message withheld", step, err)
	}
}
