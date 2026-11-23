package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// Everything in the invented ledger of these tests is made up, and looks like
// what a bank sends: every id, account, name and description carries
// ledgerSentinel, and the amounts, documents and the card number are values no
// log line holds otherwise, so a line that quotes any of them shows.
const ledgerSentinel = "SENTINELA-7Q4K"

var leakedValues = []string{
	ledgerSentinel, "sentinela", "Fulana",
	"4187.39", "418739", "2950.64", "295064", "1312.58", "131258", "877.21", "87721",
	"11122233344", "111.222.333-44", "55544433322", "555.444.333-22", "5555666677778888",
}

const (
	cardAccount = "acc-card-" + ledgerSentinel
	bankAccount = "acc-bank-" + ledgerSentinel
	// firstPurchase is the row whose date the reader is made unable to read.
	firstPurchase = "tx-" + ledgerSentinel + "-1"
)

func inventedAccounts(f *ledgertest.Fixture) {
	f.AddItem(ledgertest.Item{ID: "item-" + ledgerSentinel, Connector: "Banco " + ledgerSentinel, Status: "UPDATED", ExecutionStatus: "SUCCESS"})
	f.AddIdentity(ledgertest.Identity{ID: "identity-" + ledgerSentinel, ItemID: "item-" + ledgerSentinel,
		Document: "555.444.333-22", FullName: "Titular " + ledgerSentinel})
	f.AddAccount(ledgertest.Account{ID: bankAccount, ItemID: "item-" + ledgerSentinel, Type: "BANK", Subtype: "CHECKING_ACCOUNT",
		Name: "Conta " + ledgerSentinel, Currency: "BRL", Balance: 877.21})
	f.AddAccount(ledgertest.Account{ID: cardAccount, ItemID: "item-" + ledgerSentinel, Type: "CREDIT", Subtype: "CREDIT_CARD",
		Name: "Cartão " + ledgerSentinel, Currency: "BRL", Balance: 877.21})
}

func cardPurchase(id string, date time.Time, description string, reais float64) ledgertest.Transaction {
	return ledgertest.Transaction{ID: id, AccountID: cardAccount, Date: date, Description: description,
		Amount: reais, Currency: "BRL", Type: "DEBIT",
		Raw: map[string]any{"operationType": "PAGAMENTO", "creditCardMetadata": map[string]any{"cardNumber": "5555666677778888"}}}
}

// novemberRows are three transactions of November 2026, the tracking start
// month of a database the test clock creates, and one of October before it.
func novemberRows() []ledgertest.Transaction {
	november := func(day int) time.Time { return time.Date(2026, time.November, day, 15, 0, 0, 0, time.UTC) }
	return []ledgertest.Transaction{
		cardPurchase(firstPurchase, november(10), "PADARIA "+ledgerSentinel, 4187.39),
		cardPurchase("tx-"+ledgerSentinel+"-2", november(12), "MERCADO "+ledgerSentinel, 2950.64),
		{ID: "tx-" + ledgerSentinel + "-3", AccountID: bankAccount, Date: november(14), Description: "PIX FULANA " + ledgerSentinel,
			Amount: -1312.58, Currency: "BRL", Type: "DEBIT", Raw: map[string]any{"operationType": "PIX", "paymentData": map[string]any{
				"paymentMethod": "PIX",
				"receiver":      map[string]any{"name": "Fulana " + ledgerSentinel, "documentNumber": map[string]any{"type": "CPF", "value": "111.222.333-44"}},
			}}},
		cardPurchase("tx-"+ledgerSentinel+"-october", time.Date(2026, time.October, 20, 15, 0, 0, 0, time.UTC), "LIVRARIA "+ledgerSentinel, 877.21),
	}
}

// inventedNight writes the snapshot of a night whose runs are 1 to runs, the
// last one projected, beside path and renames it over path, as the collector
// does, so a server reading path never sees it half written. finish, when
// given, alters the file once its rows are in.
func inventedNight(t *testing.T, path string, runs int, rows []ledgertest.Transaction, finish func(*ledgertest.Fixture)) {
	t.Helper()
	next := ledgertest.NewAt(t, path+".next")
	inventedAccounts(next)
	for _, row := range rows {
		next.AddTransaction(row)
	}
	last := int64(1)
	for n := 2; n <= runs; n++ {
		last = next.AddRun(ledgertest.Run{Outcome: "complete", FinishedAt: time.Date(2026, time.November, 20+n, 6, 0, 0, 0, time.UTC)})
	}
	if last > 1 {
		next.CoverInRun(last)
		next.ProjectTo(last)
	}
	if finish != nil {
		finish(next)
	}
	if err := os.Rename(next.Path, path); err != nil {
		t.Fatal(err)
	}
}

// budgetedDatabase creates the app's database at path as the test clock would,
// with a budget from November 2026 on, and closes it.
func budgetedDatabase(t *testing.T, path string) {
	t.Helper()
	database, err := authored.Open(t.Context(), path, authored.Options{Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	authoredtest.SaveBudget(t, database, testNow, "2026-11", authoredtest.InventedBudget())
}

// everythingIn is the content of every table of the database at path.
func everythingIn(t *testing.T, path string) string {
	t.Helper()
	database, err := authored.Open(t.Context(), path, authored.Options{Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	return (&authoredtest.Scratch{Database: database, Path: path}).Everything(t)
}

func ingestedRun(t *testing.T, path string) []string {
	t.Helper()
	return columnValues(t, inspectDatabase(t, path), "SELECT value FROM meta WHERE key = 'last_ingested_run'")
}

// logRecord is one line slog's text handler wrote, by key.
type logRecord map[string]string

// parseLogLine reads key=value pairs, a value quoted when slog quoted it.
func parseLogLine(t *testing.T, line string) logRecord {
	t.Helper()
	record := logRecord{}
	for rest := line; rest != ""; {
		key, value, found := strings.Cut(rest, "=")
		if !found || key == "" || strings.Contains(key, " ") {
			t.Fatalf("not a line of key=value pairs: %q", line)
		}
		if strings.HasPrefix(value, `"`) {
			quoted, err := strconv.QuotedPrefix(value)
			if err != nil {
				t.Fatalf("a quoted value does not end: %q", line)
			}
			record[key], _ = strconv.Unquote(quoted)
			rest = strings.TrimPrefix(value[len(quoted):], " ")
			continue
		}
		record[key], rest, _ = strings.Cut(value, " ")
	}
	return record
}

// ingestRecords are the lines about a run in what a command wrote.
func ingestRecords(t *testing.T, output string) []logRecord {
	t.Helper()
	var records []logRecord
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if line == "" {
			continue
		}
		if record := parseLogLine(t, line); strings.HasPrefix(record["msg"], "ingest") {
			records = append(records, record)
		}
	}
	return records
}

var countKeys = []string{"indexed", "paired", "unpaired", "applied", "pending_low_confidence",
	"pending_unnamed_person", "pending_new_establishment", "left", "nothing_to_store", "already_decided"}

var (
	triggerNames = []string{"start", "tick", "budget-change", "command", "write"}
	failureKinds = []string{"snapshot-unavailable", "ledger-read", "rows-unplaced", "database", "panic", "unknown"}
)

// shapeProblem says what in a line about a run is anything but its trigger,
// its counts or the kind of its failure; empty when nothing is.
func shapeProblem(record logRecord) string {
	keys := []string{"time", "level", "msg", "trigger"}
	switch record["msg"] {
	case "ingest":
		keys = append(append(keys, "stopped"), countKeys...)
		if record["level"] != "INFO" || (record["stopped"] != "true" && record["stopped"] != "false") {
			return "a counts line of level " + record["level"] + " and stopped " + record["stopped"]
		}
		for _, key := range countKeys {
			if count, err := strconv.Atoi(record[key]); err != nil || count < 0 {
				return key + " is not a count"
			}
		}
	case "ingest failed":
		keys = append(keys, "kind")
		if record["level"] != "WARN" || !slices.Contains(failureKinds, record["kind"]) {
			return "a failure line of level " + record["level"] + " and a kind of the wrong shape"
		}
	default:
		return "an unknown message"
	}
	if !slices.Contains(triggerNames, record["trigger"]) {
		return "an unknown trigger"
	}
	var got []string
	for key := range record {
		got = append(got, key)
	}
	slices.Sort(got)
	slices.Sort(keys)
	if !slices.Equal(got, keys) {
		return "keys " + strings.Join(got, ",") + ", want " + strings.Join(keys, ",")
	}
	return ""
}

// requireOnlyCountsAndKinds fails for any value of the invented ledger in the
// output and for a line about a run that holds anything but counts and kinds.
func requireOnlyCountsAndKinds(t *testing.T, label string, result outcome) {
	t.Helper()
	for _, value := range leakedValues {
		if strings.Contains(strings.ToLower(result.stdout+result.stderr), strings.ToLower(value)) {
			t.Errorf("%s: the output holds %q", label, value)
		}
	}
	for _, record := range ingestRecords(t, result.stdout) {
		if problem := shapeProblem(record); problem != "" {
			t.Errorf("%s: %s", label, problem)
		}
	}
}

func TestTheShapeCheckRefusesALineThatSaysMoreThanCountsAndKinds(t *testing.T) {
	counts := "time=2026-11-30T22:30:00.000-03:00 level=INFO msg=ingest trigger=command stopped=false indexed=3 paired=0 unpaired=0 applied=0 " +
		"pending_low_confidence=0 pending_unnamed_person=1 pending_new_establishment=2 left=0 nothing_to_store=0 already_decided=0"
	failure := `time=2026-11-30T22:30:00.000-03:00 level=WARN msg="ingest failed" trigger=tick kind=ledger-read`
	for _, line := range []string{counts, failure} {
		if problem := shapeProblem(parseLogLine(t, line)); problem != "" {
			t.Errorf("a line of counts or a kind was refused: %s: %q", problem, line)
		}
	}
	for _, line := range []string{
		counts + " account=acc-1",
		strings.Replace(counts, "indexed=3", "indexed=tx-1", 1),
		failure + ` error="ingest: the ledger snapshot could not be read; nothing was taken in"`,
		strings.Replace(failure, "kind=ledger-read", `kind="row tx-1: bad date"`, 1),
		strings.Replace(failure, "trigger=tick", "trigger=PADARIA", 1),
		`time=2026-11-30T22:30:00.000-03:00 level=INFO msg="ingest took PADARIA in"`,
	} {
		if shapeProblem(parseLogLine(t, line)) == "" {
			t.Errorf("a line that says more went unnoticed: %q", line)
		}
	}
}

func TestIngestTakesANewRunInAndGoesThroughARunAlreadyTakenIn(t *testing.T) {
	directory := t.TempDir()
	ledgerPath, databasePath := filepath.Join(directory, "pluggy-snapshot.db"), filepath.Join(directory, "finwise.db")
	inventedNight(t, ledgerPath, 1, novemberRows(), nil)
	budgetedDatabase(t, databasePath)
	args := []string{"ingest", "--db", databasePath, "--ledger", ledgerPath}

	for _, want := range []string{"3", "0"} {
		result := runCommand(context.Background(), args, "", nil, nil)
		if result.code != 0 || result.stderr != "" {
			t.Fatalf("ingest exited %d: %q", result.code, result.stderr)
		}
		requireOnlyCountsAndKinds(t, "ingest", result)
		records := ingestRecords(t, result.stdout)
		if len(records) != 1 || strings.Count(result.stdout, "\n") != 1 {
			t.Fatalf("ingest wrote %q, want one line of counts", result.stdout)
		}
		if got := records[0]; got["trigger"] != "command" || got["stopped"] != "false" || got["indexed"] != want {
			t.Errorf("the line reads trigger %s, stopped %s, indexed %s; want command, false, %s", got["trigger"], got["stopped"], got["indexed"], want)
		}
		if run := ingestedRun(t, databasePath); !slices.Equal(run, []string{"1"}) {
			t.Errorf("meta records the run %v, want 1", run)
		}
	}
}

func TestIngestReadsBothPathsFromTheEnvironmentAndCreatesAMissingDatabase(t *testing.T) {
	directory := t.TempDir()
	ledgerPath, databasePath := filepath.Join(directory, "pluggy-snapshot.db"), filepath.Join(directory, "finwise.db")
	inventedNight(t, ledgerPath, 1, novemberRows(), nil)
	vars := map[string]string{"FINWISE_DB": databasePath, "FINWISE_LEDGER": ledgerPath}

	if result := runCommand(context.Background(), []string{"ingest"}, "", vars, nil); result.code != 0 {
		t.Fatalf("ingest exited %d: %q", result.code, result.stderr)
	}
	requireCreatedDatabase(t, databasePath)
	if run := ingestedRun(t, databasePath); !slices.Equal(run, []string{"1"}) {
		t.Errorf("meta records the run %v, want 1", run)
	}
}

// Each broken snapshot comes after a night already taken in, so a failed run
// that wrote anything would show in a database that holds something.
func TestIngestFailsWithAFixedMessageNamingTheKindAndChangesNothing(t *testing.T) {
	cases := map[string]struct {
		breakIt func(t *testing.T, ledgerPath string)
		kind    string
	}{
		"the snapshot missing": {func(t *testing.T, ledgerPath string) {
			if err := os.Remove(ledgerPath); err != nil {
				t.Fatal(err)
			}
		}, "snapshot-unavailable"},
		"an unknown user_version": {func(t *testing.T, ledgerPath string) {
			inventedNight(t, ledgerPath, 2, novemberRows(), func(f *ledgertest.Fixture) { f.SetUserVersion(2) })
		}, "snapshot-unavailable"},
		"a row the reader cannot read": {func(t *testing.T, ledgerPath string) {
			inventedNight(t, ledgerPath, 2, novemberRows(), func(f *ledgertest.Fixture) { f.CorruptTransactionDate(firstPurchase, "not a date") })
		}, "ledger-read"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			ledgerPath, databasePath := filepath.Join(directory, "pluggy-snapshot.db"), filepath.Join(directory, "finwise.db")
			inventedNight(t, ledgerPath, 1, novemberRows(), nil)
			budgetedDatabase(t, databasePath)
			args := []string{"ingest", "--db", databasePath, "--ledger", ledgerPath}
			if result := runCommand(context.Background(), args, "", nil, nil); result.code != 0 {
				t.Fatalf("the first night was not taken in: %d %q", result.code, result.stderr)
			}
			before := everythingIn(t, databasePath)
			c.breakIt(t, ledgerPath)

			result := runCommand(context.Background(), args, "", nil, nil)

			if want := "finwise: the ingest failed: " + c.kind + "; nothing was taken in\n"; result.code != 1 || result.stderr != want || result.stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q; want 1 and only %q", result.code, result.stdout, result.stderr, want)
			}
			requireOnlyCountsAndKinds(t, name, result)
			if after := everythingIn(t, databasePath); after != before {
				t.Errorf("a failed run changed the database:\n%s\nwas\n%s", after, before)
			}
		})
	}
}

// The failure the leak test reads is one whose cause does quote the ledger, or
// it would pass whatever the command printed.
func TestTheUnreadableRowsCauseQuotesItsIDSoTheLeakTestCanSeeIt(t *testing.T) {
	directory := t.TempDir()
	ledgerPath := filepath.Join(directory, "pluggy-snapshot.db")
	inventedNight(t, ledgerPath, 1, novemberRows(), func(f *ledgertest.Fixture) { f.CorruptTransactionDate(firstPurchase, "not a date") })
	database := authoredtest.Fresh(t, func() time.Time { return testNow })
	_, err := ingest.Run(t.Context(), ingest.Sources{Ledger: ledger.New(ledgerPath), Database: database}, ingest.TriggerCommand, testNow)
	if cause := errors.Unwrap(err); cause == nil || !strings.Contains(cause.Error(), ledgerSentinel) {
		t.Fatalf("the cause of the failure does not quote the row: %v", cause)
	}
}

func TestIngestReportsAPanicByItsKindWithoutWhatItPanickedWith(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "finwise.db")
	panicking := func(context.Context, ingest.Sources, ingest.Trigger, time.Time) (ingest.Counts, error) {
		panic("row " + firstPurchase + " of PADARIA " + ledgerSentinel)
	}
	args := []string{"ingest", "--db", databasePath, "--ledger", filepath.Join(directory, "pluggy-snapshot.db")}

	result := runCommandWith(context.Background(), args, "", nil, nil, panicking)

	if want := "finwise: the ingest failed: panic; nothing was taken in\n"; result.code != 1 || result.stderr != want {
		t.Errorf("exit %d, stderr %q; want 1 and %q", result.code, result.stderr, want)
	}
	requireOnlyCountsAndKinds(t, "a panic", result)
}

func TestIngestWithoutBothPathsIsAUsageErrorThatCreatesNothing(t *testing.T) {
	directory := t.TempDir()
	databasePath, ledgerPath := filepath.Join(directory, "finwise.db"), filepath.Join(directory, "pluggy-snapshot.db")
	cases := map[string]struct {
		args []string
		vars map[string]string
	}{
		"neither":                    {[]string{"ingest"}, nil},
		"no database":                {[]string{"ingest", "--ledger", ledgerPath}, nil},
		"no ledger":                  {[]string{"ingest", "--db", databasePath}, nil},
		"no ledger, database by env": {[]string{"ingest"}, map[string]string{"FINWISE_DB": databasePath}},
		"an argument":                {[]string{"ingest", "--db", databasePath, "--ledger", ledgerPath, "now"}, nil},
		"an unknown flag":            {[]string{"ingest", "--db", databasePath, "--ledger", ledgerPath, "--nope"}, nil},
	}
	for name, c := range cases {
		result := runCommand(context.Background(), c.args, "", c.vars, nil)
		if result.code != exitUsage || result.stdout != "" {
			t.Errorf("%s: exit %d, stdout %q; want %d and nothing", name, result.code, result.stdout, exitUsage)
		}
	}
	if files := listDirectory(t, directory); len(files) != 0 {
		t.Errorf("a usage error left %v", files)
	}
}

func TestTheUsageLineNamesIngest(t *testing.T) {
	for _, args := range [][]string{nil, {"nope"}} {
		result := runCommand(context.Background(), args, "", nil, nil)
		if result.code != exitUsage || !strings.Contains(result.stderr, "| ingest") {
			t.Errorf("%v: exit %d, stderr %q", args, result.code, result.stderr)
		}
	}
}
