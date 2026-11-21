package api

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// TestNoHandlerLogsWhatItReads runs every case and looks for the arguments and
// for text only the fixture ledger holds in every place a log could go.
func TestNoHandlerLogsWhatItReads(t *testing.T) {
	var defaultLogs, standardLogs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&defaultLogs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	log.SetOutput(&standardLogs)
	t.Cleanup(func() {
		slog.SetDefault(previous)
		log.SetOutput(os.Stderr)
	})

	var requestLog bytes.Buffer
	for _, name := range []string{"full", "empty"} {
		env := newReadEnv(t, ledgerNamed(t, name))
		for _, c := range readCases {
			env.answer(c.method, c.arguments)
		}
		requestLog.Write(env.logs.Bytes())
	}

	secrets := []string{"tx-", "acc-", "card-itau", "LIVRARIA", "EXEMPLO", "Exemplo", "2026-02", "no-such", "12345678"}
	for label, logged := range map[string][]byte{"default slog": defaultLogs.Bytes(), "log": standardLogs.Bytes(), "request log": requestLog.Bytes()} {
		for _, secret := range secrets {
			if bytes.Contains(logged, []byte(secret)) {
				t.Errorf("%s holds %q", label, secret)
			}
		}
	}
	if defaultLogs.Len() != 0 || standardLogs.Len() != 0 {
		t.Errorf("a handler wrote %d and %d bytes to the default loggers", defaultLogs.Len(), standardLogs.Len())
	}
}
