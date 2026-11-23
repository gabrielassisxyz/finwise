package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// waitFor polls until condition holds, and fails the test after ten seconds.
func waitFor(t *testing.T, label string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen", label)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// runsTakenIn is the run meta records as taken in, read again at each call
// over one connection the test keeps.
func runsTakenIn(t *testing.T, path string) func() []string {
	t.Helper()
	inspector := inspectDatabase(t, path)
	return func() []string {
		return columnValues(t, inspector, "SELECT value FROM meta WHERE key = 'last_ingested_run'")
	}
}

// countingIngest is the real ingest, counted.
func countingIngest(calls *atomic.Int32) ingestFunc {
	return func(ctx context.Context, sources ingest.Sources, trigger ingest.Trigger, now time.Time) (ingest.Counts, error) {
		calls.Add(1)
		return ingest.Run(ctx, sources, trigger, now)
	}
}

func emptyDatabase(t *testing.T, path string) {
	t.Helper()
	database, err := authored.Open(t.Context(), path, authored.Options{Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}

type servedFiles struct{ ledgerPath, databasePath string }

func newServedFiles(t *testing.T) servedFiles {
	directory := t.TempDir()
	return servedFiles{filepath.Join(directory, "pluggy-snapshot.db"), filepath.Join(directory, "finwise.db")}
}

func (files servedFiles) serveArgs(extra ...string) []string {
	return append([]string{"serve", "--db", files.databasePath, "--ledger", files.ledgerPath}, extra...)
}

func requireTriggers(t *testing.T, records []logRecord, want ...string) {
	t.Helper()
	var got []string
	for _, record := range records {
		got = append(got, record["trigger"])
	}
	if !slices.Equal(got, want) {
		t.Errorf("the lines about a run were for %v, want %v", got, want)
	}
}

func TestServeTakesTheRunInAtStartWithoutARequest(t *testing.T) {
	files := newServedFiles(t)
	inventedNight(t, files.ledgerPath, 1, novemberRows(), nil)
	budgetedDatabase(t, files.databasePath)
	taken := runsTakenIn(t, files.databasePath)

	server := startServer(t, files.serveArgs(), serveVars(t))
	waitFor(t, "the start's run", func() bool { return slices.Equal(taken(), []string{"1"}) })
	result := server.stopGracefully(t)

	requireOnlyCountsAndKinds(t, "serve", result)
	records := ingestRecords(t, result.stdout)
	requireTriggers(t, records, "start")
	if len(records) == 1 && records[0]["indexed"] != "3" {
		t.Errorf("the start evaluated %s transactions, want the three of November", records[0]["indexed"])
	}
}

// Between the two nights the timer ticks over a run already taken in, which
// writes nothing: the log holds one line for the start and one for the tick
// that took the second night in.
func TestServeTakesALaterRunInOnItsTimerAndATickWithNothingNewLogsNothing(t *testing.T) {
	files := newServedFiles(t)
	inventedNight(t, files.ledgerPath, 1, novemberRows(), nil)
	budgetedDatabase(t, files.databasePath)
	taken := runsTakenIn(t, files.databasePath)
	var calls atomic.Int32

	server := startServerWith(t, files.serveArgs("--ingest-interval", "20ms"), serveVars(t), countingIngest(&calls))
	waitFor(t, "the start's run", func() bool { return slices.Equal(taken(), []string{"1"}) })
	waitFor(t, "three ticks over the same night", func() bool { return calls.Load() >= 4 })
	inventedNight(t, files.ledgerPath, 2, novemberRows(), nil)
	waitFor(t, "the second night taken in", func() bool { return slices.Equal(taken(), []string{"2"}) })
	afterSecond := calls.Load()
	waitFor(t, "three more ticks", func() bool { return calls.Load() >= afterSecond+3 })
	result := server.stopGracefully(t)

	stopped := calls.Load()
	time.Sleep(100 * time.Millisecond)
	if later := calls.Load(); later != stopped {
		t.Errorf("the timer ran %d times after the server stopped", later-stopped)
	}
	requireOnlyCountsAndKinds(t, "serve", result)
	requireTriggers(t, ingestRecords(t, result.stdout), "start", "tick")
}

func TestServeStartsWithoutTheSnapshotAndTakesItInOnceItAppears(t *testing.T) {
	files := newServedFiles(t)
	emptyDatabase(t, files.databasePath)
	taken := runsTakenIn(t, files.databasePath)

	server := startServer(t, files.serveArgs("--ingest-interval", "20ms"), serveVars(t))
	health, err := http.Get("http://" + server.address + "/healthz")
	if err != nil || health.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %v %v", err, health)
	}
	health.Body.Close()
	inventedNight(t, files.ledgerPath, 1, novemberRows(), nil)
	waitFor(t, "the snapshot taken in once it appeared", func() bool { return slices.Equal(taken(), []string{"1"}) })
	result := server.stopGracefully(t)

	requireOnlyCountsAndKinds(t, "serve", result)
	records := ingestRecords(t, result.stdout)
	if len(records) < 2 || records[0]["trigger"] != "start" || records[0]["kind"] != "snapshot-unavailable" {
		t.Fatalf("the log does not begin with the start's failure by its kind: %v", records)
	}
	if last := records[len(records)-1]; last["msg"] != "ingest" || last["trigger"] != "tick" {
		t.Errorf("the last line about a run is %v, want the tick that took the snapshot in", last)
	}
}

// The cause of this failure quotes the row's id, which the log must not.
func TestServeLogsARowItCannotReadByTheKindAlone(t *testing.T) {
	files := newServedFiles(t)
	inventedNight(t, files.ledgerPath, 1, novemberRows(), func(f *ledgertest.Fixture) { f.CorruptTransactionDate(firstPurchase, "not a date") })
	budgetedDatabase(t, files.databasePath)

	result := startServer(t, files.serveArgs(), serveVars(t)).stopGracefully(t)

	requireOnlyCountsAndKinds(t, "serve", result)
	records := ingestRecords(t, result.stdout)
	if len(records) != 1 || records[0]["kind"] != "ledger-read" {
		t.Errorf("the lines about a run are %v, want the start's failure of kind ledger-read", records)
	}
}

func TestServeRefusesAnIngestIntervalThatIsNotAPositiveDurationWithoutRepeatingIt(t *testing.T) {
	for _, value := range []string{"soon", "0s", "-1m"} {
		for _, place := range []string{"flag", "environment"} {
			path := filepath.Join(t.TempDir(), "finwise.db")
			args, vars := []string{"serve", "--db", path}, serveVars(t)
			if place == "flag" {
				args = append(args, "--ingest-interval", value)
			} else {
				vars["FINWISE_INGEST_INTERVAL"] = value
			}
			result := refuseToStart(t, args, vars)
			if output := result.stdout + result.stderr; strings.Contains(output, value) || !strings.Contains(output, "ingest interval") {
				t.Errorf("%s %q: the output %q repeats the value or does not name the setting", place, value, output)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s %q: the database was created: %v", place, value, err)
			}
		}
	}
}

func TestTheIngestIntervalComesFromTheFlagThenTheEnvironmentThenTenMinutes(t *testing.T) {
	cases := []struct {
		args     []string
		variable string
		want     time.Duration
	}{
		{nil, "", 10 * time.Minute},
		{nil, "90s", 90 * time.Second},
		{[]string{"--ingest-interval", "2m"}, "90s", 2 * time.Minute},
	}
	for _, c := range cases {
		vars := serveVars(t)
		vars["FINWISE_INGEST_INTERVAL"] = c.variable
		var stderr strings.Builder
		env := environment{stderr: &stderr, getenv: func(name string) string { return vars[name] }}
		settings, code := readServeSettings(c.args, env)
		if code != 0 || settings.ingestInterval != c.want {
			t.Errorf("%v with %q: interval %v (exit %d %q), want %v", c.args, c.variable, settings.ingestInterval, code, stderr.String(), c.want)
		}
	}
}

func TestWithoutADatabaseServeRunsNoIngest(t *testing.T) {
	var calls atomic.Int32
	never := func(context.Context, ingest.Sources, ingest.Trigger, time.Time) (ingest.Counts, error) {
		calls.Add(1)
		return ingest.Counts{}, nil
	}
	ledgerPath := filepath.Join(t.TempDir(), "pluggy-snapshot.db")
	inventedNight(t, ledgerPath, 1, novemberRows(), nil)
	server := startServerWith(t, []string{"serve", "--ledger", ledgerPath, "--ingest-interval", "5ms"}, serveVars(t), never)
	time.Sleep(100 * time.Millisecond)
	result := server.stopGracefully(t)

	if calls.Load() != 0 || len(ingestRecords(t, result.stdout)) != 0 {
		t.Errorf("a server without a database ran the ingest %d times: %q", calls.Load(), result.stdout)
	}
	env := environment{now: func() time.Time { return testNow }, runIngest: never}
	if config := apiConfig(serveSettings{ledgerPath: ledgerPath}, nil, env, nil); config.AfterBudgetSaved != nil {
		t.Error("without a database the API was given an ingest to run after a save")
	}
}

// writeLock is a write transaction of the test's own on the app's database,
// which a run waits for at its begin, after it has read the ledger.
type writeLock struct {
	pool *sql.DB
	conn *sql.Conn
}

func holdWriteLock(t *testing.T, path string) *writeLock {
	t.Helper()
	pool, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(t.Context(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	lock := &writeLock{pool: pool, conn: conn}
	t.Cleanup(lock.release)
	return lock
}

func (lock *writeLock) release() {
	if lock.conn == nil {
		return
	}
	_, _ = lock.conn.ExecContext(context.Background(), "ROLLBACK")
	lock.conn.Close()
	lock.pool.Close()
	lock.conn = nil
}

func TestAShutdownDuringARunWaitsForItToCommitAndClosesTheDatabase(t *testing.T) {
	files := newServedFiles(t)
	inventedNight(t, files.ledgerPath, 1, novemberRows(), nil)
	budgetedDatabase(t, files.databasePath)
	lock := holdWriteLock(t, files.databasePath)

	server := startServer(t, files.serveArgs(), serveVars(t))
	server.stop()
	select {
	case result := <-server.done:
		t.Fatalf("serve returned %d while its start's run was held between its read and its write", result.code)
	case <-time.After(500 * time.Millisecond):
	}
	lock.release()
	var result outcome
	select {
	case result = <-server.done:
	case <-time.After(15 * time.Second):
		t.Fatal("the server did not stop once the run could go on")
	}

	if result.code != 0 {
		t.Fatalf("the shutdown exited %d: %s", result.code, result.stderr)
	}
	if _, err := os.Stat(files.databasePath + "-wal"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the database was not closed cleanly: its -wal file is still there (%v)", err)
	}
	requireTriggers(t, ingestRecords(t, result.stdout), "start")
	inspector := inspectDatabase(t, files.databasePath)
	if run := columnValues(t, inspector, "SELECT value FROM meta WHERE key = 'last_ingested_run'"); !slices.Equal(run, []string{"1"}) {
		t.Errorf("the run held at shutdown recorded %v, want it committed", run)
	}
	if integrity := columnValues(t, inspector, "PRAGMA integrity_check"); !slices.Equal(integrity, []string{"ok"}) {
		t.Errorf("integrity_check: %v", integrity)
	}
}

// What a timer run may do wrong is played by a fake: the first run panics with
// a value that quotes the ledger, the second fails, the others take longer
// than the interval.
func TestTheTimerRunsOneAtATimeOutlivesItsFailuresAndStopsWithTheServer(t *testing.T) {
	var (
		mutex           sync.Mutex
		calls, inFlight int
		mostAtOnce      int
		triggers        []ingest.Trigger
	)
	fake := func(_ context.Context, _ ingest.Sources, trigger ingest.Trigger, _ time.Time) (ingest.Counts, error) {
		mutex.Lock()
		calls++
		call := calls
		inFlight++
		mostAtOnce = max(mostAtOnce, inFlight)
		triggers = append(triggers, trigger)
		mutex.Unlock()
		defer func() {
			mutex.Lock()
			inFlight--
			mutex.Unlock()
		}()
		switch call {
		case 1:
			panic("row " + firstPurchase + " of PADARIA " + ledgerSentinel)
		case 2:
			return ingest.Counts{}, &ingest.Error{Failure: ingest.FailureDatabase}
		}
		time.Sleep(30 * time.Millisecond)
		return ingest.Counts{Indexed: 1, Applied: 1}, nil
	}
	callsSoFar := func() int {
		mutex.Lock()
		defer mutex.Unlock()
		return calls
	}
	files := newServedFiles(t)

	server := startServerWith(t, files.serveArgs("--ingest-interval", "5ms"), serveVars(t), fake)
	waitFor(t, "six runs", func() bool { return callsSoFar() >= 6 })
	health, err := http.Get("http://" + server.address + "/healthz")
	if err != nil || health.StatusCode != http.StatusOK {
		t.Fatalf("healthz after a panic and a failure: %v %v", err, health)
	}
	health.Body.Close()
	result := server.stopGracefully(t)
	stopped := callsSoFar()
	time.Sleep(50 * time.Millisecond)

	if later := callsSoFar(); later != stopped {
		t.Errorf("the timer ran %d times after the server stopped", later-stopped)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if mostAtOnce != 1 {
		t.Errorf("%d runs were in progress at once", mostAtOnce)
	}
	if triggers[0] != ingest.TriggerStart || slices.ContainsFunc(triggers[1:], func(trigger ingest.Trigger) bool { return trigger != ingest.TriggerTick }) {
		t.Errorf("the runs were called for %v, want the start and then ticks", triggers)
	}
	requireOnlyCountsAndKinds(t, "serve", result)
	records := ingestRecords(t, result.stdout)
	if len(records) < 3 || records[0]["kind"] != "panic" || records[1]["kind"] != "database" || records[2]["indexed"] != "1" {
		t.Errorf("the lines about a run begin %v, want the panic, the failure and then counts", records[:min(len(records), 3)])
	}
}

// A budget of November, the month of the invented transactions and the
// tracking start month of a database the test clock creates.
const novemberBudget = `["2026-11",[{"key":"rec","name":"Receita","lines":[{"name":"Salário","amount":10000}]},` +
	`{"key":"var","name":"Variáveis","lines":[{"name":"Mercado","amount":400}]}]]`

// indexState is how many transactions the index holds and how many of them
// were evaluated, as text.
func indexState(t *testing.T, inspector *sql.DB) string {
	t.Helper()
	state := columnValues(t, inspector, "SELECT count(*) || '/' || coalesce(sum(evaluated), 0) FROM tx_index")
	return strings.Join(state, "")
}

func TestABudgetSaveEvaluatesTheMonthItGaveALineBeforeItAnswers(t *testing.T) {
	files := newServedFiles(t)
	inventedNight(t, files.ledgerPath, 1, novemberRows(), nil)
	emptyDatabase(t, files.databasePath)
	taken := runsTakenIn(t, files.databasePath)
	inspector := inspectDatabase(t, files.databasePath)

	server := startServer(t, files.serveArgs(), serveVars(t))
	waitFor(t, "the start's run", func() bool { return slices.Equal(taken(), []string{"1"}) })
	if state := indexState(t, inspector); state != "0/0" {
		t.Fatalf("before any budget the index holds %s transactions, want none", state)
	}
	cookie, _ := server.unlockBody(t, testPIN)
	answer := server.postAs(t, cookie, "/api/budget/save", novemberBudget)
	// Read at once: the save answered only after its ingest.
	state := indexState(t, inspector)
	result := server.stopGracefully(t)

	if !strings.HasPrefix(answer, `{"changeId":"`) {
		t.Fatalf("the save answered %s", answer)
	}
	if state != "3/3" {
		t.Errorf("when the save answered the index held %s transactions evaluated, want the three of November and not October's", state)
	}
	requireOnlyCountsAndKinds(t, "serve", result)
	records := ingestRecords(t, result.stdout)
	requireTriggers(t, records, "start", "budget-change")
	if len(records) == 2 && records[1]["indexed"] != "3" {
		t.Errorf("the run after the save evaluated %s transactions, want 3", records[1]["indexed"])
	}
}

func TestABudgetSaveIsAnsweredAndKeptWhenItsIngestFails(t *testing.T) {
	files := newServedFiles(t)
	emptyDatabase(t, files.databasePath)

	server := startServer(t, files.serveArgs(), serveVars(t))
	cookie, _ := server.unlockBody(t, testPIN)
	answer := server.postAs(t, cookie, "/api/budget/save", novemberBudget)
	result := server.stopGracefully(t)

	if !strings.HasPrefix(answer, `{"changeId":"`) {
		t.Errorf("a save whose ingest failed answered %s", answer)
	}
	if methods := columnValues(t, inspectDatabase(t, files.databasePath), "SELECT method FROM changes"); !slices.Equal(methods, []string{"budget.save"}) {
		t.Errorf("the changes kept are %v, want the save", methods)
	}
	requireOnlyCountsAndKinds(t, "serve", result)
	failed := slices.ContainsFunc(ingestRecords(t, result.stdout), func(record logRecord) bool {
		return record["trigger"] == "budget-change" && record["kind"] == "snapshot-unavailable"
	})
	if !failed {
		t.Errorf("the failure of the ingest after the save was not logged by its kind: %q", result.stdout)
	}
}
