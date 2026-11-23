package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/suggest"
)

func runCommandWithSuggestTimeout(
	ctx context.Context,
	args []string,
	vars map[string]string,
	listening func(string),
	timeout time.Duration,
) outcome {
	var stdout, stderr bytes.Buffer
	code := run(ctx, args, environment{
		stdin:          strings.NewReader(""),
		stdout:         &stdout,
		stderr:         &stderr,
		getenv:         func(name string) string { return vars[name] },
		now:            func() time.Time { return testNow },
		listening:      listening,
		runIngest:      ingest.Run,
		runSuggest:     suggest.Run,
		suggestTimeout: timeout,
	})
	return outcome{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func startServerWithTimeout(t *testing.T, args []string, vars map[string]string, timeout time.Duration) *runningServer {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	server := &runningServer{done: make(chan outcome, 1), stop: stop}
	ready := make(chan string, 1)
	go func() {
		server.done <- runCommandWithSuggestTimeout(ctx, args, vars, func(address string) { ready <- address }, timeout)
	}()
	select {
	case server.address = <-ready:
	case result := <-server.done:
		t.Fatalf("serve exited %d before listening: %s", result.code, result.stderr)
	case <-time.After(30 * time.Second):
		t.Fatal("serve did not start")
	}
	t.Cleanup(stop)
	return server
}

func suggestRecords(t *testing.T, output string) []logRecord {
	t.Helper()
	var records []logRecord
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if line == "" {
			continue
		}
		if record := parseLogLine(t, line); strings.HasPrefix(record["msg"], "suggest") {
			records = append(records, record)
		}
	}
	return records
}

func suggestChatAnswer(t *testing.T, entries []map[string]any) []byte {
	t.Helper()
	inner, err := json.Marshal(map[string]any{"suggestions": entries})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": string(inner)}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return outer
}

func decodeSuggestRequest(t *testing.T, body []byte) suggest.UserPayload {
	t.Helper()
	var chat suggest.ChatRequest
	if err := json.Unmarshal(body, &chat); err != nil {
		t.Fatal(err)
	}
	var payload suggest.UserPayload
	if err := json.Unmarshal([]byte(chat.Messages[1].Content), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestServeSuggestsAfterIngestReplacesSnapshotKeepsPendingToPostedAndNeverReasks(t *testing.T) {
	files := newServedFiles(t)
	november := func(day int) time.Time { return time.Date(2026, time.November, day, 15, 0, 0, 0, time.UTC) }

	// Seed a database with a budget and one existing example for MERCADO so it
	// lands in the queue with low confidence, while PADARIA is a new establishment.
	budgetedDatabase(t, files.databasePath)
	db, err := authored.Open(t.Context(), files.databasePath, authored.Options{Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	view := db.View()
	budget, _ := view.BudgetOf(t.Context(), "2026-11")
	view.Close()
	var mercadoID authored.LineID
	for _, line := range budget.Lines {
		if line.Name == "Mercado" {
			mercadoID = line.ID
		}
	}
	if err := db.Change(t.Context(), authored.ChangeFacts{Author: authored.AuthorPerson, Method: "test.seed", At: testNow}, func(recorder *authored.Recorder) error {
		key, err := recorder.IndexTransaction(t.Context(), authoredtest.Seen("tx-seed-mercado", "2026-10-15", -5000, "text:MERCADO "+ledgerSentinel))
		if err != nil {
			return err
		}
		return recorder.SettleLine(t.Context(), key, mercadoID, authored.AuthorPerson)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	pendingRow := cardPurchase(pendingPurchase, november(16), "FEIRA "+ledgerSentinel, 4187.39)
	pendingRow.Status = "PENDING"
	nightOneRows := append(novemberRows(), pendingRow)
	inventedNight(t, files.ledgerPath, 1, nightOneRows, nil)

	var (
		mu             sync.Mutex
		askedPerCall   []int
		allDescriptions []string
	)
	fakeModel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		payload := decodeSuggestRequest(t, body)
		mu.Lock()
		askedPerCall = append(askedPerCall, len(payload.Purchases))
		entries := make([]map[string]any, 0, len(payload.Purchases))
		for _, p := range payload.Purchases {
			allDescriptions = append(allDescriptions, p.Description)
			entries = append(entries, map[string]any{
				"n":             p.N,
				"line":          "Mercado",
				"confidence":    86,
				"justification": "Compra em comércio de alimentação.",
			})
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(suggestChatAnswer(t, entries))
	}))
	defer fakeModel.Close()

	vars := serveVars(t)
	vars["FINWISE_MODEL_URL"] = fakeModel.URL
	vars["FINWISE_MODEL_NAME"] = "fake-model"
	vars["FINWISE_MODEL_KEY"] = "test-model-secret-key"

	inspector := inspectDatabase(t, files.databasePath)
	suggestionCount := func() string {
		return strings.Join(columnValues(t, inspector, "SELECT count(*) FROM model_suggestions"), "")
	}

	server := startServer(t, files.serveArgs("--dev-insecure-cookie", "--ingest-interval", "20ms"), vars)
	// Night 1 has 3 establishment purchases (PADARIA, MERCADO, pending FEIRA) and 1 person PIX (excluded).
	waitFor(t, "night 1 suggestions", func() bool { return suggestionCount() == "3" })

	cookie, _ := server.unlockBody(t, testPIN)
	queueBody := server.postAs(t, cookie, "/api/review/weekly", `["2026-11"]`)
	var weekly struct {
		Queue struct {
			Items []struct {
				TransactionID string `json:"transactionId"`
				Person        bool   `json:"person"`
				Pending       struct {
					Reason        string  `json:"reason"`
					Suggestion    *string `json:"suggestion"`
					Confidence    *int    `json:"confidence"`
					Justification *string `json:"justification"`
				} `json:"pending"`
			} `json:"items"`
		} `json:"queue"`
	}
	if err := json.Unmarshal([]byte(queueBody), &weekly); err != nil {
		t.Fatalf("decode queue: %v\n%s", err, queueBody)
	}
	byID := map[string]int{}
	for i, item := range weekly.Queue.Items {
		byID[item.TransactionID] = i
	}
	for _, id := range []string{firstPurchase, "tx-" + ledgerSentinel + "-2", pendingPurchase} {
		idx, ok := byID[id]
		if !ok {
			t.Fatalf("queue missing %s", id)
		}
		p := weekly.Queue.Items[idx].Pending
		if p.Suggestion == nil || *p.Suggestion != "Mercado" || p.Confidence == nil || *p.Confidence != 86 ||
			p.Justification == nil || *p.Justification != "Compra em comércio de alimentação." {
			t.Errorf("%s pending = %+v, want Mercado 86 with justification", id, p)
		}
	}
	if personIdx, ok := byID["tx-"+ledgerSentinel+"-3"]; !ok || weekly.Queue.Items[personIdx].Pending.Suggestion != nil {
		t.Errorf("person transfer got a suggestion: %+v", weekly.Queue.Items[personIdx].Pending)
	}

	// Night 2: pendingPurchase posts as postedPurchase, plus one new purchase arrives.
	postedRow := cardPurchase(postedPurchase, november(17), "FEIRA "+ledgerSentinel, 4187.39)
	newRow := cardPurchase("tx-"+ledgerSentinel+"-night2", november(18), "ACOUGUE "+ledgerSentinel, 85.00)
	nightTwoRows := append(novemberRows(), postedRow, newRow)
	inventedNight(t, files.ledgerPath, 2, nightTwoRows, nil)

	waitFor(t, "night 2 suggestion", func() bool { return suggestionCount() == "4" })

	// Posted purchase must keep its suggestion from Night 1.
	queueBody2 := server.postAs(t, cookie, "/api/review/weekly", `["2026-11"]`)
	weekly.Queue.Items = nil
	if err := json.Unmarshal([]byte(queueBody2), &weekly); err != nil {
		t.Fatal(err)
	}
	foundPosted := false
	for _, item := range weekly.Queue.Items {
		if item.TransactionID == postedPurchase {
			foundPosted = true
			if item.Pending.Suggestion == nil || *item.Pending.Suggestion != "Mercado" || item.Pending.Justification == nil {
				t.Errorf("posted purchase lost suggestion: %+v", item.Pending)
			}
		}
	}
	if !foundPosted {
		t.Error("posted purchase not found in queue")
	}

	result := server.stopGracefully(t)
	mu.Lock()
	gotCalls := slices.Clone(askedPerCall)
	gotDescs := slices.Clone(allDescriptions)
	mu.Unlock()
	if !slices.Equal(gotCalls, []int{3, 1}) {
		t.Errorf("asked per model call = %v, want [3 1] (no re-asking)", gotCalls)
	}
	for _, desc := range gotDescs {
		if strings.Contains(desc, "FULANA") {
			t.Errorf("person transfer description sent to model: %q", desc)
		}
	}
	requireOnlyCountsAndKinds(t, "serve", result)
	if strings.Contains(result.stdout+result.stderr, "test-model-secret-key") {
		t.Error("model API key leaked to output")
	}
	recs := suggestRecords(t, result.stdout)
	if len(recs) != 2 || recs[0]["asked"] != "3" || recs[1]["asked"] != "1" {
		t.Errorf("suggest log records = %+v, want 2 records with asked 3 then 1", recs)
	}
}

func TestBudgetSaveAndWriteReturnWithinOneSecondWhileFakeModelHoldsForFive(t *testing.T) {
	files := newServedFiles(t)
	inventedNight(t, files.ledgerPath, 1, novemberRows(), nil)
	emptyDatabase(t, files.databasePath)

	var modelEntered atomic.Int32
	release := make(chan struct{})
	slowModel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		modelEntered.Add(1)
		select {
		case <-release:
			return
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
		}
		_, _ = w.Write(suggestChatAnswer(t, nil))
	}))
	defer func() {
		close(release)
		slowModel.Close()
	}()

	vars := serveVars(t)
	vars["FINWISE_MODEL_URL"] = slowModel.URL
	vars["FINWISE_MODEL_NAME"] = "slow-model"

	taken := runsTakenIn(t, files.databasePath)
	server := startServer(t, files.serveArgs("--dev-insecure-cookie", "--ingest-interval", "1h"), vars)
	waitFor(t, "initial ingest", func() bool { return slices.Equal(taken(), []string{"1"}) })

	cookie, _ := server.unlockBody(t, testPIN)

	// 1. budget.save triggers ingest (budget-change) -> model runs in background.
	start := time.Now()
	status, answer := server.postStatus(t, cookie, "/api/budget/save", novemberBudget)
	elapsed := time.Since(start)
	if status != http.StatusOK || !strings.HasPrefix(answer, `{"changeId":"`) {
		t.Fatalf("budget.save answered %d %s", status, answer)
	}
	if elapsed >= time.Second {
		t.Errorf("budget.save took %v while model was held for 5s, want < 1s", elapsed)
	}
	waitFor(t, "model call started after budget.save", func() bool { return modelEntered.Load() >= 1 })

	// 2. Replace snapshot to run 2 and issue a write (trigger write) while model is still held.
	november := func(day int) time.Time { return time.Date(2026, time.November, day, 15, 0, 0, 0, time.UTC) }
	extraRow := cardPurchase("tx-"+ledgerSentinel+"-write", november(19), "CAFE "+ledgerSentinel, 19.90)
	inventedNight(t, files.ledgerPath, 2, append(novemberRows(), extraRow), nil)

	start = time.Now()
	status, answer = server.postStatus(t, cookie, "/api/transactions/addTag", `["`+firstPurchase+`","cafe"]`)
	elapsed = time.Since(start)
	if status != http.StatusOK || !strings.HasPrefix(answer, `{"changeId":"`) {
		t.Fatalf("addTag answered %d %s", status, answer)
	}
	if elapsed >= time.Second {
		t.Errorf("write triggering ingest took %v while model was held for 5s, want < 1s", elapsed)
	}

	// Graceful stop cancels the 5-second model call instead of waiting 5 seconds.
	stopStart := time.Now()
	server.stopGracefully(t)
	if stopElapsed := time.Since(stopStart); stopElapsed >= 2*time.Second {
		t.Errorf("graceful stop took %v, expected in-flight model call to be cancelled immediately", stopElapsed)
	}
}

func TestServeLogsFailuresRecordsMetaAndRetriesOnNextSnapshot(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	cases := []struct {
		name     string
		wantKind string
		setup    func(t *testing.T, ok *atomic.Bool) (url string, timeout time.Duration)
	}{
		{
			name: "status", wantKind: "status",
			setup: func(t *testing.T, ok *atomic.Bool) (string, time.Duration) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if !ok.Load() {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					body, _ := io.ReadAll(r.Body)
					payload := decodeSuggestRequest(t, body)
					var entries []map[string]any
					for _, p := range payload.Purchases {
						entries = append(entries, map[string]any{"n": p.N, "line": "Mercado", "confidence": 80, "justification": "Ok."})
					}
					_, _ = w.Write(suggestChatAnswer(t, entries))
				}))
				t.Cleanup(srv.Close)
				return srv.URL, 5 * time.Second
			},
		},
		{
			name: "timeout", wantKind: "timeout",
			setup: func(t *testing.T, ok *atomic.Bool) (string, time.Duration) {
				release := make(chan struct{})
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, _ := io.ReadAll(r.Body)
					if !ok.Load() {
						select {
						case <-release:
						case <-r.Context().Done():
						case <-time.After(2 * time.Second):
						}
						return
					}
					payload := decodeSuggestRequest(t, body)
					var entries []map[string]any
					for _, p := range payload.Purchases {
						entries = append(entries, map[string]any{"n": p.N, "line": "Mercado", "confidence": 80, "justification": "Ok."})
					}
					_, _ = w.Write(suggestChatAnswer(t, entries))
				}))
				t.Cleanup(func() {
					close(release)
					srv.Close()
				})
				return srv.URL, 60 * time.Millisecond
			},
		},
		{
			name: "unreachable", wantKind: "unreachable",
			setup: func(t *testing.T, _ *atomic.Bool) (string, time.Duration) {
				return closedURL, 5 * time.Second
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := newServedFiles(t)
			inventedNight(t, files.ledgerPath, 1, novemberRows(), nil)
			budgetedDatabase(t, files.databasePath)

			var recoverModel atomic.Bool
			url, timeout := c.setup(t, &recoverModel)
			vars := serveVars(t)
			vars["FINWISE_MODEL_URL"] = url
			vars["FINWISE_MODEL_NAME"] = "test-model"

			inspector := inspectDatabase(t, files.databasePath)
			metaRun := func() string {
				return strings.Join(columnValues(t, inspector, "SELECT value FROM meta WHERE key = 'last_model_run'"), "")
			}

			server := startServerWithTimeout(t, files.serveArgs("--ingest-interval", "20ms"), vars, timeout)
			waitFor(t, "failed model run in meta", func() bool { return strings.Contains(metaRun(), `"ok":false`) })
			if count := columnValues(t, inspector, "SELECT count(*) FROM model_suggestions"); !slices.Equal(count, []string{"0"}) {
				t.Errorf("failed run stored rows: %v", count)
			}

			if c.name != "unreachable" {
				// Flip model to healthy and replace snapshot to run 2 -> retries and succeeds.
				recoverModel.Store(true)
				inventedNight(t, files.ledgerPath, 2, novemberRows(), nil)
				waitFor(t, "retry succeeded", func() bool { return strings.Contains(metaRun(), `"ok":true`) })
				if count := columnValues(t, inspector, "SELECT count(*) FROM model_suggestions"); !slices.Equal(count, []string{"2"}) {
					t.Errorf("retry stored %v rows, want 2", count)
				}
			}

			result := server.stopGracefully(t)
			requireOnlyCountsAndKinds(t, c.name, result)
			recs := suggestRecords(t, result.stdout)
			if len(recs) == 0 || recs[0]["msg"] != "suggest failed" || recs[0]["kind"] != c.wantKind {
				t.Errorf("suggest records = %+v, want first to be suggest failed kind=%s", recs, c.wantKind)
			}
		})
	}
}

func TestServeModelConfigurationLoggingAndRefusal(t *testing.T) {
	// Unconfigured: logs message once, sends no request.
	files := newServedFiles(t)
	inventedNight(t, files.ledgerPath, 1, novemberRows(), nil)
	budgetedDatabase(t, files.databasePath)
	result := startServer(t, files.serveArgs(), serveVars(t)).stopGracefully(t)
	if count := strings.Count(result.stdout, noModelConfiguredMessage); count != 1 {
		t.Errorf("unconfigured message logged %d times, want once:\n%s", count, result.stdout)
	}

	// Misconfigured URL/name combinations refuse start without repeating values or creating DB.
	cases := map[string]struct {
		vars      map[string]string
		wantNames []string
		forbid    []string
	}{
		"url without name": {
			vars:      map[string]string{"FINWISE_MODEL_URL": "https://secret-host.example/v1"},
			wantNames: []string{"FINWISE_MODEL_URL", "FINWISE_MODEL_NAME"},
			forbid:    []string{"secret-host.example"},
		},
		"name without url": {
			vars:      map[string]string{"FINWISE_MODEL_NAME": "secret-model-xyz"},
			wantNames: []string{"FINWISE_MODEL_URL", "FINWISE_MODEL_NAME"},
			forbid:    []string{"secret-model-xyz"},
		},
		"http non-loopback host": {
			vars: map[string]string{
				"FINWISE_MODEL_URL":  "http://remote-secret.example/v1",
				"FINWISE_MODEL_NAME": "secret-model-xyz",
				"FINWISE_MODEL_KEY":  "secret-key-999",
			},
			wantNames: []string{"FINWISE_MODEL_URL"},
			forbid:    []string{"remote-secret.example", "secret-model-xyz", "secret-key-999"},
		},
		"http non-loopback ip": {
			vars: map[string]string{
				"FINWISE_MODEL_URL":  "http://192.168.1.50:11434/v1",
				"FINWISE_MODEL_NAME": "local-model",
			},
			wantNames: []string{"FINWISE_MODEL_URL"},
			forbid:    []string{"192.168.1.50", "local-model"},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "finwise.db")
			vars := serveVars(t)
			for k, v := range c.vars {
				vars[k] = v
			}
			res := refuseToStart(t, []string{"serve", "--db", path}, vars)
			combined := res.stdout + res.stderr
			for _, want := range c.wantNames {
				if !strings.Contains(res.stderr, want) {
					t.Errorf("stderr %q does not name %s", res.stderr, want)
				}
			}
			for _, secret := range c.forbid {
				if strings.Contains(combined, secret) {
					t.Errorf("output leaked %q: %q", secret, combined)
				}
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("database was created on invalid model config: %v", err)
			}
		})
	}

	// Loopback HTTP and HTTPS are allowed.
	for _, good := range []string{
		"https://api.example.com/v1",
		"http://127.0.0.1:11434/v1",
		"http://localhost:11434/v1",
		"http://[::1]:11434/v1",
	} {
		if !allowedModelURL(good) {
			t.Errorf("allowedModelURL(%q) = false, want true", good)
		}
	}
}

func TestSuggestCommandCountsExitCodesNoCreateAndNoLeaks(t *testing.T) {
	directory := t.TempDir()
	ledgerPath := filepath.Join(directory, "pluggy-snapshot.db")
	databasePath := filepath.Join(directory, "finwise.db")
	inventedNight(t, ledgerPath, 1, novemberRows(), nil)
	budgetedDatabase(t, databasePath)

	// Take the snapshot in first so the purchases are pending.
	if res := runCommand(context.Background(), []string{"ingest", "--db", databasePath, "--ledger", ledgerPath}, "", nil, nil); res.code != 0 {
		t.Fatalf("ingest failed: %d %s", res.code, res.stderr)
	}

	var calls atomic.Int32
	fakeModel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		payload := decodeSuggestRequest(t, body)
		var entries []map[string]any
		for _, p := range payload.Purchases {
			entries = append(entries, map[string]any{
				"n":             p.N,
				"line":          "Mercado",
				"confidence":    85,
				"justification": "Sugestão para " + ledgerSentinel,
			})
		}
		_, _ = w.Write(suggestChatAnswer(t, entries))
	}))
	defer fakeModel.Close()

	vars := map[string]string{
		"FINWISE_MODEL_URL":  fakeModel.URL,
		"FINWISE_MODEL_NAME": "fake-model",
		"FINWISE_MODEL_KEY":  "secret-cli-key",
	}

	// First run: asks both pending purchases, prints counts, exits 0, leaks nothing.
	args := []string{"suggest", "--db", databasePath, "--ledger", ledgerPath}
	first := runCommand(context.Background(), args, "", vars, nil)
	if first.code != 0 || first.stderr != "" {
		t.Fatalf("first suggest exited %d: %q", first.code, first.stderr)
	}
	requireOnlyCountsAndKinds(t, "suggest first", first)
	if strings.Contains(first.stdout+first.stderr, "secret-cli-key") {
		t.Error("model key leaked")
	}
	recs := suggestRecords(t, first.stdout)
	if len(recs) != 1 || recs[0]["trigger"] != "command" || recs[0]["requests"] != "1" || recs[0]["asked"] != "2" || recs[0]["suggested"] != "2" || recs[0]["unusable"] != "0" {
		t.Errorf("first suggest line = %+v", recs)
	}

	// Second run: nothing left to ask, prints zero counts, exits 0, sends 0 HTTP requests.
	second := runCommand(context.Background(), args, "", vars, nil)
	if second.code != 0 || second.stderr != "" || calls.Load() != 1 {
		t.Fatalf("second suggest exited %d stderr=%q calls=%d", second.code, second.stderr, calls.Load())
	}
	recs2 := suggestRecords(t, second.stdout)
	if len(recs2) != 1 || recs2[0]["requests"] != "0" || recs2[0]["asked"] != "0" {
		t.Errorf("second suggest line = %+v", recs2)
	}

	// No model configured -> exit 1.
	noModel := runCommand(context.Background(), args, "", nil, nil)
	if noModel.code != exitFailure || !strings.Contains(noModel.stderr, "FINWISE_MODEL_URL") {
		t.Errorf("no model: exit %d stderr %q", noModel.code, noModel.stderr)
	}

	// Missing --db -> exit 2 with usage line.
	noDB := runCommand(context.Background(), []string{"suggest", "--ledger", ledgerPath}, "", vars, nil)
	if noDB.code != exitUsage || !strings.Contains(noDB.stderr, suggestUsage) {
		t.Errorf("no --db: exit %d stderr %q", noDB.code, noDB.stderr)
	}

	// Missing DB file -> exit 1 and does NOT create it.
	missingDB := filepath.Join(t.TempDir(), "missing.db")
	missing := runCommand(context.Background(), []string{"suggest", "--db", missingDB, "--ledger", ledgerPath}, "", vars, nil)
	if missing.code != exitFailure || !strings.Contains(missing.stderr, missingDB) {
		t.Errorf("missing db: exit %d stderr %q", missing.code, missing.stderr)
	}
	if _, err := os.Stat(missingDB); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("suggest created missing database: %v", err)
	}

	// Top-level usage names suggest.
	topUsage := runCommand(context.Background(), nil, "", nil, nil)
	if topUsage.code != exitUsage || !strings.Contains(topUsage.stderr, "| suggest") {
		t.Errorf("top usage: %q", topUsage.stderr)
	}
}
