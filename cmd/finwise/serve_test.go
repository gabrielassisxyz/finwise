package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// readAccounts unlocks the server and asks it for the accounts, which
// only the ledger can answer.
func readAccounts(t *testing.T, server *runningServer) (int, string) {
	t.Helper()
	response, _ := server.post(t, "/api/session/unlock", `["`+testPIN+`"]`)
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("unlock set %d cookies", len(cookies))
	}
	request, err := http.NewRequest(http.MethodPost, "http://"+server.address+"/api/accounts/list", strings.NewReader(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookies[0])
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer answer.Body.Close()
	body, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatal(err)
	}
	return answer.StatusCode, string(body)
}

func TestServeOpensBothFilesNamedRelativelyToTheWorkingDirectory(t *testing.T) {
	shapes := map[string][2]string{
		"in directories": {filepath.Join("data", "snapshot.db"), filepath.Join("db", "finwise.db")},
		"dot slash":      {"." + string(filepath.Separator) + filepath.Join("data", "snapshot.db"), "." + string(filepath.Separator) + "finwise.db"},
		"bare names":     {"snapshot.db", "finwise.db"},
	}
	for name, paths := range shapes {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			ledgerPath, databasePath := paths[0], paths[1]
			for _, path := range paths {
				if err := os.MkdirAll(filepath.Join(directory, filepath.Dir(path)), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			snapshot := ledgertest.NewAt(t, filepath.Join(directory, ledgerPath))
			snapshot.AddItem(ledgertest.Item{ID: "item-1", Connector: "Test Bank", Status: "UPDATED", ExecutionStatus: "SUCCESS"})
			snapshot.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: "Test Checking", Currency: "BRL", Balance: 10})
			t.Chdir(directory)

			args := []string{"serve", "--dev-insecure-cookie", "--ledger", ledgerPath, "--db", databasePath}
			vars := serveVars(t)
			delete(vars, "FINWISE_LEDGER")
			server := startServer(t, args, vars)

			status, body := readAccounts(t, server)
			if status != http.StatusOK || !strings.Contains(body, "Test Checking") {
				t.Errorf("accounts.list answered %d: %s", status, body)
			}
			server.stopGracefully(t)
			created := inspectDatabase(t, filepath.Join(directory, databasePath))
			if month := columnValues(t, created, "SELECT value FROM meta WHERE key = 'tracking_start_month'"); !slices.Equal(month, []string{"2026-11"}) {
				t.Errorf("the database was not created at %s: tracking start month %v", databasePath, month)
			}
		})
	}
}
