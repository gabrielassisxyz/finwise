package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// A handler reads a person's whole financial history and its arguments name
// rows, so nothing it sees may reach a log. The request log is the only writer,
// and it records the route, the status and the kind of error, never a body or
// an argument.

// writers are the files allowed to name the logging packages.
var writers = map[string]bool{"logging.go": true, "router.go": true}

func TestOnlyTheRequestLogWritesAnything(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || writers[file] {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			if path := strings.Trim(spec.Path.Value, `"`); path == "log" || path == "log/slog" {
				t.Errorf("%s imports %s", file, path)
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok && writesOutput(call) {
				t.Errorf("%s writes to the process's output", file)
			}
			return true
		})
	}
}

func writesOutput(call *ast.CallExpr) bool {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		return function.Name == "print" || function.Name == "println"
	case *ast.SelectorExpr:
		pkg, ok := function.X.(*ast.Ident)
		return ok && pkg.Name == "fmt" && (strings.HasPrefix(function.Sel.Name, "Print") || strings.HasPrefix(function.Sel.Name, "Fprint"))
	}
	return false
}

// secrets are strings only the fixtures, the invented authored state and the
// arguments of the cases hold: none of them may reach a log. Every argument of a
// write case that is text a person would have typed, or that names a row, holds
// one of them.
var secrets = []string{
	"tx-", "acc-", "card-itau", "LIVRARIA", "EXEMPLO", "Exemplo", "2026-02", "no-such", "12345678",
	"fact-", "nota-", "tag-",
}

// capturedLogs is what reached each of the three places a log could go.
type capturedLogs struct {
	defaultLog, standardLog, request []byte
}

// captureLogs runs work with the default slog logger and the standard log's
// output captured. work adds the request log of each router it builds.
func captureLogs(work func(addRequestLog func([]byte))) capturedLogs {
	var defaultLogs, standardLogs, requestLog bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&defaultLogs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	log.SetOutput(&standardLogs)
	defer func() {
		slog.SetDefault(previous)
		log.SetOutput(os.Stderr)
	}()
	work(func(logged []byte) { requestLog.Write(logged) })
	return capturedLogs{defaultLog: defaultLogs.Bytes(), standardLog: standardLogs.Bytes(), request: requestLog.Bytes()}
}

// leakProblems names every place that holds a secret, and the two default
// loggers if they received a single byte.
func leakProblems(logs capturedLogs) []string {
	var problems []string
	for label, logged := range map[string][]byte{"default slog": logs.defaultLog, "log": logs.standardLog, "request log": logs.request} {
		for _, secret := range secrets {
			if bytes.Contains(logged, []byte(secret)) {
				problems = append(problems, label+` holds "`+secret+`"`)
			}
		}
	}
	if len(logs.defaultLog) != 0 || len(logs.standardLog) != 0 {
		problems = append(problems, fmt.Sprintf("a handler wrote %d and %d bytes to the default loggers", len(logs.defaultLog), len(logs.standardLog)))
	}
	sort.Strings(problems)
	return problems
}

// runEveryCase answers every read case over both ledgers, every authored read
// case over the third state and every write case, the harness's own included.
func runEveryCase(t *testing.T, addRequestLog func([]byte)) {
	t.Helper()
	for _, name := range []string{"full", "empty"} {
		env := newReadEnv(t, ledgerNamed(t, name))
		for _, c := range readCases {
			env.answer(c.method, c.arguments)
		}
		addRequestLog(env.logs.Bytes())
	}
	authoredEnv := newAuthoredReadEnv(t)
	for _, c := range authoredReadCases {
		authoredEnv.answer(c.method, c.arguments)
	}
	addRequestLog(authoredEnv.logs.Bytes())
	for _, c := range allWriteCases() {
		addRequestLog(runWriteCase(t, c, theRealUndo).requestLog)
	}
}

// TestNoHandlerLogsWhatItReads runs every case and looks for the arguments and
// for text only the fixtures hold in every place a log could go.
func TestNoHandlerLogsWhatItReads(t *testing.T) {
	logs := captureLogs(func(addRequestLog func([]byte)) { runEveryCase(t, addRequestLog) })
	for _, problem := range leakProblems(logs) {
		t.Error(problem)
	}
	if len(logs.request) == 0 {
		t.Error("no request was logged, so the run saw nothing")
	}
}

func TestEveryTextArgumentOfAWriteCaseIsOneTheLeakTestLooksFor(t *testing.T) {
	var calls []string
	for _, c := range allWriteCases() {
		calls = append(calls, c.arguments)
		for _, call := range c.before {
			calls = append(calls, call.arguments)
		}
	}
	for _, unlisted := range unlistedArguments(secrets, calls) {
		t.Errorf("%q is typed text or names a row and holds no string of the leak test's list", unlisted)
	}
	if len(unlistedArguments(secrets, []string{`["fact-a","unlisted text"]`, `[{"nested":["Lazer"]}]`, `["2026-03","7"]`})) != 2 {
		t.Error("the check does not tell typed text from a month or a number")
	}
}

// A handler made to log one of its arguments is caught in each of the places a
// log could go, and the cases that do not log are not accused.
func TestTheLeakCheckSeesAHandlerThatLogsItsArguments(t *testing.T) {
	logging := map[string]func(argument string){
		"slog": func(argument string) { slog.Info("saved", "argument", argument) },
		"log":  func(argument string) { log.Print("saved " + argument) },
	}
	for label, emit := range logging {
		c := singleCase
		c.handlers = withProofHandlers(func(registry *Registry, _ *authoredtest.Scratch) {
			registry.RegisterWrite(setFactMethod, func(ctx context.Context, args []json.RawMessage, writes Writes) (any, error) {
				fact, _, err := twoStrings(args)
				if err != nil {
					return nil, err
				}
				emit(fact)
				return setFactAmount(fact, 40)(ctx, args, writes)
			})
		})
		logs := captureLogs(func(addRequestLog func([]byte)) { addRequestLog(runWriteCase(t, c, theRealUndo).requestLog) })
		if len(leakProblems(logs)) == 0 {
			t.Errorf("a handler that logs through %s went unnoticed", label)
		}
	}
	clean := captureLogs(func(addRequestLog func([]byte)) { addRequestLog(runWriteCase(t, singleCase, theRealUndo).requestLog) })
	if problems := leakProblems(clean); len(problems) != 0 {
		t.Errorf("a case that does not log was accused: %v", problems)
	}
}
