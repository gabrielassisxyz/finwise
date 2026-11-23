package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
	_ "modernc.org/sqlite"
)

// refuseToStart runs serve and stops it at once if it does start listening, so
// a database that is wrongly accepted shows as a listening server, not a hang.
func refuseToStart(t *testing.T, args []string, vars map[string]string) outcome {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listened := false
	result := runCommand(ctx, args, "", refusalVars(vars), func(string) { listened = true; cancel() })
	if listened || result.code == 0 {
		t.Fatalf("serve started (listened %v, exit %d)", listened, result.code)
	}
	return result
}

func (server *runningServer) stopGracefully(t *testing.T) outcome {
	t.Helper()
	server.stop()
	select {
	case result := <-server.done:
		if result.code != 0 {
			t.Fatalf("graceful shutdown exited %d: %s", result.code, result.stderr)
		}
		return result
	case <-time.After(15 * time.Second):
		t.Fatal("the server did not stop")
	}
	return outcome{}
}

func serveVars(t *testing.T) map[string]string {
	return map[string]string{
		"FINWISE_PIN_HASH": hashOf(t), "FINWISE_LEDGER": "/nonexistent/pluggy-snapshot.db", "FINWISE_LISTEN": "127.0.0.1:0",
	}
}

func listDirectory(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func inspectDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { database.Close() })
	return database
}

func columnValues(t *testing.T, database *sql.DB, query string) []string {
	t.Helper()
	rows, err := database.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	return values
}

// requireCreatedDatabase checks a stopped server left the database the embedded
// migration set creates.
func requireCreatedDatabase(t *testing.T, path string) {
	t.Helper()
	journal, err := os.ReadFile(filepath.Join("..", "..", "internal", "authored", "migrations", "0001_journal.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(journal)
	settings, err := os.ReadFile(filepath.Join("..", "..", "internal", "authored", "migrations", "0004_settings.sql"))
	if err != nil {
		t.Fatal(err)
	}
	settingsSum := sha256.Sum256(settings)
	pots, err := os.ReadFile(filepath.Join("..", "..", "internal", "authored", "migrations", "0005_pots.sql"))
	if err != nil {
		t.Fatal(err)
	}
	potsSum := sha256.Sum256(pots)
	subscriptions, err := os.ReadFile(filepath.Join("..", "..", "internal", "authored", "migrations", "0006_subscriptions.sql"))
	if err != nil {
		t.Fatal(err)
	}
	subscriptionsSum := sha256.Sum256(subscriptions)
	session, err := os.ReadFile(filepath.Join("..", "..", "internal", "authored", "migrations", "0008_session.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sessionSum := sha256.Sum256(session)
	budget, err := os.ReadFile(filepath.Join("..", "..", "internal", "authored", "migrations", "0002_budget.sql"))
	if err != nil {
		t.Fatal(err)
	}
	budgetSum := sha256.Sum256(budget)
	transactions, err := os.ReadFile(filepath.Join("..", "..", "internal", "authored", "migrations", "0003_transactions.sql"))
	if err != nil {
		t.Fatal(err)
	}
	transactionsSum := sha256.Sum256(transactions)
	review, err := os.ReadFile(filepath.Join("..", "..", "internal", "authored", "migrations", "0007_review.sql"))
	if err != nil {
		t.Fatal(err)
	}
	reviewSum := sha256.Sum256(review)
	database := inspectDatabase(t, path)
	var id int64
	var mode string
	if err := database.QueryRow("PRAGMA application_id").Scan(&id); err != nil || id != 1179209303 {
		t.Errorf("application_id = %d (%v)", id, err)
	}
	if err := database.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q (%v)", mode, err)
	}
	tables := columnValues(t, database, "SELECT name FROM sqlite_schema WHERE type = 'table' ORDER BY name")
	if want := []string{"budget_version_lines", "budget_versions", "change_effects", "changes", "closed_months", "establishment_tags", "establishments", "history_step_reasons", "history_steps", "lines", "meta", "pin", "pot_lines", "pot_settings", "scenarios", "schema_migrations", "settings", "sqlite_sequence", "subscriptions", "tx_ids", "tx_index", "tx_split_parts", "tx_state", "tx_tags",
		"closing_steps", "dismissed_warnings", "transfer_marks"}; !slices.Equal(tables, slices.Sorted(slices.Values(want))) {
		t.Errorf("tables = %v, want %v", tables, want)
	}
	recorded := columnValues(t, database, "SELECT name || '=' || sha256 FROM schema_migrations ORDER BY name")
	if want := []string{"0001_journal=" + hex.EncodeToString(sum[:]), "0002_budget=" + hex.EncodeToString(budgetSum[:]), "0003_transactions=" + hex.EncodeToString(transactionsSum[:]), "0004_settings=" + hex.EncodeToString(settingsSum[:]), "0005_pots=" + hex.EncodeToString(potsSum[:]), "0006_subscriptions=" + hex.EncodeToString(subscriptionsSum[:]), "0008_session=" + hex.EncodeToString(sessionSum[:]),
		"0007_review=" + hex.EncodeToString(reviewSum[:])}; !slices.Equal(recorded, slices.Sorted(slices.Values(want))) {
		t.Errorf("schema_migrations = %v, want %v", recorded, want)
	}
	month := columnValues(t, database, "SELECT value FROM meta WHERE key = 'tracking_start_month'")
	if !slices.Equal(month, []string{"2026-11"}) {
		t.Errorf("tracking start month = %v, want the month of the test clock in Brazil", month)
	}
}

func TestServeWithoutADatabaseCreatesNoFile(t *testing.T) {
	workingDirectory, ledgerDirectory := t.TempDir(), t.TempDir()
	t.Chdir(workingDirectory)
	vars := serveVars(t)
	vars["FINWISE_LEDGER"] = filepath.Join(ledgerDirectory, "pluggy-snapshot.db")
	startServer(t, []string{"serve"}, vars).stopGracefully(t)
	for _, directory := range []string{workingDirectory, ledgerDirectory} {
		if files := listDirectory(t, directory); len(files) != 0 {
			t.Errorf("%s holds %v", directory, files)
		}
	}
}

func TestServeCreatesAMissingDatabaseAndStopsCleanly(t *testing.T) {
	for name, prepare := range map[string]func(path string) error{
		"missing":    func(string) error { return nil },
		"zero bytes": func(path string) error { return os.WriteFile(path, nil, 0o600) },
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "finwise.db")
			if err := prepare(path); err != nil {
				t.Fatal(err)
			}
			server := startServer(t, []string{"serve", "--db", path}, serveVars(t))
			if _, body := server.post(t, "/api/session/status", `[]`); !strings.Contains(body, "locked") {
				t.Errorf("session.status answered %s", body)
			}
			server.stopGracefully(t)
			requireCreatedDatabase(t, path)
		})
	}
}

func TestTheEnvironmentNamesTheDatabaseAndTheFlagWins(t *testing.T) {
	fromVariable := filepath.Join(t.TempDir(), "finwise.db")
	vars := serveVars(t)
	vars["FINWISE_DB"] = fromVariable
	startServer(t, []string{"serve"}, vars).stopGracefully(t)
	requireCreatedDatabase(t, fromVariable)

	ignored, fromFlag := filepath.Join(t.TempDir(), "finwise.db"), filepath.Join(t.TempDir(), "finwise.db")
	vars["FINWISE_DB"] = ignored
	startServer(t, []string{"serve", "--db", fromFlag}, vars).stopGracefully(t)
	requireCreatedDatabase(t, fromFlag)
	if files := listDirectory(t, filepath.Dir(ignored)); len(files) != 0 {
		t.Errorf("the variable's path was used although the flag was given: %v", files)
	}
}

func TestServeRefusesADatabaseItCannotCreate(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "absent", "finwise.db")
	result := refuseToStart(t, []string{"serve", "--db", path}, serveVars(t))
	if !strings.Contains(result.stderr, path) {
		t.Errorf("stderr %q does not name the path", result.stderr)
	}
	if files := listDirectory(t, parent); len(files) != 0 {
		t.Errorf("a directory was created: %v", files)
	}

	vars := serveVars(t)
	delete(vars, "FINWISE_PIN_HASH")
	path = filepath.Join(t.TempDir(), "finwise.db")
	refuseToStart(t, []string{"serve", "--db", path}, vars)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a database was created without a PIN hash: %v", err)
	}
}

const sentinel = "sentinel-5b2d-never-printed"

// foreignFile builds, alone in a directory, a file that is not finwise's.
func foreignFile(t *testing.T, kind string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finwise.db")
	switch kind {
	case "collector snapshot":
		snapshot := ledgertest.New(t)
		snapshot.AddItem(ledgertest.Item{ID: "item-1", Connector: "Test Bank", Status: "UPDATED", ExecutionStatus: "SUCCESS"})
		snapshot.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: sentinel, Currency: "BRL"})
		return snapshot.Path
	case "write-ahead, -wal never checkpointed":
		return foreignWithPendingWAL(t)
	case "text":
		writeTestFile(t, path, strings.Repeat("a note that is not a database, "+sentinel+"\n", 4))
	case "not SQLite, our id at bytes 68 to 71":
		content := []byte(strings.Repeat(".", 128))
		copy(content, "plain text, "+sentinel)
		copy(content[68:], "FINW")
		writeTestFile(t, path, string(content))
	case "ten bytes":
		writeTestFile(t, path, "SENTINEL!!")
	default:
		database, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer database.Close()
		database.SetMaxOpenConns(1)
		statements := map[string][]string{
			"write-ahead":      {"PRAGMA journal_mode = WAL", "CREATE TABLE notes (body TEXT)", "INSERT INTO notes VALUES ('" + sentinel + "')"},
			"application id 1": {"PRAGMA application_id = 1", "CREATE TABLE notes (body TEXT)", "INSERT INTO notes VALUES ('" + sentinel + "')"},
			"no table, no id":  {"PRAGMA user_version = 3"},
		}[kind]
		for _, statement := range statements {
			if _, err := database.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	return path
}

// foreignWithPendingWAL is a database whose last writes are only in its -wal,
// as a crash leaves it: opening it as a database would recover the -wal and,
// on close, checkpoint it into the main file and remove it.
func foreignWithPendingWAL(t *testing.T) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), "source.db")
	database, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	for _, statement := range []string{
		"PRAGMA journal_mode = WAL", "PRAGMA wal_autocheckpoint = 0",
		"CREATE TABLE notes (body TEXT)", "INSERT INTO notes VALUES ('" + sentinel + "')",
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "finwise.db")
	for _, suffix := range []string{"", "-wal"} {
		content, err := os.ReadFile(source + suffix)
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, path+suffix, string(content))
	}
	return path
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestServeRefusesEveryFileThatIsNotItsOwnAndLeavesItUntouched(t *testing.T) {
	for _, kind := range []string{"write-ahead", "write-ahead, -wal never checkpointed", "collector snapshot", "application id 1", "no table, no id", "text", "not SQLite, our id at bytes 68 to 71", "ten bytes"} {
		t.Run(kind, func(t *testing.T) {
			path := foreignFile(t, kind)
			directory := filepath.Dir(path)
			hashBefore, filesBefore := fileHash(t, path), listDirectory(t, directory)

			result := refuseToStart(t, []string{"serve", "--db", path}, serveVars(t))
			output := result.stdout + result.stderr
			if !strings.Contains(result.stderr, path) || strings.Contains(output, sentinel) || strings.Contains(output, "SENTINEL") {
				t.Errorf("the message must name the path and nothing from the file: %q", result.stderr)
			}
			if !strings.Contains(result.stderr, "is not finwise's database") {
				t.Errorf("the file was not refused as foreign: %q", result.stderr)
			}
			if hashAfter := fileHash(t, path); hashAfter != hashBefore {
				t.Errorf("the file changed: %s -> %s", hashBefore, hashAfter)
			}
			if filesAfter := listDirectory(t, directory); !slices.Equal(filesBefore, filesAfter) {
				t.Errorf("the directory changed: %v -> %v", filesBefore, filesAfter)
			}
		})
	}
}

// A file stamped as finwise's that already holds a table of the first
// migration's makes that migration fail with the embedded set.
func TestServeExitsNamingThePathWhenAMigrationFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finwise.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"PRAGMA application_id = 1179209303", "CREATE TABLE changes (id INTEGER)"} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	database.Close()

	result := refuseToStart(t, []string{"serve", "--db", path}, serveVars(t))
	if !strings.Contains(result.stderr, path) || !strings.Contains(result.stderr, "0001_journal") {
		t.Errorf("stderr %q does not name the path and the migration", result.stderr)
	}
	if recorded := columnValues(t, inspectDatabase(t, path), "SELECT name FROM sqlite_schema WHERE name = 'schema_migrations'"); len(recorded) != 0 {
		t.Error("the failed migration left its record behind")
	}
}

// The host's clock is read in one place, where main builds the environment;
// everything else takes the environment's, so a test can fix it.
func TestTheHostsClockIsReadOnlyWhereMainBuildsTheEnvironment(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			function, isFunction := declaration.(*ast.FuncDecl)
			ast.Inspect(declaration, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if owner, ok := selector.X.(*ast.Ident); ok && owner.Name == "time" && selector.Sel.Name == "Now" {
					where := path
					if isFunction {
						where += ":" + function.Name.Name
					}
					found = append(found, where)
				}
				return true
			})
		}
	}
	if !slices.Equal(found, []string{"main.go:main"}) {
		t.Errorf("time.Now is named in %v, want only main.go:main", found)
	}
}
